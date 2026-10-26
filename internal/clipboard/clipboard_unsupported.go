//go:build !windows && !darwin

package clipboard

// unsupportedClipboard 让非 Windows/macOS 平台仍可编译（应用本身只支持这两个平台）。
type unsupportedClipboard struct{}

func New() Clipboard {
	return unsupportedClipboard{}
}

func (unsupportedClipboard) Text() (string, bool) {
	return "", false
}

func (unsupportedClipboard) ImagePNG() ([]byte, bool, error) {
	return nil, false, nil
}

func (unsupportedClipboard) Sequence() uint32 {
	return 0
}

func (unsupportedClipboard) ForegroundWindowTitle() string {
	return ""
}
