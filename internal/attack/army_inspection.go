package attack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

const armyGuardMinCountConfidence = 0.70

func buildArmyInspection(slots []*TrackedSlot, counts []TroopCount, profile *config.FarmProfile) ArmyInspectionSnapshot {
	s := ArmyInspectionSnapshot{Timestamp: time.Now(), Ready: true}
	observed := make(map[string]int)
	quantityUnknown := make(map[string]bool)
	unknownSlots := 0

	countAt := func(x int) (TroopCount, bool) {
		for _, c := range counts {
			if c.X == x {
				return c, true
			}
		}
		return TroopCount{}, false
	}

	for _, slot := range slots {
		if slot == nil {
			continue
		}

		countInfo, countSeen := countAt(slot.X)
		count := countInfo.Count
		if count <= 0 && (slot.Category == "Hero" || slot.Category == "Siege" || slot.Category == "CC") {
			// One-shot cards do not expose a troop quantity. Presence of the
			// active card itself is enough for the inspection snapshot.
			count = 1
		}

		s.Units = append(s.Units, ArmyInspectionUnit{
			Name:       slot.UnitName,
			Category:   slot.Category,
			Count:      count,
			Confidence: slot.Confidence,
			SlotX:      slot.X,
		})

		name := strings.ToLower(strings.TrimSpace(slot.UnitName))
		if name == "" {
			unknownSlots++
			continue
		}

		observed[name] += count

		// A live troop/spell card with no reliable quantity read is not proof
		// that the unit is missing. Treat it as uncertainty so AutoArmyGuard
		// fails open instead of skipping a perfectly valid target because OCR
		// missed the small xN label.
		if (slot.Category == "Troop" || slot.Category == "Spell") &&
			(!countSeen || countInfo.Confidence < armyGuardMinCountConfidence || countInfo.Count <= 0) {
			quantityUnknown[name] = true
		}
	}

	if profile != nil {
		s.TargetTownHall = profile.TownHall
		s.TargetLabel = profile.Label

		check := func(name string, expected int, category string) {
			if expected <= 0 || strings.TrimSpace(name) == "" {
				return
			}
			key := strings.ToLower(strings.TrimSpace(name))
			got := observed[key]
			if got >= expected {
				return
			}

			s.Warnings = append(s.Warnings,
				fmt.Sprintf("%s %s: detected %d, target %d", category, name, got, expected))

			// Unknown portraits OR an unreadable quantity for this exact unit
			// mean the mismatch is not proven. Keep Ready=true and mark the
			// snapshot uncertain so callers can safely fail open.
			if unknownSlots > 0 || quantityUnknown[key] {
				s.Uncertain = true
				return
			}
			s.Ready = false
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
	return s
}

func writeArmyInspectionSnapshotAtPath(s ArmyInspectionSnapshot, path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

func writeArmyInspectionSnapshot(s ArmyInspectionSnapshot) {
	writeArmyInspectionSnapshotAtPath(s, paths.ResolveConfig("current_army.json"))
}

func (e *Executor) persistArmyInspectionSnapshot(s ArmyInspectionSnapshot) {
	writeArmyInspectionSnapshot(s)
	if e == nil || strings.TrimSpace(e.armyInspectionPath) == "" {
		return
	}
	writeArmyInspectionSnapshotAtPath(s, e.armyInspectionPath)
}

func writeArmyInspection(slots []*TrackedSlot, counts []TroopCount, profile *config.FarmProfile) {
	writeArmyInspectionSnapshot(buildArmyInspection(slots, counts, profile))
}
