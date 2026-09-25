package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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


func TestDefaultSimplePreferencesAreSafeAndCoherent(t *testing.T) {
	cfg := DefaultConfig()
	prefs := cfg.Automation.Preferences
	if !prefs.UseHeroes || !prefs.UseClanCastle || !prefs.WaitForFullArmy || !prefs.AutoRetrain {
		t.Fatalf("expected safe automatic battle defaults, got %+v", prefs)
	}
	if prefs.AutoUpgradeWalls {
		t.Fatal("automatic wall spending must remain opt-in by default")
	}
	if !cfg.Attack.UseHeroes {
		t.Fatal("generic all-heroes policy should default on")
	}
	if !cfg.Attack.UseClanCastle {
		t.Fatal("Clan Castle use should default on")
	}
}

func TestLoadKeepsNewDefaultsWhenOlderConfigOmitsNewFields(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	strategies := filepath.Join(assets, "strategies")
	if err := os.MkdirAll(strategies, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(strategies, "auto_edrag_rush.yaml"), []byte("name: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLASHGO_ASSETS_DIR", assets)

	// Mimic an older config that has no attack.use_heroes or
	// preferences.auto_upgrade_walls fields.
	raw := []byte(`{
		"attack": {"strategy_file":"auto_edrag_rush.yaml"},
		"automation": {"simple_mode":true,"preferences":{"loot_preset":"balanced"}}
	}`)
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Attack.UseHeroes {
		t.Fatal("missing use_heroes in an older config should retain the new safe default")
	}
	if cfg.Automation.Preferences.AutoUpgradeWalls {
		t.Fatal("missing auto_upgrade_walls should remain opt-in/false")
	}
}


func TestLoadMigratesLegacyHeroAndWallIntent(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	strategies := filepath.Join(assets, "strategies")
	if err := os.MkdirAll(strategies, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(strategies, "auto_edrag_rush.yaml"), []byte("name: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLASHGO_ASSETS_DIR", assets)

	raw := []byte(`{
		"attack": {
			"strategy_file":"auto_edrag_rush.yaml",
			"use_queen":false,
			"use_warden":false
		},
		"upgrade":{"upgrade_walls":true},
		"automation":{
			"simple_mode":true,
			"preferences":{"loot_preset":"balanced"}
		}
	}`)
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Attack.UseHeroes {
		t.Fatal("legacy Queen/Warden=false should migrate to generic UseHeroes=false")
	}
	if !cfg.Automation.Preferences.AutoUpgradeWalls {
		t.Fatal("legacy upgrade_walls=true should migrate into simple wall preference")
	}
}

func TestLoadExplicitNewPreferencesWinOverLegacyFields(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	strategies := filepath.Join(assets, "strategies")
	if err := os.MkdirAll(strategies, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(strategies, "auto_edrag_rush.yaml"), []byte("name: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLASHGO_ASSETS_DIR", assets)

	raw := []byte(`{
		"attack": {
			"strategy_file":"auto_edrag_rush.yaml",
			"use_heroes":true,
			"use_queen":false,
			"use_warden":false
		},
		"upgrade":{"upgrade_walls":true},
		"automation":{
			"preferences":{
				"auto_upgrade_walls":false,
				"loot_preset":"balanced"
			}
		}
	}`)
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Attack.UseHeroes {
		t.Fatal("explicit new use_heroes must win over legacy hero flags")
	}
	if cfg.Automation.Preferences.AutoUpgradeWalls {
		t.Fatal("explicit auto_upgrade_walls=false must win over legacy upgrade_walls=true")
	}
}
