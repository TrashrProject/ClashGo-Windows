package intelligence

import (
	"path/filepath"
	"testing"
	"time"
)

func testContext() AttackContext {
	return AttackContext{
		Strategy: "valk_spam",
		TownHall: 17,
		TargetScore: 100,
		TargetGold: 800000,
		TargetElixir: 800000,
		TargetDE: 8000,
	}
}

func testOutcome(edge string, stars, destruction int) ContextualOutcome {
	return ContextualOutcome{
		Context: testContext(),
		Edge: edge,
		Stars: stars,
		DestructionPct: destruction,
		GoldStolen: 700000,
		ElixirStolen: 700000,
		DarkElixirStolen: 6000,
		CycleDurationMS: 90000,
		FullRoutineDurationMS: 105000,
		SearchDurationMS: 15000,
		SearchSkips: 2,
		BattleDurationMS: 90000,
		DeploySuccess: true,
		ReturnHomeSuccess: true,
		SafeDeployment: true,
		ParsedResults: true,
		At: time.Now(),
	}
}

func TestContextualRewardOptimizesFarmThroughputNotStars(t *testing.T) {
	fastLoot := testOutcome("TopLeft", 1, 55)
	fastLoot.GoldStolen = 900000
	fastLoot.ElixirStolen = 900000
	fastLoot.DarkElixirStolen = 8000
	fastLoot.FullRoutineDurationMS = 75000

	slowStars := testOutcome("TopLeft", 3, 100)
	slowStars.GoldStolen = 450000
	slowStars.ElixirStolen = 450000
	slowStars.DarkElixirStolen = 3000
	slowStars.FullRoutineDurationMS = 180000

	if RewardForContextualOutcome(fastLoot) <= RewardForContextualOutcome(slowStars) {
		t.Fatalf("faster higher-loot 1-star farm must beat slower lower-loot 3-star attack")
	}

	crash := fastLoot
	crash.BlueStacksRestart = 1
	if RewardForContextualOutcome(crash) >= RewardForContextualOutcome(slowStars) {
		t.Fatalf("BlueStacks restart must heavily penalize otherwise excellent farm throughput")
	}
}

func TestFarmResourcesPerHourRewardsFasterCycle(t *testing.T) {
	fast := testOutcome("TopLeft", 0, 0)
	slow := fast
	fast.FullRoutineDurationMS = 60000
	slow.FullRoutineDurationMS = 120000

	if FarmResourcesPerHour(fast) <= FarmResourcesPerHour(slow) {
		t.Fatalf("faster cycle must produce a higher farm rate")
	}
}

func TestContextualEngineLearnsChampion(t *testing.T) {
	e, err := NewContextualEngine("")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := e.Observe(testOutcome("TopRight", 3, 100)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if _, err := e.Observe(testOutcome("BottomLeft", 1, 52)); err != nil {
			t.Fatal(err)
		}
	}

	rec := e.RecommendEdge(testContext(), "Rotate",
		[]string{"TopLeft", "TopRight", "BottomLeft", "BottomRight"}, false)
	if !rec.Apply {
		t.Fatalf("expected champion recommendation, got %+v", rec)
	}
	if rec.Edge != "TopRight" {
		t.Fatalf("champion=%s want TopRight (%+v)", rec.Edge, rec)
	}
	if rec.Exploratory {
		t.Fatalf("exploration must be disabled")
	}
}

func TestContextualEngineFixedEdgeIsNeverOverridden(t *testing.T) {
	e, _ := NewContextualEngine("")
	for i := 0; i < 4; i++ {
		_, _ = e.Observe(testOutcome("TopRight", 3, 100))
	}
	rec := e.RecommendEdge(testContext(), "TopLeft",
		[]string{"TopLeft", "TopRight", "BottomLeft", "BottomRight"}, true)
	if rec.Apply {
		t.Fatalf("fixed edge must not be overridden: %+v", rec)
	}
}

func TestContextualEngineFamilyTransfer(t *testing.T) {
	e, _ := NewContextualEngine("")
	base := testContext()
	for i := 0; i < 3; i++ {
		o := testOutcome("BottomRight", 3, 95)
		o.Context = base
		_, _ = e.Observe(o)
	}

	similar := base
	similar.TargetGold += 1200000
	similar.TargetElixir += 1200000

	rec := e.RecommendEdge(similar, "Random",
		[]string{"TopLeft", "TopRight", "BottomLeft", "BottomRight"}, false)
	if !rec.Apply || rec.Edge != "BottomRight" {
		t.Fatalf("expected family-level transfer to BottomRight, got %+v", rec)
	}
	if rec.ProfileScope != "family" {
		t.Fatalf("scope=%q want family", rec.ProfileScope)
	}
}

func TestContextualEnginePersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contextual.json")
	e, err := NewContextualEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := e.Observe(testOutcome("TopRight", 3, 100)); err != nil {
			t.Fatal(err)
		}
	}
	if e.TotalSamples() != 3 {
		t.Fatalf("samples=%d want 3", e.TotalSamples())
	}

	reloaded, err := NewContextualEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.TotalSamples() != 3 {
		t.Fatalf("reloaded samples=%d want 3", reloaded.TotalSamples())
	}
	rec := reloaded.RecommendEdge(testContext(), "Rotate",
		[]string{"TopLeft", "TopRight", "BottomLeft", "BottomRight"}, false)
	if !rec.Apply || rec.Edge != "TopRight" {
		t.Fatalf("reloaded champion missing: %+v", rec)
	}
}

func TestAdaptiveColdStartAlwaysResolvesLegalEdge(t *testing.T) {
	e, _ := NewContextualEngine("")
	rec := e.RecommendEdge(testContext(), "Adaptive",
		[]string{"TopLeft", "TopRight", "BottomLeft", "BottomRight"}, false)
	if !rec.Apply {
		t.Fatalf("adaptive target must resolve even in cold start: %+v", rec)
	}
	if normalizeLearningEdge(rec.Edge) == "" {
		t.Fatalf("cold-start edge is invalid: %q", rec.Edge)
	}
}


func seedFarmTargetHistory(t *testing.T, e *ContextualEngine) {
	t.Helper()
	for i := 0; i < 8; i++ {
		o := testOutcome("TopRight", 1, 60)
		o.Context.Strategy = "valk_spam"
		o.Context.TownHall = 17
		o.Context.TargetGold = 800000
		o.Context.TargetElixir = 800000
		o.Context.TargetDE = 8000
		o.GoldStolen = 700000
		o.ElixirStolen = 700000
		o.DarkElixirStolen = 6000
		o.SearchDurationMS = 15000
		o.SearchSkips = 2
		o.FullRoutineDurationMS = 105000
		if _, err := e.Observe(o); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecommendTargetRejectsWeakConfiguredTargetEarly(t *testing.T) {
	e, _ := NewContextualEngine("")
	seedFarmTargetHistory(t, e)

	rules := TargetRules{MinGold: 600000, MinElixir: 600000, MinDarkElixir: 0, SearchEnabled: true}
	target := Target{Gold: 600000, Elixir: 600000, DarkElixir: 0}
	legacy := EvaluateTarget(target, rules)
	if !legacy.Accept {
		t.Fatal("test setup requires legacy acceptance")
	}

	rec := e.RecommendTarget("valk_spam", 17, target, rules, legacy, 5*time.Second, 1, true)
	if !rec.Apply {
		t.Fatalf("expected V3 recommendation: %+v", rec)
	}
	if rec.Accept {
		t.Fatalf("weak early target should be skipped for better farm throughput: %+v", rec)
	}
	if rec.PredictedFarmRate >= rec.BaselineFarmRate {
		t.Fatalf("test setup expected target below baseline: %+v", rec)
	}
}

func TestRecommendTargetAcceptsNearThresholdAfterSearchCost(t *testing.T) {
	e, _ := NewContextualEngine("")
	seedFarmTargetHistory(t, e)

	rules := TargetRules{
		MinGold: 800000,
		MinElixir: 800000,
		MinDarkElixir: 9000,
		SearchEnabled: true,
	}
	target := Target{Gold: 700000, Elixir: 700000, DarkElixir: 8000}
	legacy := EvaluateTarget(target, rules)
	if legacy.Accept {
		t.Fatal("test setup requires legacy rejection")
	}

	rec := e.RecommendTarget("valk_spam", 17, target, rules, legacy, 40*time.Second, 8, true)
	if !rec.Apply || !rec.Accept {
		t.Fatalf("near-threshold target should be accepted after costly search: %+v", rec)
	}
}

func TestRecommendTargetSafetyModeNeverAddsAggressiveReject(t *testing.T) {
	e, _ := NewContextualEngine("")
	seedFarmTargetHistory(t, e)

	rules := TargetRules{MinGold: 600000, MinElixir: 600000, SearchEnabled: true}
	target := Target{Gold: 600000, Elixir: 600000}
	legacy := EvaluateTarget(target, rules)
	if !legacy.Accept {
		t.Fatal("test setup requires legacy acceptance")
	}

	rec := e.RecommendTarget("valk_spam", 17, target, rules, legacy, 5*time.Second, 1, false)
	if !rec.Accept {
		t.Fatalf("safety pacing must not create extra search pressure: %+v", rec)
	}
}


func TestRecommendFarmExitLearnsConservativeThreshold(t *testing.T) {
	e, _ := NewContextualEngine("")
	seedFarmTargetHistory(t, e)

	rec := e.RecommendFarmExit("valk_spam", 17)
	if !rec.Enabled {
		t.Fatalf("expected learned farm exit: %+v", rec)
	}
	if rec.MinLootPercent < 55 || rec.MinLootPercent > 90 {
		t.Fatalf("unsafe learned loot threshold: %+v", rec)
	}
	if rec.StallSeconds < 5 || rec.StallSeconds > 30 {
		t.Fatalf("unsafe learned stall duration: %+v", rec)
	}
	if rec.Samples < 6 {
		t.Fatalf("farm exit enabled without enough evidence: %+v", rec)
	}
}

func TestRecommendFarmExitNeedsEnoughHistory(t *testing.T) {
	e, _ := NewContextualEngine("")
	for i := 0; i < 5; i++ {
		_, _ = e.Observe(testOutcome("TopRight", 1, 60))
	}
	rec := e.RecommendFarmExit("valk_spam", 17)
	if rec.Enabled {
		t.Fatalf("farm exit must stay disabled during cold start: %+v", rec)
	}
}
