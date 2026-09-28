package attack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Ducky705/ClashGO/internal/paths"
)

type AttackTrace struct {
	Timestamp time.Time          `json:"timestamp"`
	Strategy  string             `json:"strategy"`
	Army      ArmyStateSnapshot  `json:"army"`
	Events    []ArmyReplayEvent  `json:"events"`
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
		Events: army.ReplayEvents(),
	}
	data, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		return
	}
	pruneAttackTraces(dir, 200)
}

func pruneAttackTraces(dir string, keep int) {
	if keep <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	if len(names) <= keep {
		return
	}
	sort.Strings(names) // timestamp filenames sort oldest -> newest
	for _, name := range names[:len(names)-keep] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}
