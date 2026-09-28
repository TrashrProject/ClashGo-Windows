package telemetry

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
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
)

type Event struct {
	Type       EventType        `json:"type"`
	At         time.Time        `json:"at"`
	SessionID  string           `json:"session_id,omitempty"`
	Fields     map[string]any   `json:"fields,omitempty"`
}

type Snapshot struct {
	Events             int64   `json:"events"`
	Searches           int64   `json:"searches"`
	TargetsFound       int64   `json:"targets_found"`
	TargetsSkipped     int64   `json:"targets_skipped"`
	AttacksFinished    int64   `json:"attacks_finished"`
	Recoveries         int64   `json:"recoveries"`
	AvgCaptureMS       float64 `json:"avg_capture_ms"`
	LastCaptureMS      float64 `json:"last_capture_ms"`
}

type Bus struct {
	sessionID string
	path      string
	ch        chan Event
	done      chan struct{}
	once      sync.Once

	events          atomic.Int64
	searches        atomic.Int64
	targetsFound    atomic.Int64
	targetsSkipped  atomic.Int64
	attacksFinished atomic.Int64
	recoveries      atomic.Int64
	captureCount    atomic.Int64
	captureMicros   atomic.Int64
	lastCaptureUS   atomic.Int64
}

func New(path string) *Bus {
	b := &Bus{
		sessionID: time.Now().UTC().Format("20060102T150405.000000000Z"),
		path:      path,
		ch:        make(chan Event, 256),
		done:      make(chan struct{}),
	}
	go b.writer()
	return b
}

func (b *Bus) Emit(t EventType, fields map[string]any) {
	if b == nil {
		return
	}
	ev := Event{Type: t, At: time.Now().UTC(), SessionID: b.sessionID, Fields: fields}
	b.record(ev)
	// High-frequency capture samples feed in-memory health metrics only.
	// Persisting every screenshot timing would create needless disk traffic
	// on the hottest loop and grow the event journal by thousands of rows.
	if t == EventCaptureSample {
		return
	}
	select {
	case b.ch <- ev:
	default:
		// Telemetry must never block the farming path. If the writer falls
		// behind, metrics remain correct and the event file may drop samples.
	}
}

func (b *Bus) record(ev Event) {
	b.events.Add(1)
	switch ev.Type {
	case EventSearchStarted:
		b.searches.Add(1)
	case EventTargetFound:
		b.targetsFound.Add(1)
	case EventTargetSkipped:
		b.targetsSkipped.Add(1)
	case EventAttackFinished:
		b.attacksFinished.Add(1)
	case EventRecovery:
		b.recoveries.Add(1)
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
	return Snapshot{
		Events:          b.events.Load(),
		Searches:        b.searches.Load(),
		TargetsFound:    b.targetsFound.Load(),
		TargetsSkipped:  b.targetsSkipped.Load(),
		AttacksFinished: b.attacksFinished.Load(),
		Recoveries:      b.recoveries.Load(),
		AvgCaptureMS:    avg,
		LastCaptureMS:   float64(b.lastCaptureUS.Load()) / 1000.0,
	}
}

func (b *Bus) Close() {
	if b == nil {
		return
	}
	b.once.Do(func() {
		close(b.done)
	})
}

func (b *Bus) writer() {
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
