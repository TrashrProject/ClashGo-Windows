package support

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testIdentity struct {
	key     string
	machine string
}

func (i *testIdentity) LicenseKey() string { return i.key }
func (i *testIdentity) MachineID() string  { return i.machine }

func newTestIdentity() *testIdentity {
	return &testIdentity{key: "CGO-TESTAA-TESTBB-TESTCC-TESTDD", machine: "machine-a"}
}

func TestReporterDeduplicatesRepeatedErrors(t *testing.T) {
	dir := t.TempDir()
	r := &Reporter{
		appVersion: "test",
		queuePath:  filepath.Join(dir, "queue.jsonl"),
		recent:     make(map[string]time.Time),
		identity:   newTestIdentity(),
	}

	line := []byte(`{"level":"error","message":"same failure","surface":"frontend"}`)
	if _, err := r.Write(line); err != nil {
		t.Fatalf("first Write failed: %v", err)
	}
	if _, err := r.Write(line); err != nil {
		t.Fatalf("second Write failed: %v", err)
	}

	data, err := os.ReadFile(r.queuePath)
	if err != nil {
		t.Fatalf("read queue: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 queued incident, got %d", len(lines))
	}
}

func TestReporterKeepsDifferentErrors(t *testing.T) {
	dir := t.TempDir()
	r := &Reporter{
		appVersion: "test",
		queuePath:  filepath.Join(dir, "queue.jsonl"),
		recent:     make(map[string]time.Time),
		identity:   newTestIdentity(),
	}

	for _, line := range [][]byte{
		[]byte(`{"level":"error","message":"first failure","surface":"frontend"}`),
		[]byte(`{"level":"error","message":"second failure","surface":"frontend"}`),
	} {
		if _, err := r.Write(line); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	data, err := os.ReadFile(r.queuePath)
	if err != nil {
		t.Fatalf("read queue: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 queued incidents, got %d", len(lines))
	}
}

func TestSanitizeFieldsRemovesSensitiveKeys(t *testing.T) {
	got := sanitizeFields(map[string]any{
		"surface":       "frontend",
		"license_key":   "CGO-SECRET",
		"token":         "token-value",
		"password":      "password-value",
		"authorization": "Bearer value",
		"secret_value":  "secret",
	})

	if got["surface"] != "frontend" {
		t.Fatalf("safe field missing: %#v", got)
	}
	for _, key := range []string{"license_key", "token", "password", "authorization", "secret_value"} {
		if _, ok := got[key]; ok {
			t.Fatalf("sensitive field %q was not removed", key)
		}
	}
}


func TestTrimQueueDropsStaleAndCapsSize(t *testing.T) {
	now := time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)
	items := make([]incident, 0, maxQueuedIncidents+25)

	items = append(items, incident{
		At:      now.Add(-31 * 24 * time.Hour).Format(time.RFC3339Nano),
		Level:   "error",
		Message: "stale",
	})
	for i := 0; i < maxQueuedIncidents+24; i++ {
		items = append(items, incident{
			At:      now.Add(-time.Duration(i) * time.Minute).Format(time.RFC3339Nano),
			Level:   "error",
			Message: fmt.Sprintf("incident-%d", i),
		})
	}

	got := trimQueue(items, now)
	if len(got) != maxQueuedIncidents {
		t.Fatalf("trimQueue length = %d, want %d", len(got), maxQueuedIncidents)
	}
	for _, ev := range got {
		if ev.Message == "stale" {
			t.Fatal("stale incident was not removed")
		}
	}
	if got[len(got)-1].Message != fmt.Sprintf("incident-%d", maxQueuedIncidents+23) {
		t.Fatalf("trimQueue did not preserve newest tail, last=%q", got[len(got)-1].Message)
	}
}


func TestReporterQueueRewriteKeepsMultipleIncidents(t *testing.T) {
	dir := t.TempDir()
	r := &Reporter{
		appVersion: "test",
		queuePath:  filepath.Join(dir, "queue.jsonl"),
		recent:     make(map[string]time.Time),
		identity:   newTestIdentity(),
	}

	for _, msg := range []string{"one", "two", "three"} {
		line := []byte(`{"level":"error","message":"` + msg + `","surface":"backend"}`)
		if _, err := r.Write(line); err != nil {
			t.Fatalf("Write(%s) failed: %v", msg, err)
		}
	}

	items := r.readQueueLocked()
	if len(items) != 3 {
		t.Fatalf("expected 3 incidents after queue rewrites, got %d", len(items))
	}
	for i, want := range []string{"one", "two", "three"} {
		if items[i].Message != want {
			t.Fatalf("item %d message = %q, want %q", i, items[i].Message, want)
		}
	}
}

func TestTrimQueueCapsAndDropsOldIncidents(t *testing.T) {
	now := time.Now().UTC()
	items := make([]incident, 0, maxQueuedIncidents+3)
	items = append(items, incident{
		At:      now.Add(-maxIncidentAge-time.Hour).Format(time.RFC3339Nano),
		Message: "too old",
	})
	for i := 0; i < maxQueuedIncidents+2; i++ {
		items = append(items, incident{
			At:      now.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano),
			Message: "keep",
		})
	}

	got := trimQueue(items, now)
	if len(got) != maxQueuedIncidents {
		t.Fatalf("expected queue cap %d, got %d", maxQueuedIncidents, len(got))
	}
	if got[0].Message == "too old" {
		t.Fatal("expired incident survived trim")
	}
}

func TestReporterTruncatesOversizedMessageAndField(t *testing.T) {
	dir := t.TempDir()
	r := &Reporter{
		appVersion: "test",
		queuePath:  filepath.Join(dir, "queue.jsonl"),
		recent:     make(map[string]time.Time),
		identity:   newTestIdentity(),
	}
	longMessage := strings.Repeat("m", maxIncidentMessageBytes+500)
	longField := strings.Repeat("x", maxIncidentFieldBytes+500)
	line, err := json.Marshal(map[string]any{
		"level":   "error",
		"message": longMessage,
		"surface": "frontend",
		"detail":  longField,
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if _, err := r.Write(line); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	items := r.readQueueLocked()
	if len(items) != 1 {
		t.Fatalf("expected one incident, got %d", len(items))
	}
	if len(items[0].Message) > maxIncidentMessageBytes+3 {
		t.Fatalf("message was not truncated: %d bytes", len(items[0].Message))
	}
	detail, _ := items[0].Fields["detail"].(string)
	if len(detail) > maxIncidentFieldBytes+3 {
		t.Fatalf("field was not truncated: %d bytes", len(detail))
	}
}


func TestReporterDoesNotReattributeQueuedIncidentsAcrossLicenses(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity()

	type receivedIncident struct {
		license string
		body    string
	}
	received := make([]receivedIncident, 0, 2)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		received = append(received, receivedIncident{
			license: req.Header.Get("X-ClashGO-License"),
			body:    string(body),
		})
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	r := &Reporter{
		baseURL:    srv.URL,
		appVersion: "test",
		identity:   identity,
		client:     srv.Client(),
		queuePath:  filepath.Join(dir, "queue.jsonl"),
		recent:     make(map[string]time.Time),
	}

	// Member A queues an error while offline / before the periodic flush.
	if _, err := r.Write([]byte(`{"level":"error","message":"member A failure","surface":"backend"}`)); err != nil {
		t.Fatal(err)
	}

	// Switch locally to member B. A's queued error must NOT be posted using B.
	identity.key = "CGO-USERBB-AAAAAA-BBBBBB-CCCCCC"
	identity.machine = "machine-a"
	r.Flush(context.Background())
	if len(received) != 0 {
		t.Fatalf("member A incident was sent under member B: %#v", received)
	}

	// B can queue and flush its own incident even while A's remains pending.
	if _, err := r.Write([]byte(`{"level":"error","message":"member B failure","surface":"backend"}`)); err != nil {
		t.Fatal(err)
	}
	r.Flush(context.Background())
	if len(received) != 1 {
		t.Fatalf("received %d incidents, want only member B", len(received))
	}
	if received[0].license != identity.key || !strings.Contains(received[0].body, "member B failure") {
		t.Fatalf("wrong member B upload: %#v", received[0])
	}
	if strings.Contains(received[0].body, "_queue_owner") {
		t.Fatalf("local queue owner metadata leaked to server: %s", received[0].body)
	}

	pending := r.readQueueLocked()
	if len(pending) != 1 || pending[0].Message != "member A failure" {
		t.Fatalf("member A incident was not preserved for its owner: %#v", pending)
	}

	// Switching back to A permits only A's own queued incident to flush.
	identity.key = "CGO-TESTAA-TESTBB-TESTCC-TESTDD"
	r.Flush(context.Background())
	if len(received) != 2 {
		t.Fatalf("received %d incidents after switching back to A, want 2", len(received))
	}
	if received[1].license != identity.key || !strings.Contains(received[1].body, "member A failure") {
		t.Fatalf("wrong member A upload: %#v", received[1])
	}
	if items := r.readQueueLocked(); len(items) != 0 {
		t.Fatalf("queue not empty after both owners flushed: %#v", items)
	}
}

func TestReporterDropsUnattributablePreActivationErrors(t *testing.T) {
	dir := t.TempDir()
	identity := &testIdentity{}
	r := &Reporter{
		appVersion: "test",
		identity:   identity,
		queuePath:  filepath.Join(dir, "queue.jsonl"),
		recent:     make(map[string]time.Time),
	}

	if _, err := r.Write([]byte(`{"level":"error","message":"pre activation"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.queuePath); !os.IsNotExist(err) {
		t.Fatalf("pre-activation error should not create an attributable queue: %v", err)
	}
}


func TestReporterDeduplicationIsScopedPerLicense(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity()
	r := &Reporter{
		appVersion: "test",
		identity:   identity,
		queuePath:  filepath.Join(dir, "queue.jsonl"),
		recent:     make(map[string]time.Time),
	}

	line := []byte(`{"level":"error","message":"same failure","surface":"frontend"}`)
	if _, err := r.Write(line); err != nil {
		t.Fatal(err)
	}

	identity.key = "CGO-USERBB-AAAAAA-BBBBBB-CCCCCC"
	if _, err := r.Write(line); err != nil {
		t.Fatal(err)
	}

	items := r.readQueueLocked()
	if len(items) != 2 {
		t.Fatalf("same error from two licenses should produce two incidents, got %d", len(items))
	}
	if items[0].QueueOwner == items[1].QueueOwner {
		t.Fatal("incidents from different licenses share the same queue owner")
	}
}
