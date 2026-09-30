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
	LastSwitchAt    time.Time `json:"last_switch_at,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Manager struct {
	mu    sync.Mutex
	path  string
	cfg   config.MultiAccountConfig
	state State
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
	if m.accountByIDLocked(m.state.ActiveAccountID) != nil {
		return
	}
	if m.cfg.ActiveAccountID != "" && m.accountByIDLocked(m.cfg.ActiveAccountID) != nil {
		m.state.ActiveAccountID = m.cfg.ActiveAccountID
		return
	}
	for i := range m.cfg.Accounts {
		a := &m.cfg.Accounts[i]
		if a.Enabled && currentTag != "" && normalizeTag(a.PlayerTag) == currentTag {
			m.state.ActiveAccountID = a.ID
			return
		}
	}
	for i := range m.cfg.Accounts {
		if m.cfg.Accounts[i].Enabled {
			m.state.ActiveAccountID = m.cfg.Accounts[i].ID
			return
		}
	}
	m.state.ActiveAccountID = ""
}

func (m *Manager) Enabled() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg.Enabled && m.enabledCountLocked() >= 2
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
	if !m.cfg.Enabled || m.enabledCountLocked() < 2 {
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
	m.state.ActiveAccountID = accountID
	m.state.AttacksThisTurn = 0
	m.state.LastSwitchAt = time.Now()
	m.state.LastError = ""
	m.state.UpdatedAt = time.Now()
	return m.saveLocked()
}

func (m *Manager) MarkSwitchFailed(err error) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.state.LastError = err.Error()
	}
	m.state.UpdatedAt = time.Now()
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
