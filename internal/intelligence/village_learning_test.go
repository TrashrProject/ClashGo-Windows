package intelligence

import (
	"path/filepath"
	"testing"
	"time"
)

func TestVillageMemoryReusesKnownWall(t *testing.T) {
	m, err := NewVillageMemory(filepath.Join(t.TempDir(), "village_model.json"))
	if err != nil { t.Fatal(err) }
	err = m.UpsertEntity(VillageEntity{
		ID: "wall-1", Kind: "wall", Level: 15,
		Position: VillagePoint{X: 420, Y: 360},
		Confidence: 0.91, LastSeen: time.Now(),
	})
	if err != nil { t.Fatal(err) }

	got, ok := m.KnownEntity("wall-1", time.Hour, 0.8)
	if !ok { t.Fatal("expected known wall") }
	if got.Position.X != 420 || got.Position.Y != 360 {
		t.Fatalf("unexpected wall position: %+v", got.Position)
	}
}

func TestVillageMemoryDropsConfidenceAfterFailures(t *testing.T) {
	m, err := NewVillageMemory(filepath.Join(t.TempDir(), "village_model.json"))
	if err != nil { t.Fatal(err) }
	_ = m.UpsertEntity(VillageEntity{
		ID: "wall-1", Kind: "wall",
		Position: VillagePoint{X: 100, Y: 100},
		Confidence: 0.9,
	})
	_ = m.MarkEntityResult("wall-1", false)
	_ = m.MarkEntityResult("wall-1", false)
	if _, ok := m.KnownEntity("wall-1", time.Hour, 0.5); ok {
		t.Fatal("entity with two failures should require rediscovery")
	}
}

func TestPlannerUsesWallForGoldOverflow(t *testing.T) {
	resources := VillageResources{Gold: 9_600_000, GoldValid: true}
	capacity := ResourceCapacity{Gold: 10_000_000}
	builders := BuilderState{Available: 1, Total: 6, Valid: true}
	candidates := []UpgradeCandidate{
		{ID: "cannon-1", Kind: "cannon", Cost: 3_000_000, Resource: ResourceGold, Priority: 2, Confidence: 0.95},
		{ID: "wall-12", Kind: "wall", Cost: 2_000_000, Resource: ResourceGold, Priority: 1, Wall: true, Confidence: 0.95},
	}
	policy := DefaultSpendPolicy()
	decision := PlanVillageSpend(resources, capacity, builders, candidates, policy)
	if decision.Candidate == nil || decision.Candidate.ID != "wall-12" {
		t.Fatalf("expected overflow wall spend, got %+v", decision)
	}
}

func TestPlannerPreservesReserve(t *testing.T) {
	resources := VillageResources{Gold: 5_000_000, GoldValid: true}
	capacity := ResourceCapacity{Gold: 10_000_000}
	builders := BuilderState{Available: 1, Total: 6, Valid: true}
	candidates := []UpgradeCandidate{
		{ID: "wall-1", Kind: "wall", Cost: 4_000_000, Resource: ResourceGold, Wall: true, Confidence: 0.95},
	}
	policy := DefaultSpendPolicy()
	policy.ReserveGold = 2_000_000
	decision := PlanVillageSpend(resources, capacity, builders, candidates, policy)
	if decision.Action != "wait" {
		t.Fatalf("reserve should block upgrade: %+v", decision)
	}
}
