package attack

import (
	"image"
	"testing"

	"github.com/Ducky705/ClashGO/internal/config"
)

func TestArmyStateManagerTracksProfileAndReconcile(t *testing.T) {
	profile := config.FarmProfile{
		TownHall: 18,
		Troops: []config.FarmUnit{
			{Name: "Electro Dragon", Count: 9, Housing: 30},
		},
		Spells: []config.FarmUnit{
			{Name: "Rage Spell", Count: 5, Housing: 2},
		},
		Heroes: []string{"Archer Queen", "Barbarian King"},
		Siege: "Stone Slammer",
	}

	m := NewArmyStateManager(profile)
	if got := m.Desired("electro dragon"); got != 9 {
		t.Fatalf("desired EDrag=%d want 9", got)
	}

	m.Attempt("Electro Dragon", 9)
	m.ObserveRemaining("Electro Dragon", 2)
	if got := m.Remaining("Electro Dragon"); got != 2 {
		t.Fatalf("remaining EDrag=%d want 2", got)
	}

	m.Attempt("Electro Dragon", 2)
	m.ObserveRemaining("Electro Dragon", 0)
	m.CompleteOneShot("Archer Queen")
	m.CompleteOneShot("Barbarian King")
	m.CompleteOneShot("Stone Slammer")

	s := m.Snapshot()
	var edComplete bool
	for _, u := range s.Units {
		if u.Name == "Electro Dragon" {
			edComplete = u.Status == ArmyComplete && u.Deployed == 9
		}
	}
	if !edComplete {
		t.Fatalf("EDrag state not complete: %+v", s)
	}
}


func TestArmyStateManagerDoesNotFailOnUnseenProfileUnit(t *testing.T) {
	profile := config.FarmProfile{
		TownHall: 18,
		Troops: []config.FarmUnit{{Name: "Electro Dragon", Count: 9, Housing: 30}},
		Heroes: []string{"Royal Champion"},
	}
	m := NewArmyStateManager(profile)

	m.Attempt("Electro Dragon", 9)
	m.ObserveRemaining("Electro Dragon", 0)

	// A profile hero that vision never observed must not create a phantom
	// incomplete deployment. The live bar remains authoritative.
	if got := m.IncompleteCount(); got != 0 {
		t.Fatalf("unseen profile unit created %d incomplete state(s)", got)
	}
}

func TestArmyStateManagerRecordsReplayGeometry(t *testing.T) {
	profile := config.FarmProfile{
		TownHall: 18,
		Troops: []config.FarmUnit{{Name: "Electro Dragon", Count: 9, Housing: 30}},
	}
	m := NewArmyStateManager(profile)

	p1 := image.Pt(120, 220)
	p2 := image.Pt(120, 420)
	m.RecordDeploy("Electro Dragon", "Troop", 9, 350, 676, "left", p1, p2)

	events := m.ReplayEvents()
	if len(events) != 1 {
		t.Fatalf("events=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Kind != "deploy" || ev.Name != "Electro Dragon" || ev.Category != "Troop" {
		t.Fatalf("unexpected replay event: %+v", ev)
	}
	if ev.Count != 9 || ev.SlotX != 350 || ev.SlotY != 676 || ev.DeploySide != "left" {
		t.Fatalf("unexpected replay metadata: %+v", ev)
	}
	if ev.P1 != p1 || ev.P2 != p2 || ev.OffsetMS < 0 {
		t.Fatalf("unexpected replay geometry/timing: %+v", ev)
	}

	// ReplayEvents must return a copy so UI/trace consumers cannot mutate the
	// live attack-state timeline.
	events[0].Name = "mutated"
	if got := m.ReplayEvents()[0].Name; got != "Electro Dragon" {
		t.Fatalf("ReplayEvents leaked mutable backing storage: %q", got)
	}
}
