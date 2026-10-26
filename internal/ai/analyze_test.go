package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lybxkl/remember-fix/internal/models"
)

func TestExtractJSON(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"raw", `{"title":"x"}`, `{"title":"x"}`},
		{"fenced", "```json\n{\"title\":\"x\"}\n```", `{"title":"x"}`},
		{"chatter", `分析如下：{"title":"x"} 希望有帮助`, `{"title":"x"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractJSON(tc.input); got != tc.want {
				t.Fatalf("extractJSON = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAnalyzeBugSendsClipboardText(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{
				"type": "text",
				"text": `{"title":"空指针崩溃","category":"crash","severity":"P1","tags":["崩溃","空指针"],"summary":"点击保存时崩溃。","steps":["打开页面","点击保存"]}`,
			}},
		})
	}))
	defer srv.Close()

	bug := &models.Bug{
		Title:           "",
		DescriptionText: "",
		Status:          models.StatusTodo,
		Severity:        "P2",
		Attachments: []models.Attachment{{
			Type: "text",
			Text: "NullPointerException at SaveButton.handleClick(App.java:42)",
		}},
	}
	client := NewClient(srv.URL, "test-key", "test-model")
	sugg, err := AnalyzeBug(context.Background(), client, bug, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotBody, "NullPointerException") {
		t.Fatalf("request body did not include clipboard text: %s", gotBody)
	}
	if !strings.Contains(gotBody, "test-model") {
		t.Fatalf("request body did not include model: %s", gotBody)
	}
	if sugg.Title != "空指针崩溃" || sugg.Category != models.CategoryCrash || sugg.Severity != "P1" {
		t.Fatalf("unexpected suggestion: %+v", sugg)
	}
	if len(sugg.Steps) != 2 || len(sugg.Tags) != 2 {
		t.Fatalf("unexpected lists: %+v", sugg)
	}
}
