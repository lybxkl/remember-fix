package fsx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DataDir 返回应用数据目录（%AppData%/remember-fix），确保 shots 子目录存在。
func DataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("获取用户配置目录失败: %w", err)
	}
	dir := filepath.Join(base, "remember-fix")
	if err := os.MkdirAll(filepath.Join(dir, "shots"), 0o755); err != nil {
		return "", fmt.Errorf("创建数据目录失败: %w", err)
	}
	return dir, nil
}

// SaveScreenshot 把图片 bytes 按 shots/年/月/uuid.png 落盘到指定数据目录，返回相对路径。
func SaveScreenshot(dataDir string, data []byte, ext string) (string, error) {
	now := time.Now()
	rel := filepath.ToSlash(filepath.Join("shots", now.Format("2006"), now.Format("01"), uuid.NewString()+ext))
	abs := filepath.Join(dataDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("创建截图目录失败: %w", err)
	}
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		return "", fmt.Errorf("写入截图失败: %w", err)
	}
	return rel, nil
}

// AbsPath 把相对数据目录的路径转成绝对路径；越界路径返回空串。
func AbsPath(dataDir, rel string) string {
	clean := filepath.Clean(filepath.FromSlash(rel))
	clean = strings.TrimLeft(clean, `/\`)
	if clean == "" || clean == "." || filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" {
		return ""
	}
	full := filepath.Join(dataDir, clean)
	back, err := filepath.Rel(dataDir, full)
	if err != nil || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) {
		return ""
	}
	return full
}
