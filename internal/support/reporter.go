package support

import (
	"bufio"
	"bytes"
	"context"
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
	Level      string         `json:"level"`
	Message    string         `json:"message"`
	AppVersion string         `json:"app_version"`
	MachineID  string         `json:"machine_id"`
	Fields     map[string]any `json:"fields,omitempty"`
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

	ev := incident{
		At: time.Now().UTC().Format(time.RFC3339Nano),
		Level: level,
		Message: message,
		AppVersion: r.appVersion,
		Fields: sanitizeFields(raw),
	}
	if r.identity != nil {
		ev.MachineID = r.identity.MachineID()
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
	key := ev.Level + "\x00" + strings.TrimSpace(ev.Message) + "\x00" + surface

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
		out[k] = v
	}
	return out
}

func (r *Reporter) enqueue(ev incident) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = os.MkdirAll(filepath.Dir(r.queuePath), 0o755)
	f, err := os.OpenFile(r.queuePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(ev)
	_, _ = f.Write(append(b, '\n'))
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
	if r.baseURL == "" || r.identity == nil || r.identity.LicenseKey() == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := os.Open(r.queuePath)
	if err != nil {
		return
	}
	var pending []incident
	s := bufio.NewScanner(f)
	for s.Scan() {
		var ev incident
		if json.Unmarshal(s.Bytes(), &ev) == nil {
			pending = append(pending, ev)
		}
	}
	_ = f.Close()
	if len(pending) == 0 {
		return
	}

	keep := make([]incident, 0)
	for i, ev := range pending {
		body, _ := json.Marshal(ev)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/v1/support/incidents", bytes.NewReader(body))
		if err != nil {
			keep = append(keep, pending[i:]...)
			break
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-ClashGO-License", r.identity.LicenseKey())
		resp, err := r.client.Do(req)
		if err != nil {
			keep = append(keep, pending[i:]...)
			break
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			keep = append(keep, pending[i:]...)
			break
		}
	}

	if len(keep) == 0 {
		_ = os.Remove(r.queuePath)
		return
	}
	tmp := r.queuePath + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	for _, ev := range keep {
		b, _ := json.Marshal(ev)
		_, _ = out.Write(append(b, '\n'))
	}
	_ = out.Close()
	_ = os.Rename(tmp, r.queuePath)
}
