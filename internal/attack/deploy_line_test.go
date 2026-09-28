package attack

import (
	"image"
	"testing"

	"github.com/rs/zerolog"
)

func TestDeployLineCountOneFallsBackToStablePointCount(t *testing.T) {
	calc := NewDeployLineCalculator(zerolog.Nop())
	zone := RedZone{Valid: true, BBox: image.Rect(180, 160, 700, 560)}
	line := calc.Calculate(zone, 860, 732, 622, "left", 1)
	if len(line.Points) != linePoints {
		t.Fatalf("count=1 produced %d points, want stable default %d", len(line.Points), linePoints)
	}
}

func TestDeployLineStaysOutsideRedZoneOnPreferredLeft(t *testing.T) {
	calc := NewDeployLineCalculator(zerolog.Nop())
	zone := RedZone{Valid: true, BBox: image.Rect(180, 160, 700, 560)}
	line := calc.Calculate(zone, 860, 732, 622, "left", 15)
	if len(line.Points) != 15 {
		t.Fatalf("got %d points, want 15", len(line.Points))
	}
	for i, p := range line.Points {
		if p.X >= zone.BBox.Min.X {
			t.Fatalf("point %d crossed red boundary: %v bbox=%v", i, p, zone.BBox)
		}
	}
}
