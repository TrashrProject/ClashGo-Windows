package main

import (
	"testing"
	"time"

	"github.com/Ducky705/ClashGO/internal/config"
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
			cfg.Attack.StrategyFile = "keep-me.yaml"

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
