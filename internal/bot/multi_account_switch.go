package bot

import (
	"encoding/json"
	"fmt"
	"image"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Ducky705/ClashGO/internal/config"
	"github.com/Ducky705/ClashGO/internal/game"
	"github.com/Ducky705/ClashGO/internal/intelligence"
	"github.com/Ducky705/ClashGO/internal/paths"
	"github.com/Ducky705/ClashGO/pkg/strategy"
	"gocv.io/x/gocv"
)

type multiAccountRect struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

func (r multiAccountRect) valid(w, h int) bool {
	return w > 0 && h > 0 &&
		r.X1 >= 0 && r.Y1 >= 0 &&
		r.X2 > r.X1 && r.Y2 > r.Y1 &&
		r.X2 <= w && r.Y2 <= h
}

type multiAccountSwitchCalibration struct {
	Version             int                         `json:"version"`
	Width               int                         `json:"width"`
	Height              int                         `json:"height"`
	SettingsButton      multiAccountRect            `json:"settings_button"`
	SupercellIDButton   multiAccountRect            `json:"supercell_id_button"`
	SwitchAccountButton multiAccountRect            `json:"switch_account_button"`
	AccountSlots        map[string]multiAccountRect `json:"account_slots"`
}

type MultiAccountRect = multiAccountRect
type MultiAccountSwitchCalibration = multiAccountSwitchCalibration


func multiAccountSwitchCalibrationPath() string {
	return paths.ResolveConfig("multi_account_switch.json")
}

func LoadMultiAccountSwitchCalibration() (MultiAccountSwitchCalibration, error) {
	return loadMultiAccountSwitchCalibration()
}

func SaveMultiAccountSwitchCalibration(c MultiAccountSwitchCalibration) error {
	if c.Version <= 0 {
		c.Version = 1
	}
	if c.Width <= 0 || c.Height <= 0 {
		return fmt.Errorf("multi-account calibration has invalid screen dimensions")
	}
	if !c.SettingsButton.valid(c.Width, c.Height) ||
		!c.SupercellIDButton.valid(c.Width, c.Height) ||
		!c.SwitchAccountButton.valid(c.Width, c.Height) {
		return fmt.Errorf("multi-account calibration is incomplete")
	}
	if len(c.AccountSlots) == 0 {
		return fmt.Errorf("multi-account calibration has no account slots")
	}
	for key, rect := range c.AccountSlots {
		if strings.TrimSpace(key) == "" || !rect.valid(c.Width, c.Height) {
			return fmt.Errorf("invalid account slot calibration %q", key)
		}
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := multiAccountSwitchCalibrationPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

func loadMultiAccountSwitchCalibration() (multiAccountSwitchCalibration, error) {
	var c multiAccountSwitchCalibration
	data, err := os.ReadFile(multiAccountSwitchCalibrationPath())
	if err != nil {
		return c, fmt.Errorf("multi-account switch is not calibrated: %w", err)
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("invalid multi-account switch calibration: %w", err)
	}
	if c.Version <= 0 {
		c.Version = 1
	}
	if c.Width <= 0 || c.Height <= 0 {
		return c, fmt.Errorf("multi-account calibration has invalid screen dimensions")
	}
	if !c.SettingsButton.valid(c.Width, c.Height) ||
		!c.SupercellIDButton.valid(c.Width, c.Height) ||
		!c.SwitchAccountButton.valid(c.Width, c.Height) {
		return c, fmt.Errorf("multi-account calibration is incomplete")
	}
	return c, nil
}

func (b *Bot) calibratedAccountCenter(c multiAccountSwitchCalibration, r multiAccountRect) (int, int, error) {
	if b == nil || b.cal == nil {
		return 0, 0, fmt.Errorf("bot calibration unavailable")
	}
	if !r.valid(c.Width, c.Height) {
		return 0, 0, fmt.Errorf("invalid calibrated account rectangle")
	}
	x := float64(r.X1+r.X2) * 0.5
	y := float64(r.Y1+r.Y2) * 0.5
	sx := float64(b.cal.PhysicalW) / float64(c.Width)
	sy := float64(b.cal.PhysicalH) / float64(c.Height)
	px := int(math.Round(x * sx))
	py := int(math.Round(y * sy))
	if px < 0 || py < 0 || px >= b.cal.PhysicalW || py >= b.cal.PhysicalH {
		return 0, 0, fmt.Errorf("scaled account tap is outside screen")
	}
	return px, py, nil
}

func (b *Bot) tapAccountRect(c multiAccountSwitchCalibration, r multiAccountRect, name string) error {
	x, y, err := b.calibratedAccountCenter(c, r)
	if err != nil {
		return err
	}
	if err := b.client.Tap(x, y); err != nil {
		return fmt.Errorf("tap %s: %w", name, err)
	}
	b.logger.Debug().Str("target", name).Int("x", x).Int("y", y).Msg("multi-account calibrated tap")
	return nil
}

func accountVisualHash(frame gocv.Mat) (uint64, error) {
	if frame.Empty() {
		return 0, fmt.Errorf("empty frame")
	}
	gray := gocv.NewMat()
	defer gray.Close()
	if frame.Channels() == 1 {
		frame.CopyTo(&gray)
	} else {
		gocv.CvtColor(frame, &gray, gocv.ColorBGRToGray)
	}
	small := gocv.NewMat()
	defer small.Close()
	gocv.Resize(gray, &small, image.Pt(9, 8), 0, 0, gocv.InterpolationArea)
	if small.Empty() || small.Rows() != 8 || small.Cols() != 9 {
		return 0, fmt.Errorf("could not build visual hash")
	}
	var hash uint64
	var bit uint
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if small.GetUCharAt(y, x) > small.GetUCharAt(y, x+1) {
				hash |= uint64(1) << bit
			}
			bit++
		}
	}
	return hash, nil
}

func (b *Bot) accountSceneFingerprint(timeout time.Duration) (uint64, uint64, error) {
	if b == nil {
		return 0, 0, fmt.Errorf("bot unavailable")
	}
	frame, err := b.runtimeFrameFresh(timeout)
	if err != nil {
		return 0, 0, err
	}
	if frame.Empty() {
		frame.Close()
		return 0, 0, fmt.Errorf("empty runtime frame")
	}
	hash, err := accountVisualHash(frame)
	frame.Close()
	return hash, b.frameSeq.Load(), err
}

func (b *Bot) waitForAccountVisualChange(startSeq, beforeHash uint64, timeout time.Duration) (int, bool) {
	deadline := time.Now().Add(timeout)
	bestDistance := 0
	for time.Now().Before(deadline) {
		if b.ctx.Err() != nil {
			return bestDistance, false
		}
		if b.frameSeq.Load() <= startSeq {
			if !b.sleepResponsive(120 * time.Millisecond) {
				return bestDistance, false
			}
			continue
		}
		afterHash, seq, err := b.accountSceneFingerprint(1200 * time.Millisecond)
		if err == nil && seq > startSeq {
			distance := bits.OnesCount64(beforeHash ^ afterHash)
			if distance > bestDistance {
				bestDistance = distance
			}
			// Eight changed dHash bits is intentionally conservative enough to
			// reject a stale frame while still tolerating small animations.
			if distance >= 8 {
				return distance, true
			}
		}
		if !b.sleepResponsive(150 * time.Millisecond) {
			return bestDistance, false
		}
	}
	return bestDistance, false
}

func (b *Bot) accountState(timeout time.Duration) (game.GameState, error) {
	screen, err := b.runtimeFrameFresh(timeout)
	if err != nil {
		return game.StateUnknown, err
	}
	if screen.Empty() {
		screen.Close()
		return game.StateUnknown, fmt.Errorf("empty runtime frame")
	}
	state, _ := b.classify(screen)
	screen.Close()
	return state, nil
}

func (b *Bot) waitAccountState(timeout time.Duration, want func(game.GameState) bool) (game.GameState, bool) {
	deadline := time.Now().Add(timeout)
	last := game.StateUnknown
	for time.Now().Before(deadline) {
		state, err := b.accountState(2 * time.Second)
		if err != nil {
			if !b.sleepResponsive(250 * time.Millisecond) {
				return last, false
			}
			continue
		}
		last = state
		if want(state) {
			return state, true
		}
		if !b.sleepResponsive(250 * time.Millisecond) {
			return last, false
		}
	}
	return last, false
}

func accountLoadingState(s game.GameState) bool {
	switch s {
	case game.StateLoading, game.StateLogo, game.StateWelcomeBack,
		game.StateTapToContinue, game.StateNewsSplash:
		return true
	default:
		return false
	}
}

func (b *Bot) switchMultiAccountIfReady(next config.ManagedAccount) (retErr error) {
	journalPrepared := false
	switchMayHaveOccurred := false
	journalPhysical := false
	defer func() {
		if journalPrepared && !switchMayHaveOccurred && b != nil && b.multiAccount != nil {
			_ = b.multiAccount.AbortPreparedSwitch(next.ID)
		}
	}()
	if b == nil || b.multiAccount == nil {
		return fmt.Errorf("multi-account scheduler unavailable")
	}
	if !b.multiAccountSwitchInFlight.CompareAndSwap(false, true) {
		return fmt.Errorf("multi-account switch already in progress")
	}
	defer b.multiAccountSwitchInFlight.Store(false)
	if strings.TrimSpace(next.ID) == "" || !next.Enabled {
		return fmt.Errorf("target multi-account profile is invalid or disabled")
	}
	if next.SwitchSlot <= 0 {
		return fmt.Errorf("account %q has no calibrated switch slot", next.ID)
	}
	if b.recoveryInFlight.Load() || b.restartInFlight.Load() || b.safePacingForced() {
		return fmt.Errorf("runtime safety governor is active")
	}
	if b.wallUpgradePending.Load() {
		return fmt.Errorf("pending wall upgrades must finish before account rotation")
	}
	if b.client.Health().ConsecutiveFails != 0 {
		return fmt.Errorf("ADB health is not clean enough for an account switch")
	}

	prepared, err := b.prepareManagedAccount(next)
	if err != nil {
		return fmt.Errorf("target account preflight failed: %w", err)
	}

	c, err := loadMultiAccountSwitchCalibration()
	if err != nil {
		return err
	}
	slot, ok := c.AccountSlots[strconv.Itoa(next.SwitchSlot)]
	if !ok || !slot.valid(c.Width, c.Height) {
		return fmt.Errorf("switch slot %d is not calibrated", next.SwitchSlot)
	}

	state, err := b.accountState(2 * time.Second)
	if err != nil {
		return fmt.Errorf("pre-switch frame: %w", err)
	}
	if state != game.StateMainVillage {
		return fmt.Errorf("account switch requires MainVillage, got %s", state.String())
	}
	if err := b.multiAccount.BeginSwitch(next.ID); err != nil {
		return fmt.Errorf("persist switch journal: %w", err)
	}
	journalPrepared = true

	if err := b.tapAccountRect(c, c.SettingsButton, "settings"); err != nil {
		return err
	}
	if _, ok := b.waitAccountState(5*time.Second, func(s game.GameState) bool {
		return s == game.StateSettings
	}); !ok {
		_ = b.client.Back()
		return fmt.Errorf("Settings screen was not confirmed")
	}

	settingsHash, settingsSeq, err := b.accountSceneFingerprint(2 * time.Second)
	if err != nil {
		_ = b.client.Back()
		return fmt.Errorf("fingerprint Settings screen: %w", err)
	}
	if err := b.tapAccountRect(c, c.SupercellIDButton, "supercell_id"); err != nil {
		_ = b.client.Back()
		return err
	}
	if distance, ok := b.waitForAccountVisualChange(settingsSeq, settingsHash, 5*time.Second); !ok {
		_ = b.client.Back()
		return fmt.Errorf("Supercell ID panel visual transition was not confirmed (distance=%d)", distance)
	}
	if state, err := b.accountState(2 * time.Second); err != nil || state == game.StateMainVillage {
		_ = b.client.Back()
		return fmt.Errorf("Supercell ID panel was not confirmed")
	}

	supercellHash, supercellSeq, err := b.accountSceneFingerprint(2 * time.Second)
	if err != nil {
		_ = b.client.Back()
		return fmt.Errorf("fingerprint Supercell ID panel: %w", err)
	}
	if err := b.tapAccountRect(c, c.SwitchAccountButton, "switch_account"); err != nil {
		_ = b.client.Back()
		return err
	}
	if distance, ok := b.waitForAccountVisualChange(supercellSeq, supercellHash, 5*time.Second); !ok {
		_ = b.client.Back()
		return fmt.Errorf("account selector visual transition was not confirmed (distance=%d)", distance)
	}
	if state, err := b.accountState(2 * time.Second); err != nil || state == game.StateMainVillage {
		_ = b.client.Back()
		return fmt.Errorf("account selector was not confirmed")
	}

	if err := b.tapAccountRect(c, slot, "account_slot"); err != nil {
		_ = b.client.Back()
		return err
	}
	// From this point on the physical account may already be changing. If the
	// process stops before MainVillage confirmation, keep the prepared journal
	// so startup enters recovery-required mode instead of guessing.
	switchMayHaveOccurred = true

	// Never mark a switch from a tap alone. Require a genuine game loading
	// state before accepting the target slot, otherwise a stale/moved selector
	// could silently bind the wrong Clash account.
	if _, ok := b.waitAccountState(10*time.Second, accountLoadingState); !ok {
		_ = b.client.Back()
		return fmt.Errorf("no Clash loading transition observed after account slot tap")
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		state, err := b.accountState(3 * time.Second)
		if err != nil {
			if !b.sleepResponsive(500 * time.Millisecond) {
				return fmt.Errorf("switch cancelled")
			}
			continue
		}
		if state == game.StateMainVillage {
			if err := b.multiAccount.MarkPhysicalSwitch(next.ID); err != nil {
				b.logger.Error().Err(err).
					Str("account_id", next.ID).
					Msg("physical switch verified but journal phase could not be persisted")
			} else {
				journalPhysical = true
			}

			// Persist scheduler state before config. Both are redundant records of
			// the same verified physical switch; if either write degrades, the
			// physical journal remains so the next boot can recover the target.
			schedulerErr := b.multiAccount.MarkSwitched(next.ID)
			activationErr := b.applyPreparedManagedAccount(prepared, next)

			b.wallUpgradePending.Store(b.cfg.Upgrade.UpgradeWalls)
			b.logger.Info().
				Str("account_id", next.ID).
				Str("account_label", next.Label).
				Str("player_tag", next.PlayerTag).
				Msg("multi-account switch verified and activated")

			if schedulerErr != nil {
				b.logger.Error().Err(schedulerErr).
					Str("account_id", next.ID).
					Msg("account switched physically; scheduler persistence degraded")
				_ = b.multiAccount.MarkSwitchWarning(fmt.Errorf("scheduler persistence degraded after verified switch: %w", schedulerErr))
			}
			if activationErr != nil {
				b.logger.Error().Err(activationErr).
					Str("account_id", next.ID).
					Msg("account switch succeeded; local profile persistence degraded")
				_ = b.multiAccount.MarkSwitchWarning(activationErr)
			}

			if schedulerErr == nil && activationErr == nil {
				if !journalPhysical {
					// One retry after both durable records succeeded. If this still
					// fails, leaving the prepared journal is safer than deleting it.
					if err := b.multiAccount.MarkPhysicalSwitch(next.ID); err == nil {
						journalPhysical = true
					}
				}
				if journalPhysical {
					if err := b.multiAccount.CompleteSwitch(next.ID); err != nil {
						b.logger.Warn().Err(err).Msg("could not clear completed account switch journal")
					}
				}
			}
			// A verified physical switch is final. Persistence degradation is a
			// warning, not a reason to navigate Supercell ID again.
			return nil
		}
		switch state {
		case game.StateWelcomeBack, game.StateTapToContinue, game.StateNewsSplash,
			game.StateConnectionLost, game.StateConfirmExit:
			b.dismissInterruptionState(state)
		}
		if !b.sleepResponsive(400 * time.Millisecond) {
			return fmt.Errorf("switch cancelled")
		}
	}
	return fmt.Errorf("new account did not reach MainVillage within switch timeout")
}

func (b *Bot) ResolveMultiAccountRecovery(accountID string) (config.ManagedAccount, error) {
	if b == nil || b.multiAccount == nil {
		return config.ManagedAccount{}, fmt.Errorf("multi-account scheduler unavailable")
	}
	recoveryRequired, _ := b.multiAccount.RecoveryStatus()
	if !recoveryRequired {
		return config.ManagedAccount{}, fmt.Errorf("no interrupted account switch requires confirmation")
	}
	account, ok := b.multiAccount.Account(accountID)
	if !ok {
		return config.ManagedAccount{}, fmt.Errorf("unknown or disabled multi-account profile %q", accountID)
	}
	prepared, err := b.prepareManagedAccount(account)
	if err != nil {
		return config.ManagedAccount{}, fmt.Errorf("confirmed account preflight failed: %w", err)
	}

	// Rebind the in-memory config and all account-scoped intelligence before
	// clearing recovery. If persistence fails here the scheduler remains
	// recovery-blocked, so no attack can run with stale account state.
	if err := b.applyPreparedManagedAccount(prepared, account); err != nil {
		return config.ManagedAccount{}, err
	}
	if err := b.multiAccount.ResolveRecovery(account.ID); err != nil {
		return config.ManagedAccount{}, fmt.Errorf("resolve interrupted account identity: %w", err)
	}
	b.wallUpgradePending.Store(b.cfg.Upgrade.UpgradeWalls)
	b.logger.Info().
		Str("account_id", account.ID).
		Str("account_label", account.Label).
		Str("player_tag", account.PlayerTag).
		Msg("multi-account identity manually confirmed; automation remains paused")
	return account, nil
}

type preparedManagedAccount struct {
	cfg           config.BotConfig
	adaptive      *intelligence.AdaptiveEngine
	contextual    *intelligence.ContextualEngine
	villageMemory *intelligence.VillageMemory
	armySlot      int
}

func applyManagedAccountConfig(target *config.BotConfig, next config.ManagedAccount) error {
	if target == nil {
		return fmt.Errorf("target bot configuration unavailable")
	}
	tag := strings.ToUpper(strings.TrimSpace(next.PlayerTag))
	if tag == "" {
		return fmt.Errorf("account %q has no player tag", next.ID)
	}
	if !strings.HasPrefix(tag, "#") {
		tag = "#" + tag
	}
	target.Account.PlayerTag = tag
	target.Account.MultiAccount.ActiveAccountID = next.ID

	if next.TownHall != 0 {
		if next.TownHall < 8 || next.TownHall > 18 {
			return fmt.Errorf("unsupported town hall %d", next.TownHall)
		}
		if _, ok := target.Attack.Farm.Profiles[strconv.Itoa(next.TownHall)]; !ok {
			return fmt.Errorf("farm profile TH%d is unavailable", next.TownHall)
		}
		target.Attack.Farm.TownHall = next.TownHall
		target.Attack.Farm.Enabled = true
	}

	if raw := strings.TrimSpace(next.StrategyFile); raw != "" {
		name := filepath.Base(raw)
		candidate := paths.Resolve(filepath.Join("strategies", name))
		if info, err := os.Stat(candidate); err != nil || info.IsDir() {
			return fmt.Errorf("strategy %q for account %q is unavailable", name, next.ID)
		}
		target.Attack.StrategyFile = candidate
	}
	return nil
}

func (b *Bot) prepareManagedAccount(next config.ManagedAccount) (*preparedManagedAccount, error) {
	if b == nil || b.cfg == nil {
		return nil, fmt.Errorf("bot configuration unavailable")
	}
	target := *b.cfg
	if err := applyManagedAccountConfig(&target, next); err != nil {
		return nil, err
	}

	strat, err := strategy.ParseYAML(target.Attack.StrategyFile)
	if err != nil {
		return nil, fmt.Errorf("strategy preflight: %w", err)
	}

	emulatorKind := "adb"
	if target.Device.BlueStacksInstance != "" ||
		strings.Contains(strings.ToLower(target.Device.DeviceID), "localhost") {
		emulatorKind = "bluestacks"
	}
	adaptive, err := intelligence.NewAdaptiveEngine(
		learningAccountStatePath(&target, "adaptive_learning.json"),
		intelligence.EnvironmentFingerprint{
			OS:           runtime.GOOS,
			Emulator:     emulatorKind,
			DeviceID:     target.Device.DeviceID,
			Width:        target.Device.Width,
			Height:       target.Device.Height,
			DPI:          target.Device.DPI,
			Strategy:     filepath.Base(target.Attack.StrategyFile),
			TownHall:     target.Attack.Farm.TownHall,
			AccountScope: learningScopeKey(&target),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("load adaptive intelligence: %w", err)
	}
	contextual, err := intelligence.NewContextualEngine(
		learningAccountStatePath(&target, "contextual_learning_v3.json"),
	)
	if err != nil {
		return nil, fmt.Errorf("load contextual intelligence: %w", err)
	}
	villageMemory, err := intelligence.NewVillageMemory(
		learningEnvironmentStatePath(&target, "village_model.json"),
	)
	if err != nil {
		return nil, fmt.Errorf("load village memory: %w", err)
	}

	return &preparedManagedAccount{
		cfg:           target,
		adaptive:      adaptive,
		contextual:    contextual,
		villageMemory: villageMemory,
		armySlot:      strat.SelectedArmySlot(),
	}, nil
}

func managedAccountByID(cfg config.MultiAccountConfig, accountID string) (config.ManagedAccount, bool) {
	accountID = strings.TrimSpace(accountID)
	for _, account := range cfg.Accounts {
		if account.ID == accountID && account.Enabled {
			return account, true
		}
	}
	return config.ManagedAccount{}, false
}

func (b *Bot) ResolveMultiAccountRecovery(accountID string) error {
	if b == nil || b.multiAccount == nil || b.cfg == nil {
		return fmt.Errorf("multi-account recovery is unavailable")
	}
	required, _ := b.multiAccount.RecoveryStatus()
	if !required {
		return nil
	}
	account, ok := managedAccountByID(b.cfg.Account.MultiAccount, accountID)
	if !ok {
		return fmt.Errorf("unknown or disabled multi-account profile %q", accountID)
	}
	prepared, err := b.prepareManagedAccount(account)
	if err != nil {
		return fmt.Errorf("prepare confirmed account %q: %w", accountID, err)
	}
	if err := b.applyPreparedManagedAccount(prepared, account); err != nil {
		// Keep recovery_required + journal intact. Clearing it when config/IA
		// persistence failed would remove our only durable ambiguity guard.
		return fmt.Errorf("apply confirmed account %q: %w", accountID, err)
	}
	if err := b.multiAccount.ResolveRecovery(account.ID); err != nil {
		return fmt.Errorf("clear account recovery state: %w", err)
	}
	b.wallUpgradePending.Store(b.cfg.Upgrade.UpgradeWalls)
	b.paused.Store(false)
	b.recordActivity()
	b.logger.Info().
		Str("account_id", account.ID).
		Str("player_tag", account.PlayerTag).
		Msg("multi-account recovery resolved; correct IA rebound")
	return nil
}

func ResolveMultiAccountRecoveryOffline(cfg *config.BotConfig, accountID string) error {
	if cfg == nil {
		return fmt.Errorf("bot configuration unavailable")
	}
	account, ok := managedAccountByID(cfg.Account.MultiAccount, accountID)
	if !ok {
		return fmt.Errorf("unknown or disabled multi-account profile %q", accountID)
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
		return nil
	}
	if err := applyManagedAccountConfig(cfg, account); err != nil {
		return err
	}
	if err := config.Save("config.json", cfg); err != nil {
		return err
	}
	return manager.ResolveRecovery(account.ID)
}

func (b *Bot) applyPreparedManagedAccount(prepared *preparedManagedAccount, next config.ManagedAccount) error {
	if b == nil || b.cfg == nil || prepared == nil {
		return fmt.Errorf("prepared account runtime unavailable")
	}

	// Copy into the existing config object instead of replacing b.cfg. The
	// attack executor holds a pointer to b.cfg.Attack; preserving the parent
	// object address keeps that pointer valid while applying the new profile.
	*b.cfg = prepared.cfg
	b.adaptive = prepared.adaptive
	b.contextual = prepared.contextual
	b.villageMemory = prepared.villageMemory
	b.armySlot = prepared.armySlot

	b.logger.Info().
		Str("account_scope", learningScopeKey(b.cfg)).
		Int("experiences", prepared.contextual.TotalSamples()).
		Int("known_entities", len(prepared.villageMemory.Snapshot().Entities)).
		Msg("account-specific intelligence rebound atomically")

	if err := config.Save("config.json", b.cfg); err != nil {
		// The physical switch and in-memory isolation are already correct.
		// Surface persistence degradation but do NOT roll back to the previous
		// account/AI, which would cross-contaminate the newly active account.
		return fmt.Errorf("account switched but config persistence failed: %w", err)
	}
	return nil
}
