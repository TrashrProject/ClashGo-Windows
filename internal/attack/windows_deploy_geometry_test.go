package attack

import (
	"image"
	"testing"
)

func TestWindowsDeployCorridorUsesWidestSafeNonBottomSide(t *testing.T) {
	zone := RedZone{Valid: true, BBox: image.Rect(120, 180, 700, 560)}
	side, p1, p2, free, ok := windowsDeployCorridor(zone, 860, 732, 622)
	if !ok {
		t.Fatal("expected valid corridor")
	}
	// left=120, right=160, top=180 => top must win.
	if side != "top" || free != 180 {
		t.Fatalf("side/free = %q/%d, want top/180", side, free)
	}
	if p1.Y >= zone.BBox.Min.Y || p2.Y >= zone.BBox.Min.Y {
		t.Fatalf("top corridor must stay outside red box: p1=%v p2=%v bbox=%v", p1, p2, zone.BBox)
	}
}

func TestWindowsDeployCorridorNeverSelectsBottomHUD(t *testing.T) {
	// Bottom has enormous space but must never be selected on Windows.
	zone := RedZone{Valid: true, BBox: image.Rect(130, 130, 760, 250)}
	side, _, _, _, ok := windowsDeployCorridor(zone, 860, 732, 622)
	if !ok {
		t.Fatal("expected valid corridor")
	}
	if side == "bottom" {
		t.Fatal("bottom HUD must never be selected as Windows deploy corridor")
	}
}

func TestWindowsDeployCorridorLeftStaysOutsideRedZoneAndHUD(t *testing.T) {
	zone := RedZone{Valid: true, BBox: image.Rect(210, 90, 820, 600)}
	side, p1, p2, _, ok := windowsDeployCorridor(zone, 860, 732, 622)
	if !ok || side != "left" {
		t.Fatalf("got ok=%v side=%q, want left", ok, side)
	}
	if p1.X >= zone.BBox.Min.X || p2.X >= zone.BBox.Min.X {
		t.Fatalf("left corridor crossed red boundary: p1=%v p2=%v bbox=%v", p1, p2, zone.BBox)
	}
	maxSafeY := 512 // floor(732 * 0.70); keep compile-time integer on Windows
	if p1.Y > maxSafeY || p2.Y > maxSafeY {
		t.Fatalf("corridor entered lower HUD: p1=%v p2=%v max=%d", p1, p2, maxSafeY)
	}
}

func TestWindowsDeployCorridorRejectsInvalidZone(t *testing.T) {
	if _, _, _, _, ok := windowsDeployCorridor(RedZone{}, 860, 732, 622); ok {
		t.Fatal("invalid red zone must not produce a corridor")
	}
}

func TestWindowsCategoryPriorityPreservesDeploymentOrder(t *testing.T) {
	cases := []struct {
		category string
		want     int
	}{
		{"Troop", 0},
		{"Hero", 1},
		{"Siege", 2},
		{"CC", 2},
		{"Spell", 3},
		{"Event", 0},
	}
	for _, tc := range cases {
		if got := windowsCategoryPriority(tc.category); got != tc.want {
			t.Errorf("priority(%q)=%d, want %d", tc.category, got, tc.want)
		}
	}
}

func TestWindowsDeployLineSafeRejectsRedZoneCrossing(t *testing.T) {
	zone := RedZone{Valid: true, BBox: image.Rect(180, 150, 720, 580)}
	if windowsDeployLineSafe(zone, 860, 732, 622, "left", image.Pt(200, 250), image.Pt(200, 420)) {
		t.Fatal("line inside/crossing red-zone X must be rejected")
	}
}

func TestWindowsDeployLineSafeRejectsLowerHUD(t *testing.T) {
	zone := RedZone{Valid: true, BBox: image.Rect(180, 150, 720, 580)}
	if windowsDeployLineSafe(zone, 860, 732, 622, "left", image.Pt(120, 610), image.Pt(120, 650)) {
		t.Fatal("line entering lower HUD must be rejected")
	}
}

func TestWindowsDeployCorridorFailsClosedWhenNoOutsideSpaceExists(t *testing.T) {
	// Red zone reaches every legal outer edge. Clamping a line would move it
	// back inside the red box, so the helper must fail closed instead.
	zone := RedZone{Valid: true, BBox: image.Rect(0, 0, 860, 600)}
	if _, _, _, _, ok := windowsDeployCorridor(zone, 860, 732, 622); ok {
		t.Fatal("expected no safe corridor when live red zone consumes all legal outer space")
	}
}

func TestWindowsAnonymousOneShotCategory(t *testing.T) {
	for _, category := range []string{"Siege", "CC"} {
		if !windowsAnonymousOneShotCategory(category) {
			t.Fatalf("%s must be treated as anonymous one-shot", category)
		}
	}
	for _, category := range []string{"Troop", "Hero", "Spell", ""} {
		if windowsAnonymousOneShotCategory(category) {
			t.Fatalf("%s must not use anonymous siege/CC one-shot guard", category)
		}
	}
}
