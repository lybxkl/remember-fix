package clipboard

// Clipboard 抽象系统剪贴板能力，供捕获器使用。
// Windows 实现见 windows.go；未来跨平台时在此接口下新增实现。
type Clipboard interface {
	// Text 读取剪贴板文本（无文本时 ok=false）。
	Text() (text string, ok bool)
	// ImagePNG 读取剪贴板图片并返回 PNG 字节（无图片时 ok=false）。
	ImagePNG() (png []byte, ok bool, err error)
	// Sequence 返回系统剪贴板序列号，变化意味着剪贴板内容更新过。
	Sequence() uint32
	// ForegroundWindowTitle 返回当前前台窗口标题（记录 bug 来源）。
	ForegroundWindowTitle() string
}
