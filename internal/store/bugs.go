package store

import (
	"encoding/json"
	"time"

	"github.com/lybxkl/remember-fix/internal/models"

	"github.com/google/uuid"
)

func jsonMarshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func jsonUnmarshal(raw string, v any) error {
	return json.Unmarshal([]byte(raw), v)
}

func timeSince(t time.Time) time.Duration { return time.Since(t) }

func scanBug(scan func(dest ...any) error) (*models.Bug, error) {
	var b models.Bug
	var tags, atts string
	var ai string
	var resolved int64
	if err := scan(&b.ID, &b.Title, &b.Description, &b.DescriptionText, &b.Status, &b.Category,
		&b.Severity, &tags, &atts, &b.SourceApp, &ai, &b.AIStatus, &b.CreatedAt, &b.UpdatedAt, &resolved); err != nil {
		return nil, err
	}
	_ = jsonUnmarshal(tags, &b.Tags)
	_ = jsonUnmarshal(atts, &b.Attachments)
	if ai != "" {
		var s models.AISuggestion
		if jsonUnmarshal(ai, &s) == nil {
			b.AI = &s
		}
	}
	if resolved > 0 {
		r := resolved
		b.ResolvedAt = &r
	}
	return &b, nil
}

const bugColumns = `id, title, description, description_text, status, category, severity,
 tags, attachments, source_app, ai, ai_status, created_at, updated_at, resolved_at`

// ListBugs 返回全部 bug（按创建时间倒序）。本地单机数据量小，前端过滤。
func (s *Store) ListBugs() ([]*models.Bug, error) {
	rows, err := s.db.Query(`SELECT ` + bugColumns + ` FROM bugs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*models.Bug{}
	for rows.Next() {
		b, err := scanBug(rows.Scan)
		if err != nil {
			continue
		}
		out = append(out, b)
	}
	return out, nil
}

func (s *Store) GetBug(id string) (*models.Bug, error) {
	row := s.db.QueryRow(`SELECT `+bugColumns+` FROM bugs WHERE id = ?`, id)
	return scanBug(row.Scan)
}

// SaveBug 插入或更新（按 id 判断），并填充时间戳/默认值。
func (s *Store) SaveBug(b *models.Bug) error {
	now := time.Now().Unix()
	if b.ID == "" {
		if id, err := uuid.NewV7(); err == nil {
			b.ID = id.String()
		} else {
			b.ID = uuid.NewString()
		}
		b.CreatedAt = now
	}
	if b.UpdatedAt == 0 || b.UpdatedAt < now-1 {
		b.UpdatedAt = now
	}
	if b.Status == "" {
		b.Status = models.StatusTodo
	}
	if b.Category == "" {
		b.Category = models.CategoryUncategorized
	}
	if b.Severity == "" {
		b.Severity = "P2"
	}
	if b.Tags == nil {
		b.Tags = []string{}
	}
	if b.Attachments == nil {
		b.Attachments = []models.Attachment{}
	}
	var resolved int64
	if b.Status == models.StatusFixed || b.Status == models.StatusVerified {
		if b.ResolvedAt == nil || *b.ResolvedAt == 0 {
			b.ResolvedAt = &now
		}
		resolved = *b.ResolvedAt
	} else {
		b.ResolvedAt = nil
	}
	_, err := s.db.Exec(`INSERT INTO bugs (`+bugColumns+`)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
  title=excluded.title, description=excluded.description, description_text=excluded.description_text,
  status=excluded.status, category=excluded.category, severity=excluded.severity,
  tags=excluded.tags, attachments=excluded.attachments, source_app=excluded.source_app,
  ai=excluded.ai, ai_status=excluded.ai_status, updated_at=excluded.updated_at, resolved_at=excluded.resolved_at`,
		b.ID, b.Title, b.Description, b.DescriptionText, b.Status, b.Category, b.Severity,
		jsonMarshal(b.Tags), jsonMarshal(b.Attachments), b.SourceApp,
		aiJSON(b.AI), b.AIStatus, b.CreatedAt, b.UpdatedAt, resolved)
	return err
}

func aiJSON(a *models.AISuggestion) string {
	if a == nil {
		return ""
	}
	return jsonMarshal(a)
}

// UpdateStatus 更新状态；进入 fixed/verified 记录 resolved_at，离开则清零。
func (s *Store) UpdateStatus(id, status string) error {
	now := time.Now().Unix()
	resolved := 0
	if status == models.StatusFixed || status == models.StatusVerified {
		resolved = int(now)
	}
	_, err := s.db.Exec(`UPDATE bugs SET status = ?, resolved_at = ?, updated_at = ? WHERE id = ?`,
		status, resolved, now, id)
	return err
}

func (s *Store) DeleteBug(id string) error {
	_, err := s.db.Exec(`DELETE FROM bugs WHERE id = ?`, id)
	return err
}

// UpdateAI 只更新 AI 建议字段与状态。
func (s *Store) UpdateAI(id string, sugg *models.AISuggestion, status string) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(`UPDATE bugs SET ai = ?, ai_status = ?, updated_at = ? WHERE id = ?`,
		aiJSON(sugg), status, now, id)
	return err
}

// SetAIStatus 只切换分析状态，保留上一次成功的建议（失败/重试时不清空）。
func (s *Store) SetAIStatus(id, status string) error {
	_, err := s.db.Exec(`UPDATE bugs SET ai_status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().Unix(), id)
	return err
}
