package exporter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lybxkl/remember-fix/internal/models"
)

func TestMarkdownCopiesScreenshotsAndIncludesAI(t *testing.T) {
	dataDir := t.TempDir()
	rel := filepath.ToSlash(filepath.Join("shots", "2026", "10", "abc.png"))
	abs := filepath.Join(dataDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("image-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	bug := &models.Bug{
		ID:              "bug-1",
		Kind:            models.KindBug,
		Title:           "保存按钮崩溃",
		DescriptionText: "点击保存后闪退",
		Status:          models.StatusTodo,
		Category:        models.CategoryCrash,
		Severity:        "P1",
		Tags:            []string{"崩溃"},
		SourceApp:       "Chrome",
		CreatedAt:       time.Now().Unix(),
		Attachments: []models.Attachment{
			{Type: "image", Path: rel},
			{Type: "text", Text: "NullPointerException"},
		},
		AI: &models.AISuggestion{
			Title:   "保存按钮空指针",
			Summary: "点击保存时访问空对象。",
			Steps:   []string{"打开页面", "点击保存"},
		},
	}
	todo := &models.Bug{
		ID:              "todo-1",
		Kind:            models.KindTodo,
		Title:           "交房租",
		Status:          models.StatusTodo,
		Severity:        "P1",
		DueAt:           time.Date(2026, 11, 2, 0, 0, 0, 0, time.Local).Unix(),
		DueAllDay:       true,
		CreatedAt:       time.Now().Unix(),
		Attachments:     []models.Attachment{},
		DescriptionText: "下周一之前交房租",
	}

	file, err := Markdown(dataDir, []*models.Bug{bug, todo})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, want := range []string{
		"保存按钮崩溃", "待处理", "AI 摘要", "NullPointerException", "assets/abc.png",
		"1. 打开页面", "## 待办（1）", "交房租", "优先级：P1", "到期：2026-11-02（全天）",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("export missing %q:\n%s", want, text)
		}
	}
	exportDir := filepath.Dir(file)
	copied, err := os.ReadFile(filepath.Join(exportDir, "assets", "abc.png"))
	if err != nil {
		t.Fatal(err)
	}
	if string(copied) != "image-bytes" {
		t.Fatalf("copied asset = %q", copied)
	}
}
