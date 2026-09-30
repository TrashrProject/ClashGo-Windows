package bot

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Ducky705/ClashGO/internal/paths"
	"github.com/Ducky705/ClashGO/internal/telemetry"
	"gocv.io/x/gocv"
)

// DiagnosticData holds the state of the bot at the time of failure.
type DiagnosticData struct {
	Timestamp       time.Time              `json:"timestamp"`
	Reason          string                 `json:"reason"`
	State           string                 `json:"state"`
	Context         map[string]interface{} `json:"context,omitempty"`
	RuntimeStats    BotStats               `json:"runtime_stats"`
	RecentActivity  []telemetry.Event      `json:"recent_activity,omitempty"`
	LastAction      time.Time              `json:"last_action,omitempty"`
	LastCapture     time.Time              `json:"last_capture,omitempty"`
	SequenceRunning bool                   `json:"sequence_running"`
}

// DumpDiagnostics saves a screenshot and a JSON file containing the bot's state.
func (b *Bot) DumpDiagnostics(reason string, screen gocv.Mat, context map[string]interface{}) error {
	timestamp := time.Now().Format("20060102_150405")
	baseName := fmt.Sprintf("diag_%s", timestamp)

	// Save screenshot from a detached privacy-safe clone. Vision keeps using the
	// original frame and is therefore never affected by the username mask.
	imgName := paths.ResolveConfig(baseName + ".png")
	var persisted gocv.Mat
	if !screen.Empty() {
		persisted = b.screenshotForPersistence(screen)
		if !persisted.Empty() {
			if ok := gocv.IMWrite(imgName, persisted); !ok {
				b.logger.Error().Str("file", imgName).Msg("failed to save diagnostic screenshot")
			} else {
				b.logger.Info().Str("file", imgName).Msg("saved diagnostic screenshot")
			}
		}
	}

	// Save JSON data
	data := DiagnosticData{
		Timestamp:       time.Now(),
		Reason:          reason,
		State:           "failed",
		Context:         context,
		RuntimeStats:    b.Stats(),
		RecentActivity:  b.RecentActivity(20),
		LastAction:      b.lastAction,
		LastCapture:     b.lastCapture,
		SequenceRunning: b.seqRunning.Load(),
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal diagnostic data: %w", err)
	}

	jsonName := paths.ResolveConfig(baseName + ".json")
	if err := os.WriteFile(jsonName, jsonData, 0644); err != nil {
		b.logger.Error().Err(err).Str("file", jsonName).Msg("failed to save diagnostic json")
		return err
	}

	b.logger.Info().Str("file", jsonName).Msg("saved diagnostic data")

	// Also symlink or copy to "last_failure" for easy access
	_ = os.Remove(paths.ResolveConfig("last_failure.png"))
	_ = os.Remove(paths.ResolveConfig("last_failure.json"))

	// Copy files to last_failure (safer than symlinks on some systems/setups)
	if !persisted.Empty() {
		_ = gocv.IMWrite(paths.ResolveConfig("last_failure.png"), persisted)
		persisted.Close()
	}
	_ = os.WriteFile(paths.ResolveConfig("last_failure.json"), jsonData, 0644)

	return nil
}
