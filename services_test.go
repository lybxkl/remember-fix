package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lybxkl/remember-fix/internal/models"
	"github.com/lybxkl/remember-fix/internal/store"
)

func TestSaveUsesIntentAndRemembersLastKind(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := &BugService{store: st, dataDir: t.TempDir()}
	_ = st.SetSetting("ai.enabled", "false")

	saved, err := svc.Save(&models.Bug{Title: "买牛奶"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Kind != models.KindTodo || saved.KindSource != models.KindSourceHeuristic {
		t.Fatalf("should detect todo: %+v", saved)
	}
	if st.GetSetting("capture.lastKind") != models.KindTodo {
		t.Fatalf("last kind not persisted: %q", st.GetSetting("capture.lastKind"))
	}

	saved, err = svc.Save(&models.Bug{Title: "登录按钮报错"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Kind != models.KindBug {
		t.Fatalf("should detect bug: %+v", saved)
	}

	manual, err := svc.Save(&models.Bug{Title: "记得明天开会", Kind: models.KindBug, KindSource: models.KindSourceUser})
	if err != nil {
		t.Fatal(err)
	}
	if manual.Kind != models.KindBug || manual.KindSource != models.KindSourceUser {
		t.Fatalf("manual kind should not be overridden by heuristics: %+v", manual)
	}
}

func TestSetKindMapsVerifiedToFixedForTodo(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := &BugService{store: st, dataDir: t.TempDir()}
	item := &models.Bug{Kind: models.KindBug, KindSource: models.KindSourceUser, Title: "已验证的 Bug", Status: models.StatusVerified}
	if err := st.SaveBug(item); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetKind(item.ID, models.KindTodo, true); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetBug(item.ID)
	if got.Kind != models.KindTodo || got.KindSource != models.KindSourceUser || got.Status != models.StatusFixed {
		t.Fatalf("todo kind should map verified to fixed: %+v", got)
	}
}

func TestAnalyzeAsyncAutoSwitchesKind(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	due := time.Now().AddDate(0, 0, 1).Format("2006-01-02") + "T09:00"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{
				"type": "text",
				"text": `{"kind":"todo","kindConfidence":0.95,"title":"交房租","category":"other","severity":"P1","tags":["房租"],"summary":"明天交房租。","steps":[],"due":"` + due + `"}`,
			}},
		})
	}))
	defer srv.Close()

	_ = st.SetSetting("ai.enabled", "true")
	_ = st.SetSetting("ai.baseURL", srv.URL)
	_ = st.SetSetting("ai.apiKey", "test-key")
	_ = st.SetSetting("ai.model", "test-model")

	item := &models.Bug{Kind: models.KindBug, KindSource: models.KindSourceHeuristic, Title: "交房租", Status: models.StatusTodo, DescriptionText: "明天要交房租"}
	if err := st.SaveBug(item); err != nil {
		t.Fatal(err)
	}
	svc := &BugService{store: st, dataDir: t.TempDir()}
	svc.analyzeAsync(item.ID)

	got, err := st.GetBug(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != models.KindTodo || got.KindSource != models.KindSourceAI {
		t.Fatalf("high-confidence AI should switch kind: %+v", got)
	}
	if got.DueAt == 0 {
		t.Fatalf("AI due should be applied: %+v", got)
	}
	if got.AIStatus != models.AIDone || got.AI == nil || got.AI.KindConfidence < 0.9 {
		t.Fatalf("AI suggestion should be persisted in the same update: %+v", got)
	}
}

func TestAnalyzeAsyncDoesNotOverrideUserKind(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{
				"type": "text",
				"text": `{"kind":"todo","kindConfidence":0.99,"title":"也许像待办","category":"other","severity":"P2","tags":[],"summary":"测试。","steps":[]}`,
			}},
		})
	}))
	defer srv.Close()

	_ = st.SetSetting("ai.enabled", "true")
	_ = st.SetSetting("ai.baseURL", srv.URL)
	_ = st.SetSetting("ai.apiKey", "test-key")
	_ = st.SetSetting("ai.model", "test-model")

	item := &models.Bug{Kind: models.KindBug, KindSource: models.KindSourceUser, Title: "用户认定的 Bug", Status: models.StatusTodo}
	if err := st.SaveBug(item); err != nil {
		t.Fatal(err)
	}
	svc := &BugService{store: st, dataDir: t.TempDir()}
	svc.analyzeAsync(item.ID)

	got, _ := st.GetBug(item.ID)
	if got.Kind != models.KindBug || got.KindSource != models.KindSourceUser {
		t.Fatalf("user-locked kind must not change: %+v", got)
	}
}

func TestUndoKindChangeRestoresStatusAndAIDue(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	due := time.Now().Add(24 * time.Hour).Unix()
	item := &models.Bug{
		Kind: models.KindTodo, KindSource: models.KindSourceAI, Title: "交房租",
		Status: models.StatusFixed, DueAt: due, DueAllDay: false,
	}
	if err := st.SaveBug(item); err != nil {
		t.Fatal(err)
	}
	svc := &BugService{store: st, dataDir: t.TempDir()}
	err = svc.UndoKindChange(KindChangeUndo{
		ID: item.ID, From: models.KindBug, To: models.KindTodo,
		FromStatus: models.StatusVerified, FromDueAt: 0, ToDueAt: due,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetBug(item.ID)
	if got.Kind != models.KindBug || got.KindSource != models.KindSourceUser ||
		got.Status != models.StatusVerified || got.DueAt != 0 || got.DueAllDay {
		t.Fatalf("undo should restore kind/status and clear AI due: %+v", got)
	}

	other := &models.Bug{
		Kind: models.KindTodo, KindSource: models.KindSourceAI, Title: "用户已推进",
		Status: models.StatusDoing,
	}
	if err := st.SaveBug(other); err != nil {
		t.Fatal(err)
	}
	if err := svc.UndoKindChange(KindChangeUndo{
		ID: other.ID, From: models.KindBug, To: models.KindTodo, FromStatus: models.StatusVerified,
	}); err != nil {
		t.Fatal(err)
	}
	gotOther, _ := st.GetBug(other.ID)
	if gotOther.Kind != models.KindBug || gotOther.Status != models.StatusDoing {
		t.Fatalf("undo must not overwrite a status changed after the switch: %+v", gotOther)
	}
}

func TestUndoKindChangeRefusesAfterUserLock(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	item := &models.Bug{Kind: models.KindTodo, KindSource: models.KindSourceUser, Title: "手动选择"}
	if err := st.SaveBug(item); err != nil {
		t.Fatal(err)
	}
	svc := &BugService{store: st, dataDir: t.TempDir()}
	if err := svc.UndoKindChange(KindChangeUndo{
		ID: item.ID, From: models.KindBug, To: models.KindTodo,
	}); err == nil {
		t.Fatal("undo should be refused after the user locked the kind")
	}
	got, _ := st.GetBug(item.ID)
	if got.Kind != models.KindTodo || got.KindSource != models.KindSourceUser {
		t.Fatalf("refused undo must leave the record untouched: %+v", got)
	}
}

func TestAIEntrypointsRequireEnabled(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	_ = st.SetSetting("ai.enabled", "false")
	_ = st.SetSetting("ai.apiKey", "test-key")
	_ = st.SetSetting("ai.model", "test-model")
	item := &models.Bug{Title: "待分析"}
	if err := st.SaveBug(item); err != nil {
		t.Fatal(err)
	}
	svc := &BugService{store: st, dataDir: t.TempDir()}
	if err := svc.Reanalyze(item.ID); err == nil {
		t.Fatal("Reanalyze should reject a disabled AI config")
	}
	if _, err := svc.AnalyzeUnanalyzed(); err == nil {
		t.Fatal("AnalyzeUnanalyzed should reject a disabled AI config")
	}
	got, _ := st.GetBug(item.ID)
	if got.AIStatus == models.AIPending {
		t.Fatalf("disabled AI must not leave a pending status: %+v", got)
	}
}

func TestSaveUpdatesExistingOnly(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_ = st.SetSetting("ai.enabled", "false")

	item := &models.Bug{Title: "原标题", Kind: models.KindBug, KindSource: models.KindSourceUser}
	if err := st.SaveBug(item); err != nil {
		t.Fatal(err)
	}
	created := item.CreatedAt
	svc := &BugService{store: st, dataDir: t.TempDir()}
	item.Title = "新标题"
	item.Status = models.StatusDoing
	updated, err := svc.Save(item)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "新标题" || updated.Status != models.StatusDoing || updated.CreatedAt != created {
		t.Fatalf("existing save mismatch: %+v", updated)
	}
	if _, err := svc.Save(&models.Bug{ID: "missing-id", Title: "不应插入"}); err == nil {
		t.Fatal("saving a missing id must not insert a new record")
	}
}
