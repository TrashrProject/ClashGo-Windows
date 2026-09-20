package adb

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrNotConnected    = errors.New("not connected to ADB server")
	ErrWriteTimeout    = errors.New("write timeout")
	ErrReadTimeout     = errors.New("read timeout")
	ErrServerFailure   = errors.New("ADB server failure")
	ErrTransportGone   = errors.New("transport lost")
	ErrInvalidResponse = errors.New("invalid ADB response")
)

const (
	DefaultHost    = "127.0.0.1"
	DefaultPort    = 5037
	DefaultTimeout = 30 * time.Second
	DialTimeout    = 5 * time.Second
)

type Logger interface {
	Debug() bool
	Debugf(format string, v ...any)
	Info(msg string)
	Warn(msg string)
	Error(msg string)
	WithFields(fields map[string]any) Logger
}

type nopLogger struct{}

func (nopLogger) Debug() bool                        { return false }
func (nopLogger) Debugf(string, ...any)              {}
func (nopLogger) Info(string)                        {}
func (nopLogger) Warn(string)                        {}
func (nopLogger) Error(string)                       {}
func (n nopLogger) WithFields(map[string]any) Logger { return n }

type Option func(*Client)

func WithHost(host string) Option {
	return func(c *Client) { c.host = host }
}

func WithPort(port int) Option {
	return func(c *Client) { c.port = port }
}

func WithLogger(l Logger) Option {
	return func(c *Client) { c.log = l }
}

func WithBlueStacksInstance(instance string) Option {
	return func(c *Client) { c.blueStacksInstance = instance }
}

func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.timeout = d
	}
}

func WithZoomKeys(out, in string) Option {
	return func(c *Client) {
		c.zoomOutKey = out
		c.zoomInKey = in
	}
}

func WithJitterTaps(v bool) Option {
	return func(c *Client) { c.jitterTaps = v }
}

func WithJitterDelays(v bool) Option {
	return func(c *Client) { c.jitterDelays = v }
}

func WithMaxJitterPixels(v float64) Option {
	return func(c *Client) { c.maxJitterPixels = v }
}

func WithJitterFraction(v float64) Option {
	return func(c *Client) { c.jitterFraction = v }
}

type Health struct {
	LastCapture      time.Time `json:"last_capture"`
	AvgCaptureMs     float64   `json:"avg_capture_ms"`
	ConsecutiveFails int       `json:"consecutive_fails"`
	CapturesTotal    uint64    `json:"captures_total"`
	ErrorsTotal      uint64    `json:"errors_total"`
	LastError        string    `json:"last_error"`
}

// Health is the immutable/copyable snapshot exposed to BotStats / JSON.
// Synchronization lives in healthTracker so copying a Health value never
// copies a sync.Mutex (which would trigger go vet copylocks and be unsafe).
func (h Health) IsHealthy() bool {
	return h.ConsecutiveFails < 3
}

type healthTracker struct {
	mu    sync.Mutex
	state Health
}

func (h *healthTracker) RecordSuccess(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.state.LastCapture = time.Now()
	h.state.CapturesTotal++
	ms := d.Seconds() * 1000
	if h.state.AvgCaptureMs == 0 {
		h.state.AvgCaptureMs = ms
	} else {
		h.state.AvgCaptureMs = h.state.AvgCaptureMs*0.9 + ms*0.1
	}
	h.state.ConsecutiveFails = 0
	h.state.LastError = ""
}

func (h *healthTracker) RecordFailure(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.state.ConsecutiveFails++
	h.state.ErrorsTotal++
	if err != nil {
		h.state.LastError = err.Error()
	}
}

func (h *healthTracker) IsHealthy() bool {
	return h.Snapshot().IsHealthy()
}

func (h *healthTracker) Snapshot() Health {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state
}
