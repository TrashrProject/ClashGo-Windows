package config

import (
	"testing"
	"time"
)

func TestDefaultAutomationGovernorPolicy(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Automation.MaxAttacksPerHour != 12 {
		t.Fatalf("MaxAttacksPerHour=%d want 12", cfg.Automation.MaxAttacksPerHour)
	}
	if cfg.Automation.BreakEveryAttacks != 5 {
		t.Fatalf("BreakEveryAttacks=%d want 5", cfg.Automation.BreakEveryAttacks)
	}
	if cfg.Automation.BreakDuration.Duration != 3*time.Minute {
		t.Fatalf("BreakDuration=%s want 3m", cfg.Automation.BreakDuration.Duration)
	}
	if cfg.Automation.RecoveryPauseThreshold != 3 {
		t.Fatalf("RecoveryPauseThreshold=%d want 3", cfg.Automation.RecoveryPauseThreshold)
	}
	if cfg.Automation.RecoveryPause.Duration != 5*time.Minute {
		t.Fatalf("RecoveryPause=%s want 5m", cfg.Automation.RecoveryPause.Duration)
	}
}
