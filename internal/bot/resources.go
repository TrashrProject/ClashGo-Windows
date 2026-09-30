package bot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/Ducky705/ClashGO/internal/game"
	"github.com/Ducky705/ClashGO/internal/intelligence"
	"github.com/Ducky705/ClashGO/internal/paths"
	"gocv.io/x/gocv"
)

type VillageResourceHistory struct {
	Current game.VillageResourceSnapshot   `json:"current"`
	History []game.VillageResourceSnapshot `json:"history"`
}

func (b *Bot) maybeScanVillageResources(screen gocv.Mat) {
	if b == nil || b.resourceReader == nil || b.cfg == nil || !b.cfg.Automation.AutoResourceTracking {
		return
	}
	if time.Since(b.lastResourceScan) < 15*time.Second {
		return
	}
	b.lastResourceScan = time.Now()

	snap := b.resourceReader.Read(screen)
	if !snap.Valid {
		return
	}

	if b.villageMemory != nil {
		if err := b.villageMemory.UpdateResources(intelligence.VillageResources{
			Gold: snap.Gold,
			Elixir: snap.Elixir,
			DarkElixir: snap.DarkElixir,
			GoldValid: snap.GoldValid,
			ElixirValid: snap.ElixirValid,
			DarkValid: snap.DarkValid,
			UpdatedAt: snap.Timestamp,
		}); err != nil {
			b.logger.Debug().Err(err).Msg("could not update persistent village resource memory")
		}
	}

	data, _ := json.MarshalIndent(snap, "", "  ")

	// Multi-account sessions write only to the PlayerTag scope. The legacy
	// global files remain single-account-only so their trend history can never
	// become a mixture of several villages.
	accountPath := AccountVillageResourcesPath(b.cfg)
	if err := os.MkdirAll(filepath.Dir(accountPath), 0o755); err == nil {
		_ = os.WriteFile(accountPath, data, 0600)
	}
	if !b.cfg.Account.MultiAccount.Enabled {
		_ = os.WriteFile(paths.ResolveConfig("village_resources.json"), data, 0600)
	}

	writeHistory := func(historyPath string) {
		var history []game.VillageResourceSnapshot
		if old, err := os.ReadFile(historyPath); err == nil {
			_ = json.Unmarshal(old, &history)
		}

		// Avoid duplicate rows when the HUD did not change. We still refresh the
		// current snapshot timestamp, but the trend history only records changes.
		if len(history) == 0 ||
			history[len(history)-1].Gold != snap.Gold ||
			history[len(history)-1].Elixir != snap.Elixir ||
			history[len(history)-1].DarkElixir != snap.DarkElixir {
			history = append(history, snap)
			if len(history) > 1000 {
				history = history[len(history)-1000:]
			}
			histData, _ := json.MarshalIndent(history, "", "  ")
			if err := os.MkdirAll(filepath.Dir(historyPath), 0o755); err == nil {
				_ = os.WriteFile(historyPath, histData, 0600)
			}
		}
	}
	if !b.cfg.Account.MultiAccount.Enabled {
		writeHistory(paths.ResolveConfig("village_resource_history.json"))
	}
	writeHistory(AccountVillageResourceHistoryPath(b.cfg))
}
