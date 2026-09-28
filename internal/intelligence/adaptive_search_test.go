package intelligence

import "testing"

func TestAdaptTargetRulesProgressivelyRelaxesWithinFloor(t *testing.T) {
	base := TargetRules{MinGold: 1000000, MinElixir: 800000, MinDarkElixir: 5000, DarkOverride: 9000, SearchEnabled: true}
	p := AdaptiveSearchPolicy{Enabled: true, StartAfterSkips: 8, StepEverySkips: 4, StepPercent: 5, FloorPercent: 70}

	got, pct := AdaptTargetRules(base, 7, p)
	if pct != 100 || got.MinGold != base.MinGold {
		t.Fatalf("rules changed before adaptive window: pct=%d got=%+v", pct, got)
	}

	got, pct = AdaptTargetRules(base, 8, p)
	if pct != 95 || got.MinGold != 950000 || got.MinElixir != 760000 || got.MinDarkElixir != 4750 {
		t.Fatalf("unexpected first adaptive step: pct=%d got=%+v", pct, got)
	}
	if got.DarkOverride != base.DarkOverride {
		t.Fatalf("explicit dark override changed: %d", got.DarkOverride)
	}

	got, pct = AdaptTargetRules(base, 200, p)
	if pct != 70 || got.MinGold != 700000 {
		t.Fatalf("floor not enforced: pct=%d got=%+v", pct, got)
	}
}

func TestAdaptTargetRulesDisabled(t *testing.T) {
	base := TargetRules{MinGold: 750000, MinElixir: 750000, MinDarkElixir: 2000}
	got, pct := AdaptTargetRules(base, 99, AdaptiveSearchPolicy{})
	if pct != 100 || got != base {
		t.Fatalf("disabled policy changed rules: pct=%d got=%+v", pct, got)
	}
}
