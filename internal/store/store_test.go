package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/lybxkl/remember-fix/internal/models"
)

func TestSaveBugSetsResolvedAtForTerminalStatus(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	bug := &models.Bug{Title: "直接标记已修复", Status: models.StatusFixed}
	if err := st.SaveBug(bug); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetBug(bug.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResolvedAt == nil || *got.ResolvedAt == 0 {
		t.Fatal("saving a fixed bug should populate resolved_at")
	}
	first := *got.ResolvedAt

	got.Status = models.StatusVerified
	if err := st.SaveBug(got); err != nil {
		t.Fatal(err)
	}
	again, _ := st.GetBug(bug.ID)
	if again.ResolvedAt == nil || *again.ResolvedAt != first {
		t.Fatalf("terminal status should keep resolved_at: %+v", again.ResolvedAt)
	}

	again.Status = models.StatusDoing
	again.ResolvedAt = &first
	if err := st.SaveBug(again); err != nil {
		t.Fatal(err)
	}
	reopened, _ := st.GetBug(bug.ID)
	if reopened.ResolvedAt != nil {
		t.Fatalf("reopened bug should clear resolved_at: %+v", reopened.ResolvedAt)
	}
}

func TestBugLifecycle(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if st.GetSetting("ai.model") == "" || st.GetSetting("ai.baseURL") == "" {
		t.Fatal("AI defaults were not seeded")
	}

	bug := &models.Bug{Title: "测试 bug", DescriptionText: "描述", Attachments: []models.Attachment{{Type: "text", Text: "log"}}}
	if err := st.SaveBug(bug); err != nil {
		t.Fatal(err)
	}
	if bug.ID == "" || bug.CreatedAt == 0 {
		t.Fatalf("id/timestamp not populated: %+v", bug)
	}
	if bug.Status != models.StatusTodo || bug.Category != models.CategoryUncategorized || bug.Severity != "P2" {
		t.Fatalf("defaults not applied: %+v", bug)
	}

	got, err := st.GetBug(bug.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "测试 bug" || len(got.Attachments) != 1 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}

	if err := st.UpdateStatus(bug.ID, models.StatusFixed); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetBug(bug.ID)
	if got.ResolvedAt == nil {
		t.Fatal("resolved_at should be set for fixed")
	}

	sugg := &models.AISuggestion{Title: "AI 标题", Category: models.CategoryLogic, Severity: "P1", Tags: []string{"t"}, Summary: "s"}
	if err := st.UpdateAI(bug.ID, sugg, models.AIDone); err != nil {
		t.Fatal(err)
	}
	if err := st.SetAIStatus(bug.ID, models.AIFailed); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetBug(bug.ID)
	if got.AIStatus != models.AIFailed || got.AI == nil || got.AI.Title != "AI 标题" {
		t.Fatalf("AI state not preserved: %+v", got.AI)
	}

	list, err := st.ListBugs()
	if err != nil || len(list) != 1 {
		t.Fatalf("ListBugs = %d, %v", len(list), err)
	}
	if err := st.DeleteBug(bug.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetBug(bug.ID); err == nil {
		t.Fatal("bug should be deleted")
	}
}

func TestSaveBugRoundTripsKindAndDue(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	due := int64(1793037600)
	item := &models.Bug{
		Kind:       models.KindTodo,
		KindSource: models.KindSourceUser,
		Title:      "交房租",
		Status:     models.StatusTodo,
		DueAt:      due,
		DueAllDay:  true,
	}
	if err := st.SaveBug(item); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetBug(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != models.KindTodo || got.KindSource != models.KindSourceUser || got.DueAt != due || !got.DueAllDay {
		t.Fatalf("kind/due roundtrip mismatch: %+v", got)
	}

	if err := st.UpdateKind(item.ID, models.KindBug, models.KindSourceAI); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetBug(item.ID)
	if got.Kind != models.KindBug || got.KindSource != models.KindSourceAI {
		t.Fatalf("UpdateKind mismatch: %+v", got)
	}
	// 类型切换保留到期时间，便于 AI 自动切换后撤销。
	if got.DueAt != due {
		t.Fatalf("due should survive kind switch: %+v", got)
	}
}

func TestMigrateLegacyDatabase(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "remember-fix.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE bugs (
		id TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '',
		description_text TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'todo',
		category TEXT NOT NULL DEFAULT 'uncategorized', severity TEXT NOT NULL DEFAULT 'P2',
		tags TEXT NOT NULL DEFAULT '[]', attachments TEXT NOT NULL DEFAULT '[]',
		source_app TEXT NOT NULL DEFAULT '', ai TEXT NOT NULL DEFAULT '', ai_status TEXT NOT NULL DEFAULT 'none',
		created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, resolved_at INTEGER NOT NULL DEFAULT 0
	);
	INSERT INTO bugs(id, title, created_at, updated_at) VALUES('legacy', '旧 Bug', 100, 100);`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.GetBug("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != models.KindBug || got.KindSource != models.KindSourceUser || got.DueAt != 0 || got.DueAllDay {
		t.Fatalf("legacy row migration mismatch: %+v", got)
	}
}

func TestResetInFlightAI(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	item := &models.Bug{Title: "分析中"}
	if err := st.SaveBug(item); err != nil {
		t.Fatal(err)
	}
	if err := st.SetAIStatus(item.ID, models.AIPending); err != nil {
		t.Fatal(err)
	}
	n, err := st.ResetInFlightAI()
	if err != nil || n != 1 {
		t.Fatalf("ResetInFlightAI = (%d,%v), want (1,nil)", n, err)
	}
	got, _ := st.GetBug(item.ID)
	if got.AIStatus != models.AIFailed {
		t.Fatalf("stale pending should become failed: %+v", got)
	}
	n, err = st.ResetInFlightAI()
	if err != nil || n != 0 {
		t.Fatalf("second ResetInFlightAI = (%d,%v), want (0,nil)", n, err)
	}
}

func TestUpdateBugDoesNotResurrectDeletedRecord(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	item := &models.Bug{Title: "会被删除"}
	if err := st.SaveBug(item); err != nil {
		t.Fatal(err)
	}
	created := item.CreatedAt
	item.Title = "更新成功"
	if ok, err := st.UpdateBug(item); err != nil || !ok {
		t.Fatalf("UpdateBug existing = (%v,%v), want (true,nil)", ok, err)
	}
	got, err := st.GetBug(item.ID)
	if err != nil || got.Title != "更新成功" || got.CreatedAt != created {
		t.Fatalf("UpdateBug should update fields and keep created_at: %+v, %v", got, err)
	}
	// 抽屉“保存”可能没有改动任何字段；这仍应算成功，而不是“记录不存在”。
	if ok, err := st.UpdateBug(got); err != nil || !ok {
		t.Fatalf("no-op UpdateBug = (%v,%v), want (true,nil)", ok, err)
	}
	if err := st.DeleteBug(item.ID); err != nil {
		t.Fatal(err)
	}
	item.Title = "AI 回写"
	ok, err := st.UpdateBug(item)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("UpdateBug should report that no row was updated")
	}
	if _, err := st.GetBug(item.ID); err == nil {
		t.Fatal("UpdateBug must not recreate a deleted record")
	}
}
