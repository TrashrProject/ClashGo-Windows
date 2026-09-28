package automation

import (
	"context"
	"sync"
	"time"

	"github.com/Ducky705/ClashGO/internal/config"
)

// Governor turns the automation settings into bounded, observable pacing.
// It is intentionally independent from game-state detection: callers ask it
// for permission immediately before starting a new attack sequence.
type Governor struct {
	cfg config.AutomationConfig

	mu              sync.Mutex
	attackTimes     []time.Time
	lastBreakAttack int32
	recoveryAnchor  int32
}

func NewGovernor(cfg config.AutomationConfig) *Governor {
	return &Governor{cfg: cfg}
}

type Gate struct {
	Wait   time.Duration
	Reason string
}

func (g *Governor) Gate(now time.Time, attacksCompleted, recoveryAttempts int32) Gate {
	g.mu.Lock()
	defer g.mu.Unlock()

	if now.IsZero() {
		now = time.Now()
	}

	// Drop timestamps that no longer belong to the rolling one-hour window.
	cutoff := now.Add(-time.Hour)
	keep := g.attackTimes[:0]
	for _, at := range g.attackTimes {
		if at.After(cutoff) {
			keep = append(keep, at)
		}
	}
	g.attackTimes = keep

	if g.cfg.MaxAttacksPerHour > 0 && len(g.attackTimes) >= g.cfg.MaxAttacksPerHour {
		oldest := g.attackTimes[0]
		wait := oldest.Add(time.Hour).Sub(now)
		if wait < 0 {
			wait = 0
		}
		return Gate{Wait: wait, Reason: "hourly_attack_limit"}
	}

	if g.cfg.BreakEveryAttacks > 0 && attacksCompleted > 0 &&
		attacksCompleted%int32(g.cfg.BreakEveryAttacks) == 0 &&
		g.lastBreakAttack != attacksCompleted {
		g.lastBreakAttack = attacksCompleted
		wait := g.cfg.BreakDuration.Duration
		if wait <= 0 {
			wait = 2 * time.Minute
		}
		return Gate{Wait: wait, Reason: "scheduled_break"}
	}

	if g.cfg.RecoveryPauseThreshold > 0 {
		delta := recoveryAttempts - g.recoveryAnchor
		if delta >= int32(g.cfg.RecoveryPauseThreshold) {
			g.recoveryAnchor = recoveryAttempts
			wait := g.cfg.RecoveryPause.Duration
			if wait <= 0 {
				wait = 5 * time.Minute
			}
			return Gate{Wait: wait, Reason: "recovery_circuit_breaker"}
		}
	}

	return Gate{}
}

func (g *Governor) RecordAttack(at time.Time) {
	if at.IsZero() {
		at = time.Now()
	}
	g.mu.Lock()
	g.attackTimes = append(g.attackTimes, at)
	g.mu.Unlock()
}

// Wait blocks until the governor allows a new attack or ctx is cancelled.
func (g *Governor) Wait(ctx context.Context, now time.Time, attacksCompleted, recoveryAttempts int32) (Gate, bool) {
	gate := g.Gate(now, attacksCompleted, recoveryAttempts)
	if gate.Wait <= 0 {
		return gate, true
	}

	timer := time.NewTimer(gate.Wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return gate, false
	case <-timer.C:
		return gate, true
	}
}
