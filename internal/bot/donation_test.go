package bot

import (
	"image"
	"testing"

	"gocv.io/x/gocv"
)

func TestDonationVisualDeltaDetectsLocalChange(t *testing.T) {
	before := gocv.NewMatWithSize(120, 120, gocv.MatTypeCV8UC3)
	defer before.Close()
	after := before.Clone()
	defer after.Close()

	pt := image.Pt(60, 60)
	if got := donationVisualDelta(before, after, pt, 20); got != 0 {
		t.Fatalf("identical frames delta=%f want 0", got)
	}

	for y := 50; y < 70; y++ {
		for x := 50; x < 70; x++ {
			after.SetUCharAt(y, x*3+2, 220)
		}
	}
	if got := donationVisualDelta(before, after, pt, 20); got < 0.015 {
		t.Fatalf("changed donation card delta=%f want >= 0.015", got)
	}
}

func TestDonationVisualDeltaIgnoresChangeFarFromTarget(t *testing.T) {
	before := gocv.NewMatWithSize(120, 120, gocv.MatTypeCV8UC3)
	defer before.Close()
	after := before.Clone()
	defer after.Close()

	for y := 5; y < 25; y++ {
		for x := 5; x < 25; x++ {
			after.SetUCharAt(y, x*3+1, 255)
		}
	}

	if got := donationVisualDelta(before, after, image.Pt(90, 90), 12); got != 0 {
		t.Fatalf("far-away change delta=%f want 0", got)
	}
}
