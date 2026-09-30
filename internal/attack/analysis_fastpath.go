package attack

import (
	"strings"

	"github.com/Ducky705/ClashGO/pkg/strategy"
)

// strategyCountSlots returns the smallest safe set of cards whose numeric
// counts are needed before deployment. If every non-ability strategy unit has
// an identified slot, OCR only touches those slots. Any identity gap falls
// back to the complete bar rather than guessing.
func strategyCountSlots(sm *SlotManager, s *strategy.DynamicStrategy) (slots []*TrackedSlot, fastPath bool) {
	if sm == nil || s == nil {
		return nil, false
	}

	required := make(map[string]struct{})
	for _, phase := range s.Phases {
		for _, unit := range phase.Units {
			if unit.Pattern == "Ability" || phase.Pattern == "Ability" {
				continue
			}
			name := strings.ToLower(strings.TrimSpace(unit.Name))
			if name != "" {
				required[name] = struct{}{}
			}
		}
	}
	if len(required) == 0 {
		return sm.GetAllSlots(), false
	}

	seenSlots := make(map[*TrackedSlot]bool)
	for name := range required {
		slot := sm.GetSlot(name)
		if slot == nil {
			return sm.GetAllSlots(), false
		}
		if !seenSlots[slot] {
			seenSlots[slot] = true
			slots = append(slots, slot)
		}
	}
	if len(slots) == 0 {
		return sm.GetAllSlots(), false
	}
	return slots, true
}
