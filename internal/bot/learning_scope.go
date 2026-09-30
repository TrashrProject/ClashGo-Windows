package bot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ducky705/ClashGO/internal/config"
	"github.com/Ducky705/ClashGO/internal/paths"
)

// learningScopeKey isolates adaptive state by Clash account when a player tag
// is linked. The raw tag is never used as a directory name or persisted in the
// learning path. When no tag is available, the scope falls back to the local
// emulator/environment contract; licence-level state archiving still prevents
// one local member profile from inheriting another member's learning folder.
func learningScopeKey(cfg *config.BotConfig) string {
	if cfg == nil {
		return "default"
	}

	tag := strings.ToUpper(strings.TrimSpace(cfg.Account.PlayerTag))
	var seed string
	if tag != "" {
		seed = "account|" + tag
	} else {
		seed = fmt.Sprintf(
			"environment|%s|%s|%dx%d|%d|th%d|%s",
			strings.ToLower(strings.TrimSpace(cfg.Device.DeviceID)),
			strings.ToLower(strings.TrimSpace(cfg.Device.BlueStacksInstance)),
			cfg.Device.Width,
			cfg.Device.Height,
			cfg.Device.DPI,
			cfg.Attack.Farm.TownHall,
			filepath.Base(cfg.Attack.StrategyFile),
		)
	}

	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:10])
}

func learningEnvironmentScopeKey(cfg *config.BotConfig) string {
	if cfg == nil {
		return "default-env"
	}
	seed := fmt.Sprintf(
		"%s|%s|%s|%dx%d|%d",
		learningScopeKey(cfg),
		strings.ToLower(strings.TrimSpace(cfg.Device.DeviceID)),
		strings.ToLower(strings.TrimSpace(cfg.Device.BlueStacksInstance)),
		cfg.Device.Width,
		cfg.Device.Height,
		cfg.Device.DPI,
	)
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:10])
}

func learningAccountStatePath(cfg *config.BotConfig, name string) string {
	return paths.ResolveConfig(filepath.Join("learning", learningScopeKey(cfg), name))
}

func learningEnvironmentStatePath(cfg *config.BotConfig, name string) string {
	return paths.ResolveConfig(filepath.Join(
		"learning",
		learningScopeKey(cfg),
		"environments",
		learningEnvironmentScopeKey(cfg),
		name,
	))
}


func multiAccountStatePath(cfg *config.BotConfig) string {
	if cfg == nil {
		return paths.ResolveConfig(filepath.Join("multi_account", "default", "state.json"))
	}
	seed := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(cfg.Device.DeviceID)),
		strings.ToLower(strings.TrimSpace(cfg.Device.BlueStacksInstance)),
		fmt.Sprintf("%dx%d@%d", cfg.Device.Width, cfg.Device.Height, cfg.Device.DPI),
	}, "|")
	sum := sha256.Sum256([]byte(seed))
	key := hex.EncodeToString(sum[:10])
	return paths.ResolveConfig(filepath.Join("multi_account", key, "state.json"))
}


func accountAttackHistoryPath(cfg *config.BotConfig) string {
	return learningAccountStatePath(cfg, "attack_history.json")
}


// ResolveMultiAccountActivePlayerTag returns the PlayerTag associated with the
// scheduler's last durably verified active account. The scheduler state is
// written only after a physical Supercell-ID switch has completed its loading
// transition and returned to MainVillage, so it is safer at process restart
// than an older per-license single-account snapshot.
func ResolveMultiAccountActivePlayerTag(cfg *config.BotConfig) (string, bool) {
	if cfg == nil || !cfg.Account.MultiAccount.Enabled {
		return "", false
	}
	data, err := os.ReadFile(multiAccountStatePath(cfg))
	if err != nil {
		return "", false
	}
	var state struct {
		ActiveAccountID string `json:"active_account_id"`
	}
	if json.Unmarshal(data, &state) != nil || strings.TrimSpace(state.ActiveAccountID) == "" {
		return "", false
	}
	for _, account := range cfg.Account.MultiAccount.Accounts {
		if !account.Enabled || account.ID != state.ActiveAccountID {
			continue
		}
		tag := strings.ToUpper(strings.TrimSpace(account.PlayerTag))
		if tag == "" {
			return "", false
		}
		if !strings.HasPrefix(tag, "#") {
			tag = "#" + tag
		}
		return tag, true
	}
	return "", false
}
