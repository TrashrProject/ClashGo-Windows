package bot

import (
	"testing"
	"time"
)

func TestRuntimeSupervisorThresholdsAreSane(t *testing.T) {
	if runtimeSupervisorTick <= 0 {
		t.Fatal("supervisor tick must be positive")
	}
	if runtimeCaptureStaleThreshold <= runtimeSupervisorTick {
		t.Fatalf("stale threshold (%s) must exceed tick (%s)", runtimeCaptureStaleThreshold, runtimeSupervisorTick)
	}
	if runtimeCaptureStaleThreshold < 40*time.Second {
		t.Fatalf("stale threshold too aggressive: %s", runtimeCaptureStaleThreshold)
	}
}

func TestCaptureHeartbeatTimestampRoundTrip(t *testing.T) {
	now := time.Now()
	n := now.UnixNano()
	roundTrip := time.Unix(0, n)
	if delta := roundTrip.Sub(now); delta != 0 {
		t.Fatalf("heartbeat timestamp lost precision: %s", delta)
	}
}
