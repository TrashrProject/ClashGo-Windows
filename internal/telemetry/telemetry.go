package telemetry

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
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
}

func New(path string) *Bus {
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
		if accepted, ok := ev.Fields["accept"].(bool); ok && accepted {
			b.targetsAccepted.Add(1)
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
	if nextCount > 0 {
		avgNext = float64(b.nextTransitionMicros.Load()) / float64(nextCount) / 1000.0
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
