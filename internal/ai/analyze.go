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
	"unicode/utf8"

	"golang.org/x/image/draw"

	"github.com/lybxkl/remember-fix/internal/models"
)

const analyzeSystemPrompt = `你是一名资深软件测试与缺陷管理助手。用户会给你一个 bug 的截图（可能有）和文字描述。
只输出一个裸 JSON 对象：不要 markdown 代码块，不要解释、注释或前后缀。
JSON 字段定义：
- "title": 简洁的 bug 标题，不超过 30 字，说清"哪里+什么问题"
- "category": 从以下枚举中选一个最贴切的: "ui"(界面/样式/布局), "logic"(功能逻辑错误), "crash"(崩溃/闪退/白屏), "performance"(卡顿/慢/内存), "compat"(兼容性/环境相关), "data"(数据错误/丢失/不一致), "security"(安全漏洞), "other"
- "severity": 从 "P0"(致命，无法使用/数据损坏), "P1"(严重，主要功能受损), "P2"(一般，次要功能问题), "P3"(轻微，体验瑕疵) 中选一个
- "tags": 3-6 个简短标签（中文，每个不超过 6 字），如 ["登录", "弹窗", "安卓端"]
- "summary": 用 2-3 句话概括这个 bug
- "steps": 猜测的复现步骤，字符串数组，没有依据时给出最可能的推测
若信息很少，做出合理推断，不要说"信息不足"。`

type analyzeResult struct {
	Title    string   `json:"title"`
	Category string   `json:"category"`
	Severity string   `json:"severity"`
	Tags     []string `json:"tags"`
	Summary  string   `json:"summary"`
	Steps    []string `json:"steps"`
}

var validCategories = map[string]bool{
	models.CategoryUI: true, models.CategoryLogic: true, models.CategoryCrash: true,
	models.CategoryPerformance: true, models.CategoryCompat: true, models.CategoryData: true,
	models.CategorySecurity: true, models.CategoryOther: true,
}
var validSeverities = map[string]bool{"P0": true, "P1": true, "P2": true, "P3": true}

// AnalyzeBug 调用视觉模型分析 bug（截图 + 描述），返回结构化建议。
// imagePathAbs: 截图绝对路径列表（自动压缩到合理大小再发送）。
func AnalyzeBug(ctx context.Context, c *Client, b *models.Bug, imagePathsAbs []string) (*models.AISuggestion, error) {
	var sb strings.Builder
	sb.WriteString("bug 描述：\n")
	desc := strings.TrimSpace(b.DescriptionText)
	if desc == "" {
		desc = "（用户未填写描述，请主要依据截图分析）"
	}
	sb.WriteString(desc)
	if utf8.RuneCountInString(desc) > 6000 {
		sb.Reset()
		sb.WriteString(truncate(desc, 6000))
	}
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
	raw, err := c.Send(ctx, analyzeSystemPrompt, blocks, 4096)
	if err != nil {
		return nil, err
	}

	var res analyzeResult
	if err := json.Unmarshal([]byte(extractJSON(raw)), &res); err != nil {
		return nil, fmt.Errorf("AI 输出不是合法 JSON: %w", err)
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
	return &models.AISuggestion{
		Title:    res.Title,
		Category: res.Category,
		Severity: res.Severity,
		Tags:     res.Tags,
		Summary:  res.Summary,
		Steps:    res.Steps,
		Model:    c.Model,
		At:       time.Now().Unix(),
	}, nil
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
