package attack

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Ducky705/ClashGO/internal/config"
	"github.com/Ducky705/ClashGO/internal/paths"
)

type ArmyInspectionUnit struct {
	Name       string  `json:"name"`
	Category   string  `json:"category"`
	Count      int     `json:"count"`
	Confidence float64 `json:"confidence"`
	SlotX      int     `json:"slot_x"`
}

type ArmyInspectionSnapshot struct {
	Timestamp      time.Time            `json:"timestamp"`
	Units          []ArmyInspectionUnit `json:"units"`
	TargetTownHall int                  `json:"target_town_hall,omitempty"`
	TargetLabel    string               `json:"target_label,omitempty"`
	Ready          bool                 `json:"ready"`
	Uncertain      bool                 `json:"uncertain"`
	Warnings       []string             `json:"warnings,omitempty"`
}

func writeArmyInspection(slots []*TrackedSlot, counts []TroopCount, profile *config.FarmProfile) {
	s := ArmyInspectionSnapshot{Timestamp: time.Now(), Ready: true}
	observed := make(map[string]int)
	unknown := 0

	for _, slot := range slots {
		if slot == nil {
			continue
		}
		count := GetCountForSlot(counts, slot.X)
		if count <= 0 && (slot.Category == "Hero" || slot.Category == "Siege" || slot.Category == "CC") {
			count = 1
		}
		s.Units = append(s.Units, ArmyInspectionUnit{
			Name: slot.UnitName,
			Category: slot.Category,
			Count: count,
			Confidence: slot.Confidence,
			SlotX: slot.X,
		})
		name := strings.ToLower(strings.TrimSpace(slot.UnitName))
		if name == "" {
			unknown++
			continue
		}
		observed[name] += count
	}

	if profile != nil {
		s.TargetTownHall = profile.TownHall
		s.TargetLabel = profile.Label

		check := func(name string, expected int, category string) {
			if expected <= 0 || strings.TrimSpace(name) == "" {
				return
			}
			got := observed[strings.ToLower(strings.TrimSpace(name))]
			if got >= expected {
				return
			}
			msg := fmt.Sprintf("%s %s: detected %d, target %d", category, name, got, expected)
			s.Warnings = append(s.Warnings, msg)
			if unknown > 0 {
				s.Uncertain = true
			} else {
				s.Ready = false
			}
		}

		for _, unit := range profile.Troops {
			check(unit.Name, unit.Count, "troop")
		}
		for _, unit := range profile.Spells {
			check(unit.Name, unit.Count, "spell")
		}
		// Hero/siege quantities are visually one-shot and portrait templates
		// may vary by skin. Do not mark the army unready solely because a
		// named hero was not recognized; the structural hero detector handles
		// them safely during deployment.
	}

	if len(s.Warnings) == 0 {
		s.Uncertain = false
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(paths.ResolveConfig("current_army.json"), data, 0o600)
}
