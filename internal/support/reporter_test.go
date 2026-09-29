package support

import (
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
