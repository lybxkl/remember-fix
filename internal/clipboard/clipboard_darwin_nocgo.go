//go:build darwin && !cgo

package clipboard

// DarwinClipboard 在没有 cgo 时退化为空实现；正式 macOS 构建需要 CGO 以读取 NSPasteboard。
type DarwinClipboard struct{}

func NewDarwinClipboard() *DarwinClipboard {
	return &DarwinClipboard{}
}

func (c *DarwinClipboard) Text() (string, bool) {
	return "", false
}

func (c *DarwinClipboard) ImagePNG() ([]byte, bool, error) {
	return nil, false, nil
}

func (c *DarwinClipboard) Sequence() uint32 {
	return 0
}

func (c *DarwinClipboard) ForegroundWindowTitle() string {
	return ""
}
