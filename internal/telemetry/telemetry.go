package telemetry

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type EventType string

const (
	EventBotStarted      EventType = "bot_started"
	EventStateChanged    EventType = "state_changed"
	EventSearchStarted   EventType = "search_started"
	EventTargetFound     EventType = "target_found"
	EventTargetSkipped   EventType = "target_skipped"
	EventAttackStarted   EventType = "attack_started"
	EventAttackFinished  EventType = "attack_finished"
	EventRecovery        EventType = "recovery"
	EventCaptureSample   EventType = "capture_sample"
	EventSpeedProfile    EventType = "speed_profile"
	EventReturnHome      EventType = "return_home"
	EventAnomaly         EventType = "anomaly"
)

type Event struct {
	Type       EventType        `json:"type"`
	At         time.Time        `json:"at"`
	SessionID  string           `json:"session_id,omitempty"`
	Fields     map[string]any   `json:"fields,omitempty"`
}

type Incident struct {
	At       time.Time `json:"at"`
	Reason   string    `json:"reason"`
	Metrics  Snapshot  `json:"metrics"`
	Recent   []Event   `json:"recent_events"`
}

type Snapshot struct {
	Events             int64   `json:"events"`
	Searches           int64   `json:"searches"`
	TargetsFound       int64   `json:"targets_found"`
	TargetsAccepted    int64   `json:"targets_accepted"`
	TargetsSkipped     int64   `json:"targets_skipped"`
	AttacksFinished    int64   `json:"attacks_finished"`
	Recoveries         int64   `json:"recoveries"`
	Anomalies          int64   `json:"anomalies"`
	AvgCaptureMS       float64 `json:"avg_capture_ms"`
	LastCaptureMS      float64 `json:"last_capture_ms"`
	AvgTargetScanMS    float64 `json:"avg_target_scan_ms"`
	LastTargetScanMS   float64 `json:"last_target_scan_ms"`
	AvgNextTransitionMS float64 `json:"avg_next_transition_ms"`
	LastNextTransitionMS float64 `json:"last_next_transition_ms"`
	NextTransitions      int64   `json:"next_transitions"`
	NextRetries          int64   `json:"next_retries"`
	NextFirstPassRate    float64 `json:"next_first_pass_rate"`
	AvgNextVerifyProbes  float64 `json:"avg_next_verify_probes"`
	AvgAcceptedGE       float64 `json:"avg_accepted_ge"`
	AvgRejectedGE       float64 `json:"avg_rejected_ge"`
	AvgAcceptedDE       float64 `json:"avg_accepted_de"`
	AvgRejectedDE       float64 `json:"avg_rejected_de"`
	AvgAcceptedScore    float64 `json:"avg_accepted_score"`
	AvgRejectedScore    float64 `json:"avg_rejected_score"`
}

type Bus struct {
	sessionID string
	path      string
	ch        chan Event
	done      chan struct{}
	closed    chan struct{}
	once      sync.Once
	recentMu   sync.RWMutex
	recent     []Event

	events          atomic.Int64
	searches        atomic.Int64
	targetsFound    atomic.Int64
	targetsAccepted atomic.Int64
	targetsSkipped  atomic.Int64
	attacksFinished atomic.Int64
	recoveries      atomic.Int64
	anomalies       atomic.Int64
	captureCount     atomic.Int64
	captureMicros    atomic.Int64
	lastCaptureUS    atomic.Int64
	targetScanCount  atomic.Int64
	targetScanMicros atomic.Int64
	lastTargetScanUS   atomic.Int64
	nextTransitionCount atomic.Int64
	nextTransitionMicros atomic.Int64
	lastNextTransitionUS atomic.Int64
	nextRetries          atomic.Int64
	nextVerifyProbes     atomic.Int64

	acceptedGESum    atomic.Int64
	rejectedGESum    atomic.Int64
	acceptedDESum    atomic.Int64
	rejectedDESum    atomic.Int64
	acceptedScoreSum atomic.Int64
	rejectedScoreSum atomic.Int64
}

func New(path string) *Bus {
	if path != "" {
		rotateJournalIfOversize(path, 8*1024*1024)
	}
	b := &Bus{
		sessionID: time.Now().UTC().Format("20060102T150405.000000000Z"),
		path:      path,
		ch:        make(chan Event, 256),
		done:      make(chan struct{}),
		closed:    make(chan struct{}),
	}
	go b.writer()
	return b
}

func rotateJournalIfOversize(path string, maxBytes int64) {
	if path == "" || maxBytes <= 0 {
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() <= maxBytes {
		return
	}
	_ = os.Remove(path + ".1")
	_ = os.Rename(path, path+".1")
}

func pruneIncidentDir(dir string, keep int) {
	if dir == "" || keep < 1 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	files := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		files = append(files, entry)
	}
	if len(files) <= keep {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name() > files[j].Name() })
	for _, entry := range files[keep:] {
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
}

func (b *Bus) SessionID() string {
	if b == nil {
		return ""
	}
	return b.sessionID
}

func (b *Bus) Emit(t EventType, fields map[string]any) {
	if b == nil {
		return
	}
	ev := Event{Type: t, At: time.Now().UTC(), SessionID: b.sessionID, Fields: fields}
	b.record(ev)
	// High-frequency samples stay in memory only. Persisting every capture,
	// rejected target and skip would create constant disk churn on the farming
	// hot path while adding little diagnostic value. Accepted targets and
	// attack/recovery lifecycle events remain journaled.
	if t == EventCaptureSample || t == EventTargetSkipped {
		return
	}
	if t == EventTargetFound {
		if accepted, ok := fields["accept"].(bool); ok && !accepted {
			return
		}
	}
	b.remember(ev)
	select {
	case b.ch <- ev:
	default:
		// Telemetry must never block the farming path. If the writer falls
		// behind, metrics remain correct and the event file may drop samples.
	}
}

func (b *Bus) remember(ev Event) {
	b.recentMu.Lock()
	b.recent = append(b.recent, ev)
	if len(b.recent) > 80 {
		copy(b.recent, b.recent[len(b.recent)-80:])
		b.recent = b.recent[:80]
	}
	b.recentMu.Unlock()
}

// Recent returns the newest high-level events first. Capture samples are
// intentionally excluded so this remains an understandable activity feed,
// not another noisy log stream.
func (b *Bus) Recent(limit int) []Event {
	if b == nil || limit <= 0 {
		return []Event{}
	}
	b.recentMu.RLock()
	defer b.recentMu.RUnlock()
	if limit > len(b.recent) {
		limit = len(b.recent)
	}
	out := make([]Event, 0, limit)
	for i := len(b.recent) - 1; i >= len(b.recent)-limit; i-- {
		out = append(out, b.recent[i])
	}
	return out
}

func (b *Bus) record(ev Event) {
	b.events.Add(1)
	switch ev.Type {
	case EventSearchStarted:
		b.searches.Add(1)
	case EventTargetFound:
		b.targetsFound.Add(1)
		accepted, _ := ev.Fields["accept"].(bool)
		gold, _ := telemetryInt64(ev.Fields["gold"])
		elixir, _ := telemetryInt64(ev.Fields["elixir"])
		de, _ := telemetryInt64(ev.Fields["de"])
		score, _ := telemetryInt64(ev.Fields["score"])
		if accepted {
			b.targetsAccepted.Add(1)
			b.acceptedGESum.Add(gold + elixir)
			b.acceptedDESum.Add(de)
			b.acceptedScoreSum.Add(score)
		} else {
			b.rejectedGESum.Add(gold + elixir)
			b.rejectedDESum.Add(de)
			b.rejectedScoreSum.Add(score)
		}
		if raw, ok := ev.Fields["scan_us"]; ok {
			switch v := raw.(type) {
			case int64:
				b.targetScanCount.Add(1)
				b.targetScanMicros.Add(v)
				b.lastTargetScanUS.Store(v)
			case int:
				b.targetScanCount.Add(1)
				b.targetScanMicros.Add(int64(v))
				b.lastTargetScanUS.Store(int64(v))
			}
		}
	case EventTargetSkipped:
		b.targetsSkipped.Add(1)
		if retry, ok := ev.Fields["retry_used"].(bool); ok && retry {
			b.nextRetries.Add(1)
		}
		if probes, ok := telemetryInt64(ev.Fields["verify_probes"]); ok {
			b.nextVerifyProbes.Add(probes)
		}
		if raw, ok := ev.Fields["transition_us"]; ok {
			switch v := raw.(type) {
			case int64:
				b.nextTransitionCount.Add(1)
				b.nextTransitionMicros.Add(v)
				b.lastNextTransitionUS.Store(v)
			case int:
				b.nextTransitionCount.Add(1)
				b.nextTransitionMicros.Add(int64(v))
				b.lastNextTransitionUS.Store(int64(v))
			case float64:
				b.nextTransitionCount.Add(1)
				b.nextTransitionMicros.Add(int64(v))
				b.lastNextTransitionUS.Store(int64(v))
			}
		}
	case EventAttackFinished:
		b.attacksFinished.Add(1)
	case EventRecovery:
		b.recoveries.Add(1)
	case EventAnomaly:
		b.anomalies.Add(1)
	case EventCaptureSample:
		if raw, ok := ev.Fields["duration_us"]; ok {
			switch v := raw.(type) {
			case int64:
				b.captureCount.Add(1)
				b.captureMicros.Add(v)
				b.lastCaptureUS.Store(v)
			case int:
				b.captureCount.Add(1)
				b.captureMicros.Add(int64(v))
				b.lastCaptureUS.Store(int64(v))
			}
		}
	}
}

func telemetryInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func (b *Bus) Snapshot() Snapshot {
	if b == nil {
		return Snapshot{}
	}
	count := b.captureCount.Load()
	avg := 0.0
	if count > 0 {
		avg = float64(b.captureMicros.Load()) / float64(count) / 1000.0
	}
	scanCount := b.targetScanCount.Load()
	avgScan := 0.0
	if scanCount > 0 {
		avgScan = float64(b.targetScanMicros.Load()) / float64(scanCount) / 1000.0
	}
	nextCount := b.nextTransitionCount.Load()
	avgNext := 0.0
	nextFirstPassRate := 0.0
	avgNextVerifyProbes := 0.0
	if nextCount > 0 {
		avgNext = float64(b.nextTransitionMicros.Load()) / float64(nextCount) / 1000.0
		nextFirstPassRate = float64(nextCount-b.nextRetries.Load()) * 100 / float64(nextCount)
		avgNextVerifyProbes = float64(b.nextVerifyProbes.Load()) / float64(nextCount)
	}
	acceptedCount := b.targetsAccepted.Load()
	rejectedCount := b.targetsFound.Load() - acceptedCount
	avgAcceptedGE, avgRejectedGE := 0.0, 0.0
	avgAcceptedDE, avgRejectedDE := 0.0, 0.0
	avgAcceptedScore, avgRejectedScore := 0.0, 0.0
	if acceptedCount > 0 {
		avgAcceptedGE = float64(b.acceptedGESum.Load()) / float64(acceptedCount)
		avgAcceptedDE = float64(b.acceptedDESum.Load()) / float64(acceptedCount)
		avgAcceptedScore = float64(b.acceptedScoreSum.Load()) / float64(acceptedCount)
	}
	if rejectedCount > 0 {
		avgRejectedGE = float64(b.rejectedGESum.Load()) / float64(rejectedCount)
		avgRejectedDE = float64(b.rejectedDESum.Load()) / float64(rejectedCount)
		avgRejectedScore = float64(b.rejectedScoreSum.Load()) / float64(rejectedCount)
	}
	return Snapshot{
		Events:          b.events.Load(),
		Searches:        b.searches.Load(),
		TargetsFound:    b.targetsFound.Load(),
		TargetsAccepted: b.targetsAccepted.Load(),
		TargetsSkipped:  b.targetsSkipped.Load(),
		AttacksFinished: b.attacksFinished.Load(),
		Recoveries:      b.recoveries.Load(),
		Anomalies:       b.anomalies.Load(),
		AvgCaptureMS:     avg,
		LastCaptureMS:    float64(b.lastCaptureUS.Load()) / 1000.0,
		AvgTargetScanMS:  avgScan,
		LastTargetScanMS: float64(b.lastTargetScanUS.Load()) / 1000.0,
		AvgNextTransitionMS: avgNext,
		LastNextTransitionMS: float64(b.lastNextTransitionUS.Load()) / 1000.0,
		NextTransitions: nextCount,
		NextRetries: b.nextRetries.Load(),
		NextFirstPassRate: nextFirstPassRate,
		AvgNextVerifyProbes: avgNextVerifyProbes,
		AvgAcceptedGE: avgAcceptedGE,
		AvgRejectedGE: avgRejectedGE,
		AvgAcceptedDE: avgAcceptedDE,
		AvgRejectedDE: avgRejectedDE,
		AvgAcceptedScore: avgAcceptedScore,
		AvgRejectedScore: avgRejectedScore,
	}
}

// WriteIncident persists a compact black-box snapshot only on exceptional
// paths. Normal farming never pays this disk-write cost.
func (b *Bus) WriteIncident(reason string) string {
	if b == nil || b.path == "" {
		return ""
	}
	clean := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_").Replace(strings.TrimSpace(reason))
	if clean == "" {
		clean = "incident"
	}
	if len(clean) > 64 {
		clean = clean[:64]
	}
	incident := Incident{
		At:      time.Now().UTC(),
		Reason:  reason,
		Metrics: b.Snapshot(),
		Recent:  b.Recent(60),
	}
	data, err := json.MarshalIndent(incident, "", "  ")
	if err != nil {
		return ""
	}
	dir := filepath.Join(filepath.Dir(b.path), "incidents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	name := incident.At.Format("20060102T150405.000000000Z") + "_" + clean + ".json"
	out := filepath.Join(dir, name)
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return ""
	}
	// Incident writes are exceptional/non-hot-path, so pruning here has zero
	// cost during normal farming. Keep the newest 24 black-box snapshots.
	pruneIncidentDir(dir, 24)
	return out
}

func (b *Bus) Close() {
	if b == nil {
		return
	}
	b.once.Do(func() {
		close(b.done)
	})
	// Wait until the writer flushed and released the file handle. This keeps
	// shutdown deterministic and prevents tests / app restarts from racing a
	// still-open telemetry journal.
	<-b.closed
}

func (b *Bus) writer() {
	defer close(b.closed)
	if b.path == "" {
		for {
			select {
			case <-b.ch:
			case <-b.done:
				return
			}
		}
	}
	_ = os.MkdirAll(filepath.Dir(b.path), 0o755)
	f, err := os.OpenFile(b.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		for {
			select {
			case <-b.ch:
			case <-b.done:
				return
			}
		}
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 64*1024)
	defer w.Flush()

	flushTicker := time.NewTicker(2 * time.Second)
	defer flushTicker.Stop()

	for {
		select {
		case ev := <-b.ch:
			if data, err := json.Marshal(ev); err == nil {
				_, _ = w.Write(data)
				_ = w.WriteByte('\n')
			}
		case <-flushTicker.C:
			_ = w.Flush()
		case <-b.done:
			for {
				select {
				case ev := <-b.ch:
					if data, err := json.Marshal(ev); err == nil {
						_, _ = w.Write(data)
						_ = w.WriteByte('\n')
					}
				default:
					_ = w.Flush()
					return
				}
			}
		}
	}
}
