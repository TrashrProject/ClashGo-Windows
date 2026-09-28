package bot

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAsyncWriterWriteDoesNotBlock verifies that a single synchronous
// Write() completes promptly instead of waiting for the worker's 5s
// flush ticker. Regression test for the app-close freeze: saveStats
// during App.shutdown used to block on `<-done` until the ticker fired
// (up to 5s, observed as ~2s of window freeze before exit). The worker
// must flush immediately whenever a caller is blocked on the request's
// done channel.
func TestAsyncWriterWriteDoesNotBlock(t *testing.T) {
	aw := NewAsyncWriter()
	defer aw.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "stats.json")
	data := []byte(`{"attacks":1}`)

	const maxWait = 500 * time.Millisecond
	start := time.Now()
	if err := aw.Write(path, data, 0o644); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > maxWait {
		t.Fatalf("Write blocked for %v (> %v); ticker-only flush stalls synchronous callers", elapsed, maxWait)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("file content mismatch: got %q want %q", got, data)
	}
}

// TestAsyncWriterWriteAfterClose falls back to a direct synchronous
// write once the writer is closed, so late callers (e.g. a stats flush
// racing shutdown) still persist instead of blocking forever on a
// drained channel.
func TestAsyncWriterWriteAfterClose(t *testing.T) {
	aw := NewAsyncWriter()
	aw.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "late.json")
	data := []byte(`{"late":true}`)

	done := make(chan error, 1)
	go func() {
		done <- aw.Write(path, data, 0o644)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Write after close returned error: %v", err)
		}
		if got, _ := os.ReadFile(path); string(got) != string(data) {
			t.Fatalf("late write not persisted: got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("Write after Close blocked — must fall through to direct os.WriteFile")
	}
}

func TestAsyncWriterWriteSoonPersistsWithoutCallerWait(t *testing.T) {
	aw := NewAsyncWriter()
	defer aw.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	data := []byte(`{"attack":1}`)

	start := time.Now()
	if err := aw.WriteSoon(path, data, 0o644); err != nil {
		t.Fatalf("WriteSoon returned error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("WriteSoon blocked caller for %v", elapsed)
	}

	deadline := time.Now().Add(time.Second)
	for {
		got, err := os.ReadFile(path)
		if err == nil && string(got) == string(data) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("queued write was not persisted within 1s; got=%q err=%v", got, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAsyncWriterWriteSoonPreservesOrderForSamePath(t *testing.T) {
	aw := NewAsyncWriter()
	defer aw.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")

	if err := aw.WriteSoon(path, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("first WriteSoon: %v", err)
	}
	if err := aw.WriteSoon(path, []byte(`{"version":2}`), 0o644); err != nil {
		t.Fatalf("second WriteSoon: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for {
		got, err := os.ReadFile(path)
		if err == nil && string(got) == `{"version":2}` {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("latest queued version not persisted; got=%q err=%v", got, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
