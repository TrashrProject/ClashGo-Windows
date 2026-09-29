package main

import (
	"os"
	"path/filepath"
	"strings"
	"strings"
	"testing"
	"time"

	"github.com/Ducky705/ClashGO/internal/config"
)

func TestMemberRuntimeConfigReadyAcceptsValidMemberPacing(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Automation.SpeedProfile = "normal"
	cfg.Automation.MaxAttacksPerHour = 12
	cfg.Attack.MaxAttackPerSession = 50
	cfg.Automation.BreakEveryAttacks = 5
	cfg.Automation.BreakDuration = config.Duration{Duration: 3 * time.Minute}

	ok, message := memberRuntimeConfigReady(cfg)
	if !ok {
		t.Fatalf("expected valid member pacing, got %q", message)
	}
	if !strings.Contains(message, "Normale") || !strings.Contains(message, "12 attaques/h") || !strings.Contains(message, "50/session") {
		t.Fatalf("unexpected readiness summary: %q", message)
	}
}

func TestMemberRuntimeConfigReadyRejectsInvalidBounds(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*config.BotConfig)
		want   string
	}{
		{
			name: "speed",
			mutate: func(cfg *config.BotConfig) {
				cfg.Automation.SpeedProfile = "turbo"
			},
			want: "Profil de vitesse invalide",
		},
		{
			name: "hourly cap",
			mutate: func(cfg *config.BotConfig) {
				cfg.Automation.MaxAttacksPerHour = 25
			},
			want: "attaques par heure",
		},
		{
			name: "session cap",
			mutate: func(cfg *config.BotConfig) {
				cfg.Attack.MaxAttackPerSession = 0
			},
			want: "attaques par session",
		},
		{
			name: "break cadence",
			mutate: func(cfg *config.BotConfig) {
				cfg.Automation.BreakEveryAttacks = 21
			},
			want: "fréquence des pauses",
		},
		{
			name: "break duration",
			mutate: func(cfg *config.BotConfig) {
				cfg.Automation.BreakDuration = config.Duration{Duration: 31 * time.Minute}
			},
			want: "durée des pauses",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Automation.SpeedProfile = "normal"
			cfg.Automation.MaxAttacksPerHour = 12
			cfg.Attack.MaxAttackPerSession = 50
			cfg.Automation.BreakEveryAttacks = 5
			cfg.Automation.BreakDuration = config.Duration{Duration: 3 * time.Minute}
			tc.mutate(cfg)

			ok, message := memberRuntimeConfigReady(cfg)
			if ok {
				t.Fatalf("expected invalid pacing, got ready with %q", message)
			}
			if !strings.Contains(strings.ToLower(message), strings.ToLower(tc.want)) {
				t.Fatalf("message %q does not contain %q", message, tc.want)
			}
		})
	}
}




func TestNewStartupCheckItemClearsRemediationWhenReady(t *testing.T) {
	item := newStartupCheckItem(
		"license",
		"Licence",
		true,
		"Licence valide",
		"account",
		"Ouvrir Mon ClashGO",
	)
	if !item.OK {
		t.Fatal("ready check unexpectedly marked as blocked")
	}
	if item.Action != "" || item.ActionLabel != "" {
		t.Fatalf("ready check leaked remediation action: %+v", item)
	}
}

func TestNewStartupCheckItemKeepsRemediationWhenBlocked(t *testing.T) {
	item := newStartupCheckItem(
		"adb",
		"ADB",
		false,
		"ADB n’est pas détecté",
		"settings",
		"Configurer ADB",
	)
	if item.OK {
		t.Fatal("blocked check unexpectedly marked ready")
	}
	if item.Action != "settings" || item.ActionLabel != "Configurer ADB" {
		t.Fatalf("blocked check lost remediation action: %+v", item)
	}
}


func TestStartupAdvisoryDoesNotBlockReadiness(t *testing.T) {
	item := newStartupAdvisoryItem(
		"account",
		"Compte Clash",
		false,
		"Aucun tag joueur lié · optionnel",
		"account",
		"Lier le compte",
	)
	if item.OK {
		t.Fatal("missing optional account should remain visible as not configured")
	}
	if item.Blocking {
		t.Fatal("optional Clash account unexpectedly blocks startup")
	}
	if item.Action != "account" || item.ActionLabel == "" {
		t.Fatalf("optional account lost its action: %+v", item)
	}
}

func TestStartupFailureRemainsBlockingByDefault(t *testing.T) {
	item := newStartupCheckItem(
		"strategy",
		"Stratégie",
		false,
		"Fichier introuvable",
		"automation",
		"Ouvrir Automatisation",
	)
	if !item.Blocking {
		t.Fatalf("required startup failure became advisory: %+v", item)
	}
}


func TestAttackStrategyReady(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Attack.StrategyFile = ""
	if ok, _ := attackStrategyReady(cfg); ok {
		t.Fatal("empty strategy unexpectedly reported ready")
	}

	cfg.Attack.StrategyFile = filepath.Join(t.TempDir(), "missing.yaml")
	if ok, _ := attackStrategyReady(cfg); ok {
		t.Fatal("missing strategy unexpectedly reported ready")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "farm.yaml")
	if err := os.WriteFile(path, []byte("name: test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg.Attack.StrategyFile = path
	ok, message := attackStrategyReady(cfg)
	if !ok {
		t.Fatalf("existing strategy reported unavailable: %s", message)
	}
	if !strings.Contains(message, "farm.yaml") {
		t.Fatalf("strategy readiness message does not identify selected file: %q", message)
	}
}
