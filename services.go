package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"runtime"
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
	"github.com/lybxkl/remember-fix/internal/intent"
	"github.com/lybxkl/remember-fix/internal/models"
	"github.com/lybxkl/remember-fix/internal/store"
)

// ─── 应用外壳：窗口、热键与退出状态 ───

type AppShell struct {
	app        *application.App
	dataDir    string
	mainWin    *application.WebviewWindow
	captureWin *application.WebviewWindow
	edgeWin    *application.WebviewWindow
	trigger    func()

	edgeDock      string
	edgeScreenID  string
	edgeOffset    int
	edgeOffsetSet bool
	edgeStore     *store.Store
	edgeLayoutMu  sync.Mutex

	edgeDragMu       sync.Mutex
	edgeDragging     bool
	edgeDragEndTimer *time.Timer

	hotkeyMu sync.Mutex
	hotkey   string

	quitting atomic.Bool
}

const (
	edgeCollapsedWidth  = 24
	edgeCollapsedHeight = 64
	edgeDockBarWidth    = 64
	edgeDockBarHeight   = 24
	edgeExpandedWidth   = 184
	edgeExpandedHeight  = 232
)

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// parseEdgeOffset 把保存的沿边中心坐标转成有效偏移。
// -1 是旧版本“未设置”的哨兵值；已经保存屏幕 ID 后，负数就是副屏上的真实坐标。
func parseEdgeOffset(raw, screenID string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	if raw == "-1" && screenID == "" {
		return 0, false
	}
	return v, true
}

func (s *AppShell) initEdgePosition(st *store.Store) {
	switch st.GetSetting("edge.dock") {
	case "left", "top", "bottom":
		s.edgeDock = st.GetSetting("edge.dock")
	default:
		s.edgeDock = "right"
	}
	s.edgeScreenID = st.GetSetting("edge.screenID")
	if v, ok := parseEdgeOffset(st.GetSetting("edge.offset"), s.edgeScreenID); ok {
		s.edgeOffset = v
		s.edgeOffsetSet = true
	}
}

func edgeWindowSize(dock string, expanded bool) (int, int) {
	if expanded {
		return edgeExpandedWidth, edgeExpandedHeight
	}
	if dock == "top" || dock == "bottom" {
		return edgeDockBarWidth, edgeDockBarHeight
	}
	return edgeCollapsedWidth, edgeCollapsedHeight
}

func (s *AppShell) layoutEdge(expanded bool) {
	s.edgeLayoutMu.Lock()
	defer s.edgeLayoutMu.Unlock()
	s.layoutEdgeLocked(expanded)
}

// edgeScreen 优先恢复到上次拖到的屏幕；屏幕被拔掉时退回窗口当前屏幕/主屏。
// 必须在 edgeLayoutMu 内或应用启动完成前调用。
func (s *AppShell) edgeScreen() *application.Screen {
	if s.app == nil {
		return nil
	}
	if s.edgeScreenID != "" {
		if screen := s.app.Screen.GetByID(s.edgeScreenID); screen != nil {
			return screen
		}
	}
	if s.edgeWin != nil {
		if screen, err := s.edgeWin.GetScreen(); err == nil && screen != nil {
			return screen
		}
	}
	return s.app.Screen.GetPrimary()
}

func (s *AppShell) layoutEdgeLocked(expanded bool) {
	if s.edgeWin == nil || s.app == nil {
		return
	}
	screen := s.edgeScreen()
	if screen == nil || screen.WorkArea.Width <= 0 || screen.WorkArea.Height <= 0 {
		return
	}
	dock := s.edgeDock
	switch dock {
	case "left", "top", "bottom":
	default:
		dock = "right"
	}
	width, height := edgeWindowSize(dock, expanded)
	s.edgeWin.SetSize(width, height)
	// 部分 Windows/WebView2 组合会给极窄窗口一个最小尺寸，按实际尺寸重新贴边。
	if actualWidth, actualHeight := s.edgeWin.Size(); actualWidth > 0 && actualHeight > 0 {
		width, height = actualWidth, actualHeight
	}
	wa := screen.WorkArea
	vertical := dock == "left" || dock == "right"
	var center int
	if vertical {
		if s.edgeOffsetSet {
			center = s.edgeOffset
		} else {
			center = wa.Y + wa.Height/2
		}
		center = clampInt(center, wa.Y+height/2, wa.Y+wa.Height-height/2)
	} else {
		if s.edgeOffsetSet {
			center = s.edgeOffset
		} else {
			center = wa.X + wa.Width/2
		}
		center = clampInt(center, wa.X+width/2, wa.X+wa.Width-width/2)
	}
	var x, y int
	switch dock {
	case "left":
		x, y = wa.X, center-height/2
	case "top":
		x, y = center-width/2, wa.Y
	case "bottom":
		x, y = center-width/2, wa.Y+wa.Height-height
	default:
		x, y = wa.X+wa.Width-width, center-height/2
	}
	s.edgeWin.SetPosition(x, y)
}

func (s *AppShell) resetEdgeDrag() {
	s.edgeDragMu.Lock()
	s.edgeDragging = false
	if s.edgeDragEndTimer != nil {
		s.edgeDragEndTimer.Stop()
		s.edgeDragEndTimer = nil
	}
	s.edgeDragMu.Unlock()
}

func (s *AppShell) beginEdgeDrag() {
	s.edgeDragMu.Lock()
	s.edgeDragging = true
	if s.edgeDragEndTimer != nil {
		s.edgeDragEndTimer.Stop()
		s.edgeDragEndTimer = nil
	}
	s.edgeDragMu.Unlock()
}

// macOS 没有单独的窗口拖动结束事件，用最后一次移动后的静默期兜底；
// 正常情况下前端会在 mouseup 时直接调用 EndDrag。
func (s *AppShell) noteEdgeMoved() {
	if runtime.GOOS == "windows" {
		return
	}
	s.edgeDragMu.Lock()
	if !s.edgeDragging {
		s.edgeDragMu.Unlock()
		return
	}
	if s.edgeDragEndTimer != nil {
		s.edgeDragEndTimer.Stop()
	}
	s.edgeDragEndTimer = time.AfterFunc(900*time.Millisecond, s.finishEdgeDrag)
	s.edgeDragMu.Unlock()
}

// edgeDockTarget 用窗口中心到工作区四边的距离选最近边，并给出沿边中心（收起状态）。
func edgeDockTarget(wa application.Rect, x, y, width, height int) (dock string, offset int) {
	if width <= 0 || height <= 0 {
		width, height = edgeExpandedWidth, edgeExpandedHeight
	}
	cx := x + width/2
	cy := y + height/2
	dock = "right"
	nearest := wa.X + wa.Width - cx
	if left := cx - wa.X; left < nearest {
		dock, nearest = "left", left
	}
	if top := cy - wa.Y; top < nearest {
		dock, nearest = "top", top
	}
	if bottom := wa.Y + wa.Height - cy; bottom < nearest {
		dock = "bottom"
	}
	if dock == "left" || dock == "right" {
		offset = clampInt(cy, wa.Y+edgeCollapsedHeight/2, wa.Y+wa.Height-edgeCollapsedHeight/2)
	} else {
		offset = clampInt(cx, wa.X+edgeDockBarWidth/2, wa.X+wa.Width-edgeDockBarWidth/2)
	}
	return dock, offset
}

// finishEdgeDrag 以窗口实际位置计算最近边，写入设置并吸附；可被窗口事件和前端重复调用。
func (s *AppShell) finishEdgeDrag() {
	s.edgeDragMu.Lock()
	if !s.edgeDragging {
		s.edgeDragMu.Unlock()
		return
	}
	s.edgeDragging = false
	if s.edgeDragEndTimer != nil {
		s.edgeDragEndTimer.Stop()
		s.edgeDragEndTimer = nil
	}
	s.edgeDragMu.Unlock()
	defer emit("edge:drag-end", nil)

	if s.edgeWin == nil || s.app == nil {
		return
	}
	screen := (*application.Screen)(nil)
	if current, err := s.edgeWin.GetScreen(); err == nil && current != nil {
		screen = current
	} else {
		screen = s.app.Screen.GetPrimary()
	}
	if screen == nil || screen.WorkArea.Width <= 0 || screen.WorkArea.Height <= 0 {
		return
	}
	x, y := s.edgeWin.Position()
	width, height := s.edgeWin.Size()
	if width <= 0 || height <= 0 {
		width, height = edgeExpandedWidth, edgeExpandedHeight
	}
	dock, offset := edgeDockTarget(screen.WorkArea, x, y, width, height)
	s.edgeLayoutMu.Lock()
	s.edgeDock = dock
	s.edgeScreenID = screen.ID
	s.edgeOffset = offset
	s.edgeOffsetSet = true
	s.layoutEdgeLocked(true)
	s.edgeLayoutMu.Unlock()
	if s.edgeStore != nil {
		_ = s.edgeStore.SetSetting("edge.dock", dock)
		_ = s.edgeStore.SetSetting("edge.screenID", screen.ID)
		_ = s.edgeStore.SetSetting("edge.offset", strconv.Itoa(offset))
	}
	emit("edge:dock", dock)
}

func (s *AppShell) SetEdgeEnabled(enabled bool) {
	if s.edgeWin == nil {
		return
	}
	s.resetEdgeDrag()
	if !enabled {
		emit("edge:collapse", nil)
		s.edgeWin.Hide()
		return
	}
	s.layoutEdge(false)
	s.edgeWin.Show()
	emit("edge:collapse", nil)
}

func (s *AppShell) ShowMain() {
	if s.mainWin == nil {
		return
	}
	s.mainWin.UnMinimise()
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
	s.shell.resetEdgeDrag()
	s.shell.layoutEdge(false)
	emit("edge:collapse", nil)
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

// ─── 贴边宠物服务 ───

type EdgeService struct {
	shell *AppShell
	store *store.Store
}

func (s *EdgeService) ready() error {
	if s.shell == nil || s.shell.edgeWin == nil {
		return errors.New("贴边宠物尚未就绪")
	}
	return nil
}

func (s *EdgeService) SetExpanded(expanded bool) error {
	if err := s.ready(); err != nil {
		return err
	}
	s.shell.layoutEdge(expanded)
	return nil
}

// BeginDrag 标记原生拖窗开始；实际移动由系统接管，这里不再逐帧改窗口位置。
func (s *EdgeService) BeginDrag() error {
	if err := s.ready(); err != nil {
		return err
	}
	s.shell.beginEdgeDrag()
	return nil
}

// EndDrag 在松手（macOS 的 pointerup 或兜底路径）后吸附到最近屏幕边缘。
func (s *EdgeService) EndDrag() error {
	if err := s.ready(); err != nil {
		return err
	}
	s.shell.finishEdgeDrag()
	return nil
}

// Trigger 先收起宠物，再打开捕获窗。
func (s *EdgeService) Trigger() error {
	if s.shell == nil || s.shell.trigger == nil {
		return errors.New("捕获服务尚未就绪")
	}
	s.shell.resetEdgeDrag()
	s.shell.layoutEdge(false)
	go s.shell.trigger()
	return nil
}

// ReminderState 返回贴边窗挂载时所需的待处理摘要。
func (s *EdgeService) ReminderState() (ReminderState, error) {
	if s.store == nil {
		return ReminderState{}, errors.New("数据尚未就绪")
	}
	return pendingSummary(s.store)
}

// OpenBoard 收起宠物并打开主窗；有逾期/今日到期时优先过滤到期事项，否则只看待处理。
func (s *EdgeService) OpenBoard() error {
	if s.shell == nil {
		return errors.New("应用尚未就绪")
	}
	s.shell.resetEdgeDrag()
	s.shell.layoutEdge(false)
	emit("edge:collapse", nil)
	s.shell.ShowMain()
	due := ""
	if st, err := pendingSummary(s.store); err == nil {
		if st.Overdue > 0 {
			due = "overdue"
		} else if st.DueToday > 0 {
			due = "today"
		}
	}
	emit("ui:open-pending", map[string]string{"due": due})
	return nil
}

// ─── 事项服务 ───

type DetectRequest struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	SourceApp string `json:"sourceApp"`
	HasImage  bool   `json:"hasImage"`
}

type kindChangedEvent struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	From       string  `json:"from"`
	To         string  `json:"to"`
	FromStatus string  `json:"fromStatus"`
	FromDueAt  int64   `json:"fromDueAt"`
	ToDueAt    int64   `json:"toDueAt"`
	Confidence float64 `json:"confidence"`
}

// KindChangeUndo 是看板撤销 AI 改型时回传的副作用快照。
type KindChangeUndo struct {
	ID         string `json:"id"`
	From       string `json:"from"`
	To         string `json:"to"`
	FromStatus string `json:"fromStatus"`
	FromDueAt  int64  `json:"fromDueAt"`
	ToDueAt    int64  `json:"toDueAt"`
}

// kindUndoneEvent 让打开中的详情抽屉同步撤销后的类型/状态/到期。
type kindUndoneEvent struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	DueAt     int64  `json:"dueAt"`
	DueAllDay bool   `json:"dueAllDay"`
}

type BugService struct {
	store     *store.Store
	dataDir   string
	reminders *ReminderScheduler
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

// DetectIntent 供捕获窗在输入时实时显示类型；模糊时沿用上次保存的类型。
func (s *BugService) DetectIntent(req DetectRequest) intent.Result {
	return intent.Detect(intent.Input{
		Title:     req.Title,
		Body:      req.Body,
		SourceApp: req.SourceApp,
		HasImage:  req.HasImage,
		LastKind:  s.store.GetSetting("capture.lastKind"),
	})
}

// Save 保存（新建 id 为空则生成）。仅新建记录触发自动 AI 分析，编辑不重复分析。
func (s *BugService) Save(b *models.Bug) (*models.Bug, error) {
	if b == nil {
		return nil, errors.New("记录不能为空")
	}
	isNew := strings.TrimSpace(b.ID) == ""
	if isNew && !models.ValidKind(b.Kind) {
		res := s.DetectIntent(DetectRequest{Title: b.Title, Body: b.DescriptionText, SourceApp: b.SourceApp, HasImage: hasImage(b)})
		b.Kind = res.Kind
	}
	if !models.ValidKind(b.Kind) {
		b.Kind = models.KindBug
	}
	if !models.ValidKindSource(b.KindSource) {
		b.KindSource = models.KindSourceHeuristic
	}
	if b.Kind == models.KindTodo && b.Status == models.StatusVerified {
		b.Status = models.StatusFixed
	}
	if isNew {
		if err := s.store.SaveBug(b); err != nil {
			return nil, err
		}
	} else {
		ok, err := s.store.UpdateBug(b)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errors.New("记录不存在或已被删除")
		}
	}
	if isNew {
		_ = s.store.SetSetting("capture.lastKind", b.Kind)
	}
	saved, err := s.store.GetBug(b.ID)
	if err != nil {
		return b, nil
	}
	emit("bug:changed", saved)
	s.refreshReminders()
	if isNew {
		go s.analyzeAsync(saved.ID)
	}
	return saved, nil
}

func hasImage(b *models.Bug) bool {
	for _, att := range b.Attachments {
		if att.Type == "image" && att.Path != "" {
			return true
		}
	}
	return false
}

func (s *BugService) UpdateStatus(id, status string) error {
	if !models.ValidStatus(status) {
		return errors.New("无效的状态")
	}
	b, err := s.store.GetBug(id)
	if err != nil {
		return errors.New("记录不存在")
	}
	// 待办没有「已验证」，合并到「已完成」，避免看板出现空列。
	if b.Kind == models.KindTodo && status == models.StatusVerified {
		status = models.StatusFixed
	}
	if err := s.store.UpdateStatus(id, status); err != nil {
		return err
	}
	if b, err := s.store.GetBug(id); err == nil {
		emit("bug:changed", b)
	}
	s.refreshReminders()
	return nil
}

// SetKind 切换类型；lock=true 表示用户手动决定（含撤销），之后 AI 不再自动改。
func (s *BugService) SetKind(id, kind string, lock bool) error {
	if !models.ValidKind(kind) {
		return errors.New("无效的类型")
	}
	b, err := s.store.GetBug(id)
	if err != nil {
		return errors.New("记录不存在")
	}
	source := models.KindSourceAI
	if lock {
		source = models.KindSourceUser
	}
	if b.Kind == kind && (!lock || b.KindSource == models.KindSourceUser) {
		return nil
	}
	if err := s.store.UpdateKind(id, kind, source); err != nil {
		return err
	}
	if fresh, err := s.store.GetBug(id); err == nil {
		emit("bug:changed", fresh)
	}
	s.refreshReminders()
	return nil
}

// UndoKindChange 撤销一次 AI 自动改型。除了类型，还会还原 AI 造成的
// 已验证→已完成映射和 AI 自动补的到期时间；用户之后改过的字段保持不动。
func (s *BugService) UndoKindChange(req KindChangeUndo) error {
	if !models.ValidKind(req.From) || !models.ValidKind(req.To) || req.From == req.To {
		return errors.New("无效的撤销请求")
	}
	b, err := s.store.GetBug(req.ID)
	if err != nil {
		return errors.New("记录不存在")
	}
	if b.KindSource != models.KindSourceAI || b.Kind != req.To {
		return errors.New("记录已发生变化，无法撤销")
	}

	status := b.Status
	if models.ValidStatus(req.FromStatus) && b.Status != req.FromStatus &&
		b.Status == kindStatusAfterSwitch(req.FromStatus, req.To) {
		status = req.FromStatus
	}
	dueAt, dueAllDay := b.DueAt, b.DueAllDay
	if req.To == models.KindTodo && req.FromDueAt == 0 && req.ToDueAt > 0 && b.DueAt == req.ToDueAt {
		dueAt, dueAllDay = 0, false
	}

	ok, err := s.store.UndoKindSwitch(req.ID, req.From, status, dueAt, dueAllDay)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("记录已发生变化，无法撤销")
	}
	if fresh, err := s.store.GetBug(req.ID); err == nil {
		emit("bug:changed", fresh)
		emit("bug:kind-undone", kindUndoneEvent{
			ID: fresh.ID, Kind: fresh.Kind, Status: fresh.Status,
			DueAt: fresh.DueAt, DueAllDay: fresh.DueAllDay,
		})
	}
	s.refreshReminders()
	return nil
}

// kindStatusAfterSwitch 是 AI 自动改型时对状态的唯一映射：待办没有「已验证」。
func kindStatusAfterSwitch(status, toKind string) string {
	if toKind == models.KindTodo && status == models.StatusVerified {
		return models.StatusFixed
	}
	return status
}

func (s *BugService) Delete(id string) error {
	if err := s.store.DeleteBug(id); err != nil {
		return err
	}
	emit("bug:deleted", map[string]string{"id": id})
	s.refreshReminders()
	return nil
}

func (s *BugService) refreshReminders() {
	if s.reminders != nil {
		s.reminders.Refresh()
	}
}

// Reanalyze 手动重新触发 AI 分析。
func (s *BugService) Reanalyze(id string) error {
	_, key, model, enabled := s.aiConfig()
	if !enabled {
		return errors.New("AI 分析已在设置中关闭，请先启用")
	}
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
	_, key, model, enabled := s.aiConfig()
	if !enabled {
		return 0, errors.New("AI 分析已在设置中关闭，请先启用")
	}
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
// AI 结果只填充空白字段；未手动锁定类型时，高置信类型判断会自动切换并支持撤销。
func (s *BugService) analyzeAsync(id string) {
	if _, loaded := s.analyzing.LoadOrStore(id, struct{}{}); loaded {
		return
	}
	defer s.analyzing.Delete(id)

	baseURL, key, model, enabled := s.aiConfig()
	if !enabled || key == "" || model == "" {
		slog.Info("跳过 AI 分析：未启用或未配置", "item", id)
		return
	}
	item, err := s.store.GetBug(id)
	if err != nil {
		return
	}
	wasAutoTitle := strings.TrimSpace(item.Title) == ""
	_ = s.store.SetAIStatus(id, models.AIPending)
	if pending, err := s.store.GetBug(id); err == nil {
		emit("bug:changed", pending)
	}

	var paths []string
	for _, att := range item.Attachments {
		if att.Type == "image" && att.Path != "" {
			if p := fsx.AbsPath(s.dataDir, att.Path); p != "" {
				paths = append(paths, p)
			}
		}
	}
	client := ai.NewClient(baseURL, key, model)
	sugg, err := ai.AnalyzeBug(context.Background(), client, item, paths)
	if err != nil {
		slog.Warn("AI 分析失败", "item", id, "model", model, "err", err)
		_ = s.store.SetAIStatus(id, models.AIFailed)
		if b2, err2 := s.store.GetBug(id); err2 == nil {
			emit("bug:changed", b2)
		}
		return
	}

	fresh, err := s.store.GetBug(id)
	if err != nil {
		return
	}
	fresh.AI = sugg
	fresh.AIStatus = models.AIDone
	if strings.TrimSpace(fresh.Title) == "" && strings.TrimSpace(sugg.Title) != "" {
		fresh.Title = strings.TrimSpace(sugg.Title)
	}
	if fresh.Title == "" {
		fresh.Title = fallbackTitle(fresh)
	}
	fromKind, fromStatus := "", ""
	fromDueAt := int64(0)
	if fresh.KindSource != models.KindSourceUser && models.ValidKind(sugg.Kind) &&
		sugg.Kind != fresh.Kind && sugg.KindConfidence >= 0.8 {
		fromKind = fresh.Kind
		fromStatus = fresh.Status
		fromDueAt = fresh.DueAt
		fresh.Kind = sugg.Kind
		fresh.KindSource = models.KindSourceAI
		if fresh.Kind == models.KindTodo && fresh.Status == models.StatusVerified {
			fresh.Status = models.StatusFixed
		}
	}
	if fresh.Kind == models.KindBug && (fresh.Category == "" || fresh.Category == models.CategoryUncategorized) {
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
	dueFilled := false
	if fresh.Kind == models.KindTodo && fresh.DueAt == 0 && sugg.DueAt > 0 {
		fresh.DueAt = sugg.DueAt
		fresh.DueAllDay = sugg.DueAllDay
		dueFilled = true
	}
	toDueAt := int64(0)
	if dueFilled {
		toDueAt = fresh.DueAt
	}
	ok, err := s.store.UpdateBug(fresh)
	if err != nil {
		slog.Warn("保存 AI 分析结果失败", "item", id, "err", err)
		_ = s.store.SetAIStatus(id, models.AIFailed)
		if b, err := s.store.GetBug(id); err == nil {
			emit("bug:changed", b)
		}
		return
	}
	if !ok {
		slog.Info("跳过 AI 结果回写：记录已删除", "item", id)
		return
	}
	final, err := s.store.GetBug(id)
	if err != nil {
		final = fresh
	}
	emit("bug:changed", final)
	if dueFilled || fromKind != "" {
		s.refreshReminders()
	}
	if fromKind != "" {
		slog.Info("AI 自动切换事项类型", "item", id, "from", fromKind, "to", final.Kind, "confidence", sugg.KindConfidence)
		emit("bug:kind-changed", kindChangedEvent{
			ID: id, Title: final.Title, From: fromKind, To: final.Kind,
			FromStatus: fromStatus, FromDueAt: fromDueAt, ToDueAt: toDueAt,
			Confidence: sugg.KindConfidence,
		})
	}
}

// GenerateFixPrompt 基于 bug 信息生成可直接投喂 AI 编程工具的修复指令（markdown）。
func (s *BugService) GenerateFixPrompt(id string) (string, error) {
	b, err := s.store.GetBug(id)
	if err != nil {
		return "", err
	}
	if b.Kind == models.KindTodo {
		return "", errors.New("待办事项没有修复指令")
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
	return "未命名事项"
}

// ─── 配置服务 ───

type ConfigService struct {
	store     *store.Store
	shell     *AppShell
	reminders *ReminderScheduler
}

func (s *ConfigService) GetAll() map[string]string {
	cfg := s.store.GetAllSettings()
	cfg["autostart"] = strconv.FormatBool(autostart.Enabled())
	if _, ok := cfg["edge.enabled"]; !ok {
		cfg["edge.enabled"] = "true"
	}
	if _, ok := cfg["edge.dock"]; !ok {
		dock := s.shell.edgeDock
		if dock == "" {
			dock = "right"
		}
		cfg["edge.dock"] = dock
	}
	if _, ok := cfg[reminderEnabledKey]; !ok {
		cfg[reminderEnabledKey] = "true"
	}
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

// SetEdgeEnabled 持久化贴边宠物开关并立即生效。
func (s *ConfigService) SetEdgeEnabled(enabled bool) error {
	if err := s.store.SetSetting("edge.enabled", strconv.FormatBool(enabled)); err != nil {
		return err
	}
	s.shell.SetEdgeEnabled(enabled)
	return nil
}

// SetEdgeReminderEnabled 关闭后停止定时脉冲，徽标仍随待处理数量更新。
func (s *ConfigService) SetEdgeReminderEnabled(enabled bool) error {
	if err := s.store.SetSetting(reminderEnabledKey, strconv.FormatBool(enabled)); err != nil {
		return err
	}
	if s.reminders != nil {
		s.reminders.SetEnabled(enabled)
	}
	return nil
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

// OpenURL 只允许 http/https，并用系统默认浏览器打开，避免 AI 输出里的链接
// 直接把看板 WebView 导航走。
func (s *ConfigService) OpenURL(raw string) bool {
	if !validExternalURL(raw) || s.shell == nil || s.shell.app == nil {
		return false
	}
	return s.shell.app.Browser.OpenURL(strings.TrimSpace(raw)) == nil
}

func validExternalURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == "http" || scheme == "https"
}
