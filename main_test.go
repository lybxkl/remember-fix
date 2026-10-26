package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEdgeOffset(t *testing.T) {
	cases := []struct {
		raw    string
		screen string
		want   int
		wantOK bool
	}{
		{"", "", 0, false},
		{"-1", "", 0, false},
		{"-1", "screen-2", -1, true},
		{"-960", "", -960, true},
		{"540", "screen-1", 540, true},
		{"abc", "screen-1", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseEdgeOffset(tc.raw, tc.screen)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("parseEdgeOffset(%q,%q) = (%d,%v), want (%d,%v)",
				tc.raw, tc.screen, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestValidExternalURL(t *testing.T) {
	cases := map[string]bool{
		"https://example.com/a":     true,
		"HTTP://example.com":        true,
		"http://localhost:5178":     true,
		"javascript:alert(1)":       false,
		"file:///etc/passwd":        false,
		"data:text/html,hello":      false,
		"//example.com/path":        false,
		"https://":                  false,
		"  https://example.com/x  ": true,
	}
	for raw, want := range cases {
		if got := validExternalURL(raw); got != want {
			t.Errorf("validExternalURL(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestAssetHandlerOnlyServesShotImages(t *testing.T) {
	dataDir := t.TempDir()
	shotsDir := filepath.Join(dataDir, "shots", "2026", "10")
	if err := os.MkdirAll(shotsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "remember-fix.db"), []byte("db-only"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "secret.png"), []byte("db-only"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shotsDir, "shot.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "shots", "flat.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shotsDir, "notes.txt"), []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := assetHandler(assets, dataDir)
	cases := []struct {
		path string
		want int
	}{
		{"/shots/shots/2026/10/shot.png", http.StatusOK},
		{"/shots/2026/10/shot.png", http.StatusOK},
		{"/shots/flat.png", http.StatusOK},
		{"/shots/", http.StatusNotFound},
		{"/shots/shots/2026/10/notes.txt", http.StatusNotFound},
		{"/shots/remember-fix.db", http.StatusNotFound},
		{"/shots/secret.png", http.StatusNotFound},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.path, rec.Code, tc.want)
		}
		if strings.Contains(rec.Body.String(), "db-only") {
			t.Errorf("%s leaked database contents", tc.path)
		}
	}
}
