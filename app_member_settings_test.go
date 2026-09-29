package main

import (
	"testing"
	"time"

	"github.com/Ducky705/ClashGO/internal/config"
	"encoding/json"
	"os"
	"path/filepath"
)

func TestNormalizeSpeedProfile(t *testing.T) {
	tests := map[string]string{
		"":          "normal",
		"NORMAL":    "normal",
		" cautious ": "cautious",
		"FAST":      "fast",
		"turbo":     "normal",
	}
	for input, want := range tests {
		if got := normalizeSpeedProfile(input); got != want {
			t.Fatalf("normalizeSpeedProfile(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestApplyMemberSpeedProfile(t *testing.T) {
	tests := []struct {
		name       string
		profile    string
		drop       time.Duration
		spell      time.Duration
		attacks    int
		breakEvery int
		breakFor   time.Duration
		minGap     int
	}{
		{"cautious", "cautious", 700 * time.Millisecond, 2200 * time.Millisecond, 8, 4, 4 * time.Minute, 45},
		{"normal", "normal", 500 * time.Millisecond, 2 * time.Second, 12, 5, 3 * time.Minute, 30},
		{"fast", "fast", 300 * time.Millisecond, 1300 * time.Millisecond, 16, 6, 2 * time.Minute, 20},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Automation.BreakEveryAttacks = 0
			cfg.Automation.BreakDuration = config.Duration{}
			cfg.Search.MinLootGold = 987654
			cfg.Search.MinLootElixir = 876543
			cfg.Search.MinLootDarkElixir = 4321
			cfg.Attack.StrategyFile = "keep-me.yaml"
			cfg.Attack.MaxAttackPerSession = 37
			cfg.Upgrade.UpgradeWalls = true
			cfg.Attack.UseQueen = true
			cfg.Attack.LootExitEnabled = true

			applyMemberSpeedProfile(cfg, tc.profile)

			if cfg.Automation.SpeedProfile != tc.profile {
				t.Fatalf("SpeedProfile = %q, want %q", cfg.Automation.SpeedProfile, tc.profile)
			}
			if cfg.Attack.DropDelay.Duration != tc.drop {
				t.Fatalf("DropDelay = %s, want %s", cfg.Attack.DropDelay.Duration, tc.drop)
			}
			if cfg.Attack.SpellDelay.Duration != tc.spell {
				t.Fatalf("SpellDelay = %s, want %s", cfg.Attack.SpellDelay.Duration, tc.spell)
			}
			if cfg.Automation.MaxAttacksPerHour != tc.attacks {
				t.Fatalf("MaxAttacksPerHour = %d, want %d", cfg.Automation.MaxAttacksPerHour, tc.attacks)
			}
			if cfg.Automation.BreakEveryAttacks != tc.breakEvery {
				t.Fatalf("BreakEveryAttacks = %d, want %d", cfg.Automation.BreakEveryAttacks, tc.breakEvery)
			}
			if cfg.Automation.BreakDuration.Duration != tc.breakFor {
				t.Fatalf("BreakDuration = %s, want %s", cfg.Automation.BreakDuration.Duration, tc.breakFor)
			}
			if cfg.Attack.MinSecondsBetweenAttacks != tc.minGap {
				t.Fatalf("MinSecondsBetweenAttacks = %d, want %d", cfg.Attack.MinSecondsBetweenAttacks, tc.minGap)
			}

			// Speed profiles must not touch unrelated working settings.
			if cfg.Search.MinLootGold != 987654 {
				t.Fatalf("speed profile changed loot threshold: %d", cfg.Search.MinLootGold)
			}
			if cfg.Attack.StrategyFile != "keep-me.yaml" {
				t.Fatalf("speed profile changed strategy: %q", cfg.Attack.StrategyFile)
			}
			if cfg.Search.MinLootElixir != 876543 || cfg.Search.MinLootDarkElixir != 4321 {
				t.Fatalf("speed profile changed loot thresholds: E=%d DE=%d", cfg.Search.MinLootElixir, cfg.Search.MinLootDarkElixir)
			}
			if cfg.Attack.MaxAttackPerSession != 37 {
				t.Fatalf("speed profile changed session attack cap: %d", cfg.Attack.MaxAttackPerSession)
			}
			if !cfg.Upgrade.UpgradeWalls {
				t.Fatal("speed profile changed wall-upgrade preference")
			}
			if !cfg.Attack.UseQueen {
				t.Fatal("speed profile changed hero preference")
			}
			if !cfg.Attack.LootExitEnabled {
				t.Fatal("speed profile changed loot-exit preference")
			}
		})
	}
}

func TestApplyMemberSpeedProfileKeepsExplicitBreakSettings(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Automation.BreakEveryAttacks = 9
	cfg.Automation.BreakDuration = config.Duration{Duration: 7 * time.Minute}

	applyMemberSpeedProfile(cfg, "fast")

	if cfg.Automation.BreakEveryAttacks != 9 {
		t.Fatalf("explicit BreakEveryAttacks overwritten: %d", cfg.Automation.BreakEveryAttacks)
	}
	if cfg.Automation.BreakDuration.Duration != 7*time.Minute {
		t.Fatalf("explicit BreakDuration overwritten: %s", cfg.Automation.BreakDuration.Duration)
	}
}


func TestSanitizeMemberSettingsClampsUnsafeValues(t *testing.T) {
	got := sanitizeMemberSettings(MemberSettings{
		SpeedProfile:      "turbo",
		MaxAttacksPerHour: 999,
		BreakEveryAttacks: -5,
		BreakMinutes:      99,
	})

	if got.SpeedProfile != "normal" {
		t.Fatalf("SpeedProfile = %q, want normal", got.SpeedProfile)
	}
	if got.MaxAttacksPerHour != 24 {
		t.Fatalf("MaxAttacksPerHour = %d, want 24", got.MaxAttacksPerHour)
	}
	if got.BreakEveryAttacks != 0 {
		t.Fatalf("BreakEveryAttacks = %d, want 0", got.BreakEveryAttacks)
	}
	if got.BreakMinutes != 30 {
		t.Fatalf("BreakMinutes = %d, want 30", got.BreakMinutes)
	}
}

func TestSanitizeMemberSettingsRaisesMinimumAttackRate(t *testing.T) {
	got := sanitizeMemberSettings(MemberSettings{
		SpeedProfile:      "fast",
		MaxAttacksPerHour: 0,
		BreakEveryAttacks: 50,
		BreakMinutes:      -1,
	})

	if got.SpeedProfile != "fast" {
		t.Fatalf("SpeedProfile = %q, want fast", got.SpeedProfile)
	}
	if got.MaxAttacksPerHour != 1 {
		t.Fatalf("MaxAttacksPerHour = %d, want 1", got.MaxAttacksPerHour)
	}
	if got.BreakEveryAttacks != 20 {
		t.Fatalf("BreakEveryAttacks = %d, want 20", got.BreakEveryAttacks)
	}
	if got.BreakMinutes != 0 {
		t.Fatalf("BreakMinutes = %d, want 0", got.BreakMinutes)
	}
}


func TestDefaultMemberSettingsAreSafeAndUsable(t *testing.T) {
	got := defaultMemberSettings()
	if got.SpeedProfile != "normal" {
		t.Fatalf("SpeedProfile=%q want normal", got.SpeedProfile)
	}
	if got.MaxAttacksPerHour != 12 || got.BreakEveryAttacks != 5 || got.BreakMinutes != 3 {
		t.Fatalf("unexpected default pacing: %+v", got)
	}
	if !got.AdaptiveSearch || !got.AutoProfileSync || !got.AutoArmyGuard || !got.AutoResourceTracking {
		t.Fatalf("expected recommended automations enabled: %+v", got)
	}
}

func TestApplyMemberSettingsToConfigPreservesUnrelatedFarmSettings(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Search.MinLootGold = 765432
	cfg.Search.MinLootElixir = 654321
	cfg.Search.MinLootDarkElixir = 4321
	cfg.Attack.StrategyFile = "custom-strategy.yaml"
	cfg.Upgrade.UpgradeWalls = true
	cfg.Attack.LootExitEnabled = true
	cfg.Attack.LootExitPercent = 77

	settings := MemberSettings{
		SpeedProfile:         "fast",
		MaxAttacksPerHour:    14,
		BreakEveryAttacks:    7,
		BreakMinutes:         2,
		AdaptiveSearch:       false,
		AutoProfileSync:      true,
		AutoArmyGuard:        true,
		AutoResourceTracking: true,
	}
	applyMemberSettingsToConfig(cfg, settings)

	if cfg.Automation.SpeedProfile != "fast" {
		t.Fatalf("speed profile=%q want fast", cfg.Automation.SpeedProfile)
	}
	if cfg.Automation.MaxAttacksPerHour != 14 || cfg.Automation.BreakEveryAttacks != 7 {
		t.Fatalf("member pacing not applied: %+v", cfg.Automation)
	}
	if cfg.Search.AdaptiveSearch {
		t.Fatal("adaptive search preference was not applied")
	}

	if cfg.Search.MinLootGold != 765432 || cfg.Search.MinLootElixir != 654321 || cfg.Search.MinLootDarkElixir != 4321 {
		t.Fatalf("member settings changed loot thresholds: %+v", cfg.Search)
	}
	if cfg.Attack.StrategyFile != "custom-strategy.yaml" {
		t.Fatalf("member settings changed attack strategy: %q", cfg.Attack.StrategyFile)
	}
	if !cfg.Upgrade.UpgradeWalls {
		t.Fatal("member settings changed wall-upgrade preference")
	}
	if !cfg.Attack.LootExitEnabled || cfg.Attack.LootExitPercent != 77 {
		t.Fatalf("member settings changed loot-exit behavior: %+v", cfg.Attack)
	}
}


func TestMemberProfileFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members", "profile.json")
	want := MemberSettings{
		SpeedProfile:         "fast",
		MaxAttacksPerHour:    16,
		BreakEveryAttacks:    6,
		BreakMinutes:         2,
		AdaptiveSearch:       true,
		AutoProfileSync:      true,
		AutoArmyGuard:        true,
		AutoResourceTracking: true,
	}

	if err := saveMemberProfileFile(path, want); err != nil {
		t.Fatalf("saveMemberProfileFile: %v", err)
	}
	got, ok := loadMemberProfileFile(path)
	if !ok {
		t.Fatal("expected saved member profile to load")
	}
	if got != sanitizeMemberSettings(want) {
		t.Fatalf("loaded member settings mismatch: got=%+v want=%+v", got, sanitizeMemberSettings(want))
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary member profile survived successful save: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup member profile survived successful save: %v", err)
	}
}

func TestMemberProfileFileRecoversBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members", "profile.json")
	want := MemberSettings{
		SpeedProfile:         "cautious",
		MaxAttacksPerHour:    8,
		BreakEveryAttacks:    4,
		BreakMinutes:         4,
		AdaptiveSearch:       false,
		AutoProfileSync:      true,
		AutoArmyGuard:        true,
		AutoResourceTracking: true,
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := loadMemberProfileFile(path)
	if !ok {
		t.Fatal("expected backup member profile to recover")
	}
	if got != sanitizeMemberSettings(want) {
		t.Fatalf("recovered member settings mismatch: got=%+v want=%+v", got, sanitizeMemberSettings(want))
	}
	if primary, err := os.ReadFile(path); err != nil {
		t.Fatalf("recovered primary missing: %v", err)
	} else {
		var restored MemberSettings
		if json.Unmarshal(primary, &restored) != nil {
			t.Fatalf("recovered primary is not valid json: %q", string(primary))
		}
	}
}


func TestClearCachedPlayerProfileIfDifferentRemovesOtherMember(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLASHGO_CONFIG_DIR", dir)

	profile := ClashPlayerProfile{Tag: "#OLD123", Name: "Old Member"}
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "account_profile.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	clearCachedPlayerProfileIfDifferent("#NEW456")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected mismatched cached profile to be removed, stat err=%v", err)
	}
}

func TestClearCachedPlayerProfileIfDifferentKeepsMatchingMember(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLASHGO_CONFIG_DIR", dir)

	profile := ClashPlayerProfile{Tag: "#SAME123", Name: "Same Member"}
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "account_profile.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	clearCachedPlayerProfileIfDifferent("#same123")

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected matching cached profile to remain, err=%v", err)
	}
}


func TestMemberAccountFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members", "profile.account.json")
	want := "#ABC123XYZ"

	if err := saveMemberAccountFile(path, want); err != nil {
		t.Fatalf("saveMemberAccountFile: %v", err)
	}
	got, ok := loadMemberAccountFile(path)
	if !ok {
		t.Fatal("expected saved member account to load")
	}
	if got != want {
		t.Fatalf("loaded tag=%q want=%q", got, want)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary member account survived successful save: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup member account survived successful save: %v", err)
	}
}

func TestMemberAccountFileRecoversBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members", "profile.account.json")
	want := "#RECOVER9"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	backup, err := json.Marshal(memberAccountProfile{PlayerTag: want})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", backup, 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := loadMemberAccountFile(path)
	if !ok {
		t.Fatal("expected backup member account to recover")
	}
	if got != want {
		t.Fatalf("recovered tag=%q want=%q", got, want)
	}
	primary, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("recovered primary missing: %v", err)
	}
	var restored memberAccountProfile
	if err := json.Unmarshal(primary, &restored); err != nil {
		t.Fatalf("recovered primary invalid json: %v", err)
	}
	if restored.PlayerTag != want {
		t.Fatalf("recovered primary tag=%q want=%q", restored.PlayerTag, want)
	}
}


func TestMemberAccountFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "member.account.json")
	if err := saveMemberAccountFile(path, "#ABC123"); err != nil {
		t.Fatalf("saveMemberAccountFile failed: %v", err)
	}
	tag, ok := loadMemberAccountFile(path)
	if !ok {
		t.Fatal("expected member account file to load")
	}
	if tag != "#ABC123" {
		t.Fatalf("tag=%q want #ABC123", tag)
	}
}

func TestMemberAccountFileRecoversFromBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "member.account.json")
	backup := path + ".bak"

	good, err := json.Marshal(memberAccountProfile{PlayerTag: "#SAFE123"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, good, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken-json"), 0o600); err != nil {
		t.Fatal(err)
	}

	tag, ok := loadMemberAccountFile(path)
	if !ok {
		t.Fatal("expected backup recovery to succeed")
	}
	if tag != "#SAFE123" {
		t.Fatalf("recovered tag=%q want #SAFE123", tag)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("self-healed primary missing: %v", err)
	}
	var healed memberAccountProfile
	if err := json.Unmarshal(data, &healed); err != nil {
		t.Fatalf("self-healed primary invalid: %v", err)
	}
	if healed.PlayerTag != "#SAFE123" {
		t.Fatalf("self-healed tag=%q want #SAFE123", healed.PlayerTag)
	}
}
