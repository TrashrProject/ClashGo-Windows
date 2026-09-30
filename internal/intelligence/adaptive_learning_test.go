package intelligence

import (
	"path/filepath"
	"testing"
)

func TestRewardPenalizesRecoveryAndRestart(t *testing.T) {
	good := RewardForOutcome(LearningOutcome{
		Clean: true, DeploySuccess: true, ReturnHomeSuccess: true,
		SafeDeployment: true, ParsedResults: true, PlanningMS: 2500, DeployMS: 9000,
	})
	bad := RewardForOutcome(LearningOutcome{
		DeploySuccess: false, ReturnHomeSuccess: false,
		RecoveryCount: 1, BlueStacksRestart: 1, PlanningMS: 8000,
	})
	if good <= bad {
		t.Fatalf("good reward %.1f <= bad reward %.1f", good, bad)
	}
	if bad >= 0 {
		t.Fatalf("bad reward %.1f should be negative", bad)
	}
}

func TestShadowLearnsButDoesNotApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "adaptive_learning.json")
	e, err := NewAdaptiveEngine(path, EnvironmentFingerprint{
		OS: "windows", Emulator: "bluestacks", Width: 860, Height: 732, DPI: 160,
		Strategy: "edrag",
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		_, err = e.Observe(LearningOutcome{
			Domain: "attack",
			Parameters: map[string]float64{"card_settle_ms": 125},
			Clean: true, DeploySuccess: true, ReturnHomeSuccess: true,
			SafeDeployment: true, ParsedResults: true, PlanningMS: 2500,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	s := e.Suggest("attack", "card_settle_ms", 150)
	if s.Apply {
		t.Fatalf("shadow suggestion unexpectedly applies: %+v", s)
	}
	if s.Proposed != 135 {
		t.Fatalf("proposed %.0f, want bounded step 135", s.Proposed)
	}
}

func TestLearningRollbackAfterTwoBadTrials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "adaptive_learning.json")
	e, err := NewAdaptiveEngine(path, EnvironmentFingerprint{OS: "windows", Width: 860, Height: 732})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetMode(LearningAdaptive); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		_, err = e.Observe(LearningOutcome{
			Domain: "attack",
			Parameters: map[string]float64{"planning_capture_ms": 200},
			DeploySuccess: false,
			RecoveryCount: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := e.Mode(); got != LearningStable {
		t.Fatalf("mode=%q, want stable rollback", got)
	}
	if snap := e.Snapshot(); snap.Rollbacks != 1 {
		t.Fatalf("rollbacks=%d, want 1", snap.Rollbacks)
	}
}

func TestSuggestionNeverExceedsSafeStep(t *testing.T) {
	path := filepath.Join(t.TempDir(), "adaptive_learning.json")
	e, err := NewAdaptiveEngine(path, EnvironmentFingerprint{OS: "windows", Width: 860, Height: 732})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		_, _ = e.Observe(LearningOutcome{
			Domain: "attack",
			Parameters: map[string]float64{"search_capture_ms": 450},
			Clean: true, DeploySuccess: true, ReturnHomeSuccess: true,
			SafeDeployment: true, ParsedResults: true,
		})
	}
	if err := e.SetMode(LearningAdaptive); err != nil {
		t.Fatal(err)
	}
	s := e.Suggest("attack", "search_capture_ms", 700)
	if !s.Apply {
		t.Fatalf("expected applicable learning suggestion: %+v", s)
	}
	if s.Proposed != 625 {
		t.Fatalf("proposed %.0f, want max-step-limited 625", s.Proposed)
	}
}
