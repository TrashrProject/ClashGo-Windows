package attack

import (
	"encoding/json"
	"os"
	"time"

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
	Timestamp time.Time            `json:"timestamp"`
	Units     []ArmyInspectionUnit `json:"units"`
}

func writeArmyInspection(slots []*TrackedSlot, counts []TroopCount) {
	s := ArmyInspectionSnapshot{Timestamp: time.Now()}
	for _, slot := range slots {
		if slot == nil {
			continue
		}
		s.Units = append(s.Units, ArmyInspectionUnit{
			Name: slot.UnitName,
			Category: slot.Category,
			Count: GetCountForSlot(counts, slot.X),
			Confidence: slot.Confidence,
			SlotX: slot.X,
		})
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(paths.ResolveConfig("current_army.json"), data, 0o600)
}
