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

func multiAccountSwitchCalibrationPath() string {
	return paths.ResolveConfig("multi_account_switch.json")
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
	if strings.TrimSpace(next.ID) == "" || !next.Enabled {
		return fmt.Errorf("target multi-account profile is invalid or disabled")
	}
	if next.SwitchSlot <= 0 {
		return fmt.Errorf("account %q has no calibrated switch slot", next.ID)
	}
	if b.recoveryInFlight.Load() || b.restartInFlight.Load() || b.safePacingForced() {
		return fmt.Errorf("runtime safety governor is active")
	}
	if b.client.Health().ConsecutiveFails != 0 {
		return fmt.Errorf("ADB health is not clean enough for an account switch")
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
			if err := b.activateManagedAccount(next); err != nil {
				return err
			}
			if err := b.multiAccount.MarkSwitched(next.ID); err != nil {
				return err
			}
			b.wallUpgradePending.Store(b.cfg.Upgrade.UpgradeWalls)
			b.logger.Info().
				Str("account_id", next.ID).
				Str("account_label", next.Label).
				Str("player_tag", next.PlayerTag).
				Msg("multi-account switch verified and activated")
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

func (b *Bot) activateManagedAccount(next config.ManagedAccount) error {
	if b == nil || b.cfg == nil {
		return fmt.Errorf("bot configuration unavailable")
	}
	oldTag := b.cfg.Account.PlayerTag
	oldID := b.cfg.Account.MultiAccount.ActiveAccountID
	oldTownHall := b.cfg.Attack.Farm.TownHall
	oldStrategy := b.cfg.Attack.StrategyFile

	tag := strings.ToUpper(strings.TrimSpace(next.PlayerTag))
	if tag != "" && !strings.HasPrefix(tag, "#") {
		tag = "#" + tag
	}
	b.cfg.Account.PlayerTag = tag
	b.cfg.Account.MultiAccount.ActiveAccountID = next.ID

	if next.TownHall >= 8 && next.TownHall <= 18 {
		if _, ok := b.cfg.Attack.Farm.Profiles[strconv.Itoa(next.TownHall)]; ok {
			b.cfg.Attack.Farm.TownHall = next.TownHall
			b.cfg.Attack.Farm.Enabled = true
		}
	}
	if raw := strings.TrimSpace(next.StrategyFile); raw != "" {
		name := filepath.Base(raw)
		candidate := paths.Resolve(filepath.Join("strategies", name))
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			b.cfg.Attack.StrategyFile = candidate
		} else {
			b.cfg.Account.PlayerTag = oldTag
			b.cfg.Account.MultiAccount.ActiveAccountID = oldID
			b.cfg.Attack.Farm.TownHall = oldTownHall
			b.cfg.Attack.StrategyFile = oldStrategy
			return fmt.Errorf("strategy %q for account %q is unavailable", name, next.ID)
		}
	}

	if err := config.Save("config.json", b.cfg); err != nil {
		b.cfg.Account.PlayerTag = oldTag
		b.cfg.Account.MultiAccount.ActiveAccountID = oldID
		b.cfg.Attack.Farm.TownHall = oldTownHall
		b.cfg.Attack.StrategyFile = oldStrategy
		return fmt.Errorf("persist active multi-account profile: %w", err)
	}

	if err := b.rebindAccountIntelligence(); err != nil {
		return err
	}
	if strat, err := strategy.ParseYAML(b.cfg.Attack.StrategyFile); err == nil {
		b.armySlot = strat.SelectedArmySlot()
	}
	return nil
}

func (b *Bot) rebindAccountIntelligence() error {
	if b == nil || b.cfg == nil {
		return fmt.Errorf("bot configuration unavailable")
	}

	emulatorKind := "adb"
	if b.cfg.Device.BlueStacksInstance != "" ||
		strings.Contains(strings.ToLower(b.cfg.Device.DeviceID), "localhost") {
		emulatorKind = "bluestacks"
	}

	adaptive, err := intelligence.NewAdaptiveEngine(
		learningAccountStatePath(b.cfg, "adaptive_learning.json"),
		intelligence.EnvironmentFingerprint{
			OS:           runtime.GOOS,
			Emulator:     emulatorKind,
			DeviceID:     b.cfg.Device.DeviceID,
			Width:        b.cfg.Device.Width,
			Height:       b.cfg.Device.Height,
			DPI:          b.cfg.Device.DPI,
			Strategy:     filepath.Base(b.cfg.Attack.StrategyFile),
			TownHall:     b.cfg.Attack.Farm.TownHall,
			AccountScope: learningScopeKey(b.cfg),
		},
	)
	if err != nil {
		return fmt.Errorf("load adaptive intelligence for account: %w", err)
	}
	contextual, err := intelligence.NewContextualEngine(
		learningAccountStatePath(b.cfg, "contextual_learning_v3.json"),
	)
	if err != nil {
		return fmt.Errorf("load contextual intelligence for account: %w", err)
	}
	villageMemory, err := intelligence.NewVillageMemory(
		learningEnvironmentStatePath(b.cfg, "village_model.json"),
	)
	if err != nil {
		return fmt.Errorf("load village memory for account: %w", err)
	}

	b.adaptive = adaptive
	b.contextual = contextual
	b.villageMemory = villageMemory

	b.logger.Info().
		Str("account_scope", learningScopeKey(b.cfg)).
		Int("experiences", contextual.TotalSamples()).
		Int("known_entities", len(villageMemory.Snapshot().Entities)).
		Msg("account-specific intelligence rebound")
	return nil
}
