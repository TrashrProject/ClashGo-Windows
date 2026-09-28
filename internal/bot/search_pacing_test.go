package bot

import (
	"testing"
	"time"

	"github.com/Ducky705/ClashGO/internal/adb"
)

func TestChooseSearchPacingHealthy(t *testing.T) {
	p := chooseSearchPacing(adb.Health{AvgCaptureMs: 350})
	if p.Mode != "Fast" || p.PostTransitionPause != 750*time.Millisecond || p.StabilityRestEvery != 10 || p.StabilityRest != 850*time.Millisecond ||
		p.PrepSettlePause != 350*time.Millisecond || p.PrepRetryPause != 400*time.Millisecond || p.PrepPollPause != 80*time.Millisecond {
		t.Fatalf("unexpected healthy pacing: %+v", p)
	}
}

func TestChooseSearchPacingDegraded(t *testing.T) {
	p := chooseSearchPacing(adb.Health{AvgCaptureMs: 1000})
	if p.Mode != "Safe" || p.PostTransitionPause != 1100*time.Millisecond || p.StabilityRestEvery != 8 || p.StabilityRest != 1500*time.Millisecond ||
		p.PrepSettlePause != 650*time.Millisecond || p.PrepRetryPause != 650*time.Millisecond || p.PrepPollPause != 120*time.Millisecond {
		t.Fatalf("unexpected degraded pacing: %+v", p)
	}
}

func TestChooseSearchPacingFailureForcesSafeMode(t *testing.T) {
	p := chooseSearchPacing(adb.Health{AvgCaptureMs: 200, ConsecutiveFails: 1})
	if p.Mode != "Safe" || p.PostTransitionPause != 1100*time.Millisecond || p.StabilityRestEvery != 8 {
		t.Fatalf("transport failure must force conservative pacing: %+v", p)
	}
}

func TestChooseSearchPacingBalancedByDefault(t *testing.T) {
	p := chooseSearchPacing(adb.Health{})
	if p.Mode != "Balanced" || p.PostTransitionPause != 900*time.Millisecond ||
		p.PrepSettlePause != 500*time.Millisecond || p.PrepRetryPause != 500*time.Millisecond || p.PrepPollPause != 100*time.Millisecond {
		t.Fatalf("unexpected default pacing: %+v", p)
	}
}

func TestChooseSearchPacingUsesReactiveLatency(t *testing.T) {
	p := chooseSearchPacing(adb.Health{
		AvgCaptureMs: 300,
		FastCaptureMs: 1050,
	})
	if p.Mode != "Safe" {
		t.Fatalf("reactive slowdown should force Safe mode, got %+v", p)
	}
}

func TestChooseSearchPacingReactiveFastWinsOverSlowHistoricalAverage(t *testing.T) {
	p := chooseSearchPacing(adb.Health{
		AvgCaptureMs: 700,
		FastCaptureMs: 420,
	})
	if p.Mode != "Fast" {
		t.Fatalf("reactive recovery should permit Fast mode, got %+v", p)
	}
}

func TestSearchReliabilityKeepsFastOnStrongFirstPassRate(t *testing.T) {
	p := chooseSearchPacingWithReliability(adb.Health{FastCaptureMs: 350}, 12, 99)
	if p.Mode != "Fast" {
		t.Fatalf("strong Next reliability should keep Fast mode, got %+v", p)
	}
}

func TestSearchReliabilityDowngradesFastToBalanced(t *testing.T) {
	p := chooseSearchPacingWithReliability(adb.Health{FastCaptureMs: 350}, 12, 88)
	if p.Mode != "Balanced" {
		t.Fatalf("moderate ignored-Next rate should downgrade Fast to Balanced, got %+v", p)
	}
}

func TestSearchReliabilityForcesSafeOnPoorFirstPassRate(t *testing.T) {
	p := chooseSearchPacingWithReliability(adb.Health{FastCaptureMs: 350}, 12, 70)
	if p.Mode != "Safe" {
		t.Fatalf("poor Next reliability should force Safe mode, got %+v", p)
	}
}

func TestSearchReliabilityIgnoresTinySample(t *testing.T) {
	p := chooseSearchPacingWithReliability(adb.Health{FastCaptureMs: 350}, 3, 50)
	if p.Mode != "Fast" {
		t.Fatalf("tiny sample should not override ADB pacing, got %+v", p)
	}
}
