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
	Pending    int `json:"pending"`
	OldestDays int `json:"oldestDays"`
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
	_, sent := s.sentToday(now)
	next := nextReminderFire(now, sent, s.jitter)
	s.scheduleAtLocked(next.Sub(now))
	s.mu.Unlock()
	slog.Info("待处理提醒已启用", "next", next.Format("2006-01-02 15:04"))
	s.Refresh()
}

// Refresh 重算待处理数量；只有数值变化时才广播，避免 AI 分析更新造成的重复事件。
func (s *ReminderScheduler) Refresh() {
	st, err := pendingSummary(s.store)
	if err != nil {
		return
	}
	s.emit(st, false)
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
	_, sent := s.sentToday(now)
	s.scheduleAtLocked(nextReminderFire(now, sent, s.jitter).Sub(now))
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
	bugs, err := st.ListBugs()
	if err != nil {
		return ReminderState{}, err
	}
	var state ReminderState
	var oldest int64
	for _, b := range bugs {
		if b.Status != models.StatusTodo && b.Status != models.StatusDoing {
			continue
		}
		state.Pending++
		if oldest == 0 || b.CreatedAt < oldest {
			oldest = b.CreatedAt
		}
	}
	if oldest > 0 {
		if days := int((now.Unix() - oldest) / 86400); days > 0 {
			state.OldestDays = days
		}
	}
	return state, nil
}
