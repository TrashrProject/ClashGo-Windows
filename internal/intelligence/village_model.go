package intelligence

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type VillagePoint struct {
	X int
	Y int
}

type VillageEntity struct {
	ID         string
	Kind       string
	Level      int
	Position   VillagePoint
	Confidence float64
	LastSeen   time.Time
	LastUsed   time.Time
	Failures   int
}

type VillageResources struct {
	Gold       int
	Elixir     int
	DarkElixir int
	GoldValid  bool
	ElixirValid bool
	DarkValid  bool
	UpdatedAt  time.Time
}

type BuilderState struct {
	Available int
	Total     int
	UpdatedAt time.Time
	Valid     bool
}

type VillageState struct {
	Version   int
	Resources VillageResources
	Builders  BuilderState
	Entities  map[string]VillageEntity
	UpdatedAt time.Time
}

type VillageMemory struct {
	mu    sync.RWMutex
	path  string
	state VillageState
}

func NewVillageMemory(path string) (*VillageMemory, error) {
	m := &VillageMemory{
		path: path,
		state: VillageState{
			Version:  1,
			Entities: map[string]VillageEntity{},
		},
	}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &m.state); err != nil {
			return nil, err
		}
		if m.state.Entities == nil {
			m.state.Entities = map[string]VillageEntity{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return m, nil
}

func (m *VillageMemory) UpdateResources(r VillageResources) error {
	if m == nil {
		return nil
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = time.Now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Resources = r
	m.state.UpdatedAt = r.UpdatedAt
	return m.saveLocked()
}

func (m *VillageMemory) UpdateBuilders(b BuilderState) error {
	if m == nil {
		return nil
	}
	if b.UpdatedAt.IsZero() {
		b.UpdatedAt = time.Now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Builders = b
	m.state.UpdatedAt = b.UpdatedAt
	return m.saveLocked()
}

func (m *VillageMemory) UpsertEntity(entity VillageEntity) error {
	if m == nil || strings.TrimSpace(entity.ID) == "" {
		return nil
	}
	entity.Confidence = clamp(entity.Confidence, 0, 1)
	if entity.LastSeen.IsZero() {
		entity.LastSeen = time.Now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.state.Entities[entity.ID]; ok {
		// Smooth positional observations instead of jumping to one noisy match.
		if old.Confidence > 0 && entity.Confidence > 0 {
			wOld := old.Confidence
			wNew := entity.Confidence
			entity.Position.X = int(math.Round((float64(old.Position.X)*wOld + float64(entity.Position.X)*wNew) / (wOld + wNew)))
			entity.Position.Y = int(math.Round((float64(old.Position.Y)*wOld + float64(entity.Position.Y)*wNew) / (wOld + wNew)))
			if entity.Level <= 0 {
				entity.Level = old.Level
			}
			if entity.Kind == "" {
				entity.Kind = old.Kind
			}
		}
	}
	m.state.Entities[entity.ID] = entity
	m.state.UpdatedAt = time.Now()
	return m.saveLocked()
}

func (m *VillageMemory) MarkEntityResult(id string, success bool) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entity, ok := m.state.Entities[id]
	if !ok {
		return nil
	}
	entity.LastUsed = time.Now()
	if success {
		entity.Failures = 0
		entity.Confidence = clamp(entity.Confidence+0.04, 0, 1)
	} else {
		entity.Failures++
		entity.Confidence = clamp(entity.Confidence-0.18, 0, 1)
	}
	m.state.Entities[id] = entity
	m.state.UpdatedAt = time.Now()
	return m.saveLocked()
}

func (m *VillageMemory) KnownEntity(id string, maxAge time.Duration, minConfidence float64) (VillageEntity, bool) {
	if m == nil {
		return VillageEntity{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	entity, ok := m.state.Entities[id]
	if !ok || entity.Confidence < minConfidence {
		return VillageEntity{}, false
	}
	if maxAge > 0 && time.Since(entity.LastSeen) > maxAge {
		return VillageEntity{}, false
	}
	if entity.Failures >= 2 {
		return VillageEntity{}, false
	}
	return entity, true
}

func (m *VillageMemory) KnownByKind(kind string, maxAge time.Duration, minConfidence float64) []VillageEntity {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]VillageEntity, 0)
	for _, entity := range m.state.Entities {
		if !strings.EqualFold(strings.TrimSpace(entity.Kind), strings.TrimSpace(kind)) {
			continue
		}
		if entity.Confidence < minConfidence || entity.Failures >= 2 {
			continue
		}
		if maxAge > 0 && time.Since(entity.LastSeen) > maxAge {
			continue
		}
		out = append(out, entity)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Confidence == out[j].Confidence {
			return out[i].LastSeen.After(out[j].LastSeen)
		}
		return out[i].Confidence > out[j].Confidence
	})
	return out
}

func (m *VillageMemory) Snapshot() VillageState {
	if m == nil {
		return VillageState{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, _ := json.Marshal(m.state)
	var out VillageState
	_ = json.Unmarshal(data, &out)
	return out
}

func (m *VillageMemory) saveLocked() error {
	if m.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	_ = os.Remove(m.path)
	return os.Rename(tmp, m.path)
}
