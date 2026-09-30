package attack

import (
	"strings"

	"github.com/Ducky705/ClashGO/pkg/strategy"
	"gocv.io/x/gocv"
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


// strategyTemplateSubset limits the expensive first-pass portrait matcher to
// units actually referenced by the active strategy. It is enabled only when
// every required strategy unit has a concrete template; otherwise callers
// keep the full template set so no identity evidence is lost.
func strategyTemplateSubset(all map[string]gocv.Mat, s *strategy.DynamicStrategy) (map[string]gocv.Mat, bool) {
	if len(all) == 0 || s == nil {
		return all, false
	}

	required := make(map[string]struct{})
	for _, phase := range s.Phases {
		for _, unit := range phase.Units {
			if unit.Pattern == "Ability" || phase.Pattern == "Ability" {
				continue
			}
			name := strings.ToLower(strings.TrimSpace(unit.Name))
			if name == "" {
				continue
			}
			required[strings.ReplaceAll(name, " ", "_")] = struct{}{}
		}
	}
	if len(required) == 0 {
		return all, false
	}

	subset := make(map[string]gocv.Mat, len(required)+3)
	for name := range required {
		tpl, ok := all[name]
		if !ok || tpl.Empty() {
			return all, false
		}
		subset[name] = tpl
	}

	// Generic aliases are cheap and useful for layouts where the active siege
	// or clan-castle card is represented by a generic portrait.
	for _, alias := range []string{"siege_machine", "clan_castle", "cc"} {
		if tpl, ok := all[alias]; ok && !tpl.Empty() {
			subset[alias] = tpl
		}
	}
	return subset, true
}
