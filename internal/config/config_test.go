package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadRebasesRelativeStrategyIntoAssets(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	strategies := filepath.Join(assets, "strategies")
	if err := os.MkdirAll(strategies, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(strategies, "auto_edrag_rush.yaml")
	if err := os.WriteFile(want, []byte("name: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLASHGO_ASSETS_DIR", assets)

	cfg := DefaultConfig()
	cfg.Attack.StrategyFile = "auto_edrag_rush.yaml"
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(configPath, b, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attack.StrategyFile != want {
		t.Fatalf("strategy path=%q want %q", got.Attack.StrategyFile, want)
	}
}

func TestLoadRebasesStaleAbsoluteStrategy(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	strategies := filepath.Join(assets, "strategies")
	if err := os.MkdirAll(strategies, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(strategies, "valk_spam.yaml")
	if err := os.WriteFile(want, []byte("name: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLASHGO_ASSETS_DIR", assets)

	cfg := DefaultConfig()
	cfg.Attack.StrategyFile = filepath.Join(root, "old-install", "assets", "strategies", "valk_spam.yaml")
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(configPath, b, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attack.StrategyFile != want {
		t.Fatalf("strategy path=%q want %q", got.Attack.StrategyFile, want)
	}
}

func TestDefaultAutomationIsSimpleAndAutomatic(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Automation.SimpleMode {
		t.Fatal("simple mode should be enabled by default")
	}
	if !cfg.Automation.AutoFarmProfile || !cfg.Automation.AutoArmyGuard ||
		!cfg.Automation.AutoResourceTracking || !cfg.Automation.AutoProfileSync {
		t.Fatalf("automatic behaviors should default on: %+v", cfg.Automation)
	}
	if _, ok := cfg.Attack.Farm.Profiles["18"]; !ok {
		t.Fatal("expected default TH18 farm profile")
	}
}


func TestDefaultClashCoreInspiredFeaturesAreSafe(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Automation.AutoCollectors {
		t.Fatal("collector tapping must stay opt-in until live calibration is confirmed")
	}
	if cfg.Automation.CollectorInterval.Duration != 10*time.Minute {
		t.Fatalf("collector interval=%v want 10m", cfg.Automation.CollectorInterval.Duration)
	}
	if !cfg.Automation.PrivacyMaskUsername {
		t.Fatal("persisted screenshots should mask the player identity by default")
	}
	if !cfg.Search.SaveAcceptedBaseScreenshots {
		t.Fatal("accepted target screenshots should be enabled by default")
	}
	if cfg.Attack.EndAtStars != 0 {
		t.Fatalf("star exit must remain opt-in, got %d", cfg.Attack.EndAtStars)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	cfg := DefaultConfig()
	cfg.Search.MinLootGold = 1234567
	cfg.Automation.SpeedProfile = "fast"
	cfg.Automation.MaxAttacksPerHour = 16

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Search.MinLootGold != 1234567 {
		t.Fatalf("MinLootGold=%d", got.Search.MinLootGold)
	}
	if got.Automation.SpeedProfile != "fast" || got.Automation.MaxAttacksPerHour != 16 {
		t.Fatalf("automation round trip mismatch: %+v", got.Automation)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary config file survived successful save: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup config file survived successful save: %v", err)
	}
}

func TestLoadRecoversValidBackupWhenPrimaryIsCorrupt(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")

	cfg := DefaultConfig()
	cfg.Search.MinLootElixir = 765432
	backup, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", backup, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load should recover backup: %v", err)
	}
	if got.Search.MinLootElixir != 765432 {
		t.Fatalf("recovered MinLootElixir=%d", got.Search.MinLootElixir)
	}
}

func TestLoadRejectsCorruptPrimaryAndBackup(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("{also-broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected Load to reject corrupt primary and backup")
	}
}


func TestDurationRejectsNonStringJSONWithoutPanic(t *testing.T) {
	var d Duration
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Duration.UnmarshalJSON panicked on malformed input: %v", r)
		}
	}()
	if err := json.Unmarshal([]byte(`123`), &d); err == nil {
		t.Fatal("expected numeric duration JSON to be rejected")
	}
	if err := json.Unmarshal([]byte(`"10m"`), &d); err != nil {
		t.Fatalf("valid duration string rejected: %v", err)
	}
	if d.Duration != 10*time.Minute {
		t.Fatalf("duration=%v want 10m", d.Duration)
	}
}

func TestLoadArchivesCorruptPrimaryWhenBackupRecovers(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	cfg := DefaultConfig()
	cfg.Automation.MaxRunMinutes = 90

	backup, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"automation":{"collector_interval":123}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", backup, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load should recover backup: %v", err)
	}
	if got.Automation.MaxRunMinutes != 90 {
		t.Fatalf("MaxRunMinutes=%d want 90", got.Automation.MaxRunMinutes)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("corrupt primary should have been moved aside, stat err=%v", err)
	}
	matches, err := filepath.Glob(filepath.Join(root, "config.corrupt.*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one preserved corrupt config, got %v", matches)
	}
}

func TestMaxRunMinutesRoundTrip(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	cfg := DefaultConfig()
	cfg.Automation.MaxRunMinutes = 135
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Automation.MaxRunMinutes != 135 {
		t.Fatalf("MaxRunMinutes=%d want 135", got.Automation.MaxRunMinutes)
	}
}
