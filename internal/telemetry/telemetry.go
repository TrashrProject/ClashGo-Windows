package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Ducky705/ClashGO/internal/paths"
)

// Event is the privacy-scrubbed envelope sent to the private developer endpoint.
type Event struct {
	InstallID string                 `json:"install_id"`
	Version   string                 `json:"version"`
	OS        string                 `json:"os"`
	Arch      string                 `json:"arch"`
	SentAt    string                 `json:"sent_at"`
	Log       map[string]interface{} `json:"log"`
}

// Writer implements io.Writer so it can be attached directly to zerolog's
// MultiLevelWriter. It never blocks the bot on network I/O: writes enqueue
// into a bounded buffer and a single background worker ships batches.
type Writer struct {
	endpoint string
	token    string
	version  string
	osName   string
	arch     string
	id       string
	client   *http.Client

	queue chan []byte
	stop  chan struct{}
	once  sync.Once
	wg    sync.WaitGroup
}

var (
	winUserPath = regexp.MustCompile(`(?i)[A-Z]:\\Users\\[^\\\s"']+`)
	unixUserPath = regexp.MustCompile(`/(Users|home)/[^/\s"']+`)
	emailLike = regexp.MustCompile(`(?i)[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}`)
	tokenKV = regexp.MustCompile(`(?i)(token|secret|password|passwd|api[_-]?key|authorization)["'=:\s]+[^\s",}]+`)
)

func NewFromEnv(version, osName, arch string) *Writer {
	// Remote telemetry is opt-in and only starts when BOTH values are present.
	// This lets us ship the client safely before enabling it in production/UI.
	if strings.TrimSpace(os.Getenv("CLASHGO_TELEMETRY_ENABLED")) != "1" {
		return nil
	}
	endpoint := strings.TrimSpace(os.Getenv("CLASHGO_TELEMETRY_URL"))
	if endpoint == "" {
		return nil
	}

	w := &Writer{
		endpoint: strings.TrimRight(endpoint, "/"),
		token:    strings.TrimSpace(os.Getenv("CLASHGO_TELEMETRY_TOKEN")),
		version:  version,
		osName:   osName,
		arch:     arch,
		id:       loadOrCreateInstallID(),
		client:   &http.Client{Timeout: 8 * time.Second},
		queue:    make(chan []byte, 1024),
		stop:     make(chan struct{}),
	}
	w.wg.Add(1)
	go w.loop()
	return w
}

func (w *Writer) Write(p []byte) (int, error) {
	if w == nil {
		return len(p), nil
	}
	cp := append([]byte(nil), p...)
	select {
	case w.queue <- cp:
	default:
		// Drop rather than ever slowing ADB / vision / attack timing.
	}
	return len(p), nil
}

func (w *Writer) Close() {
	if w == nil {
		return
	}
	w.once.Do(func() { close(w.stop) })
	w.wg.Wait()
}

func (w *Writer) loop() {
	defer w.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	batch := make([]Event, 0, 64)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		w.send(batch)
		batch = batch[:0]
	}

	for {
		select {
		case raw := <-w.queue:
			if ev, ok := w.makeEvent(raw); ok {
				batch = append(batch, ev)
				if len(batch) >= 64 {
					flush()
				}
			}
		case <-ticker.C:
			flush()
		case <-w.stop:
			for {
				select {
				case raw := <-w.queue:
					if ev, ok := w.makeEvent(raw); ok {
						batch = append(batch, ev)
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

func (w *Writer) makeEvent(raw []byte) (Event, bool) {
	line := strings.TrimSpace(string(raw))
	if line == "" {
		return Event{}, false
	}
	line = scrub(line)

	var fields map[string]interface{}
	if err := json.Unmarshal([]byte(line), &fields); err != nil {
		fields = map[string]interface{}{"message": line}
	}

	// Belt-and-suspenders: remove common sensitive structured fields entirely.
	for k := range fields {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "password") || strings.Contains(lk, "secret") ||
			strings.Contains(lk, "token") || strings.Contains(lk, "api_key") ||
			strings.Contains(lk, "authorization") {
			delete(fields, k)
		}
	}

	return Event{
		InstallID: w.id,
		Version:   w.version,
		OS:        w.osName,
		Arch:      w.arch,
		SentAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Log:       fields,
	}, true
}

func (w *Writer) send(events []Event) {
	payload, err := json.Marshal(map[string]interface{}{"events": events})
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, w.endpoint+"/v1/logs", bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if w.token != "" {
		req.Header.Set("X-ClashGO-Ingest", w.token)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
}

func scrub(s string) string {
	s = winUserPath.ReplaceAllString(s, `C:\Users\<redacted>`)
	s = unixUserPath.ReplaceAllString(s, "/$1/<redacted>")
	s = emailLike.ReplaceAllString(s, "<redacted-email>")
	s = tokenKV.ReplaceAllString(s, "$1=<redacted>")
	return s
}

func loadOrCreateInstallID() string {
	p := paths.ResolveConfig("telemetry_install_id")
	if b, err := os.ReadFile(p); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id
		}
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "unknown"
	}
	id := hex.EncodeToString(raw[:])
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(id+"\n"), 0o600)
	return id
}
