package fsx

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAbsPathStaysInsideDataDir(t *testing.T) {
	dir := t.TempDir()
	got := AbsPath(dir, "shots/2026/10/a.png")
	want := filepath.Join(dir, "shots", "2026", "10", "a.png")
	if got != want {
		t.Fatalf("AbsPath = %q, want %q", got, want)
	}

	for _, bad := range []string{"", ".", "..", "../secret", `..\secret`} {
		if got := AbsPath(dir, bad); got != "" {
			t.Errorf("AbsPath(%q) = %q, want empty", bad, got)
		}
	}
	if runtime.GOOS == "windows" {
		if got := AbsPath(dir, `C:\Windows\win.ini`); got != "" {
			t.Errorf("absolute volume path escaped: %q", got)
		}
	}
}

func TestSaveScreenshot(t *testing.T) {
	dir := t.TempDir()
	rel, err := SaveScreenshot(dir, []byte("png-bytes"), ".png")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rel, "shots/") || !strings.HasSuffix(rel, ".png") {
		t.Fatalf("unexpected relative path: %q", rel)
	}
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "png-bytes" {
		t.Fatalf("content = %q", data)
	}
}
