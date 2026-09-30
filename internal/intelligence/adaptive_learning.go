package intelligence

import (
	"crypto/sha256"
	"encoding/hex"
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

type LearningMode string

const (
	LearningShadow   LearningMode = "shadow"
	LearningStable   LearningMode = "stable"
	LearningAdaptive LearningMode = "learning"
)

type EnvironmentFingerprint struct {
	OS       string
	Emulator string
	DeviceID string
	Width    int
	Height   int
	DPI      int
	Strategy string
	TownHall int
}

func (f EnvironmentFingerprint) Key() string {
	raw := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(f.OS)),
		strings.ToLower(strings.TrimSpace(f.Emulator)),
		strings.ToLower(strings.TrimSpace(f.DeviceID)),
		strings.TrimSpace(f.Strategy),
		itoa(f.Width), itoa(f.Height), itoa(f.DPI), itoa(f.TownHall),
	}, "|")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:8])
}

type ParameterBounds struct {
	Min     float64
	Max     float64
	MaxStep float64
}

var DefaultSafeBounds = map[string]ParameterBounds{
	"search_capture_ms":   {Min: 450, Max: 1200, MaxStep: 75},
	"planning_capture_ms": {Min: 200, Max: 600, MaxStep: 50},
	"deploy_capture_ms":   {Min: 250, Max: 700, MaxStep: 50},
	"card_settle_ms":      {Min: 100, Max: 250, MaxStep: 15},
	"red_zone_margin_px":  {Min: 50, Max: 110, MaxStep: 5},
	"camera_zoom_steps":   {Min: 1, Max: 3, MaxStep: 1},
	"camera_pan_steps":       {Min: 0, Max: 2, MaxStep: 1},
	"wall_search_attempt":    {Min: 0, Max: 12, MaxStep: 1},
	"builder_open_settle_ms": {Min: 900, Max: 2000, MaxStep: 100},
	"wall_focus_settle_ms":   {Min: 1500, Max: 4000, MaxStep: 250},
	"wall_scroll_step_ms":    {Min: 300, Max: 1200, MaxStep: 100},
}

type LearningOutcome struct {
	Domain            string
	Parameters        map[string]float64
	Clean             bool
	DeploySuccess     bool
	ReturnHomeSuccess bool
	SafeDeployment    bool
	ParsedResults     bool
	RecoveryCount     int
	BlueStacksRestart int
	PlanningMS        int64
	DeployMS          int64
	CaptureMS         float64
	RewardOverride    *float64
	At                time.Time
}

func RewardForOutcome(o LearningOutcome) float64 {
	if o.RewardOverride != nil {
		return clamp(*o.RewardOverride, -100, 100)
	}
	score := 0.0
	if o.Clean {
		score += 45
	}
	if o.DeploySuccess {
		score += 20
	}
	if o.ReturnHomeSuccess {
		score += 12
	}
	if o.SafeDeployment {
		score += 10
	}
	if o.ParsedResults {
		score += 5
	}
	if o.PlanningMS > 0 {
		switch {
		case o.PlanningMS <= 3000:
			score += 8
		case o.PlanningMS <= 5000:
			score += 4
		default:
			score -= math.Min(15, float64(o.PlanningMS-5000)/500)
		}
	}
	if o.DeployMS > 0 && o.DeployMS <= 15000 {
		score += 3
	}
	score -= float64(o.RecoveryCount) * 30
	score -= float64(o.BlueStacksRestart) * 75
	if !o.DeploySuccess {
		score -= 25
	}
	if !o.ReturnHomeSuccess {
		score -= 15
	}
	return clamp(score, -100, 100)
}

type ParameterStat struct {
	Samples       int
	MeanReward    float64
	BestReward    float64
	BestValue     float64
	LastGoodValue float64
	LastValue     float64
	FailureStreak int
}

type DomainProfile struct {
	Samples       int
	CleanSamples  int
	MeanReward    float64
	BestReward    float64
	FailureStreak int
	Parameters    map[string]*ParameterStat
}

type EnvironmentProfile struct {
	Fingerprint EnvironmentFingerprint
	Mode        LearningMode
	Domains     map[string]*DomainProfile
	UpdatedAt   time.Time
	Rollbacks   int
}

type AdaptiveState struct {
	Version  int
	Profiles map[string]*EnvironmentProfile
}

type AdaptiveEngine struct {
	mu     sync.Mutex
	path   string
	key    string
	state  AdaptiveState
	bounds map[string]ParameterBounds
}

type Suggestion struct {
	Parameter string
	Current   float64
	Proposed  float64
	Apply     bool
	Mode      LearningMode
	Reason    string
}

func NewAdaptiveEngine(path string, env EnvironmentFingerprint) (*AdaptiveEngine, error) {
	e := &AdaptiveEngine{
		path:   path,
		key:    env.Key(),
		state:  AdaptiveState{Version: 1, Profiles: map[string]*EnvironmentProfile{}},
		bounds: copyBounds(DefaultSafeBounds),
	}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &e.state); err != nil {
			return nil, err
		}
		if e.state.Profiles == nil {
			e.state.Profiles = map[string]*EnvironmentProfile{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if _, ok := e.state.Profiles[e.key]; !ok {
		e.state.Profiles[e.key] = &EnvironmentProfile{
			Fingerprint: env,
			Mode:        LearningShadow,
			Domains:     map[string]*DomainProfile{},
			UpdatedAt:   time.Now(),
		}
	}
	return e, nil
}

func (e *AdaptiveEngine) SetMode(mode LearningMode) error {
	if mode != LearningShadow && mode != LearningStable && mode != LearningAdaptive {
		return errors.New("invalid learning mode")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.profileLocked()
	p.Mode = mode
	p.UpdatedAt = time.Now()
	return e.saveLocked()
}

func (e *AdaptiveEngine) Mode() LearningMode {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.profileLocked().Mode
}

func (e *AdaptiveEngine) Observe(o LearningOutcome) (float64, error) {
	if strings.TrimSpace(o.Domain) == "" {
		o.Domain = "general"
	}
	if o.At.IsZero() {
		o.At = time.Now()
	}
	reward := RewardForOutcome(o)

	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.profileLocked()
	d := p.Domains[o.Domain]
	if d == nil {
		d = &DomainProfile{BestReward: -101, Parameters: map[string]*ParameterStat{}}
		p.Domains[o.Domain] = d
	}
	d.Samples++
	if o.Clean {
		d.CleanSamples++
	}
	d.MeanReward += (reward - d.MeanReward) / float64(d.Samples)
	if reward > d.BestReward {
		d.BestReward = reward
	}
	if reward < 0 || o.RecoveryCount > 0 || o.BlueStacksRestart > 0 || !o.DeploySuccess {
		d.FailureStreak++
	} else {
		d.FailureStreak = 0
	}

	keys := make([]string, 0, len(o.Parameters))
	for k := range o.Parameters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		value := o.Parameters[name]
		b, ok := e.bounds[name]
		if !ok {
			continue
		}
		value = clamp(value, b.Min, b.Max)
		ps := d.Parameters[name]
		if ps == nil {
			ps = &ParameterStat{BestReward: -101, BestValue: value, LastGoodValue: value}
			d.Parameters[name] = ps
		}
		ps.Samples++
		ps.MeanReward += (reward - ps.MeanReward) / float64(ps.Samples)
		ps.LastValue = value
		if reward > ps.BestReward {
			ps.BestReward = reward
			ps.BestValue = value
		}
		if reward >= 55 && o.Clean {
			ps.LastGoodValue = value
			ps.FailureStreak = 0
		} else if reward < 0 || o.RecoveryCount > 0 || o.BlueStacksRestart > 0 {
			ps.FailureStreak++
		}
	}

	if p.Mode == LearningAdaptive && d.FailureStreak >= 2 {
		p.Mode = LearningStable
		p.Rollbacks++
	}
	p.UpdatedAt = time.Now()
	return reward, e.saveLocked()
}

func (e *AdaptiveEngine) Suggest(domain, parameter string, current float64) Suggestion {
	e.mu.Lock()
	defer e.mu.Unlock()

	p := e.profileLocked()
	s := Suggestion{Parameter: parameter, Current: current, Proposed: current, Mode: p.Mode}
	b, ok := e.bounds[parameter]
	if !ok {
		s.Reason = "parameter not tunable"
		return s
	}
	current = clamp(current, b.Min, b.Max)
	s.Current, s.Proposed = current, current

	d := p.Domains[domain]
	if d == nil {
		s.Reason = "no domain history"
		return s
	}
	ps := d.Parameters[parameter]
	if ps == nil || ps.Samples < 6 {
		s.Reason = "insufficient samples"
		return s
	}

	target := ps.BestValue
	if p.Mode == LearningStable && ps.LastGoodValue != 0 {
		target = ps.LastGoodValue
	}
	delta := clamp(target-current, -b.MaxStep, b.MaxStep)
	s.Proposed = clamp(current+delta, b.Min, b.Max)

	switch p.Mode {
	case LearningShadow:
		s.Apply = false
		s.Reason = "shadow recommendation only"
	case LearningStable:
		s.Apply = math.Abs(s.Proposed-current) > 0.001
		s.Reason = "returning toward last known-good value"
	case LearningAdaptive:
		s.Apply = math.Abs(s.Proposed-current) > 0.001
		s.Reason = "bounded move toward best observed value"
	}
	return s
}

func (e *AdaptiveEngine) Snapshot() EnvironmentProfile {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.profileLocked()
	data, _ := json.Marshal(p)
	var out EnvironmentProfile
	_ = json.Unmarshal(data, &out)
	return out
}

func (e *AdaptiveEngine) profileLocked() *EnvironmentProfile {
	p := e.state.Profiles[e.key]
	if p == nil {
		p = &EnvironmentProfile{Mode: LearningShadow, Domains: map[string]*DomainProfile{}}
		e.state.Profiles[e.key] = p
	}
	if p.Domains == nil {
		p.Domains = map[string]*DomainProfile{}
	}
	return p
}

func (e *AdaptiveEngine) saveLocked() error {
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

func copyBounds(in map[string]ParameterBounds) map[string]ParameterBounds {
	out := make(map[string]ParameterBounds, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [24]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
