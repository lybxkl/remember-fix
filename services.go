package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/lybxkl/remember-fix/internal/ai"
	"github.com/lybxkl/remember-fix/internal/autostart"
	"github.com/lybxkl/remember-fix/internal/capture"
	"github.com/lybxkl/remember-fix/internal/exporter"
	"github.com/lybxkl/remember-fix/internal/fsx"
	"github.com/lybxkl/remember-fix/internal/models"
	"github.com/lybxkl/remember-fix/internal/store"
)

// ─── 应用外壳：窗口、热键与退出状态 ───

type AppShell struct {
	app        *application.App
	dataDir    string
	mainWin    *application.WebviewWindow
	captureWin *application.WebviewWindow
	trigger    func()

	hotkeyMu sync.Mutex
	hotkey   string

	quitting atomic.Bool
}

func (s *AppShell) ShowMain() {
	if s.mainWin == nil {
		return
	}
	s.mainWin.Show()
	s.mainWin.Focus()
}

func (s *AppShell) ShowCapture() {
	if s.captureWin == nil {
		return
	}
	s.captureWin.Center()
	s.captureWin.Show()
	s.captureWin.Focus()
}

func (s *AppShell) HideCapture() {
	if s.captureWin != nil {
		s.captureWin.Hide()
	}
}

// registerHotkey 动态注册全局热键；失败时回滚到旧热键并返回可读错误。
func (s *AppShell) registerHotkey(accel string) error {
	accel = strings.TrimSpace(accel)
	if accel == "" {
		return errors.New("热键不能为空")
	}
	s.hotkeyMu.Lock()
	defer s.hotkeyMu.Unlock()
	if s.app == nil {
		return errors.New("应用尚未就绪")
	}
	if s.hotkey != "" && strings.EqualFold(s.hotkey, accel) {
		return nil
	}
	old := s.hotkey
	if old != "" {
		_ = s.app.GlobalShortcut.Unregister(old)
	}
	callback := func() {
		if s.trigger != nil {
			go s.trigger()
		}
	}
	if err := s.app.GlobalShortcut.Register(accel, callback); err != nil {
		if old != "" {
			if restoreErr := s.app.GlobalShortcut.Register(old, callback); restoreErr != nil {
				slog.Error("恢复旧热键失败", "hotkey", old, "err", restoreErr)
				s.hotkey = ""
			}
		}
		return fmt.Errorf("热键 %s 注册失败（可能已被其他程序占用）: %w", accel, err)
	}
	s.hotkey = accel
	return nil
}

func (s *AppShell) Quit() {
	s.quitting.Store(true)
	if s.app != nil {
		s.app.Quit()
	}
}

// ─── 捕获服务 ───

type CaptureService struct {
	capturer *capture.Capturer
	shell    *AppShell
}

// Trigger 抓取剪贴板快照，推送给捕获窗并显示窗口。
func (s *CaptureService) Trigger() {
	snap := s.capturer.SnapshotNow()
	emit("capture:ready", snap)
	s.shell.ShowCapture()
	s.capturer.LateCheck()
}

// Snapshot 返回当前剪贴板历史（捕获窗挂载时兜底拉取）。
func (s *CaptureService) Snapshot() capture.Snapshot {
	return s.capturer.Snapshot()
}

// Cancel 隐藏捕获窗（放弃保存）。
func (s *CaptureService) Cancel() {
	s.shell.HideCapture()
}

// Consume 把已保存的剪贴板条目移出历史，避免下次触发重复记录。
func (s *CaptureService) Consume(id string) {
	if id != "" {
		s.capturer.Consume(id)
	}
}

// ─── Bug 服务 ───

type BugService struct {
	store     *store.Store
	dataDir   string
	analyzing sync.Map // id -> struct{}，避免同一条记录并发分析
}

func (s *BugService) aiConfig() (baseURL, apiKey, model string, enabled bool) {
	cfg := s.store.GetAllSettings()
	return cfg["ai.baseURL"], cfg["ai.apiKey"], cfg["ai.model"], cfg["ai.enabled"] == "true"
}

func (s *BugService) List() ([]*models.Bug, error) {
	return s.store.ListBugs()
}

func (s *BugService) Get(id string) (*models.Bug, error) {
	return s.store.GetBug(id)
}

// Save 保存（新建 id 为空则生成）。仅新建记录触发自动 AI 分析，编辑不重复分析。
func (s *BugService) Save(b *models.Bug) (*models.Bug, error) {
	if b == nil {
		return nil, errors.New("bug 不能为空")
	}
	isNew := strings.TrimSpace(b.ID) == ""
	if err := s.store.SaveBug(b); err != nil {
		return nil, err
	}
	saved, err := s.store.GetBug(b.ID)
	if err != nil {
		return b, nil
	}
	emit("bug:changed", saved)
	if isNew {
		go s.analyzeAsync(saved.ID)
	}
	return saved, nil
}

func (s *BugService) UpdateStatus(id, status string) error {
	valid := map[string]bool{
		models.StatusTodo: true, models.StatusDoing: true, models.StatusFixed: true,
		models.StatusVerified: true, models.StatusWontfix: true,
	}
	if !valid[status] {
		return errors.New("无效的状态")
	}
	if err := s.store.UpdateStatus(id, status); err != nil {
		return err
	}
	if b, err := s.store.GetBug(id); err == nil {
		emit("bug:changed", b)
	}
	return nil
}

func (s *BugService) Delete(id string) error {
	if err := s.store.DeleteBug(id); err != nil {
		return err
	}
	emit("bug:deleted", map[string]string{"id": id})
	return nil
}

// Reanalyze 手动重新触发 AI 分析。
func (s *BugService) Reanalyze(id string) error {
	_, key, model, _ := s.aiConfig()
	if key == "" || model == "" {
		return errors.New("请先在设置里配置 AI API Key 与模型")
	}
	if _, err := s.store.GetBug(id); err != nil {
		return errors.New("记录不存在")
	}
	go s.analyzeAsync(id)
	return nil
}

// AnalyzeUnanalyzed 批量分析所有尚未成功分析过的记录，返回本次排队数量。
func (s *BugService) AnalyzeUnanalyzed() (int, error) {
	_, key, model, _ := s.aiConfig()
	if key == "" || model == "" {
		return 0, errors.New("请先在设置里配置 AI API Key 与模型")
	}
	all, err := s.store.ListBugs()
	if err != nil {
		return 0, err
	}
	var ids []string
	for _, b := range all {
		if b.AIStatus == models.AINone || b.AIStatus == models.AIFailed {
			ids = append(ids, b.ID)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	go func() {
		for _, id := range ids {
			s.analyzeAsync(id)
		}
	}()
	return len(ids), nil
}

// ExportMarkdown 导出全部记录为一个含截图副本的 Markdown 文件夹，返回首页文件路径。
func (s *BugService) ExportMarkdown() (string, error) {
	all, err := s.store.ListBugs()
	if err != nil {
		return "", err
	}
	return exporter.Markdown(s.dataDir, all)
}

// analyzeAsync 后台执行 AI 分析：pending → done/failed，并广播事件。
// AI 结果只填充空白字段，用户手动输入的内容永不被覆盖。
func (s *BugService) analyzeAsync(id string) {
	if _, loaded := s.analyzing.LoadOrStore(id, struct{}{}); loaded {
		return
	}
	defer s.analyzing.Delete(id)

	baseURL, key, model, enabled := s.aiConfig()
	if !enabled || key == "" || model == "" {
		slog.Info("跳过 AI 分析：未启用或未配置", "bug", id)
		return
	}
	bug, err := s.store.GetBug(id)
	if err != nil {
		return
	}
	wasAutoTitle := strings.TrimSpace(bug.Title) == ""
	_ = s.store.SetAIStatus(id, models.AIPending)
	if pending, err := s.store.GetBug(id); err == nil {
		emit("bug:changed", pending)
	}

	var paths []string
	for _, att := range bug.Attachments {
		if att.Type == "image" && att.Path != "" {
			if p := fsx.AbsPath(s.dataDir, att.Path); p != "" {
				paths = append(paths, p)
			}
		}
	}
	client := ai.NewClient(baseURL, key, model)
	sugg, err := ai.AnalyzeBug(context.Background(), client, bug, paths)
	if err != nil {
		slog.Warn("AI 分析失败", "bug", id, "model", model, "err", err)
		_ = s.store.SetAIStatus(id, models.AIFailed)
		if b2, err2 := s.store.GetBug(id); err2 == nil {
			emit("bug:changed", b2)
		}
		return
	}

	_ = s.store.UpdateAI(id, sugg, models.AIDone)
	fresh, err := s.store.GetBug(id)
	if err != nil {
		return
	}
	if strings.TrimSpace(fresh.Title) == "" && strings.TrimSpace(sugg.Title) != "" {
		fresh.Title = strings.TrimSpace(sugg.Title)
	}
	if fresh.Title == "" {
		fresh.Title = fallbackTitle(fresh)
	}
	if fresh.Category == "" || fresh.Category == models.CategoryUncategorized {
		if sugg.Category != "" {
			fresh.Category = sugg.Category
		}
	}
	if len(fresh.Tags) == 0 && len(sugg.Tags) > 0 {
		fresh.Tags = sugg.Tags
	}
	if fresh.Severity == "" || (wasAutoTitle && fresh.Severity == "P2" && sugg.Severity != "") {
		if sugg.Severity != "" {
			fresh.Severity = sugg.Severity
		}
	}
	_ = s.store.SaveBug(fresh)
	if final, err := s.store.GetBug(id); err == nil {
		emit("bug:changed", final)
	}
}

// GenerateFixPrompt 基于 bug 信息生成可直接投喂 AI 编程工具的修复指令（markdown）。
func (s *BugService) GenerateFixPrompt(id string) (string, error) {
	b, err := s.store.GetBug(id)
	if err != nil {
		return "", err
	}
	baseURL, key, model, _ := s.aiConfig()
	if key == "" || model == "" {
		return "", errors.New("请先在设置里配置 AI API Key 与模型")
	}
	var sb strings.Builder
	sb.WriteString("## Bug 信息\n")
	sb.WriteString("- 标题：" + b.Title + "\n")
	sb.WriteString("- 分类：" + b.Category + " / 严重度：" + b.Severity + "\n")
	if len(b.Tags) > 0 {
		sb.WriteString("- 标签：" + strings.Join(b.Tags, "、") + "\n")
	}
	if b.SourceApp != "" {
		sb.WriteString("- 来源应用：" + b.SourceApp + "\n")
	}
	sb.WriteString("- 用户描述：\n" + strings.TrimSpace(b.DescriptionText) + "\n")
	if b.AI != nil {
		if b.AI.Summary != "" {
			sb.WriteString("\n### AI 概括\n" + b.AI.Summary + "\n")
		}
		if len(b.AI.Steps) > 0 {
			sb.WriteString("\n### 疑似复现步骤\n")
			for i, step := range b.AI.Steps {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
			}
		}
	}
	for _, att := range b.Attachments {
		if att.Type == "text" && att.Text != "" {
			sb.WriteString("\n### 相关文本/日志\n```\n" + att.Text + "\n```\n")
		}
		if att.Type == "image" && att.Path != "" {
			if p := fsx.AbsPath(s.dataDir, att.Path); p != "" {
				sb.WriteString("\n### 截图文件\n" + p + "\n")
			}
		}
	}

	system := "你是资深软件工程师。基于用户提供的 bug 信息，输出一份给 AI 编程工具（如 Claude Code、Cursor）的修复指令 markdown，" +
		"包含：问题定位建议（可能在哪些模块/文件）、修复思路、验收标准（如何验证修好了）、回归风险提示。" +
		"不要寒暄，直接输出 markdown 正文。若信息不足请列出需要补充的问题清单。"
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	return ai.NewClient(baseURL, key, model).Send(ctx, system, sb.String(), 4096)
}

func fallbackTitle(b *models.Bug) string {
	candidates := []string{b.DescriptionText}
	for _, att := range b.Attachments {
		if att.Type == "text" {
			candidates = append(candidates, att.Text)
		}
	}
	for _, text := range candidates {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				r := []rune(line)
				if len(r) > 40 {
					return string(r[:40]) + "…"
				}
				return line
			}
		}
	}
	return "未命名 Bug"
}

// ─── 配置服务 ───

type ConfigService struct {
	store *store.Store
	shell *AppShell
}

func (s *ConfigService) GetAll() map[string]string {
	cfg := s.store.GetAllSettings()
	cfg["autostart"] = strconv.FormatBool(autostart.Enabled())
	return cfg
}

var aiSettingKeys = map[string]bool{
	"ai.baseURL": true, "ai.apiKey": true, "ai.model": true, "ai.enabled": true,
}

func (s *ConfigService) SetMany(kv map[string]string) error {
	for k, v := range kv {
		if !aiSettingKeys[k] {
			continue
		}
		if err := s.store.SetSetting(k, v); err != nil {
			return err
		}
	}
	return nil
}

// SetHotkey 立即注册新热键，成功后才落库。
func (s *ConfigService) SetHotkey(accel string) error {
	if err := s.shell.registerHotkey(accel); err != nil {
		return err
	}
	return s.store.SetSetting("hotkey", strings.TrimSpace(accel))
}

func (s *ConfigService) SetAutostart(enabled bool) error {
	return autostart.Set(enabled)
}

// TestAI 用当前保存的配置发一条测试消息，返回模型回复首行。
func (s *ConfigService) TestAI() (string, error) {
	cfg := s.store.GetAllSettings()
	key, model := cfg["ai.apiKey"], cfg["ai.model"]
	if key == "" {
		return "", errors.New("未配置 API Key")
	}
	if model == "" {
		return "", errors.New("未配置模型")
	}
	client := ai.NewClient(cfg["ai.baseURL"], key, model)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	reply, err := client.Send(ctx, "你是连通性测试助手，只回答用户要求的内容。", "请只回复：连接成功", 2048)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.SplitN(reply, "\n", 2)[0]), nil
}

func (s *ConfigService) OpenDataDir() error {
	if s.shell.app == nil {
		return errors.New("应用尚未就绪")
	}
	return s.shell.app.Browser.OpenFile(s.shell.dataDir)
}

// CopyText 写入系统剪贴板（避免 WebView 的剪贴板权限差异）。
func (s *ConfigService) CopyText(text string) bool {
	if s.shell.app == nil || text == "" {
		return false
	}
	return s.shell.app.Clipboard.SetText(text)
}
