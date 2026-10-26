//go:build !windows && !darwin

package autostart

func Enabled() bool { return false }

func Set(bool) error { return nil }
