package multiaccount

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

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


func TestManagerDisabledPreviousActiveReconcilesToEnabled(t *testing.T) {
	cfg := testConfig()
	m, err := NewManager("", cfg, "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.MarkSwitched("b"); err != nil {
		t.Fatal(err)
	}

	cfg.Accounts[1].Enabled = false
	cfg.Accounts[2].Enabled = true
	if err := m.UpdateConfig(cfg, "#AAA"); err != nil {
		t.Fatal(err)
	}
	active, ok := m.Active()
	if !ok {
		t.Fatal("expected enabled active account after reconciliation")
	}
	if !active.Enabled || active.ID == "b" {
		t.Fatalf("disabled account remained active: %+v", active)
	}
}

func TestManagerFailedSwitchUsesBackoff(t *testing.T) {
	m, err := NewManager("", testConfig(), "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.MarkSwitchFailed(fmt.Errorf("temporary failure")); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	allowed, remaining := m.SwitchAttemptAllowed(now)
	if allowed || remaining <= 0 {
		t.Fatalf("expected switch backoff, allowed=%v remaining=%s", allowed, remaining)
	}

	allowed, remaining = m.SwitchAttemptAllowed(now.Add(31 * time.Second))
	if !allowed || remaining != 0 {
		t.Fatalf("expected first backoff to expire, allowed=%v remaining=%s", allowed, remaining)
	}
}


func TestSwitchWarningDoesNotCreateBackoff(t *testing.T) {
	m, err := NewManager("", testConfig(), "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.MarkSwitched("b"); err != nil {
		t.Fatal(err)
	}
	if err := m.MarkSwitchWarning(fmt.Errorf("config persistence degraded")); err != nil {
		t.Fatal(err)
	}

	st := m.State()
	if st.ActiveAccountID != "b" {
		t.Fatalf("active account=%q want b", st.ActiveAccountID)
	}
	if st.ConsecutiveSwitchFailures != 0 {
		t.Fatalf("warning incremented switch failures: %+v", st)
	}
	if st.LastError == "" {
		t.Fatal("warning should remain visible in runtime status")
	}
	allowed, remaining := m.SwitchAttemptAllowed(time.Now().Add(time.Second))
	if !allowed || remaining != 0 {
		t.Fatalf("warning must not create switch backoff: allowed=%v remaining=%s", allowed, remaining)
	}
}

func TestPhysicalFailureStillCreatesBackoff(t *testing.T) {
	m, err := NewManager("", testConfig(), "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.MarkSwitchFailed(fmt.Errorf("selector not confirmed")); err != nil {
		t.Fatal(err)
	}
	st := m.State()
	if st.ConsecutiveSwitchFailures != 1 {
		t.Fatalf("physical failure count=%d want 1", st.ConsecutiveSwitchFailures)
	}
	allowed, remaining := m.SwitchAttemptAllowed(time.Now())
	if allowed || remaining <= 0 {
		t.Fatalf("physical failure must create backoff: allowed=%v remaining=%s", allowed, remaining)
	}
}


func TestPreparedSwitchJournalRequiresRecoveryAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.json")
	m, err := NewManager(path, testConfig(), "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.BeginSwitch("b"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewManager(path, testConfig(), "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	required, target := reloaded.RecoveryStatus()
	if !required || target != "b" {
		t.Fatalf("recovery=%v target=%q want true/b", required, target)
	}
	if reloaded.Enabled() {
		t.Fatal("rotation must be disabled while account identity is unresolved")
	}
	if _, due := reloaded.NextDue(); due {
		t.Fatal("no rotation may be scheduled while recovery is required")
	}
}

func TestPhysicalSwitchJournalRecoversTargetAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.json")
	m, err := NewManager(path, testConfig(), "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.BeginSwitch("b"); err != nil {
		t.Fatal(err)
	}
	if err := m.MarkPhysicalSwitch("b"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewManager(path, testConfig(), "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	required, target := reloaded.RecoveryStatus()
	if required {
		t.Fatalf("verified physical switch must not require manual recovery: target=%q", target)
	}
	active, ok := reloaded.Active()
	if !ok || active.ID != "b" {
		t.Fatalf("active=%+v ok=%v want b/true", active, ok)
	}
	if target != "b" {
		t.Fatalf("recovery target=%q want b for boot self-heal", target)
	}
}

func TestResolveRecoveryClearsJournalAndRestoresRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.json")
	m, err := NewManager(path, testConfig(), "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.BeginSwitch("b"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewManager(path, testConfig(), "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.ResolveRecovery("a"); err != nil {
		t.Fatal(err)
	}
	required, target := reloaded.RecoveryStatus()
	if required || target != "" {
		t.Fatalf("recovery not cleared: required=%v target=%q", required, target)
	}
	active, ok := reloaded.Active()
	if !ok || active.ID != "a" {
		t.Fatalf("active=%+v ok=%v want a/true", active, ok)
	}
	if !reloaded.Enabled() {
		t.Fatal("multi-account rotation should resume after explicit recovery resolution")
	}

	again, err := NewManager(path, testConfig(), "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	required, target = again.RecoveryStatus()
	if required || target != "" {
		t.Fatalf("recovery journal survived explicit resolution: required=%v target=%q", required, target)
	}
}


func TestManagerManualRecoveryResolutionClearsInterruptedJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.json")
	cfg := testConfig()

	m, err := NewManager(path, cfg, "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.BeginSwitch("b"); err != nil {
		t.Fatal(err)
	}

	// Simulate the process stopping after switch navigation began but before
	// MainVillage was verified.
	reloaded, err := NewManager(path, cfg, "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	required, target := reloaded.RecoveryStatus()
	if !required || target != "b" {
		t.Fatalf("recovery required=%v target=%q want true/b", required, target)
	}

	// The human sees that account B is actually loaded and confirms it.
	if err := reloaded.ResolveRecovery("b"); err != nil {
		t.Fatal(err)
	}
	required, target = reloaded.RecoveryStatus()
	if required || target != "" {
		t.Fatalf("recovery still active: required=%v target=%q", required, target)
	}
	active, ok := reloaded.Active()
	if !ok || active.ID != "b" {
		t.Fatalf("active=%+v ok=%v want b/true", active, ok)
	}
	if _, err := os.Stat(path + ".switch.json"); !os.IsNotExist(err) {
		t.Fatalf("switch journal should be removed after manual recovery, err=%v", err)
	}
}

func TestManagerRecoveryResolutionRejectsDisabledAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.json")
	cfg := testConfig()
	m, err := NewManager(path, cfg, "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.BeginSwitch("b"); err != nil {
		t.Fatal(err)
	}

	cfg.Accounts[1].Enabled = false
	reloaded, err := NewManager(path, cfg, "#AAA")
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.ResolveRecovery("b"); err == nil {
		t.Fatal("expected disabled account recovery confirmation to fail")
	}
}
