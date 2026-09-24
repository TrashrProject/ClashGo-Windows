package attack

import (
	"encoding/json"
	"os"
	"time"

	"github.com/Ducky705/ClashGO/internal/config"
	"github.com/Ducky705/ClashGO/internal/paths"
)

type TrainingPlanItem struct {
	Name      string `json:"name"`
	Category  string `json:"category"`
	Current   int    `json:"current"`
	Target    int    `json:"target"`
	ToTrain   int    `json:"to_train"`
	Housing   int    `json:"housing"`
	SpaceNeed int    `json:"space_need"`
	Confident bool   `json:"confident"`
}

type TrainingPlan struct {
	CreatedAt     time.Time          `json:"created_at"`
	TownHall      int                `json:"town_hall"`
	ProfileLabel  string             `json:"profile_label,omitempty"`
	Ready         bool               `json:"ready"`
	HasUncertain  bool               `json:"has_uncertain"`
	TotalHousing  int                `json:"total_housing_to_train"`
	Items         []TrainingPlanItem `json:"items,omitempty"`
}

// BuildTrainingPlan turns the visual army guard into an explicit queue request.
// Only confident deficits are actionable. Uncertain reads remain in the plan so
// diagnostics/UI can surface them, but they are never blindly queued.
func BuildTrainingPlan(profile config.FarmProfile, guard ArmyGuardResult) TrainingPlan {
	p := TrainingPlan{
		CreatedAt:    time.Now(),
		TownHall:     profile.TownHall,
		ProfileLabel: profile.Label,
		Ready:        guard.Decision == ArmyGuardReady,
	}
	for _, d := range guard.Deficits {
		item := TrainingPlanItem{
			Name: d.Name, Category: d.Category,
			Current: d.Have, Target: d.Need, ToTrain: d.Missing,
			Housing: d.Housing, Confident: d.Confident,
		}
		if d.Housing > 0 && d.Missing > 0 {
			item.SpaceNeed = d.Housing * d.Missing
		}
		if !d.Confident {
			p.HasUncertain = true
		}
		if item.ToTrain > 0 {
			p.Items = append(p.Items, item)
			if item.Confident {
				p.TotalHousing += item.SpaceNeed
			}
		}
	}
	return p
}

func WriteTrainingPlan(plan TrainingPlan) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.ResolveConfig("pending_training.json"), data, 0o600)
}
