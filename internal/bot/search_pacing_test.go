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

func TestSafePacingActiveExpiresCleanly(t *testing.T) {
	const now = int64(1_000_000)
	if !safePacingActive(now+1, now) {
		t.Fatal("future safety window should be active")
	}
	if safePacingActive(now, now) {
		t.Fatal("safety window must expire exactly at deadline")
	}
	if safePacingActive(now-1, now) {
		t.Fatal("expired safety window must be inactive")
	}
}

func TestNextVerificationPacingStartsConservative(t *testing.T) {
	p := chooseNextVerificationPacing(adb.Health{FastCaptureMs: 350}, 5, 100)
	if p.InitialWait != 650*time.Millisecond || p.ProbeGap != 550*time.Millisecond || p.Mode != "Safe" {
		t.Fatalf("small sample must keep proven timings: %+v", p)
	}
}

func TestNextVerificationPacingUnlocksBalancedAfterReliableSample(t *testing.T) {
	p := chooseNextVerificationPacing(adb.Health{FastCaptureMs: 600}, 12, 97)
	if p.InitialWait != 615*time.Millisecond || p.ProbeGap != 515*time.Millisecond || p.Mode != "Balanced" {
		t.Fatalf("unexpected balanced Next pacing: %+v", p)
	}
}

func TestNextVerificationPacingUnlocksFastOnlyWithStrongEvidence(t *testing.T) {
	p := chooseNextVerificationPacing(adb.Health{FastCaptureMs: 420}, 25, 99)
	if p.InitialWait != 575*time.Millisecond || p.ProbeGap != 475*time.Millisecond || p.Mode != "Fast" {
		t.Fatalf("unexpected fast Next pacing: %+v", p)
	}
}

func TestNextVerificationPacingFailsClosedOnADBPressure(t *testing.T) {
	p := chooseNextVerificationPacing(adb.Health{FastCaptureMs: 1100}, 100, 100)
	if p.Mode != "Safe" || p.InitialWait != 650*time.Millisecond {
		t.Fatalf("ADB pressure must restore proven Next timings: %+v", p)
	}
}
