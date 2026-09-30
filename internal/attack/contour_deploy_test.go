package attack

import (
	"image"
	"testing"

	"github.com/rs/zerolog"
)

func TestContourAwareDeployLineFollowsGeneratedBoundary(t *testing.T) {
	zone := RedZone{
		Valid: true,
		BBox:  image.Rect(200, 150, 660, 560),
	}
	// Irregular left boundary: x moves inward as y increases.
	for y := 170; y <= 540; y += 10 {
		x := 220 + (y-170)/10
		zone.LeftBoundary = append(zone.LeftBoundary, image.Pt(x, y))
		zone.RightBoundary = append(zone.RightBoundary, image.Pt(640, y))
	}
	for x := 220; x <= 640; x += 10 {
		zone.TopBoundary = append(zone.TopBoundary, image.Pt(x, 160))
		zone.BottomBoundary = append(zone.BottomBoundary, image.Pt(x, 550))
	}

	calc := NewDeployLineCalculator(zerolog.Nop())
	line := calc.Calculate(zone, 860, 732, 620, "left", 9)
	if len(line.Points) != 9 {
		t.Fatalf("got %d points, want 9", len(line.Points))
	}
	if line.Side != "left" || !line.Outside {
		t.Fatalf("unexpected line metadata: side=%q outside=%v", line.Side, line.Outside)
	}
	// A contour-aware line should not be perfectly vertical for this fake
	// slanted boundary.
	if line.Points[0].X == line.Points[len(line.Points)-1].X {
		t.Fatalf("expected contour-aware X variation, got vertical line at x=%d", line.Points[0].X)
	}
}
