//go:build !windows

package autostart

func Enabled() bool { return false }

func Set(bool) error { return nil }
