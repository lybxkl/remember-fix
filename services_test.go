package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestEdgeDockTarget(t *testing.T) {
	wa := application.Rect{X: 0, Y: 0, Width: 1920, Height: 1040}
	cases := []struct {
		name   string
		x, y   int
		dock   string
		offset int
	}{
		{"左边缘", 0, 400, "left", 516},
		{"右边缘", 1736, 400, "right", 516},
		{"上边缘", 868, 0, "top", 960},
		{"下边缘", 868, 808, "bottom", 960},
		{"上边比右边更近", 1500, 0, "top", 1592},
		{"左边比下边更近", 0, 700, "left", 816},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dock, offset := edgeDockTarget(wa, tc.x, tc.y, edgeExpandedWidth, edgeExpandedHeight)
			if dock != tc.dock || offset != tc.offset {
				t.Fatalf("edgeDockTarget(%d,%d) = (%s,%d), want (%s,%d)", tc.x, tc.y, dock, offset, tc.dock, tc.offset)
			}
		})
	}
}

func TestEdgeDockTargetUsesWindowCenter(t *testing.T) {
	wa := application.Rect{X: 1920, Y: 0, Width: 2560, Height: 1400}
	dock, offset := edgeDockTarget(wa, 3600, 100, 184, 232)
	if dock != "top" || offset != 3692 {
		t.Fatalf("edgeDockTarget = (%s,%d), want (top,3692)", dock, offset)
	}
}

func TestEdgeDockTargetClampsOffset(t *testing.T) {
	wa := application.Rect{X: 0, Y: 0, Width: 300, Height: 300}
	dock, offset := edgeDockTarget(wa, 200, 174, 184, 232)
	if dock != "right" || offset != 268 {
		t.Fatalf("edgeDockTarget = (%s,%d), want (right,268)", dock, offset)
	}
}
