package main

import (
	"testing"
	"time"

	"github.com/lybxkl/remember-fix/internal/models"
	"github.com/lybxkl/remember-fix/internal/store"
)

func minJitter(min, _ time.Duration) time.Duration { return min }

func TestNextReminderFire(t *testing.T) {
	utc := time.UTC
	at := func(day, hour, minute int) time.Time {
		return time.Date(2026, 10, day, hour, minute, 0, 0, utc)
	}
	cases := []struct {
		name string
		now  time.Time
		sent int
		want time.Time
	}{
		{"普通时段 45 分钟后", at(26, 10, 0), 0, at(26, 10, 45)},
		{"刚好 09:00 可提醒", at(26, 9, 0), 0, at(26, 9, 45)},
		{"22:00 进入静默", at(26, 22, 0), 0, at(27, 9, 0)},
		{"深夜顺延到次日早上", at(26, 23, 0), 0, at(27, 9, 0)},
		{"清晨等到 09:00", at(26, 8, 0), 0, at(26, 9, 0)},
		{"跨入静默区间则顺延", at(26, 21, 30), 0, at(27, 9, 0)},
		{"达到每日上限", at(26, 10, 0), 3, at(27, 9, 0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nextReminderFire(tc.now, tc.sent, minJitter)
			if !got.Equal(tc.want) {
				t.Fatalf("nextReminderFire(%s, sent=%d) = %s, want %s", tc.now, tc.sent, got, tc.want)
			}
		})
	}
}

func TestNextReminderFireUsesJitter(t *testing.T) {
	utc := time.UTC
	now := time.Date(2026, 10, 26, 10, 0, 0, 0, utc)
	maxJitter := func(_, max time.Duration) time.Duration { return max }
	if got, want := nextReminderFire(now, 0, maxJitter), now.Add(90*time.Minute); !got.Equal(want) {
		t.Fatalf("普通时段应使用 45–90 分钟上限: got %s, want %s", got, want)
	}
	quietNow := time.Date(2026, 10, 26, 23, 0, 0, 0, utc)
	want := time.Date(2026, 10, 27, 9, 9, 0, 0, utc)
	if got := nextReminderFire(quietNow, 0, maxJitter); !got.Equal(want) {
		t.Fatalf("静默后应加最长晨间抖动: got %s, want %s", got, want)
	}
}

func TestReminderQuietBoundaries(t *testing.T) {
	utc := time.UTC
	cases := []struct {
		hour, minute int
		quiet        bool
	}{
		{21, 59, false},
		{22, 0, true},
		{23, 30, true},
		{0, 0, true},
		{8, 59, true},
		{9, 0, false},
		{12, 0, false},
	}
	for _, tc := range cases {
		got := reminderQuiet(time.Date(2026, 10, 26, tc.hour, tc.minute, 0, 0, utc))
		if got != tc.quiet {
			t.Fatalf("reminderQuiet(%02d:%02d) = %v, want %v", tc.hour, tc.minute, got, tc.quiet)
		}
	}
}

func TestReminderSentTodayPersists(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := newReminderScheduler(st, nil)
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	s.setSentToday(now, 2)
	if day, sent := s.sentToday(now); day != "2026-10-26" || sent != 2 {
		t.Fatalf("sentToday = (%s,%d), want (2026-10-26,2)", day, sent)
	}
	if day, sent := s.sentToday(now.Add(24 * time.Hour)); day != "2026-10-27" || sent != 0 {
		t.Fatalf("跨天应清零, got (%s,%d)", day, sent)
	}
	if err := st.SetSetting(reminderCountKey, "abc"); err != nil {
		t.Fatal(err)
	}
	if _, sent := s.sentToday(now); sent != 0 {
		t.Fatalf("损坏的计数应回退为 0, got %d", sent)
	}
}

func TestReminderStartStopRespectsSetting(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := newReminderScheduler(st, nil)
	defer s.Stop()
	if err := st.SetSetting(reminderEnabledKey, "false"); err != nil {
		t.Fatal(err)
	}
	s.Start()
	if schedulerRunning(s) {
		t.Fatal("关闭设置时不应启动调度器")
	}
	if err := st.SetSetting(reminderEnabledKey, "true"); err != nil {
		t.Fatal(err)
	}
	s.Start()
	if !schedulerRunning(s) {
		t.Fatal("开启设置后应启动调度器")
	}
	s.Stop()
	if schedulerRunning(s) {
		t.Fatal("Stop 后不应继续运行")
	}
}

func schedulerRunning(s *ReminderScheduler) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func TestPendingSummary(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	save := func(id, status string, createdAt time.Time) {
		t.Helper()
		b := &models.Bug{ID: id, Status: status, CreatedAt: createdAt.Unix()}
		if err := st.SaveBug(b); err != nil {
			t.Fatal(err)
		}
	}
	save("todo", models.StatusTodo, now.Add(-10*24*time.Hour))
	save("doing", models.StatusDoing, now.Add(-2*24*time.Hour))
	save("fixed", models.StatusFixed, now.Add(-30*24*time.Hour))
	save("verified", models.StatusVerified, now.Add(-20*24*time.Hour))
	save("wontfix", models.StatusWontfix, now.Add(-8*24*time.Hour))

	got, err := pendingSummaryAt(st, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Pending != 2 || got.OldestDays != 10 {
		t.Fatalf("pendingSummaryAt = %+v, want {Pending:2 OldestDays:10}", got)
	}

	for _, id := range []string{"todo", "doing"} {
		if err := st.UpdateStatus(id, models.StatusFixed); err != nil {
			t.Fatal(err)
		}
	}
	got, err = pendingSummaryAt(st, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Pending != 0 || got.OldestDays != 0 {
		t.Fatalf("全部修完后应为空: %+v", got)
	}
}
