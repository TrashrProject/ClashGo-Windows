package multiaccount

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Ducky705/ClashGO/internal/config"
)

type State struct {
	Version         int       `json:"version"`
	ActiveAccountID string    `json:"active_account_id,omitempty"`
	AttacksThisTurn int       `json:"attacks_this_turn"`
	TotalSwitches   int       `json:"total_switches"`
	LastSwitchAt             time.Time `json:"last_switch_at,omitempty"`
	LastSwitchAttemptAt      time.Time `json:"last_switch_attempt_at,omitempty"`
	ConsecutiveSwitchFailures int      `json:"consecutive_switch_failures"`
	LastError                string    `json:"last_error,omitempty"`
	RecoveryRequired         bool      `json:"recovery_required,omitempty"`
	RecoveryTargetAccountID  string    `json:"recovery_target_account_id,omitempty"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type Manager struct {
	mu    sync.Mutex
	path  string
	cfg   config.MultiAccountConfig
	state State
}

type switchJournal struct {
	Version       int       `json:"version"`
	FromAccountID string    `json:"from_account_id,omitempty"`
	ToAccountID   string    `json:"to_account_id"`
	ToPlayerTag   string    `json:"to_player_tag,omitempty"`
	Phase         string    `json:"phase"`
	StartedAt     time.Time `json:"started_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}


func NewManager(path string, cfg config.MultiAccountConfig, currentTag string) (*Manager, error) {
	m := &Manager{
		path: path,
		cfg:  normalizeConfig(cfg),
		state: State{Version: 1},
	}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &m.state); err != nil {
			return nil, fmt.Errorf("decode multi-account state: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if m.state.Version <= 0 {
		m.state.Version = 1
	}
	m.reconcileActiveLocked(currentTag)
	if journal, ok := m.loadSwitchJournalLocked(); ok {
		switch journal.Phase {
		case "physical_switched":
			if target := m.accountByIDLocked(journal.ToAccountID); target != nil && target.Enabled {
				m.state.ActiveAccountID = target.ID
				m.state.AttacksThisTurn = 0
				m.state.RecoveryRequired = false
				m.state.RecoveryTargetAccountID = target.ID
				m.state.LastError = "recovered verified account switch from journal"
			}
		case "prepared":
			// The process stopped after navigation started but before a new
			// MainVillage was confirmed. We cannot prove which account Clash is
			// showing, so block unattended farming instead of guessing.
			m.state.RecoveryRequired = true
			m.state.RecoveryTargetAccountID = journal.ToAccountID
			m.state.LastError = "account switch was interrupted before verification; confirm the active Clash account"
		}
	}
	if err := m.saveLocked(); err != nil {
		return nil, err
	}
	return m, nil
}

func normalizeConfig(cfg config.MultiAccountConfig) config.MultiAccountConfig {
	if cfg.DefaultAttacksTurn <= 0 {
		cfg.DefaultAttacksTurn = 10
	}
	seen := map[string]bool{}
	out := make([]config.ManagedAccount, 0, len(cfg.Accounts))
	for i, a := range cfg.Accounts {
		a.ID = strings.TrimSpace(a.ID)
		a.Label = strings.TrimSpace(a.Label)
		a.PlayerTag = normalizeTag(a.PlayerTag)
		if a.ID == "" {
			if a.PlayerTag != "" {
				a.ID = strings.TrimPrefix(strings.ToLower(a.PlayerTag), "#")
			} else {
				a.ID = fmt.Sprintf("account-%d", i+1)
			}
		}
		if seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		if a.MaxAttacksPerTurn < 0 {
			a.MaxAttacksPerTurn = 0
		}
		out = append(out, a)
	}
	cfg.Accounts = out
	return cfg
}

func normalizeTag(tag string) string {
	tag = strings.ToUpper(strings.TrimSpace(tag))
	if tag == "" {
		return ""
	}
	if !strings.HasPrefix(tag, "#") {
		tag = "#" + tag
	}
	return tag
}

func (m *Manager) reconcileActiveLocked(currentTag string) {
	currentTag = normalizeTag(currentTag)

	// The PlayerTag is persisted only after ClashGO has observed a verified
	// account load. Prefer it over scheduler state so a process stop/crash
	// between config.Save and MarkSwitched self-heals on the next boot.
	for i := range m.cfg.Accounts {
		a := &m.cfg.Accounts[i]
		if a.Enabled && currentTag != "" && normalizeTag(a.PlayerTag) == currentTag {
			if m.state.ActiveAccountID != a.ID {
				m.state.AttacksThisTurn = 0
			}
			m.state.ActiveAccountID = a.ID
			return
		}
	}

	if configured := m.accountByIDLocked(m.cfg.ActiveAccountID); m.cfg.ActiveAccountID != "" && configured != nil && configured.Enabled {
		if m.state.ActiveAccountID != m.cfg.ActiveAccountID {
			m.state.AttacksThisTurn = 0
		}
		m.state.ActiveAccountID = m.cfg.ActiveAccountID
		return
	}

	if active := m.accountByIDLocked(m.state.ActiveAccountID); active != nil && active.Enabled {
		return
	}

	for i := range m.cfg.Accounts {
		if m.cfg.Accounts[i].Enabled {
			m.state.ActiveAccountID = m.cfg.Accounts[i].ID
			m.state.AttacksThisTurn = 0
			return
		}
	}
	m.state.ActiveAccountID = ""
	m.state.AttacksThisTurn = 0
}
func (m *Manager) Enabled() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg.Enabled && m.enabledCountLocked() >= 2 && !m.state.RecoveryRequired
}

func (m *Manager) State() State {
	if m == nil {
		return State{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *Manager) Active() (config.ManagedAccount, bool) {
	if m == nil {
		return config.ManagedAccount{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.accountByIDLocked(m.state.ActiveAccountID)
	if a == nil {
		return config.ManagedAccount{}, false
	}
	return *a, true
}

func (m *Manager) Account(accountID string) (config.ManagedAccount, bool) {
	if m == nil {
		return config.ManagedAccount{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.accountByIDLocked(strings.TrimSpace(accountID))
	if a == nil || !a.Enabled {
		return config.ManagedAccount{}, false
	}
	return *a, true
}


func (m *Manager) ObserveAttack() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.cfg.Enabled || m.enabledCountLocked() < 2 {
		return nil
	}
	m.state.AttacksThisTurn++
	m.state.UpdatedAt = time.Now()
	return m.saveLocked()
}

func (m *Manager) NextDue() (config.ManagedAccount, bool) {
	if m == nil {
		return config.ManagedAccount{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.cfg.Enabled || m.enabledCountLocked() < 2 || m.state.RecoveryRequired {
		return config.ManagedAccount{}, false
	}
	active := m.accountByIDLocked(m.state.ActiveAccountID)
	if active == nil {
		return config.ManagedAccount{}, false
	}
	limit := active.MaxAttacksPerTurn
	if limit <= 0 {
		limit = m.cfg.DefaultAttacksTurn
	}
	if limit <= 0 || m.state.AttacksThisTurn < limit {
		return config.ManagedAccount{}, false
	}

	idx := -1
	for i := range m.cfg.Accounts {
		if m.cfg.Accounts[i].ID == active.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return config.ManagedAccount{}, false
	}
	for step := 1; step <= len(m.cfg.Accounts); step++ {
		candidate := m.cfg.Accounts[(idx+step)%len(m.cfg.Accounts)]
		if candidate.Enabled && candidate.ID != active.ID {
			return candidate, true
		}
	}
	return config.ManagedAccount{}, false
}

func (m *Manager) SwitchAttemptAllowed(now time.Time) (bool, time.Duration) {
	if m == nil {
		return false, 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.ConsecutiveSwitchFailures <= 0 || m.state.LastSwitchAttemptAt.IsZero() {
		return true, 0
	}
	failures := m.state.ConsecutiveSwitchFailures
	if failures > 4 {
		failures = 4
	}
	cooldown := 30 * time.Second * time.Duration(1<<uint(failures-1))
	if cooldown > 4*time.Minute {
		cooldown = 4 * time.Minute
	}
	remaining := cooldown - now.Sub(m.state.LastSwitchAttemptAt)
	if remaining <= 0 {
		return true, 0
	}
	return false, remaining
}

func (m *Manager) BeginSwitch(accountID string) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	target := m.accountByIDLocked(accountID)
	if target == nil || !target.Enabled {
		return fmt.Errorf("unknown or disabled multi-account profile %q", accountID)
	}
	now := time.Now()
	j := switchJournal{
		Version:       1,
		FromAccountID: m.state.ActiveAccountID,
		ToAccountID:   target.ID,
		ToPlayerTag:   target.PlayerTag,
		Phase:         "prepared",
		StartedAt:     now,
		UpdatedAt:     now,
	}
	return m.saveSwitchJournalLocked(j)
}

func (m *Manager) MarkPhysicalSwitch(accountID string) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.loadSwitchJournalLocked()
	if !ok || j.ToAccountID != accountID {
		return fmt.Errorf("switch journal missing or targets another account")
	}
	j.Phase = "physical_switched"
	j.UpdatedAt = time.Now()
	m.state.RecoveryRequired = false
	m.state.RecoveryTargetAccountID = accountID
	return m.saveSwitchJournalLocked(j)
}

func (m *Manager) AbortPreparedSwitch(accountID string) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.loadSwitchJournalLocked()
	if !ok {
		return nil
	}
	if j.ToAccountID != accountID || j.Phase != "prepared" {
		return nil
	}
	return m.removeSwitchJournalLocked()
}

func (m *Manager) CompleteSwitch(accountID string) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.loadSwitchJournalLocked()
	if !ok {
		return nil
	}
	if j.ToAccountID != accountID {
		return fmt.Errorf("cannot complete switch journal for %q", accountID)
	}
	m.state.RecoveryRequired = false
	m.state.RecoveryTargetAccountID = ""
	return m.removeSwitchJournalLocked()
}

func (m *Manager) RecoveryStatus() (bool, string) {
	if m == nil {
		return false, ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.RecoveryRequired, m.state.RecoveryTargetAccountID
}

func (m *Manager) ResolveRecovery(accountID string) error {
	if m == nil {
		return fmt.Errorf("multi-account manager unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	accountID = strings.TrimSpace(accountID)
	a := m.accountByIDLocked(accountID)
	if a == nil || !a.Enabled {
		return fmt.Errorf("unknown or disabled multi-account profile %q", accountID)
	}
	if !m.state.RecoveryRequired {
		return fmt.Errorf("multi-account recovery is not required")
	}

	now := time.Now()
	m.state.ActiveAccountID = a.ID
	m.state.AttacksThisTurn = 0
	m.state.RecoveryRequired = false
	m.state.RecoveryTargetAccountID = ""
	m.state.ConsecutiveSwitchFailures = 0
	m.state.LastError = ""
	m.state.UpdatedAt = now
	if err := m.saveLocked(); err != nil {
		return err
	}
	// Clear the ambiguous journal only after the manually confirmed active
	// account has been persisted. If removal fails, the next boot pauses again
	// rather than trusting an uncertain state.
	return m.removeSwitchJournalLocked()
}


func (m *Manager) MarkSwitched(accountID string) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.accountByIDLocked(accountID)
	if a == nil || !a.Enabled {
		return fmt.Errorf("unknown or disabled multi-account profile %q", accountID)
	}
	if m.state.ActiveAccountID != accountID {
		m.state.TotalSwitches++
	}
	now := time.Now()
	m.state.ActiveAccountID = accountID
	m.state.AttacksThisTurn = 0
	m.state.LastSwitchAt = now
	m.state.LastSwitchAttemptAt = now
	m.state.ConsecutiveSwitchFailures = 0
	m.state.LastError = ""
	m.state.RecoveryRequired = false
	m.state.RecoveryTargetAccountID = ""
	m.state.UpdatedAt = now
	return m.saveLocked()
}

func (m *Manager) MarkSwitchFailed(err error) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.state.LastSwitchAttemptAt = now
	m.state.ConsecutiveSwitchFailures++
	if err != nil {
		m.state.LastError = err.Error()
	}
	m.state.UpdatedAt = now
	return m.saveLocked()
}

func (m *Manager) MarkSwitchWarning(err error) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.state.LastError = err.Error()
	}
	m.state.UpdatedAt = time.Now()
	// A warning happens only after the physical account switch has already
	// been verified. Do not increment failure/backoff counters or the bot may
	// later re-click Supercell ID for an account that is already active.
	return m.saveLocked()
}

func (m *Manager) UpdateConfig(cfg config.MultiAccountConfig, currentTag string) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = normalizeConfig(cfg)
	m.reconcileActiveLocked(currentTag)
	m.state.UpdatedAt = time.Now()
	return m.saveLocked()
}

func (m *Manager) accountByIDLocked(id string) *config.ManagedAccount {
	for i := range m.cfg.Accounts {
		if m.cfg.Accounts[i].ID == id {
			return &m.cfg.Accounts[i]
		}
	}
	return nil
}

func (m *Manager) enabledCountLocked() int {
	n := 0
	for _, a := range m.cfg.Accounts {
		if a.Enabled {
			n++
		}
	}
	return n
}

func (m *Manager) switchJournalPathLocked() string {
	if strings.TrimSpace(m.path) == "" {
		return ""
	}
	return m.path + ".switch.json"
}

func (m *Manager) loadSwitchJournalLocked() (switchJournal, bool) {
	path := m.switchJournalPathLocked()
	if path == "" {
		return switchJournal{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return switchJournal{}, false
	}
	var j switchJournal
	if json.Unmarshal(data, &j) != nil || j.ToAccountID == "" || j.Phase == "" {
		return switchJournal{}, false
	}
	return j, true
}

func (m *Manager) saveSwitchJournalLocked(j switchJournal) error {
	path := m.switchJournalPathLocked()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

func (m *Manager) removeSwitchJournalLocked() error {
	path := m.switchJournalPathLocked()
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (m *Manager) saveLocked() error {
	if m.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	_ = os.Remove(m.path)
	return os.Rename(tmp, m.path)
}
