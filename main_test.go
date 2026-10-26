package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
