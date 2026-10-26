package models

// 事项类型
const (
	KindBug  = "bug"
	KindTodo = "todo"
)

// 类型来源：本地判断可被 AI 高置信纠正；手动选择后锁定。
const (
	KindSourceHeuristic = "heuristic"
	KindSourceAI        = "ai"
	KindSourceUser      = "user"
)

// Bug 状态
const (
	StatusTodo     = "todo"
	StatusDoing    = "doing"
	StatusFixed    = "fixed"
	StatusVerified = "verified"
	StatusWontfix  = "wontfix"
)

// Bug 分类
const (
	CategoryUI            = "ui"
	CategoryLogic         = "logic"
	CategoryCrash         = "crash"
	CategoryPerformance   = "performance"
	CategoryCompat        = "compat"
	CategoryData          = "data"
	CategorySecurity      = "security"
	CategoryOther         = "other"
	CategoryUncategorized = "uncategorized"
)

// AI 分析状态
const (
	AINone    = "none"
	AIPending = "pending"
	AIDone    = "done"
	AIFailed  = "failed"
)

func ValidKind(kind string) bool {
	return kind == KindBug || kind == KindTodo
}

func ValidKindSource(source string) bool {
	return source == KindSourceHeuristic || source == KindSourceAI || source == KindSourceUser
}

func ValidStatus(status string) bool {
	switch status {
	case StatusTodo, StatusDoing, StatusFixed, StatusVerified, StatusWontfix:
		return true
	default:
		return false
	}
}

// IsOpen 表示事项还没结束（待处理 / 进行中）。
func IsOpen(status string) bool {
	return status == StatusTodo || status == StatusDoing
}

type Attachment struct {
	Type string `json:"type"` // image | text
	Path string `json:"path"` // image: 相对数据目录的路径 shots/2026/10/xx.png
	Text string `json:"text"` // type=text 时的内容（截断）
}

type AISuggestion struct {
	Kind           string   `json:"kind"`
	KindConfidence float64  `json:"kindConfidence"`
	Title          string   `json:"title"`
	Category       string   `json:"category"`
	Severity       string   `json:"severity"`
	Tags           []string `json:"tags"`
	Summary        string   `json:"summary"`
	Steps          []string `json:"steps"`
	DueAt          int64    `json:"dueAt"`
	DueAllDay      bool     `json:"dueAllDay"`
	Model          string   `json:"model"`
	At             int64    `json:"at"`
}

// Bug 是库里的一条事项；为兼容旧数据表仍沿用 bugs 表名与 Bug 类型名。
type Bug struct {
	ID              string        `json:"id"`
	Kind            string        `json:"kind"`
	KindSource      string        `json:"kindSource"`
	Title           string        `json:"title"`
	Description     string        `json:"description"`     // TipTap JSON
	DescriptionText string        `json:"descriptionText"` // 纯文本，搜索/分析用
	Status          string        `json:"status"`
	Category        string        `json:"category"`
	Severity        string        `json:"severity"`
	DueAt           int64         `json:"dueAt"`
	DueAllDay       bool          `json:"dueAllDay"`
	Tags            []string      `json:"tags"`
	Attachments     []Attachment  `json:"attachments"`
	SourceApp       string        `json:"sourceApp"`
	AI              *AISuggestion `json:"ai"`
	AIStatus        string        `json:"aiStatus"`
	CreatedAt       int64         `json:"createdAt"`
	UpdatedAt       int64         `json:"updatedAt"`
	ResolvedAt      *int64        `json:"resolvedAt"`
}
