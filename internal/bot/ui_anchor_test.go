package bot

import (
	"image"
	"testing"

	"gocv.io/x/gocv"
)

func TestLocateColoredButtonNearRequiresRealBlob(t *testing.T) {
	screen := gocv.NewMatWithSize(220, 320, gocv.MatTypeCV8UC3)
	defer screen.Close()

	spec := buttonColorSpec{
		low: gocv.NewScalar(0, 70, 110, 0),
		high: gocv.NewScalar(200, 255, 255, 0),
		minW: 18, minH: 12, minArea: 180,
		halfW: 70, halfH: 55,
	}

	// A valid orange/gold button-like rectangle centered at ~160,120.
	for y := 105; y < 136; y++ {
		for x := 138; x < 183; x++ {
			screen.SetVecbAt(y, x, gocv.Vecb{20, 150, 220})
		}
	}

	x, y, ok := locateColoredButtonNear(screen, image.Pt(160, 120), spec)
	if !ok {
		t.Fatal("expected cached local verification to find real button blob")
	}
	if x < 155 || x > 165 || y < 115 || y > 125 {
		t.Fatalf("unexpected localized center: %d,%d", x, y)
	}

	// The same image must NOT verify when the remembered anchor points at a
	// completely unrelated region. Full locator remains the fallback.
	if _, _, ok := locateColoredButtonNear(screen, image.Pt(40, 40), spec); ok {
		t.Fatal("cached verification must fail outside the actual button region")
	}
}

func TestButtonColorSpecsExistOnlyForVerifiedFastPathButtons(t *testing.T) {
	b := &Bot{}
	for _, name := range []string{"Attack", "Find Match", "Battle Attack", "Next"} {
		if _, ok := b.buttonColorSpec(name); !ok {
			t.Fatalf("missing color spec for %q", name)
		}
	}
	if _, ok := b.buttonColorSpec("Army Arrow"); ok {
		t.Fatal("Army Arrow must keep full existing locator path until a dedicated verified color spec exists")
	}
}
