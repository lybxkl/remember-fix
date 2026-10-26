//go:build windows

package clipboard

// New 返回当前平台可用的剪贴板实现。
func New() Clipboard {
	return NewWindowsClipboard()
}
