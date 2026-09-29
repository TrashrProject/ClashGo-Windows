package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"github.com/Ducky705/ClashGO/internal/attack"
	"github.com/Ducky705/ClashGO/internal/bot"
	"github.com/Ducky705/ClashGO/internal/config"
	"github.com/Ducky705/ClashGO/internal/logger"
	"github.com/Ducky705/ClashGO/internal/licensing"
	"github.com/Ducky705/ClashGO/internal/paths"
	"github.com/Ducky705/ClashGO/internal/support"
	"github.com/Ducky705/ClashGO/internal/telemetry"
	"github.com/Ducky705/ClashGO/internal/updater"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx       context.Context
	bot       *bot.Bot
	botCtx    context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	stopping  bool
	lastStats bot.BotStats

	// Logs are high-frequency and unrelated to bot lifecycle ownership.
	// Keep them off the main App mutex so console traffic cannot delay
	// Start/Stop/GetStats or bot assignment.
	logMu     sync.RWMutex
	logBuffer []string

	// cachedHistory is the in-memory mirror of attack history so React never
	// needs filesystem I/O on its normal poll/event path. It is loaded lazily
	// from disk on cold start and, while the bot runs, replaced directly from
	// Bot.HistorySnapshot() at attack boundaries. Matchmaking skips intentionally never
	// refresh history: their counters are atomic and React already polls live
	// stats, so disk I/O stays off the search hot path. RWMutex because read
	// dominates on the hot IPC path.
	// Every eager refresh is a FORCED disk re-read (see
	// refreshHistory) — a warm cache must never be treated as
	// authoritative, or the latest attack would never surface.
	cachedHistory   []bot.AttackReport
	cachedHistoryMu sync.RWMutex

	// License + automatic support reporting.
	license         *licensing.Service
	supportReporter *support.Reporter

	// Updater wiring
	updater       *updater.Service
	updaterBgCtx  context.Context
	updaterBgStop context.CancelFunc
}

type WailsLogWriter struct {
	app *App
}

// Write bridges zerolog to a bounded in-memory ring buffer read by
// App.GetLogs() on the React poll cadence. A prior revision also
// emitted `"bot_log"` Events here for a future streaming-log
// viewer, but the bridge emit had no React subscriber (verified:
// no EventsOn("bot_log", ...) anywhere in web/src/components) and
// cost a WailsIPC round-trip per zerolog line. Removed.
func (w *WailsLogWriter) Write(p []byte) (n int, err error) {
	msg := string(p)

	w.app.logMu.Lock()
	w.app.logBuffer = append(w.app.logBuffer, msg)
	if len(w.app.logBuffer) > 100 {
		w.app.logBuffer = w.app.logBuffer[len(w.app.logBuffer)-100:]
	}
	w.app.logMu.Unlock()

	return len(p), nil
}

// NewApp creates a new App application struct
func NewApp() *App {
	cfg := config.LoadOrDefault("config.json")
	controlURL := clashControlServiceURL(cfg)
	licenseService := licensing.New(controlURL, version)
	return &App{
		logBuffer: make([]string, 0, 100),
		license:   licenseService,
		updater:   updater.New(updater.DefaultConfigWithChannel(version, updateChannel)),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Setup license state + automatic support reporter before the logger so
	// error/fatal/panic records can be queued from the first startup failure.
	if a.license == nil {
		cfg := config.LoadOrDefault("config.json")
		a.license = licensing.New(clashControlServiceURL(cfg), version)
	}
	if a.license.GetState().Activated {
		if err := a.applyMemberProfileForCurrentLicense(); err != nil {
			log.Warn().Err(err).Msg("failed to restore member settings profile")
		}
		if err := a.restoreTestSessionSettings(); err != nil {
			log.Warn().Err(err).Msg("failed to recover interrupted test-session settings")
		}
		if err := a.applyMemberAccountForCurrentLicense(); err != nil {
			log.Warn().Err(err).Msg("failed to restore member Clash account")
		}
		if err := a.applyMemberAutomationForCurrentLicense(); err != nil {
			log.Warn().Err(err).Msg("failed to restore member automation preferences")
		}
		if err := a.restoreMemberRuntimeState(true); err != nil {
			log.Warn().Err(err).Msg("failed to restore member runtime state")
		}
	}
	cfg := config.LoadOrDefault("config.json")
	a.supportReporter = support.New(clashControlServiceURL(cfg), version, a.license)

	// Setup log bridge.
	wailsWriter := &WailsLogWriter{app: a}
	logger.Init(os.Getenv("DEBUG") != "", wailsWriter, a.supportReporter)
	a.loadPersistedStats()

	// Validate a previously activated license without blocking first paint.
	go func() {
		state := a.license.Validate(context.Background())
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "license_state", state)
		}
		a.supportReporter.Flush(context.Background())
	}()

	// Bring up the updater service. If NewApp wasn't used (rare
	// test scaffold), construct lazily.
	if a.updater == nil {
		a.updater = updater.New(updater.DefaultConfigWithChannel(version, updateChannel))
	}
	a.updater.CleanupOrphanDownloads()
	bgCtx, bgCancel := context.WithCancel(context.Background())
	a.updaterBgCtx = bgCtx
	a.updaterBgStop = bgCancel
	a.updater.StartBackgroundPoller(bgCtx)
	go a.forwardUpdaterStatus(bgCtx)
	go a.licenseValidationLoop(bgCtx)

	// Skip the standalone web dashboard on `wails dev`. Wails injects
	// its own dev proxy at :34115 → Vite at :5173 by parsing stdout
	// for `http://host:port` patterns. If we start Echo (which prints
	// its own listen URL), `wails dev` mis-reads it as Vite re-pointing
	// to :8080 and re-aims the WkWebView there — where Echo serves the
	// (stale or empty) `web/dist` instead of Vite's HMR graph. The user
	// sees a half-mounted layout on the transparent webview, presenting
	// as the dark-zinc frame color through the transparent layers, i.e.
	// a "black screen after 1 second".
	//
	// IMPORTANT: detect dev mode via Wails' canonical runtime API rather
	// than an env-var check — the V2 CLI does NOT set WAILS_DEV (or any
	// equivalent) on the spawned GUI process, so `os.Getenv("WAILS_DEV")`
	// was always empty and would have started Echo in dev too.
	if runtime.Environment(ctx).BuildType != "dev" {
		// Start Web Server for Remote Access (production-only).
		go a.startWebServer()
	}
}

var memberRuntimeStateFiles = []string{
	"stats.json",
	"attack_history.json",
	"last_attack_report.json",
	"village_resources.json",
	"village_resource_history.json",
	"current_army.json",
	filepath.Join("output", "session_reports", "latest.json"),
}

func (a *App) memberRuntimeStateDir() string {
	if a == nil || a.license == nil {
		return ""
	}
	id := strings.TrimSpace(a.license.ProfileID())
	if id == "" {
		return ""
	}
	return paths.ResolveConfig(filepath.Join("members", id, "state"))
}

func copyRuntimeStateFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	tmp := dst + ".tmp"
	backup := dst + ".bak"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}

	_ = os.Remove(backup)
	hadOriginal := false
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, backup); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		hadOriginal = true
	}

	if err := os.Rename(tmp, dst); err != nil {
		if hadOriginal {
			_ = os.Rename(backup, dst)
		}
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func (a *App) archiveMemberRuntimeState(clearShared bool) error {
	dir := a.memberRuntimeStateDir()
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, rel := range memberRuntimeStateFiles {
		src := paths.ResolveConfig(rel)
		dst := filepath.Join(dir, rel)

		if _, err := os.Stat(src); os.IsNotExist(err) {
			// Absence is meaningful (for example after ResetStats): remove any
			// older archived copy so deleted member data cannot reappear later.
			_ = os.Remove(dst)
			_ = os.Remove(dst + ".bak")
			_ = os.Remove(dst + ".tmp")
			continue
		}

		if err := copyRuntimeStateFile(src, dst); err != nil {
			return fmt.Errorf("archive member state %s: %w", rel, err)
		}
		if clearShared {
			_ = os.Remove(src)
		}
	}
	marker := filepath.Join(dir, ".initialized")
	if err := os.WriteFile(marker, []byte("1"), 0o600); err != nil {
		return err
	}
	return nil
}

func (a *App) restoreMemberRuntimeState(adoptShared bool) error {
	dir := a.memberRuntimeStateDir()
	if dir == "" {
		return nil
	}
	marker := filepath.Join(dir, ".initialized")

	if adoptShared {
		// A persisted local activation means the shared runtime files belong to
		// this same member from the previous process. If any are present, they
		// are newer than (or equal to) the archived snapshot and therefore are
		// authoritative. Refresh the member archive without clearing them.
		hasShared := false
		for _, rel := range memberRuntimeStateFiles {
			if _, err := os.Stat(paths.ResolveConfig(rel)); err == nil {
				hasShared = true
				break
			}
		}
		if hasShared {
			return a.archiveMemberRuntimeState(false)
		}
	}

	if _, err := os.Stat(marker); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		// A newly activated license must start clean. A startup migration with
		// no shared data also starts clean and simply initializes its namespace.
		if !adoptShared {
			for _, rel := range memberRuntimeStateFiles {
				_ = os.Remove(paths.ResolveConfig(rel))
			}
		}
		return os.WriteFile(marker, []byte("1"), 0o600)
	}

	for _, rel := range memberRuntimeStateFiles {
		src := filepath.Join(dir, rel)
		dst := paths.ResolveConfig(rel)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			_ = os.Remove(dst)
			continue
		}
		if err := copyRuntimeStateFile(src, dst); err != nil {
			return fmt.Errorf("restore member state %s: %w", rel, err)
		}
	}
	return nil
}

func (a *App) clearInMemoryMemberRuntimeState() {
	a.mu.Lock()
	a.lastStats = bot.BotStats{}
	a.mu.Unlock()
	a.cachedHistoryMu.Lock()
	a.cachedHistory = nil
	a.cachedHistoryMu.Unlock()
}

func clearSharedMemberRuntimeStateFiles() {
	for _, rel := range memberRuntimeStateFiles {
		_ = os.Remove(paths.ResolveConfig(rel))
	}
}

func (a *App) waitForBotTeardown(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		a.mu.Lock()
		stopping := a.stopping
		a.mu.Unlock()
		if !stopping {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("bot teardown did not finish within %s", timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (a *App) loadPersistedStats() {
	data, err := os.ReadFile(paths.ResolveConfig("stats.json"))
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warn().Err(err).Msg("failed to read persisted stats")
		}
		return
	}

	var stats bot.BotStats
	if err := json.Unmarshal(data, &stats); err != nil {
		log.Warn().Err(err).Msg("failed to parse persisted stats; starting counters at zero")
		return
	}

	// Connection/CPU fields are live process metrics, not durable history.
	stats.AdbHealth.LastCapture = time.Time{}
	stats.AdbHealth.AvgCaptureMs = 0
	stats.AdbHealth.ConsecutiveFails = 0
	stats.AdbHealth.CapturesTotal = 0
	stats.AdbHealth.ErrorsTotal = 0
	stats.AdbHealth.LastError = ""
	stats.CPUTimeSec = 0
	stats.CPUCores = 0

	a.mu.Lock()
	a.lastStats = stats
	a.mu.Unlock()

	log.Info().
		Int32("attacks", stats.AttacksCompleted).
		Int64("gold", stats.TotalGold).
		Int64("elixir", stats.TotalElixir).
		Msg("restored persisted session totals")
}

func (a *App) shutdown(ctx context.Context) {
	if a.supportReporter != nil {
		a.supportReporter.Flush(context.Background())
		a.supportReporter.Close()
	}

	// Stop the updater's background poller so it can't fire an HTTP
	// check mid-teardown.
	if a.updaterBgStop != nil {
		a.updaterBgStop()
	}

	// Cancel a running bot (or an in-flight boot) synchronously so it
	// stops issuing taps/captures the instant the user closes the
	// window — the captureLoop and any attack sequence observe the
	// cancelled context on their next check (sub-millisecond). We do
	// NOT run the heavier bot.Stop() teardown (ADB client close, async-
	// writer drain, file flush) here: it is detached from the process
	// exit path on purpose, and blocking the close on it would reintro-
	// duce exactly the freeze this fix removes. The OS reclaims those
	// handles when the process exits.
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	if a.bot != nil {
		a.bot.Cancel()
	}
	a.mu.Unlock()

	// Persist final stats. saveStats is a synchronous barrier through the
	// process-global writer, so attack-history writes queued before it are on
	// disk before we refresh the member snapshot.
	a.saveStats()
	if a.license != nil && a.license.GetState().Activated {
		if err := a.archiveMemberRuntimeState(false); err != nil {
			log.Warn().Err(err).Msg("failed to persist member runtime state on shutdown")
		}
	}
	bot.CloseAsyncWriter()
}

func (a *App) licenseValidationLoop(ctx context.Context) {
	if a.license == nil || !a.GetLicensePolicy().Enforced {
		return
	}

	// The startup validation runs immediately in startup(). Subsequent checks
	// keep server-side revocations and role changes effective without requiring
	// an application restart.
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			state := a.license.Validate(ctx)
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "license_state", state)
			}
			if !state.Activated && a.IsRunning() {
				log.Warn().Str("reason", state.Error).Msg("license became invalid; stopping bot")
				_ = a.StopBot()
			}
		}
	}
}

// forwardUpdaterStatus pushes the updater's status to the React side
// on every meaningful change. We use a 2s ticker with equality check
// to avoid spamming the UI with identical payloads; React renders
// only when the status struct actually changes.
func (a *App) forwardUpdaterStatus(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	var lastJSON string
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if a.ctx == nil || a.updater == nil {
				continue
			}
			st := a.updater.GetStatus()
			b, _ := json.Marshal(st)
			s := string(b)
			if s == lastJSON {
				continue
			}
			lastJSON = s
			runtime.EventsEmit(a.ctx, "updater_status", st)
		}
	}
}

func (a *App) marshalStatsSnapshot() ([]byte, error) {
	a.mu.Lock()
	stats := a.lastStats
	if a.bot != nil {
		stats = mergeStats(a.lastStats, a.bot.Stats())
	}
	a.mu.Unlock()
	return json.MarshalIndent(stats, "", "  ")
}

func (a *App) saveStats() {
	bytes, err := a.marshalStatsSnapshot()
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal stats")
		return
	}
	if err := bot.AsyncWriteFile(paths.ResolveConfig("stats.json"), bytes, 0644); err != nil {
		log.Error().Err(err).Msg("failed to write stats.json")
	}
}

func (a *App) saveStatsSoon() {
	bytes, err := a.marshalStatsSnapshot()
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal stats")
		return
	}
	if err := bot.AsyncWriteFileSoon(paths.ResolveConfig("stats.json"), bytes, 0644); err != nil {
		log.Error().Err(err).Msg("failed to queue stats.json")
	}
}

// mergeStats accumulates the live bot's session counters into acc.
// Bot counters are zeroed on every NewBot, so persisted/returned totals
// are acc + current. AdbHealth and CPU metrics are live values and are
// always taken from current.
func mergeStats(acc, current bot.BotStats) bot.BotStats {
	res := bot.BotStats{
		AttacksCompleted:  acc.AttacksCompleted + current.AttacksCompleted,
		SearchSkips:       acc.SearchSkips + current.SearchSkips,
		TotalGold:         acc.TotalGold + current.TotalGold,
		TotalElixir:       acc.TotalElixir + current.TotalElixir,
		TotalDE:           acc.TotalDE + current.TotalDE,
		Stars0:            acc.Stars0 + current.Stars0,
		Stars1:            acc.Stars1 + current.Stars1,
		Stars2:            acc.Stars2 + current.Stars2,
		Stars3:            acc.Stars3 + current.Stars3,
		Uptime:            acc.Uptime + current.Uptime,
		AdbHealth:         current.AdbHealth,
		CPUTimeSec:        current.CPUTimeSec,
		CPUCores:          current.CPUCores,
		RecoveryAttempts:  acc.RecoveryAttempts + current.RecoveryAttempts,
		RecoverySuccesses: acc.RecoverySuccesses + current.RecoverySuccesses,
		BlueStacksRestarts: acc.BlueStacksRestarts + current.BlueStacksRestarts,

		// These are runtime-quality metrics, not additive counters. While the
		// bot is active the newest live value is authoritative; persisting them
		// still gives the stopped dashboard a useful last-known snapshot.
		AverageCaptureMS:        current.AverageCaptureMS,
		LastCaptureMS:           current.LastCaptureMS,
		TelemetryEvents:         current.TelemetryEvents,
		Anomalies:               current.Anomalies,
		TargetsSkipped:          current.TargetsSkipped,
		HealthScore:             current.HealthScore,
		SpeedProfile:            current.SpeedProfile,
		TargetsSeen:             current.TargetsSeen,
		TargetsAccepted:         current.TargetsAccepted,
		TargetAcceptanceRate:    current.TargetAcceptanceRate,
		AvgSkipsPerAttack:       current.AvgSkipsPerAttack,
		AverageTargetScanMS:     current.AverageTargetScanMS,
		LastTargetScanMS:        current.LastTargetScanMS,
		AverageReturnHomeMS:     current.AverageReturnHomeMS,
		LastReturnHomeMS:        current.LastReturnHomeMS,
		AverageNextTransitionMS: current.AverageNextTransitionMS,
		LastNextTransitionMS:    current.LastNextTransitionMS,
		NextTransitions:         current.NextTransitions,
		NextRetries:             current.NextRetries,
		NextFirstPassRate:       current.NextFirstPassRate,
		AvgNextVerifyProbes:     current.AvgNextVerifyProbes,
		AvgAcceptedGE:           current.AvgAcceptedGE,
		AvgRejectedGE:           current.AvgRejectedGE,
		AvgAcceptedDE:           current.AvgAcceptedDE,
		AvgRejectedDE:           current.AvgRejectedDE,
		AvgAcceptedScore:        current.AvgAcceptedScore,
		AvgRejectedScore:        current.AvgRejectedScore,
		PreferredScaleAttempts:  current.PreferredScaleAttempts,
		PreferredScaleHits:      current.PreferredScaleHits,
		PreferredScaleFallbacks: current.PreferredScaleFallbacks,
		PreferredScaleHitRate:   current.PreferredScaleHitRate,
		PreferredScaleEnabled:   current.PreferredScaleEnabled,
		UIAnchorAttempts:        current.UIAnchorAttempts,
		UIAnchorHits:            current.UIAnchorHits,
		UIAnchorFallbacks:       current.UIAnchorFallbacks,
		UIAnchorHitRate:         current.UIAnchorHitRate,
		UIAnchorEnabled:         current.UIAnchorEnabled,
		NearMissTargets:         current.NearMissTargets,
		NearMiss5Targets:        current.NearMiss5Targets,
		NearMiss10Targets:       current.NearMiss10Targets,
		NearMiss15Targets:       current.NearMiss15Targets,
		TopRejectedTargets:      current.TopRejectedTargets,
	}

	// Recovery rate is meaningful over the persisted + live totals.
	if res.RecoveryAttempts > 0 {
		res.RecoverySuccessRate = float64(res.RecoverySuccesses) * 100 / float64(res.RecoveryAttempts)
	}

	// Lifetime rates use the same accumulated counters and accumulated uptime,
	// so restarting the app does not make Gold/h jump merely because the new
	// process has only been alive for a few minutes.
	hours := res.Uptime.Hours()
	if hours > 0 {
		res.GoldPerHour = float64(res.TotalGold) / hours
		res.ElixirPerHour = float64(res.TotalElixir) / hours
		res.DEPerHour = float64(res.TotalDE) / hours
	}
	if res.AttacksCompleted > 0 {
		totalStars := int64(res.Stars1) + 2*int64(res.Stars2) + 3*int64(res.Stars3)
		res.AverageStars = float64(totalStars) / float64(res.AttacksCompleted)
		res.ThreeStarRate = float64(res.Stars3) * 100 / float64(res.AttacksCompleted)
	}

	return res
}

func (a *App) ResetStats() error {
	a.mu.Lock()
	if a.bot != nil || a.cancel != nil || a.stopping {
		a.mu.Unlock()
		return fmt.Errorf("wait for the bot to finish stopping before resetting statistics")
	}
	a.lastStats = bot.BotStats{}
	a.mu.Unlock()

	a.cachedHistoryMu.Lock()
	a.cachedHistory = nil
	a.cachedHistoryMu.Unlock()

	// Remove every persisted runtime artifact that belongs to the active
	// member, including the latest session report. Using the same canonical
	// list as archive/restore prevents new state files from being forgotten
	// when ResetStats evolves.
	for _, rel := range memberRuntimeStateFiles {
		path := paths.ResolveConfig(rel)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	// Purge the per-license snapshot immediately as well. Otherwise a crash
	// between ResetStats and the next archive could restore stale statistics
	// on the following launch.
	if stateDir := a.memberRuntimeStateDir(); stateDir != "" {
		for _, rel := range memberRuntimeStateFiles {
			path := filepath.Join(stateDir, rel)
			_ = os.Remove(path)
			_ = os.Remove(path + ".bak")
			_ = os.Remove(path + ".tmp")
		}
	}

	if traces, globErr := filepath.Glob(paths.ResolveConfig("output/attack_traces/*.json")); globErr == nil {
		for _, trace := range traces {
			_ = os.Remove(trace)
		}
	}
	return nil
}

func (a *App) startWebServer() {
	e := echo.New()
	e.HideBanner = true
	// Defense in depth: even if a future startup path accidentally enables
	// startWebServer under wails dev (e.g. removing the WAILS_DEV gate),
	// suppressing the listen-port banner removes the `http://host:port`
	// pattern that `wails dev` parses for proxy-target discovery.
	e.HidePort = true

	// Basic API for remote control
	e.GET("/status", func(c echo.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		running := a.bot != nil
		return c.JSON(200, map[string]interface{}{"running": running})
	})

	e.GET("/stats", func(c echo.Context) error {
		return c.JSON(200, a.GetStats())
	})

	e.GET("/history", func(c echo.Context) error {
		return c.JSON(200, a.GetAttackHistory())
	})

	// Static assets from embed would be ideal, but for now just API
	// Or we can serve the built dist folder if it exists
	e.Static("/", "web/dist")

	// NOTE: do NOT print "http://127.0.0.1:8080" here — `wails dev` watches
	// stdout for `http://host:port` patterns to discover the Vite dev
	// server, and it would mis-read this line as Vite's URL flipping to
	// :8080. Once that happens the WkWebView gets re-pointed to the Echo
	// server, which serves an empty / stale `web/dist/` in dev (Vite
	// never writes to disk in dev), leaving the window painted as the
	// WkWebView's transparent background — i.e. the `bg-zinc-950` frame
	// color, which presents as a solid black screen. Keep this as a plain
	// "port NNN" string so the regex in `wails dev` ignores it.
	log.Info().Msg("Web Dashboard available on port 8080")
	if err := e.Start("127.0.0.1:8080"); err != nil {
		log.Error().Err(err).Msg("failed to start web server")
	}
}

type BotStatus struct {
	Running bool   `json:"running"`
	Message string `json:"message"`
}

type StartupCheckItem struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	OK          bool   `json:"ok"`
	Message     string `json:"message"`
	Action      string `json:"action,omitempty"`
	ActionLabel string `json:"action_label,omitempty"`
}

type StartupReadiness struct {
	Ready  bool               `json:"ready"`
	Checks []StartupCheckItem `json:"checks"`
}

func memberRuntimeConfigReady(cfg *config.BotConfig) (bool, string) {
	if cfg == nil {
		return false, "Configuration membre indisponible"
	}
	speed := strings.ToLower(strings.TrimSpace(cfg.Automation.SpeedProfile))
	if speed != "cautious" && speed != "normal" && speed != "fast" {
		return false, "Profil de vitesse invalide"
	}
	if cfg.Automation.MaxAttacksPerHour < 1 || cfg.Automation.MaxAttacksPerHour > 24 {
		return false, "La limite d’attaques par heure doit être comprise entre 1 et 24"
	}
	if cfg.Attack.MaxAttackPerSession < 1 || cfg.Attack.MaxAttackPerSession > 500 {
		return false, "La limite d’attaques par session doit être comprise entre 1 et 500"
	}
	if cfg.Automation.BreakEveryAttacks < 0 || cfg.Automation.BreakEveryAttacks > 20 {
		return false, "La fréquence des pauses doit être comprise entre 0 et 20 attaques"
	}
	breakMinutes := int(cfg.Automation.BreakDuration.Duration / time.Minute)
	if breakMinutes < 0 || breakMinutes > 30 {
		return false, "La durée des pauses doit être comprise entre 0 et 30 minutes"
	}
	return true, fmt.Sprintf(
		"%s · %d attaques/h · %d/session",
		map[string]string{"cautious": "Prudente", "normal": "Normale", "fast": "Rapide"}[speed],
		cfg.Automation.MaxAttacksPerHour,
		cfg.Attack.MaxAttackPerSession,
	)
}

func (a *App) GetStartupReadiness() StartupReadiness {
	checks := make([]StartupCheckItem, 0, 7)
	add := func(id, label string, ok bool, message, action, actionLabel string) {
		if ok {
			action = ""
			actionLabel = ""
		}
		checks = append(checks, StartupCheckItem{
			ID: id, Label: label, OK: ok, Message: message,
			Action: action, ActionLabel: actionLabel,
		})
	}

	licenseOK := true
	licenseMessage := "Mode beta local"
	if a.GetLicensePolicy().Enforced {
		licenseOK = false
		licenseMessage = "Licence absente, expirée ou invalide"
		if a.license != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			state := a.license.Validate(ctx)
			cancel()
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "license_state", state)
			}
			licenseOK = state.Activated
			if licenseOK {
				if strings.Contains(strings.ToLower(state.Error), "offline") {
					licenseMessage = "Licence valide · mode hors ligne temporaire"
				} else {
					licenseMessage = "Licence valide"
				}
			} else if msg := strings.TrimSpace(state.Error); msg != "" {
				licenseMessage = msg
			}
		}
	}
	add("license", "Licence", licenseOK, licenseMessage, "account", "Ouvrir Mon ClashGO")

	cfg := config.LoadOrDefault("config.json")
	accountOK := strings.TrimSpace(cfg.Account.PlayerTag) != ""
	accountMessage := "Compte Clash lié"
	if !accountOK {
		accountMessage = "Aucun tag joueur lié"
	}
	add("account", "Compte Clash", accountOK, accountMessage, "account", "Lier le compte")

	diag := collectSystemDiagnostics()
	add("assets", "Fichiers ClashGO", diag.AssetsReady, func() string {
		if diag.AssetsReady {
			return "Tous les fichiers nécessaires sont présents"
		}
		if len(diag.MissingAssets) > 0 {
			return "Manquants : " + strings.Join(diag.MissingAssets, ", ")
		}
		return "Certains fichiers nécessaires sont manquants"
	}(), "settings", "Voir le diagnostic")

	if goruntime.GOOS == "windows" {
		add("bluestacks", "BlueStacks 5", diag.Emulator.BlueStacksPlayerFound, func() string {
			if diag.Emulator.BlueStacksPlayerFound {
				if diag.Emulator.BlueStacksRunning {
					return "BlueStacks est installé et démarré"
				}
				return "BlueStacks est installé"
			}
			return "BlueStacks 5 n’est pas détecté"
		}(), "settings", "Configurer Windows")
		add("adb", "ADB", diag.Emulator.ADBFound, func() string {
			if diag.Emulator.ADBFound {
				return "ADB est disponible"
			}
			return "ADB n’est pas détecté"
		}(), "settings", "Configurer ADB")
		instanceOK := strings.TrimSpace(diag.Emulator.PreferredInstance) != ""
		add("instance", "Instance", instanceOK, func() string {
			if instanceOK {
				return "Instance : " + diag.Emulator.PreferredInstance
			}
			return "Aucune instance BlueStacks utilisable"
		}(), "settings", "Choisir l’instance")
	}

	strategyPath := strings.TrimSpace(cfg.Attack.StrategyFile)
	strategyOK := false
	strategyMessage := "Aucune stratégie d’attaque configurée"
	if strategyPath != "" {
		if !filepath.IsAbs(strategyPath) {
			strategyPath = paths.Resolve(filepath.Join("strategies", filepath.Base(strategyPath)))
		}
		if info, err := os.Stat(strategyPath); err == nil && !info.IsDir() {
			strategyOK = true
			strategyMessage = "Stratégie disponible : " + filepath.Base(strategyPath)
		} else {
			strategyMessage = "Fichier de stratégie introuvable : " + filepath.Base(strategyPath)
		}
	}
	add("strategy", "Stratégie", strategyOK, strategyMessage, "automation", "Ouvrir Automatisation")

	pacingOK, pacingMessage := memberRuntimeConfigReady(cfg)
	add("member_pacing", "Cadence membre", pacingOK, pacingMessage, "member_settings", "Ouvrir Réglages bot")

	ready := true
	for _, check := range checks {
		if !check.OK {
			ready = false
			break
		}
	}
	return StartupReadiness{Ready: ready, Checks: checks}
}

type AttackReplayPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type AttackReplayEventView struct {
	OffsetMS   int64             `json:"offset_ms"`
	Kind       string            `json:"kind"`
	Name       string            `json:"name,omitempty"`
	Category   string            `json:"category,omitempty"`
	Count      int               `json:"count,omitempty"`
	SlotX      int               `json:"slot_x,omitempty"`
	SlotY      int               `json:"slot_y,omitempty"`
	DeploySide string            `json:"deploy_side,omitempty"`
	P1         AttackReplayPoint `json:"p1"`
	P2         AttackReplayPoint `json:"p2"`
}

type AttackReplayView struct {
	Available bool                    `json:"available"`
	Timestamp string                  `json:"timestamp,omitempty"`
	Strategy  string                  `json:"strategy,omitempty"`
	Complete  bool                    `json:"complete"`
	Events    []AttackReplayEventView `json:"events"`
}

func (a *App) GetLatestAttackReplay() AttackReplayView {
	dir := paths.ResolveConfig("output/attack_traces")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return AttackReplayView{Events: []AttackReplayEventView{}}
	}

	var latest string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		if entry.Name() > latest {
			latest = entry.Name()
		}
	}
	if latest == "" {
		return AttackReplayView{Events: []AttackReplayEventView{}}
	}

	data, err := os.ReadFile(filepath.Join(dir, latest))
	if err != nil {
		return AttackReplayView{Events: []AttackReplayEventView{}}
	}
	var trace attack.AttackTrace
	if err := json.Unmarshal(data, &trace); err != nil {
		return AttackReplayView{Events: []AttackReplayEventView{}}
	}

	view := AttackReplayView{
		Available: true,
		Timestamp: trace.Timestamp.Format(time.RFC3339Nano),
		Strategy: trace.Strategy,
		Complete: trace.DeployComplete,
		Events: make([]AttackReplayEventView, 0, len(trace.Events)),
	}
	for _, ev := range trace.Events {
		view.Events = append(view.Events, AttackReplayEventView{
			OffsetMS: ev.OffsetMS,
			Kind: ev.Kind,
			Name: ev.Name,
			Category: ev.Category,
			Count: ev.Count,
			SlotX: ev.SlotX,
			SlotY: ev.SlotY,
			DeploySide: ev.DeploySide,
			P1: AttackReplayPoint{X: ev.P1.X, Y: ev.P1.Y},
			P2: AttackReplayPoint{X: ev.P2.X, Y: ev.P2.Y},
		})
	}
	return view
}


// StartBot starts the bot with the given thresholds
//
// The returned BotStatus reports running=true immediately: the boot
// runs in a background goroutine (BlueStacks launch + ADB connect +
// boot probe can take 1-3 minutes on a cold start). If the boot
// fails, the `bot_error` / `bot_init_failed` events flip the UI back
// to stopped; if the user clicks Stop mid-boot, the captured
// per-call context is cancelled and the boot goroutine aborts
// instead of finishing the boot and starting anyway (the old
// behavior — see the concurrency notes in StopBot).
func (a *App) StartBot(gold, elixir, dark int, upgradeWalls bool, searchEnabled bool) BotStatus {
	if a.GetLicensePolicy().Enforced {
		if a.license == nil {
			return BotStatus{Running: false, Message: "Le service de licence ClashGO est indisponible"}
		}

		// Revalidate at the exact moment the user starts the bot. The background
		// validation loop runs every 15 minutes, but Start must never rely on a
		// stale in-memory entitlement after a revoke, expiry or machine reset.
		// A short timeout keeps Start responsive; Validate still preserves the
		// configured offline grace when the control service is temporarily down.
		licenseCtx, cancelLicense := context.WithTimeout(context.Background(), 4*time.Second)
		state := a.license.Validate(licenseCtx)
		cancelLicense()
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "license_state", state)
		}
		if !state.Activated {
			msg := "Une licence ClashGO valide est nécessaire"
			if strings.TrimSpace(state.Error) != "" {
				msg += ": " + state.Error
			}
			return BotStatus{Running: false, Message: msg}
		}
	}

	cfgForPacing := config.LoadOrDefault("config.json")
	if pacingOK, pacingMessage := memberRuntimeConfigReady(cfgForPacing); !pacingOK {
		return BotStatus{Running: false, Message: "Réglage membre invalide : " + pacingMessage}
	}

	diag := collectSystemDiagnostics()
	if !diag.AssetsReady {
		return BotStatus{
			Running: false,
			Message: "Fichiers nécessaires manquants : " + strings.Join(diag.MissingAssets, ", "),
		}
	}

	if goruntime.GOOS == "windows" {
		if !diag.Emulator.BlueStacksPlayerFound {
			return BotStatus{Running: false, Message: "BlueStacks 5 n’a pas été détecté. Installe ou démarre BlueStacks 5 puis réessaie."}
		}
		if !diag.Emulator.ADBFound {
			return BotStatus{Running: false, Message: "ADB n’a pas été détecté. Redémarre BlueStacks ou vérifie son installation."}
		}
		if strings.TrimSpace(diag.Emulator.PreferredInstance) == "" {
			return BotStatus{Running: false, Message: "Aucune instance BlueStacks n’a été détectée. Démarre ton instance depuis le gestionnaire multi-instance puis réessaie."}
		}
	}

	a.mu.Lock()

	if a.stopping {
		a.mu.Unlock()
		return BotStatus{Running: false, Message: "La session précédente est encore en cours de fermeture — réessaie dans un instant"}
	}
	if a.bot != nil {
		a.mu.Unlock()
		return BotStatus{Running: true, Message: "Le bot est déjà en cours"}
	}
	if a.cancel != nil {
		a.mu.Unlock()
		return BotStatus{Running: true, Message: "Le bot est encore en cours de démarrage — attends la connexion ou arrête-le d’abord"}
	}

	cfg := config.LoadOrDefault("config.json")
	applySimpleAutomationDefaults(cfg)
	cfg.Search.MinLootGold = gold
	cfg.Search.MinLootElixir = elixir
	cfg.Search.MinLootDarkElixir = dark
	cfg.Upgrade.UpgradeWalls = upgradeWalls
	cfg.Search.Enabled = searchEnabled

	// Create a placeholder to indicate the bot is starting. The
	// context is captured by value into the goroutine so a quick
	// Stop → Start cycle can't have the OLD goroutine observe the NEW
	// context (and wrongly claim success).
	a.botCtx, a.cancel = context.WithCancel(context.Background())
	bootCtx := a.botCtx
	a.mu.Unlock()

	go func(bootCtx context.Context) {
		b, err := bot.NewBotWithContext(bootCtx, cfg)
		if err != nil {
			if bootCtx.Err() != nil {
				// The user clicked Stop while BlueStacks was booting.
				// NewBotWithContext already closed its client; just
				// reset the start state. No error events — the stop
				// was intentional.
				log.Info().Msg("bot boot cancelled during startup")
				runtime.EventsEmit(a.ctx, "bot_boot_cancelled", map[string]interface{}{
					"message": "Le démarrage du bot a été annulé.",
				})
				a.mu.Lock()
				a.clearStartStateLocked()
				a.mu.Unlock()
				if err := a.restoreTestSessionSettings(); err != nil {
					log.Error().Err(err).Msg("failed to restore member settings after cancelled test startup")
				}
				return
			}

			// The orchestrator wraps the error with a Summary(); the
			// underlying cause is still reachable via errors.Unwrap
			// for programmatic consumers. The console logger now
			// surfaces `error="..."` so the user no longer has to
			// grep app.log to see what failed.
			log.Error().Err(err).Msg("failed to initialize bot")
			runtime.EventsEmit(a.ctx, "bot_error", fmt.Sprintf("Initialization Error: %v", err))
			runtime.EventsEmit(a.ctx, "bot_init_failed", map[string]interface{}{
				"message": err.Error(),
			})

			a.mu.Lock()
			a.clearStartStateLocked()
			a.mu.Unlock()
			if err := a.restoreTestSessionSettings(); err != nil {
				log.Error().Err(err).Msg("failed to restore member settings after test startup failure")
			}
			return
		}

		// Note: b.OnFrame used to be wired to a `"live_feed"`
		// EventsEmit that pushed a 50–150 KB base64 JPEG over the
		// WailsIPC bridge at capture-loop frequency (up to 10 FPS).
		// The React UI never subscribed to "live_feed" (verified:
		// web/src/components/* has no EventsOn matching that name)
		// so each emit was a wasted IPC round-trip. The live
		// screenshot now flows exclusively through GetLiveScreenshot()
		// — it returns the same b.lastFrame string from atomic.Value
		// without burning the bridge. See App.GetLiveScreenshot.

		b.OnStatsUpdate = func() {
			// The bot's in-memory history is authoritative while a session is
			// running. Mirror it directly into the App cache instead of forcing
			// attack_history.json to be written and re-read before React can see
			// the new row.
			history := b.HistorySnapshot()
			a.cachedHistoryMu.Lock()
			a.cachedHistory = make([]bot.AttackReport, len(history))
			copy(a.cachedHistory, history)
			a.cachedHistoryMu.Unlock()

			a.saveStatsSoon()

			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "attack_history_updated", history)
				runtime.EventsEmit(a.ctx, "stats_updated", a.GetStats())
			}
		}

		a.mu.Lock()
		if bootCtx.Err() != nil {
			// Stop was clicked between NewBotWithContext returning and
			// this assignment (e.g. while the bot was still settling
			// the game). Discard the freshly-booted bot instead of
			// starting it behind the user's back.
			a.clearStartStateLocked()
			a.mu.Unlock()
			log.Info().Msg("bot boot finished after startup cancellation; discarding and shutting down")
			runtime.EventsEmit(a.ctx, "bot_boot_cancelled", map[string]interface{}{
				"message": "Le démarrage du bot a été annulé.",
			})
			if err := a.restoreTestSessionSettings(); err != nil {
				log.Error().Err(err).Msg("failed to restore member settings after discarded test startup")
			}
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Error().Interface("panic", r).Msg("recovered panic during discarded-bot teardown")
					}
				}()
				b.Stop()
			}()
			return
		}
		a.bot = b
		a.mu.Unlock()

		// Use the LOCAL b, never a.bot: StopBot nulls a.bot (under
		// lock) the moment a Stop lands, and reading a.bot without the
		// lock here would panic with a nil deref exactly in the race
		// this fix is supposed to make reliable.
		if err := b.Start(); err != nil {
			if bootCtx.Err() != nil {
				// Stop landed between the assignment and Start() — with
				// the fail-fast client, b.Start() fails on the closed
				// transport. Intentional stop: no error event, just
				// tear down the booted bot so the next Start is clean.
				log.Info().Msg("bot start aborted by startup cancellation; discarding")
				runtime.EventsEmit(a.ctx, "bot_boot_cancelled", map[string]interface{}{
					"message": "Le démarrage du bot a été annulé.",
				})
				a.mu.Lock()
				a.clearStartStateLocked()
				a.mu.Unlock()
				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Error().Interface("panic", r).Msg("recovered panic during discarded-bot teardown")
						}
					}()
					b.Stop()
				}()
				return
			}

			log.Error().Err(err).Msg("failed to start bot")
			runtime.EventsEmit(a.ctx, "bot_error", fmt.Sprintf("Start Error: %v", err))

			// Clear the WHOLE start placeholder (bot AND cancel/botCtx)
			// — leaving a.cancel set would make every future StartBot
			// return "still starting up" forever.
			a.mu.Lock()
			a.clearStartStateLocked()
			a.mu.Unlock()
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Error().Interface("panic", r).Msg("recovered panic during failed-start teardown")
					}
				}()
				b.Stop()
			}()
			return
		}

		// Startup is now fully complete. The frontend keeps a separate
		// STARTING state and only switches to RUNNING after this event,
		// preventing the Start button from becoming an active Stop button
		// while BlueStacks/ADB are still booting.
		log.Info().Msg("bot startup complete; runtime active")
		runtime.EventsEmit(a.ctx, "bot_started", map[string]interface{}{
			"message": "Le bot est en cours.",
		})

		// Watch the bot-owned runtime context. This is essential for autonomous
		// stops such as the per-session attack cap: no UI action calls StopBot
		// in that path, so without this watcher a.bot would stay non-nil and the
		// frontend would keep showing "Bot en cours" after automation ended.
		go a.watchBotRuntime(b)
	}(bootCtx)

	return BotStatus{Running: true, Message: "Initialisation du bot démarrée"}
}

func (a *App) watchBotRuntime(b *bot.Bot) {
	if a == nil || b == nil {
		return
	}
	<-b.Done()

	a.mu.Lock()
	// Manual StopBot clears a.bot before cancelling/tearing down. In that
	// case it already owns cleanup, so this watcher must be a no-op.
	if a.bot != b {
		a.mu.Unlock()
		return
	}

	current := b.Stats()
	a.lastStats = mergeStats(a.lastStats, current)
	a.bot = nil
	a.cancel = nil
	a.botCtx = nil
	a.stopping = true
	a.mu.Unlock()

	log.Info().Msg("bot runtime ended autonomously; synchronizing application state")
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "bot_stopped", map[string]interface{}{
			"message": "La session ClashGO est terminée.",
			"automatic": true,
		})
	}

	// Runtime cancellation is already visible immediately. Do heavier native
	// teardown asynchronously so React never waits on OpenCV/ADB cleanup.
	go func() {
		defer func() {
			a.mu.Lock()
			a.stopping = false
			a.mu.Unlock()
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).Msg("recovered panic during autonomous bot teardown")
			}
		}()
		b.Stop()
		a.saveStats()
		if err := a.restoreTestSessionSettings(); err != nil {
			log.Error().Err(err).Msg("failed to restore member settings after test session")
		}
		a.cachedHistoryMu.Lock()
		a.cachedHistory = nil
		a.cachedHistoryMu.Unlock()
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "stats_updated", a.GetStats())
		}
	}()
}

// clearStartStateLocked resets the start placeholder after a failed
// or user-cancelled boot so a subsequent StartBot starts fresh.
// Caller MUST hold a.mu.
func (a *App) clearStartStateLocked() {
	a.bot = nil
	a.cancel = nil
	a.botCtx = nil
}

// StopBot stops the bot instantly.
//
// Behavior change vs. the previous implementation: the heavy teardown
// is detached to a goroutine so the IPC returns to React as fast as
// possible. Previously the IPC blocked on the entire graceful-shutdown
// chain — bot.Stop() (cancel + ADB client Close + globalAsyncWriter.Close
// which itself `wg.Wait()`s the in-flight stats write drain, plus
// template cache close + NDJSON file close) followed by saveStats()
// (synchronous JSON marshal + file write to disk). On an active attack
// that could run 1–3s before the IPC reply reached React, and the user
// perceived the Stop button as broken while the UI stayed on "Running".
//
// Now: we synchronously cancel the bot's internal context (so the
// captureLoop and any in-flight executeAttackSequence see the stop on
// their next `b.ctx.Done()` check — sub-millisecond) and detach the
// heavy teardown. The bot stops issuing new taps, captures, and state
// transitions immediately, which is what "stop right where we are"
// means in practice. React's `setIsRunning(false)` flips on the next
// React tick (within the 2s poll), so the UI feels instant.
//
// Concurrency: the heavy teardown runs in a detached goroutine that
// captures the local `bot` reference, so a subsequent StartBot can't
// observe a half-torn-down bot. The async-writer's global singleton
// still gets closed, which means a quick Stop → Start sequence could
// see AsyncWriteFile fall through to a synchronous os.WriteFile until
// the next NewAsyncWriter — acceptable, since the previous code path
// had the same constraint and the new behaviour is strictly an
// improvement on the slow path.
func (a *App) StopBot() BotStatus {
	a.mu.Lock()

	if a.stopping {
		a.mu.Unlock()
		return BotStatus{Running: false, Message: "Bot teardown already in progress"}
	}

	if a.cancel != nil {
		a.cancel()
	}

	if a.bot == nil {
		// The cancel above is the important part: it aborts a boot in
		// progress (the boot goroutine observes the cancelled context
		// and discards the bot instead of starting it). Report the
		// state honestly so the UI doesn't think a running bot exists.
		startupInFlight := a.cancel != nil
		a.mu.Unlock()
		if startupInFlight {
			return BotStatus{Running: false, Message: "Bot stop requested (startup cancelled)"}
		}
		return BotStatus{Running: false, Message: "Bot not running"}
	}

	// Capture and accumulate final stats before stopping. All counters
	// are atomic.Int* loads, so this is O(1) and non-blocking.
	current := a.bot.Stats()
	a.lastStats = mergeStats(a.lastStats, current)

	// Snapshot the bot pointer + synchronously cancel its context so
	// the captureLoop and any in-flight executeAttackSequence see the
	// stop on their next `b.ctx.Done()` check. This is the
	// user-visible "stop right where we are" — no more taps, captures,
	// or state transitions issued from this point on.
	bot := a.bot
	bot.Cancel()

	// Detach references under the lock so:
	//   1. `IsRunning()` returns false the moment the lock is released
	//      (drives the React `setIsRunning(false)` flip in <1 React
	//      tick).
	//   2. A concurrent StartBot sees `a.bot == nil` and proceeds to
	//      construct a new bot without observing a half-torn-down one.
	a.bot = nil
	a.cancel = nil
	a.stopping = true
	a.mu.Unlock()

	// Detach the slow teardown. The captureLoop will exit on its own
	// now that b.ctx is cancelled; this goroutine just releases OS
	// handles (ADB pipe, async-writer drain, template cache, NDJSON
	// file) and flushes the final stats snapshot to disk. None of that
	// is required for correctness of the user-visible stop signal.
	go func() {
		defer func() {
			a.mu.Lock()
			a.stopping = false
			a.mu.Unlock()
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).Msg("recovered panic during async bot stop")
			}
		}()
		bot.Stop()
		a.saveStats()
		if err := a.restoreTestSessionSettings(); err != nil {
			log.Error().Err(err).Msg("failed to restore member settings after stopped test session")
		}
		// Re-seed attack-history cache after teardown. If the user
		// manually edited attack_history.json while the bot was
		// stopped, the next React poll re-reads from disk instead
		// of serving the stale pre-edit snapshot.
		a.cachedHistoryMu.Lock()
		a.cachedHistory = nil
		a.cachedHistoryMu.Unlock()
	}()

	return BotStatus{Running: false, Message: "Bot arrêté"}
}

// IsRunning returns if the bot is currently running
func (a *App) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.bot != nil
}

type LicensePolicy struct {
	Enforced          bool   `json:"enforced"`
	ServiceConfigured bool   `json:"service_configured"`
	ServiceURL        string `json:"service_url,omitempty"`
}

func (a *App) ReportUIError(message string, componentStack string) {
	message = strings.TrimSpace(message)
	componentStack = strings.TrimSpace(componentStack)
	if message == "" {
		message = "unknown frontend error"
	}
	if len(message) > 4000 {
		message = message[:4000]
	}
	if len(componentStack) > 12000 {
		componentStack = componentStack[:12000]
	}

	log.Error().
		Str("surface", "frontend").
		Str("ui_error", message).
		Str("component_stack", componentStack).
		Msg("ClashGO UI crash")
}

func (a *App) GetLatestBootReport() *bot.BootReportView {
	data, err := os.ReadFile(paths.ResolveConfig("logs/last_boot_report.json"))
	if err != nil {
		return nil
	}
	var report bot.BootReportView
	if json.Unmarshal(data, &report) != nil || report.StartedAt.IsZero() {
		return nil
	}
	return &report
}

func (a *App) GetLicensePolicy() LicensePolicy {
	cfg := config.LoadOrDefault("config.json")
	serviceURL := clashControlServiceURL(cfg)

	// Licensing becomes mandatory only when a real control endpoint has been
	// explicitly configured for the build/runtime. The localhost fallback is
	// intentionally development-only and must never lock beta testers out.
	explicit := strings.TrimSpace(os.Getenv("CLASHGO_CONTROL_API_URL")) != "" ||
		strings.TrimSpace(controlServiceURL) != ""

	return LicensePolicy{
		Enforced:          explicit,
		ServiceConfigured: explicit && strings.TrimSpace(serviceURL) != "",
		ServiceURL:        func() string {
			if explicit {
				return serviceURL
			}
			return ""
		}(),
	}
}

// GetLicenseState exposes safe activation metadata to the UI. The full
// license key is intentionally never returned through Wails.
func (a *App) GetLicenseState() licensing.State {
	if a.license == nil {
		return licensing.State{}
	}
	return a.license.GetState()
}

func (a *App) RefreshLicense() licensing.State {
	if a.license == nil {
		return licensing.State{Activated: false, Error: "license service is unavailable"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	state := a.license.Validate(ctx)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "license_state", state)
	}
	return state
}

// ActivateLicense validates and binds a license to this Windows machine.
func (a *App) ActivateLicense(key string) (licensing.State, error) {
	if a.license == nil {
		cfg := config.LoadOrDefault("config.json")
		a.license = licensing.New(clashControlServiceURL(cfg), version)
	}
	state, err := a.license.Activate(context.Background(), key)
	if err != nil {
		return state, err
	}
	if err := a.applyMemberProfileForCurrentLicense(); err != nil {
		log.Warn().Err(err).Msg("license activated but member preferences could not be restored")
	}
	if err := a.applyMemberAccountForCurrentLicense(); err != nil {
		log.Warn().Err(err).Msg("license activated but member Clash account could not be restored")
	}
	if err := a.applyMemberAutomationForCurrentLicense(); err != nil {
		log.Warn().Err(err).Msg("license activated but member automation preferences could not be restored")
	}
	a.clearInMemoryMemberRuntimeState()
	if err := a.restoreMemberRuntimeState(false); err != nil {
		log.Warn().Err(err).Msg("license activated but member runtime state could not be restored")
	}
	a.loadPersistedStats()
	if a.supportReporter != nil {
		a.supportReporter.Flush(context.Background())
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "license_state", state)
	}
	return state, nil
}

// DeactivateLicense removes only the local activation data. Server-side
// machine binding remains until an authorized developer/admin resets it.
func (a *App) DeactivateLicense() error {
	if a.license == nil {
		return nil
	}

	// A local deactivation must immediately end an active automation session.
	if a.IsRunning() {
		_ = a.StopBot()
	}
	if err := a.waitForBotTeardown(20 * time.Second); err != nil {
		return err
	}

	// Flush and snapshot the current member without deleting shared runtime
	// files yet. Every destructive step happens only after the entitlement and
	// cleared runtime config can both be committed successfully.
	a.saveStats()
	if err := a.archiveMemberRuntimeState(false); err != nil {
		return err
	}

	cfg := config.LoadOrDefault("config.json")
	oldTag := strings.TrimSpace(cfg.Account.PlayerTag)
	if err := a.persistMemberAccountTag(oldTag); err != nil {
		return err
	}

	cfg.Account.PlayerTag = ""
	cfg.Account.LegacyAPIKey = ""
	if err := config.Save("config.json", cfg); err != nil {
		return err
	}

	if err := a.license.DeactivateLocal(); err != nil {
		// Best-effort rollback: the member still owns the active entitlement,
		// so restore their account into the shared runtime config.
		cfg.Account.PlayerTag = oldTag
		_ = config.Save("config.json", cfg)
		return err
	}

	// The entitlement is now locally removed and the shared config no longer
	// carries member identity. It is safe to clear shared runtime artifacts.
	clearSharedMemberRuntimeStateFiles()
	a.clearInMemoryMemberRuntimeState()
	_ = os.Remove(accountProfileCachePath())

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "license_state", a.license.GetState())
	}
	return nil
}

func (a *App) developerControlGET(path string) ([]map[string]any, error) {
	if a.license == nil {
		return nil, fmt.Errorf("license service is not initialized")
	}
	state := a.license.GetState()
	if !state.Activated || (state.Role != licensing.RoleDeveloper && state.Role != licensing.RoleAdmin) {
		return nil, fmt.Errorf("developer license required")
	}

	cfg := config.LoadOrDefault("config.json")
	baseURL := clashControlServiceURL(cfg)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-ClashGO-License", a.license.LicenseKey())
	req.Header.Set("X-ClashGO-Machine", a.license.MachineID())
	req.Header.Set("User-Agent", "ClashGO/"+version)

	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		var payload struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &payload)
		if payload.Message == "" {
			payload.Message = resp.Status
		}
		return nil, fmt.Errorf("developer support service: %s", payload.Message)
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	var rows []map[string]any
	for _, key := range []string{"incidents", "licenses"} {
		if raw, ok := envelope[key]; ok {
			if err := json.Unmarshal(raw, &rows); err != nil {
				return nil, err
			}
			return rows, nil
		}
	}
	return []map[string]any{}, nil
}

func (a *App) GetDeveloperIncidents() ([]map[string]any, error) {
	return a.developerControlGET("/v1/developer/incidents")
}

func (a *App) GetDeveloperLicenses() ([]map[string]any, error) {
	return a.developerControlGET("/v1/developer/licenses")
}

// GetConfig returns the current config.json settings
func (a *App) GetConfig() *config.BotConfig {
	cfg := config.LoadOrDefault("config.json")
	if cfg.Attack.StrategyFile != "" {
		cfg.Attack.StrategyFile = filepath.Base(cfg.Attack.StrategyFile)
	}
	return cfg
}

type ClashAccountPublicConfig struct {
	PlayerTag         string `json:"player_tag"`
	ServiceConfigured bool   `json:"service_configured"`
	ServiceURL        string `json:"service_url,omitempty"`
}

type ClashPlayerClan struct {
	Tag       string `json:"tag"`
	Name      string `json:"name"`
	ClanLevel int    `json:"clanLevel"`
}

type ClashPlayerLeague struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ClashPlayerUnit struct {
	Name     string `json:"name"`
	Level    int    `json:"level"`
	MaxLevel int    `json:"maxLevel"`
	Village  string `json:"village"`
}

type ClashPlayerProfile struct {
	Tag                 string             `json:"tag"`
	Name                string             `json:"name"`
	TownHallLevel       int                `json:"townHallLevel"`
	TownHallWeaponLevel int                `json:"townHallWeaponLevel,omitempty"`
	ExpLevel            int                `json:"expLevel"`
	Trophies            int                `json:"trophies"`
	BestTrophies        int                `json:"bestTrophies"`
	WarStars            int                `json:"warStars"`
	AttackWins          int                `json:"attackWins"`
	DefenseWins         int                `json:"defenseWins"`
	Donations           int                `json:"donations"`
	DonationsReceived   int                `json:"donationsReceived"`
	Clan                *ClashPlayerClan   `json:"clan,omitempty"`
	League              *ClashPlayerLeague `json:"league,omitempty"`
	Troops              []ClashPlayerUnit  `json:"troops"`
	Heroes              []ClashPlayerUnit  `json:"heroes"`
	Spells              []ClashPlayerUnit  `json:"spells"`
	HeroEquipment       []ClashPlayerUnit  `json:"heroEquipment"`
}

func normalizePlayerTag(tag string) (string, error) {
	tag = strings.ToUpper(strings.TrimSpace(tag))
	tag = strings.ReplaceAll(tag, " ", "")
	if tag == "" {
		return "", fmt.Errorf("player tag is required")
	}
	if !strings.HasPrefix(tag, "#") {
		tag = "#" + tag
	}
	body := tag[1:]
	if len(body) < 5 || len(body) > 15 {
		return "", fmt.Errorf("invalid player tag length")
	}
	for _, r := range body {
		if !(r >= '0' && r <= '9') && !(r >= 'A' && r <= 'Z') {
			return "", fmt.Errorf("invalid player tag")
		}
	}
	return tag, nil
}

func clashAccountServiceURL(cfg *config.BotConfig) string {
	if raw := strings.TrimSpace(os.Getenv("CLASHGO_ACCOUNT_API_URL")); raw != "" {
		return strings.TrimRight(raw, "/")
	}
	if raw := strings.TrimSpace(accountServiceURL); raw != "" {
		return strings.TrimRight(raw, "/")
	}
	if cfg != nil {
		if raw := strings.TrimSpace(cfg.Account.ProxyURL); raw != "" {
			return strings.TrimRight(raw, "/")
		}
	}
	// Development fallback. Production builds should inject
	// CLASHGO_ACCOUNT_API_URL or persist account.proxy_url.
	return "http://127.0.0.1:8787"
}

func clashAccountServiceConfigured(cfg *config.BotConfig) bool {
	if strings.TrimSpace(os.Getenv("CLASHGO_ACCOUNT_API_URL")) != "" {
		return true
	}
	if strings.TrimSpace(accountServiceURL) != "" {
		return true
	}
	return cfg != nil && strings.TrimSpace(cfg.Account.ProxyURL) != ""
}

func clashControlServiceURL(cfg *config.BotConfig) string {
	if raw := strings.TrimSpace(os.Getenv("CLASHGO_CONTROL_API_URL")); raw != "" {
		return strings.TrimRight(raw, "/")
	}
	if raw := strings.TrimSpace(controlServiceURL); raw != "" {
		return strings.TrimRight(raw, "/")
	}
	// Local development reuses the combined Go service. Production builds
	// normally embed the Cloudflare Worker URL independently.
	return clashAccountServiceURL(cfg)
}

// GetAccountConfig returns safe account metadata only. End users never see,
// create, or store a Clash developer API key in the desktop application.
func (a *App) GetAccountConfig() ClashAccountPublicConfig {
	cfg := config.LoadOrDefault("config.json")
	serviceURL := clashAccountServiceURL(cfg)
	return ClashAccountPublicConfig{
		PlayerTag:         cfg.Account.PlayerTag,
		ServiceConfigured: clashAccountServiceConfigured(cfg),
		ServiceURL:        serviceURL,
	}
}

func accountProfileCachePath() string {
	return paths.ResolveConfig("account_profile.json")
}

func savePlayerProfileFile(path string, profile *ClashPlayerProfile) error {
	if strings.TrimSpace(path) == "" || profile == nil || strings.TrimSpace(profile.Tag) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	backupPath := path + ".bak"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	_ = os.Remove(backupPath)
	hadOriginal := false
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, backupPath); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		hadOriginal = true
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if hadOriginal {
			_ = os.Rename(backupPath, path)
		}
		_ = os.Remove(tmpPath)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func loadPlayerProfileFile(path string) (*ClashPlayerProfile, bool) {
	if strings.TrimSpace(path) == "" {
		return nil, false
	}
	read := func(candidate string) (*ClashPlayerProfile, bool) {
		data, err := os.ReadFile(candidate)
		if err != nil {
			return nil, false
		}
		var profile ClashPlayerProfile
		if json.Unmarshal(data, &profile) != nil || strings.TrimSpace(profile.Tag) == "" {
			return nil, false
		}
		return &profile, true
	}
	if profile, ok := read(path); ok {
		return profile, true
	}
	if profile, ok := read(path + ".bak"); ok {
		_ = savePlayerProfileFile(path, profile)
		return profile, true
	}
	return nil, false
}

func (a *App) memberPlayerProfilePath() string {
	if a == nil || a.license == nil {
		return ""
	}
	id := strings.TrimSpace(a.license.ProfileID())
	if id == "" {
		return ""
	}
	return paths.ResolveConfig(filepath.Join("members", id+".player.json"))
}

func applySimpleAutomationDefaults(cfg *config.BotConfig) {
	if cfg == nil || !cfg.Automation.SimpleMode {
		return
	}

	cfg.Automation.AutoFarmProfile = true
	cfg.Automation.AutoArmyGuard = true
	cfg.Automation.AutoResourceTracking = true
	cfg.Automation.AutoProfileSync = true

	// Keep automatic mode quiet and stable. Explicit failure diagnostics still
	// write their targeted captures when something goes wrong.
	cfg.Debug.SaveScreenshots = false
	cfg.Debug.TemplateDebug = false
	cfg.Debug.StateDebug = false

	// A cached public profile is enough to recover the correct HDV farm
	// profile even when the account service is temporarily offline.
	if data, err := os.ReadFile(accountProfileCachePath()); err == nil {
		var profile ClashPlayerProfile
		if json.Unmarshal(data, &profile) == nil && profile.TownHallLevel > 0 {
			if _, ok := cfg.Attack.Farm.Profiles[fmt.Sprintf("%d", profile.TownHallLevel)]; ok {
				cfg.Attack.Farm.TownHall = profile.TownHallLevel
				cfg.Attack.Farm.Enabled = true
			}
		}
	}
}

// GetCachedPlayerProfile returns the most recent successful account sync.
// The UI can render this immediately at launch while the network refresh runs
// in the background, so reopening ClashGO never presents an empty account page.
func (a *App) GetCachedPlayerProfile() *ClashPlayerProfile {
	profile, ok := loadPlayerProfileFile(accountProfileCachePath())
	if !ok {
		return nil
	}
	return profile
}

func persistPlayerProfile(profile *ClashPlayerProfile) {
	_ = savePlayerProfileFile(accountProfileCachePath(), profile)
}

func (a *App) persistPlayerProfileForCurrentLicense(profile *ClashPlayerProfile) {
	if profile == nil || strings.TrimSpace(profile.Tag) == "" {
		return
	}
	persistPlayerProfile(profile)
	if path := a.memberPlayerProfilePath(); path != "" {
		if err := savePlayerProfileFile(path, profile); err != nil {
			log.Warn().Err(err).Msg("could not persist per-license Clash profile cache")
		}
	}
}

// SetSimpleMode toggles the one-click automation experience. Turning it on
// also enables the dependent automatic behaviors so users do not have to hunt
// through multiple settings pages to obtain a coherent setup.
type MemberSettings struct {
	InterfaceLevel       string `json:"interface_level,omitempty"`
	SpeedProfile         string `json:"speed_profile"`
	MaxAttacksPerHour    int    `json:"max_attacks_per_hour"`
	MaxAttacksPerSession int    `json:"max_attacks_per_session"`
	BreakEveryAttacks    int    `json:"break_every_attacks"`
	BreakMinutes         int    `json:"break_minutes"`
	AdaptiveSearch       bool   `json:"adaptive_search"`
	AutoProfileSync      bool   `json:"auto_profile_sync"`
	AutoArmyGuard        bool   `json:"auto_army_guard"`
	AutoResourceTracking bool   `json:"auto_resource_tracking"`
}

func normalizeSpeedProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "cautious":
		return "cautious"
	case "fast":
		return "fast"
	default:
		return "normal"
	}
}

func applyMemberSpeedProfile(cfg *config.BotConfig, profile string) {
	if cfg == nil {
		return
	}
	profile = normalizeSpeedProfile(profile)
	cfg.Automation.SpeedProfile = profile

	switch profile {
	case "cautious":
		cfg.Attack.DropDelay = config.Duration{Duration: 700 * time.Millisecond}
		cfg.Attack.SpellDelay = config.Duration{Duration: 2200 * time.Millisecond}
		cfg.Attack.MinSecondsBetweenAttacks = 45
		cfg.Automation.MaxAttacksPerHour = 8
		if cfg.Automation.BreakEveryAttacks <= 0 {
			cfg.Automation.BreakEveryAttacks = 4
		}
		if cfg.Automation.BreakDuration.Duration <= 0 {
			cfg.Automation.BreakDuration = config.Duration{Duration: 4 * time.Minute}
		}
	case "fast":
		cfg.Attack.DropDelay = config.Duration{Duration: 300 * time.Millisecond}
		cfg.Attack.SpellDelay = config.Duration{Duration: 1300 * time.Millisecond}
		cfg.Attack.MinSecondsBetweenAttacks = 20
		cfg.Automation.MaxAttacksPerHour = 16
		if cfg.Automation.BreakEveryAttacks <= 0 {
			cfg.Automation.BreakEveryAttacks = 6
		}
		if cfg.Automation.BreakDuration.Duration <= 0 {
			cfg.Automation.BreakDuration = config.Duration{Duration: 2 * time.Minute}
		}
	default:
		cfg.Attack.DropDelay = config.Duration{Duration: 500 * time.Millisecond}
		cfg.Attack.SpellDelay = config.Duration{Duration: 2 * time.Second}
		cfg.Attack.MinSecondsBetweenAttacks = 30
		cfg.Automation.MaxAttacksPerHour = 12
		if cfg.Automation.BreakEveryAttacks <= 0 {
			cfg.Automation.BreakEveryAttacks = 5
		}
		if cfg.Automation.BreakDuration.Duration <= 0 {
			cfg.Automation.BreakDuration = config.Duration{Duration: 3 * time.Minute}
		}
	}
}

func applyMemberPreset(settings MemberSettings, preset string) (MemberSettings, error) {
	preset = strings.ToLower(strings.TrimSpace(preset))
	settings = sanitizeMemberSettings(settings)
	settings.AdaptiveSearch = true

	switch preset {
	case "short":
		settings.SpeedProfile = "normal"
		settings.MaxAttacksPerHour = 12
		settings.MaxAttacksPerSession = 10
		settings.BreakEveryAttacks = 5
		settings.BreakMinutes = 3
	case "balanced":
		settings.SpeedProfile = "normal"
		settings.MaxAttacksPerHour = 12
		settings.MaxAttacksPerSession = 50
		settings.BreakEveryAttacks = 5
		settings.BreakMinutes = 3
	case "fast":
		settings.SpeedProfile = "fast"
		settings.MaxAttacksPerHour = 16
		settings.MaxAttacksPerSession = 100
		settings.BreakEveryAttacks = 6
		settings.BreakMinutes = 2
	default:
		return settings, fmt.Errorf("unknown member preset %q", preset)
	}
	return sanitizeMemberSettings(settings), nil
}

func sanitizeMemberSettings(settings MemberSettings) MemberSettings {
	switch strings.ToLower(strings.TrimSpace(settings.InterfaceLevel)) {
	case "advanced":
		settings.InterfaceLevel = "advanced"
	default:
		settings.InterfaceLevel = "simple"
	}
	settings.SpeedProfile = normalizeSpeedProfile(settings.SpeedProfile)

	if settings.MaxAttacksPerHour < 1 {
		settings.MaxAttacksPerHour = 1
	}
	if settings.MaxAttacksPerHour > 24 {
		settings.MaxAttacksPerHour = 24
	}

	// Zero is the migration value for profiles written before this setting
	// existed. Keep their historical ClashGO default instead of turning an
	// old profile into a one-attack session.
	if settings.MaxAttacksPerSession <= 0 {
		settings.MaxAttacksPerSession = 100
	}
	if settings.MaxAttacksPerSession > 500 {
		settings.MaxAttacksPerSession = 500
	}

	if settings.BreakEveryAttacks < 0 {
		settings.BreakEveryAttacks = 0
	}
	if settings.BreakEveryAttacks > 20 {
		settings.BreakEveryAttacks = 20
	}

	if settings.BreakMinutes < 0 {
		settings.BreakMinutes = 0
	}
	if settings.BreakMinutes > 30 {
		settings.BreakMinutes = 30
	}
	return settings
}

func defaultMemberSettings() MemberSettings {
	return MemberSettings{
		InterfaceLevel:       "simple",
		SpeedProfile:         "normal",
		MaxAttacksPerHour:    12,
		MaxAttacksPerSession: 100,
		BreakEveryAttacks:    5,
		BreakMinutes:         3,
		AdaptiveSearch:       true,
		AutoProfileSync:      true,
		AutoArmyGuard:        true,
		AutoResourceTracking: true,
	}
}

func applyMemberSettingsToConfig(cfg *config.BotConfig, settings MemberSettings) {
	if cfg == nil {
		return
	}
	settings = sanitizeMemberSettings(settings)
	applyMemberSpeedProfile(cfg, settings.SpeedProfile)
	cfg.Automation.MaxAttacksPerHour = settings.MaxAttacksPerHour
	cfg.Attack.MaxAttackPerSession = settings.MaxAttacksPerSession
	cfg.Automation.BreakEveryAttacks = settings.BreakEveryAttacks
	cfg.Automation.BreakDuration = config.Duration{Duration: time.Duration(settings.BreakMinutes) * time.Minute}
	cfg.Search.AdaptiveSearch = settings.AdaptiveSearch
	cfg.Automation.AutoProfileSync = settings.AutoProfileSync
	cfg.Automation.AutoArmyGuard = settings.AutoArmyGuard
	cfg.Automation.AutoResourceTracking = settings.AutoResourceTracking
}

func (a *App) memberProfilePath() string {
	if a == nil || a.license == nil {
		return ""
	}
	id := strings.TrimSpace(a.license.ProfileID())
	if id == "" {
		return ""
	}
	return paths.ResolveConfig(filepath.Join("members", id+".json"))
}

func saveMemberProfileFile(path string, settings MemberSettings) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	settings = sanitizeMemberSettings(settings)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	backupPath := path + ".bak"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	_ = os.Remove(backupPath)
	hadOriginal := false
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, backupPath); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		hadOriginal = true
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if hadOriginal {
			_ = os.Rename(backupPath, path)
		}
		_ = os.Remove(tmpPath)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func loadMemberProfileFile(path string) (MemberSettings, bool) {
	if strings.TrimSpace(path) == "" {
		return MemberSettings{}, false
	}

	read := func(candidate string) (MemberSettings, bool) {
		data, err := os.ReadFile(candidate)
		if err != nil {
			return MemberSettings{}, false
		}
		var settings MemberSettings
		if json.Unmarshal(data, &settings) != nil {
			return MemberSettings{}, false
		}
		return sanitizeMemberSettings(settings), true
	}

	if settings, ok := read(path); ok {
		return settings, true
	}
	if settings, ok := read(path + ".bak"); ok {
		// Best-effort self-heal so the next launch reads a valid primary file.
		_ = saveMemberProfileFile(path, settings)
		return settings, true
	}
	return MemberSettings{}, false
}

func (a *App) persistMemberProfile(settings MemberSettings) error {
	return saveMemberProfileFile(a.memberProfilePath(), settings)
}

func (a *App) loadMemberProfile() (MemberSettings, bool) {
	return loadMemberProfileFile(a.memberProfilePath())
}

func (a *App) applyMemberProfileForCurrentLicense() error {
	if a == nil || a.license == nil || !a.license.GetState().Activated {
		return nil
	}
	settings, ok := a.loadMemberProfile()
	if !ok {
		settings = defaultMemberSettings()
		if err := a.persistMemberProfile(settings); err != nil {
			return err
		}
	}

	cfg := config.LoadOrDefault("config.json")
	// InterfaceLevel is presentation only. Automatic/manual bot behaviour is
	// restored independently by the per-license automation profile.
	applyMemberSettingsToConfig(cfg, settings)
	if err := config.Save("config.json", cfg); err != nil {
		return err
	}

	a.mu.Lock()
	if a.bot != nil {
		a.bot.UpdateConfig(cfg)
	}
	a.mu.Unlock()
	return nil
}

type memberAccountProfile struct {
	PlayerTag string `json:"player_tag"`
}

func (a *App) memberAccountPath() string {
	if a == nil || a.license == nil {
		return ""
	}
	id := strings.TrimSpace(a.license.ProfileID())
	if id == "" {
		return ""
	}
	return paths.ResolveConfig(filepath.Join("members", id+".account.json"))
}

func saveMemberAccountFile(path, tag string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload := memberAccountProfile{PlayerTag: strings.TrimSpace(tag)}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	backupPath := path + ".bak"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	_ = os.Remove(backupPath)
	hadOriginal := false
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, backupPath); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		hadOriginal = true
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if hadOriginal {
			_ = os.Rename(backupPath, path)
		}
		_ = os.Remove(tmpPath)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func loadMemberAccountFile(path string) (string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	read := func(candidate string) (string, bool) {
		data, err := os.ReadFile(candidate)
		if err != nil {
			return "", false
		}
		var profile memberAccountProfile
		if json.Unmarshal(data, &profile) != nil {
			return "", false
		}
		return strings.TrimSpace(profile.PlayerTag), true
	}

	if tag, ok := read(path); ok {
		return tag, true
	}
	if tag, ok := read(path + ".bak"); ok {
		_ = saveMemberAccountFile(path, tag)
		return tag, true
	}
	return "", false
}

func (a *App) persistMemberAccountTag(tag string) error {
	return saveMemberAccountFile(a.memberAccountPath(), tag)
}

func (a *App) loadMemberAccountTag() (string, bool) {
	return loadMemberAccountFile(a.memberAccountPath())
}

func clearCachedPlayerProfileIfDifferent(tag string) {
	data, err := os.ReadFile(accountProfileCachePath())
	if err != nil {
		return
	}
	var profile ClashPlayerProfile
	if json.Unmarshal(data, &profile) != nil {
		_ = os.Remove(accountProfileCachePath())
		return
	}
	if !strings.EqualFold(strings.TrimSpace(profile.Tag), strings.TrimSpace(tag)) {
		_ = os.Remove(accountProfileCachePath())
	}
}

func (a *App) applyMemberAccountForCurrentLicense() error {
	if a == nil || a.license == nil || !a.license.GetState().Activated {
		return nil
	}
	cfg := config.LoadOrDefault("config.json")
	tag, exists := a.loadMemberAccountTag()
	if !exists {
		// Migration path for the first version that introduces per-license
		// accounts: the currently linked tag belongs to the currently active
		// license. Deactivation clears it, so a future different license can
		// never accidentally adopt the previous member's account.
		tag = strings.TrimSpace(cfg.Account.PlayerTag)
		if err := a.persistMemberAccountTag(tag); err != nil {
			return err
		}
	}

	cfg.Account.PlayerTag = tag
	cfg.Account.LegacyAPIKey = ""
	if err := config.Save("config.json", cfg); err != nil {
		return err
	}

	// Restore the member's last successful public Clash profile so HDV/farm
	// information is immediately available even when the account service is
	// offline. Migrate the legacy shared cache into the current member profile
	// only when its player tag matches.
	memberCache := a.memberPlayerProfilePath()
	if profile, ok := loadPlayerProfileFile(memberCache); ok && strings.EqualFold(strings.TrimSpace(profile.Tag), tag) {
		_ = savePlayerProfileFile(accountProfileCachePath(), profile)
	} else if legacy, ok := loadPlayerProfileFile(accountProfileCachePath()); ok && strings.EqualFold(strings.TrimSpace(legacy.Tag), tag) {
		_ = savePlayerProfileFile(memberCache, legacy)
	} else {
		clearCachedPlayerProfileIfDifferent(tag)
	}

	a.mu.Lock()
	if a.bot != nil {
		a.bot.UpdateConfig(cfg)
	}
	a.mu.Unlock()
	return nil
}

type MemberAutomationProfile struct {
	SimpleMode          *bool                         `json:"simple_mode,omitempty"`
	SearchEnabled       bool                          `json:"search_enabled"`
	MinLootGold         int                           `json:"min_loot_gold"`
	MinLootElixir       int                           `json:"min_loot_elixir"`
	MinLootDarkElixir   int                           `json:"min_loot_dark_elixir"`
	UpgradeWalls        bool                          `json:"upgrade_walls"`
	StrategyFile        string                        `json:"strategy_file"`
	StallTimerSeconds   int                           `json:"stall_timer_seconds"`
	LootExitEnabled     bool                          `json:"loot_exit_enabled"`
	LootExitPercent     int                           `json:"loot_exit_percent"`
	FarmEnabled         bool                          `json:"farm_enabled"`
	FarmTownHall        int                           `json:"farm_town_hall"`
	FarmProfiles        map[string]config.FarmProfile `json:"farm_profiles,omitempty"`
}

func memberAutomationFromConfig(cfg *config.BotConfig) MemberAutomationProfile {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	simpleMode := cfg.Automation.SimpleMode
	profile := MemberAutomationProfile{
		SimpleMode:        &simpleMode,
		SearchEnabled:     cfg.Search.Enabled,
		MinLootGold:       cfg.Search.MinLootGold,
		MinLootElixir:     cfg.Search.MinLootElixir,
		MinLootDarkElixir: cfg.Search.MinLootDarkElixir,
		UpgradeWalls:      cfg.Upgrade.UpgradeWalls,
		StrategyFile:      filepath.Base(cfg.Attack.StrategyFile),
		StallTimerSeconds: cfg.Attack.StallTimerSeconds,
		LootExitEnabled:   cfg.Attack.LootExitEnabled,
		LootExitPercent:   cfg.Attack.LootExitPercent,
		FarmEnabled:       cfg.Attack.Farm.Enabled,
		FarmTownHall:      cfg.Attack.Farm.TownHall,
		FarmProfiles:      map[string]config.FarmProfile{},
	}
	for key, value := range cfg.Attack.Farm.Profiles {
		profile.FarmProfiles[key] = value
	}
	return sanitizeMemberAutomationProfile(profile)
}

func sanitizeMemberAutomationProfile(profile MemberAutomationProfile) MemberAutomationProfile {
	const maxLootThreshold = 10_000_000
	profile.MinLootGold = max(0, min(maxLootThreshold, profile.MinLootGold))
	profile.MinLootElixir = max(0, min(maxLootThreshold, profile.MinLootElixir))
	profile.MinLootDarkElixir = max(0, min(maxLootThreshold, profile.MinLootDarkElixir))
	profile.StallTimerSeconds = max(0, min(600, profile.StallTimerSeconds))
	profile.LootExitPercent = max(0, min(100, profile.LootExitPercent))
	if profile.FarmTownHall < 8 || profile.FarmTownHall > 18 {
		profile.FarmTownHall = 18
	}
	profile.StrategyFile = filepath.Base(strings.TrimSpace(profile.StrategyFile))
	if profile.FarmProfiles == nil {
		profile.FarmProfiles = map[string]config.FarmProfile{}
	}
	return profile
}

func applyMemberAutomationToConfig(cfg *config.BotConfig, profile MemberAutomationProfile) {
	if cfg == nil {
		return
	}
	profile = sanitizeMemberAutomationProfile(profile)
	if profile.SimpleMode != nil {
		cfg.Automation.SimpleMode = *profile.SimpleMode
	}
	cfg.Search.Enabled = profile.SearchEnabled
	cfg.Search.MinLootGold = profile.MinLootGold
	cfg.Search.MinLootElixir = profile.MinLootElixir
	cfg.Search.MinLootDarkElixir = profile.MinLootDarkElixir
	cfg.Upgrade.UpgradeWalls = profile.UpgradeWalls
	cfg.Attack.StallTimerSeconds = profile.StallTimerSeconds
	cfg.Attack.LootExitEnabled = profile.LootExitEnabled
	cfg.Attack.LootExitPercent = profile.LootExitPercent

	if profile.StrategyFile != "" {
		candidate := paths.Resolve(filepath.Join("strategies", filepath.Base(profile.StrategyFile)))
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			cfg.Attack.StrategyFile = candidate
		}
	}

	cfg.Attack.Farm.Enabled = profile.FarmEnabled
	cfg.Attack.Farm.TownHall = profile.FarmTownHall
	if len(profile.FarmProfiles) > 0 {
		cfg.Attack.Farm.Profiles = make(map[string]config.FarmProfile, len(profile.FarmProfiles))
		for key, value := range profile.FarmProfiles {
			cfg.Attack.Farm.Profiles[key] = value
		}
	}
}

func (a *App) memberAutomationPath() string {
	if a == nil || a.license == nil {
		return ""
	}
	id := strings.TrimSpace(a.license.ProfileID())
	if id == "" {
		return ""
	}
	return paths.ResolveConfig(filepath.Join("members", id+".automation.json"))
}

func saveMemberAutomationFile(path string, profile MemberAutomationProfile) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	profile = sanitizeMemberAutomationProfile(profile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	backupPath := path + ".bak"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	_ = os.Remove(backupPath)
	hadOriginal := false
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, backupPath); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		hadOriginal = true
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if hadOriginal {
			_ = os.Rename(backupPath, path)
		}
		_ = os.Remove(tmpPath)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func loadMemberAutomationFile(path string) (MemberAutomationProfile, bool) {
	if strings.TrimSpace(path) == "" {
		return MemberAutomationProfile{}, false
	}
	read := func(candidate string) (MemberAutomationProfile, bool) {
		data, err := os.ReadFile(candidate)
		if err != nil {
			return MemberAutomationProfile{}, false
		}
		var profile MemberAutomationProfile
		if json.Unmarshal(data, &profile) != nil {
			return MemberAutomationProfile{}, false
		}
		return sanitizeMemberAutomationProfile(profile), true
	}
	if profile, ok := read(path); ok {
		return profile, true
	}
	if profile, ok := read(path + ".bak"); ok {
		_ = saveMemberAutomationFile(path, profile)
		return profile, true
	}
	return MemberAutomationProfile{}, false
}

func (a *App) persistMemberAutomation(cfg *config.BotConfig) error {
	if a == nil || a.license == nil || !a.license.GetState().Activated {
		return nil
	}
	return saveMemberAutomationFile(a.memberAutomationPath(), memberAutomationFromConfig(cfg))
}

func (a *App) applyMemberAutomationForCurrentLicense() error {
	if a == nil || a.license == nil || !a.license.GetState().Activated {
		return nil
	}
	cfg := config.LoadOrDefault("config.json")
	profile, ok := loadMemberAutomationFile(a.memberAutomationPath())
	if !ok {
		// Upgrade migration: the first active licence adopts the user's current
		// advanced automation choices exactly once.
		return saveMemberAutomationFile(a.memberAutomationPath(), memberAutomationFromConfig(cfg))
	}
	if profile.SimpleMode == nil {
		// Profiles written before automatic/manual mode became independent from
		// the UI level inherit today's behavior once, then persist it.
		current := cfg.Automation.SimpleMode
		profile.SimpleMode = &current
		if err := saveMemberAutomationFile(a.memberAutomationPath(), profile); err != nil {
			return err
		}
	}
	applyMemberAutomationToConfig(cfg, profile)

	// In Simple mode the linked member account remains authoritative for the
	// farm HDV. applyMemberAccountForCurrentLicense() has already restored this
	// licence's cached Clash profile before this function is called, so this
	// re-applies only the existing automatic defaults using the correct member
	// cache. Advanced mode keeps the member's explicitly saved farm selection.
	if cfg.Automation.SimpleMode {
		applySimpleAutomationDefaults(cfg)
	}

	if err := config.Save("config.json", cfg); err != nil {
		return err
	}
	a.mu.Lock()
	if a.bot != nil {
		a.bot.UpdateConfig(cfg)
	}
	a.mu.Unlock()
	return nil
}

func (a *App) GetMemberSettings() MemberSettings {
	cfg := config.LoadOrDefault("config.json")
	profile := normalizeSpeedProfile(cfg.Automation.SpeedProfile)
	interfaceLevel := "simple"
	if saved, ok := a.loadMemberProfile(); ok && saved.InterfaceLevel == "advanced" {
		interfaceLevel = "advanced"
	}
	return MemberSettings{
		InterfaceLevel:       interfaceLevel,
		SpeedProfile:         profile,
		MaxAttacksPerHour:    cfg.Automation.MaxAttacksPerHour,
		MaxAttacksPerSession: cfg.Attack.MaxAttackPerSession,
		BreakEveryAttacks:    cfg.Automation.BreakEveryAttacks,
		BreakMinutes:         int(cfg.Automation.BreakDuration.Duration / time.Minute),
		AdaptiveSearch:       cfg.Search.AdaptiveSearch,
		AutoProfileSync:      cfg.Automation.AutoProfileSync,
		AutoArmyGuard:        cfg.Automation.AutoArmyGuard,
		AutoResourceTracking: cfg.Automation.AutoResourceTracking,
	}
}

func (a *App) GetMemberInterfaceLevel() string {
	if a == nil || a.license == nil || !a.license.GetState().Activated {
		return ""
	}
	if settings, ok := a.loadMemberProfile(); ok {
		return sanitizeMemberSettings(settings).InterfaceLevel
	}
	return "simple"
}

func (a *App) SaveMemberInterfaceLevel(level string) error {
	level = strings.ToLower(strings.TrimSpace(level))
	if level != "simple" && level != "advanced" {
		return fmt.Errorf("member interface level must be simple or advanced")
	}

	// In unenforced/local beta mode the React localStorage preference remains
	// the fallback. A licensed member stores this presentation preference in
	// their own profile without mutating any bot automation setting.
	if a.license == nil || !a.license.GetState().Activated {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	cfg := config.LoadOrDefault("config.json")
	settings, ok := a.loadMemberProfile()
	if !ok {
		settings = MemberSettings{
			InterfaceLevel:       level,
			SpeedProfile:         normalizeSpeedProfile(cfg.Automation.SpeedProfile),
			MaxAttacksPerHour:    cfg.Automation.MaxAttacksPerHour,
			MaxAttacksPerSession: cfg.Attack.MaxAttackPerSession,
			BreakEveryAttacks:    cfg.Automation.BreakEveryAttacks,
			BreakMinutes:         int(cfg.Automation.BreakDuration.Duration / time.Minute),
			AdaptiveSearch:       cfg.Search.AdaptiveSearch,
			AutoProfileSync:      cfg.Automation.AutoProfileSync,
			AutoArmyGuard:        cfg.Automation.AutoArmyGuard,
			AutoResourceTracking: cfg.Automation.AutoResourceTracking,
		}
	}
	settings.InterfaceLevel = level
	return a.persistMemberProfile(settings)
}

func (a *App) testSessionRestorePath() string {
	if a != nil && a.license != nil {
		if id := strings.TrimSpace(a.license.ProfileID()); id != "" {
			return paths.ResolveConfig(filepath.Join("members", id+".test-session.json"))
		}
	}
	return paths.ResolveConfig("test-session-restore.json")
}

func removeTestSessionRestoreFiles(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	_ = os.Remove(path)
	_ = os.Remove(path + ".bak")
	_ = os.Remove(path + ".tmp")
}

func (a *App) restoreTestSessionSettings() error {
	if a == nil {
		return nil
	}
	path := a.testSessionRestorePath()
	settings, ok := loadMemberProfileFile(path)
	if !ok {
		return nil
	}
	if _, err := a.SaveMemberSettings(settings); err != nil {
		return err
	}
	removeTestSessionRestoreFiles(path)
	log.Info().Msg("temporary test-session settings restored")
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "member_test_session_restored", map[string]interface{}{
			"message": "Les réglages membre précédents ont été restaurés.",
		})
	}
	return nil
}

func (a *App) StartTestSession(gold, elixir, dark int, upgradeWalls bool, searchEnabled bool) BotStatus {
	a.mu.Lock()
	busy := a.bot != nil || a.cancel != nil || a.stopping
	a.mu.Unlock()
	if busy {
		return BotStatus{Running: false, Message: "Une session ClashGO est déjà active ou en cours d’arrêt"}
	}

	// Recover an interrupted previous test before creating a new backup.
	if err := a.restoreTestSessionSettings(); err != nil {
		return BotStatus{Running: false, Message: "Impossible de restaurer les réglages avant le test : " + err.Error()}
	}

	original := a.GetMemberSettings()
	path := a.testSessionRestorePath()
	if err := saveMemberProfileFile(path, original); err != nil {
		return BotStatus{Running: false, Message: "Impossible de sauvegarder les réglages avant le test : " + err.Error()}
	}

	testSettings, err := applyMemberPreset(original, "short")
	if err != nil {
		removeTestSessionRestoreFiles(path)
		return BotStatus{Running: false, Message: err.Error()}
	}
	if _, err := a.SaveMemberSettings(testSettings); err != nil {
		removeTestSessionRestoreFiles(path)
		return BotStatus{Running: false, Message: "Impossible de préparer la session test : " + err.Error()}
	}

	status := a.StartBot(gold, elixir, dark, upgradeWalls, searchEnabled)
	if !status.Running {
		if err := a.restoreTestSessionSettings(); err != nil {
			log.Error().Err(err).Msg("failed to restore member settings after rejected test session")
		}
	}
	return status
}

func (a *App) ApplyMemberPreset(preset string) (MemberSettings, error) {
	current := a.GetMemberSettings()
	next, err := applyMemberPreset(current, preset)
	if err != nil {
		return MemberSettings{}, err
	}
	return a.SaveMemberSettings(next)
}

func (a *App) SaveMemberSettings(settings MemberSettings) (MemberSettings, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	oldProfile, hadOldProfile := a.loadMemberProfile()
	if strings.TrimSpace(settings.InterfaceLevel) == "" && hadOldProfile {
		settings.InterfaceLevel = oldProfile.InterfaceLevel
	}
	settings = sanitizeMemberSettings(settings)
	cfg := config.LoadOrDefault("config.json")
	applyMemberSettingsToConfig(cfg, settings)

	// The runtime config and the per-license profile form one logical update.
	// Persist the member copy first and restore it if config.json cannot be
	// committed, so a failed save can never reappear as a different setting
	// on the next launch.
	profilePath := a.memberProfilePath()
	if err := a.persistMemberProfile(settings); err != nil {
		return MemberSettings{}, err
	}
	if err := config.Save("config.json", cfg); err != nil {
		if hadOldProfile {
			_ = saveMemberProfileFile(profilePath, oldProfile)
		} else if profilePath != "" {
			_ = os.Remove(profilePath)
			_ = os.Remove(profilePath + ".bak")
			_ = os.Remove(profilePath + ".tmp")
		}
		return MemberSettings{}, err
	}

	if a.bot != nil {
		a.bot.UpdateConfig(cfg)
		a.bot.RecordMemberSettingsChange(
			settings.SpeedProfile,
			settings.MaxAttacksPerHour,
			settings.MaxAttacksPerSession,
			settings.BreakEveryAttacks,
			settings.BreakMinutes,
		)
	}
	return a.GetMemberSettings(), nil
}

func (a *App) SetSimpleMode(enabled bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	cfg := config.LoadOrDefault("config.json")
	cfg.Automation.SimpleMode = enabled
	if enabled {
		applySimpleAutomationDefaults(cfg)
	}

	automationPath := a.memberAutomationPath()
	oldAutomation, hadOldAutomation := loadMemberAutomationFile(automationPath)
	if err := a.persistMemberAutomation(cfg); err != nil {
		return err
	}
	if err := config.Save("config.json", cfg); err != nil {
		if hadOldAutomation {
			_ = saveMemberAutomationFile(automationPath, oldAutomation)
		} else if automationPath != "" {
			_ = os.Remove(automationPath)
			_ = os.Remove(automationPath + ".bak")
			_ = os.Remove(automationPath + ".tmp")
		}
		return err
	}
	if a.bot != nil {
		a.bot.UpdateConfig(cfg)
	}
	return nil
}

// SaveAccountConfig stores only the player's tag. The Clash API credential
// lives on the ClashGO account service, never in the distributed EXE.
func (a *App) SaveAccountConfig(playerTag string) error {
	tag, err := normalizePlayerTag(playerTag)
	if err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	cfg := config.LoadOrDefault("config.json")
	cfg.Account.PlayerTag = tag
	// Purge legacy desktop keys during the first save after upgrading.
	cfg.Account.LegacyAPIKey = ""

	oldTag, hadOldTag := a.loadMemberAccountTag()
	accountPath := a.memberAccountPath()
	if err := a.persistMemberAccountTag(tag); err != nil {
		return err
	}
	if err := config.Save("config.json", cfg); err != nil {
		if hadOldTag {
			_ = saveMemberAccountFile(accountPath, oldTag)
		} else if accountPath != "" {
			_ = os.Remove(accountPath)
			_ = os.Remove(accountPath + ".bak")
			_ = os.Remove(accountPath + ".tmp")
		}
		return err
	}
	if a.bot != nil {
		a.bot.UpdateConfig(cfg)
	}
	return nil
}

// ClearAccount removes the local player link. No developer credential is
// stored on the client anymore, so unlinking is intentionally lightweight.
func (a *App) ClearAccount() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	cfg := config.LoadOrDefault("config.json")
	cfg.Account.PlayerTag = ""
	cfg.Account.LegacyAPIKey = ""

	oldTag, hadOldTag := a.loadMemberAccountTag()
	accountPath := a.memberAccountPath()
	if err := a.persistMemberAccountTag(""); err != nil {
		return err
	}
	if err := config.Save("config.json", cfg); err != nil {
		if hadOldTag {
			_ = saveMemberAccountFile(accountPath, oldTag)
		} else if accountPath != "" {
			_ = os.Remove(accountPath)
			_ = os.Remove(accountPath + ".bak")
			_ = os.Remove(accountPath + ".tmp")
		}
		return err
	}

	// Remove cached public profile only after both member identity stores are
	// durable. The explicit unlink also removes this member's private cache;
	// deactivation alone keeps it so the same licence can recover offline.
	_ = os.Remove(accountProfileCachePath())
	if memberCache := a.memberPlayerProfilePath(); memberCache != "" {
		_ = os.Remove(memberCache)
		_ = os.Remove(memberCache + ".bak")
		_ = os.Remove(memberCache + ".tmp")
	}
	if a.bot != nil {
		a.bot.UpdateConfig(cfg)
	}
	return nil
}

// GetPlayerProfile asks the ClashGO account service for the linked player.
// The service owns the official Clash developer key and forwards only the
// public player payload back to the desktop app.
func (a *App) GetPlayerProfile() (*ClashPlayerProfile, error) {
	cfg := config.LoadOrDefault("config.json")
	tag, err := normalizePlayerTag(cfg.Account.PlayerTag)
	if err != nil {
		return nil, err
	}
	serviceURL := clashAccountServiceURL(cfg)
	if strings.TrimSpace(serviceURL) == "" {
		return nil, fmt.Errorf("ClashGO account service is not configured")
	}

	endpoint := serviceURL + "/v1/player/" + url.PathEscape(tag)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ClashGO/"+version)

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ClashGO account service unavailable: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &apiErr)
		msg := strings.TrimSpace(apiErr.Message)
		if msg == "" {
			msg = strings.TrimSpace(apiErr.Reason)
		}
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("ClashGO account service: %s", msg)
	}

	var profile ClashPlayerProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, fmt.Errorf("parse Clash player profile: %w", err)
	}

	a.persistPlayerProfileForCurrentLicense(&profile)

	// Simple mode turns the linked account into the source of truth for HDV.
	// We only select a bundled profile when ClashGO actually has one for that
	// TH; unsupported/future TH values leave the user's current profile alone.
	if cfg.Automation.AutoFarmProfile {
		if _, ok := cfg.Attack.Farm.Profiles[fmt.Sprintf("%d", profile.TownHallLevel)]; ok {
			cfg.Attack.Farm.TownHall = profile.TownHallLevel
			cfg.Attack.Farm.Enabled = true

			if writeErr := config.Save("config.json", cfg); writeErr != nil {
				log.Warn().Err(writeErr).Msg("account sync: could not persist automatic farm profile")
			} else {
				if profileErr := a.persistMemberAutomation(cfg); profileErr != nil {
					log.Warn().Err(profileErr).Msg("account sync: could not persist member automation profile")
				}
				a.mu.Lock()
				if a.bot != nil {
					a.bot.UpdateConfig(cfg)
				}
				a.mu.Unlock()
			}
		}
	}

	return &profile, nil
}

// SetBlueStacksInstance persists the preferred BlueStacks 5 instance.
// An empty value restores automatic instance selection. The setting is only
// changed while the bot is stopped so the active ADB transport cannot jump
// to another emulator mid-session.
func (a *App) SetBlueStacksInstance(instance string) error {
	instance = strings.TrimSpace(instance)

	a.mu.Lock()
	if a.bot != nil || a.cancel != nil || a.stopping {
		a.mu.Unlock()
		return fmt.Errorf("wait for the bot to finish stopping before changing BlueStacks instance")
	}
	a.mu.Unlock()

	if instance != "" {
		diag := collectSystemDiagnostics()
		found := false
		for _, inst := range diag.Emulator.Instances {
			if strings.EqualFold(inst.Name, instance) {
				instance = inst.Name
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("BlueStacks instance %q was not detected", instance)
		}
	}

	cfg := config.LoadOrDefault("config.json")
	cfg.Device.BlueStacksInstance = instance
	return config.Save("config.json", cfg)
}

// GetActivity returns a compact high-level feed for the dashboard. It is
// intentionally capped and contains no screenshot/capture spam.
func (a *App) GetActivity() []telemetry.Event {
	a.mu.Lock()
	b := a.bot
	a.mu.Unlock()
	if b == nil {
		return []telemetry.Event{}
	}
	return b.RecentActivity(24)
}

// GetSessionReport returns the live session summary while the bot is
// running, or the most recently persisted report after Stop. This keeps the
// dashboard useful even when no Bot instance is active.
func (a *App) GetSessionReport() bot.SessionReport {
	a.mu.Lock()
	b := a.bot
	a.mu.Unlock()
	if b != nil {
		return b.CurrentSessionReport()
	}

	var report bot.SessionReport
	data, err := os.ReadFile(paths.ResolveConfig("output/session_reports/latest.json"))
	if err != nil {
		return report
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return bot.SessionReport{}
	}
	return report
}

// GetStats returns the bot's live runtime statistics
func (a *App) GetStats() bot.BotStats {
	a.mu.Lock()
	defer a.mu.Unlock()

	res := a.lastStats
	if a.bot != nil {
		res = mergeStats(a.lastStats, a.bot.Stats())
	}
	return res
}

// GetLogs returns the buffered logs
type CurrentArmyUnit struct {
	Name       string  `json:"name"`
	Category   string  `json:"category"`
	Count      int     `json:"count"`
	Confidence float64 `json:"confidence"`
	SlotX      int     `json:"slot_x"`
}

type CurrentArmySnapshot struct {
	Timestamp      time.Time         `json:"timestamp"`
	Units          []CurrentArmyUnit `json:"units"`
	TargetTownHall int               `json:"target_town_hall,omitempty"`
	TargetLabel    string            `json:"target_label,omitempty"`
	Ready          bool              `json:"ready"`
	Uncertain      bool              `json:"uncertain"`
	Warnings       []string          `json:"warnings,omitempty"`
}

func (a *App) GetCurrentArmy() *CurrentArmySnapshot {
	data, err := os.ReadFile(paths.ResolveConfig("current_army.json"))
	if err != nil {
		return nil
	}
	var snap CurrentArmySnapshot
	if json.Unmarshal(data, &snap) != nil {
		return nil
	}
	if len(snap.Units) == 0 {
		return nil
	}
	return &snap
}

type VillageResourceSnapshot struct {
	Timestamp   time.Time `json:"timestamp"`
	Gold        int       `json:"gold"`
	Elixir      int       `json:"elixir"`
	DarkElixir  int       `json:"dark_elixir"`
	GoldValid   bool      `json:"gold_valid"`
	ElixirValid bool      `json:"elixir_valid"`
	DarkValid   bool      `json:"dark_valid"`
	Valid       bool      `json:"valid"`
}

// GetVillageResources returns the latest locally observed home-village
// balances. These values come from the BlueStacks HUD scanner, not from the
// public Clash player API.
func (a *App) GetVillageResources() *VillageResourceSnapshot {
	data, err := os.ReadFile(paths.ResolveConfig("village_resources.json"))
	if err != nil {
		return nil
	}
	var snap VillageResourceSnapshot
	if json.Unmarshal(data, &snap) != nil || !snap.Valid {
		return nil
	}
	return &snap
}

// GetLatestAttackTrace returns the newest structured deployment trace as
// JSON. It is intended for the advanced diagnostics panel and support export;
// normal users never need to interact with it.
func (a *App) GetLatestAttackTrace() string {
	matches, err := filepath.Glob(paths.ResolveConfig("output/attack_traces/*.json"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	latest := matches[len(matches)-1]
	data, err := os.ReadFile(latest)
	if err != nil {
		return ""
	}
	return string(data)
}

func (a *App) GetVillageResourceHistory() []VillageResourceSnapshot {
	data, err := os.ReadFile(paths.ResolveConfig("village_resource_history.json"))
	if err != nil {
		return []VillageResourceSnapshot{}
	}
	var history []VillageResourceSnapshot
	if json.Unmarshal(data, &history) != nil {
		return []VillageResourceSnapshot{}
	}
	if len(history) > 1000 {
		history = history[len(history)-1000:]
	}
	return history
}

func (a *App) GetLogs() []string {
	a.logMu.RLock()
	defer a.logMu.RUnlock()
	// Return a copy to avoid race conditions.
	res := make([]string, len(a.logBuffer))
	copy(res, a.logBuffer)
	return res
}

// GetAttackHistory returns the persistent log of attacks.
//
// Reads from an in-memory cache that is refreshed:
//   - lazily on the first call after process start (cold boot, before
//     any OnStatsUpdate has fired), via double-check locking so only
//     ONE goroutine ever performs the disk read; and
//   - eagerly in OnStatsUpdate at the end of every attack.
//
// React polls this at 0.5 Hz; without the cache that translates to
// a filesystem read + JSON unmarshal + re-marshal on every tick.
// With the cache, the per-tick cost is an atomic read + slice copy
// — no syscalls, no JSON.
func (a *App) GetAttackHistory() []bot.AttackReport {
	// Fast path: cache hit. RLock allows concurrent IPC polls to
	// all read in parallel.
	a.cachedHistoryMu.RLock()
	if a.cachedHistory != nil {
		out := make([]bot.AttackReport, len(a.cachedHistory))
		copy(out, a.cachedHistory)
		a.cachedHistoryMu.RUnlock()
		return out
	}
	a.cachedHistoryMu.RUnlock()

	// Cache miss: take the write lock and lazy-load via the shared
	// helper. Double-check inside the helper means only ONE
	// goroutine in the entire process performs the disk read on
	// cold start, even if React fires multiple polls back-to-back.
	a.cachedHistoryMu.Lock()
	a.ensureHistoryLoadedLocked()
	out := make([]bot.AttackReport, len(a.cachedHistory))
	copy(out, a.cachedHistory)
	a.cachedHistoryMu.Unlock()
	return out
}

// ensureHistoryLoadedLocked populates the cache from disk IF the
// cache is still nil. Caller MUST hold cachedHistoryMu.Lock()
// (write lock) on entry. Centralising the read+parse+assign here
// keeps the two refresh paths (cold-miss GetAttackHistory + per-
// attack OnStatsUpdate) behaviourally identical — and means a fix
// to the parse logic only needs to be made once.
//
// Failure modes are intentionally non-fatal: a missing or malformed
// file leaves the previous cache untouched (nil on cold-start).
// This matches the legacy behaviour where a bad file silently
// returned []string{}.
func (a *App) ensureHistoryLoadedLocked() {
	if a.cachedHistory != nil {
		return
	}
	data, err := os.ReadFile(paths.ResolveConfig("attack_history.json"))
	if err != nil {
		return
	}
	var hist []bot.AttackReport
	if err := json.Unmarshal(data, &hist); err != nil {
		return
	}
	a.cachedHistory = hist
}

// refreshHistory is the eager (per-attack-end) refresh path. It
// runs from inside the bot's OnStatsUpdate callback, so the cadence
// is bounded by attack frequency (~one refresh every few minutes of
// normal play) — well below the 0.5 Hz React poll.
//
// This MUST force a disk re-read on every call. The naive approach
// (just calling ensureHistoryLoadedLocked) no-ops once cachedHistory
// is warm — and the cache is warmed by React's very first
// GetAttackHistory poll at app launch. That froze the UI on the
// launch-time snapshot forever: loot totals kept climbing (they're
// read live from atomics in GetStats) while the latest attack never
// appeared in history. Nulling the cache first routes through the
// shared parse path so read+parse logic stays in one place.
//
// Holds cachedHistoryMu through the disk read so concurrent React
// polls wait on the writer rather than racing the assign. The
// per-attack write-lock window is ~50 ms (read+parse) which is
// imperceptible at 0.5 Hz polling.
func (a *App) refreshHistory() {
	a.cachedHistoryMu.Lock()
	a.cachedHistory = nil
	a.ensureHistoryLoadedLocked()
	a.cachedHistoryMu.Unlock()
}

// SaveConfig updates config.json settings
func (a *App) SaveConfig(minGold, minElixir, minDE int, upgradeWalls bool, strategyFile string, searchEnabled bool, stall int, lootExitEnabled bool, lootExitPercent int) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	const maxLootThreshold = 10_000_000
	if minGold < 0 || minGold > maxLootThreshold {
		return fmt.Errorf("gold threshold must be between 0 and %d", maxLootThreshold)
	}
	if minElixir < 0 || minElixir > maxLootThreshold {
		return fmt.Errorf("elixir threshold must be between 0 and %d", maxLootThreshold)
	}
	if minDE < 0 || minDE > maxLootThreshold {
		return fmt.Errorf("dark elixir threshold must be between 0 and %d", maxLootThreshold)
	}
	if stall < 0 || stall > 600 {
		return fmt.Errorf("stall timer must be between 0 and 600 seconds")
	}

	cfg := config.LoadOrDefault("config.json")
	cfg.Search.MinLootGold = minGold
	cfg.Search.MinLootElixir = minElixir
	cfg.Search.MinLootDarkElixir = minDE
	cfg.Upgrade.UpgradeWalls = upgradeWalls
	cfg.Search.Enabled = searchEnabled
	cfg.Attack.StallTimerSeconds = stall
	cfg.Attack.LootExitEnabled = lootExitEnabled
	if lootExitPercent < 0 {
		lootExitPercent = 0
	}
	if lootExitPercent > 100 {
		lootExitPercent = 100
	}
	cfg.Attack.LootExitPercent = lootExitPercent
	if strategyFile != "" {
		name := filepath.Base(filepath.Clean(strategyFile))
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".csv" {
			return fmt.Errorf("unsupported strategy file %q", name)
		}
		resolved := paths.Resolve(filepath.Join("strategies", name))
		info, err := os.Stat(resolved)
		if err != nil || info.IsDir() {
			return fmt.Errorf("strategy %q was not found in packaged assets", name)
		}
		cfg.Attack.StrategyFile = resolved
	}

	automationPath := a.memberAutomationPath()
	oldAutomation, hadOldAutomation := loadMemberAutomationFile(automationPath)
	if err := a.persistMemberAutomation(cfg); err != nil {
		return err
	}
	if err := config.Save("config.json", cfg); err != nil {
		if hadOldAutomation {
			_ = saveMemberAutomationFile(automationPath, oldAutomation)
		} else if automationPath != "" {
			_ = os.Remove(automationPath)
			_ = os.Remove(automationPath + ".bak")
			_ = os.Remove(automationPath + ".tmp")
		}
		return err
	}

	// Apply live only after both durable copies succeeded.
	if a.bot != nil {
		a.bot.UpdateConfig(cfg)
	}
	return nil
}

// SaveFarmComposition persists the selected HDV farm profile.
// profileJSON is used instead of a large Wails struct signature so the UI can
// edit a profile freely without regenerating a bespoke binding for every field.
func (a *App) SaveFarmComposition(enabled bool, townHall int, profileJSON string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if townHall < 8 || townHall > 18 {
		return fmt.Errorf("town hall must be between 8 and 18")
	}

	var profile config.FarmProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		return fmt.Errorf("invalid farm composition: %w", err)
	}
	profile.TownHall = townHall

	if profile.TroopCapacity <= 0 || profile.SpellCapacity <= 0 {
		return fmt.Errorf("invalid farm composition capacities")
	}
	troopUsed := 0
	for _, u := range profile.Troops {
		if strings.TrimSpace(u.Name) == "" || u.Count < 0 || u.Housing <= 0 {
			return fmt.Errorf("invalid troop entry")
		}
		troopUsed += u.Count * u.Housing
	}
	if troopUsed > profile.TroopCapacity {
		return fmt.Errorf("troop composition uses %d/%d housing", troopUsed, profile.TroopCapacity)
	}

	spellUsed := 0
	for _, u := range profile.Spells {
		if strings.TrimSpace(u.Name) == "" || u.Count < 0 || u.Housing <= 0 {
			return fmt.Errorf("invalid spell entry")
		}
		spellUsed += u.Count * u.Housing
	}
	if spellUsed > profile.SpellCapacity {
		return fmt.Errorf("spell composition uses %d/%d housing", spellUsed, profile.SpellCapacity)
	}
	if len(profile.Heroes) > 4 {
		return fmt.Errorf("at most 4 heroes can be selected for the active farm army")
	}

	cfg := config.LoadOrDefault("config.json")
	if cfg.Attack.Farm.Profiles == nil {
		cfg.Attack.Farm.Profiles = map[string]config.FarmProfile{}
	}
	cfg.Attack.Farm.Enabled = enabled
	cfg.Attack.Farm.TownHall = townHall
	cfg.Attack.Farm.Profiles[fmt.Sprintf("%d", townHall)] = profile

	automationPath := a.memberAutomationPath()
	oldAutomation, hadOldAutomation := loadMemberAutomationFile(automationPath)
	if err := a.persistMemberAutomation(cfg); err != nil {
		return err
	}
	if err := config.Save("config.json", cfg); err != nil {
		if hadOldAutomation {
			_ = saveMemberAutomationFile(automationPath, oldAutomation)
		} else if automationPath != "" {
			_ = os.Remove(automationPath)
			_ = os.Remove(automationPath + ".bak")
			_ = os.Remove(automationPath + ".tmp")
		}
		return err
	}
	if a.bot != nil {
		a.bot.UpdateConfig(cfg)
	}
	return nil
}

// GetStrategies lists available strategy files
//
// Returns a non-nil empty slice when no strategies exist. A nil slice
// would marshal to JSON `null`, and the React side does
// `strategies.length` in ConfigView — null used to crash the Config
// page in the packaged app (where the assets dir initially resolved
// to an empty /assets).
func (a *App) GetStrategies() []string {
	files := []string{}
	matches, err := filepath.Glob(paths.Resolve("strategies/*.yaml"))
	if err == nil {
		for _, m := range matches {
			files = append(files, filepath.Base(filepath.ToSlash(m)))
		}
	}
	csvMatches, err := filepath.Glob(paths.Resolve("strategies/*.csv"))
	if err == nil {
		for _, m := range csvMatches {
			files = append(files, filepath.Base(filepath.ToSlash(m)))
		}
	}
	return files
}

// ---- Updater bindings ----
//
// The following methods are exposed to the React side via Wails. The
// names are stable; renaming requires regenerating wailsjs/ bindings
// AND matching the UpdateBanner.tsx imports.

// GetAppVersion returns the embedded build version (ldflags-injected
// by the Makefile — see version.go).
func (a *App) GetAppVersion() string { return version }

// GetUpdateStatus returns the current updater status snapshot.
func (a *App) GetUpdateStatus() updater.Status {
	if a.updater == nil {
		return updater.Status{
			CurrentVersion: version,
			State:          updater.StateError,
			Error:          "updater not initialized",
		}
	}
	return a.updater.GetStatus()
}

// CheckForUpdate triggers an immediate GitHub-Releases check. Returns
// the fresh status; the `updater_status` event is also emitted.
func (a *App) CheckForUpdate() (updater.Status, error) {
	if a.updater == nil {
		return updater.Status{State: updater.StateError}, fmt.Errorf("updater not initialized")
	}
	return a.updater.Check(a.ctx)
}

// DownloadUpdate downloads + SHA256-verifies the matched asset. The
// result is the absolute path of the verified file.
func (a *App) DownloadUpdate() (string, error) {
	if a.updater == nil {
		return "", fmt.Errorf("updater not initialized")
	}
	return a.updater.Download(a.ctx)
}

// ApplyUpdate opens the downloaded zip in Finder so the user can
// drag-replace the running app (Phase-2 fast / always-works path).
// Use InstallAndRestart for an in-place auto-replace.
func (a *App) ApplyUpdate() error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}
	return a.updater.Apply()
}

// InstallAndRestart is the one-click auto-install path.
// It downloads + SHA256-verifies an available update when necessary,
// stops the bot only after the archive is ready, launches the platform
// update helper, publishes the restarting state, then exits so the helper
// can replace the running bundle and relaunch ClashGO.
func (a *App) InstallAndRestart() error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}

	// True one-click path: if the update is only "available", download and
	// SHA256-verify it first. Previous builds jumped straight to ApplyAuto(),
	// which correctly rejected the request with "download not ready".
	st := a.updater.GetStatus()
	if st.State != updater.StateReady {
		if !st.Available {
			return fmt.Errorf("no update is ready or available")
		}
		log.Info().
			Str("version", st.LatestVersion).
			Msg("InstallAndRestart: downloading update before installation")
		if _, err := a.updater.Download(a.ctx); err != nil {
			return fmt.Errorf("download update before install: %w", err)
		}
	}

	// Stop the bot only after the archive has been fully downloaded and
	// verified, so normal automation is not interrupted during the download.
	if a.IsRunning() {
		log.Info().Msg("InstallAndRestart: stopping bot to drain ADB before exit")
		_ = a.StopBot()
	}
	a.saveStats()

	// ApplyAuto requires StateReady and a SHA256-backed manifest.
	started, err := a.updater.ApplyAuto()
	if err != nil || !started {
		log.Warn().Err(err).Msg("InstallAndRestart: auto helper unavailable, falling back to manual reveal")
		if fallbackErr := a.updater.Apply(); fallbackErr != nil {
			if err != nil {
				return fmt.Errorf("automatic install failed: %v; manual fallback failed: %w", err, fallbackErr)
			}
			return fallbackErr
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("automatic install helper did not start")
	}

	a.updater.SetState(updater.StateRestarting)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "updater_status", a.updater.GetStatus())
	}

	go func() {
		time.Sleep(1200 * time.Millisecond)
		log.Info().Msg("InstallAndRestart: helper detached, exiting for Windows bundle swap")
		os.Exit(0)
	}()

	return nil
}

// SkipCurrentVersion marks the current latest version as "skip this".
// Persisted to ~/Library/Application Support/ClashGO/skip_version.txt.
func (a *App) SkipCurrentVersion() error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}
	st := a.updater.GetStatus()
	if st.LatestVersion == "" {
		return fmt.Errorf("no version available to skip")
	}
	a.updater.SetSkipVersion(st.LatestVersion)
	return nil
}

// ClearSkippedVersion is the inverse of SkipCurrentVersion.
func (a *App) ClearSkippedVersion() error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}
	a.updater.SetSkipVersion("")
	return nil
}
