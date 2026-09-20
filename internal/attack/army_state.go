package attack

import (
	"strings"
	"sync"
	"time"

	"github.com/Ducky705/ClashGO/internal/config"
)

type ArmyUnitStatus string

const (
	ArmyPending    ArmyUnitStatus = "pending"
	ArmyDeploying  ArmyUnitStatus = "deploying"
	ArmyComplete   ArmyUnitStatus = "complete"
	ArmyUnavailable ArmyUnitStatus = "unavailable"
	ArmyFailed     ArmyUnitStatus = "failed"
)

type ArmyUnitState struct {
	Name      string         `json:"name"`
	Category  string         `json:"category"`
	Expected  int            `json:"expected"`
	Remaining int            `json:"remaining"`
	Deployed  int            `json:"deployed"`
	Attempts  int            `json:"attempts"`
	Status    ArmyUnitStatus `json:"status"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type ArmyStateSnapshot struct {
	TownHall int             `json:"town_hall"`
	Units    []ArmyUnitState `json:"units"`
	Complete bool            `json:"complete"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ArmyStateManager is the attack-time source of truth. Vision supplies
// observations; the farm profile supplies intent. Keeping both in one place
// prevents the deploy loop from independently guessing quantities and status.
type ArmyStateManager struct {
	mu sync.Mutex
	th int
	units map[string]*ArmyUnitState
}

func NewArmyStateManager(profile config.FarmProfile) *ArmyStateManager {
	m := &ArmyStateManager{th: profile.TownHall, units: make(map[string]*ArmyUnitState)}
	for _, u := range profile.Troops {
		m.add(u.Name, "Troop", u.Count)
	}
	for _, u := range profile.Spells {
		m.add(u.Name, "Spell", u.Count)
	}
	for _, h := range profile.Heroes {
		m.add(h, "Hero", 1)
	}
	if strings.TrimSpace(profile.Siege) != "" {
		m.add(profile.Siege, "Siege", 1)
	}
	return m
}

func armyKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (m *ArmyStateManager) add(name, category string, expected int) {
	if expected <= 0 || strings.TrimSpace(name) == "" {
		return
	}
	k := armyKey(name)
	m.units[k] = &ArmyUnitState{
		Name: name, Category: category, Expected: expected,
		Remaining: expected, Status: ArmyPending, UpdatedAt: time.Now(),
	}
}

func (m *ArmyStateManager) Desired(name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u := m.units[armyKey(name)]; u != nil {
		return u.Expected
	}
	return 0
}

func (m *ArmyStateManager) Remaining(name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u := m.units[armyKey(name)]; u != nil {
		return u.Remaining
	}
	return 0
}

func (m *ArmyStateManager) ObserveRemaining(name string, remaining int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u := m.units[armyKey(name)]
	if u == nil || remaining < 0 {
		return
	}
	if remaining > u.Expected {
		remaining = u.Expected
	}
	u.Remaining = remaining
	u.Deployed = u.Expected - remaining
	if u.Deployed < 0 { u.Deployed = 0 }
	if remaining == 0 {
		u.Status = ArmyComplete
	} else if u.Attempts > 0 {
		u.Status = ArmyDeploying
	}
	u.UpdatedAt = time.Now()
}

func (m *ArmyStateManager) Attempt(name string, sent int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u := m.units[armyKey(name)]
	if u == nil {
		return
	}
	u.Attempts++
	if u.Status != ArmyComplete {
		u.Status = ArmyDeploying
	}
	// This is only an optimistic estimate until the next OCR observation.
	if sent > 0 && u.Remaining > 0 {
		guess := u.Remaining - sent
		if guess < 0 { guess = 0 }
		u.Remaining = guess
		u.Deployed = u.Expected - guess
	}
	u.UpdatedAt = time.Now()
}

func (m *ArmyStateManager) CompleteOneShot(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u := m.units[armyKey(name)]
	if u == nil { return }
	u.Attempts++
	u.Remaining = 0
	u.Deployed = u.Expected
	u.Status = ArmyComplete
	u.UpdatedAt = time.Now()
}

func (m *ArmyStateManager) Fail(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u := m.units[armyKey(name)]; u != nil {
		u.Status = ArmyFailed
		u.UpdatedAt = time.Now()
	}
}

func (m *ArmyStateManager) IncompleteCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, u := range m.units {
		if u.Status != ArmyComplete && u.Status != ArmyUnavailable {
			n++
		}
	}
	return n
}

func (m *ArmyStateManager) IncompleteUnits() []ArmyUnitState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ArmyUnitState, 0)
	for _, u := range m.units {
		if u.Status != ArmyComplete && u.Status != ArmyUnavailable {
			out = append(out, *u)
		}
	}
	return out
}

func (m *ArmyStateManager) Snapshot() ArmyStateSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := ArmyStateSnapshot{TownHall: m.th, Complete: true, UpdatedAt: time.Now()}
	for _, u := range m.units {
		cp := *u
		s.Units = append(s.Units, cp)
		if cp.Status != ArmyComplete {
			s.Complete = false
		}
	}
	return s
}
