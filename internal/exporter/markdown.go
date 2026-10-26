// Package exporter 把 bug 清单导出为可分享的 Markdown 目录（含截图副本）。
package exporter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lybxkl/remember-fix/internal/fsx"
	"github.com/lybxkl/remember-fix/internal/models"
)

var statusOrderBug = []struct{ key, label string }{
	{models.StatusTodo, "待处理"},
	{models.StatusDoing, "进行中"},
	{models.StatusFixed, "已修复"},
	{models.StatusVerified, "已验证"},
	{models.StatusWontfix, "不修"},
}

var statusOrderTodo = []struct{ key, label string }{
	{models.StatusTodo, "待办"},
	{models.StatusDoing, "进行中"},
	{models.StatusFixed, "已完成"},
	{models.StatusVerified, "已完成"},
	{models.StatusWontfix, "已搁置"},
}

var kindOrder = []struct{ key, label string }{
	{models.KindBug, "Bug"},
	{models.KindTodo, "待办"},
}

var categoryLabels = map[string]string{
	models.CategoryUI: "界面 UI", models.CategoryLogic: "逻辑", models.CategoryCrash: "崩溃",
	models.CategoryPerformance: "性能", models.CategoryCompat: "兼容性", models.CategoryData: "数据",
	models.CategorySecurity: "安全", models.CategoryOther: "其他", models.CategoryUncategorized: "未分类",
}

// Markdown 将全部事项导出到 <dataDir>/exports/remember-fix-<时间戳>/items.md。
// 截图会复制到同级 assets/ 目录，便于整个文件夹直接分享。
func Markdown(dataDir string, items []*models.Bug) (string, error) {
	stamp := time.Now().Format("20060102-150405")
	dir := filepath.Join(dataDir, "exports", "remember-fix-"+stamp)
	assetsDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return "", fmt.Errorf("创建导出目录失败: %w", err)
	}

	var bugs, todos int
	for _, item := range items {
		if item.Kind == models.KindTodo {
			todos++
		} else {
			bugs++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# 记得修 · 事项导出\n\n")
	fmt.Fprintf(&b, "> 导出时间：%s  \n> 共 %d 条记录（Bug %d · 待办 %d）\n\n",
		time.Now().Format("2006-01-02 15:04"), len(items), bugs, todos)

	copied := map[string]string{}
	written := map[string]bool{}
	for _, kind := range kindOrder {
		var group []*models.Bug
		for _, item := range items {
			if item.Kind == kind.key {
				group = append(group, item)
			}
		}
		if len(group) == 0 {
			continue
		}
		written[kind.key] = true
		fmt.Fprintf(&b, "## %s（%d）\n\n", kind.label, len(group))
		writeStatusGroups(&b, group, kind.key, dataDir, assetsDir, copied)
	}
	// 兜底：未知类型的记录也不能丢。
	var other []*models.Bug
	for _, item := range items {
		if !written[item.Kind] {
			other = append(other, item)
		}
	}
	if len(other) > 0 {
		fmt.Fprintf(&b, "## 其他事项（%d）\n\n", len(other))
		writeStatusGroups(&b, other, models.KindBug, dataDir, assetsDir, copied)
	}

	file := filepath.Join(dir, "items.md")
	if err := os.WriteFile(file, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("写入导出文件失败: %w", err)
	}
	return file, nil
}

func writeStatusGroups(b *strings.Builder, items []*models.Bug, kind, dataDir, assetsDir string, copied map[string]string) {
	order := statusOrderBug
	if kind == models.KindTodo {
		order = statusOrderTodo
	}
	seen := map[string]bool{}
	for _, group := range order {
		if seen[group.key] {
			continue
		}
		seen[group.key] = true
		var groupItems []*models.Bug
		for _, item := range items {
			if item.Status == group.key {
				groupItems = append(groupItems, item)
			}
		}
		if len(groupItems) == 0 {
			continue
		}
		fmt.Fprintf(b, "### %s（%d）\n\n", group.label, len(groupItems))
		for _, item := range groupItems {
			writeItem(b, item, kind, dataDir, assetsDir, copied)
		}
	}
	var other []*models.Bug
	for _, item := range items {
		known := false
		for _, group := range order {
			if item.Status == group.key {
				known = true
				break
			}
		}
		if !known {
			other = append(other, item)
		}
	}
	if len(other) > 0 {
		fmt.Fprintf(b, "### 其他（%d）\n\n", len(other))
		for _, item := range other {
			writeItem(b, item, kind, dataDir, assetsDir, copied)
		}
	}
}

func writeItem(b *strings.Builder, item *models.Bug, kind, dataDir, assetsDir string, copied map[string]string) {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = firstLine(item.DescriptionText)
	}
	if title == "" {
		title = "未命名事项"
	}
	fmt.Fprintf(b, "#### %s\n\n", title)
	if kind == models.KindTodo {
		fmt.Fprintf(b, "- 类型：待办 · 状态：%s · 优先级：%s\n", statusLabel(item.Status, kind), item.Severity)
		if item.DueAt > 0 {
			due := time.Unix(item.DueAt, 0).Format("2006-01-02")
			if !item.DueAllDay {
				due += " " + time.Unix(item.DueAt, 0).Format("15:04")
			} else {
				due += "（全天）"
			}
			fmt.Fprintf(b, "- 到期：%s\n", due)
		}
	} else {
		fmt.Fprintf(b, "- 类型：Bug · 状态：%s · 分类：%s · 严重度：%s\n",
			statusLabel(item.Status, kind), categoryLabel(item.Category), item.Severity)
	}
	if len(item.Tags) > 0 {
		fmt.Fprintf(b, "- 标签：%s\n", strings.Join(item.Tags, "、"))
	}
	fmt.Fprintf(b, "- 创建：%s", time.Unix(item.CreatedAt, 0).Format("2006-01-02 15:04"))
	if item.SourceApp != "" {
		fmt.Fprintf(b, " · 来源：%s", item.SourceApp)
	}
	fmt.Fprintf(b, "\n- ID：`%s`\n\n", item.ID)

	if strings.TrimSpace(item.DescriptionText) != "" {
		fmt.Fprintf(b, "%s\n\n", strings.TrimSpace(item.DescriptionText))
	}

	hasImages := false
	for _, att := range item.Attachments {
		if att.Type == "image" && att.Path != "" {
			if !hasImages {
				fmt.Fprintf(b, "**截图**\n\n")
				hasImages = true
			}
			name, ok := copyShot(dataDir, assetsDir, att.Path, copied)
			if ok {
				fmt.Fprintf(b, "![截图](assets/%s)\n\n", name)
			}
		}
		if att.Type == "text" && strings.TrimSpace(att.Text) != "" {
			fmt.Fprintf(b, "**相关文本**\n\n```\n%s\n```\n\n", strings.TrimSpace(att.Text))
		}
	}

	if item.AI != nil {
		fmt.Fprintf(b, "**AI 摘要**\n\n%s\n\n", item.AI.Summary)
		if kind != models.KindTodo && len(item.AI.Steps) > 0 {
			fmt.Fprintf(b, "**疑似复现步骤**\n\n")
			for i, step := range item.AI.Steps {
				fmt.Fprintf(b, "%d. %s\n", i+1, step)
			}
			fmt.Fprintln(b)
		}
	}
	fmt.Fprintf(b, "---\n\n")
}

func copyShot(dataDir, assetsDir, rel string, copied map[string]string) (string, bool) {
	if name, ok := copied[rel]; ok {
		return name, true
	}
	src := fsx.AbsPath(dataDir, rel)
	if src == "" {
		return "", false
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", false
	}
	name := filepath.Base(rel)
	dst := filepath.Join(assetsDir, name)
	if _, err := os.Stat(dst); err == nil {
		name = fmt.Sprintf("%d-%s", time.Now().UnixNano(), name)
		dst = filepath.Join(assetsDir, name)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", false
	}
	copied[rel] = name
	return name, true
}

func statusLabel(key, kind string) string {
	order := statusOrderBug
	if kind == models.KindTodo {
		order = statusOrderTodo
	}
	for _, s := range order {
		if s.key == key {
			return s.label
		}
	}
	return key
}

func categoryLabel(key string) string {
	if label, ok := categoryLabels[key]; ok {
		return label
	}
	return key
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
