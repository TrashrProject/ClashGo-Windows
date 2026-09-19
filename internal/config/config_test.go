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
