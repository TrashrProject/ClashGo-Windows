package bot

import (
	"testing"
	"time"

	"github.com/Ducky705/ClashGO/internal/adb"
)

func TestChooseSearchPacingHealthy(t *testing.T) {
	p := chooseSearchPacing(adb.Health{AvgCaptureMs: 350})
	if p.PostTransitionPause != 750*time.Millisecond || p.StabilityRestEvery != 10 || p.StabilityRest != 850*time.Millisecond {
		t.Fatalf("unexpected healthy pacing: %+v", p)
	}
}

func TestChooseSearchPacingDegraded(t *testing.T) {
	p := chooseSearchPacing(adb.Health{AvgCaptureMs: 1000})
	if p.PostTransitionPause != 1100*time.Millisecond || p.StabilityRestEvery != 8 || p.StabilityRest != 1500*time.Millisecond {
		t.Fatalf("unexpected degraded pacing: %+v", p)
	}
}

func TestChooseSearchPacingFailureForcesSafeMode(t *testing.T) {
	p := chooseSearchPacing(adb.Health{AvgCaptureMs: 200, ConsecutiveFails: 1})
	if p.PostTransitionPause != 1100*time.Millisecond || p.StabilityRestEvery != 8 {
		t.Fatalf("transport failure must force conservative pacing: %+v", p)
	}
}
