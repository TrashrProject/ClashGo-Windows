package support

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Ducky705/ClashGO/internal/paths"
)

type IdentityProvider interface {
	LicenseKey() string
	MachineID() string
}

type incident struct {
	At         string         `json:"at"`
	QueueOwner string         `json:"_queue_owner,omitempty"`
	Level      string         `json:"level"`
	Message    string         `json:"message"`
	AppVersion string         `json:"app_version"`
	MachineID  string         `json:"machine_id"`
	Fields     map[string]any `json:"fields,omitempty"`
}

const (
	maxQueuedIncidents      = 500
	maxIncidentAge          = 30 * 24 * time.Hour
	maxIncidentMessageBytes = 4000
	maxIncidentFieldBytes   = 8000
)

func queueOwnerHash(licenseKey string) string {
	key := strings.ToUpper(strings.TrimSpace(licenseKey))
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func truncateText(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	if max == 1 {
		return value[:1]
	}
	return value[:max-1] + "…"
}

type Reporter struct {
	baseURL    string
	appVersion string
	identity   IdentityProvider
	client     *http.Client
	queuePath  string
	mu         sync.Mutex
	recentMu   sync.Mutex
	recent     map[string]time.Time
	stop       chan struct{}
}

func New(baseURL, appVersion string, identity IdentityProvider) *Reporter {
	r := &Reporter{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		appVersion: appVersion,
		identity: identity,
		client: &http.Client{Timeout: 8 * time.Second},
		queuePath: paths.ResolveConfig("support/error-queue.jsonl"),
		recent: make(map[string]time.Time),
		stop: make(chan struct{}),
	}
	go r.flushLoop()
	return r
}

func (r *Reporter) SetBaseURL(raw string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.baseURL = strings.TrimRight(strings.TrimSpace(raw), "/")
	r.mu.Unlock()
}

func (r *Reporter) Close() {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
}

func (r *Reporter) Write(p []byte) (int, error) {
	var raw map[string]any
	if json.Unmarshal(bytes.TrimSpace(p), &raw) != nil {
		return len(p), nil
	}
	level, _ := raw["level"].(string)
	level = strings.ToLower(level)
	if level != "error" && level != "fatal" && level != "panic" {
		return len(p), nil
	}
	message, _ := raw["message"].(string)
	delete(raw, "license_key")
	delete(raw, "token")
	delete(raw, "password")
	delete(raw, "authorization")

	if r.identity == nil {
		return len(p), nil
	}
	licenseKey := strings.TrimSpace(r.identity.LicenseKey())
	machineID := strings.TrimSpace(r.identity.MachineID())
	if licenseKey == "" || machineID == "" {
		// Pre-activation errors cannot be attributed safely later.
		return len(p), nil
	}

	ev := incident{
		At: time.Now().UTC().Format(time.RFC3339Nano),
		QueueOwner: queueOwnerHash(licenseKey),
		Level: level,
		Message: truncateText(message, maxIncidentMessageBytes),
		AppVersion: r.appVersion,
		MachineID: machineID,
		Fields: sanitizeFields(raw),
	}
	if !r.shouldQueue(ev) {
		return len(p), nil
	}
	r.enqueue(ev)
	return len(p), nil
}

func (r *Reporter) shouldQueue(ev incident) bool {
	now := time.Now()
	surface := ""
	if ev.Fields != nil {
		if value, ok := ev.Fields["surface"].(string); ok {
			surface = strings.TrimSpace(value)
		}
	}
	key := ev.QueueOwner + "\x00" + ev.Level + "\x00" + strings.TrimSpace(ev.Message) + "\x00" + surface

	r.recentMu.Lock()
	defer r.recentMu.Unlock()
	if r.recent == nil {
		r.recent = make(map[string]time.Time)
	}

	const window = 2 * time.Minute
	if last, ok := r.recent[key]; ok && now.Sub(last) < window {
		return false
	}
	r.recent[key] = now

	// Opportunistic cleanup keeps the map bounded during long sessions.
	if len(r.recent) > 256 {
		for k, seen := range r.recent {
			if now.Sub(seen) > 10*time.Minute {
				delete(r.recent, k)
			}
		}
	}
	return true
}

func sanitizeFields(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "token") || strings.Contains(lk, "password") || strings.Contains(lk, "secret") || strings.Contains(lk, "license") || strings.Contains(lk, "authorization") {
			continue
		}
		if text, ok := v.(string); ok {
			out[k] = truncateText(text, maxIncidentFieldBytes)
			continue
		}
		out[k] = v
	}
	return out
}

func (r *Reporter) writeQueueLocked(items []incident) bool {
	if len(items) == 0 {
		_ = os.Remove(r.queuePath)
		return true
	}
	tmp := r.queuePath + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	for _, item := range items {
		b, _ := json.Marshal(item)
		if _, err := out.Write(append(b, '\n')); err != nil {
			_ = out.Close()
			_ = os.Remove(tmp)
			return false
		}
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return false
	}

	// os.Rename does not replace an existing destination reliably on Windows.
	// The queue is already protected by r.mu, so remove the old destination
	// immediately before swapping the freshly-written temp file into place.
	_ = os.Remove(r.queuePath)
	if err := os.Rename(tmp, r.queuePath); err != nil {
		_ = os.Remove(tmp)
		return false
	}
	return true
}

func (r *Reporter) enqueue(ev incident) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = os.MkdirAll(filepath.Dir(r.queuePath), 0o755)

	pending := r.readQueueLocked()
	pending = append(pending, ev)
	pending = trimQueue(pending, time.Now().UTC())

	r.writeQueueLocked(pending)
}

func trimQueue(items []incident, now time.Time) []incident {
	if len(items) == 0 {
		return nil
	}
	cutoff := now.Add(-maxIncidentAge)
	kept := make([]incident, 0, len(items))
	for _, ev := range items {
		if ev.At != "" {
			if at, err := time.Parse(time.RFC3339Nano, ev.At); err == nil && at.Before(cutoff) {
				continue
			}
		}
		kept = append(kept, ev)
	}
	if len(kept) > maxQueuedIncidents {
		kept = kept[len(kept)-maxQueuedIncidents:]
	}
	return kept
}

func (r *Reporter) readQueueLocked() []incident {
	f, err := os.Open(r.queuePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	pending := make([]incident, 0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var ev incident
		if json.Unmarshal(scanner.Bytes(), &ev) == nil {
			pending = append(pending, ev)
		}
	}
	return pending
}

func (r *Reporter) flushLoop() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-t.C:
			r.Flush(context.Background())
		}
	}
}

func (r *Reporter) Flush(ctx context.Context) {
	if r.baseURL == "" || r.identity == nil {
		return
	}
	licenseKey := strings.TrimSpace(r.identity.LicenseKey())
	machineID := strings.TrimSpace(r.identity.MachineID())
	if licenseKey == "" || machineID == "" {
		return
	}
	currentOwner := queueOwnerHash(licenseKey)

	r.mu.Lock()
	defer r.mu.Unlock()

	pending := trimQueue(r.readQueueLocked(), time.Now().UTC())
	if len(pending) == 0 {
		_ = os.Remove(r.queuePath)
		return
	}

	keep := make([]incident, 0)
	for i, ev := range pending {
		// Queue routing metadata binds the incident to the licence that owned it
		// when the error occurred. Never send an old member's error under a
		// newly active licence. Legacy unowned rows are discarded because their
		// original owner cannot be proven safely.
		if ev.QueueOwner == "" {
			continue
		}
		if ev.QueueOwner != currentOwner || strings.TrimSpace(ev.MachineID) != machineID {
			keep = append(keep, ev)
			continue
		}

		send := ev
		send.QueueOwner = "" // local routing metadata must never leave the PC.
		body, _ := json.Marshal(send)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/v1/support/incidents", bytes.NewReader(body))
		if err != nil {
			keep = append(keep, ev)
			keep = append(keep, pending[i+1:]...)
			break
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-ClashGO-License", licenseKey)
		resp, err := r.client.Do(req)
		if err != nil {
			keep = append(keep, ev)
			keep = append(keep, pending[i+1:]...)
			break
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			keep = append(keep, ev)
			keep = append(keep, pending[i+1:]...)
			break
		}
	}

	if len(keep) == 0 {
		_ = os.Remove(r.queuePath)
		return
	}
	r.writeQueueLocked(keep)
}
