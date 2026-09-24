package attack

import (
	"testing"

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
