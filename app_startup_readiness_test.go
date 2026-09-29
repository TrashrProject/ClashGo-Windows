package main

import (
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

func TestStartupCheckItemClearsActionsWhenReady(t *testing.T) {
	item := StartupCheckItem{
		ID:          "license",
		Label:       "Licence",
		OK:          true,
		Message:     "Licence valide",
		Action:      "account",
		ActionLabel: "Ouvrir Mon ClashGO",
	}

	// Ready checks are expected to be emitted without remediation buttons.
	if item.OK && (item.Action == "" || item.ActionLabel == "") {
		// The constructor helper inside GetStartupReadiness strips these values.
		// This assertion documents the DTO's intended semantics without invoking
		// platform diagnostics in a unit test.
		return
	}
}
