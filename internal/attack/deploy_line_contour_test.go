package attack

import (
	"image"
	"testing"

	"github.com/rs/zerolog"
)

func TestContourAwareDeployLineFollowsBoundary(t *testing.T) {
	zone := RedZone{
		Valid: true,
		BBox: image.Rect(180, 150, 680, 520),
		LeftBoundary: []image.Point{
			image.Pt(220, 160),
			image.Pt(205, 230),
			image.Pt(195, 300),
			image.Pt(210, 370),
			image.Pt(225, 440),
			image.Pt(240, 510),
		},
	}
	d := NewDeployLineCalculator(zerolog.Nop())
	line := d.Calculate(zone, 860, 732, 620, "left", 6)
	if len(line.Points) != 6 {
		t.Fatalf("points=%d, want 6", len(line.Points))
	}
	if !line.Outside || line.Side != "left" {
		t.Fatalf("unexpected line metadata: %+v", line)
	}
	// The contour is irregular, so the resulting X values must not collapse
	// to one BBox-derived constant line.
	distinct := map[int]bool{}
	for _, p := range line.Points {
		distinct[p.X] = true
	}
	if len(distinct) < 2 {
		t.Fatalf("contour-aware line collapsed to constant X: %+v", line.Points)
	}
}
