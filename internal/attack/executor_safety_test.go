package attack

import (
	"image"
	"testing"
)

func TestReliableLinePointsAvoidExactEndpoints(t *testing.T) {
	p1 := image.Pt(100, 200)
	p2 := image.Pt(100, 500)
	points := reliableLinePoints(p1, p2, 9)
	if len(points) != 9 {
		t.Fatalf("points=%d want 9", len(points))
	}
	if points[0] == p1 || points[len(points)-1] == p2 {
		t.Fatalf("reliable line must avoid exact endpoints: first=%v last=%v", points[0], points[len(points)-1])
	}
	wantFirstY := 236 // 200 + 12%% of 300
	wantLastY := 464  // 200 + 88%% of 300
	if points[0].Y != wantFirstY || points[len(points)-1].Y != wantLastY {
		t.Fatalf("inner spread changed: first=%v last=%v want y=%d/%d", points[0], points[len(points)-1], wantFirstY, wantLastY)
	}
	for _, p := range points {
		if p.X != 100 || p.Y <= 200 || p.Y >= 500 {
			t.Fatalf("point escaped verified inner line: %v", p)
		}
	}
}

func TestReliableLinePointsSingleUsesCenter(t *testing.T) {
	points := reliableLinePoints(image.Pt(100, 200), image.Pt(300, 400), 1)
	if len(points) != 1 || points[0] != image.Pt(200, 300) {
		t.Fatalf("single reliable point=%v want center (200,300)", points)
	}
}

func TestReliableLinePointsRejectsNonPositiveCount(t *testing.T) {
	if got := reliableLinePoints(image.Pt(1, 1), image.Pt(2, 2), 0); len(got) != 0 {
		t.Fatalf("zero count produced points: %v", got)
	}
}

func TestSanitizeWindowsDeployPointBlocksLowerHUD(t *testing.T) {
	got := sanitizeWindowsDeployPoint(image.Pt(500, 700), 860, 732)
	if got.Y > 512 { // floor(732 * .70)
		t.Fatalf("lower HUD point not clamped: %v", got)
	}
}

func TestSanitizeWindowsDeployPointProtectsSurrenderRegion(t *testing.T) {
	got := sanitizeWindowsDeployPoint(image.Pt(100, 700), 860, 732)
	wantY := 453 // floor(732 * 0.62); keep compile-time integer on Windows
	if got.Y != wantY {
		t.Fatalf("surrender-region point y=%d want %d", got.Y, wantY)
	}
}

func TestSanitizeWindowsDeployPointLeavesSafePointAlone(t *testing.T) {
	in := image.Pt(420, 360)
	if got := sanitizeWindowsDeployPoint(in, 860, 732); got != in {
		t.Fatalf("safe point changed: got=%v want=%v", got, in)
	}
}
