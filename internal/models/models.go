package models

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

type Attachment struct {
	Type string `json:"type"` // image | text
	Path string `json:"path"` // image: 相对数据目录的路径 shots/2026/10/xx.png
	Text string `json:"text"` // type=text 时的内容（截断）
}

type AISuggestion struct {
	Title    string   `json:"title"`
	Category string   `json:"category"`
	Severity string   `json:"severity"`
	Tags     []string `json:"tags"`
	Summary  string   `json:"summary"`
	Steps    []string `json:"steps"`
	Model    string   `json:"model"`
	At       int64    `json:"at"`
}

type Bug struct {
	ID              string        `json:"id"`
	Title           string        `json:"title"`
	Description     string        `json:"description"`     // TipTap JSON
	DescriptionText string        `json:"descriptionText"` // 纯文本，搜索/分析用
	Status          string        `json:"status"`
	Category        string        `json:"category"`
	Severity        string        `json:"severity"`
	Tags            []string      `json:"tags"`
	Attachments     []Attachment  `json:"attachments"`
	SourceApp       string        `json:"sourceApp"`
	AI              *AISuggestion `json:"ai"`
	AIStatus        string        `json:"aiStatus"`
	CreatedAt       int64         `json:"createdAt"`
	UpdatedAt       int64         `json:"updatedAt"`
	ResolvedAt      *int64        `json:"resolvedAt"`
}
