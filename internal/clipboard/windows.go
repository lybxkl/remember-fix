//go:build windows

package clipboard

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	pOpenClipboard              = user32.NewProc("OpenClipboard")
	pCloseClipboard             = user32.NewProc("CloseClipboard")
	pGetClipboardData           = user32.NewProc("GetClipboardData")
	pIsClipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	pRegisterClipboardFormat    = user32.NewProc("RegisterClipboardFormatW")
	pGetClipboardSequenceNumber = user32.NewProc("GetClipboardSequenceNumber")
	pGetForegroundWindow        = user32.NewProc("GetForegroundWindow")
	pGetWindowTextW             = user32.NewProc("GetWindowTextW")

	pGlobalLock   = kernel32.NewProc("GlobalLock")
	pGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	pGlobalSize   = kernel32.NewProc("GlobalSize")
)

// Win32 标准剪贴板格式
const (
	cfUnicodeText = 13
	cfDIB         = 8
	cfDIBV5       = 17
)

// pngHeader 是 PNG 文件魔数，用于校验注册格式 "PNG" 的数据。
var pngHeader = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

type WindowsClipboard struct {
	mu sync.Mutex // 剪贴板是全局资源，串行访问
}

func NewWindowsClipboard() *WindowsClipboard {
	return &WindowsClipboard{}
}

// withClipboard 打开剪贴板执行 fn 后关闭。截图工具常短暂占用剪贴板，重试等待。
func (c *WindowsClipboard) withClipboard(fn func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		ret, _, err := pOpenClipboard.Call(0)
		if ret != 0 {
			defer func() {
				pCloseClipboard.Call()
			}()
			return fn()
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("打开剪贴板失败（可能被占用）: %w", lastErr)
}

func (c *WindowsClipboard) Sequence() uint32 {
	ret, _, _ := pGetClipboardSequenceNumber.Call()
	return uint32(ret)
}

func (c *WindowsClipboard) Text() (string, bool) {
	var result string
	var got bool
	err := c.withClipboard(func() error {
		if ret, _, _ := pIsClipboardFormatAvailable.Call(cfUnicodeText); ret == 0 {
			return nil
		}
		handle, _, _ := pGetClipboardData.Call(cfUnicodeText)
		if handle == 0 {
			return nil
		}
		ptr, _, _ := pGlobalLock.Call(handle)
		if ptr == 0 {
			return nil
		}
		defer pGlobalUnlock.Call(handle)
		size, _, _ := pGlobalSize.Call(handle)
		if size < 2 {
			return nil
		}
		// 以 GlobalSize 为上界扫描，避免访问越界；分配空间通常带尾部冗余。
		raw := sliceAt[uint16](ptr, int(size/2))
		n := 0
		for n < len(raw) && raw[n] != 0 {
			n++
		}
		if n == 0 {
			return nil
		}
		result = syscall.UTF16ToString(raw[:n])
		got = true
		return nil
	})
	if err != nil || !got {
		return "", false
	}
	return result, true
}

// ImagePNG 优先读截图工具放置的原始 "PNG" 注册格式（保 alpha、无损），
// 其次 CF_DIBV5，最后 CF_DIB，统一转成 PNG 字节。
func (c *WindowsClipboard) ImagePNG() ([]byte, bool, error) {
	var out []byte
	var ok bool
	var readErr error
	err := c.withClipboard(func() error {
		// 1) 注册格式 "PNG"（Win10/11 截图工具会放）
		if data, got := readPNGFormat(); got {
			out, ok = data, true
			return nil
		}
		// 2) CF_DIBV5
		if data, got := readHGlobal(cfDIBV5); got {
			if pngBytes, err := dibToPNG(data); err == nil {
				out, ok = pngBytes, true
				return nil
			} else {
				readErr = err
			}
		}
		// 3) CF_DIB
		if data, got := readHGlobal(cfDIB); got {
			if pngBytes, err := dibToPNG(data); err == nil {
				out, ok = pngBytes, true
				return nil
			} else {
				readErr = err
			}
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if !ok && readErr != nil {
		return nil, false, readErr
	}
	return out, ok, nil
}

// readPNGFormat 读取注册剪贴板格式 "PNG"。
func readPNGFormat() ([]byte, bool) {
	name, _ := windows.UTF16PtrFromString("PNG")
	format, _, _ := pRegisterClipboardFormat.Call(uintptr(unsafe.Pointer(name)))
	if format == 0 {
		return nil, false
	}
	data, ok := readHGlobal(uint32(format))
	if !ok || len(data) < 8 || !bytes.Equal(data[:8], pngHeader) {
		return nil, false
	}
	return data, true
}

// readHGlobal 读取指定格式句柄指向的全部字节。需在剪贴板打开状态下调用。
func readHGlobal(format uint32) ([]byte, bool) {
	if ret, _, _ := pIsClipboardFormatAvailable.Call(uintptr(format)); ret == 0 {
		return nil, false
	}
	handle, _, _ := pGetClipboardData.Call(uintptr(format))
	if handle == 0 {
		return nil, false
	}
	ptr, _, _ := pGlobalLock.Call(handle)
	if ptr == 0 {
		return nil, false
	}
	defer pGlobalUnlock.Call(handle)
	size, _, _ := pGlobalSize.Call(handle)
	if size == 0 {
		return nil, false
	}
	buf := make([]byte, size)
	copy(buf, sliceAt[byte](ptr, int(size)))
	return buf, true
}

// sliceAt 把 Win32 GlobalLock 返回的地址临时映射为 Go 切片。
// 声明期间剪贴板保持打开，内存由系统持有，指针有效。
//
//go:nocheckptr
func sliceAt[T any](ptr uintptr, n int) []T {
	return unsafe.Slice((*T)(unsafe.Pointer(ptr)), n)
}

// dibToPNG 解析 DIB（BITMAPINFOHEADER/V4/V5，24/32bpp，BI_RGB）并编码为 PNG。
func dibToPNG(dib []byte) ([]byte, error) {
	if len(dib) < 40 {
		return nil, fmt.Errorf("DIB 数据过短: %d 字节", len(dib))
	}
	headerSize := binary.LittleEndian.Uint32(dib[0:4])
	if headerSize < 40 || headerSize > 124 {
		return nil, fmt.Errorf("不支持的 DIB 头大小: %d", headerSize)
	}
	if int(headerSize) > len(dib) {
		return nil, fmt.Errorf("DIB 头不完整")
	}
	width := int(int32(binary.LittleEndian.Uint32(dib[4:8])))
	heightRaw := int32(binary.LittleEndian.Uint32(dib[8:12]))
	bitCount := binary.LittleEndian.Uint16(dib[14:16])
	compression := binary.LittleEndian.Uint32(dib[16:20])
	if compression != 0 && compression != 3 { // BI_RGB / BI_BITFIELDS
		return nil, fmt.Errorf("不支持的 DIB 压缩方式: %d", compression)
	}
	if width <= 0 || heightRaw == 0 {
		return nil, fmt.Errorf("非法 DIB 尺寸: %dx%d", width, heightRaw)
	}
	bottomUp := heightRaw > 0
	height := int(heightRaw)
	if !bottomUp {
		height = -height
	}
	if bitCount != 24 && bitCount != 32 {
		return nil, fmt.Errorf("不支持的 DIB 位深: %d", bitCount)
	}
	if compression == 3 && bitCount != 32 {
		return nil, fmt.Errorf("不支持的 BITFIELDS 位深: %d", bitCount)
	}
	bpp := int(bitCount)
	stride := (width*bpp + 31) / 32 * 4
	pixStart := int(headerSize)
	// BITMAPINFOHEADER + BI_BITFIELDS：三色掩码紧跟在 40 字节头之后。
	if compression == 3 && headerSize == 40 {
		pixStart += 12
	}
	if pixStart+stride*height > len(dib) {
		return nil, fmt.Errorf("DIB 像素数据不完整: 需要 %d 只有 %d", pixStart+stride*height, len(dib))
	}

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	allAlphaZero := bpp == 32
	for row := 0; row < height; row++ {
		srcRow := row
		if bottomUp {
			srcRow = height - 1 - row
		}
		src := dib[pixStart+srcRow*stride:]
		dst := img.Pix[row*img.Stride:]
		for x := 0; x < width; x++ {
			if bpp == 32 {
				b, g, r, a := src[x*4], src[x*4+1], src[x*4+2], src[x*4+3]
				if a != 0 {
					allAlphaZero = false
				}
				dst[x*4+0] = r
				dst[x*4+1] = g
				dst[x*4+2] = b
				dst[x*4+3] = a
			} else {
				b, g, r := src[x*3], src[x*3+1], src[x*3+2]
				dst[x*4+0] = r
				dst[x*4+1] = g
				dst[x*4+2] = b
				dst[x*4+3] = 0xFF
			}
		}
	}
	// 部分软件写入 CF_DIBV5 时 alpha 通道全为 0，实际应视为不透明。
	if allAlphaZero {
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 0xFF
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("PNG 编码失败: %w", err)
	}
	return buf.Bytes(), nil
}

func (c *WindowsClipboard) ForegroundWindowTitle() string {
	hwnd, _, _ := pGetForegroundWindow.Call()
	if hwnd == 0 {
		return ""
	}
	var buf [512]uint16
	ret, _, _ := pGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if ret == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:ret])
}
