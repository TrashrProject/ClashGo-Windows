package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ducky705/ClashGO/internal/paths"
)

type BotConfig struct {
	Device     DeviceConfig     `json:"device"`
	Training   TrainingConfig   `json:"training"`
	Attack     AttackConfig     `json:"attack"`
	Search     SearchConfig     `json:"search"`
	Upgrade    UpgradeConfig    `json:"upgrade"`
	Debug      DebugConfig      `json:"debug"`
	Account    AccountConfig    `json:"account"`
	Automation AutomationConfig `json:"automation"`
}

type SimplePreferences struct {
	// Keep the beginner surface intentionally small. These are player-facing
	// choices; technical CV/ADB thresholds stay internal.
	AutoDonate          bool   `json:"auto_donate"`
	DonateOnlyRequested bool   `json:"donate_only_requested"`
	UseHeroes           bool   `json:"use_heroes"`
	UseClanCastle       bool   `json:"use_clan_castle"`
	WaitForFullArmy     bool   `json:"wait_for_full_army"`
	AutoRetrain         bool   `json:"auto_retrain"`
	AutoUpgradeWalls    bool   `json:"auto_upgrade_walls"`
	LootPreset          string `json:"loot_preset"` // relaxed | balanced | rich
}

type AutomationConfig struct {
	// SimpleMode is the default user experience: ClashGO derives sane values
	// from the linked account and only exposes a few meaningful controls.
	SimpleMode bool `json:"simple_mode"`

	// AutoFarmProfile keeps the selected farm HDV aligned with the linked
	// Clash account after every successful account sync.
	AutoFarmProfile bool `json:"auto_farm_profile"`

	// AutoArmyGuard blocks/repairs attack flow when the detected deployment
	// state disagrees with the target farm composition.
	AutoArmyGuard bool `json:"auto_army_guard"`

	// AutoResourceTracking enables low-rate village resource snapshots.
	AutoResourceTracking bool `json:"auto_resource_tracking"`

	// AutoProfileSync keeps account data fresh without manual Sync clicks.
	AutoProfileSync bool `json:"auto_profile_sync"`

	// Preferences contains the tiny set of choices exposed in Easy Mode.
	Preferences SimplePreferences `json:"preferences"`
}

type AccountConfig struct {
	PlayerTag string `json:"player_tag"`
	// ProxyURL points to the ClashGO account service. End users never need
	// a Clash developer key; the server owns that credential.
	ProxyURL string `json:"proxy_url,omitempty"`
	// LegacyAPIKey is kept only so older config.json files still unmarshal.
	// The desktop app no longer uses or exposes it.
	LegacyAPIKey string `json:"api_key,omitempty"`
}

type DeviceConfig struct {
	ADBHost               string `json:"adb_host"`
	ADBPort               int    `json:"adb_port"`
	DeviceID              string `json:"device_id"`
	BlueStacksInstance    string `json:"bluestacks_instance,omitempty"`
	PackageName           string `json:"package_name"`
	ZoomOutKey            string `json:"zoom_out_key"` // Key to press for zoom out (e.g., "-")
	ZoomInKey             string `json:"zoom_in_key"`  // Key to press for zoom in (e.g., "+")
	Width                 int    `json:"width"`
	Height                int    `json:"height"`
	DPI                   int    `json:"dpi"`
	RestartOnStartup      bool   `json:"restart_on_startup"`
	DisableChestDismissal bool   `json:"disable_chest_dismissal"`
}

type TrainingConfig struct {
	Enabled              bool     `json:"enabled"`
	FullArmyBeforeAttack bool     `json:"full_army_before_attack"`
	TrainDeadTroops      bool     `json:"train_dead_troops"`
	MinBarracksLevel     int      `json:"min_barracks_level"`
	SleepAfterTrain      Duration `json:"sleep_after_train"`
}

type AttackConfig struct {
	Enabled             bool     `json:"enabled"`
	StrategyFile        string   `json:"strategy_file"`
	AttackWhenFull      bool     `json:"attack_when_full"`
	MaxAttackPerSession int      `json:"max_attack_per_session"`
	DropDelay           Duration `json:"drop_delay"`
	SpellDelay          Duration `json:"spell_delay"`
	EndBattleDelay      Duration `json:"end_battle_delay"`
	UseHeroes           bool     `json:"use_heroes"`
	UseQueen            bool     `json:"use_queen"`
	UseWarden           bool     `json:"use_warden"`
	UseClanCastle       bool     `json:"use_clan_castle"`
	QueenChargeAtPct    int      `json:"queen_charge_at_pct"`
	WardenUseAtPct      int      `json:"warden_use_at_pct"`
	ReserveDEPercent    int      `json:"reserve_de_percent"`
	StallTimerSeconds   int      `json:"stall_timer_seconds"`
	LootExitEnabled     bool     `json:"loot_exit_enabled"`
	LootExitPercent     int      `json:"loot_exit_percent"`
	// MinSecondsBetweenAttacks is a short UI/recovery cooldown after Return
	// Home. Modern Clash applies army recipes instantly, so this is no longer
	// a troop-training timer. 0 disables the pause.
	MinSecondsBetweenAttacks int `json:"min_seconds_between_attacks"`

	// FarmComposition gives the Windows live deployer a deterministic army
	// contract instead of making troop quantity/hero decisions from OCR alone.
	Farm FarmConfig `json:"farm"`
}

type FarmUnit struct {
	Name    string `json:"name"`
	Count   int    `json:"count"`
	Housing int    `json:"housing"`
}

type FarmProfile struct {
	TownHall                int        `json:"town_hall"`
	Label                   string     `json:"label"`
	TroopCapacity           int        `json:"troop_capacity"`
	SpellCapacity           int        `json:"spell_capacity"`
	ClanCastleTroopCapacity int        `json:"clan_castle_troop_capacity"`
	ClanCastleSpellCapacity int        `json:"clan_castle_spell_capacity"`
	ClanCastleSiegeCapacity int        `json:"clan_castle_siege_capacity"`
	Troops                  []FarmUnit `json:"troops"`
	Spells                  []FarmUnit `json:"spells"`
	Heroes                  []string   `json:"heroes"`
	Siege                   string     `json:"siege"`
}

type FarmConfig struct {
	Enabled  bool                   `json:"enabled"`
	TownHall int                    `json:"town_hall"`
	Profiles map[string]FarmProfile `json:"profiles"`
}

// ActiveProfile returns the selected TH profile.
func (f FarmConfig) ActiveProfile() (FarmProfile, bool) {
	if !f.Enabled {
		return FarmProfile{}, false
	}
	p, ok := f.Profiles[fmt.Sprintf("%d", f.TownHall)]
	return p, ok
}

// DesiredCount returns the configured amount for a named troop/spell.
func (p FarmProfile) DesiredCount(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, u := range append(append([]FarmUnit{}, p.Troops...), p.Spells...) {
		if strings.EqualFold(strings.TrimSpace(u.Name), name) {
			return u.Count
		}
	}
	return 0
}

func (p FarmProfile) UsesHero(name string) bool {
	for _, h := range p.Heroes {
		if strings.EqualFold(strings.TrimSpace(h), strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

func defaultFarmProfiles() map[string]FarmProfile {
	makeP := func(th, troopCap, spellCap, ccTroop, ccSpell, ccSiege int, troops, spells []FarmUnit, heroes []string, siege string) FarmProfile {
		return FarmProfile{
			TownHall: th, Label: fmt.Sprintf("HDV %d - Farm Air", th),
			TroopCapacity: troopCap, SpellCapacity: spellCap,
			ClanCastleTroopCapacity: ccTroop, ClanCastleSpellCapacity: ccSpell, ClanCastleSiegeCapacity: ccSiege,
			Troops: troops, Spells: spells, Heroes: heroes, Siege: siege,
		}
	}
	rage := func(n int) []FarmUnit { return []FarmUnit{{Name:"Rage Spell", Count:n, Housing:2}, {Name:"Ice Spell", Count:1, Housing:1}} }
	return map[string]FarmProfile{
		"8":  makeP(8, 200, 7, 25, 1, 0, []FarmUnit{{Name:"Balloon", Count:40, Housing:5}}, rage(3), []string{"Barbarian King","Archer Queen"}, ""),
		"9":  makeP(9, 220, 9, 30, 1, 0, []FarmUnit{{Name:"Balloon", Count:44, Housing:5}}, rage(4), []string{"Barbarian King","Archer Queen","Minion Prince"}, ""),
		"10": makeP(10, 240, 11, 35, 1, 1, []FarmUnit{{Name:"Balloon", Count:48, Housing:5}}, rage(5), []string{"Barbarian King","Archer Queen","Minion Prince"}, "Stone Slammer"),
		"11": makeP(11, 260, 11, 35, 2, 1, []FarmUnit{{Name:"Electro Dragon", Count:8, Housing:30},{Name:"Balloon", Count:4, Housing:5}}, rage(5), []string{"Barbarian King","Archer Queen","Minion Prince","Grand Warden"}, "Stone Slammer"),
		"12": makeP(12, 280, 11, 40, 2, 1, []FarmUnit{{Name:"Electro Dragon", Count:9, Housing:30},{Name:"Balloon", Count:2, Housing:5}}, rage(5), []string{"Barbarian King","Archer Queen","Minion Prince","Grand Warden"}, "Stone Slammer"),
		"13": makeP(13, 300, 11, 45, 2, 1, []FarmUnit{{Name:"Electro Dragon", Count:10, Housing:30}}, rage(5), []string{"Barbarian King","Archer Queen","Grand Warden","Royal Champion"}, "Stone Slammer"),
		"14": makeP(14, 300, 11, 45, 3, 1, []FarmUnit{{Name:"Electro Dragon", Count:10, Housing:30}}, rage(5), []string{"Barbarian King","Archer Queen","Grand Warden","Royal Champion"}, "Stone Slammer"),
		"15": makeP(15, 320, 11, 50, 3, 1, []FarmUnit{{Name:"Electro Dragon", Count:10, Housing:30},{Name:"Balloon", Count:4, Housing:5}}, rage(5), []string{"Barbarian King","Archer Queen","Grand Warden","Royal Champion"}, "Stone Slammer"),
		"16": makeP(16, 320, 11, 50, 3, 2, []FarmUnit{{Name:"Electro Dragon", Count:10, Housing:30},{Name:"Balloon", Count:4, Housing:5}}, rage(5), []string{"Barbarian King","Archer Queen","Grand Warden","Royal Champion"}, "Stone Slammer"),
		"17": makeP(17, 340, 11, 55, 3, 2, []FarmUnit{{Name:"Electro Dragon", Count:11, Housing:30},{Name:"Balloon", Count:2, Housing:5}}, rage(5), []string{"Barbarian King","Archer Queen","Grand Warden","Royal Champion"}, "Stone Slammer"),
		"18": makeP(18, 352, 11, 55, 4, 2, []FarmUnit{{Name:"Electro Dragon", Count:11, Housing:30},{Name:"Balloon", Count:4, Housing:5}}, rage(5), []string{"Barbarian King","Archer Queen","Grand Warden","Royal Champion"}, "Stone Slammer"),
	}
}

type SearchConfig struct {
	Enabled              bool `json:"enabled"`
	// MaxSkipsBeforeForceAttack mirrors the proven "force attack after N
	// searches" escape hatch used by mature CoC automation. 0 disables it.
	// A high default keeps normal loot filtering intact while guaranteeing
	// the bot cannot spend an entire unattended session cycling bases forever.
	MaxSkipsBeforeForceAttack int `json:"max_skips_before_force_attack"`
	MinTrophies          int  `json:"min_trophies"`
	MaxTrophies          int  `json:"max_trophies"`
	MinTownHall          int  `json:"min_town_hall"`
	MaxTownHall          int  `json:"max_town_hall"`
	SkipBigBase          bool `json:"skip_big_base"`
	SkipMaxTH            bool `json:"skip_max_th"`
	AttackIfDarkElixirGT int  `json:"attack_if_de_gt"`
	AttackIfTrophiesGT   int  `json:"attack_if_trophies_gt"`
	MinLootGold          int  `json:"min_loot_gold"`
	MinLootElixir        int  `json:"min_loot_elixir"`
	MinLootDarkElixir    int  `json:"min_loot_de"`
}

type DebugConfig struct {
	CaptureDebug    bool `json:"capture_debug"`
	SaveScreenshots bool `json:"save_screenshots"`
	TemplateDebug   bool `json:"template_debug"`
	StateDebug      bool `json:"state_debug"`
	// UseShellPipe enables a persistent adb "shell:sh" connection during
	// attack cycles, amortizing the per-command `app_process` JVM spin-up
	// cost (100-300ms tax per tap). Default false; safe fallback to legacy
	// per-command transport.Exec when disabled or when the pipe breaks
	// mid-cycle. Stable on BlueStacks / macOS AVD; do not flip on hosts
	// where persistent shells are aggressively terminated.
	UseShellPipe bool `json:"use_shell_pipe"`
	// ShellPipeSyncFlush controls Tap semantics under the persistent pipe:
	//   true  = Tap blocks until the bytes are flushed to the socket (safer;
	//           preserves ordering relative to CaptureToMat).
	//   false = Tap is fire-and-forget (fastest; rely on HumanSleep between
	//           batches to preserve ordering).
	// Only consulted when UseShellPipe is true.
	ShellPipeSyncFlush bool    `json:"shell_pipe_sync_flush"`
	JitterTaps         bool    `json:"jitter_taps"`
	JitterDelays       bool    `json:"jitter_delays"`
	MaxJitterPixels    float64 `json:"max_jitter_pixels"`
	JitterFraction     float64 `json:"jitter_fraction"`
}

type UpgradeConfig struct {
	UpgradeWalls bool `json:"upgrade_walls"`
}

type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	s := string(b)
	s = s[1 : len(s)-1]

	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}
func DefaultConfig() *BotConfig {
	return &BotConfig{
		Device: DeviceConfig{
			ADBHost:               "127.0.0.1",
			ADBPort:               5037,
			DeviceID:              "localhost:5555",
			BlueStacksInstance:    "",
			PackageName:           "com.supercell.clashofclans",
			ZoomOutKey:            "i",
			ZoomInKey:             "o",
			Width:                 860,
			Height:                732,
			DPI:                   160,
			RestartOnStartup:      true,
			DisableChestDismissal: false, // chest dismissal loop IS on by default
		},
		Training: TrainingConfig{
			Enabled:              true,
			FullArmyBeforeAttack: true,
			SleepAfterTrain:      Duration{5 * time.Second},
		},
		Attack: AttackConfig{
			Enabled:                  true,
			StrategyFile:             paths.Resolve("strategies/auto_edrag_rush.yaml"),
			MaxAttackPerSession:      100,
			DropDelay:                Duration{500 * time.Millisecond},
			SpellDelay:               Duration{2 * time.Second},
			EndBattleDelay:           Duration{30 * time.Second},
			UseHeroes:                true,
			UseQueen:                 true,
			UseWarden:                true,
			UseClanCastle:            true,
			QueenChargeAtPct:         50,
			WardenUseAtPct:           30,
			ReserveDEPercent:         200,
			StallTimerSeconds:        10,
			LootExitEnabled:          false,
			LootExitPercent:          100,
			MinSecondsBetweenAttacks: 3,
			Farm: FarmConfig{
				Enabled:  false,
				TownHall: 18,
				Profiles: defaultFarmProfiles(),
			},
		},
		Search: SearchConfig{
			Enabled:                   true,
			MaxSkipsBeforeForceAttack: 999,
			MinTrophies:          0,
			MaxTrophies:          3000,
			MinTownHall:          7,
			MaxTownHall:          13,
			SkipMaxTH:            false,
			AttackIfDarkElixirGT: 0,
			MinLootGold:          750000,
			MinLootElixir:        750000,
			MinLootDarkElixir:    2000,
		},
		Upgrade: UpgradeConfig{
			UpgradeWalls: false,
		},
		Debug: DebugConfig{
			CaptureDebug:       false,
			SaveScreenshots:    false,
			TemplateDebug:      false,
			StateDebug:         false,
			UseShellPipe:       true,
			ShellPipeSyncFlush: true,
			JitterTaps:         true,
			JitterDelays:       true,
			MaxJitterPixels:    2.0,
			JitterFraction:     0.15,
		},
		Account: AccountConfig{},
		Automation: AutomationConfig{
			SimpleMode:            true,
			AutoFarmProfile:       true,
			AutoArmyGuard:         true,
			AutoResourceTracking:  true,
			AutoProfileSync:       true,
			Preferences: SimplePreferences{
				AutoDonate:          false,
				DonateOnlyRequested: true,
				UseHeroes:           true,
				UseClanCastle:       true,
				WaitForFullArmy:     true,
				AutoRetrain:         true,
				AutoUpgradeWalls:    false,
				LootPreset:          "balanced",
			},
		},
	}
}

func normalizeStrategyFile(cfg *BotConfig) {
	if cfg == nil {
		return
	}
	raw := strings.TrimSpace(cfg.Attack.StrategyFile)
	if raw == "" {
		cfg.Attack.StrategyFile = paths.Resolve("strategies/auto_edrag_rush.yaml")
		return
	}

	// Relative values are always interpreted as a strategy basename inside
	// the packaged assets tree. This makes config.json portable.
	if !filepath.IsAbs(raw) {
		cfg.Attack.StrategyFile = paths.Resolve(filepath.Join("strategies", filepath.Base(raw)))
		return
	}

	// Keep a valid absolute path (older configs), but rebase stale absolute
	// paths after the portable folder has been moved or updated.
	if info, err := os.Stat(raw); err == nil && !info.IsDir() {
		return
	}
	candidate := paths.Resolve(filepath.Join("strategies", filepath.Base(raw)))
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		cfg.Attack.StrategyFile = candidate
	}
}

func Load(path string) (*BotConfig, error) {
	if path == "config.json" {
		path = paths.ResolveConfig("config.json")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := *DefaultConfig()

	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Backward-compatible migration for fields introduced by the Windows
	// automation scheduler. We need presence information here, not just bool
	// values: an omitted JSON bool and an explicitly-false bool both decode to
	// false. Preserve old user intent when legacy fields are present while
	// retaining new-install defaults when they are absent altogether.
	var presence struct {
		Attack map[string]json.RawMessage `json:"attack"`
		Upgrade struct {
			UpgradeWalls *bool `json:"upgrade_walls"`
		} `json:"upgrade"`
		Automation struct {
			Preferences map[string]json.RawMessage `json:"preferences"`
		} `json:"automation"`
	}
	if json.Unmarshal(data, &presence) == nil {
		if _, hasGenericHeroes := presence.Attack["use_heroes"]; !hasGenericHeroes {
			_, hadQueen := presence.Attack["use_queen"]
			_, hadWarden := presence.Attack["use_warden"]
			if hadQueen || hadWarden {
				cfg.Attack.UseHeroes = cfg.Attack.UseQueen || cfg.Attack.UseWarden
			}
		}

		if _, hasWallPreference := presence.Automation.Preferences["auto_upgrade_walls"]; !hasWallPreference &&
			presence.Upgrade.UpgradeWalls != nil {
			cfg.Automation.Preferences.AutoUpgradeWalls = *presence.Upgrade.UpgradeWalls
		}
	}

	normalizeStrategyFile(&cfg)

	return &cfg, nil
}

func LoadOrDefault(path string) *BotConfig {
	cfg, err := Load(path)
	if err != nil {
		return DefaultConfig()
	}
	return cfg
}
