package multiaccount

import (
	"path/filepath"
	"testing"

	"github.com/Ducky705/ClashGO/internal/config"
)

func testConfig() config.MultiAccountConfig {
	return config.MultiAccountConfig{
		Enabled:            true,
		DefaultAttacksTurn: 2,
		Accounts: []config.ManagedAccount{
			{ID: "a", Label: "Main", PlayerTag: "#AAA", Enabled: true, SwitchSlot: 1},
			{ID: "b", Label: "Second", PlayerTag: "#BBB", Enabled: true, SwitchSlot: 2},
			{ID: "c", Label: "Off", PlayerTag: "#CCC", Enabled: false, SwitchSlot: 3},
		},
	}
}

func TestManagerRotatesAfterAttackLimit(t *testing.T) {
	m, err := NewManager("", testConfig(), "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Enabled() {
		t.Fatal("expected multi-account manager enabled")
	}
	if err := m.ObserveAttack(); err != nil {
		t.Fatal(err)
	}
	if _, due := m.NextDue(); due {
		t.Fatal("rotation should not be due after one attack")
	}
	if err := m.ObserveAttack(); err != nil {
		t.Fatal(err)
	}
	next, due := m.NextDue()
	if !due || next.ID != "b" {
		t.Fatalf("next=%+v due=%v want b/true", next, due)
	}
	if err := m.MarkSwitched("b"); err != nil {
		t.Fatal(err)
	}
	st := m.State()
	if st.ActiveAccountID != "b" || st.AttacksThisTurn != 0 || st.TotalSwitches != 1 {
		t.Fatalf("unexpected state after switch: %+v", st)
	}
}

func TestManagerSkipsDisabledAccounts(t *testing.T) {
	cfg := testConfig()
	cfg.Accounts[1].Enabled = false
	cfg.Accounts[2].Enabled = true
	m, err := NewManager("", cfg, "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	_ = m.ObserveAttack()
	_ = m.ObserveAttack()
	next, due := m.NextDue()
	if !due || next.ID != "c" {
		t.Fatalf("next=%+v due=%v want c/true", next, due)
	}
}

func TestManagerPersistsRotationState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.json")
	m, err := NewManager(path, testConfig(), "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	_ = m.ObserveAttack()
	_ = m.ObserveAttack()
	if err := m.MarkSwitched("b"); err != nil {
		t.Fatal(err)
	}
	_ = m.ObserveAttack()

	reloaded, err := NewManager(path, testConfig(), "#BBB")
	if err != nil {
		t.Fatal(err)
	}
	st := reloaded.State()
	if st.ActiveAccountID != "b" || st.AttacksThisTurn != 1 || st.TotalSwitches != 1 {
		t.Fatalf("persisted state mismatch: %+v", st)
	}
}

func TestPerAccountAttackLimitOverridesDefault(t *testing.T) {
	cfg := testConfig()
	cfg.Accounts[0].MaxAttacksPerTurn = 1
	m, _ := NewManager("", cfg, "#AAA")
	_ = m.ObserveAttack()
	next, due := m.NextDue()
	if !due || next.ID != "b" {
		t.Fatalf("expected immediate rotation to b, got %+v due=%v", next, due)
	}
}


func TestManagerDoesNotKeepDisabledActiveAccount(t *testing.T) {
	cfg := testConfig()
	path := filepath.Join(t.TempDir(), "multi.json")

	m, err := NewManager(path, cfg, "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.MarkSwitched("b"); err != nil {
		t.Fatal(err)
	}

	cfg.Accounts[1].Enabled = false
	if err := m.UpdateConfig(cfg, "#AAA"); err != nil {
		t.Fatal(err)
	}
	active, ok := m.Active()
	if !ok {
		t.Fatal("expected fallback active account")
	}
	if active.ID != "a" {
		t.Fatalf("active=%q want a after disabling b", active.ID)
	}
}
