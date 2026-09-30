package bot

import (
	"testing"
	"time"

	"github.com/Ducky705/ClashGO/internal/adb"
	"github.com/Ducky705/ClashGO/internal/game"
)

func TestRuntimeHealthScoresDegradePredictably(t *testing.T) {
	healthy := combineRuntimeHealth(100, 100, 100, 100, 100)
	if healthy.Overall != 100 || healthy.Mode != "healthy" {
		t.Fatalf("healthy=%+v", healthy)
	}
	badCapture := captureHealthScore(adb.Health{FastCaptureMs: 2600, ConsecutiveFails: 2})
	if badCapture >= 40 {
		t.Fatalf("bad capture score=%d want <40", badCapture)
	}
	badADB := adbHealthScore(adb.Health{ConsecutiveFails: 4})
	if badADB >= 40 {
		t.Fatalf("bad adb score=%d want <40", badADB)
	}
}

func TestUIHealthUnknownFallsWithAge(t *testing.T) {
	now := time.Now()
	fresh := uiHealthScore(game.StateUnknown, now.Add(-5*time.Second), now)
	old := uiHealthScore(game.StateUnknown, now.Add(-70*time.Second), now)
	if fresh <= old {
		t.Fatalf("fresh=%d old=%d; expected degradation", fresh, old)
	}
	if old > 40 {
		t.Fatalf("old unknown UI score=%d want <=40", old)
	}
}

func TestVisionHealthNeedsEvidenceBeforePenalty(t *testing.T) {
	if got := visionHealthScore(3, 0, false); got != 100 {
		t.Fatalf("small sample should stay neutral, got %d", got)
	}
	if got := visionHealthScore(100, 10, false); got >= 70 {
		t.Fatalf("poor anchor hit rate should degrade score, got %d", got)
	}
	if got := visionHealthScore(100, 90, false); got <= 90 {
		t.Fatalf("strong anchor hit rate should stay healthy, got %d", got)
	}
}
