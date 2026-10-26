//go:build darwin

package autostart

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const label = "com.rememberfix.desktop"

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

// launchTarget 返回 LaunchAgent 的启动命令和目标路径。
// 在 .app 内运行时通过 open 交给 LaunchServices，避免直接运行 bundle 内二进制。
func launchTarget() (args []string, target string, err error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, "", err
	}
	if strings.Contains(exe, ".app/Contents/MacOS/") {
		app := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
		return []string{"/usr/bin/open", app}, app, nil
	}
	return []string{exe}, exe, nil
}

// Enabled 报告当前用户是否已为本应用登记登录启动。
func Enabled() bool {
	path, err := plistPath()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	_, target, err := launchTarget()
	if err != nil {
		return false
	}
	return strings.Contains(string(data), xmlEscape(target))
}

// Set 写入或删除 LaunchAgent；登录后生效。
func Set(enabled bool) error {
	path, err := plistPath()
	if err != nil {
		return err
	}
	if !enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	args, _, err := launchTarget()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(plistContent(args)), 0o644)
}

func plistContent(args []string) string {
	var items strings.Builder
	for _, arg := range args {
		items.WriteString("    <string>" + xmlEscape(arg) + "</string>\n")
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
%s  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>LimitLoadToSessionType</key>
  <string>Aqua</string>
</dict>
</plist>
`, label, items.String())
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
