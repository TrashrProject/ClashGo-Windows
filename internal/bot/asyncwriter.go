package bot

import (
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type writeRequest struct {
	path     string
	data     []byte
	perms    os.FileMode
	done     chan error
	flushNow bool
}

type AsyncWriter struct {
	requests chan writeRequest
	wg       sync.WaitGroup
	closed   bool
	mu       sync.Mutex
}

func NewAsyncWriter() *AsyncWriter {
	aw := &AsyncWriter{
		requests: make(chan writeRequest, 100),
	}
	aw.wg.Add(1)
	go aw.worker()
	return aw
}

func (aw *AsyncWriter) worker() {
	defer aw.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	pending := make([]writeRequest, 0, 10)

	flush := func() {
		for _, req := range pending {
			if err := os.WriteFile(req.path, req.data, req.perms); err != nil {
				log.Error().Err(err).Str("path", req.path).Msg("async write failed")
				if req.done != nil {
					req.done <- err
				}
			} else if req.done != nil {
				req.done <- nil
			}
		}
		pending = pending[:0]
	}

	for {
		select {
		case req, ok := <-aw.requests:
			if !ok {
				flush()
				return
			}
			pending = append(pending, req)
			// Flush immediately whenever a caller is BLOCKED waiting on
			// this request's done channel (Write() → `return <-done`).
			// Previously the worker only flushed on the 5s ticker or a
			// full 10-item batch, so a single synchronous write (e.g.
			// saveStats during app shutdown) could stall its caller for
			// up to 5 seconds — observed as the app window freezing for
			// ~2s after clicking close before it finally exited.
			// Fire-and-forget requests (done == nil) still batch and
			// ride the ticker, so high-frequency best-effort writes keep
			// their coalescing.
			if req.done != nil || req.flushNow || len(pending) >= 10 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (aw *AsyncWriter) Write(path string, data []byte, perms os.FileMode) error {
	// Hold mu across the closed-check AND the channel send. The worker
	// never takes mu, so this can't deadlock, and it closes the race
	// where a concurrent Close() closed the channel between the check
	// and the send — which would panic with "send on closed channel"
	// and kill the whole app process (a close-click crash waiting to
	// happen). If the worker ever drained the buffer and Close() is
	// blocked on wg.Wait(), it still can't deadlock: Close's wg.Wait
	// runs only after it releases mu, which Write releases the moment
	// the send lands.
	aw.mu.Lock()
	if aw.closed {
		aw.mu.Unlock()
		return os.WriteFile(path, data, perms)
	}

	done := make(chan error, 1)
	select {
	case aw.requests <- writeRequest{path: path, data: data, perms: perms, done: done}:
		aw.mu.Unlock()
		return <-done
	default:
		aw.mu.Unlock()
		return os.WriteFile(path, data, perms)
	}
}

// WriteSoon queues an immediate-flush write without waiting for disk I/O.
// It is intended for hot-path snapshots such as attack_history.json where the
// in-memory state is already authoritative for the UI. Shutdown-critical
// persistence continues to use Write(), which waits for completion.
func (aw *AsyncWriter) WriteSoon(path string, data []byte, perms os.FileMode) error {
	payload := append([]byte(nil), data...)

	aw.mu.Lock()
	if aw.closed {
		aw.mu.Unlock()
		// Once the worker is closed, preserve the caller's durability
		// expectation rather than silently dropping a late write.
		return os.WriteFile(path, payload, perms)
	}

	select {
	case aw.requests <- writeRequest{
		path: path, data: payload, perms: perms, flushNow: true,
	}:
		aw.mu.Unlock()
		return nil
	default:
		aw.mu.Unlock()
		// Saturation should be exceptionally rare (queue=100, attack-level
		// cadence). Fall back to a direct write so history is never dropped.
		return os.WriteFile(path, payload, perms)
	}
}

func (aw *AsyncWriter) Close() {
	aw.mu.Lock()
	if aw.closed {
		aw.mu.Unlock()
		return
	}
	aw.closed = true
	close(aw.requests)
	aw.mu.Unlock()
	aw.wg.Wait()
}

var globalAsyncWriter = NewAsyncWriter()

func AsyncWriteFile(path string, data []byte, perms os.FileMode) error {
	return globalAsyncWriter.Write(path, data, perms)
}

// AsyncWriteFileSoon queues a flush-immediate write and returns after enqueue.
// Use only where the caller already owns an in-memory authoritative snapshot.
func AsyncWriteFileSoon(path string, data []byte, perms os.FileMode) error {
	return globalAsyncWriter.WriteSoon(path, data, perms)
}

// CloseAsyncWriter shuts down the process-global persistence worker.
// Bot Stop/Start cycles intentionally do NOT call this; otherwise every later
// session falls back to synchronous os.WriteFile and loses the async path.
func CloseAsyncWriter() {
	globalAsyncWriter.Close()
}
