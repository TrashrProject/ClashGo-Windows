package intelligence

import (
	"sort"
	"strings"
)

type ResourceKind string

const (
	ResourceGold       ResourceKind = "gold"
	ResourceElixir     ResourceKind = "elixir"
	ResourceDarkElixir ResourceKind = "dark_elixir"
)

type ResourceCapacity struct {
	Gold       int
	Elixir     int
	DarkElixir int
}

type UpgradeCandidate struct {
	ID         string
	Kind       string
	Level      int
	Cost       int
	Resource   ResourceKind
	Priority   int
	Wall       bool
	Confidence float64
}

type SpendPolicy struct {
	ReserveGold       int
	ReserveElixir     int
	ReserveDarkElixir int
	NearFullPercent   float64
	PreferWallsOnOverflow bool
	MinimumConfidence float64
}

type VillageDecision struct {
	Action      string
	Candidate   *UpgradeCandidate
	Reason      string
	Score       float64
	Resource    ResourceKind
	Spend       int
	Remaining   int
}

func DefaultSpendPolicy() SpendPolicy {
	return SpendPolicy{
		NearFullPercent:       0.92,
		PreferWallsOnOverflow: true,
		MinimumConfidence:     0.72,
	}
}

func PlanVillageSpend(resources VillageResources, capacity ResourceCapacity, builders BuilderState, candidates []UpgradeCandidate, policy SpendPolicy) VillageDecision {
	if policy.NearFullPercent <= 0 || policy.NearFullPercent > 1 {
		policy.NearFullPercent = 0.92
	}
	if policy.MinimumConfidence <= 0 || policy.MinimumConfidence > 1 {
		policy.MinimumConfidence = 0.72
	}
	if !builders.Valid || builders.Available <= 0 {
		return VillageDecision{Action: "wait", Reason: "no builder available"}
	}

	type scored struct {
		c UpgradeCandidate
		score float64
		remaining int
		overflow bool
	}
	var options []scored
	for _, c := range candidates {
		if strings.TrimSpace(c.ID) == "" || c.Cost <= 0 || c.Confidence < policy.MinimumConfidence {
			continue
		}
		available, reserve, capValue, valid := resourceValues(resources, capacity, policy, c.Resource)
		if !valid || available < c.Cost || available-c.Cost < reserve {
			continue
		}
		ratio := 0.0
		if capValue > 0 {
			ratio = float64(available) / float64(capValue)
		}
		overflow := capValue > 0 && ratio >= policy.NearFullPercent
		score := float64(c.Priority) * 10
		score += c.Confidence * 10
		if overflow {
			score += 35
			if c.Wall && policy.PreferWallsOnOverflow {
				score += 20
			}
		}
		// Prefer spending a meaningful amount when nearly full, but avoid
		// spending down to the reserve floor for marginal low-priority work.
		if available > 0 {
			score += float64(c.Cost) / float64(available) * 8
		}
		if c.Wall && !overflow {
			score -= 8
		}
		options = append(options, scored{
			c: c, score: score, remaining: available-c.Cost, overflow: overflow,
		})
	}

	if len(options) == 0 {
		return VillageDecision{Action: "wait", Reason: "no safe affordable upgrade"}
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].score == options[j].score {
			return options[i].c.Cost < options[j].c.Cost
		}
		return options[i].score > options[j].score
	})

	best := options[0]
	reason := "highest safe upgrade priority"
	if best.overflow && best.c.Wall {
		reason = "resource storage near full; spending overflow on known wall"
	} else if best.overflow {
		reason = "resource storage near full; spending on priority upgrade"
	}
	candidate := best.c
	return VillageDecision{
		Action:    "upgrade",
		Candidate: &candidate,
		Reason:    reason,
		Score:     best.score,
		Resource:  candidate.Resource,
		Spend:     candidate.Cost,
		Remaining: best.remaining,
	}
}

func resourceValues(r VillageResources, cap ResourceCapacity, p SpendPolicy, kind ResourceKind) (available, reserve, capacity int, valid bool) {
	switch kind {
	case ResourceGold:
		return r.Gold, p.ReserveGold, cap.Gold, r.GoldValid
	case ResourceElixir:
		return r.Elixir, p.ReserveElixir, cap.Elixir, r.ElixirValid
	case ResourceDarkElixir:
		return r.DarkElixir, p.ReserveDarkElixir, cap.DarkElixir, r.DarkValid
	default:
		return 0, 0, 0, false
	}
}
