package capture

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

type fakeClipboard struct {
	seq      uint32
	text     string
	hasText  bool
	image    []byte
	hasImage bool
}

func (f *fakeClipboard) Text() (string, bool)            { return f.text, f.hasText }
func (f *fakeClipboard) ImagePNG() ([]byte, bool, error) { return f.image, f.hasImage, nil }
func (f *fakeClipboard) Sequence() uint32                { return f.seq }
func (f *fakeClipboard) ForegroundWindowTitle() string   { return "测试窗口" }

func TestSnapshotDeduplicatesAndConsumes(t *testing.T) {
	dir := t.TempDir()
	clip := &fakeClipboard{seq: 7, image: []byte("fake-png"), hasImage: true}
	c := New(dir, clip, nil)

	first := c.SnapshotNow()
	if len(first.Items) != 1 || first.Items[0].Type != "image" {
		t.Fatalf("unexpected first snapshot: %+v", first)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "shots", "*", "*", "*.png"))
	if len(files) != 1 {
		t.Fatalf("expected 1 saved image, got %d", len(files))
	}

	second := c.SnapshotNow()
	if len(second.Items) != 1 {
		t.Fatalf("same sequence should not duplicate, got %d items", len(second.Items))
	}
	files, _ = filepath.Glob(filepath.Join(dir, "shots", "*", "*", "*.png"))
	if len(files) != 1 {
		t.Fatalf("same sequence should not write another file, got %d", len(files))
	}

	c.Consume(first.Items[0].ID)
	if got := c.Snapshot(); len(got.Items) != 0 {
		t.Fatalf("consumed item still present: %+v", got.Items)
	}
}

func TestTextIsTruncatedByRunes(t *testing.T) {
	clip := &fakeClipboard{seq: 1, text: strings.Repeat("错", 5000), hasText: true}
	c := New(t.TempDir(), clip, nil)
	snap := c.SnapshotNow()
	if len(snap.Items) != 1 || snap.Items[0].Type != "text" {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	text := snap.Items[0].Text
	if utf8.RuneCountInString(text) != textLimit+1 {
		t.Fatalf("truncated length = %d runes, want %d", utf8.RuneCountInString(text), textLimit+1)
	}
	if !strings.HasSuffix(text, "…") {
		t.Fatal("truncated text should end with ellipsis")
	}
}
