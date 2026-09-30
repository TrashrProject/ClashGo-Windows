package intelligence

import "testing"

func TestEvaluateTargetThresholds(t *testing.T) {
	rules := TargetRules{MinGold: 750000, MinElixir: 750000, MinDarkElixir: 2000, SearchEnabled: true}
	d := EvaluateTarget(Target{Gold: 800000, Elixir: 900000, DarkElixir: 3000}, rules)
	if !d.Accept || d.Score <= 0 {
		t.Fatalf("expected accepted target, got %+v", d)
	}
}

func TestEvaluateTargetDarkOverride(t *testing.T) {
	rules := TargetRules{MinGold: 750000, MinElixir: 750000, MinDarkElixir: 2000, DarkOverride: 10000, SearchEnabled: true}
	d := EvaluateTarget(Target{Gold: 100000, Elixir: 100000, DarkElixir: 12000}, rules)
	if !d.Accept || d.Reason != "dark elixir override met" {
		t.Fatalf("expected DE override, got %+v", d)
	}
}

func TestEvaluateTargetRejectsLowLoot(t *testing.T) {
	rules := TargetRules{MinGold: 750000, MinElixir: 750000, MinDarkElixir: 2000, SearchEnabled: true}
	d := EvaluateTarget(Target{Gold: 100000, Elixir: 100000, DarkElixir: 100}, rules)
	if d.Accept {
		t.Fatalf("expected rejection, got %+v", d)
	}
}
