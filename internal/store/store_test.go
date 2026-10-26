package store

import (
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
