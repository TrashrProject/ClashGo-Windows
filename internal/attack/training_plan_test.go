package attack

import (
	"testing"
	"time"

	"github.com/Ducky705/ClashGO/internal/config"
)

func TestBuildTrainingPlanUsesOnlyMeasuredDeficitsForHousing(t *testing.T) {
	profile := config.FarmProfile{
		TownHall: 18,
		Label: "TH18",
		Troops: []config.FarmUnit{{Name:"Electro Dragon", Count:11, Housing:30}},
		Spells: []config.FarmUnit{{Name:"Rage Spell", Count:5, Housing:2}},
	}
	guard := ArmyGuardResult{
		Decision: ArmyGuardNotReady,
		Deficits: []ArmyDeficit{
			{Name:"Electro Dragon", Category:"troop", Have:8, Need:11, Missing:3, Housing:30, Confident:true},
			{Name:"Rage Spell", Category:"spell", Have:0, Need:5, Missing:5, Housing:2, Confident:false},
		},
	}
	p := BuildTrainingPlan(profile, guard)
	if p.TotalHousing != 90 {
		t.Fatalf("TotalHousing=%d want 90", p.TotalHousing)
	}
	if !p.HasUncertain {
		t.Fatal("expected uncertain flag")
	}
	if len(p.Items) != 2 {
		t.Fatalf("items=%d want 2", len(p.Items))
	}
}

func TestValidateTrainingPlanRejectsInconsistentDeficit(t *testing.T) {
	profile := config.FarmProfile{TownHall: 16, TroopCapacity: 320}
	plan := TrainingPlan{
		TownHall: 16,
		Items: []TrainingPlanItem{{
			Name: "Balloon", Category: "troop", Current: 5, Target: 10,
			ToTrain: 4, Housing: 5, SpaceNeed: 20, Confident: true,
		}},
	}
	if err := ValidateTrainingPlan(plan, profile); err == nil {
		t.Fatal("expected inconsistent deficit to be rejected")
	}
}

func TestActionableTrainingItemsSkipsUncertainRows(t *testing.T) {
	plan := TrainingPlan{Items: []TrainingPlanItem{
		{Name: "Balloon", Category: "troop", ToTrain: 3, Confident: true},
		{Name: "Rage Spell", Category: "spell", ToTrain: 2, Confident: false},
	}}
	items := ActionableTrainingItems(plan)
	if len(items) != 1 || items[0].Name != "Balloon" {
		t.Fatalf("actionable items=%v want only Balloon", items)
	}
}

func TestTrainingPlanStale(t *testing.T) {
	now := time.Unix(10_000, 0)
	fresh := TrainingPlan{CreatedAt: now.Add(-5 * time.Minute)}
	if fresh.Stale(now, 15*time.Minute) {
		t.Fatal("fresh plan reported stale")
	}
	old := TrainingPlan{CreatedAt: now.Add(-20 * time.Minute)}
	if !old.Stale(now, 15*time.Minute) {
		t.Fatal("old plan should be stale")
	}
	if !(TrainingPlan{}).Stale(now, 15*time.Minute) {
		t.Fatal("zero timestamp plan should be stale")
	}
}
