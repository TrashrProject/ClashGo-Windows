package support

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReporterDeduplicatesRepeatedErrors(t *testing.T) {
	dir := t.TempDir()
	r := &Reporter{
		appVersion: "test",
		queuePath:  filepath.Join(dir, "queue.jsonl"),
		recent:     make(map[string]time.Time),
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
