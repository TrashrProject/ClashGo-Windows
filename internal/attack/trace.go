package attack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ducky705/ClashGO/internal/paths"
)

type AttackTrace struct {
	Timestamp time.Time         `json:"timestamp"`
	Strategy  string            `json:"strategy"`
	Army      ArmyStateSnapshot `json:"army"`
}

func writeAttackTrace(strategy string, army *ArmyStateManager) {
	if army == nil {
		return
	}
	dir := paths.ResolveConfig("output/attack_traces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	name := time.Now().Format("20060102_150405.000") + ".json"
	trace := AttackTrace{
		Timestamp: time.Now(),
		Strategy: strings.TrimSpace(strategy),
		Army: army.Snapshot(),
	}
	data, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, name), data, 0o600)
}
