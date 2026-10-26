package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store 封装 SQLite 访问。桌面单进程应用，单连接 + WAL 足够。
type Store struct {
	db *sql.DB
}

// 默认配置（首次启动种子）。AI 走 Anthropic 协议兼容端点。
// 该端点实测可用 qwen3.8-max：支持视觉输入，适合截图分析。
const (
	DefaultAIBaseURL = "https://token-plan.cn-beijing.maas.aliyuncs.com/apps/anthropic"
	DefaultAIModel   = "qwen3.8-max"
)

func Open(dataDir string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)",
		filepath.Join(dataDir, "remember-fix.db"))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	s.seedSettings()
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS bugs (
  id               TEXT PRIMARY KEY,
  title            TEXT NOT NULL DEFAULT '',
  description      TEXT NOT NULL DEFAULT '',
  description_text TEXT NOT NULL DEFAULT '',
  status           TEXT NOT NULL DEFAULT 'todo',
  category         TEXT NOT NULL DEFAULT 'uncategorized',
  severity         TEXT NOT NULL DEFAULT 'P2',
  tags             TEXT NOT NULL DEFAULT '[]',
  attachments      TEXT NOT NULL DEFAULT '[]',
  source_app       TEXT NOT NULL DEFAULT '',
  ai               TEXT NOT NULL DEFAULT '',
  ai_status        TEXT NOT NULL DEFAULT 'none',
  created_at       INTEGER NOT NULL,
  updated_at       INTEGER NOT NULL,
  resolved_at      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_bugs_status   ON bugs(status);
CREATE INDEX IF NOT EXISTS idx_bugs_category ON bugs(category);
CREATE INDEX IF NOT EXISTS idx_bugs_created  ON bugs(created_at);
CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);
`)
	if err != nil {
		return fmt.Errorf("建表失败: %w", err)
	}
	return nil
}

func (s *Store) seedSettings() {
	defaults := map[string]string{
		"ai.baseURL":            DefaultAIBaseURL,
		"ai.apiKey":             "",
		"ai.model":              DefaultAIModel,
		"ai.enabled":            "true",
		"hotkey":                DefaultHotkey,
		"edge.enabled":          "true",
		"edge.dock":             "right",
		"edge.offset":           "-1",
		"edge.reminder.enabled": "true",
	}
	for k, v := range defaults {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO settings(key, value) VALUES(?, ?)`, k, v); err != nil {
			slog.Warn("种子配置写入失败", "key", k, "err", err)
		}
	}
}

// GetSetting 读取配置，不存在返回空串。
func (s *Store) GetSetting(key string) string {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err != nil {
		return ""
	}
	return v
}

// SetSetting 写入配置。
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// GetAllSettings 返回全部配置。
func (s *Store) GetAllSettings() map[string]string {
	out := map[string]string{}
	rows, err := s.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			out[k] = v
		}
	}
	return out
}

// CleanupOrphanShots 删除未挂到任何 bug 上、且超过 1 小时的截图（捕获后放弃保存的残留）。
func (s *Store) CleanupOrphanShots(dataDir string) {
	used := map[string]bool{}
	rows, err := s.db.Query(`SELECT attachments FROM bugs`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			continue
		}
		var atts []struct {
			Path string `json:"path"`
		}
		if jsonUnmarshal(raw, &atts) == nil {
			for _, a := range atts {
				if a.Path != "" {
					used[a.Path] = true
				}
			}
		}
	}
	shotsDir := filepath.Join(dataDir, "shots")
	var candidates []string
	filepath.WalkDir(shotsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(dataDir, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !used[rel] && timeSince(info.ModTime()) > 3600 {
			candidates = append(candidates, path)
		}
		return nil
	})
	for _, p := range candidates {
		if err := os.Remove(p); err == nil {
			slog.Info("清理孤儿截图", "file", p)
		}
	}
}
