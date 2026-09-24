package attack

import (
	"encoding/json"
	"fmt"
	"strings"
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

// ValidateTrainingPlan rejects impossible or internally inconsistent plans
// before a future training executor is allowed to act on them.
func ValidateTrainingPlan(plan TrainingPlan, profile config.FarmProfile) error {
	if plan.TownHall != 0 && profile.TownHall != 0 && plan.TownHall != profile.TownHall {
		return fmt.Errorf("training plan TH%d does not match active TH%d profile", plan.TownHall, profile.TownHall)
	}

	seen := make(map[string]bool)
	troopSpace := 0
	spellSpace := 0
	for _, item := range plan.Items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return fmt.Errorf("training plan contains unnamed item")
		}
		key := strings.ToLower(strings.TrimSpace(item.Category)) + ":" + strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("training plan contains duplicate item %s", name)
		}
		seen[key] = true

		if item.Current < 0 || item.Target < 0 || item.ToTrain < 0 || item.Housing < 0 || item.SpaceNeed < 0 {
			return fmt.Errorf("training plan contains negative values for %s", name)
		}
		if item.Current > item.Target {
			return fmt.Errorf("training plan current count exceeds target for %s", name)
		}
		if item.ToTrain != item.Target-item.Current {
			return fmt.Errorf("training plan deficit mismatch for %s", name)
		}
		if item.Housing > 0 && item.SpaceNeed != item.Housing*item.ToTrain {
			return fmt.Errorf("training plan housing mismatch for %s", name)
		}

		// Only confident rows are actionable and therefore count toward
		// capacity validation. Uncertain rows are diagnostic-only.
		if item.Confident {
			switch strings.ToLower(strings.TrimSpace(item.Category)) {
			case "troop":
				troopSpace += item.SpaceNeed
			case "spell":
				spellSpace += item.SpaceNeed
			}
		}
	}

	if profile.TroopCapacity > 0 && troopSpace > profile.TroopCapacity {
		return fmt.Errorf("training plan troop deficit %d exceeds troop capacity %d", troopSpace, profile.TroopCapacity)
	}
	if profile.SpellCapacity > 0 && spellSpace > profile.SpellCapacity {
		return fmt.Errorf("training plan spell deficit %d exceeds spell capacity %d", spellSpace, profile.SpellCapacity)
	}
	return nil
}

// ActionableTrainingItems returns only deficits backed by confident visual/OCR
// evidence. A UI executor must never act on uncertain diagnostic rows.
func ActionableTrainingItems(plan TrainingPlan) []TrainingPlanItem {
	out := make([]TrainingPlanItem, 0, len(plan.Items))
	for _, item := range plan.Items {
		if item.Confident && item.ToTrain > 0 {
			out = append(out, item)
		}
	}
	return out
}

func WriteTrainingPlan(plan TrainingPlan) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.ResolveConfig("pending_training.json"), data, 0o600)
}
