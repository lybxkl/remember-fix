// Package capture 负责剪贴板快照抓取与历史管理。
package capture

import (
	"log/slog"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/lybxkl/remember-fix/internal/clipboard"
	"github.com/lybxkl/remember-fix/internal/fsx"
)

const (
	historyCap  = 10                     // 剪贴板历史容量
	textLimit   = 4000                   // 文本条目最大字符数
	pollEvery   = 400 * time.Millisecond // 序列号轮询间隔
	lateCheckIn = 350 * time.Millisecond // 截图工具写剪贴板的延迟兜底窗口
)

// Item 是一条剪贴板捕获内容。
type Item struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // image | text
	Path      string `json:"path"` // image: 相对数据目录路径
	Text      string `json:"text"` // type=text 时的内容
	Seq       uint32 `json:"seq"`
	At        int64  `json:"at"`
	SourceApp string `json:"sourceApp"`
}

// Snapshot 是捕获窗打开时的完整数据：历史条目（新→旧）+ 默认选中项。
type Snapshot struct {
	SelectedID string `json:"selectedId"`
	Items      []Item `json:"items"`
}

type Capturer struct {
	clip       clipboard.Clipboard
	mu         sync.Mutex // 保护 history / lastSeq
	grabMu     sync.Mutex // 串行化剪贴板读取，避免并发重复落盘
	history    []Item     // 新→旧
	lastSeq    uint32
	hasLastSeq bool
	dataDir    string
	onUpdate   func(Snapshot) // 延迟兜底抓到内容时通知前端
	stop       chan struct{}
	stopOnce   sync.Once
}

func New(dataDir string, clip clipboard.Clipboard, onUpdate func(Snapshot)) *Capturer {
	return &Capturer{
		clip:     clip,
		dataDir:  dataDir,
		onUpdate: onUpdate,
		stop:     make(chan struct{}),
	}
}

// Start 后台监听剪贴板序列号，变化时抓取内容入历史。
// Sequence() 不打开剪贴板，轮询成本极低；内容读取只在变化后发生。
func (c *Capturer) Start() {
	go func() {
		ticker := time.NewTicker(pollEvery)
		defer ticker.Stop()
		for {
			select {
			case <-c.stop:
				return
			case <-ticker.C:
				c.grab()
			}
		}
	}()
}

func (c *Capturer) Stop() {
	c.stopOnce.Do(func() { close(c.stop) })
}

// grab 读取当前剪贴板并入历史。序列号未变化时不重复读取/落盘。
func (c *Capturer) grab() *Item {
	c.grabMu.Lock()
	defer c.grabMu.Unlock()

	seq := c.clip.Sequence()
	c.mu.Lock()
	if c.hasLastSeq && c.lastSeq == seq {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	var item *Item
	if pngData, ok, err := c.clip.ImagePNG(); err != nil {
		// 读取失败时不推进 lastSeq，下一轮轮询会重试（剪贴板可能正被截图工具占用）。
		slog.Warn("读取剪贴板图片失败", "err", err)
		return nil
	} else if ok {
		if rel, err := fsx.SaveScreenshot(c.dataDir, pngData, ".png"); err == nil {
			item = &Item{
				ID:        uuid.NewString(),
				Type:      "image",
				Path:      rel,
				Seq:       seq,
				At:        time.Now().Unix(),
				SourceApp: c.clip.ForegroundWindowTitle(),
			}
		} else {
			slog.Warn("保存截图失败", "err", err)
		}
	}
	if item == nil {
		if text, ok := c.clip.Text(); ok && text != "" {
			item = &Item{
				ID:        uuid.NewString(),
				Type:      "text",
				Text:      truncateRunes(text, textLimit),
				Seq:       seq,
				At:        time.Now().Unix(),
				SourceApp: c.clip.ForegroundWindowTitle(),
			}
		}
	}

	c.mu.Lock()
	c.lastSeq = seq
	c.hasLastSeq = true
	if item != nil {
		c.pushLocked(*item)
	}
	c.mu.Unlock()
	return item
}

// pushLocked 入历史（调用方需持有 c.mu）。
func (c *Capturer) pushLocked(item Item) {
	if len(c.history) > 0 && c.history[0].Seq == item.Seq {
		return
	}
	c.history = append([]Item{item}, c.history...)
	if len(c.history) > historyCap {
		c.history = c.history[:historyCap]
	}
}

// SnapshotNow 触发捕获时调用：若剪贴板已变化则抓一次，返回完整快照。
func (c *Capturer) SnapshotNow() Snapshot {
	c.grab()
	return c.Snapshot()
}

// Snapshot 返回当前历史快照（不主动读取剪贴板）。
func (c *Capturer) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	items := make([]Item, len(c.history))
	copy(items, c.history)
	snap := Snapshot{Items: items}
	if len(items) > 0 {
		snap.SelectedID = items[0].ID
	}
	return snap
}

// LateCheck 应对“截图工具还没写完剪贴板用户就点了助手”：
// 延迟后再看序列号，若变化则抓取并通知前端刷新预览。
func (c *Capturer) LateCheck() {
	go func() {
		time.Sleep(lateCheckIn)
		c.mu.Lock()
		last := c.lastSeq
		hasLast := c.hasLastSeq
		c.mu.Unlock()
		if !hasLast || c.clip.Sequence() != last {
			if item := c.grab(); item != nil && c.onUpdate != nil {
				c.onUpdate(c.Snapshot())
			}
		}
	}()
}

// Consume 从历史中移除已保存的条目，避免下一次触发重复记录同一截图。
func (c *Capturer) Consume(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.history {
		if c.history[i].ID == id {
			c.history = append(c.history[:i], c.history[i+1:]...)
			return
		}
	}
}

// AbsPath 图片条目的绝对路径（给 AI 分析读文件用）。
func (c *Capturer) AbsPath(it Item) string {
	if it.Type != "image" {
		return ""
	}
	return fsx.AbsPath(c.dataDir, filepath.FromSlash(it.Path))
}

func truncateRunes(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}
