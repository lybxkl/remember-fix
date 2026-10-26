package store

import (
	"encoding/json"
	"errors"
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
	var dueAllDay int
	if err := scan(&b.ID, &b.Title, &b.Description, &b.DescriptionText, &b.Status, &b.Category,
		&b.Severity, &b.Kind, &b.KindSource, &b.DueAt, &dueAllDay, &tags, &atts, &b.SourceApp,
		&ai, &b.AIStatus, &b.CreatedAt, &b.UpdatedAt, &resolved); err != nil {
		return nil, err
	}
	b.DueAllDay = dueAllDay != 0
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
 kind, kind_source, due_at, due_all_day, tags, attachments, source_app, ai, ai_status,
 created_at, updated_at, resolved_at`

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
	_, err := s.saveBug(b, false)
	return err
}

// UpdateBug 只更新已存在的记录，绝不插入。用于后台 AI 回写等可能与删除
// 并发的路径，避免把用户刚删掉的记录重新“复活”。
func (s *Store) UpdateBug(b *models.Bug) (bool, error) {
	if b == nil || b.ID == "" {
		return false, errors.New("记录不存在")
	}
	return s.saveBug(b, true)
}

func (s *Store) saveBug(b *models.Bug, updateOnly bool) (bool, error) {
	if b == nil {
		return false, errors.New("记录不能为空")
	}
	now := time.Now().Unix()
	if b.ID == "" {
		if updateOnly {
			return false, errors.New("记录不存在")
		}
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
	if !models.ValidKind(b.Kind) {
		b.Kind = models.KindBug
	}
	if !models.ValidKindSource(b.KindSource) {
		b.KindSource = models.KindSourceHeuristic
	}
	if b.Category == "" {
		b.Category = models.CategoryUncategorized
	}
	if b.Severity == "" {
		b.Severity = "P2"
	}
	if b.DueAt <= 0 {
		b.DueAt = 0
		b.DueAllDay = false
	}
	if b.Kind == models.KindTodo && b.Status == models.StatusVerified {
		b.Status = models.StatusFixed
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
	dueAllDay := 0
	if b.DueAllDay && b.DueAt > 0 {
		dueAllDay = 1
	}
	if updateOnly {
		res, err := s.db.Exec(`UPDATE bugs SET
			title=?, description=?, description_text=?, status=?, category=?, severity=?,
			kind=?, kind_source=?, due_at=?, due_all_day=?, tags=?, attachments=?, source_app=?,
			ai=?, ai_status=?, updated_at=?, resolved_at=? WHERE id=?`,
			b.Title, b.Description, b.DescriptionText, b.Status, b.Category, b.Severity,
			b.Kind, b.KindSource, b.DueAt, dueAllDay,
			jsonMarshal(b.Tags), jsonMarshal(b.Attachments), b.SourceApp,
			aiJSON(b.AI), b.AIStatus, b.UpdatedAt, resolved, b.ID)
		if err != nil {
			return false, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return false, err
		}
		return n > 0, nil
	}
	_, err := s.db.Exec(`INSERT INTO bugs (`+bugColumns+`)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
  title=excluded.title, description=excluded.description, description_text=excluded.description_text,
  status=excluded.status, category=excluded.category, severity=excluded.severity,
  kind=excluded.kind, kind_source=excluded.kind_source, due_at=excluded.due_at, due_all_day=excluded.due_all_day,
  tags=excluded.tags, attachments=excluded.attachments, source_app=excluded.source_app,
  ai=excluded.ai, ai_status=excluded.ai_status, updated_at=excluded.updated_at, resolved_at=excluded.resolved_at`,
		b.ID, b.Title, b.Description, b.DescriptionText, b.Status, b.Category, b.Severity,
		b.Kind, b.KindSource, b.DueAt, dueAllDay,
		jsonMarshal(b.Tags), jsonMarshal(b.Attachments), b.SourceApp,
		aiJSON(b.AI), b.AIStatus, b.CreatedAt, b.UpdatedAt, resolved)
	return err == nil, err
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

// UpdateKind 只切换类型与来源；切到待办时把「已验证」归并为「已完成」。
func (s *Store) UpdateKind(id, kind, source string) error {
	_, err := s.db.Exec(`UPDATE bugs SET kind = ?, kind_source = ?,
		status = CASE WHEN ? = 'todo' AND status = 'verified' THEN 'fixed' ELSE status END,
		updated_at = ? WHERE id = ?`, kind, source, kind, time.Now().Unix(), id)
	return err
}

// UndoKindSwitch 撤销一次 AI 自动改型，并同时还原它造成的状态/到期副作用。
// 只在记录仍是 AI 判定、且类型仍落在目标类型时才更新，避免覆盖用户后续修改。
func (s *Store) UndoKindSwitch(id, kind, status string, dueAt int64, dueAllDay bool) (bool, error) {
	if dueAt <= 0 {
		dueAt = 0
		dueAllDay = false
	}
	allDay := 0
	if dueAllDay {
		allDay = 1
	}
	res, err := s.db.Exec(`UPDATE bugs SET kind = ?, kind_source = ?, status = ?,
		due_at = ?, due_all_day = ?, updated_at = ?
		WHERE id = ? AND kind_source = ?`,
		kind, models.KindSourceUser, status, dueAt, allDay, time.Now().Unix(), id, models.KindSourceAI)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
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

// ResetInFlightAI 在进程启动时清理上一次运行遗留的 pending 状态。
// 崩溃/强退不会执行收尾，若不重置，重试按钮会因 pending 永久禁用。
func (s *Store) ResetInFlightAI() (int64, error) {
	res, err := s.db.Exec(`UPDATE bugs SET ai_status = ? WHERE ai_status = ?`,
		models.AIFailed, models.AIPending)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
