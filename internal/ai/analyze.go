package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/image/draw"

	"github.com/lybxkl/remember-fix/internal/models"
)

// analyzeSystemPrompt 让同一套异步分析同时服务 Bug 与待办：
// 先判断类型，再按类型补全字段，避免用户手动选择类型。
func analyzeSystemPrompt(now time.Time) string {
	weekdays := [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
	return `你是一名资深软件工程师兼个人事项整理助手。用户会给你一条随手记录，可能附带截图、剪贴板文本和来源应用。
先判断它属于哪种事项：
- "bug"：软件、网页或设备出现异常、报错、崩溃、样式错位、功能不符合预期，需要排查和修复。
- "todo"：用户自己后续要做、容易忘记的事情，例如提交、缴费、预约、购买、联系、准备、出行、学习；不一定与软件缺陷有关。
类型以内容本质为准：即使以“记得修”开头，只要描述的是软件异常，就判为 bug；反过来，讨论产品改进计划但没有异常证据时判为 todo。
只输出一个裸 JSON 对象：不要 markdown 代码块，不要解释、注释或前后缀。
JSON 字段定义：
- "kind": "bug" 或 "todo"
- "kindConfidence": 0 到 1 之间的小数，表示类型判断把握；证据弱时给低分
- "title": 简洁标题，不超过 30 字，说清“哪里/做什么 + 什么问题/要做什么”
- "category": 仅 bug 需要，从 "ui"(界面/样式/布局), "logic"(功能逻辑错误), "crash"(崩溃/闪退/白屏), "performance"(卡顿/慢/内存), "compat"(兼容性/环境相关), "data"(数据错误/丢失/不一致), "security"(安全漏洞), "other" 中选一个；todo 返回 "other"
- "severity": bug 用 "P0"(致命), "P1"(严重), "P2"(一般), "P3"(轻微)；todo 用 "P1"(紧急), "P2"(一般), "P3"(不急) 表示优先级
- "tags": 3-6 个简短标签（中文，每个不超过 6 字），如 ["登录", "弹窗", "安卓端"]
- "summary": 用 2-3 句话概括这条记录
- "steps": 仅 bug：猜测的复现步骤，字符串数组；todo 固定返回 []
- "due": 仅 todo，且用户明确提到日期或时间时才填写。全天用 "YYYY-MM-DD"，具体时间用 "YYYY-MM-DDTHH:MM"；没有明确时间返回 ""
当前时间：` + now.Format("2006-01-02 15:04") + `（` + weekdays[now.Weekday()] + `），相对时间（今天、明天、下周一、月底等）都按这个时间换算。
若信息很少，做出合理推断，不要说“信息不足”。`
}

type analyzeResult struct {
	Kind           string   `json:"kind"`
	KindConfidence float64  `json:"kindConfidence"`
	Title          string   `json:"title"`
	Category       string   `json:"category"`
	Severity       string   `json:"severity"`
	Tags           []string `json:"tags"`
	Summary        string   `json:"summary"`
	Steps          []string `json:"steps"`
	Due            string   `json:"due"`
}

var validCategories = map[string]bool{
	models.CategoryUI: true, models.CategoryLogic: true, models.CategoryCrash: true,
	models.CategoryPerformance: true, models.CategoryCompat: true, models.CategoryData: true,
	models.CategorySecurity: true, models.CategoryOther: true,
}
var validSeverities = map[string]bool{"P0": true, "P1": true, "P2": true, "P3": true}
var validKinds = map[string]bool{models.KindBug: true, models.KindTodo: true}

// AnalyzeBug 调用视觉模型分析一条事项（截图 + 描述），返回结构化建议。
// imagePathAbs: 截图绝对路径列表（自动压缩到合理大小再发送）。
func AnalyzeBug(ctx context.Context, c *Client, b *models.Bug, imagePathsAbs []string) (*models.AISuggestion, error) {
	now := time.Now()
	var sb strings.Builder
	sb.WriteString("事项描述：\n")
	desc := strings.TrimSpace(b.DescriptionText)
	if desc == "" {
		desc = "（用户未填写描述，请主要依据截图分析）"
	}
	sb.WriteString(truncate(desc, 6000))
	if b.SourceApp != "" {
		sb.WriteString("\n\n来源应用窗口：" + b.SourceApp)
	}
	var clipTexts []string
	for _, att := range b.Attachments {
		if att.Type == "text" && strings.TrimSpace(att.Text) != "" {
			clipTexts = append(clipTexts, truncate(strings.TrimSpace(att.Text), 3000))
		}
	}
	if len(clipTexts) > 0 {
		sb.WriteString("\n\n剪贴板文本/日志：\n")
		for i, text := range clipTexts {
			if i > 0 {
				sb.WriteString("\n---\n")
			}
			sb.WriteString(text)
		}
	}
	sb.WriteString(fmt.Sprintf("\n\n当前状态：%s，严重度：%s", b.Status, b.Severity))
	if len(imagePathsAbs) > 0 {
		sb.WriteString(fmt.Sprintf("\n（附带 %d 张截图）", len(imagePathsAbs)))
	}

	blocks := []any{TextBlock{Type: "text", Text: sb.String()}}
	for i, p := range imagePathsAbs {
		if i >= 4 { // 防御性上限
			break
		}
		data, mediaType, err := loadImageForSend(p)
		if err != nil {
			continue
		}
		blocks = append(blocks, ImageBlock{
			Type:   "image",
			Source: ImageSource{Type: "base64", MediaType: mediaType, Data: base64.StdEncoding.EncodeToString(data)},
		})
	}

	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	raw, err := c.Send(ctx, analyzeSystemPrompt(now), blocks, 4096)
	if err != nil {
		return nil, err
	}

	var res analyzeResult
	if err := json.Unmarshal([]byte(extractJSON(raw)), &res); err != nil {
		return nil, fmt.Errorf("AI 输出不是合法 JSON: %w", err)
	}
	if !validKinds[res.Kind] {
		res.Kind = models.KindBug
		res.KindConfidence = 0
	}
	if res.KindConfidence < 0 {
		res.KindConfidence = 0
	}
	if res.KindConfidence > 1 {
		res.KindConfidence = 1
	}
	if !validCategories[res.Category] {
		res.Category = models.CategoryOther
	}
	if !validSeverities[res.Severity] {
		res.Severity = "P2"
	}
	if res.Tags == nil {
		res.Tags = []string{}
	}
	if res.Steps == nil {
		res.Steps = []string{}
	}
	if res.Kind == models.KindTodo {
		res.Steps = []string{}
	}
	dueAt, dueAllDay := int64(0), false
	if res.Kind == models.KindTodo {
		dueAt, dueAllDay = parseDue(res.Due, now)
	}
	return &models.AISuggestion{
		Kind:           res.Kind,
		KindConfidence: res.KindConfidence,
		Title:          res.Title,
		Category:       res.Category,
		Severity:       res.Severity,
		Tags:           res.Tags,
		Summary:        res.Summary,
		Steps:          res.Steps,
		DueAt:          dueAt,
		DueAllDay:      dueAllDay,
		Model:          c.Model,
		At:             time.Now().Unix(),
	}, nil
}

// parseDue 解析模型给出的日期/时间。日期按本地零点存为全天事项；
// 已过去的日期或过远的日期直接丢弃，避免把“明天”误解析成历史时间。
func parseDue(raw string, now time.Time) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	layouts := []struct {
		layout string
		allDay bool
	}{
		{"2006-01-02T15:04:05", false},
		{"2006-01-02T15:04", false},
		{"2006-01-02 15:04", false},
		{"2006-01-02", true},
	}
	var parsed time.Time
	allDay := false
	for _, l := range layouts {
		t, err := time.ParseInLocation(l.layout, raw, now.Location())
		if err == nil {
			parsed, allDay = t, l.allDay
			break
		}
	}
	if parsed.IsZero() {
		return 0, false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if parsed.Before(today) || parsed.After(now.AddDate(5, 0, 0)) {
		return 0, false
	}
	return parsed.Unix(), allDay
}

// jsonBlockRe 匹配 ```json ... ``` 围栏代码块。
var jsonBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")

// extractJSON 从模型输出中提取 JSON 对象（围栏块优先，其次首尾大括号）。
func extractJSON(s string) string {
	if m := jsonBlockRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

// loadImageForSend 读取图片文件；超过 1.2MB 时缩放到最大边 1568px 并转 JPEG，控制 API 请求体积。
func loadImageForSend(path string) ([]byte, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	if len(data) <= 1200*1024 {
		return data, mediaTypeByExt(path), nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// 解码失败但体积不大时仍然发送原始数据
		if len(data) <= 3600*1024 {
			return data, mediaTypeByExt(path), nil
		}
		return nil, "", err
	}
	const maxSide = 1568
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > maxSide || h > maxSide {
		scale := float64(maxSide) / float64(max(w, h))
		nw, nh := int(float64(w)*scale), int(float64(h)*scale)
		if nw < 1 {
			nw = 1
		}
		if nh < 1 {
			nh = 1
		}
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
		img = dst
	}
	enc, err := encodeJPEG(img, 85)
	if err != nil {
		return data, mediaTypeByExt(path), nil
	}
	return enc, "image/jpeg", nil
}

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func mediaTypeByExt(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	default:
		return "image/png"
	}
}
