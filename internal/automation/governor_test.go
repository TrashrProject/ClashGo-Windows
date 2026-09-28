package automation

import (
	"testing"
	"time"

	"github.com/Ducky705/ClashGO/internal/config"
)

func TestGovernorScheduledBreakOnlyOncePerBoundary(t *testing.T) {
	g := NewGovernor(config.AutomationConfig{
		BreakEveryAttacks: 3,
		BreakDuration: config.Duration{Duration: time.Minute},
	})
	now := time.Unix(1000, 0)

	first := g.Gate(now, 3, 0)
	if first.Reason != "scheduled_break" || first.Wait != time.Minute {
		t.Fatalf("unexpected first gate: %+v", first)
	}
	second := g.Gate(now, 3, 0)
	if second.Wait != 0 {
		t.Fatalf("same attack boundary must not break twice: %+v", second)
	}
}

func TestGovernorHourlyAttackLimit(t *testing.T) {
	g := NewGovernor(config.AutomationConfig{MaxAttacksPerHour: 2})
	base := time.Now()
	g.RecordAttack(base.Add(-40 * time.Minute))
	g.RecordAttack(base.Add(-10 * time.Minute))

	gate := g.Gate(base, 2, 0)
	if gate.Reason != "hourly_attack_limit" {
		t.Fatalf("expected hourly limit, got %+v", gate)
	}
	if gate.Wait <= 19*time.Minute || gate.Wait > 21*time.Minute {
		t.Fatalf("expected about 20m wait, got %s", gate.Wait)
	}
}

func TestGovernorRecoveryCircuitBreaker(t *testing.T) {
	g := NewGovernor(config.AutomationConfig{
		RecoveryPauseThreshold: 2,
		RecoveryPause: config.Duration{Duration: 4 * time.Minute},
	})

	gate := g.Gate(time.Now(), 0, 2)
	if gate.Reason != "recovery_circuit_breaker" || gate.Wait != 4*time.Minute {
		t.Fatalf("unexpected recovery gate: %+v", gate)
	}
	if again := g.Gate(time.Now(), 0, 2); again.Wait != 0 {
		t.Fatalf("same recovery incidents must not trigger twice: %+v", again)
	}
}
