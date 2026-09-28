package bot

import (
	"image"
	"image/color"
	"testing"
	"time"

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
	// Use GoCV's drawing API instead of SetVecbAt, which is not available in
	// the Windows GoCV build used by CI.
	gocv.Rectangle(&screen, image.Rect(138, 105, 183, 136), color.RGBA{R: 220, G: 150, B: 20, A: 255}, -1)

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

func TestUIAnchorCircuitBreakerFallsBackSafely(t *testing.T) {
	b := &Bot{
		uiAnchors: map[string]image.Point{"Attack": image.Pt(160, 120)},
	}
	// Pretend this runtime has already accumulated a poor verified-anchor
	// history. The breaker must disable only the cache; callers still invoke
	// the full locator immediately after this function returns false.
	b.uiAnchorAttempts.Store(19)
	b.uiAnchorHits.Store(5)

	screen := gocv.NewMatWithSize(220, 320, gocv.MatTypeCV8UC3)
	defer screen.Close()

	// No matching orange blob near the remembered anchor -> 20th miss path.
	if _, _, ok := b.locateRememberedButton("Attack", screen); ok {
		t.Fatal("empty image must not produce a cached anchor hit")
	}
	if b.uiAnchorDisabledUntilUS.Load() <= time.Now().UnixMicro() {
		t.Fatal("poor anchor hit-rate should temporarily disable the cache")
	}

	attempts := b.uiAnchorAttempts.Load()
	if _, _, ok := b.locateRememberedButton("Attack", screen); ok {
		t.Fatal("disabled cache must not return a hit")
	}
	if b.uiAnchorAttempts.Load() != attempts {
		t.Fatal("disabled cache should bypass local probes without inflating attempts")
	}
}
