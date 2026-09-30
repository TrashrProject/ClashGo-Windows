package bot

import (
	"time"

	"github.com/Ducky705/ClashGO/internal/adb"
)

// searchPacing controls only non-critical matchmaking idle time. It never
// changes troop-card selection delays, red-zone geometry or deployment tap
// cadence. The profile automatically falls back to conservative timings when
// BlueStacks/ADB shows pressure.
type nextVerificationPacing struct {
	InitialWait time.Duration
	ProbeGap    time.Duration
	Mode        string
}

// chooseNextVerificationPacing only shortens the time BEFORE checking whether
// Clash accepted a verified Next tap. It never adds extra taps and never
// changes the retry path. The fast timings unlock only after enough successful
// transitions prove the current BlueStacks/ADB session is stable.
func chooseNextVerificationPacing(h adb.Health, transitions int64, firstPassRate float64) nextVerificationPacing {
	captureMs := h.FastCaptureMs
	if captureMs <= 0 {
		captureMs = h.AvgCaptureMs
	}

	safe := nextVerificationPacing{
		InitialWait: 650 * time.Millisecond,
		ProbeGap:    550 * time.Millisecond,
		Mode:        "Safe",
	}
	if h.ConsecutiveFails > 0 || captureMs >= 900 {
		return safe
	}

	if transitions >= 20 && firstPassRate >= 98 && captureMs > 0 && captureMs <= 500 {
		return nextVerificationPacing{
			InitialWait: 575 * time.Millisecond,
			ProbeGap:    475 * time.Millisecond,
			Mode:        "Fast",
		}
	}

	if transitions >= 10 && firstPassRate >= 95 && captureMs > 0 && captureMs <= 700 {
		return nextVerificationPacing{
			InitialWait: 615 * time.Millisecond,
			ProbeGap:    515 * time.Millisecond,
			Mode:        "Balanced",
		}
	}

	return safe
}

type searchPacing struct {
	Mode                string
	PostTransitionPause time.Duration
	StabilityRestEvery  int
	StabilityRest       time.Duration

	// Preparation pacing affects only menu/UI settling before matchmaking.
	// It never changes battle deployment timing.
	PrepSettlePause     time.Duration
	PrepRetryPause      time.Duration
	PrepPollPause       time.Duration
}


func chooseSearchPacingWithReliability(h adb.Health, nextTransitions int64, firstPassRate float64) searchPacing {
	p := chooseSearchPacing(h)
	// Do not react to tiny samples. Five confirmed transitions is enough to
	// detect a bad streak without bouncing modes on one ignored tap.
	if nextTransitions < 5 {
		return p
	}

	if firstPassRate < 80 {
		// Multiple ignored Next taps are a stronger instability signal than raw
		// capture latency. Preserve the proven conservative timings.
		return chooseSearchPacing(adb.Health{ConsecutiveFails: 1})
	}
	if firstPassRate < 92 && p.Mode == "Fast" {
		// A healthy ADB pipe can still outrun Clash's UI. Step down one level
		// instead of forcing Safe immediately.
		return chooseSearchPacing(adb.Health{})
	}
	return p
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
			PrepPollPause:       120 * time.Millisecond,
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
			PrepPollPause:       80 * time.Millisecond,
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
		PrepPollPause:       100 * time.Millisecond,
	}
}
