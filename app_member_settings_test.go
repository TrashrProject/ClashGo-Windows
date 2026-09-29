package main

import (
	"context"
	"testing"
	"time"

	"github.com/Ducky705/ClashGO/internal/config"
	"github.com/Ducky705/ClashGO/internal/licensing"
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
		SpeedProfile:         "turbo",
		MaxAttacksPerHour:    999,
		MaxAttacksPerSession: 9999,
		BreakEveryAttacks:    -5,
		BreakMinutes:      99,
	})

	if got.SpeedProfile != "normal" {
		t.Fatalf("SpeedProfile = %q, want normal", got.SpeedProfile)
	}
	if got.MaxAttacksPerHour != 24 {
		t.Fatalf("MaxAttacksPerHour = %d, want 24", got.MaxAttacksPerHour)
	}
	if got.MaxAttacksPerSession != 500 {
		t.Fatalf("MaxAttacksPerSession = %d, want 500", got.MaxAttacksPerSession)
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
		SpeedProfile:         "fast",
		MaxAttacksPerHour:    0,
		MaxAttacksPerSession: 0,
		BreakEveryAttacks:    50,
		BreakMinutes:      -1,
	})

	if got.SpeedProfile != "fast" {
		t.Fatalf("SpeedProfile = %q, want fast", got.SpeedProfile)
	}
	if got.MaxAttacksPerHour != 1 {
		t.Fatalf("MaxAttacksPerHour = %d, want 1", got.MaxAttacksPerHour)
	}
	if got.MaxAttacksPerSession != 100 {
		t.Fatalf("MaxAttacksPerSession = %d, want migration default 100", got.MaxAttacksPerSession)
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
	if got.MaxAttacksPerHour != 12 || got.MaxAttacksPerSession != 100 || got.BreakEveryAttacks != 5 || got.BreakMinutes != 3 {
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
		MaxAttacksPerSession: 42,
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
	if cfg.Attack.MaxAttackPerSession != 42 {
		t.Fatalf("session attack cap=%d want 42", cfg.Attack.MaxAttackPerSession)
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
		MaxAttacksPerSession: 75,
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
		MaxAttacksPerSession: 25,
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


func TestLegacyMemberProfileGetsSafeSessionDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members", "legacy.json")
	legacy := map[string]any{
		"speed_profile": "normal",
		"max_attacks_per_hour": 12,
		"break_every_attacks": 5,
		"break_minutes": 3,
		"adaptive_search": true,
		"auto_profile_sync": true,
		"auto_army_guard": true,
		"auto_resource_tracking": true,
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := loadMemberProfileFile(path)
	if !ok {
		t.Fatal("expected legacy member profile to load")
	}
	if got.MaxAttacksPerSession != 100 {
		t.Fatalf("legacy MaxAttacksPerSession=%d want 100", got.MaxAttacksPerSession)
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


func TestMemberAccountFileRoundTripMinimal(t *testing.T) {
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


func TestMemberInterfaceLevelDefaultsToSimple(t *testing.T) {
	got := sanitizeMemberSettings(MemberSettings{
		InterfaceLevel:       "",
		SpeedProfile:         "normal",
		MaxAttacksPerHour:    12,
		BreakEveryAttacks:    5,
		BreakMinutes:         3,
		AdaptiveSearch:       true,
		AutoProfileSync:      true,
		AutoArmyGuard:        true,
		AutoResourceTracking: true,
	})
	if got.InterfaceLevel != "simple" {
		t.Fatalf("InterfaceLevel=%q want simple", got.InterfaceLevel)
	}
}

func TestMemberInterfaceLevelPersistsAdvanced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members", "advanced-profile.json")
	want := defaultMemberSettings()
	want.InterfaceLevel = "advanced"

	if err := saveMemberProfileFile(path, want); err != nil {
		t.Fatalf("saveMemberProfileFile: %v", err)
	}
	got, ok := loadMemberProfileFile(path)
	if !ok {
		t.Fatal("expected advanced member profile to load")
	}
	if got.InterfaceLevel != "advanced" {
		t.Fatalf("InterfaceLevel=%q want advanced", got.InterfaceLevel)
	}
}

func TestMemberInterfaceLevelCannotPersistDeveloper(t *testing.T) {
	got := sanitizeMemberSettings(MemberSettings{
		InterfaceLevel:    "developer",
		SpeedProfile:      "fast",
		MaxAttacksPerHour: 16,
		BreakEveryAttacks: 6,
		BreakMinutes:      2,
	})
	if got.InterfaceLevel != "simple" {
		t.Fatalf("member profile stored forbidden interface level %q", got.InterfaceLevel)
	}
}


func TestPlayerProfileFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members", "profile.player.json")
	want := &ClashPlayerProfile{
		Tag:           "#ABC123",
		Name:          "Member",
		TownHallLevel: 17,
		ExpLevel:      250,
		Trophies:      5200,
	}
	if err := savePlayerProfileFile(path, want); err != nil {
		t.Fatalf("savePlayerProfileFile: %v", err)
	}
	got, ok := loadPlayerProfileFile(path)
	if !ok {
		t.Fatal("expected cached player profile to load")
	}
	if got.Tag != want.Tag || got.Name != want.Name || got.TownHallLevel != want.TownHallLevel {
		t.Fatalf("loaded player profile mismatch: got=%+v want=%+v", got, want)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary player profile survived successful save: %v", err)
	}
}

func TestPlayerProfileFileRecoversBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members", "profile.player.json")
	want := &ClashPlayerProfile{
		Tag:           "#BACKUP1",
		Name:          "Recovered",
		TownHallLevel: 16,
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	blob, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", blob, 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := loadPlayerProfileFile(path)
	if !ok {
		t.Fatal("expected backup player profile to recover")
	}
	if got.Tag != want.Tag || got.TownHallLevel != want.TownHallLevel {
		t.Fatalf("recovered player profile mismatch: got=%+v want=%+v", got, want)
	}
	primary, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("recovered primary missing: %v", err)
	}
	var restored ClashPlayerProfile
	if err := json.Unmarshal(primary, &restored); err != nil {
		t.Fatalf("recovered primary invalid json: %v", err)
	}
	if restored.Tag != want.Tag {
		t.Fatalf("recovered primary tag=%q want=%q", restored.Tag, want.Tag)
	}
}


func testLicensedApp(t *testing.T, key string) *App {
	t.Helper()
	dir := os.Getenv("CLASHGO_CONFIG_DIR")
	if dir == "" {
		t.Fatal("CLASHGO_CONFIG_DIR must be set before creating licensed test app")
	}
	payload := map[string]any{
		"license_key": key,
		"role": "member",
		"machine_id": "test-machine",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "license.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return &App{license: licensing.New("", "test")}
}

func TestMemberRuntimeStateArchiveAndRestore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLASHGO_CONFIG_DIR", dir)
	a := testLicensedApp(t, "CGO-AAAAAA-BBBBBB-CCCCCC-DDDDDD")

	originalStats := []byte(`{"attacks_completed":7,"total_gold":123456}`)
	originalHistory := []byte(`[{"timestamp":"one","stars":3}]`)
	if err := os.WriteFile(filepath.Join(dir, "stats.json"), originalStats, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "attack_history.json"), originalHistory, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := a.archiveMemberRuntimeState(true); err != nil {
		t.Fatalf("archiveMemberRuntimeState failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stats.json")); !os.IsNotExist(err) {
		t.Fatalf("shared stats should be cleared after archive, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "attack_history.json")); !os.IsNotExist(err) {
		t.Fatalf("shared history should be cleared after archive, err=%v", err)
	}

	// Simulate unrelated/stale shared files before restoring this member.
	if err := os.WriteFile(filepath.Join(dir, "stats.json"), []byte(`{"attacks_completed":99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.restoreMemberRuntimeState(false); err != nil {
		t.Fatalf("restoreMemberRuntimeState failed: %v", err)
	}

	gotStats, err := os.ReadFile(filepath.Join(dir, "stats.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotStats) != string(originalStats) {
		t.Fatalf("restored stats=%s want=%s", gotStats, originalStats)
	}
	gotHistory, err := os.ReadFile(filepath.Join(dir, "attack_history.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotHistory) != string(originalHistory) {
		t.Fatalf("restored history=%s want=%s", gotHistory, originalHistory)
	}
}

func TestNewLicenseRuntimeStateNeverInheritsSharedFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLASHGO_CONFIG_DIR", dir)
	a := testLicensedApp(t, "CGO-111111-222222-333333-444444")

	if err := os.WriteFile(filepath.Join(dir, "stats.json"), []byte(`{"attacks_completed":42}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "attack_history.json"), []byte(`[{"timestamp":"other-member"}]`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := a.restoreMemberRuntimeState(false); err != nil {
		t.Fatalf("restoreMemberRuntimeState for new member failed: %v", err)
	}
	for _, name := range []string{"stats.json", "attack_history.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("new member inherited shared %s, stat err=%v", name, err)
		}
	}

	marker := filepath.Join(a.memberRuntimeStateDir(), ".initialized")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("new member state marker missing: %v", err)
	}
}


func TestResetStatsPurgesCurrentMemberRuntimeSnapshot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLASHGO_CONFIG_DIR", dir)
	a := testLicensedApp(t, "CGO-RESET1-RESET2-RESET3-RESET4")

	stateDir := a.memberRuntimeStateDir()
	if err := os.MkdirAll(filepath.Join(stateDir, "output", "session_reports"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "output", "session_reports"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{
		"stats.json",
		"attack_history.json",
		filepath.Join("output", "session_reports", "latest.json"),
	} {
		shared := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(shared), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(shared, []byte("{\"stale\":true}"), 0o600); err != nil {
			t.Fatal(err)
		}

		archived := filepath.Join(stateDir, rel)
		if err := os.MkdirAll(filepath.Dir(archived), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(archived, []byte("{\"stale\":true}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := a.ResetStats(); err != nil {
		t.Fatalf("ResetStats failed: %v", err)
	}

	for _, rel := range []string{
		"stats.json",
		"attack_history.json",
		filepath.Join("output", "session_reports", "latest.json"),
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); !os.IsNotExist(err) {
			t.Fatalf("shared %s survived reset, err=%v", rel, err)
		}
		if _, err := os.Stat(filepath.Join(stateDir, rel)); !os.IsNotExist(err) {
			t.Fatalf("archived %s survived reset, err=%v", rel, err)
		}
	}
}

func TestArchiveMemberRuntimeStateRemovesDeletedArchivedFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLASHGO_CONFIG_DIR", dir)
	a := testLicensedApp(t, "CGO-DELETE-STATE1-STATE2-STATE3")

	stateDir := a.memberRuntimeStateDir()
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(stateDir, "stats.json")
	if err := os.WriteFile(stale, []byte(`{"attacks_completed":99}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// Shared stats are intentionally absent, as after ResetStats.
	if err := a.archiveMemberRuntimeState(false); err != nil {
		t.Fatalf("archiveMemberRuntimeState failed: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale archived stats should be removed, stat err=%v", err)
	}
}


func TestMemberAutomationProfileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members", "profile.automation.json")
	want := MemberAutomationProfile{
		SearchEnabled:     true,
		MinLootGold:       650000,
		MinLootElixir:     700000,
		MinLootDarkElixir: 3500,
		UpgradeWalls:      true,
		StrategyFile:      "auto_edrag_rush.yaml",
		StallTimerSeconds: 25,
		LootExitEnabled:   true,
		LootExitPercent:   82,
		FarmEnabled:       true,
		FarmTownHall:      17,
		FarmProfiles: map[string]config.FarmProfile{
			"17": {
				TownHall:      17,
				Label:         "Test Farm",
				TroopCapacity: 340,
				SpellCapacity: 11,
				Troops: []config.FarmUnit{
					{Name: "Electro Dragon", Count: 11, Housing: 30},
				},
				Spells: []config.FarmUnit{
					{Name: "Rage Spell", Count: 5, Housing: 2},
				},
			},
		},
	}

	if err := saveMemberAutomationFile(path, want); err != nil {
		t.Fatalf("saveMemberAutomationFile: %v", err)
	}
	got, ok := loadMemberAutomationFile(path)
	if !ok {
		t.Fatal("expected automation profile to load")
	}
	if got.MinLootGold != want.MinLootGold ||
		got.MinLootElixir != want.MinLootElixir ||
		got.MinLootDarkElixir != want.MinLootDarkElixir ||
		got.UpgradeWalls != want.UpgradeWalls ||
		got.StrategyFile != want.StrategyFile ||
		got.StallTimerSeconds != want.StallTimerSeconds ||
		got.LootExitEnabled != want.LootExitEnabled ||
		got.LootExitPercent != want.LootExitPercent ||
		got.FarmTownHall != want.FarmTownHall {
		t.Fatalf("automation profile mismatch: got=%+v want=%+v", got, want)
	}
	if _, ok := got.FarmProfiles["17"]; !ok {
		t.Fatal("farm profile was not preserved")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary automation profile survived successful save: %v", err)
	}
}

func TestMemberAutomationProfileRecoversBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members", "profile.automation.json")
	want := MemberAutomationProfile{
		SearchEnabled:     true,
		MinLootGold:       500000,
		MinLootElixir:     510000,
		MinLootDarkElixir: 2500,
		StrategyFile:      "auto_edrag_rush.yaml",
		StallTimerSeconds: 20,
		LootExitPercent:   90,
		FarmTownHall:      16,
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	blob, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", blob, 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := loadMemberAutomationFile(path)
	if !ok {
		t.Fatal("expected backup automation profile to recover")
	}
	if got.MinLootGold != want.MinLootGold || got.FarmTownHall != want.FarmTownHall {
		t.Fatalf("recovered automation mismatch: got=%+v want=%+v", got, want)
	}
	primary, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("recovered primary missing: %v", err)
	}
	var restored MemberAutomationProfile
	if err := json.Unmarshal(primary, &restored); err != nil {
		t.Fatalf("recovered primary invalid json: %v", err)
	}
}

func TestApplyMemberAutomationPreservesInternalEngineSettings(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Attack.UseQueen = true
	cfg.Attack.UseWarden = true
	cfg.Attack.UseClanCastle = true
	cfg.Attack.MaxAttackPerSession = 41
	cfg.Attack.DropDelay = config.Duration{Duration: 777 * time.Millisecond}
	cfg.Attack.SpellDelay = config.Duration{Duration: 1777 * time.Millisecond}
	cfg.Attack.MinSecondsBetweenAttacks = 33
	cfg.Search.AdaptiveSearch = false
	cfg.Automation.MaxAttacksPerHour = 14
	cfg.Automation.BreakEveryAttacks = 9

	profile := MemberAutomationProfile{
		SearchEnabled:     false,
		MinLootGold:       900000,
		MinLootElixir:     800000,
		MinLootDarkElixir: 4000,
		UpgradeWalls:      true,
		StallTimerSeconds: 44,
		LootExitEnabled:   true,
		LootExitPercent:   75,
		FarmEnabled:       false,
		FarmTownHall:      17,
	}
	applyMemberAutomationToConfig(cfg, profile)

	if cfg.Search.Enabled {
		t.Fatal("search enabled was not restored from member automation")
	}
	if cfg.Search.MinLootGold != 900000 || cfg.Search.MinLootElixir != 800000 || cfg.Search.MinLootDarkElixir != 4000 {
		t.Fatalf("loot settings not applied: %+v", cfg.Search)
	}
	if !cfg.Upgrade.UpgradeWalls || cfg.Attack.StallTimerSeconds != 44 ||
		!cfg.Attack.LootExitEnabled || cfg.Attack.LootExitPercent != 75 {
		t.Fatalf("user automation settings not applied")
	}

	// These are engine/member-pacing settings owned by other subsystems and
	// must never be overwritten by an advanced automation profile restore.
	if !cfg.Attack.UseQueen || !cfg.Attack.UseWarden || !cfg.Attack.UseClanCastle {
		t.Fatal("automation profile changed hero/clan-castle behavior")
	}
	if cfg.Attack.MaxAttackPerSession != 41 {
		t.Fatalf("session cap changed: %d", cfg.Attack.MaxAttackPerSession)
	}
	if cfg.Attack.DropDelay.Duration != 777*time.Millisecond ||
		cfg.Attack.SpellDelay.Duration != 1777*time.Millisecond ||
		cfg.Attack.MinSecondsBetweenAttacks != 33 {
		t.Fatal("automation profile changed pacing internals")
	}
	if cfg.Search.AdaptiveSearch {
		t.Fatal("automation profile changed adaptive search member preference")
	}
	if cfg.Automation.MaxAttacksPerHour != 14 || cfg.Automation.BreakEveryAttacks != 9 {
		t.Fatal("automation profile changed member governor preferences")
	}
}

func TestSanitizeMemberAutomationBounds(t *testing.T) {
	got := sanitizeMemberAutomationProfile(MemberAutomationProfile{
		MinLootGold:       -1,
		MinLootElixir:     99_000_000,
		MinLootDarkElixir: -500,
		StallTimerSeconds:  9999,
		LootExitPercent:   300,
		FarmTownHall:      99,
		StrategyFile:      "../unsafe.yaml",
	})
	if got.MinLootGold != 0 || got.MinLootElixir != 10_000_000 || got.MinLootDarkElixir != 0 {
		t.Fatalf("loot bounds not sanitized: %+v", got)
	}
	if got.StallTimerSeconds != 600 || got.LootExitPercent != 100 {
		t.Fatalf("timer/loot exit bounds not sanitized: %+v", got)
	}
	if got.FarmTownHall != 18 {
		t.Fatalf("FarmTownHall=%d want 18", got.FarmTownHall)
	}
	if got.StrategyFile != "unsafe.yaml" {
		t.Fatalf("strategy path was not reduced to basename: %q", got.StrategyFile)
	}
}


func TestStartupAdoptsNewerSharedRuntimeState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLASHGO_CONFIG_DIR", dir)
	a := testLicensedApp(t, "CGO-START1-START2-START3-START4")

	stateDir := a.memberRuntimeStateDir()
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, ".initialized"), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "stats.json"), []byte(`{"attacks_completed":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	newer := []byte(`{"attacks_completed":9}`)
	if err := os.WriteFile(filepath.Join(dir, "stats.json"), newer, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := a.restoreMemberRuntimeState(true); err != nil {
		t.Fatalf("startup adoption failed: %v", err)
	}

	shared, err := os.ReadFile(filepath.Join(dir, "stats.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(shared) != string(newer) {
		t.Fatalf("startup overwrote newer shared state: %s", shared)
	}
	archived, err := os.ReadFile(filepath.Join(stateDir, "stats.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(archived) != string(newer) {
		t.Fatalf("member archive was not refreshed from newer shared state: %s", archived)
	}
}

func TestActivationRestoresMemberArchiveOverResidualSharedState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLASHGO_CONFIG_DIR", dir)
	a := testLicensedApp(t, "CGO-REST01-REST02-REST03-REST04")

	stateDir := a.memberRuntimeStateDir()
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, ".initialized"), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	member := []byte(`{"attacks_completed":4}`)
	if err := os.WriteFile(filepath.Join(stateDir, "stats.json"), member, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stats.json"), []byte(`{"attacks_completed":88}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := a.restoreMemberRuntimeState(false); err != nil {
		t.Fatalf("activation restore failed: %v", err)
	}
	shared, err := os.ReadFile(filepath.Join(dir, "stats.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(shared) != string(member) {
		t.Fatalf("activation did not restore member archive: %s", shared)
	}
}


func TestMemberAutomationSimpleModeIsIndependent(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Automation.SimpleMode = true

	profile := memberAutomationFromConfig(cfg)
	if profile.SimpleMode == nil || !*profile.SimpleMode {
		t.Fatal("member automation profile did not capture automatic mode")
	}

	cfg.Automation.SimpleMode = false
	applyMemberAutomationToConfig(cfg, profile)
	if !cfg.Automation.SimpleMode {
		t.Fatal("member automation profile did not restore automatic mode")
	}

	manual := false
	profile.SimpleMode = &manual
	applyMemberAutomationToConfig(cfg, profile)
	if cfg.Automation.SimpleMode {
		t.Fatal("member automation profile did not restore manual mode")
	}
}

func TestLegacyAutomationProfileWithoutSimpleModePreservesCurrentMode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Automation.SimpleMode = true

	legacy := MemberAutomationProfile{
		SimpleMode:          nil,
		SearchEnabled:       false,
		MinLootGold:         600000,
		MinLootElixir:       600000,
		MinLootDarkElixir:   3000,
		StallTimerSeconds:   20,
		LootExitPercent:     100,
		FarmTownHall:        17,
	}
	applyMemberAutomationToConfig(cfg, legacy)

	if !cfg.Automation.SimpleMode {
		t.Fatal("legacy automation profile unexpectedly changed automatic mode")
	}
	if cfg.Search.Enabled {
		t.Fatal("legacy automation profile did not apply its supported settings")
	}
}


func TestApplyMemberPresetUsesSafeProfiles(t *testing.T) {
	base := MemberSettings{
		InterfaceLevel:       "advanced",
		SpeedProfile:         "cautious",
		MaxAttacksPerHour:    8,
		MaxAttacksPerSession: 25,
		BreakEveryAttacks:    4,
		BreakMinutes:         4,
		AdaptiveSearch:       false,
		AutoProfileSync:      true,
		AutoArmyGuard:        true,
		AutoResourceTracking: true,
	}

	tests := []struct {
		name       string
		preset     string
		speed      string
		perHour    int
		perSession int
		breakEvery int
		breakMins  int
	}{
		{"short", "short", "normal", 12, 10, 5, 3},
		{"balanced", "balanced", "normal", 12, 50, 5, 3},
		{"fast", "fast", "fast", 16, 100, 6, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := applyMemberPreset(base, tc.preset)
			if err != nil {
				t.Fatalf("applyMemberPreset(%q): %v", tc.preset, err)
			}
			if got.SpeedProfile != tc.speed ||
				got.MaxAttacksPerHour != tc.perHour ||
				got.MaxAttacksPerSession != tc.perSession ||
				got.BreakEveryAttacks != tc.breakEvery ||
				got.BreakMinutes != tc.breakMins {
				t.Fatalf("unexpected preset result: %+v", got)
			}
			if !got.AdaptiveSearch {
				t.Fatal("preset should enable adaptive search")
			}
			if got.InterfaceLevel != "advanced" {
				t.Fatalf("preset changed interface level: %q", got.InterfaceLevel)
			}
			if !got.AutoProfileSync || !got.AutoArmyGuard || !got.AutoResourceTracking {
				t.Fatalf("preset changed recommended automations: %+v", got)
			}
		})
	}
}

func TestApplyMemberPresetRejectsUnknownPreset(t *testing.T) {
	_, err := applyMemberPreset(defaultMemberSettings(), "turbo-plus")
	if err == nil {
		t.Fatal("expected unknown preset to be rejected")
	}
}


func TestMemberInterfaceLevelDoesNotChangeAutomationMode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Automation.SimpleMode = false

	settings := defaultMemberSettings()
	settings.InterfaceLevel = "simple"
	applyMemberSettingsToConfig(cfg, settings)
	if cfg.Automation.SimpleMode {
		t.Fatal("simple interface unexpectedly enabled automatic bot mode")
	}

	cfg.Automation.SimpleMode = true
	settings.InterfaceLevel = "advanced"
	applyMemberSettingsToConfig(cfg, settings)
	if !cfg.Automation.SimpleMode {
		t.Fatal("advanced interface unexpectedly disabled automatic bot mode")
	}
}


func TestMemberRuntimeConfigReady(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Automation.SpeedProfile = "normal"
	cfg.Automation.MaxAttacksPerHour = 12
	cfg.Attack.MaxAttackPerSession = 50
	cfg.Automation.BreakEveryAttacks = 5
	cfg.Automation.BreakDuration = config.Duration{Duration: 3 * time.Minute}

	ok, message := memberRuntimeConfigReady(cfg)
	if !ok {
		t.Fatalf("expected valid member runtime config, got %q", message)
	}
	if message == "" {
		t.Fatal("expected readable pacing summary")
	}

	cfg.Automation.SpeedProfile = "turbo"
	if ok, _ := memberRuntimeConfigReady(cfg); ok {
		t.Fatal("invalid speed profile should fail readiness")
	}

	cfg.Automation.SpeedProfile = "normal"
	cfg.Automation.MaxAttacksPerHour = 25
	if ok, _ := memberRuntimeConfigReady(cfg); ok {
		t.Fatal("out-of-range attacks/hour should fail readiness")
	}

	cfg.Automation.MaxAttacksPerHour = 12
	cfg.Attack.MaxAttackPerSession = 0
	if ok, _ := memberRuntimeConfigReady(cfg); ok {
		t.Fatal("zero session cap should fail readiness")
	}

	cfg.Attack.MaxAttackPerSession = 50
	cfg.Automation.BreakEveryAttacks = 21
	if ok, _ := memberRuntimeConfigReady(cfg); ok {
		t.Fatal("out-of-range break frequency should fail readiness")
	}

	cfg.Automation.BreakEveryAttacks = 5
	cfg.Automation.BreakDuration = config.Duration{Duration: 31 * time.Minute}
	if ok, _ := memberRuntimeConfigReady(cfg); ok {
		t.Fatal("out-of-range break duration should fail readiness")
	}
}


func TestShortSessionPresetPreservesFarmAndAttackPreferences(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Search.MinLootGold = 888888
	cfg.Search.MinLootElixir = 777777
	cfg.Search.MinLootDarkElixir = 3333
	cfg.Search.Enabled = false
	cfg.Attack.StrategyFile = "my-working-strategy.yaml"
	cfg.Upgrade.UpgradeWalls = true
	cfg.Attack.LootExitEnabled = true
	cfg.Attack.LootExitPercent = 73
	cfg.Attack.StallTimerSeconds = 27
	cfg.Attack.UseQueen = true
	cfg.Attack.UseWarden = true
	cfg.Attack.UseClanCastle = true

	current := defaultMemberSettings()
	current.InterfaceLevel = "advanced"
	next, err := applyMemberPreset(current, "short")
	if err != nil {
		t.Fatalf("applyMemberPreset(short): %v", err)
	}
	applyMemberSettingsToConfig(cfg, next)

	if cfg.Attack.MaxAttackPerSession != 10 {
		t.Fatalf("session cap=%d want 10", cfg.Attack.MaxAttackPerSession)
	}
	if cfg.Automation.MaxAttacksPerHour != 12 {
		t.Fatalf("hourly cap=%d want 12", cfg.Automation.MaxAttacksPerHour)
	}

	if cfg.Search.MinLootGold != 888888 ||
		cfg.Search.MinLootElixir != 777777 ||
		cfg.Search.MinLootDarkElixir != 3333 ||
		cfg.Search.Enabled {
		t.Fatalf("short preset changed search preferences: %+v", cfg.Search)
	}
	if cfg.Attack.StrategyFile != "my-working-strategy.yaml" {
		t.Fatalf("short preset changed strategy: %q", cfg.Attack.StrategyFile)
	}
	if !cfg.Upgrade.UpgradeWalls {
		t.Fatal("short preset changed wall-upgrade preference")
	}
	if !cfg.Attack.LootExitEnabled || cfg.Attack.LootExitPercent != 73 {
		t.Fatalf("short preset changed loot-exit settings: %+v", cfg.Attack)
	}
	if cfg.Attack.StallTimerSeconds != 27 {
		t.Fatalf("short preset changed stall timer: %d", cfg.Attack.StallTimerSeconds)
	}
	if !cfg.Attack.UseQueen || !cfg.Attack.UseWarden || !cfg.Attack.UseClanCastle {
		t.Fatal("short preset changed hero/clan-castle preferences")
	}
}


func TestTemporaryTestSessionSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLASHGO_CONFIG_DIR", dir)
	a := testLicensedApp(t, "CGO-TEST01-TEST02-TEST03-TEST04")

	original := MemberSettings{
		InterfaceLevel:       "advanced",
		SpeedProfile:         "cautious",
		MaxAttacksPerHour:    8,
		MaxAttacksPerSession: 37,
		BreakEveryAttacks:    4,
		BreakMinutes:         7,
		AdaptiveSearch:       false,
		AutoProfileSync:      true,
		AutoArmyGuard:        true,
		AutoResourceTracking: true,
	}
	if _, err := a.SaveMemberSettings(original); err != nil {
		t.Fatalf("save original settings: %v", err)
	}

	restorePath := a.testSessionRestorePath()
	if err := saveMemberProfileFile(restorePath, original); err != nil {
		t.Fatalf("save temporary restore snapshot: %v", err)
	}

	testProfile, err := applyMemberPreset(original, "short")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveMemberSettings(testProfile); err != nil {
		t.Fatalf("apply test profile: %v", err)
	}

	during := a.GetMemberSettings()
	if during.MaxAttacksPerSession != 10 || during.SpeedProfile != "normal" {
		t.Fatalf("test preset was not applied: %+v", during)
	}

	if err := a.restoreTestSessionSettings(); err != nil {
		t.Fatalf("restore test settings: %v", err)
	}
	got := a.GetMemberSettings()
	if got.InterfaceLevel != original.InterfaceLevel ||
		got.SpeedProfile != original.SpeedProfile ||
		got.MaxAttacksPerHour != original.MaxAttacksPerHour ||
		got.MaxAttacksPerSession != original.MaxAttacksPerSession ||
		got.BreakEveryAttacks != original.BreakEveryAttacks ||
		got.BreakMinutes != original.BreakMinutes ||
		got.AdaptiveSearch != original.AdaptiveSearch {
		t.Fatalf("settings were not restored: got=%+v want=%+v", got, original)
	}

	if _, err := os.Stat(restorePath); !os.IsNotExist(err) {
		t.Fatalf("temporary restore snapshot still exists after restoration: %v", err)
	}
}

func TestInterruptedTestSessionRecoveryIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLASHGO_CONFIG_DIR", dir)
	a := testLicensedApp(t, "CGO-TEST11-TEST12-TEST13-TEST14")

	original := defaultMemberSettings()
	original.MaxAttacksPerSession = 73
	original.BreakMinutes = 6

	if err := saveMemberProfileFile(a.testSessionRestorePath(), original); err != nil {
		t.Fatal(err)
	}
	if err := a.restoreTestSessionSettings(); err != nil {
		t.Fatalf("first recovery failed: %v", err)
	}
	if err := a.restoreTestSessionSettings(); err != nil {
		t.Fatalf("second recovery should be a no-op: %v", err)
	}

	got := a.GetMemberSettings()
	if got.MaxAttacksPerSession != 73 || got.BreakMinutes != 6 {
		t.Fatalf("recovered settings changed after idempotent restore: %+v", got)
	}
}


func TestMemberSettingsAreLockedWhileTemporaryTestSessionIsActive(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLASHGO_CONFIG_DIR", dir)
	a := testLicensedApp(t, "CGO-LOCK01-LOCK02-LOCK03-LOCK04")

	original := defaultMemberSettings()
	original.MaxAttacksPerSession = 42
	if _, err := a.SaveMemberSettings(original); err != nil {
		t.Fatalf("save original: %v", err)
	}
	if err := saveMemberProfileFile(a.testSessionRestorePath(), original); err != nil {
		t.Fatalf("save test restore snapshot: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.mu.Lock()
	a.botCtx = ctx
	a.cancel = cancel
	a.mu.Unlock()

	edited := original
	edited.MaxAttacksPerHour = 20
	if _, err := a.SaveMemberSettings(edited); err == nil {
		t.Fatal("expected member settings edit to be rejected while test session is active")
	}

	a.mu.Lock()
	a.cancel = nil
	a.botCtx = nil
	a.mu.Unlock()

	got := a.GetMemberSettings()
	if got.MaxAttacksPerHour != original.MaxAttacksPerHour {
		t.Fatalf("blocked edit leaked into config: got %d want %d", got.MaxAttacksPerHour, original.MaxAttacksPerHour)
	}
	if !a.testSessionRestorePending() {
		t.Fatal("test-session restore snapshot was unexpectedly removed")
	}
}


func TestMemberRuntimeStateIsolatesBootReport(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLASHGO_CONFIG_DIR", dir)
	a := testLicensedApp(t, "CGO-BOOT01-BOOT02-BOOT03-BOOT04")

	rel := filepath.Join("logs", "last_boot_report.json")
	shared := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(shared), 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"outcome":"failed","final_error":"member-specific"}`)
	if err := os.WriteFile(shared, want, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := a.archiveMemberRuntimeState(true); err != nil {
		t.Fatalf("archive runtime state: %v", err)
	}
	if _, err := os.Stat(shared); !os.IsNotExist(err) {
		t.Fatalf("shared boot report should be cleared after archive, err=%v", err)
	}

	if err := os.WriteFile(shared, []byte(`{"outcome":"failed","final_error":"other-member"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.restoreMemberRuntimeState(false); err != nil {
		t.Fatalf("restore runtime state: %v", err)
	}

	got, err := os.ReadFile(shared)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("boot report crossed member boundary: got=%s want=%s", got, want)
	}
}
