package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Ducky705/ClashGO/internal/config"
	"github.com/Ducky705/ClashGO/internal/multiaccount"
	"github.com/Ducky705/ClashGO/internal/paths"
)

func applyManagedAccountIdentity(cfg *config.BotConfig, account config.ManagedAccount) error {
	if cfg == nil {
		return fmt.Errorf("nil bot configuration")
	}
	if !account.Enabled {
		return fmt.Errorf("multi-account profile %q is disabled", account.ID)
	}

	tag := strings.ToUpper(strings.TrimSpace(account.PlayerTag))
	if tag == "" {
		return fmt.Errorf("multi-account profile %q has no player tag", account.ID)
	}
	if !strings.HasPrefix(tag, "#") {
		tag = "#" + tag
	}
	cfg.Account.PlayerTag = tag
	cfg.Account.MultiAccount.ActiveAccountID = account.ID

	if account.TownHall != 0 {
		if account.TownHall < 8 || account.TownHall > 18 {
			return fmt.Errorf("unsupported town hall %d for account %q", account.TownHall, account.ID)
		}
		if _, ok := cfg.Attack.Farm.Profiles[strconv.Itoa(account.TownHall)]; !ok {
			return fmt.Errorf("farm profile TH%d is unavailable for account %q", account.TownHall, account.ID)
		}
		cfg.Attack.Farm.TownHall = account.TownHall
		cfg.Attack.Farm.Enabled = true
	}

	if raw := strings.TrimSpace(account.StrategyFile); raw != "" {
		name := filepath.Base(filepath.Clean(raw))
		candidate := paths.Resolve(filepath.Join("strategies", name))
		if info, err := os.Stat(candidate); err != nil || info.IsDir() {
			return fmt.Errorf("strategy %q for account %q is unavailable", name, account.ID)
		}
		cfg.Attack.StrategyFile = candidate
	}
	return nil
}

// recoverVerifiedMultiAccountSwitch runs during Bot construction, before any
// account-scoped intelligence is opened. A physical_switched journal is proof
// that Clash reached the target village, so the config must be rebound to that
// account before Adaptive/V3/VillageMemory are loaded.
func recoverVerifiedMultiAccountSwitch(cfg *config.BotConfig, manager *multiaccount.Manager) error {
	if cfg == nil || manager == nil {
		return nil
	}
	required, targetID := manager.RecoveryStatus()
	if required || strings.TrimSpace(targetID) == "" {
		return nil
	}
	active, ok := manager.Active()
	if !ok || active.ID != targetID {
		return fmt.Errorf("verified account-switch journal targets %q but scheduler active account is inconsistent", targetID)
	}
	if err := applyManagedAccountIdentity(cfg, active); err != nil {
		return err
	}
	if err := config.Save("config.json", cfg); err != nil {
		// Keep the physical journal in place. The current process still uses the
		// correct in-memory account, and a future boot retries persistence.
		return fmt.Errorf("persist recovered multi-account identity: %w", err)
	}
	if err := manager.CompleteSwitch(active.ID); err != nil {
		return fmt.Errorf("clear recovered multi-account journal: %w", err)
	}
	return nil
}

// LoadMultiAccountRuntimeStatus is the stopped-bot equivalent of
// Bot.MultiAccountStatus. It intentionally opens the scheduler state so the UI
// can surface an interrupted switch even before automation starts.
func LoadMultiAccountRuntimeStatus(cfg *config.BotConfig) MultiAccountRuntimeStatus {
	if cfg == nil {
		return MultiAccountRuntimeStatus{}
	}
	manager, err := multiaccount.NewManager(
		multiAccountStatePath(cfg),
		cfg.Account.MultiAccount,
		cfg.Account.PlayerTag,
	)
	if err != nil {
		return MultiAccountRuntimeStatus{
			Enabled:         cfg.Account.MultiAccount.Enabled,
			ActiveAccountID: cfg.Account.MultiAccount.ActiveAccountID,
			LastError:       err.Error(),
		}
	}
	return multiAccountRuntimeStatus(manager, false)
}

func multiAccountRuntimeStatus(manager *multiaccount.Manager, switchInFlight bool) MultiAccountRuntimeStatus {
	if manager == nil {
		return MultiAccountRuntimeStatus{}
	}
	st := manager.State()
	recoveryRequired, recoveryTarget := manager.RecoveryStatus()
	out := MultiAccountRuntimeStatus{
		Enabled:          manager.Enabled(),
		ActiveAccountID:  st.ActiveAccountID,
		AttacksThisTurn:  st.AttacksThisTurn,
		TotalSwitches:    st.TotalSwitches,
		LastSwitchAt:     st.LastSwitchAt,
		LastError:        st.LastError,
		RecoveryRequired: recoveryRequired,
		RecoveryTargetID: recoveryTarget,
		SwitchInFlight:   switchInFlight,
	}
	if active, ok := manager.Active(); ok {
		out.ActiveAccountLabel = active.Label
	}
	if next, due := manager.NextDue(); due {
		out.RotationDue = true
		out.NextAccountID = next.ID
		out.NextAccountLabel = next.Label
	}
	return out
}

// ResolveMultiAccountRecovery is deliberately explicit: after a crash during
// an unverified switch, a human confirms which Clash account is visibly open.
// Only then do we clear the journal and permit unattended farming again.
func ResolveMultiAccountRecovery(cfg *config.BotConfig, accountID string) error {
	if cfg == nil {
		return fmt.Errorf("nil bot configuration")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return fmt.Errorf("account id is required")
	}

	manager, err := multiaccount.NewManager(
		multiAccountStatePath(cfg),
		cfg.Account.MultiAccount,
		cfg.Account.PlayerTag,
	)
	if err != nil {
		return err
	}
	required, _ := manager.RecoveryStatus()
	if !required {
		return fmt.Errorf("multi-account recovery is not required")
	}

	var selected *config.ManagedAccount
	for i := range cfg.Account.MultiAccount.Accounts {
		account := &cfg.Account.MultiAccount.Accounts[i]
		if account.ID == accountID && account.Enabled {
			selected = account
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("unknown or disabled multi-account profile %q", accountID)
	}

	if err := applyManagedAccountIdentity(cfg, *selected); err != nil {
		return err
	}
	if err := config.Save("config.json", cfg); err != nil {
		return fmt.Errorf("persist confirmed account identity: %w", err)
	}
	if err := manager.ResolveRecovery(accountID); err != nil {
		return fmt.Errorf("clear multi-account recovery state: %w", err)
	}
	return nil
}
