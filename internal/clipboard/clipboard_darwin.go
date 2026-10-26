//go:build darwin && cgo

package clipboard

/*
#cgo CFLAGS: -mmacosx-version-min=12.0
#cgo LDFLAGS: -framework Cocoa -mmacosx-version-min=12.0
#include <stdlib.h>
#include "clipboard_darwin.h"
*/
import "C"

import (
	"sync"
	"unsafe"
)

// DarwinClipboard 通过 NSPasteboard 读取剪贴板；图片统一转成 PNG。
type DarwinClipboard struct {
	mu sync.Mutex
}

func NewDarwinClipboard() *DarwinClipboard {
	return &DarwinClipboard{}
}

func (c *DarwinClipboard) Sequence() uint32 {
	return uint32(C.rf_pasteboard_change_count())
}

func (c *DarwinClipboard) Text() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ptr := C.rf_pasteboard_text()
	if ptr == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(ptr))
	text := C.GoString(ptr)
	return text, text != ""
}

func (c *DarwinClipboard) ImagePNG() ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var length C.size_t
	ptr := C.rf_pasteboard_png(&length)
	if ptr == nil || length == 0 {
		return nil, false, nil
	}
	defer C.free(ptr)
	return C.GoBytes(ptr, C.int(length)), true, nil
}

func (c *DarwinClipboard) ForegroundWindowTitle() string {
	ptr := C.rf_frontmost_app()
	if ptr == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(ptr))
	return C.GoString(ptr)
}
