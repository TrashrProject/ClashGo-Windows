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
		DeploySuccess: true,
		ReturnHomeSuccess: true,
		SafeDeployment: true,
		ParsedResults: true,
		At: time.Now(),
	}
}

func TestContextualRewardPrefersBetterBattle(t *testing.T) {
	one := testOutcome("TopLeft", 1, 55)
	three := testOutcome("TopLeft", 3, 100)
	if RewardForContextualOutcome(three) <= RewardForContextualOutcome(one) {
		t.Fatalf("3-star reward must beat 1-star reward")
	}

	crash := three
	crash.BlueStacksRestart = 1
	if RewardForContextualOutcome(crash) >= RewardForContextualOutcome(one) {
		t.Fatalf("BlueStacks restart must heavily penalize otherwise good outcome")
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
