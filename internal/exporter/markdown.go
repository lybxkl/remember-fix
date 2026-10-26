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

var statusOrder = []struct{ key, label string }{
	{models.StatusTodo, "待处理"},
	{models.StatusDoing, "进行中"},
	{models.StatusFixed, "已修复"},
	{models.StatusVerified, "已验证"},
	{models.StatusWontfix, "不修"},
}

var categoryLabels = map[string]string{
	models.CategoryUI: "界面 UI", models.CategoryLogic: "逻辑", models.CategoryCrash: "崩溃",
	models.CategoryPerformance: "性能", models.CategoryCompat: "兼容性", models.CategoryData: "数据",
	models.CategorySecurity: "安全", models.CategoryOther: "其他", models.CategoryUncategorized: "未分类",
}

// Markdown 将全部 bug 导出到 <dataDir>/exports/remember-fix-<时间戳>/bugs.md。
// 截图会复制到同级 assets/ 目录，便于整个文件夹直接分享。
func Markdown(dataDir string, bugs []*models.Bug) (string, error) {
	stamp := time.Now().Format("20060102-150405")
	dir := filepath.Join(dataDir, "exports", "remember-fix-"+stamp)
	assetsDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return "", fmt.Errorf("创建导出目录失败: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# 记得修 · Bug 导出\n\n")
	fmt.Fprintf(&b, "> 导出时间：%s  \n> 共 %d 条记录\n\n", time.Now().Format("2006-01-02 15:04"), len(bugs))

	copied := map[string]string{}
	for _, group := range statusOrder {
		var groupBugs []*models.Bug
		for _, bug := range bugs {
			if bug.Status == group.key {
				groupBugs = append(groupBugs, bug)
			}
		}
		if len(groupBugs) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s（%d）\n\n", group.label, len(groupBugs))
		for _, bug := range groupBugs {
			writeBug(&b, bug, dataDir, assetsDir, copied)
		}
	}
	// 兜底：未知状态的记录也不能丢。
	var other []*models.Bug
	for _, bug := range bugs {
		known := false
		for _, group := range statusOrder {
			if bug.Status == group.key {
				known = true
				break
			}
		}
		if !known {
			other = append(other, bug)
		}
	}
	if len(other) > 0 {
		fmt.Fprintf(&b, "## 其他（%d）\n\n", len(other))
		for _, bug := range other {
			writeBug(&b, bug, dataDir, assetsDir, copied)
		}
	}

	file := filepath.Join(dir, "bugs.md")
	if err := os.WriteFile(file, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("写入导出文件失败: %w", err)
	}
	return file, nil
}

func writeBug(b *strings.Builder, bug *models.Bug, dataDir, assetsDir string, copied map[string]string) {
	title := strings.TrimSpace(bug.Title)
	if title == "" {
		title = firstLine(bug.DescriptionText)
	}
	if title == "" {
		title = "未命名 Bug"
	}
	fmt.Fprintf(b, "### %s\n\n", title)
	fmt.Fprintf(b, "- 状态：%s · 分类：%s · 严重度：%s\n", statusLabel(bug.Status), categoryLabel(bug.Category), bug.Severity)
	if len(bug.Tags) > 0 {
		fmt.Fprintf(b, "- 标签：%s\n", strings.Join(bug.Tags, "、"))
	}
	fmt.Fprintf(b, "- 创建：%s", time.Unix(bug.CreatedAt, 0).Format("2006-01-02 15:04"))
	if bug.SourceApp != "" {
		fmt.Fprintf(b, " · 来源：%s", bug.SourceApp)
	}
	fmt.Fprintf(b, "\n- ID：`%s`\n\n", bug.ID)

	if strings.TrimSpace(bug.DescriptionText) != "" {
		fmt.Fprintf(b, "%s\n\n", strings.TrimSpace(bug.DescriptionText))
	}

	hasImages := false
	for _, att := range bug.Attachments {
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

	if bug.AI != nil {
		fmt.Fprintf(b, "**AI 摘要**\n\n%s\n\n", bug.AI.Summary)
		if len(bug.AI.Steps) > 0 {
			fmt.Fprintf(b, "**疑似复现步骤**\n\n")
			for i, step := range bug.AI.Steps {
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

func statusLabel(key string) string {
	for _, s := range statusOrder {
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
