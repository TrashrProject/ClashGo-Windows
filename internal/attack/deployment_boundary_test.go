package attack

import (
	"image"
	"testing"
)

func TestNearestBoundaryPoint(t *testing.T) {
	points := []deploymentBoundaryPoint{
		{Point: image.Pt(100, 100)},
		{Point: image.Pt(140, 120)},
		{Point: image.Pt(220, 220)},
	}
	got, ok := nearestBoundaryPoint(points, image.Pt(135, 118), 30)
	if !ok {
		t.Fatal("expected nearby boundary point")
	}
	if got != (image.Pt(140, 120)) {
		t.Fatalf("got %v want %v", got, image.Pt(140, 120))
	}

	if _, ok := nearestBoundaryPoint(points, image.Pt(400, 400), 25); ok {
		t.Fatal("expected far target to remain unsnapped")
	}
}
