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

// AttackContext is deliberately built only from data ClashGO already owns.
// Intelligence V3 never asks BlueStacks for an extra screenshot just to learn.
type AttackContext struct {
	Strategy    string `json:"strategy"`
	TownHall    int    `json:"town_hall"`
	TargetScore int    `json:"target_score"`
	TargetGold  int    `json:"target_gold"`
	TargetElixir int   `json:"target_elixir"`
	TargetDE    int    `json:"target_de"`
}

func (c AttackContext) Key() string {
	strategy := strings.ToLower(strings.TrimSpace(c.Strategy))
	if strategy == "" {
		strategy = "unknown"
	}
	// Coarse buckets are intentional: nearby opponents should transfer
	// knowledge instead of each creating a tiny, isolated profile.
	scoreBucket := bucketInt(c.TargetScore, 25)
	lootBucket := bucketInt(c.TargetGold+c.TargetElixir, 500000)
	darkBucket := bucketInt(c.TargetDE, 5000)
	return strings.Join([]string{
		"ctx",
		strategy,
		itoa(c.TownHall),
		itoa(scoreBucket),
		itoa(lootBucket),
		itoa(darkBucket),
	}, "|")
}

func (c AttackContext) FamilyKey() string {
	strategy := strings.ToLower(strings.TrimSpace(c.Strategy))
	if strategy == "" {
		strategy = "unknown"
	}
	return strings.Join([]string{"family", strategy, itoa(c.TownHall)}, "|")
}

type ContextualOutcome struct {
	Context              AttackContext `json:"context"`
	Edge                 string        `json:"edge"`
	Stars                int           `json:"stars"`
	DestructionPct       int           `json:"destruction_pct"`
	GoldStolen           int           `json:"gold_stolen"`
	ElixirStolen         int           `json:"elixir_stolen"`
	DarkElixirStolen     int           `json:"dark_elixir_stolen"`
	CycleDurationMS      int64         `json:"cycle_duration_ms"`
	FullRoutineDurationMS int64        `json:"full_routine_duration_ms"`
	DeploySuccess        bool          `json:"deploy_success"`
	ReturnHomeSuccess    bool          `json:"return_home_success"`
	SafeDeployment       bool          `json:"safe_deployment"`
	ParsedResults        bool          `json:"parsed_results"`
	RecoveryCount        int           `json:"recovery_count"`
	BlueStacksRestart    int           `json:"bluestacks_restart"`
	At                   time.Time     `json:"at"`
}

func weightedFarmResources(gold, elixir, dark int) float64 {
	// Dark elixir is two orders of magnitude scarcer than gold/elixir. Giving
	// it a 100x conversion lets the learner compare mixed-loot attacks without
	// allowing a tiny raw DE number to disappear next to million-scale G/E.
	return float64(maxIntLearning(gold, 0)+maxIntLearning(elixir, 0)) +
		float64(maxIntLearning(dark, 0))*100
}

func FarmResourcesPerHour(o ContextualOutcome) float64 {
	elapsedMS := o.FullRoutineDurationMS
	if elapsedMS <= 0 {
		elapsedMS = o.CycleDurationMS
	}
	if elapsedMS <= 0 {
		return 0
	}
	return weightedFarmResources(o.GoldStolen, o.ElixirStolen, o.DarkElixirStolen) *
		3600000 / float64(elapsedMS)
}

func RewardForContextualOutcome(o ContextualOutcome) float64 {
	// Intelligence V3 is a FARM optimizer. The primary objective is how much
	// loot is extracted and how quickly the full attack cycle finishes.
	// Stars/destruction are intentionally only tiny secondary signals.
	stolen := weightedFarmResources(o.GoldStolen, o.ElixirStolen, o.DarkElixirStolen)
	available := weightedFarmResources(o.Context.TargetGold, o.Context.TargetElixir, o.Context.TargetDE)

	captureRatio := 0.0
	if available > 0 {
		captureRatio = clamp(stolen/available, 0, 1.25)
	}

	// 50 points: proportion of available loot actually collected.
	score := captureRatio * 50

	// 35 points: absolute farm throughput. The reference rate is deliberately
	// not a hard requirement; values above it continue receiving some credit,
	// capped so stability can never be traded away for reckless speed.
	if rate := FarmResourcesPerHour(o); rate > 0 {
		const referenceWeightedPerHour = 30_000_000.0
		score += clamp(rate/referenceWeightedPerHour, 0, 1.35) * 35
	} else if stolen > 0 {
		// Old history rows may predate full-cycle timing. They still contribute
		// through loot capture, but cannot falsely appear faster than live data.
		score += math.Min(10, stolen/250000)
	}

	// Stars/destruction are not the farming target. They only break close ties
	// when two attacks yield similar loot/time.
	stars := o.Stars
	if stars < 0 {
		stars = 0
	}
	if stars > 3 {
		stars = 3
	}
	score += float64(stars) * 0.75
	pct := o.DestructionPct
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	score += float64(pct) * 0.015

	if o.DeploySuccess {
		score += 3
	} else {
		score -= 30
	}
	if o.ReturnHomeSuccess {
		score += 3
	} else {
		score -= 25
	}
	if o.SafeDeployment {
		score += 4
	} else {
		score -= 15
	}
	if o.ParsedResults {
		score += 1
	}

	// Emulator stability stays a hard constraint. A strategy that earns a lot
	// but destabilizes BlueStacks must never become champion.
	score -= float64(o.RecoveryCount) * 25
	score -= float64(o.BlueStacksRestart) * 100
	return clamp(score, -100, 100)
}

func maxIntLearning(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type EdgeLearningStat struct {
	Samples       int       `json:"samples"`
	MeanReward    float64   `json:"mean_reward"`
	EWMAReward    float64   `json:"ewma_reward"`
	BestReward    float64   `json:"best_reward"`
	FailureStreak int       `json:"failure_streak"`
	CleanStreak   int       `json:"clean_streak"`
	LastSeen      time.Time `json:"last_seen"`
}

type ContextLearningProfile struct {
	Samples int                          `json:"samples"`
	Edges   map[string]*EdgeLearningStat `json:"edges"`
}

type ContextualLearningState struct {
	Version     int                                `json:"version"`
	Profiles    map[string]*ContextLearningProfile `json:"profiles"`
	Experiences []ContextualOutcome                `json:"experiences,omitempty"`
}

type ContextualEngine struct {
	mu    sync.Mutex
	path  string
	state ContextualLearningState
}

type EdgeRecommendation struct {
	Edge            string  `json:"edge"`
	Apply           bool    `json:"apply"`
	Exploratory     bool    `json:"exploratory"`
	Confidence      float64 `json:"confidence"`
	Reason          string  `json:"reason"`
	ContextSamples  int     `json:"context_samples"`
	ChampionReward  float64 `json:"champion_reward"`
	ProfileScope    string  `json:"profile_scope"`
}

func NewContextualEngine(path string) (*ContextualEngine, error) {
	e := &ContextualEngine{
		path: path,
		state: ContextualLearningState{
			Version:  1,
			Profiles: map[string]*ContextLearningProfile{},
		},
	}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &e.state); err != nil {
			return nil, err
		}
		if e.state.Profiles == nil {
			e.state.Profiles = map[string]*ContextLearningProfile{}
		}
		if e.state.Version <= 0 {
			e.state.Version = 1
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return e, nil
}

func (e *ContextualEngine) TotalSamples() int {
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.state.Experiences)
}

func (e *ContextualEngine) Observe(o ContextualOutcome) (float64, error) {
	if e == nil {
		return 0, nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	reward := e.observeLocked(o)
	return reward, e.saveLocked()
}

func (e *ContextualEngine) ObserveMany(outcomes []ContextualOutcome) error {
	if e == nil || len(outcomes) == 0 {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, o := range outcomes {
		e.observeLocked(o)
	}
	return e.saveLocked()
}

func (e *ContextualEngine) observeLocked(o ContextualOutcome) float64 {
	o.Edge = normalizeLearningEdge(o.Edge)
	if o.Edge == "" {
		return 0
	}
	if o.At.IsZero() {
		o.At = time.Now()
	}
	reward := RewardForContextualOutcome(o)
	for _, key := range []string{o.Context.Key(), o.Context.FamilyKey()} {
		e.updateProfileLocked(key, o.Edge, reward, o)
	}
	e.state.Experiences = append(e.state.Experiences, o)
	if len(e.state.Experiences) > 256 {
		e.state.Experiences = append([]ContextualOutcome(nil), e.state.Experiences[len(e.state.Experiences)-256:]...)
	}
	return reward
}

func (e *ContextualEngine) updateProfileLocked(key, edge string, reward float64, o ContextualOutcome) {
	p := e.state.Profiles[key]
	if p == nil {
		p = &ContextLearningProfile{Edges: map[string]*EdgeLearningStat{}}
		e.state.Profiles[key] = p
	}
	if p.Edges == nil {
		p.Edges = map[string]*EdgeLearningStat{}
	}
	p.Samples++
	stat := p.Edges[edge]
	if stat == nil {
		stat = &EdgeLearningStat{BestReward: -101}
		p.Edges[edge] = stat
	}
	stat.Samples++
	stat.MeanReward += (reward - stat.MeanReward) / float64(stat.Samples)
	if stat.Samples == 1 {
		stat.EWMAReward = reward
	} else {
		const alpha = 0.35
		stat.EWMAReward = alpha*reward + (1-alpha)*stat.EWMAReward
	}
	if reward > stat.BestReward {
		stat.BestReward = reward
	}
	failed := reward < 0 || !o.DeploySuccess || !o.ReturnHomeSuccess ||
		o.RecoveryCount > 0 || o.BlueStacksRestart > 0
	if failed {
		stat.FailureStreak++
		stat.CleanStreak = 0
	} else {
		stat.FailureStreak = 0
		stat.CleanStreak++
	}
	stat.LastSeen = o.At
}

func (e *ContextualEngine) RecommendEdge(ctx AttackContext, current string, candidates []string, allowExplore bool) EdgeRecommendation {
	rec := EdgeRecommendation{Edge: current, Reason: "learning disabled for fixed target edge"}
	if e == nil {
		return rec
	}

	mode := strings.ToLower(strings.TrimSpace(current))
	adaptiveTarget := mode == "adaptive"
	if mode != "rotate" && mode != "random" && !adaptiveTarget {
		return rec
	}

	valid := normalizeCandidateEdges(candidates)
	if len(valid) == 0 {
		rec.Reason = "no valid candidate edges"
		return rec
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	key := ctx.Key()
	scope := "context"
	profile := e.state.Profiles[key]
	if profile == nil || profile.Samples < 4 {
		key = ctx.FamilyKey()
		scope = "family"
		profile = e.state.Profiles[key]
	}
	if profile == nil || profile.Samples == 0 {
		if adaptiveTarget {
			rec.Edge = deterministicEdge(ctx.Key(), valid)
			rec.Apply = true
			rec.Exploratory = true
			rec.Reason = "adaptive cold-start exploration"
			rec.ProfileScope = scope
		} else {
			rec.Reason = "insufficient contextual history; keeping existing rotate/random behavior"
		}
		return rec
	}

	type candidateScore struct {
		edge  string
		stat  *EdgeLearningStat
		score float64
	}
	scored := make([]candidateScore, 0, len(valid))
	for _, edge := range valid {
		stat := profile.Edges[edge]
		if stat == nil {
			scored = append(scored, candidateScore{edge: edge})
			continue
		}
		score := 0.65*stat.EWMAReward + 0.35*stat.MeanReward
		score -= float64(stat.FailureStreak) * 14
		score += math.Min(6, float64(stat.CleanStreak))
		scored = append(scored, candidateScore{edge: edge, stat: stat, score: score})
	}

	// Learn every legal edge once in explicit Adaptive mode. Rotate/Random
	// retain their original behavior until real evidence exists.
	if adaptiveTarget && allowExplore {
		for _, c := range scored {
			if c.stat == nil || c.stat.Samples == 0 {
				rec.Edge = c.edge
				rec.Apply = true
				rec.Exploratory = true
				rec.ContextSamples = profile.Samples
				rec.ProfileScope = scope
				rec.Reason = "challenger has no samples yet"
				return rec
			}
		}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		si, sj := scored[i], scored[j]
		if si.stat == nil {
			return false
		}
		if sj.stat == nil {
			return true
		}
		if si.score == sj.score {
			return si.stat.Samples > sj.stat.Samples
		}
		return si.score > sj.score
	})

	var champion *candidateScore
	for i := range scored {
		if scored[i].stat != nil && scored[i].stat.Samples >= 2 && scored[i].stat.FailureStreak < 2 {
			champion = &scored[i]
			break
		}
	}
	if champion == nil {
		if adaptiveTarget {
			rec.Edge = deterministicEdge(ctx.Key()+itoa(profile.Samples), valid)
			rec.Apply = true
			rec.Exploratory = true
			rec.ContextSamples = profile.Samples
			rec.ProfileScope = scope
			rec.Reason = "building initial champion evidence"
		} else {
			rec.Reason = "not enough safe repeated samples for a champion"
		}
		return rec
	}

	// Bounded challenger exploration. It is deterministic and sparse: at most
	// one in six observations, only for an edge with no active failure streak.
	// The caller disables this completely whenever BlueStacks safety pacing is
	// active, so learning never competes with emulator recovery.
	if allowExplore && profile.Samples%6 == 5 {
		var challenger *candidateScore
		for i := range scored {
			c := &scored[i]
			if c.edge == champion.edge || c.stat == nil || c.stat.FailureStreak > 0 {
				continue
			}
			if c.stat.Samples < champion.stat.Samples {
				if challenger == nil || c.stat.Samples < challenger.stat.Samples {
					challenger = c
				}
			}
		}
		if challenger != nil {
			rec.Edge = challenger.edge
			rec.Apply = true
			rec.Exploratory = true
			rec.ContextSamples = profile.Samples
			rec.ChampionReward = champion.score
			rec.ProfileScope = scope
			rec.Confidence = clamp(float64(champion.stat.Samples)/6, 0, 1)
			rec.Reason = "bounded champion/challenger exploration"
			return rec
		}
	}

	rec.Edge = champion.edge
	rec.Apply = true
	rec.ContextSamples = profile.Samples
	rec.ChampionReward = champion.score
	rec.ProfileScope = scope
	rec.Confidence = clamp(float64(champion.stat.Samples)/6, 0, 1)
	rec.Reason = "contextual champion from recent and historical outcomes"
	return rec
}

func normalizeLearningEdge(edge string) string {
	switch strings.ToLower(strings.TrimSpace(edge)) {
	case "topleft":
		return "TopLeft"
	case "topright":
		return "TopRight"
	case "bottomleft":
		return "BottomLeft"
	case "bottomright":
		return "BottomRight"
	default:
		return ""
	}
}

func normalizeCandidateEdges(edges []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(edges))
	for _, edge := range edges {
		normalized := normalizeLearningEdge(edge)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	return out
}

func deterministicEdge(key string, edges []string) string {
	if len(edges) == 0 {
		return ""
	}
	var h uint64 = 1469598103934665603
	for i := 0; i < len(key); i++ {
		h ^= uint64(key[i])
		h *= 1099511628211
	}
	return edges[int(h%uint64(len(edges)))]
}

func bucketInt(v, size int) int {
	if size <= 0 {
		return v
	}
	if v < 0 {
		v = 0
	}
	return (v / size) * size
}

func (e *ContextualEngine) saveLocked() error {
	if e.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(e.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(e.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := e.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	_ = os.Remove(e.path)
	return os.Rename(tmp, e.path)
}
