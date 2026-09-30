package attack

import (
	"testing"

	"github.com/Ducky705/ClashGO/pkg/strategy"
)

// Regression guard for the deploy router. These categories must remain
// mutually exclusive so a siege can never be fired once as a troop and again
// through the dedicated siege path, and heroes/abilities never leak into the
// regular troop loop.
func TestDeploymentResolversAreMutuallyExclusive(t *testing.T) {
	plan := PhasePlan{UnitPlans: []UnitPlan{
		{Unit: strategy.Unit{Name: "Electro Dragon"}},
		{Unit: strategy.Unit{Name: "Stone Slammer"}, IsSiege: true},
		{Unit: strategy.Unit{Name: "Archer Queen"}, IsHero: true},
		{Unit: strategy.Unit{Name: "Rage Spell"}, IsSpell: true},
		{Unit: strategy.Unit{Name: "Archer Queen Ability"}, IsHero: true, IsAbility: true},
	}}

	troops := ResolveTroopTargets(plan)
	sieges := ResolveSiegeTargets(plan)
	heroes := ResolveHeroTargets(plan)
	spells := ResolveSpellTargets(plan)

	if len(troops) != 1 || troops[0].Unit.Name != "Electro Dragon" {
		t.Fatalf("troop routing regression: %+v", troops)
	}
	if len(sieges) != 1 || sieges[0].Unit.Name != "Stone Slammer" {
		t.Fatalf("siege routing regression: %+v", sieges)
	}
	if len(heroes) != 1 || heroes[0].Unit.Name != "Archer Queen" {
		t.Fatalf("hero routing regression: %+v", heroes)
	}
	if len(spells) != 1 || spells[0].Unit.Name != "Rage Spell" {
		t.Fatalf("spell routing regression: %+v", spells)
	}

	seen := map[string]string{}
	groups := map[string][]UnitPlan{
		"troop": troops,
		"siege": sieges,
		"hero": heroes,
		"spell": spells,
	}
	for group, units := range groups {
		for _, u := range units {
			if previous, exists := seen[u.Unit.Name]; exists {
				t.Fatalf("%q routed twice (%s and %s)", u.Unit.Name, previous, group)
			}
			seen[u.Unit.Name] = group
		}
	}
}

func TestSiegeNeverAppearsInTroopResolver(t *testing.T) {
	plan := PhasePlan{UnitPlans: []UnitPlan{
		{Unit: strategy.Unit{Name: "Battle Blimp"}, IsSiege: true},
		{Unit: strategy.Unit{Name: "Log Launcher"}, IsSiege: true},
	}}
	if got := ResolveTroopTargets(plan); len(got) != 0 {
		t.Fatalf("sieges leaked into troop path: %+v", got)
	}
	if got := ResolveSiegeTargets(plan); len(got) != 2 {
		t.Fatalf("expected both sieges in siege path, got %+v", got)
	}
}

func TestAbilityNeverAppearsInHeroDeployTargets(t *testing.T) {
	plan := PhasePlan{UnitPlans: []UnitPlan{
		{Unit: strategy.Unit{Name: "Grand Warden"}, IsHero: true},
		{Unit: strategy.Unit{Name: "Grand Warden Ability"}, IsHero: true, IsAbility: true},
	}}
	got := ResolveHeroTargets(plan)
	if len(got) != 1 || got[0].Unit.Name != "Grand Warden" {
		t.Fatalf("hero ability leaked into initial hero deploy: %+v", got)
	}
}
