package bot

import (
	"encoding/json"
	"fmt"
	"math"
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

func (b *Bot) switchMultiAccountIfReady(next config.ManagedAccount) error {
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

	if err := b.tapAccountRect(c, c.SettingsButton, "settings"); err != nil {
		return err
	}
	if _, ok := b.waitAccountState(5*time.Second, func(s game.GameState) bool {
		return s == game.StateSettings
	}); !ok {
		_ = b.client.Back()
		return fmt.Errorf("Settings screen was not confirmed")
	}

	if err := b.tapAccountRect(c, c.SupercellIDButton, "supercell_id"); err != nil {
		_ = b.client.Back()
		return err
	}
	if !b.sleepResponsive(700 * time.Millisecond) {
		return fmt.Errorf("switch cancelled")
	}
	if state, err := b.accountState(2 * time.Second); err != nil || state == game.StateMainVillage {
		_ = b.client.Back()
		return fmt.Errorf("Supercell ID panel was not confirmed")
	}

	if err := b.tapAccountRect(c, c.SwitchAccountButton, "switch_account"); err != nil {
		_ = b.client.Back()
		return err
	}
	if !b.sleepResponsive(650 * time.Millisecond) {
		return fmt.Errorf("switch cancelled")
	}
	if state, err := b.accountState(2 * time.Second); err != nil || state == game.StateMainVillage {
		_ = b.client.Back()
		return fmt.Errorf("account selector was not confirmed")
	}

	if err := b.tapAccountRect(c, slot, "account_slot"); err != nil {
		_ = b.client.Back()
		return err
	}

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
			activationErr := b.applyPreparedManagedAccount(prepared, next)
			// At this point the physical Supercell switch is already proven by a
			// loading transition + MainVillage return. The scheduler MUST advance
			// even if a later disk persistence step is degraded, otherwise the next
			// cycle could attempt to switch the same account a second time.
			if err := b.multiAccount.MarkSwitched(next.ID); err != nil {
				// MarkSwitched mutates the in-memory scheduler before persistence.
				// Never reinterpret this as a physical switch failure.
				b.logger.Error().Err(err).
					Str("account_id", next.ID).
					Msg("account switched physically; scheduler persistence degraded")
				_ = b.multiAccount.MarkSwitchWarning(fmt.Errorf("scheduler persistence degraded after verified switch: %w", err))
			}
			b.wallUpgradePending.Store(b.cfg.Upgrade.UpgradeWalls)
			b.logger.Info().
				Str("account_id", next.ID).
				Str("account_label", next.Label).
				Str("player_tag", next.PlayerTag).
				Msg("multi-account switch verified and activated")
			if activationErr != nil {
				b.logger.Error().Err(activationErr).
					Str("account_id", next.ID).
					Msg("account switch succeeded; local profile persistence degraded")
				_ = b.multiAccount.MarkSwitchWarning(activationErr)
			}
			// Physical switch success is final. Persistence warnings must never
			// bubble up to the caller as switch failures, otherwise the outer
			// backoff path could schedule duplicate Supercell-ID navigation.
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

type preparedManagedAccount struct {
	cfg           config.BotConfig
	adaptive      *intelligence.AdaptiveEngine
	contextual    *intelligence.ContextualEngine
	villageMemory *intelligence.VillageMemory
	armySlot      int
}

func (b *Bot) prepareManagedAccount(next config.ManagedAccount) (*preparedManagedAccount, error) {
	if b == nil || b.cfg == nil {
		return nil, fmt.Errorf("bot configuration unavailable")
	}
	target := *b.cfg

	tag := strings.ToUpper(strings.TrimSpace(next.PlayerTag))
	if tag == "" {
		return nil, fmt.Errorf("account %q has no player tag", next.ID)
	}
	if !strings.HasPrefix(tag, "#") {
		tag = "#" + tag
	}
	target.Account.PlayerTag = tag
	target.Account.MultiAccount.ActiveAccountID = next.ID

	if next.TownHall != 0 {
		if next.TownHall < 8 || next.TownHall > 18 {
			return nil, fmt.Errorf("unsupported town hall %d", next.TownHall)
		}
		if _, ok := target.Attack.Farm.Profiles[strconv.Itoa(next.TownHall)]; !ok {
			return nil, fmt.Errorf("farm profile TH%d is unavailable", next.TownHall)
		}
		target.Attack.Farm.TownHall = next.TownHall
		target.Attack.Farm.Enabled = true
	}

	if raw := strings.TrimSpace(next.StrategyFile); raw != "" {
		name := filepath.Base(raw)
		candidate := paths.Resolve(filepath.Join("strategies", name))
		if info, err := os.Stat(candidate); err != nil || info.IsDir() {
			return nil, fmt.Errorf("strategy %q for account %q is unavailable", name, next.ID)
		}
		target.Attack.StrategyFile = candidate
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
