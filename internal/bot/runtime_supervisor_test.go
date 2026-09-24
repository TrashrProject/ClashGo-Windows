package bot

import (
	"testing"
	"time"

	"github.com/Ducky705/ClashGO/internal/game"
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

func TestRuntimeStateTimeouts(t *testing.T) {
	tests := []struct {
		state game.GameState
		min   time.Duration
		max   time.Duration
	}{
		{game.StateMainVillage, 0, 0},
		{game.StateConnectionLost, 10 * time.Second, 20 * time.Second},
		{game.StateConfirmExit, 10 * time.Second, 20 * time.Second},
		{game.StateTapToContinue, 15 * time.Second, 30 * time.Second},
		{game.StateUnknown, 20 * time.Second, 35 * time.Second},
		{game.StateSearchMap, 30 * time.Second, 50 * time.Second},
		{game.StateLogo, 2 * time.Minute, 4 * time.Minute},
	}

	for _, tc := range tests {
		got := runtimeStateTimeout(tc.state)
		if tc.min == 0 && tc.max == 0 {
			if got != 0 {
				t.Fatalf("%s should be unbounded, got %s", tc.state, got)
			}
			continue
		}
		if got < tc.min || got > tc.max {
			t.Fatalf("%s timeout=%s, want between %s and %s", tc.state, got, tc.min, tc.max)
		}
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


func TestAutomationTaskTimeoutBudgets(t *testing.T) {
	tests := []struct {
		name string
		min  time.Duration
		max  time.Duration
	}{
		{"resource scan", 10 * time.Second, 30 * time.Second},
		{"donation", 45 * time.Second, 90 * time.Second},
		{"army check", 60 * time.Second, 2 * time.Minute},
		{"wall upgrades", 90 * time.Second, 3 * time.Minute},
		{"attack", 8 * time.Minute, 15 * time.Minute},
	}
	for _, tc := range tests {
		got := automationTaskTimeout(tc.name)
		if got < tc.min || got > tc.max {
			t.Fatalf("%s timeout=%s want between %s and %s", tc.name, got, tc.min, tc.max)
		}
	}
}
