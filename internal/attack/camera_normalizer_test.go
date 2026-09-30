package attack

import (
	"image"
	"testing"
)

func TestMarginsForRedZone(t *testing.T) {
	z := RedZone{Valid: true, BBox: image.Rect(100, 80, 700, 540)}
	m := marginsForRedZone(z, 860, 620)
	if m.Left != 100 || m.Right != 160 || m.Top != 80 || m.Bottom != 80 {
		t.Fatalf("unexpected margins: %+v", m)
	}
}

func TestMarginForSideFallsBackToLargest(t *testing.T) {
	m := deploymentMargins{Left: 42, Right: 118, Top: 75, Bottom: 60}
	if got := marginForSide(m, "right"); got != 118 {
		t.Fatalf("right margin = %d, want 118", got)
	}
	if got := marginForSide(m, ""); got != 118 {
		t.Fatalf("fallback margin = %d, want 118", got)
	}
}

func TestPreferredPanMovesVillageAwayFromDeploySide(t *testing.T) {
	tests := []struct {
		side string
		check func(from, to image.Point) bool
	}{
		{"left", func(a, b image.Point) bool { return b.X > a.X && b.Y == a.Y }},
		{"right", func(a, b image.Point) bool { return b.X < a.X && b.Y == a.Y }},
		{"top", func(a, b image.Point) bool { return b.Y > a.Y && b.X == a.X }},
		{"bottom", func(a, b image.Point) bool { return b.Y < a.Y && b.X == a.X }},
	}
	for _, tc := range tests {
		from, to, ok := preferredPan(tc.side, 860, 620)
		if !ok {
			t.Fatalf("%s: expected pan", tc.side)
		}
		if !tc.check(from, to) {
			t.Fatalf("%s: unexpected pan %v -> %v", tc.side, from, to)
		}
	}
}
