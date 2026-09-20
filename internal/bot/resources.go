package bot

import (
	"encoding/json"
	"os"
	"time"

	"github.com/Ducky705/ClashGO/internal/game"
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

	path := paths.ResolveConfig("village_resources.json")
	data, _ := json.MarshalIndent(snap, "", "  ")
	_ = os.WriteFile(path, data, 0600)

	historyPath := paths.ResolveConfig("village_resource_history.json")
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
		_ = os.WriteFile(historyPath, histData, 0600)
	}
}
