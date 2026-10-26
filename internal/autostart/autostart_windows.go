//go:build windows

// Package autostart 管理“开机自启”注册表项（当前用户级，无需管理员权限）。
package autostart

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey    = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueName = "RememberFix"
)

// Enabled 报告当前可执行文件是否已登记开机自启。
func Enabled() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	val, _, err := k.GetStringValue(valueName)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.Trim(val, `"`), exe)
}

// Set 开启或关闭开机自启。
func Set(enabled bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !enabled {
		if err := k.DeleteValue(valueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(valueName, `"`+exe+`"`)
}
