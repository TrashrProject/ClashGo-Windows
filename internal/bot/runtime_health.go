package bot

import (
	"strings"
	"time"

	"github.com/Ducky705/ClashGO/internal/adb"
	"github.com/Ducky705/ClashGO/internal/game"
)

type RuntimeHealthSnapshot struct {
	Overall  int    `json:"overall"`
	ADB      int    `json:"adb"`
	Capture  int    `json:"capture"`
	Vision   int    `json:"vision"`
	UI       int    `json:"ui"`
	Recovery int    `json:"recovery"`
	Mode     string `json:"mode"`
}

type FeatureCircuitSnapshot struct {
	CollectorsOpen       bool  `json:"collectors_open"`
	CollectorsFailures   int32 `json:"collectors_failures"`
	CollectorsRetryUnix  int64 `json:"collectors_retry_unix"`
	WallsOpen            bool  `json:"walls_open"`
	WallsFailures        int32 `json:"walls_failures"`
	WallsRetryUnix       int64 `json:"walls_retry_unix"`
}

func clampHealth(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func captureHealthScore(h adb.Health) int {
	ms := h.FastCaptureMs
	if ms <= 0 {
		ms = h.AvgCaptureMs
	}
	score := 100
	switch {
	case ms >= 2500:
		score -= 70
	case ms >= 1500:
		score -= 50
	case ms >= 1000:
		score -= 30
	case ms >= 700:
		score -= 15
	case ms >= 500:
		score -= 5
	}
	score -= h.ConsecutiveFails * 12
	return clampHealth(score)
}

func adbHealthScore(h adb.Health) int {
	score := 100 - h.ConsecutiveFails*18
	if h.CapturesTotal > 20 {
		errorRate := float64(h.ErrorsTotal) / float64(h.CapturesTotal+h.ErrorsTotal)
		switch {
		case errorRate >= 0.20:
			score -= 40
		case errorRate >= 0.10:
			score -= 25
		case errorRate >= 0.03:
			score -= 10
		}
	}
	if strings.TrimSpace(h.LastError) != "" {
		score -= 5
	}
	return clampHealth(score)
}

func visionHealthScore(attempts, hits int64, disabled bool) int {
	if disabled {
		return 45
	}
	if attempts < 10 {
		return 100
	}
	rate := float64(hits) / float64(attempts)
	return clampHealth(55 + int(rate*45))
}

func uiHealthScore(state game.GameState, stateSince time.Time, now time.Time) int {
	if now.IsZero() {
		now = time.Now()
	}
	if stateSince.IsZero() {
		return 100
	}
	age := now.Sub(stateSince)
	score := 100
	if state == game.StateUnknown {
		switch {
		case age >= 90*time.Second:
			score = 15
		case age >= 60*time.Second:
			score = 35
		case age >= 30*time.Second:
			score = 60
		case age >= 15*time.Second:
			score = 80
		}
		return score
	}
	timeout := runtimeStateTimeout(state)
	if timeout > 0 {
		ratio := float64(age) / float64(timeout)
		switch {
		case ratio >= 1:
			score = 20
		case ratio >= 0.8:
			score = 45
		case ratio >= 0.6:
			score = 70
		}
	}
	return score
}

func recoveryHealthScore(attempts, successes, restarts int32) int {
	score := 100
	if attempts > 0 {
		failed := attempts - successes
		if failed < 0 {
			failed = 0
		}
		score -= int(failed) * 15
		successRate := float64(successes) / float64(attempts)
		if attempts >= 3 && successRate < 0.5 {
			score -= 20
		}
	}
	score -= int(restarts) * 3
	return clampHealth(score)
}

func combineRuntimeHealth(adbScore, capture, vision, ui, recovery int) RuntimeHealthSnapshot {
	// Transport/capture dominate because every automation action depends on
	// them. Vision/UI and recovery still influence the final gate.
	overall := (adbScore*25 + capture*25 + vision*18 + ui*17 + recovery*15) / 100
	mode := "healthy"
	switch {
	case overall < 20:
		mode = "critical"
	case overall < 40:
		mode = "hold"
	case overall < 60:
		mode = "safe"
	case overall < 80:
		mode = "degraded"
	}
	return RuntimeHealthSnapshot{
		Overall: clampHealth(overall),
		ADB: clampHealth(adbScore),
		Capture: clampHealth(capture),
		Vision: clampHealth(vision),
		UI: clampHealth(ui),
		Recovery: clampHealth(recovery),
		Mode: mode,
	}
}

func (b *Bot) RuntimeHealth() RuntimeHealthSnapshot {
	if b == nil || b.client == nil {
		return RuntimeHealthSnapshot{Mode: "unavailable"}
	}
	h := b.client.Health()
	attempts := b.uiAnchorAttempts.Load()
	hits := b.uiAnchorHits.Load()
	disabled := b.uiAnchorDisabledUntilUS.Load() > time.Now().UnixMicro()
	state := game.GameState(b.runtimeState.Load())
	sinceNS := b.runtimeStateSince.Load()
	var since time.Time
	if sinceNS > 0 {
		since = time.Unix(0, sinceNS)
	}
	return combineRuntimeHealth(
		adbHealthScore(h),
		captureHealthScore(h),
		visionHealthScore(attempts, hits, disabled),
		uiHealthScore(state, since, time.Now()),
		recoveryHealthScore(b.recoveryAttempts.Load(), b.recoverySuccesses.Load(), b.blueStacksRestarts.Load()),
	)
}

func (b *Bot) healthAttackHoldActive() bool {
	return b != nil && b.healthHoldUntilUS.Load() > time.Now().UnixMicro()
}

func (b *Bot) FeatureCircuits() FeatureCircuitSnapshot {
	if b == nil {
		return FeatureCircuitSnapshot{}
	}
	now := time.Now().UnixMicro()
	collectorUntil := b.collectorDisabledUntilUS.Load()
	wallUntil := b.wallDisabledUntilUS.Load()
	return FeatureCircuitSnapshot{
		CollectorsOpen: collectorUntil > now,
		CollectorsFailures: b.collectorFailureStreak.Load(),
		CollectorsRetryUnix: collectorUntil / int64(time.Second/time.Microsecond),
		WallsOpen: wallUntil > now,
		WallsFailures: b.wallFailureStreak.Load(),
		WallsRetryUnix: wallUntil / int64(time.Second/time.Microsecond),
	}
}

func (b *Bot) collectorCircuitOpen() bool {
	return b != nil && b.collectorDisabledUntilUS.Load() > time.Now().UnixMicro()
}

func (b *Bot) recordCollectorVerification(increased, comparable bool) {
	if b == nil || !comparable {
		return
	}
	if increased {
		b.collectorFailureStreak.Store(0)
		return
	}
	streak := b.collectorFailureStreak.Add(1)
	if streak >= 3 {
		until := time.Now().Add(30 * time.Minute)
		b.collectorDisabledUntilUS.Store(until.UnixMicro())
		b.collectorFailureStreak.Store(0)
		b.logger.Warn().
			Time("retry_at", until).
			Msg("collector circuit breaker opened after repeated verified no-gain taps")
	}
}

func (b *Bot) wallCircuitOpen() bool {
	return b != nil && b.wallDisabledUntilUS.Load() > time.Now().UnixMicro()
}

func (b *Bot) runWallUpgradeWithCircuit(gc *game.GameContext) bool {
	if b == nil {
		return false
	}
	if b.wallCircuitOpen() {
		b.logger.Debug().Msg("wall upgrade circuit breaker open; skipping optional wall stage")
		// A disabled optional feature must never block farming/account rotation.
		return true
	}
	ok := b.UpgradeWalls(gc)
	if b.ctx.Err() != nil {
		return ok
	}
	if ok {
		b.wallFailureStreak.Store(0)
		return true
	}
	streak := b.wallFailureStreak.Add(1)
	if streak >= 3 {
		until := time.Now().Add(45 * time.Minute)
		b.wallDisabledUntilUS.Store(until.UnixMicro())
		b.wallFailureStreak.Store(0)
		b.logger.Warn().
			Time("retry_at", until).
			Msg("wall-upgrade circuit breaker opened after repeated failed stages; farming will continue")
		return true
	}
	return false
}

// applyRuntimeHealthPolicy converts the passive health score into bounded,
// reversible protection. It never interrupts an active attack.
func (b *Bot) applyRuntimeHealthPolicy(now time.Time) {
	if b == nil || b.ctx.Err() != nil {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	h := b.RuntimeHealth()
	if h.Overall < 60 {
		b.forceSafePacing("runtime_health_"+h.Mode, 2*time.Minute)
	}
	if h.Overall < 40 && !b.seqRunning.Load() {
		until := now.Add(30 * time.Second).UnixMicro()
		if until > b.healthHoldUntilUS.Load() {
			b.healthHoldUntilUS.Store(until)
		}
	}

	if h.Overall >= 25 {
		b.healthCriticalStreak.Store(0)
		return
	}
	if b.seqRunning.Load() || b.recoveryInFlight.Load() || b.restartInFlight.Load() {
		return
	}
	if b.healthCriticalStreak.Add(1) < 2 {
		return
	}
	b.healthCriticalStreak.Store(0)

	b.healthPolicyMu.Lock()
	if b.healthRestartWindowStart.IsZero() || now.Sub(b.healthRestartWindowStart) > 10*time.Minute {
		b.healthRestartWindowStart = now
		b.healthRestartsInWindow = 0
	}
	b.healthRestartsInWindow++
	restarts := b.healthRestartsInWindow
	b.healthPolicyMu.Unlock()

	if restarts >= 3 {
		b.logger.Error().
			Int("health", h.Overall).
			Msg("runtime health remained critical after repeated restarts; stopping session safely")
		b.cancel()
		return
	}

	b.logger.Warn().
		Int("health", h.Overall).
		Str("mode", h.Mode).
		Msg("runtime health critical; restarting Clash before another attack")
	go b.restartGame()
}
