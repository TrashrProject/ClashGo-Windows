package bot

import (
	"time"

	"github.com/Ducky705/ClashGO/internal/adb"
)

// searchPacing controls only non-critical matchmaking idle time. It never
// changes troop-card selection delays, red-zone geometry or deployment tap
// cadence. The profile automatically falls back to conservative timings when
// BlueStacks/ADB shows pressure.
type searchPacing struct {
	Mode                string
	PostTransitionPause time.Duration
	StabilityRestEvery  int
	StabilityRest       time.Duration

	// Preparation pacing affects only menu/UI settling before matchmaking.
	// It never changes battle deployment timing.
	PrepSettlePause     time.Duration
	PrepRetryPause      time.Duration
}

func chooseSearchPacing(h adb.Health) searchPacing {
	// Use the reactive EWMA when available so a sudden BlueStacks slowdown
	// changes pacing within a few captures. Fall back to the stable average
	// for old persisted health snapshots / tests.
	captureMs := h.FastCaptureMs
	if captureMs <= 0 {
		captureMs = h.AvgCaptureMs
	}

	// Any active transport instability keeps the battle-tested conservative
	// behavior.
	if h.ConsecutiveFails > 0 || captureMs >= 900 {
		return searchPacing{
			Mode:                "Safe",
			PostTransitionPause: 1100 * time.Millisecond,
			StabilityRestEvery:  8,
			StabilityRest:       1500 * time.Millisecond,
			PrepSettlePause:     650 * time.Millisecond,
			PrepRetryPause:      650 * time.Millisecond,
		}
	}

	// Fast path for a healthy BlueStacks session. We deliberately keep the
	// 650ms immediate post-Next settle and verification sleeps elsewhere;
	// only the redundant pause after a confirmed transition is shortened.
	if captureMs > 0 && captureMs <= 500 {
		return searchPacing{
			Mode:                "Fast",
			PostTransitionPause: 750 * time.Millisecond,
			StabilityRestEvery:  10,
			StabilityRest:       850 * time.Millisecond,
			PrepSettlePause:     350 * time.Millisecond,
			PrepRetryPause:      400 * time.Millisecond,
		}
	}

	// Unknown/normal health uses a modest improvement, not an aggressive one.
	return searchPacing{
		Mode:                "Balanced",
		PostTransitionPause: 900 * time.Millisecond,
		StabilityRestEvery:  9,
		StabilityRest:       1100 * time.Millisecond,
		PrepSettlePause:     500 * time.Millisecond,
		PrepRetryPause:      500 * time.Millisecond,
	}
}
