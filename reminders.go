package main

import (
	"log/slog"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	"github.com/lybxkl/remember-fix/internal/models"
	"github.com/lybxkl/remember-fix/internal/store"
)

const (
	reminderMinInterval   = 45 * time.Minute
	reminderMaxInterval   = 90 * time.Minute
	reminderMaxPerDay     = 3
	reminderQuietStart    = 22
	reminderQuietEnd      = 9
	reminderRetryDelay    = 7 * time.Minute
	reminderMorningJitter = 9 * time.Minute
	reminderEnabledKey    = "edge.reminder.enabled"
	reminderDayKey        = "reminder.day"
	reminderCountKey      = "reminder.count"
)

// ReminderState 是贴边窗徽标与悬停文案共用的唯一数据源。
type ReminderState struct {
	Pending    int   `json:"pending"`
	OldestDays int   `json:"oldestDays"`
	DueToday   int   `json:"dueToday"`
	Overdue    int   `json:"overdue"`
	NextDueAt  int64 `json:"nextDueAt"`
}

type reminderEvent struct {
	ReminderState
	Pulse bool `json:"pulse"`
}

// ReminderScheduler 只负责按规则推送“状态已更新”事件，不展开窗口、不弹系统通知。
type ReminderScheduler struct {
	store *store.Store
	shell *AppShell

	mu      sync.Mutex
	timer   *time.Timer
	nextAt  time.Time
	running bool
	last    ReminderState
	hasLast bool
	wg      sync.WaitGroup

	now    func() time.Time
	jitter func(min, max time.Duration) time.Duration
}

func newReminderScheduler(st *store.Store, shell *AppShell) *ReminderScheduler {
	return &ReminderScheduler{
		store: st,
		shell: shell,
		now:   time.Now,
		jitter: func(min, max time.Duration) time.Duration {
			if max <= min {
				return min
			}
			return min + time.Duration(rand.Int64N(int64(max-min)+1))
		},
	}
}

func reminderQuiet(t time.Time) bool {
	h := t.Hour()
	return h >= reminderQuietStart || h < reminderQuietEnd
}

// nextReminderFire 计算下一次可提醒的时间：45–90 分钟抖动、日上限 3 次、22:00–09:00 静默。
func nextReminderFire(now time.Time, sentToday int, jitter func(min, max time.Duration) time.Duration) time.Time {
	morning := time.Date(now.Year(), now.Month(), now.Day(), reminderQuietEnd, 0, 0, 0, now.Location())
	nextMorning := func() time.Time {
		if morning.After(now) {
			return morning
		}
		return morning.AddDate(0, 0, 1)
	}
	if sentToday >= reminderMaxPerDay || reminderQuiet(now) {
		return nextMorning().Add(jitter(0, reminderMorningJitter))
	}
	candidate := now.Add(jitter(reminderMinInterval, reminderMaxInterval))
	if reminderQuiet(candidate) {
		target := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), reminderQuietEnd, 0, 0, 0, candidate.Location())
		if !target.After(candidate) {
			target = target.AddDate(0, 0, 1)
		}
		return target.Add(jitter(0, reminderMorningJitter))
	}
	return candidate
}

func (s *ReminderScheduler) Start() {
	if s.store.GetSetting(reminderEnabledKey) == "false" {
		return
	}
	s.SetEnabled(true)
}

func (s *ReminderScheduler) Stop() {
	s.mu.Lock()
	s.running = false
	s.nextAt = time.Time{}
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	// 等正在执行的 tick 结束，避免退出时数据库先于调度器关闭。
	s.wg.Wait()
}

func (s *ReminderScheduler) SetEnabled(enabled bool) {
	s.mu.Lock()
	if !enabled {
		s.running = false
		s.nextAt = time.Time{}
		if s.timer != nil {
			s.timer.Stop()
			s.timer = nil
		}
		s.mu.Unlock()
		return
	}
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	now := s.now()
	next := s.nextFire(now)
	s.scheduleAtLocked(next.Sub(now))
	s.mu.Unlock()
	slog.Info("待处理提醒已启用", "next", next.Format("2006-01-02 15:04"))
	s.Refresh()
}

// Refresh 重算待处理数量并广播；如果新增/修改的到期时间早于当前定时器，
// 立即把下一次检查提前，保证“到期优先提醒”不只对启动时已有的数据生效。
func (s *ReminderScheduler) Refresh() {
	now := s.now()
	st, err := pendingSummaryAt(s.store, now)
	if err != nil {
		return
	}
	s.emit(st, false)
	s.pullDueForward(st, now)
}

func (s *ReminderScheduler) pullDueForward(st ReminderState, now time.Time) {
	due, ok := nextDueTime(st, now)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	if s.nextAt.IsZero() || due.Before(s.nextAt) {
		s.scheduleAtLocked(due.Sub(now))
	}
}

func (s *ReminderScheduler) emit(st ReminderState, pulse bool) {
	s.mu.Lock()
	if pulse && !s.running {
		s.mu.Unlock()
		return
	}
	if !pulse && s.hasLast && s.last == st {
		s.mu.Unlock()
		return
	}
	s.last = st
	s.hasLast = true
	s.mu.Unlock()
	emit("edge:reminder", reminderEvent{ReminderState: st, Pulse: pulse})
}

func (s *ReminderScheduler) scheduleAtLocked(delay time.Duration) {
	if delay < time.Minute {
		delay = time.Minute
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.nextAt = s.now().Add(delay)
	s.timer = time.AfterFunc(delay, s.tick)
}

func (s *ReminderScheduler) tick() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.wg.Add(1)
	s.timer = nil
	s.nextAt = time.Time{}
	s.mu.Unlock()
	defer s.wg.Done()

	now := s.now()
	if reminderQuiet(now) {
		// 系统时间跳变或休眠唤醒后可能落在静默时段，兜底顺延到早上。
		s.rescheduleNormal(now)
		return
	}
	if s.captureOpen() {
		// 捕获窗打开时不打断，几分钟后再检查。
		slog.Info("跳过贴边提醒：捕获窗正打开")
		s.rescheduleIn(reminderRetryDelay + s.jitter(0, 3*time.Minute))
		return
	}
	if !s.edgeVisible() {
		s.rescheduleNormal(now)
		return
	}
	st, err := pendingSummary(s.store)
	if err != nil {
		s.rescheduleNormal(now)
		return
	}
	if st.Pending == 0 {
		s.emit(st, false)
		s.rescheduleNormal(now)
		return
	}
	_, sent := s.sentToday(now)
	if sent >= reminderMaxPerDay {
		s.rescheduleNormal(now)
		return
	}
	s.setSentToday(now, sent+1)
	slog.Info("发送贴边提醒脉冲", "pending", st.Pending, "oldestDays", st.OldestDays, "sentToday", sent+1)
	s.emit(st, true)
	s.rescheduleNormal(now)
}

func (s *ReminderScheduler) rescheduleNormal(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.scheduleAtLocked(s.nextFire(now).Sub(now))
}

// nextDueTime 提取最近一次可提醒的具体到期时刻（忽略静默时段与已到期）。
// 全天到期事项交给随机脉冲与悬停文案。
func nextDueTime(st ReminderState, now time.Time) (time.Time, bool) {
	if st.NextDueAt == 0 {
		return time.Time{}, false
	}
	due := time.Unix(st.NextDueAt, 0).In(now.Location())
	if !due.After(now) || reminderQuiet(due) {
		return time.Time{}, false
	}
	return due, true
}

// nextFire 在随机脉冲之外，把最近的带具体时间的到期事项提前到候选时间。
// 所有脉冲仍共用每日上限与静默时段；全天到期事项交给随机脉冲与悬停文案。
func (s *ReminderScheduler) nextFire(now time.Time) time.Time {
	_, sent := s.sentToday(now)
	next := nextReminderFire(now, sent, s.jitter)
	st, err := pendingSummaryAt(s.store, now)
	if err != nil {
		return next
	}
	if due, ok := nextDueTime(st, now); ok && due.Before(next) {
		return due
	}
	return next
}

func (s *ReminderScheduler) rescheduleIn(delay time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.scheduleAtLocked(delay)
}

func (s *ReminderScheduler) sentToday(now time.Time) (string, int) {
	day := now.Format("2006-01-02")
	if s.store.GetSetting(reminderDayKey) != day {
		return day, 0
	}
	n, err := strconv.Atoi(s.store.GetSetting(reminderCountKey))
	if err != nil || n < 0 {
		return day, 0
	}
	return day, n
}

func (s *ReminderScheduler) setSentToday(now time.Time, n int) {
	_ = s.store.SetSetting(reminderDayKey, now.Format("2006-01-02"))
	_ = s.store.SetSetting(reminderCountKey, strconv.Itoa(n))
}

func (s *ReminderScheduler) captureOpen() bool {
	return s.shell != nil && s.shell.captureWin != nil && s.shell.captureWin.IsVisible()
}

func (s *ReminderScheduler) edgeVisible() bool {
	return s.shell != nil && s.shell.edgeWin != nil && s.shell.edgeWin.IsVisible()
}

func pendingSummary(st *store.Store) (ReminderState, error) {
	return pendingSummaryAt(st, time.Now())
}

func pendingSummaryAt(st *store.Store, now time.Time) (ReminderState, error) {
	items, err := st.ListBugs()
	if err != nil {
		return ReminderState{}, err
	}
	var state ReminderState
	var oldest int64
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for _, b := range items {
		if !models.IsOpen(b.Status) {
			continue
		}
		state.Pending++
		if oldest == 0 || b.CreatedAt < oldest {
			oldest = b.CreatedAt
		}
		if b.DueAt <= 0 || b.Kind != models.KindTodo {
			continue
		}
		due := time.Unix(b.DueAt, 0).In(now.Location())
		if b.DueAllDay {
			dueDay := time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, now.Location())
			switch {
			case dueDay.Before(today):
				state.Overdue++
			case dueDay.Equal(today):
				state.DueToday++
			}
			continue
		}
		if due.Before(now) {
			state.Overdue++
			continue
		}
		if due.Year() == now.Year() && due.YearDay() == now.YearDay() {
			state.DueToday++
		}
		if state.NextDueAt == 0 || due.Unix() < state.NextDueAt {
			state.NextDueAt = due.Unix()
		}
	}
	if oldest > 0 {
		if days := int((now.Unix() - oldest) / 86400); days > 0 {
			state.OldestDays = days
		}
	}
	return state, nil
}
