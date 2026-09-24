package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gocv.io/x/gocv"

	"github.com/Ducky705/ClashGO/internal/adb"
	"github.com/Ducky705/ClashGO/internal/attack"
	"github.com/Ducky705/ClashGO/internal/config"
	"github.com/Ducky705/ClashGO/internal/game"
	"github.com/Ducky705/ClashGO/internal/paths"
	"github.com/Ducky705/ClashGO/internal/vision"
	"github.com/Ducky705/ClashGO/pkg/strategy"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Bot struct {
	client     *adb.Client
	cal        *game.Calibration
	classifier *game.Classifier
	navigator  *game.Navigator
	graph      *game.StateGraph
	templates  *game.TemplateStore
	recognizer     *game.Recognizer
	resourceReader *game.VillageResourceReader
	cfg            *config.BotConfig

	classify func(gocv.Mat) (game.GameState, int)

	attackExec *attack.Executor

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	logger zerolog.Logger

	attackCount atomic.Int32
	skipsCount  atomic.Int32
	totalGold   atomic.Int64
	totalElixir atomic.Int64
	totalDE     atomic.Int64
	totalStars  atomic.Int32
	stars0      atomic.Int32
	stars1      atomic.Int32
	stars2      atomic.Int32
	stars3      atomic.Int32
	seqRunning        atomic.Bool
	zoomedOut         atomic.Bool
	recoveryInFlight  atomic.Bool
	restartInFlight   atomic.Bool
	captureHeartbeat  atomic.Int64
	runtimeState      atomic.Int32
	runtimeStateSince atomic.Int64
	runtimeProgress   atomic.Int64
	runtimePhase      atomic.Int32
	runtimePhaseSince atomic.Int64
	armyWaitUntil     atomic.Int64
	villageAction     atomic.Int32
	recoveryAttempts  atomic.Int32
	recoverySuccesses atomic.Int32
	blueStacksRestarts atomic.Int32

	chestDismissInFlight  atomic.Bool
	rewardDismissInFlight atomic.Bool
	splashDismissInFlight atomic.Bool
	connLostDismissInFlight atomic.Bool
	donationInFlight        atomic.Bool
	// automationTaskInFlight is the single global village-task lease. Features
	// may all be enabled, but only one automation task can own the UI at once.
	// Safety/recovery handlers remain outside this lease so they can interrupt.
	automationTaskInFlight   atomic.Bool
	automationTaskMu         sync.RWMutex
	automationTaskName       string
	donationChecks          atomic.Int32
	donationsSent           atomic.Int32
	lastDonationUnix        atomic.Int64
	donationNextCheck       atomic.Int64
	trainingItemsPending    atomic.Int32
	trainingHousingPending  atomic.Int32
	trainingPlanUncertain   atomic.Bool
	armyRepairAttempts      atomic.Int32
	armyRepairSuccesses     atomic.Int32
	statusMu                sync.RWMutex
	trainingPending         []attack.TrainingPlanItem
	villageReason           string
	villageNextAt           time.Time
	lastDonationResult      string
	lastArmyCampGuardLog    time.Time
	lastDonationScan        time.Time
	startedAt             time.Time
	lastAction            time.Time
	lastSequenceStart     time.Time
	lastNav               time.Time
	lastCapture           time.Time
	lastIdlePan           time.Time
	lastVisionLog         time.Time
	lastResourceScan      time.Time
	// lastAttackEnd is stamped when a battle fully returns home; the
	// inter-attack cooldown (cfg.Attack.MinSecondsBetweenAttacks) is
	// measured from it. Written by the attack goroutine only.
	lastAttackEnd time.Time
	stuckTimeout  time.Duration
	cpuSampler    *cpuSampler

	dukePicksFile *os.File
	// armySlot is the 1-based saved-army recipe the strategy wants armed
	// before attacking (strategy YAML army_slot; default 1). Loaded once
	// at construction so clickSequence can select it before the strategy
	// phases run.
	armySlot int

	historyCache []AttackReport

	OnStatsUpdate func()
}

// NewBot builds a fully-booted Bot using a background context (no
// external cancellation). CLI and tests use this; the Wails app uses
// NewBotWithContext so a Stop click can abort a boot in progress.
func NewBot(cfg *config.BotConfig) (*Bot, error) {
	return NewBotWithContext(context.Background(), cfg)
}

// NewBotWithContext boots the bot under the caller's context. The
// context is threaded through the boot orchestrator AND becomes the
// parent of the bot's runtime context, so cancelling it (the app's
// Stop click) aborts an in-progress boot and stops a running bot.
//
// On any error the freshly-constructed adb.Client is closed — a
// failed boot must not leak a half-open transport (visible as a
// lingering localhost:5555 ghost that breaks the next Start).
func NewBotWithContext(bootCtx context.Context, cfg *config.BotConfig) (b *Bot, err error) {
	zl := &adbLogAdapter{log: log.Logger}

	client := adb.NewClient(
		adb.WithHost(cfg.Device.ADBHost),
		adb.WithPort(cfg.Device.ADBPort),
		adb.WithLogger(zl),
		adb.WithTimeout(30*time.Second),
		adb.WithBlueStacksInstance(cfg.Device.BlueStacksInstance),
		adb.WithZoomKeys(cfg.Device.ZoomOutKey, cfg.Device.ZoomInKey),
		adb.WithJitterTaps(cfg.Debug.JitterTaps),
		adb.WithJitterDelays(cfg.Debug.JitterDelays),
		adb.WithMaxJitterPixels(cfg.Debug.MaxJitterPixels),
		adb.WithJitterFraction(cfg.Debug.JitterFraction),
	)
	client.DeviceID = cfg.Device.DeviceID

	// If any step below fails before the client is handed to the Bot,
	// release the transport so a subsequent StartBot starts clean.
	defer func() {
		if err != nil && b == nil {
			_ = client.Close()
		}
	}()

	log.Info().Msg("initializing bot startup sequence...")

	bootCfg := NewBootConfigFromBotConfig(cfg)
	if devFastFail() {
		bootCfg = bootCfg.WithDevFastFail()
	}
	orchestrator := NewBootOrchestrator(bootCfg, client, log.Logger)
	bctx, err := orchestrator.Boot(bootCtx)
	if err != nil {

		wrapped := fmt.Errorf("%s: %w", orchestrator.Report().Summary(), err)
		log.Error().Err(wrapped).
			Str("suggested_action", orchestrator.Report().Snapshot().SuggestedAction).
			Str("steps", orchestrator.Report().JoinedStepSummary(200)).
			Msg("boot orchestrator failed; structured report at logs/last_boot_report.json")
		return nil, wrapped
	}

	log.Info().
		Int("screen_w", bctx.ScreenW).
		Int("screen_h", bctx.ScreenH).
		Str("recovery", strings.Join(bctx.RecoveryUsed, ",")).
		Dur("boot_duration", bctx.BootDuration).
		Msg("boot complete; calibrating...")

	w, h := bctx.ScreenW, bctx.ScreenH
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("boot returned invalid screen size %dx%d; cannot calibrate", w, h)
	}
	cal := &game.Calibration{
		PhysicalW:  w,
		PhysicalH:  h,
		ScaleX:     float64(w) / float64(game.RefWidth),
		ScaleY:     float64(h) / float64(game.RefHeight),
		MidOffsetY: (h - game.RefHeight) / 2,
		BottomOffY: h - game.RefHeight,
		Verified:   true,
	}

	packageName := cfg.Device.PackageName
	if packageName == "" {
		packageName = "com.supercell.clashofclans"
	}
	if cfg.Device.RestartOnStartup {
		log.Info().Str("package", packageName).Msg("ensuring clean state by restarting game...")
		if err := client.ForceStop(packageName); err != nil {
			log.Warn().Err(err).Msg("failed to force stop game during startup")
		}
		client.JitteredSleep(2 * time.Second)

		log.Info().Str("package", packageName).Msg("launching game...")
		if err := client.StartApp(packageName); err != nil {
			return nil, fmt.Errorf("failed to start game: %w", err)
		}
		log.Info().Msg("waiting for game to settle...")
		client.JitteredSleep(bootCfg.WaitForGameSettle)
	} else {
		log.Info().Msg("skipping game restart on startup (restart_on_startup=false)")
	}

	if profile, perr := LoadBootProfile(paths.ResolveConfig("boot_profile.json")); perr == nil {
		profile.AddSample(BootProfileSample{
			StartedAt: bctx.Report.StartedAt,
			Duration:  bctx.BootDuration.Milliseconds(),
			Outcome:   "ok",
		})
		if perr := profile.Save(paths.ResolveConfig("boot_profile.json")); perr != nil {
			log.Debug().Err(perr).Msg("failed to persist boot profile")
		}
	}

	graph := game.NewStateGraph()
	graph.AddNode(game.StateMainVillage)

	startedWall := time.Now()

	dukePicksDir := paths.ResolveConfig("output/duke_picks")
	if err := os.MkdirAll(dukePicksDir, 0o755); err != nil {
		log.Warn().Err(err).Str("dir", dukePicksDir).Msg("failed to create duke_picks dir")
	}
	dukePicksPath := filepath.Join(dukePicksDir, startedWall.Format("20060102_150405")+".ndjson")
	dukePicksFile, dpErr := os.OpenFile(dukePicksPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if dpErr != nil {
		log.Warn().Err(dpErr).Str("path", dukePicksPath).Msg("failed to open duke_picks NDJSON; will skip")
	}

	attackExec := attack.NewExecutor(client, cal, &cfg.Attack, log.Logger)

	var templates *game.TemplateStore
	templates, err = game.NewTemplateStore(paths.Resolve("templates"))
	if err != nil {
		// NEVER leave templates nil: every consumer (classifier,
		// navigator, loot recognizer, attack executor) dereferences it
		// without a nil check, and the first such dereference panics
		// (observed live: SIGSEGV in LootRecognizer.prepareDigitTemplates
		// killing the bot the moment it entered an attack). The empty
		// store degrades every lookup to "not found" so the bot keeps
		// running on its color/pinpoint heuristics.
		log.Warn().Err(err).Msg("template store init failed; continuing WITHOUT templates (color/pinpoint fallbacks only)")
		templates = game.NewEmptyTemplateStore()
	}

	if templates != nil {
		templates.LoadTemplates()
		log.Info().Int("templates", templates.Count()).Msg("templates loaded")
	}

	recognizer := game.NewRecognizer()
	// Derive the bot's runtime context from the caller's context so a
	// Stop that cancels the boot also tears down a successfully-booted
	// bot without waiting for the App-level bot.Cancel() to be called.
	ctx, cancel := context.WithCancel(bootCtx)

	resourceReader := game.NewVillageResourceReader(cal, templates, log.Logger)

	b = &Bot{
		client:            client,
		cal:               cal,
		graph:             graph,
		templates:         templates,
		recognizer:        recognizer,
		resourceReader:    resourceReader,
		cfg:               cfg,
		attackExec:        attackExec,
		ctx:               ctx,
		cancel:            cancel,
		done:              make(chan struct{}),
		logger:            log.With().Str("bot", "orchestrator").Logger(),
		startedAt:         startedWall,
		lastAction:        time.Now(),
		lastSequenceStart: time.Now(),
		lastIdlePan:       time.Now(),
		stuckTimeout:      35 * time.Second,
		cpuSampler:        newCPUSampler(),
		dukePicksFile:     dukePicksFile,
	}

	// Restore only a recent, profile-matching plan. This keeps the beginner UI
	// coherent after an EXE restart without ever acting on stale army data.
	if profile, ok := cfg.Attack.Farm.ActiveProfile(); ok {
		if plan, err := attack.ReadTrainingPlan(); err == nil &&
			!plan.Ready &&
			!plan.Stale(time.Now(), 15*time.Minute) &&
			attack.ValidateTrainingPlan(plan, profile) == nil {
			actionable := attack.ActionableTrainingItems(plan)
			b.trainingItemsPending.Store(int32(len(actionable)))
			b.trainingHousingPending.Store(int32(plan.TotalHousing))
			b.trainingPlanUncertain.Store(plan.HasUncertain)
			b.statusMu.Lock()
			b.trainingPending = append([]attack.TrainingPlanItem(nil), plan.Items...)
			b.statusMu.Unlock()
			log.Info().
				Int("items", len(plan.Items)).
				Int("actionable_items", len(actionable)).
				Msg("restored recent validated training plan")
		}
	}

	// Resolve the strategy's declared army slot once at boot so the
	// pre-battle click sequence can arm the right saved recipe. The
	// strategy is parsed again later for the deploy phases; this early
	// read only needs army_slot (falls back to slot 1 on any error).
	if strat, err := strategy.ParseYAML(cfg.Attack.StrategyFile); err == nil {
		b.armySlot = strat.SelectedArmySlot()
		log.Info().Int("army_slot", b.armySlot).Str("strategy", strat.Name).Msg("resolved strategy army slot")
	} else {
		b.armySlot = 1
		log.Warn().Err(err).Str("path", cfg.Attack.StrategyFile).Msg("could not pre-read strategy for army slot; defaulting to slot 1")
	}

	// Close `done` the moment the bot's runtime context is cancelled
	// (Stop click in the GUI, or the attack-cap graceful shutdown after
	// a --once CLI run) so external owners — the CLI's main() — can
	// wait on b.Done() instead of polling stats or blocking on a
	// signal that never arrives.
	go func() {
		<-ctx.Done()
		close(b.done)
	}()

	if dukePicksFile != nil {
		writePick := func(target, chosen string) {
			line := fmt.Sprintf(`{"timestamp":%q,"target_edge":%q,"chosen_edge":%q}`+"\n",
				time.Now().Format(time.RFC3339Nano), target, chosen)
			if _, err := dukePicksFile.WriteString(line); err != nil {
				log.Warn().Err(err).Msg("failed to write duke pick to NDJSON")
			}
		}
		attackExec.OnDukePick = writePick
	}

	if histData, err := os.ReadFile(paths.ResolveConfig("attack_history.json")); err == nil {
		var seeded []AttackReport
		if jsonErr := json.Unmarshal(histData, &seeded); jsonErr == nil {
			b.historyCache = seeded
		} else {
			b.logger.Warn().Err(jsonErr).Msg("failed to parse attack history, starting fresh")
		}
	}

	b.classifier = game.NewClassifier(cal, game.DefaultClassifierConfig(), b.logger)
	if b.templates != nil {
		b.classifier.SetTemplates(b.templates)
	}

	b.classify = func(mat gocv.Mat) (game.GameState, int) {
		return b.classifier.ClassifyState(mat)
	}

	b.navigator = game.NewNavigator(client, cal, graph, b.classify, b.logger)
	if b.templates != nil {
		b.navigator.SetTemplates(b.templates)
	}

	b.navigator.SetDisableChestDismissal(b.cfg.Device.DisableChestDismissal)

	b.attackExec.SetClassifier(b.classify)

	return b, nil
}

func (b *Bot) Start() error {
	if err := b.client.EnsureConnected(); err != nil {
		return fmt.Errorf("ensure connect: %w", err)
	}

	sw, sh, err := b.client.ScreenSize()
	if err != nil {
		b.logger.Warn().Err(err).Msg("could not get screen size")
	} else {
		b.logger.Info().
			Str("device", b.cfg.Device.DeviceID).
			Str("resolution", fmt.Sprintf("%dx%d", sw, sh)).
			Str("scale", fmt.Sprintf("%.3fx%.3f", b.cal.ScaleX, b.cal.ScaleY)).
			Msg("connected")
	}

	focusX, focusY := b.cal.ScaleRef(842, 345)
	b.logger.Info().Int("x", focusX).Int("y", focusY).Msg("performing initial focus click")
	b.client.Tap(focusX, focusY)
	b.client.JitteredSleep(250 * time.Millisecond)

	now := time.Now().UnixNano()
	b.captureHeartbeat.Store(now)
	b.runtimeState.Store(int32(game.StateUnknown))
	b.runtimeStateSince.Store(now)
	b.runtimeProgress.Store(now)
	b.runtimePhase.Store(int32(PhaseIdle))
	b.runtimePhaseSince.Store(now)

	go b.captureLoop()
	go b.runtimeSupervisorLoop()
	return nil
}

func (b *Bot) Stop() {
	b.cancel()
	b.client.Close()
	globalAsyncWriter.Close()
	vision.CloseTemplateCache()
	if b.resourceReader != nil {
		b.resourceReader.Close()
	}
	if b.dukePicksFile != nil {
		_ = b.dukePicksFile.Close()
	}
}

// Cancel signals the bot's context to stop. The captureLoop and any
// in-flight attack sequence see the cancellation on their next
// `b.ctx.Done()` check (sub-millisecond), which is what makes the
// Stop button feel "instant" to the user — no more taps, captures,
// state transitions, or attack progression. The full `Stop()` path
// (ADB close, async-writer drain, file flush) is intentionally kept
// out of this method so callers can split the work: `Cancel()` for
// the synchronous "stop what you're doing" signal, and `Stop()` for
// the heavier teardown that App.StopBot detaches into a goroutine.
func (b *Bot) Cancel() {
	b.cancel()
}

// Done returns a channel that closes when the bot's runtime context
// is cancelled — i.e. after a Stop, or after the attack-cap graceful
// shutdown (a --once CLI run). External owners (the CLI main) wait on
// it so they can exit once the bot finishes its session instead of
// blocking on a signal that never arrives.
func (b *Bot) Done() <-chan struct{} { return b.done }
func (b *Bot) captureLoop() {
	gc := game.NewGameContext()

	type frame struct {
		mat gocv.Mat
		err error
		dur time.Duration
	}
	frames := make(chan frame, 1)

	getCaptureInterval := func() time.Duration {
		// While an attack/search sequence is running, that goroutine already
		// performs its own fresh screenshots for state, loot and deployment.
		// Keeping the background capture loop at 150ms at the same time meant
		// BlueStacks was being hammered by two independent screencap streams.
		// On the user's Pie64 instance this can terminate/restart the emulator
		// with no Go error at all. Keep one low-rate observer alive for popup /
		// health handling, but remove the duplicate high-frequency pressure.
		if b.seqRunning.Load() {
			// The active attack/search goroutine owns screencaps while a
			// sequence is running. Keep only a very low-rate observer so
			// BlueStacks is never hit by two concurrent screencap streams.
			return 2500 * time.Millisecond
		}

		switch gc.State {
		case game.StateBattle, game.StateSearchMap, game.StateLoading:
			return 300 * time.Millisecond
		case game.StateMainVillage, game.StateArmySelection, game.StateArmyCamp:
			return 250 * time.Millisecond
		default:
			return 500 * time.Millisecond
		}
	}

	go func() {
		var lastCapture time.Time
		for {
			interval := getCaptureInterval()
			nextCapture := lastCapture.Add(interval)
			sleepTime := time.Until(nextCapture)
			if sleepTime > 0 {
				select {
				case <-b.ctx.Done():
					return
				case <-time.After(sleepTime):
				}
			}

			select {
			case <-b.ctx.Done():
				return
			default:
			}

			start := time.Now()
			screen, err := b.client.CaptureToMat()
			dur := time.Since(start)
			lastCapture = time.Now()
			b.lastCapture = lastCapture
			b.captureHeartbeat.Store(lastCapture.UnixNano())

			if err != nil || screen.Empty() || screen.Cols() < 2 || screen.Rows() < 2 {
				screen.Close()
				b.logger.Debug().Err(err).Msg("empty/degenerate capture dropped")
				continue
			}

			select {
			case frames <- frame{mat: screen, err: err, dur: dur}:
			default:
				screen.Close()
			}
		}
	}()

	for {
		select {
		case <-b.ctx.Done():
			select {
			case f := <-frames:
				if f.mat.Cols() >= 1 && f.mat.Rows() >= 1 {
					f.mat.Close()
				}
			default:
			}
			return
		case f := <-frames:
			// Panic guard: one bad frame (degenerate mat, classifier
			// edge case, cgo hiccup) must never kill the whole bot
			// process — an unattended farm would stay dead until a
			// human notices. Recover, release the frame, and keep
			// the loop alive; the stuck-watchdog handles the rest.
			func() {
				defer func() {
					if r := recover(); r != nil {
						b.logger.Error().Interface("panic", r).Msg("recovered panic in frame processing; continuing capture loop")
						if !f.mat.Empty() {
							f.mat.Close()
						}
						b.recordActivity()
					}
				}()
				b.checkStuck(gc)
				b.processFrame(gc, f.mat, f.err, f.dur)
			}()
		}
	}
}

// recordActivity marks the bot as having taken a meaningful action.
// Called after real forward progress (successful clicks/state transitions)
// so the stuck-check distinguishes "spinning" from "working".
func (b *Bot) recordActivity() {
	now := time.Now()
	b.lastAction = now
	b.runtimeProgress.Store(now.UnixNano())
}

// checkStuck enforces a global watchdog: if the capture pipeline is dead,
// or if the bot is in-progress for absurdly long, or has been sitting in
// one place doing nothing for too long, we cycle the game to recover from
// hangs / dialogs / out-of-game screens without requiring user intervention.
func (b *Bot) checkStuck(gc *game.GameContext) {
	if gc.ReadHealth().ConsecutiveFails >= 10 {
		b.logger.Error().
			Int("consecutive_fails", gc.ReadHealth().ConsecutiveFails).
			Str("state", gc.State.String()).
			Msg("capture pipeline appears unhealthy, beginning device recovery ladder...")
		b.recoverEmulator()
		b.lastSequenceStart = time.Now()
		return
	}

	// Domain-specific search/deploy/battle loops own their normal timeouts.
	// This is only a final safety ceiling for a sequence goroutine that never
	// returns at all.
	if b.seqRunning.Load() && time.Since(b.lastSequenceStart) > 15*time.Minute {
		b.logger.Warn().
			Dur("seq_time", time.Since(b.lastSequenceStart)).
			Msg("attack sequence exceeded hard safety ceiling; restarting Clash")
		b.restartGame()
		b.lastSequenceStart = time.Now()
	}
}

func (b *Bot) restartGame() {
	if !b.restartInFlight.CompareAndSwap(false, true) {
		b.logger.Debug().Msg("game restart already in progress; suppressing duplicate restart")
		return
	}
	defer b.restartInFlight.Store(false)

	pkg := b.cfg.Device.PackageName
	if pkg == "" {
		pkg = "com.supercell.clashofclans"
	}

	b.logger.Info().Str("package", pkg).Msg("restarting game...")

	if err := b.client.ForceStop(pkg); err != nil {
		b.logger.Error().Err(err).Msg("failed to force stop game")
	}

	if !b.sleepResponsive(750 * time.Millisecond) {
		return
	}

	if err := b.client.StartApp(pkg); err != nil {
		b.logger.Error().Err(err).Msg("failed to start app")
		return
	}

	// Do not blind-sleep for 15 seconds. The capture/classifier loop can
	// observe Logo/TapToContinue/News as soon as they actually appear.
	if !b.sleepResponsive(1200 * time.Millisecond) {
		return
	}

	b.zoomedOut.Store(false)
	now := time.Now()
	b.lastAction = now
	b.lastNav = now
	b.lastSequenceStart = now
	b.runtimeProgress.Store(now.UnixNano())
	b.runtimeState.Store(int32(game.StateUnknown))
	b.runtimeStateSince.Store(now.UnixNano())
}

// recoverEmulator is the mid-run escalation for a dead capture
// pipeline. restartGame only force-stops CoC — when the EMULATOR
// itself is gone (BlueStacks crashed, adb-server wedged, transport
// socket stale) that just fails silently and the bot spins at high
// CPU against a dead device forever. recoverEmulator walks a
// cheap→destructive ladder, re-probing liveness (wm size) after
// each step, and only relaunches BlueStacks as a last resort:
//
//  1. screen-size probe     — device alive? just restart the game
//  2. transport Reconnect   — stale socket after a BlueStacks blip
//  3. ResetAdbServer        — stale adb-server registration
//     (note: drops ALL adb connections on this host — logged)
//  4. EnsureBlueStacksMac   — emulator really gone; relaunch at the
//     configured resolution, then poll up to 2 min for adb
func (b *Bot) recoverEmulator() {
	if !b.recoveryInFlight.CompareAndSwap(false, true) {
		b.logger.Debug().Msg("device recovery already in progress; suppressing duplicate recovery")
		return
	}
	defer b.recoveryInFlight.Store(false)

	b.recoveryAttempts.Add(1)
	b.logger.Warn().Msg("capture pipeline dead; beginning device recovery ladder")

	deviceOK := func() bool {
		_, _, err := b.client.ScreenSize()
		return err == nil
	}

	if deviceOK() {
		b.logger.Info().Msg("device still responsive; restarting game only")
		b.restartGame()
		b.recoverySuccesses.Add(1)
		return
	}

	b.logger.Warn().Msg("device unresponsive to wm size; reconnecting ADB transport")
	if err := b.client.Reconnect(); err != nil {
		b.logger.Warn().Err(err).Msg("transport reconnect failed")
	}
	if deviceOK() {
		b.restartGame()
		b.recoverySuccesses.Add(1)
		return
	}

	b.logger.Warn().Msg("device still unreachable; resetting adb server (drops ALL adb connections on this host)")
	if err := b.client.ResetAdbServer(); err != nil {
		b.logger.Warn().Err(err).Msg("adb server reset failed")
	}
	if !b.sleepResponsive(2 * time.Second) {
		return
	}
	_ = b.client.Reconnect()
	if deviceOK() {
		b.restartGame()
		b.recoverySuccesses.Add(1)
		return
	}

	b.logger.Error().Msg("device unreachable after transport + adb-server recovery; relaunching BlueStacks")
	b.blueStacksRestarts.Add(1)
	if err := b.client.EnsureBlueStacks(b.cfg.Device.Width, b.cfg.Device.Height, b.cfg.Device.DPI); err != nil {
		b.logger.Error().Err(err).Msg("BlueStacks relaunch failed; will retry on next stuck check")
	}
	// Give the freshly-relaunched emulator up to 2 minutes to expose
	// its adb daemon (cold VM boot can take 45-70s on this hardware).
	recovered := false
	for i := 0; i < 60; i++ {
		if deviceOK() {
			recovered = true
			break
		}
		if !b.sleepResponsive(2 * time.Second) {
			b.logger.Info().Msg("BlueStacks recovery wait cancelled")
			return
		}
	}
	if !recovered {
		b.logger.Error().Msg("device remained unreachable after BlueStacks recovery window; deferring until next watchdog cycle")
		return
	}
	b.restartGame()
	b.recoverySuccesses.Add(1)
}

// locateRewardPopup detects the seasonal/event "Pick a Reward!" modal.
// The popup has a wide saturated red banner across the upper-middle of the
// 860x732 reference frame. We deliberately use a region/color signature
// rather than English text so it keeps working if the UI language changes.
// The returned point is the center of the right-most card (gold/resource in
// current events), which is always a valid selectable reward.
func (b *Bot) locateRewardPopup(screen gocv.Mat) (int, int, bool) {
	if screen.Empty() {
		return 0, 0, false
	}

	x0, y0 := b.cal.ScaleRef(185, 105)
	x1, y1 := b.cal.ScaleRef(675, 185)
	if x0 < 0 { x0 = 0 }
	if y0 < 0 { y0 = 0 }
	if x1 > screen.Cols() { x1 = screen.Cols() }
	if y1 > screen.Rows() { y1 = screen.Rows() }
	if x1-x0 < 10 || y1-y0 < 10 {
		return 0, 0, false
	}

	roi := screen.Region(image.Rect(x0, y0, x1, y1))
	defer roi.Close()
	hsv := vision.GetMat(roi.Rows(), roi.Cols(), gocv.MatTypeCV8UC3)
	defer vision.PutMat(hsv)
	gocv.CvtColor(roi, &hsv, gocv.ColorBGRToHSV)

	m1 := vision.GetMat(roi.Rows(), roi.Cols(), gocv.MatTypeCV8UC1)
	defer vision.PutMat(m1)
	m2 := vision.GetMat(roi.Rows(), roi.Cols(), gocv.MatTypeCV8UC1)
	defer vision.PutMat(m2)
	mask := vision.GetMat(roi.Rows(), roi.Cols(), gocv.MatTypeCV8UC1)
	defer vision.PutMat(mask)

	gocv.InRangeWithScalar(hsv, gocv.NewScalar(0, 120, 110, 0), gocv.NewScalar(12, 255, 255, 0), &m1)
	gocv.InRangeWithScalar(hsv, gocv.NewScalar(168, 120, 110, 0), gocv.NewScalar(180, 255, 255, 0), &m2)
	gocv.BitwiseOr(m1, m2, &mask)

	redPixels := gocv.CountNonZero(mask)
	total := roi.Rows() * roi.Cols()
	if total <= 0 || float64(redPixels)/float64(total) < 0.12 {
		return 0, 0, false
	}

	// Current modal card centers in the 860x732 reference layout are roughly
	// x=220/455/705, y=366. Pick the right-most one to avoid event-troop
	// inventory constraints and keep reward handling deterministic.
	x, y := b.cal.ScaleRef(705, 366)
	return x, y, true
}

func (b *Bot) processFrame(gc *game.GameContext, screen gocv.Mat, err error, captureMs time.Duration) {
	if err != nil {
		gc.RecordCaptureError()
		b.logger.Debug().Err(err).Msg("capture failed")
		screen.Close()
		return
	}
	if screen.Empty() || screen.Cols() < 2 || screen.Rows() < 2 {
		screen.Close()
		return
	}

	// The effective wm size reported by Android is not always the same as the
	// actual screencap matrix dimensions on BlueStacks. Vision and tap scaling
	// must follow the pixels we are really processing, not a metadata guess.
	// Recalibrate from the live frame whenever they differ. All consumers keep
	// a pointer to b.cal, so classifier/navigator/attack executor immediately
	// use the corrected scale.
	if screen.Cols() != b.cal.PhysicalW || screen.Rows() != b.cal.PhysicalH {
		oldW, oldH := b.cal.PhysicalW, b.cal.PhysicalH
		b.cal.PhysicalW = screen.Cols()
		b.cal.PhysicalH = screen.Rows()
		b.cal.ScaleX = float64(screen.Cols()) / float64(game.RefWidth)
		b.cal.ScaleY = float64(screen.Rows()) / float64(game.RefHeight)
		b.cal.MidOffsetY = (screen.Rows() - game.RefHeight) / 2
		b.cal.BottomOffY = screen.Rows() - game.RefHeight
		b.cal.Verified = true

		b.logger.Warn().
			Int("reported_w", oldW).
			Int("reported_h", oldH).
			Int("capture_w", screen.Cols()).
			Int("capture_h", screen.Rows()).
			Str("scale", fmt.Sprintf("%.3fx%.3f", b.cal.ScaleX, b.cal.ScaleY)).
			Msg("live capture size differed from reported display size; recalibrated to actual frame")
	}

	state, score := b.classify(screen)

	// Keep the console useful without flooding Wails/React at the faster
	// capture cadence. Log immediately on state changes and at most roughly
	// once per 750ms while a state remains stable.
	if state != gc.State || time.Since(b.lastVisionLog) >= 750*time.Millisecond || time.Since(b.startedAt) < 3*time.Second {
		b.lastVisionLog = time.Now()
		b.logger.Info().
			Str("vision_state", state.String()).
			Int("score", score).
			Int("capture_w", screen.Cols()).
			Int("capture_h", screen.Rows()).
			Msg(fmt.Sprintf("vision frame classified: state=%s score=%d capture=%dx%d", state.String(), score, screen.Cols(), screen.Rows()))
	}

	gc.UpdateScreen(screen, captureMs)

	// Seasonal/event battles can interrupt combat with a "Pick a Reward!"
	// overlay. It is not a normal game state and used to leave the attack
	// sequence waiting behind the modal. Detect the large red reward banner
	// directly from the live frame and pick the right-most reward card.
	// Reward cards only exist during an active battle. Never run this detector
	// on MainVillage/ArmySelection: the broad red-banner signature can match
	// normal home/menu artwork and was stealing focus from Attack -> Find Match.
	if state == game.StateBattle || state == game.StateSearchMap {
		if rewardX, rewardY, ok := b.locateRewardPopup(screen); ok {
			if b.rewardDismissInFlight.CompareAndSwap(false, true) {
				b.logger.Info().Int("x", rewardX).Int("y", rewardY).Msg("Pick a Reward popup detected during battle; selecting reward")
				go func(x, y int) {
					defer b.rewardDismissInFlight.Store(false)
					time.Sleep(220 * time.Millisecond)
					if err := b.client.TapFast(x, y, 0.7); err != nil {
						b.logger.Warn().Err(err).Msg("reward selection tap failed; will retry")
						return
					}
					b.recordActivity()
					b.logger.Info().Msg("reward selected; resuming battle")
				}(rewardX, rewardY)
			}
			return
		}
	}

	if !b.zoomedOut.Load() {
		// Only zoom when we have evidence that this is actually the home
		// village. The old fallback used a single orange pixel / loose
		// template hit, which can also occur on the troop bar and battle HUD.
		// That caused a live battle/search screen to be logged as "village
		// detected" and consumed the frame before the attack logic could run.
		isVillage := state == game.StateMainVillage ||
			((state == game.StateUnknown || state == game.StateArmyCamp) && b.findAttackButton(screen, 0.30))

		if isVillage {
			if b.zoomedOut.CompareAndSwap(false, true) {
				// Windows/BlueStacks: the native sendevent pinch path has proven
				// unstable on some installations and can terminate the Wails
				// process immediately after "performing ... zoom out". The bot
				// already forces the reference 860x732 display override, so the
				// startup zoom is not required for coordinate calibration.
				// Mark initialization complete and continue without injecting a
				// multi-touch gesture; attack-button detection will decide whether
				// the village is usable.
				b.logger.Info().Msg("village detected; skipping native startup zoom on Windows-safe path")
				b.recordActivity()
				return
			}
		}
	}

	if state != gc.State && state != game.StateUnknown && state != game.StateLoading {
		b.recordActivity()
	}

	if gc.ConfirmState(state) {
		now := time.Now()
		b.observeRuntimeState(state, now)
		gc.UpdateState(state, now)

		b.logger.Debug().
			Str("state", state.String()).
			Int("score", score).
			Msg("state detected")
	}

	if state == game.StateChestReward {
		if b.cfg.Device.DisableChestDismissal {

			b.logger.Debug().Msg("chest detected but dismissal disabled by config; deferring to stuck-watchdog")
			return
		}
		if b.chestDismissInFlight.CompareAndSwap(false, true) {
			b.logger.Info().Msg("chest reward screen detected; dispatching dismiss goroutine")
			go func() {
				defer b.chestDismissInFlight.Store(false)
				start := time.Now()
				if err := b.navigator.DismissChestReward(); err != nil {
					b.logger.Warn().Err(err).
						Dur("elapsed", time.Since(start)).
						Msg("chest dismiss failed; will retry on next detection if still on screen")
				} else {
					b.logger.Info().
						Dur("elapsed", time.Since(start)).
						Msg("chest dismissed")
				}
			}()
		}
		return
	}

	// Post-boot splash screens. The game shows a short chain after every
	// relaunch: the "ТАР!" tap-to-continue / collect splash, then the CoC
	// castle logo (which sits static 1-3 min while the session connects),
	// then an optional news/announcement splash with a Continue button
	// before the village appears. The bot must tap each prompt to advance;
	// previously these read as Battle/Unknown and the stuck-watchdog
	// force-restarted the game in an endless loop.
	if state == game.StateTapToContinue || state == game.StateNewsSplash {
		if b.splashDismissInFlight.CompareAndSwap(false, true) {
			b.logger.Info().Str("state", state.String()).Msg("boot splash detected; dispatching dismiss tap")
			go func(st game.GameState) {
				defer b.splashDismissInFlight.Store(false)
				if !b.sleepResponsive(250 * time.Millisecond) { return }

				var x, y int
				switch st {
				case game.StateTapToContinue:
					// Tap the "ТАР!" prompt text (ref 450,195). Verified live:
					// this dismisses the collect splash into the game.
					x, y = b.cal.ScaleRef(450, 195)
				case game.StateNewsSplash:
					// Tap the green Continue button (ref 403,535).
					x, y = b.cal.ScaleRef(403, 535)
				}
				if err := b.client.TapRandomized(x, y); err != nil {
					b.logger.Warn().Err(err).Msg("boot splash dismiss tap failed; will retry on next detection")
					return
				}
				// Deliberately NO recordActivity here: the adb tap reports
				// success even when the splash did not actually dismiss, so
				// recording activity would keep resetting lastAction and
				// the stuck-watchdog (including the boot-splash grace in
				// checkStuck) could never fire on a genuinely stuck splash
				// variant. The real progress signal is the resulting state
				// transition out of the splash, which processFrame records
				// via its own state-change activity call.
				b.logger.Info().Str("state", st.String()).Msg("boot splash dismissed")
			}(state)
		}
		return
	}

	if b.seqRunning.Load() {
		return
	}

	// Connection-lost dialog. CoC shows this whenever the game's own
	// server link drops (emulator network blip, server restart); the
	// classifier used to misread it as StateBattleEnd — the dialog's
	// RETURN HOME button satisfied that rule's template-only match —
	// and the bot tapped result-screen coordinates forever (observed
	// live: 18:26 boot → 18:28 ReturnHome fallback → 18:33 emergency
	// restart loop). Tapping TRY AGAIN (ref 300,478) makes the game
	// reconnect in place. Like the splash dismissal below, activity is
	// deliberately NOT recorded: the adb tap reports success even when
	// the dialog survives, and the stuck-watchdog must still fire if
	// reconnect keeps failing.
	if state == game.StateConnectionLost {
		if b.connLostDismissInFlight.CompareAndSwap(false, true) {
			b.logger.Warn().Msg("connection lost dialog detected; tapping TRY AGAIN...")
			go func() {
				defer b.connLostDismissInFlight.Store(false)
				if !b.sleepResponsive(250 * time.Millisecond) { return }
				x, y := b.cal.ScaleRef(300, 478)
				if err := b.client.TapRandomized(x, y); err != nil {
					b.logger.Warn().Err(err).Msg("connection-lost dismiss tap failed; will retry on next detection")
					return
				}
				b.logger.Info().Msg("connection-lost TRY AGAIN tapped; waiting for reconnect")
			}()
		}
		return
	}

	// Quit-confirm dialog ("Do you want to quit the game?", Cancel /
	// Okay). Defense in depth for the ArmyCamp misclassification guard
	// above: if a Back press still lands on the real main village, this
	// dialog appears and must be dismissed with CANCEL — otherwise the
	// bot sits on it for the boot-splash grace (5 min) then force-
	// restarts. Same no-recordActivity reasoning as the splash handler.
	if state == game.StateConfirmExit {
		if b.connLostDismissInFlight.CompareAndSwap(false, true) {
			b.logger.Warn().Msg("quit-confirm dialog detected; tapping Cancel...")
			go func() {
				defer b.connLostDismissInFlight.Store(false)
				if !b.sleepResponsive(200 * time.Millisecond) { return }
				x, y := b.cal.ScaleRef(279, 429)
				if err := b.client.TapRandomized(x, y); err != nil {
					b.logger.Warn().Err(err).Msg("quit-confirm cancel tap failed; will retry on next detection")
					return
				}
				b.logger.Info().Msg("quit-confirm Cancel tapped")
			}()
		}
		return
	}

	if gc.State == game.StateBattleEnd || gc.State == game.StateReturnHome {
		b.logger.Info().Str("state", gc.State.String()).Msg("detected terminal state without active sequence, returning home...")
		go b.attackExec.ReturnHome()
		b.recordActivity()
		return
	}

	// Central village automation coordinator. One decision per frame means
	// donation, resource tracking, army waiting and matchmaking can no longer
	// race each other through separate ad-hoc branches.
	if b.zoomedOut.Load() && (state == game.StateMainVillage || state == game.StateUnknown || gc.State == game.StateMainVillage || gc.State == game.StateUnknown) {
		attackVisible := b.findAttackButton(screen, 0.30)
		villageVerified := state == game.StateMainVillage || gc.State == game.StateMainVillage || attackVisible
		now := time.Now()
		armyUntil := time.Time{}
		if n := b.armyWaitUntil.Load(); n > 0 {
			armyUntil = time.Unix(0, n)
		}

		decision := decideVillageAction(VillageDecisionInput{
			Now:                 now,
			VillageVerified:     villageVerified,
			SequenceRunning:     b.seqRunning.Load() || b.automationTaskInFlight.Load(),
			DonationInFlight:    b.donationInFlight.Load(),
			DonationEnabled:     b.cfg.Automation.Preferences.AutoDonate,
			LastDonationScan:    b.lastDonationScan,
			DonationNextCheck: func() time.Time {
				if n := b.donationNextCheck.Load(); n > 0 { return time.Unix(0, n) }
				return time.Time{}
			}(),
			DonationInterval:    90 * time.Second,
			ResourceEnabled:     b.cfg.Automation.AutoResourceTracking,
			LastResourceScan:    b.lastResourceScan,
			ResourceInterval:    15 * time.Second,
			ArmyWaitUntil:       armyUntil,
			AttackEnabled:       b.cfg.Attack.Enabled,
			AttackCapReached:    int(b.attackCount.Load()) >= b.cfg.Attack.MaxAttackPerSession,
			AttackNotBefore: func() time.Time {
				if b.cfg.Attack.MinSecondsBetweenAttacks <= 0 || b.lastAttackEnd.IsZero() { return time.Time{} }
				return b.lastAttackEnd.Add(time.Duration(b.cfg.Attack.MinSecondsBetweenAttacks) * time.Second)
			}(),
			AttackButtonVisible: attackVisible,
		})
		b.villageAction.Store(int32(decision.Action))
		b.statusMu.Lock()
		b.villageReason = decision.Reason
		b.villageNextAt = decision.NextAt
		b.statusMu.Unlock()

		switch decision.Action {
		case VillageActionDonate:
			if b.maybeStartDonationCycle(screen) {
				return
			}
		case VillageActionScanResources:
			if b.tryBeginAutomationTask("resource scan") {
				b.maybeScanVillageResources(screen)
				b.endAutomationTask("resource scan")
			}
			return
		case VillageActionWaitArmy:
			if time.Since(b.lastArmyCampGuardLog) > 10*time.Second {
				b.lastArmyCampGuardLog = time.Now()
				b.logger.Info().
					Time("retry_after", armyUntil).
					Msg("automation brain: army not ready yet; using village time for safe housekeeping")
			}
			return
		case VillageActionCooldown:
			return
		case VillageActionSessionComplete:
			return
		case VillageActionAttack:
			if !b.tryBeginAutomationTask("attack") {
				return
			}
			b.logger.Info().
				Str("reason", decision.Reason).
				Msg("automation brain: starting matchmaking")
			b.lastSequenceStart = time.Now()
			go func() {
				defer b.endAutomationTask("attack")
				b.executeAttackSequence(gc)
			}()
			return
		case VillageActionHold:
			return
		}
	}

	// Idle humanization: while confirmed on the main village with no
	// attack button in sight (army still training / waiting), drift the
	// camera the way a waiting player would. Throttled so the wander
	// never overlaps an attack sequence, and deliberately NOT
	// recordActivity — a genuinely stuck bot must still trip the
	// stuck-watchdog and cycle the game.
	//
	// Dispatched in a goroutine (mirroring the chest-dismiss pattern)
	// so the ~3s sendevent gesture can't freeze the capture loop's UI
	// frame stream; the seqRunning re-check keeps it from colliding
	// with a freshly-started attack sequence. The pan swipes the map
	// center, never the fixed HUD chrome, so a mid-pan capture still
	// sees the attack button.
	if gc.State == game.StateMainVillage && time.Since(b.lastIdlePan) > 18*time.Second {
		b.lastIdlePan = time.Now()
		b.logger.Debug().Msg("idle in village, wandering camera")
		go func() {
			if b.seqRunning.Load() {
				return
			}
			b.navigator.IdlePan()
		}()
	}

	if gc.State == game.StateArmyCamp && time.Since(b.lastNav) > 3*time.Second {
		// Guard against a misclassification, not a real camp: a dim or
		// zoomed village frame can pass the ArmyCamp rule's single loose
		// brown pixel check. Pressing Back on the actual main village opens
		// CoC's "Do you want to quit the game?" confirm dialog, which no
		// rule detects — the bot then sat on that dialog for the full
		// boot-splash grace (5 min) and force-restarted, every cycle
		// (observed live 11:32–11:43). A frame that still shows the real
		// Attack! button IS the main village; skip the Back press.
		if b.findAttackButton(screen, 0.30) {
			// Throttle the log: the guard can fire every frame while the
			// misclassification persists, which would spam 10 lines/sec.
			if time.Since(b.lastArmyCampGuardLog) > 10*time.Second {
				b.lastArmyCampGuardLog = time.Now()
				b.logger.Info().Msg("ArmyCamp state but attack button visible; treating as main village (misclassification guard)")
			}
			b.recordActivity()
			return
		}
		b.lastNav = time.Now()
		b.logger.Info().Msg("in ArmyCamp, returning to main village...")
		go b.navigator.NavigateToMainVillage(gc)
		return
	}
}

// tryBeginAutomationTask acquires the one-at-a-time automation lease.
// Enabling several capabilities means they are eligible for scheduling; it
// never means ClashGO may click through several flows simultaneously.
func (b *Bot) tryBeginAutomationTask(name string) bool {
	if !b.automationTaskInFlight.CompareAndSwap(false, true) {
		return false
	}
	b.automationTaskMu.Lock()
	b.automationTaskName = name
	b.automationTaskMu.Unlock()
	return true
}

func (b *Bot) endAutomationTask(name string) {
	b.automationTaskMu.Lock()
	if b.automationTaskName == name {
		b.automationTaskName = ""
	}
	b.automationTaskMu.Unlock()
	b.automationTaskInFlight.Store(false)
}

func (b *Bot) currentAutomationTask() string {
	b.automationTaskMu.RLock()
	defer b.automationTaskMu.RUnlock()
	return b.automationTaskName
}

func (b *Bot) findAttackButton(screen gocv.Mat, threshold float32) bool {
	// Prefer locating the actual orange button body in the tight bottom-left
	// HUD ROI. This is robust across language and avoids assuming one fixed
	// center coordinate.
	if x, y, ok := b.locateAttackButtonColor(screen); ok {
		b.logger.Info().Int("x", x).Int("y", y).Msg("attack button verified via localized orange region")
		return true
	}

	pinX, pinY := b.cal.ScaleRef(60, 695)
	if b.isOrange(screen, pinX, pinY) {
		b.logger.Debug().Msg("attack button confirmed via pinpoint color check")
		return true
	}

	tpl, ok := b.templates.Get("btn_attack")
	if !ok {
		return false
	}

	roi := b.buttonROI("btn_attack")
	physROI := image.Rect(
		int(float64(roi.Min.X)*b.cal.ScaleX),
		int(float64(roi.Min.Y)*b.cal.ScaleY),
		int(float64(roi.Max.X)*b.cal.ScaleX),
		int(float64(roi.Max.Y)*b.cal.ScaleY),
	)

	matches, err := vision.MatchMultiScaleROICached(screen, tpl, "btn_attack", 0.2, 2.0, 5, threshold, physROI)
	if err != nil || len(matches) == 0 {
		if err != nil {
			b.logger.Debug().Err(err).Msg("btn_attack template match error")
		}
		return false
	}

	best := matches[0]
	expectedX, expectedY := b.cal.ScaleRef(64, 666)
	dx := best.Point.X - expectedX
	if dx < 0 {
		dx = -dx
	}
	dy := best.Point.Y - expectedY
	if dy < 0 {
		dy = -dy
	}
	if dx > 80 || dy > 60 {
		b.logger.Debug().
			Float64("conf", best.Confidence).
			Int("match_x", best.Point.X).
			Int("match_y", best.Point.Y).
			Int("expected_x", expectedX).
			Int("expected_y", expectedY).
			Msg("attack-like template rejected: outside safe Attack button area")
		return false
	}

	b.logger.Info().
		Float64("conf", best.Confidence).
		Int("x", best.Point.X).
		Int("y", best.Point.Y).
		Msg("attack button verified via template and position")

	return true
}

func (b *Bot) isOrange(screen gocv.Mat, x, y int) bool {

	return b.colorCheck(screen, x, y,
		gocv.NewScalar(0, 100, 150, 0),
		gocv.NewScalar(150, 255, 255, 0),
		20)
}

// hasAttackButtonColor looks only at the tight bottom-left HUD zone where the
// village Attack button lives. A region test is much more robust than a single
// sampled pixel across languages, button animations and text overlays, while
// the tight ROI keeps it from confusing unrelated orange UI elsewhere.
func (b *Bot) hasAttackButtonColor(screen gocv.Mat) bool {
	_, _, ok := b.locateAttackButtonColor(screen)
	return ok
}

// locateFindMatchButtonColor finds the large orange/gold "Find Match" button
// after the village Attack button opens the attack menu. The old flow relied
// on a text/template match plus StateFindMatch, but the current CoC UI can
// still classify that overlay as MainVillage because the village remains
// visible behind it. A tight ROI + large orange blob is a safer signal.
func (b *Bot) locateFindMatchButtonColor(screen gocv.Mat) (int, int, bool) {
	x0, y0 := b.cal.ScaleRef(40, 420)
	x1, y1 := b.cal.ScaleRef(420, 640)

	if x0 < 0 { x0 = 0 }
	if y0 < 0 { y0 = 0 }
	if x1 > screen.Cols() { x1 = screen.Cols() }
	if y1 > screen.Rows() { y1 = screen.Rows() }
	if x1-x0 < 2 || y1-y0 < 2 {
		return 0, 0, false
	}

	roi := screen.Region(image.Rect(x0, y0, x1, y1))
	defer roi.Close()

	mask := vision.GetMat(roi.Rows(), roi.Cols(), gocv.MatTypeCV8UC1)
	defer vision.PutMat(mask)

	gocv.InRangeWithScalar(
		roi,
		gocv.NewScalar(0, 70, 110, 0),
		gocv.NewScalar(210, 255, 255, 0),
		&mask,
	)

	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()

	bestArea := 0.0
	bestRect := image.Rectangle{}
	for i := 0; i < contours.Size(); i++ {
		contour := contours.At(i)
		area := gocv.ContourArea(contour)
		if area <= bestArea {
			continue
		}
		rect := gocv.BoundingRect(contour)
		if rect.Dx() < 55 || rect.Dy() < 24 {
			continue
		}
		bestArea = area
		bestRect = rect
	}

	if bestArea < 900 || bestRect.Empty() {
		return 0, 0, false
	}

	x := x0 + bestRect.Min.X + bestRect.Dx()/2
	y := y0 + bestRect.Min.Y + bestRect.Dy()/2

	b.logger.Info().
		Float64("area", bestArea).
		Int("x", x).
		Int("y", y).
		Int("w", bestRect.Dx()).
		Int("h", bestRect.Dy()).
		Msg("Find Match button verified via localized orange region")

	return x, y, true
}

// locateAttackButtonColor returns the center of the largest orange/gold blob
// inside the tight bottom-left Attack-button ROI. Using the detected blob
// center is safer than tapping a historical hard-coded point: on the user's
// Windows/BlueStacks layout the fixed point landed on a neighbouring control.
// locateNextButtonColor finds the orange "Next" button on the live
// matchmaking/battle screen. This avoids depending on a localized text
// template and, importantly, lets us use the still-live capture instead of
// touching a cv::Mat after screen.Close().
func (b *Bot) locateNextButtonColor(screen gocv.Mat) (int, int, bool) {
	x0, y0 := b.cal.ScaleRef(650, 430)
	x1, y1 := b.cal.ScaleRef(860, 660)

	if x0 < 0 { x0 = 0 }
	if y0 < 0 { y0 = 0 }
	if x1 > screen.Cols() { x1 = screen.Cols() }
	if y1 > screen.Rows() { y1 = screen.Rows() }
	if x1-x0 < 2 || y1-y0 < 2 {
		return 0, 0, false
	}

	roi := screen.Region(image.Rect(x0, y0, x1, y1))
	defer roi.Close()

	mask := vision.GetMat(roi.Rows(), roi.Cols(), gocv.MatTypeCV8UC1)
	defer vision.PutMat(mask)

	// Broad orange/gold BGR range for the current CoC Next button.
	gocv.InRangeWithScalar(
		roi,
		gocv.NewScalar(0, 85, 145, 0),
		gocv.NewScalar(190, 255, 255, 0),
		&mask,
	)

	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()

	bestArea := 0.0
	bestRect := image.Rectangle{}
	for i := 0; i < contours.Size(); i++ {
		contour := contours.At(i)
		area := gocv.ContourArea(contour)
		if area <= bestArea {
			continue
		}
		rect := gocv.BoundingRect(contour)
		if rect.Dx() < 45 || rect.Dy() < 22 {
			continue
		}
		bestArea = area
		bestRect = rect
	}

	if bestArea < 700 || bestRect.Empty() {
		return 0, 0, false
	}

	x := x0 + bestRect.Min.X + bestRect.Dx()/2
	y := y0 + bestRect.Min.Y + bestRect.Dy()/2

	b.logger.Info().
		Float64("area", bestArea).
		Int("x", x).
		Int("y", y).
		Int("w", bestRect.Dx()).
		Int("h", bestRect.Dy()).
		Msg("Next button verified via orange region")

	return x, y, true
}

// locateBattleButtonColor finds the large green "Attack!" button in the
// army-selection screen. The current CoC layout keeps StateArmySelection
// visible while the old btn_battle template can match neighboring green UI,
// causing repeated taps that never leave the screen.
func (b *Bot) locateBattleButtonColor(screen gocv.Mat) (int, int, bool) {
	x0, y0 := b.cal.ScaleRef(560, 430)
	x1, y1 := b.cal.ScaleRef(860, 650)

	if x0 < 0 { x0 = 0 }
	if y0 < 0 { y0 = 0 }
	if x1 > screen.Cols() { x1 = screen.Cols() }
	if y1 > screen.Rows() { y1 = screen.Rows() }
	if x1-x0 < 2 || y1-y0 < 2 {
		return 0, 0, false
	}

	roi := screen.Region(image.Rect(x0, y0, x1, y1))
	defer roi.Close()

	mask := vision.GetMat(roi.Rows(), roi.Cols(), gocv.MatTypeCV8UC1)
	defer vision.PutMat(mask)

	// BGR green/lime family used by the large Attack! button.
	gocv.InRangeWithScalar(
		roi,
		gocv.NewScalar(0, 110, 70, 0),
		gocv.NewScalar(170, 255, 210, 0),
		&mask,
	)

	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()

	bestArea := 0.0
	bestRect := image.Rectangle{}
	for i := 0; i < contours.Size(); i++ {
		contour := contours.At(i)
		area := gocv.ContourArea(contour)
		if area <= bestArea {
			continue
		}
		rect := gocv.BoundingRect(contour)
		if rect.Dx() < 70 || rect.Dy() < 24 {
			continue
		}
		bestArea = area
		bestRect = rect
	}

	if bestArea < 1100 || bestRect.Empty() {
		return 0, 0, false
	}

	x := x0 + bestRect.Min.X + bestRect.Dx()/2
	y := y0 + bestRect.Min.Y + bestRect.Dy()/2

	b.logger.Info().
		Float64("area", bestArea).
		Int("x", x).
		Int("y", y).
		Int("w", bestRect.Dx()).
		Int("h", bestRect.Dy()).
		Msg("Battle Attack button verified via green region")

	return x, y, true
}

func (b *Bot) locateAttackButtonColor(screen gocv.Mat) (int, int, bool) {
	x0, y0 := b.cal.ScaleRef(0, 600)
	x1, y1 := b.cal.ScaleRef(145, 731)

	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > screen.Cols() {
		x1 = screen.Cols()
	}
	if y1 > screen.Rows() {
		y1 = screen.Rows()
	}
	if x1-x0 < 2 || y1-y0 < 2 {
		return 0, 0, false
	}

	roi := screen.Region(image.Rect(x0, y0, x1, y1))
	defer roi.Close()

	mask := vision.GetMat(roi.Rows(), roi.Cols(), gocv.MatTypeCV8UC1)
	defer vision.PutMat(mask)

	gocv.InRangeWithScalar(
		roi,
		gocv.NewScalar(0, 70, 110, 0),
		gocv.NewScalar(200, 255, 255, 0),
		&mask,
	)

	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()

	bestArea := 0.0
	bestRect := image.Rectangle{}
	for i := 0; i < contours.Size(); i++ {
		contour := contours.At(i)
		area := gocv.ContourArea(contour)
		if area <= bestArea {
			continue
		}
		rect := gocv.BoundingRect(contour)
		// Ignore tiny orange HUD/text fragments. The Attack button body should
		// form a materially sized blob in this ROI.
		if rect.Dx() < 18 || rect.Dy() < 12 {
			continue
		}
		bestArea = area
		bestRect = rect
	}

	if bestArea < 180 || bestRect.Empty() {
		return 0, 0, false
	}

	x := x0 + bestRect.Min.X + bestRect.Dx()/2
	y := y0 + bestRect.Min.Y + bestRect.Dy()/2

	b.logger.Debug().
		Float64("area", bestArea).
		Int("x", x).
		Int("y", y).
		Int("w", bestRect.Dx()).
		Int("h", bestRect.Dy()).
		Msg("localized Attack-button blob located")

	return x, y, true
}

// buttonROI returns the normalized (reference-resolution) region of interest
// for a known UI button template. Centralized here so the wait-for-button and
// find-and-click paths share one definition and cannot drift apart.
func (b *Bot) buttonROI(templateName string) image.Rectangle {
	switch templateName {
	case "btn_attack":
		// Bottom-left HUD only. The previous 300x232 ROI also contained
		// unrelated action buttons and produced false positives.
		return image.Rect(0, 600, 150, 732)
	case "btn_find_match":
		return image.Rect(50, 400, 400, 600)
	case "btn_battle":
		return image.Rect(300, 150, 860, 732)
	case "btn_army_arrow":
		return image.Rect(350, 100, 700, 300)
	case "btn_army_1":
		return image.Rect(400, 150, 650, 350)
	case "btn_next":
		return image.Rect(600, 450, 860, 732)
	default:
		return image.Rect(0, 0, 860, 732)
	}
}

func (b *Bot) templateMatch(screen gocv.Mat, name string, threshold float32) bool {
	tpl, ok := b.templates.Get(name)
	if !ok {
		return false
	}
	matches, err := vision.MatchMultiScaleROICached(screen, tpl, name, 0.2, 2.0, 5, threshold, image.Rect(0, 0, screen.Cols(), screen.Rows()))
	if err != nil {
		return false
	}
	return len(matches) > 0
}

func (b *Bot) executeAttackSequence(gc *game.GameContext) {
	// Panic guard for the long-running attack goroutine (spawned from
	// processFrame outside the frame-loop recover). A panic anywhere in
	// the deploy/search/battle-parse pipeline must release seqRunning
	// (via the pre-existing defer below, which LIFO-runs first) and log
	// instead of killing the whole bot process. The stuck-watchdog then
	// decides the next move.
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error().Interface("panic", r).Msg("recovered panic in attack sequence; abandoning sequence and continuing")
			b.recordActivity()
		}
	}()
	if !b.seqRunning.CompareAndSwap(false, true) {
		return
	}
	b.setRuntimePhase(PhaseAttackNavigation)
	defer func() {
		b.setRuntimePhase(PhaseIdle)
		b.seqRunning.Store(false)
	}()

	if b.cfg.Debug.UseShellPipe && runtime.GOOS != "windows" {
		b.client.EnablePersistentShell(b.cfg.Debug.ShellPipeSyncFlush)
		defer b.client.ClosePersistentShell()
	} else if b.cfg.Debug.UseShellPipe && runtime.GOOS == "windows" {
		// The persistent interactive ADB shell is not reliable enough on
		// BlueStacks/Windows yet. Live runs showed the pipe closing mid-sequence
		// ("use of closed network connection"), followed by capture/tap failures.
		// Use the proven one-shot transport path on Windows until the pipe has a
		// dedicated Windows implementation.
		b.logger.Info().Msg("persistent adb shell pipe disabled on Windows-safe path")
	}

	if b.attackCount.Load() >= int32(b.cfg.Attack.MaxAttackPerSession) {
		return
	}

	// Inter-attack cooldown. Armies need real time to retrain; without a
	// gate the bot re-attacked ~8s after every Return Home with whatever
	// the camps held (observed live: three near-identical defeats in <4
	// minutes). min_seconds_between_attacks is the human-real pause;
	// waiting inside the sequence goroutine (seqRunning is already held)
	// keeps the capture loop from starting a second sequence meanwhile.
	gap := time.Duration(b.cfg.Attack.MinSecondsBetweenAttacks) * time.Second
	if gap > 0 && !b.lastAttackEnd.IsZero() {
		if wait := gap - time.Since(b.lastAttackEnd); wait > 0 {
			b.logger.Info().
				Dur("wait", wait).
				Int("min_gap_s", b.cfg.Attack.MinSecondsBetweenAttacks).
				Msg("waiting out inter-attack cooldown before next search")
			select {
			case <-time.After(wait):
			case <-b.ctx.Done():
				return
			}
		}
	}

	if !b.clickSequence() {
		if until := b.armyWaitUntil.Load(); until > time.Now().UnixNano() {
			b.logger.Info().
				Time("retry_after", time.Unix(0, until)).
				Msg("attack sequence paused because army is not ready yet")
			return
		}
		b.logger.Warn().Msg("attack click sequence failed, restarting game to recover...")
		b.restartGame()
		return
	}

	b.setRuntimePhase(PhaseSearching)
	b.logger.Info().Msg("waiting for base to be found...")

	lootRec := game.NewLootRecognizer(b.cal, b.templates, b.logger)

	var remainingUndeployed int
	var deployErr error
	var stratName string = "Unknown"
	var targetEdge string = "Unknown"

	searchStart := time.Now()
	consecutiveNextFailures := 0
	skipsSinceRest := 0
	skipsThisSearch := 0
	for {
		// Stop check: a user Stop must abort the search loop even
		// though CaptureToMat below would silently reconnect a closed
		// transport and keep searching forever.
		select {
		case <-b.ctx.Done():
			b.logger.Info().Msg("search loop cancelled by stop, abandoning attack sequence")
			lootRec.Close()
			return
		default:
		}

		if time.Since(searchStart) > 5*time.Minute {
			b.logger.Error().Msg("searching/skipping bases took too long (stuck in clouds?), restarting game...")
			b.restartGame()
			lootRec.Close()
			return
		}

		if !b.sleepResponsive(400 * time.Millisecond) { lootRec.Close(); return }

		screen, err := b.client.CaptureToMat()
		if err != nil {
			return
		}

		state, _ := b.classify(screen)
		if state != game.StateBattle {
			if state == game.StateSearchMap || state == game.StateLoading {
				b.logger.Info().Str("state", state.String()).Msg("still searching (clouds)...")
				screen.Close()
				continue
			}
			b.logger.Info().Str("state", state.String()).Msg("searching area (wait)...")

			b.dismissInterruptions()
			screen.Close()
			continue
		}

		b.logger.Info().Msg("base found, reading loot...")
		loot, err := lootRec.ReadAvailableLoot(screen)
		if err != nil {
			b.logger.Warn().Err(err).Msg("failed to read loot")
			b.DumpDiagnostics("loot_read_failed", screen, map[string]interface{}{
				"error": err.Error(),
			})
		}

		b.logger.Info().
			Int("gold", loot.Gold).
			Int("elixir", loot.Elixir).
			Int("de", loot.DarkElixir).
			Msg("loot detected")

		meetsReq := !b.cfg.Search.Enabled || (loot.Gold >= b.cfg.Search.MinLootGold &&
			loot.Elixir >= b.cfg.Search.MinLootElixir &&
			loot.DarkElixir >= b.cfg.Search.MinLootDarkElixir)

		forcedBySearchCap := shouldForceAttackAfterSkips(b.cfg.Search.MaxSkipsBeforeForceAttack, skipsThisSearch)
		if forcedBySearchCap {
			b.logger.Warn().
				Int("skips", skipsThisSearch).
				Int("limit", b.cfg.Search.MaxSkipsBeforeForceAttack).
				Int("gold", loot.Gold).
				Int("elixir", loot.Elixir).
				Int("de", loot.DarkElixir).
				Msg("search cap reached; accepting current base to prevent endless matchmaking")
			meetsReq = true
		}

		if meetsReq {
			b.logger.Info().Msg("loot requirements met, starting attack!")
			b.setRuntimePhase(PhaseDeploying)
			b.attackExec.SetInitialLoot(loot.Gold, loot.Elixir, loot.DarkElixir)
			if strat, err := strategy.ParseYAML(b.cfg.Attack.StrategyFile); err == nil {
				stratName = strat.Name
				targetEdge = strat.TargetEdge
			}
			remainingUndeployed, deployErr = b.deployTroops(screen)
			b.attackExec.SetEarlyExitAllowed(deployErr == nil && remainingUndeployed == 0)
			if deployErr != nil || remainingUndeployed > 0 {
				b.logger.Warn().
					Err(deployErr).
					Int("remaining", remainingUndeployed).
					Msg("deployment not complete; keeping battle active and recording diagnostics")
				failScreen, err := b.client.CaptureToMat()
				if err == nil {
					b.DumpDiagnostics("deployment_failed", failScreen, map[string]interface{}{
						"error":      fmt.Sprintf("%v", deployErr),
						"remaining":  remainingUndeployed,
						"stratName":  stratName,
						"targetEdge": targetEdge,
					})
					failScreen.Close()
				}
			} else {
				b.logger.Info().Msg("all live deployable troop slots verified empty")
			}
			screen.Close()
			break
		}

		b.logger.Info().Msg("loot too low, skipping base...")

		// BlueStacks stability guard: changing opponents endlessly at full
		// speed can put sustained pressure on HD-Player.exe. Rest briefly
		// every few successful skips instead of hammering Next/capture forever.
		if skipsSinceRest >= 8 {
			b.logger.Info().Msg("matchmaking stability pause after 8 skips")
			if !b.sleepResponsive(1500 * time.Millisecond) { lootRec.Close(); return }
			skipsSinceRest = 0
		}

		// NEXT is handled as a state transition, not as a blind tap.
		// A successful ADB tap only means Android received the event; it does
		// NOT mean Clash accepted it. We click once, then wait until clouds /
		// loading / Unknown proves that matchmaking actually advanced.
		clickNextFresh := func() bool {
			fresh, capErr := b.client.CaptureToMat()
			if capErr != nil || fresh.Empty() {
				if !fresh.Empty() { fresh.Close() }
				return false
			}
			defer fresh.Close()

			if x, y, ok := b.locateNextButtonColor(fresh); ok {
				b.logger.Info().Int("x", x).Int("y", y).Msg("Next button freshly verified; precision clicking")
				if err := b.client.TapFast(x, y, 0.6); err == nil {
					b.recordActivity()
					return true
				}
			}
			return false
		}

		// Use the already-live frame first.
		nextClicked := false
		if x, y, ok := b.locateNextButtonColor(screen); ok {
			b.logger.Info().Int("x", x).Int("y", y).Msg("Next button verified; precision clicking detected center")
			if err := b.client.TapFast(x, y, 0.6); err == nil {
				b.recordActivity()
				nextClicked = true
			}
		}
		screen.Close()

		if !nextClicked {
			nextClicked = clickNextFresh()
		}

		transitioned := false
		if nextClicked {
			// Give Clash/BlueStacks time to start the clouds transition before
			// asking for another screenshot. The old 220ms polling burst could
			// issue 8-12 PNG screencaps immediately after every Next tap and
			// was correlated with HD-Player.exe access-violation crashes.
			if !b.sleepResponsive(650 * time.Millisecond) { lootRec.Close(); return }
			for verify := 0; verify < 3 && !transitioned; verify++ {
				probe, capErr := b.client.CaptureToMat()
				if capErr == nil && !probe.Empty() {
					st, _ := b.classify(probe)
					probe.Close()
					if st == game.StateSearchMap || st == game.StateLoading || st == game.StateUnknown {
						transitioned = true
						break
					}
				} else if !probe.Empty() {
					probe.Close()
				}
				if verify < 2 {
					if !b.sleepResponsive(550 * time.Millisecond) { lootRec.Close(); return }
				}
			}
		}

		// If Clash ignored the first tap, reacquire the button and try ONCE.
		// This replaces the situation where the bot looked "lost" until the
		// user manually clicked Next, while also preventing rapid tap spam.
		if !transitioned {
			b.logger.Warn().Msg("Next tap did not start matchmaking; reacquiring button for one controlled retry")
			if !b.sleepResponsive(450 * time.Millisecond) { lootRec.Close(); return }
			if clickNextFresh() {
				if !b.sleepResponsive(700 * time.Millisecond) { lootRec.Close(); return }
				for verify := 0; verify < 3 && !transitioned; verify++ {
					probe, capErr := b.client.CaptureToMat()
					if capErr == nil && !probe.Empty() {
						st, _ := b.classify(probe)
						probe.Close()
						if st == game.StateSearchMap || st == game.StateLoading || st == game.StateUnknown {
							transitioned = true
							break
						}
					} else if !probe.Empty() {
						probe.Close()
					}
					if verify < 2 {
						if !b.sleepResponsive(600 * time.Millisecond) { lootRec.Close(); return }
					}
				}
			}
		}

		if transitioned {
			consecutiveNextFailures = 0
			skipsSinceRest++
			skipsThisSearch++
			b.skipsCount.Add(1)
			if b.OnStatsUpdate != nil {
				b.OnStatsUpdate()
			}
			b.logger.Info().Msg("matchmaking transition confirmed after Next")
			if !b.sleepResponsive(700 * time.Millisecond) { lootRec.Close(); return }
			continue
		}

		// Never fall back to repeated blind coordinates. If two verified
		// attempts fail, back off. After 3 consecutive failures restart only
		// Clash (not BlueStacks) to recover a wedged matchmaking UI.
		consecutiveNextFailures++
		b.logger.Warn().
			Int("failures", consecutiveNextFailures).
			Msg("Next transition not confirmed; backing off instead of spamming taps")

		if consecutiveNextFailures >= 3 {
			b.logger.Error().Msg("Next remained unresponsive after controlled retries; restarting Clash to recover matchmaking")
			lootRec.Close()
			b.restartGame()
			return
		}

		if !b.sleepResponsive(900 * time.Millisecond) { lootRec.Close(); return }
	}

	if deployErr != nil || remainingUndeployed > 0 {
		b.logger.Warn().
			Int("remaining", remainingUndeployed).
			Msg("deployment ended with units still unverified; battle continues but deployment is NOT marked complete")
	} else {
		b.logger.Info().Msg("battle deployment complete: all live deployable units verified, waiting for battle to end naturally...")
	}

	b.setRuntimePhase(PhaseBattle)
	var battleStars int = 0
	var battleGold int = 0
	var battleElixir int = 0
	var battleDE int = 0
	var bonusGold, bonusElixir, bonusDE int = 0, 0, 0
	var parsedResults bool = false

	if b.attackExec.WaitForBattleEndCtx(b.ctx, 4*time.Minute) {
		b.setRuntimePhase(PhaseParsingResult)

		// WaitForBattleEnd returns the moment the result overlay's Return
		// Home button is detected, but the overlay is still animating in:
		// the star counter lights up one-by-one and the loot numbers count
		// up over ~1.5-2s. Capturing right then freezes the animation at
		// frame 0, and the OCR reads it as 0 stars / 0 loot (observed live:
		// a 50%-destruction battle parsed as 0 stars / 0 loot, while the
		// SAME screenshot parsed minutes later as 1 star / 444k gold).
		//
		// Settle first, then parse. If the read comes back all-zero we
		// cannot assume the animation is still running — a genuine 0%
		// destruction loss reads exactly the same. The discriminator is
		// frame stability: once two captures ~1s apart are pixel-identical
		// in the result panel, the count-up finished and the (possibly
		// zero) value is the true result. Every attempt overwrites
		// last_battle_result.png with the freshest frame so the saved
		// artifact matches the final parse.
		b.client.JitteredSleep(1800 * time.Millisecond)

		// Authoritative star signal: the destruction percentage the battle
		// wait sampled from the stall ROI (proven live: valk runs tracked
		// 23% -> 50% tick by tick). CoC scores stars from the outcome,
		// not from the result-screen art: >=50% = 1 star, TH destroyed =
		// +1, 100% = 3 (see game.StarsFromOutcome). When the wait had no
		// percent ROI to sample (no stall_config, no end_at_percent) this
		// stays 0 and the visual parse below is the fallback.
		finalPct := b.attackExec.LastDestructionPercent()

		var parsedResult game.BattleResult
		parsedOK := false
		prevHash := uint64(0)
		for attempt := 0; attempt < 3 && !parsedOK; attempt++ {
			resultScreen, err := b.client.CaptureToMat()
			if err != nil {
				b.logger.Warn().Err(err).Msg("battle result capture failed; retrying")
				time.Sleep(500 * time.Millisecond)
				continue
			}
			gocv.IMWrite(paths.ResolveConfig("last_battle_result.png"), resultScreen)
			b.logger.Info().Msg("saved battle result screenshot to last_battle_result.png")

			lootRec := game.NewLootRecognizer(b.cal, b.templates, b.logger)
			res, rerr := lootRec.ReadBattleResult(resultScreen)
			hash := resultPanelHash(resultScreen, b.cal)
			resultScreen.Close()
			lootRec.Close()

			if rerr != nil {
				b.logger.Warn().Err(rerr).Msg("battle result parse error; retrying")
				time.Sleep(800 * time.Millisecond)
				continue
			}

			settled := hash != 0 && hash == prevHash
			if res.Stars > 0 || res.Loot.Gold > 0 || res.Loot.Elixir > 0 || res.Loot.DarkElixir > 0 || settled {
				parsedResult = res
				parsedOK = true
				if settled {
					b.logger.Debug().Msg("battle result accepted: result panel stable across captures")
				} else {
					b.logger.Debug().Msg("battle result accepted (non-empty read)")
				}
				break
			}

			prevHash = hash
			b.logger.Warn().Int("attempt", attempt).Msg("battle result read empty and panel still changing; overlay animating, retrying...")
			time.Sleep(1000 * time.Millisecond)
		}

		if parsedOK {
			visualStars := parsedResult.Stars
			battleGold = parsedResult.Loot.Gold
			battleElixir = parsedResult.Loot.Elixir
			battleDE = parsedResult.Loot.DarkElixir
			bonusGold = parsedResult.Bonus.Gold
			bonusElixir = parsedResult.Bonus.Elixir
			bonusDE = parsedResult.Bonus.DarkElixir
			parsedResults = true

			// Reconcile stars against the measured destruction. The result
			// screen remains useful for distinguishing 1 vs 2 stars, but it
			// may never claim an impossible outcome (e.g. 3 stars below 100%
			// or 0 stars at >=50%). This removes the "random" history stars
			// while still preserving a genuine TH star under 50%.
			battleStars = visualStars
			if finalPct >= 100 {
				battleStars = 3
			} else if finalPct > 0 {
				ruleStars := game.StarsFromOutcome(finalPct, b.attackExec.ThDestroyed())
				if finalPct >= 50 {
					if visualStars < 1 || visualStars > 2 {
						battleStars = ruleStars
					}
				} else {
					if visualStars < 0 || visualStars > 1 {
						battleStars = ruleStars
					}
				}
				if battleStars != visualStars {
					b.logger.Warn().
						Int("visual_stars", visualStars).
						Int("reconciled_stars", battleStars).
						Int("destruction_pct", finalPct).
						Bool("th_destroyed", b.attackExec.ThDestroyed()).
						Msg("result-screen stars rejected as inconsistent with battle outcome")
				}
			}

			// Prefer the battle's live Available-Loot delta over themed
			// result-screen OCR whenever two stable live reads were accepted.
			// This directly measures what disappeared from the enemy's loot
			// counters and is substantially more stable across CoC themes.
			if liveLoot, ok := b.attackExec.EstimatedLootStolen(); ok {
				b.logger.Info().
					Int("live_gold", liveLoot.Gold).
					Int("ocr_gold", battleGold).
					Int("live_elixir", liveLoot.Elixir).
					Int("ocr_elixir", battleElixir).
					Int("live_de", liveLoot.DarkElixir).
					Int("ocr_de", battleDE).
					Msg("using stable live-loot delta as authoritative attack loot")
				battleGold = liveLoot.Gold
				battleElixir = liveLoot.Elixir
				battleDE = liveLoot.DarkElixir
			}
		} else {
			if finalPct > 0 {
				battleStars = game.StarsFromOutcome(finalPct, b.attackExec.ThDestroyed())
				b.logger.Warn().
					Int("stars", battleStars).
					Int("destruction_pct", finalPct).
					Msg("result-screen OCR failed; stars derived from measured battle outcome")
			} else {
				b.logger.Error().Msg("battle result OCR failed after retries; no reliable destruction read available")
			}

			if liveLoot, ok := b.attackExec.EstimatedLootStolen(); ok {
				battleGold = liveLoot.Gold
				battleElixir = liveLoot.Elixir
				battleDE = liveLoot.DarkElixir
				b.logger.Info().
					Int("gold", battleGold).
					Int("elixir", battleElixir).
					Int("de", battleDE).
					Msg("result-screen OCR failed; using stable live-loot delta")
			}
		}

		// Totals always use the reconciled values that are also written to
		// attack_history.json, so dashboard cards and attack rows can no
		// longer disagree.
		b.totalGold.Add(int64(battleGold + bonusGold))
		b.totalElixir.Add(int64(battleElixir + bonusElixir))
		b.totalDE.Add(int64(battleDE + bonusDE))
		b.totalStars.Add(int32(battleStars))

		switch battleStars {
		case 0:
			b.stars0.Add(1)
		case 1:
			b.stars1.Add(1)
		case 2:
			b.stars2.Add(1)
		case 3:
			b.stars3.Add(1)
		}

		b.logger.Info().
			Int("stars", battleStars).
			Int("gold", battleGold).
			Int("elixir", battleElixir).
			Int("de", battleDE).
			Int("bonus_gold", bonusGold).
			Int("destruction_pct", finalPct).
			Msg("battle result reconciled and ready for history")
	} else if b.ctx.Err() != nil {
		// The bot was stopped mid-battle. Exit cleanly — no forced
		// restart (the ADB client is already being torn down by the
		// detached Stop, and restartGame would just log a stream of
		// transport errors against a closed client).
		b.logger.Info().Msg("battle ended by user stop, abandoning sequence")
		return
	} else {
		b.logger.Error().Msg("battle end timeout (stuck in battle?), restarting game...")
		b.restartGame()
		return
	}

	// Record the attack in history IMMEDIATELY after the result is
	// parsed, so the Attack History row appears in the UI at the same
	// moment the loot totals tick up. Previously this block ran after
	// ReturnHome + wall upgrades (which can take minutes), so the
	// dashboard showed fresh gold/elixir totals for minutes before the
	// history row appeared — and the entry was dropped entirely if
	// ReturnHome failed.
	b.attackCount.Add(1)

	depErrStr := ""
	if deployErr != nil {
		depErrStr = deployErr.Error()
	}

	rep := AttackReport{
		Timestamp:        time.Now().Format(time.RFC3339),
		Strategy:         stratName,
		TargetEdge:       targetEdge,
		DeploySuccess:    deployErr == nil && remainingUndeployed == 0,
		UndeployedSlots:  remainingUndeployed,
		DeployError:      depErrStr,
		ParsedResults:    parsedResults,
		Stars:            battleStars,
		GoldStolen:       battleGold,
		ElixirStolen:     battleElixir,
		DarkElixirStolen: battleDE,
		BonusGold:        bonusGold,
		BonusElixir:      bonusElixir,
		BonusDE:          bonusDE,
		TotalAttacks:     b.attackCount.Load(),
	}

	if repBytes, err := json.MarshalIndent(rep, "", "  "); err == nil {
		_ = AsyncWriteFile(paths.ResolveConfig("last_attack_report.json"), repBytes, 0644)
	}

	history := b.historyCache
	if history == nil {
		if histData, err := os.ReadFile(paths.ResolveConfig("attack_history.json")); err == nil {
			_ = json.Unmarshal(histData, &history)
		}
	}
	history = append([]AttackReport{rep}, history...)
	if len(history) > 500 {
		history = history[:500]
	}
	b.historyCache = history
	if histBytes, err := json.MarshalIndent(history, "", "  "); err == nil {
		_ = AsyncWriteFile(paths.ResolveConfig("attack_history.json"), histBytes, 0644)
	}

	// Notify the UI AFTER the report is in historyCache and
	// attack_history.json is flushed to disk. Firing this earlier
	// (right after ReadBattleResult) raced the App's refreshHistory
	// cache re-read with the report write, so the UI stayed a full
	// attack behind even though the loot totals (live atomics) moved
	// instantly. AsyncWriteFile blocks until the worker flushes, so
	// by the time we get here the file on disk contains this report.
	if b.OnStatsUpdate != nil {
		b.OnStatsUpdate()
	}

	b.setRuntimePhase(PhaseReturningHome)
	returnedHome := false
	if err := b.attackExec.ReturnHome(); err == nil {
		returnedHome = true
	} else {
		b.logger.Warn().Err(err).Msg("ReturnHome failed, attempting template fallback")
		for i := 0; i < 3; i++ {
			if b.findAndClick("btn_return_home", "Return Home", 1) {
				returnedHome = true
				break
			}
			time.Sleep(1 * time.Second)
		}
	}

	if !returnedHome {
		b.logger.Error().Msg("failed to return home after battle, restarting game...")
		b.restartGame()
		return
	}

	// Stamp the attack boundary so the inter-attack cooldown has a clean
	// reference point (set only on a real return home, not on a restart).
	b.lastAttackEnd = time.Now()

	sideX := int(537 * b.cal.ScaleX)
	sideY := int(693 * b.cal.ScaleY)
	b.logger.Info().Msg("Tapping side area to dismiss potential post-attack popups...")
	_ = b.client.Tap(sideX, sideY)
	time.Sleep(1000 * time.Millisecond)

	if b.cfg.Upgrade.UpgradeWalls {
		b.UpgradeWalls(gc)
	}

	// Cap check stays after wall upgrades so the graceful shutdown (2s
	// grace then cancel) never interrupts an in-progress wall loop; the
	// count itself was already incremented when the report was recorded.
	if int(b.attackCount.Load()) >= b.cfg.Attack.MaxAttackPerSession {
		b.logger.Info().
			Int32("attacks", b.attackCount.Load()).
			Int("cap", b.cfg.Attack.MaxAttackPerSession).
			Msg("attack cap reached, scheduling graceful shutdown...")
		go func() {

			time.Sleep(2 * time.Second)
			b.cancel()
		}()
	}

	deployStatus := "SUCCESS (100% Deployed)"
	if !rep.DeploySuccess {
		if remainingUndeployed > 0 {
			deployStatus = fmt.Sprintf("FAILED (%d Slots Undeployed)", remainingUndeployed)
		} else {
			deployStatus = fmt.Sprintf("FAILED (%s)", depErrStr)
		}
	}

	fmt.Println()
	fmt.Println("=========================================")
	fmt.Println("          BATTLE REPORT SUMMARY          ")
	fmt.Println("=========================================")
	fmt.Printf("Strategy:      %s\n", rep.Strategy)
	fmt.Printf("Target Edge:   %s\n", rep.TargetEdge)
	fmt.Printf("Deploy Health: %s\n", deployStatus)
	fmt.Printf("Stars Earned:  %d ⭐\n", rep.Stars)
	fmt.Println("Loot Collected:")
	fmt.Printf("  - Gold:      %d\n", rep.GoldStolen)
	fmt.Printf("  - Elixir:    %d\n", rep.ElixirStolen)
	fmt.Printf("  - DE:        %d\n", rep.DarkElixirStolen)
	fmt.Println("=========================================")
	fmt.Println()

	b.logger.Info().
		Int32("attacks", b.attackCount.Load()).
		Str("stars", fmt.Sprintf("3⭐:%d | 2⭐:%d | 1⭐:%d | 0⭐:%d", b.stars3.Load(), b.stars2.Load(), b.stars1.Load(), b.stars0.Load())).
		Str("loot", fmt.Sprintf("Gold: %d | Elixir: %d | DE: %d", b.totalGold.Load(), b.totalElixir.Load(), b.totalDE.Load())).
		Dur("uptime", time.Since(b.startedAt)).
		Msg("=== SESSION SUMMARY ===")

	b.zoomedOut.Store(false)
}

// focusedButtonClick trades a tiny amount of latency for much better UI
// precision. At the faster capture cadence a button can still be moving during
// its opening animation; clicking the first detected contour can therefore hit
// an edge or neighboring control. We require two consecutive detections with a
// stable center, average them, then issue a very-low-jitter TapFast.
func (b *Bot) focusedButtonClick(name string, locator func(gocv.Mat) (int, int, bool), attempts int) bool {
	if attempts <= 0 {
		attempts = 1
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		first, err := b.client.CaptureToMat()
		if err != nil || first.Empty() {
			if !first.Empty() { first.Close() }
			time.Sleep(70 * time.Millisecond)
			continue
		}
		x1, y1, ok1 := locator(first)
		first.Close()
		if !ok1 {
			time.Sleep(70 * time.Millisecond)
			continue
		}

		// Let the button finish a few animation frames, then confirm its center.
		time.Sleep(85 * time.Millisecond)
		second, err := b.client.CaptureToMat()
		if err != nil || second.Empty() {
			if !second.Empty() { second.Close() }
			continue
		}
		x2, y2, ok2 := locator(second)
		second.Close()
		if !ok2 {
			continue
		}

		dx := x2 - x1
		if dx < 0 { dx = -dx }
		dy := y2 - y1
		if dy < 0 { dy = -dy }

		// More than ~10 px movement means the UI is still animating or the two
		// frames latched onto different blobs. Wait for the next stable pair.
		maxDrift := int(10.0 * b.cal.ScaleX)
		if maxDrift < 6 { maxDrift = 6 }
		if dx > maxDrift || dy > maxDrift {
			b.logger.Debug().
				Str("button", name).
				Int("dx", dx).
				Int("dy", dy).
				Msg("button center still moving; waiting for stable focus")
			time.Sleep(70 * time.Millisecond)
			continue
		}

		x := (x1 + x2) / 2
		y := (y1 + y2) / 2
		b.logger.Info().
			Str("button", name).
			Int("x", x).
			Int("y", y).
			Int("drift_x", dx).
			Int("drift_y", dy).
			Msg("focused click locked on stable button center")

		// 0.6px sigma keeps a microscopic human-like variation without the
		// several-pixel spread of TapRandomized (3.5px sigma).
		if err := b.client.TapFast(x, y, 0.6); err != nil {
			b.logger.Warn().Err(err).Str("button", name).Msg("focused tap failed")
			continue
		}
		b.recordActivity()
		return true
	}
	return false
}

// waitForStableLocator waits until a target is visible at a stable center.
// This is intentionally used BETWEEN critical menu clicks so the bot never
// chains taps into an animation that has not finished opening yet.
func (b *Bot) waitForStableLocator(name string, locator func(gocv.Mat) (int, int, bool), timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	var lastX, lastY int
	stable := 0

	for time.Now().Before(deadline) {
		screen, err := b.client.CaptureToMat()
		if err != nil || screen.Empty() {
			if !screen.Empty() { screen.Close() }
			time.Sleep(90 * time.Millisecond)
			continue
		}
		x, y, ok := locator(screen)
		screen.Close()
		if !ok {
			stable = 0
			time.Sleep(90 * time.Millisecond)
			continue
		}

		if stable > 0 {
			dx := x-lastX; if dx < 0 { dx = -dx }
			dy := y-lastY; if dy < 0 { dy = -dy }
			if dx <= 8 && dy <= 8 {
				stable++
			} else {
				stable = 1
			}
		} else {
			stable = 1
		}
		lastX, lastY = x, y

		if stable >= 2 {
			b.logger.Info().Str("target", name).Int("x", x).Int("y", y).Msg("next UI target is stable and ready")
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	b.logger.Warn().Str("target", name).Dur("timeout", timeout).Msg("next UI target did not become stable in time")
	return false
}

func (b *Bot) clickSequence() bool {

	attackClicked := false
	for attempt := 0; attempt < 3; attempt++ {
		// findAttackButton already has the Windows-safe localized/color checks.
		// Do not require the older text/template matcher a second time here:
		// that created the contradictory "Attack detected" -> "could not find
		// Attack button" failure seen on localized/animated village frames.
		if b.focusedButtonClick("Attack", b.locateAttackButtonColor, 2) {
			attackClicked = true
			break
		}
		if screen, err := b.client.CaptureToMat(); err == nil {
			if b.findAttackButton(screen, 0.30) {
				x, y := b.cal.ScaleRef(64, 666)
				screen.Close()
				b.logger.Info().Int("x", x).Int("y", y).Msg("Attack fallback verified; precision tapping canonical center")
				if err := b.client.TapFast(x, y, 0.5); err == nil {
					b.recordActivity()
					attackClicked = true
					break
				}
			} else {
				screen.Close()
			}
		}
		if !b.sleepResponsive(220 * time.Millisecond) { return false }
	}
	if !attackClicked {
		b.logger.Warn().Msg("could not find or click Attack button")
		if screen, err := b.client.CaptureToMat(); err == nil {
			b.DumpDiagnostics("click_attack_failed", screen, nil)
			screen.Close()
		}
		return false
	}
	// Do not chain directly into the next tap. Wait for the attack menu to
	// finish opening and for Find Match to be stable in two consecutive frames.
	if !b.waitForStableLocator("Find Match", b.locateFindMatchButtonColor, 3*time.Second) {
		b.logger.Warn().Msg("attack menu did not settle on Find Match after Attack click")
	}

	findMatchClicked := false
	for attempt := 0; attempt < 3; attempt++ {
		if b.focusedButtonClick("Find Match", b.locateFindMatchButtonColor, 2) {
			findMatchClicked = true
			break
		}
		if screen, err := b.client.CaptureToMat(); err == nil {
			state, score := b.classify(screen)
			screen.Close()
			if state == game.StateFindMatch {
				x, y := b.cal.ScaleRef(215, 563)
				b.logger.Info().
					Int("score", score).
					Int("x", x).
					Int("y", y).
					Msg("Find Match screen verified by classifier; precision tapping canonical center")
				if err := b.client.TapFast(x, y, 0.5); err == nil {
					b.recordActivity()
					findMatchClicked = true
					break
				}
			} else {
				b.logger.Info().
					Str("state", state.String()).
					Int("score", score).
					Msg("Find Match retry: no stable focused target yet")
			}
		}

		// Keep the legacy template path as a final fallback, not the primary
		// detector for this localized/current CoC screen.
		if b.findAndClick("btn_find_match", "Find Match", 1) {
			findMatchClicked = true
			break
		}

		if !b.sleepResponsive(220 * time.Millisecond) { return false }
	}
	if !findMatchClicked {
		b.logger.Warn().Msg("could not find or click Find Match button")
		if screen, err := b.client.CaptureToMat(); err == nil {
			state, score := b.classify(screen)
			b.DumpDiagnostics("click_find_match_failed", screen, map[string]interface{}{
				"classified_state": state.String(),
				"classified_score": score,
			})
			screen.Close()
		}
		return false
	}
	// Find Match opens a transition/menu. Give it a real state transition
	// window instead of firing Army Arrow at a stale frame.
	armyReadyDeadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(armyReadyDeadline) {
		s, err := b.client.CaptureToMat()
		if err == nil && !s.Empty() {
			st, _ := b.classify(s)
			s.Close()
			if st == game.StateArmySelection || st == game.StateArmyCamp {
				b.logger.Info().Str("state", st.String()).Msg("army menu state confirmed before next click")
				break
			}
		}
		if !b.sleepResponsive(120 * time.Millisecond) { return false }
	}

	armyArrowClicked := false
	for attempt := 0; attempt < 3; attempt++ {
		if b.findAndClick("btn_army_arrow", "Army Arrow", 1) {
			armyArrowClicked = true
			break
		}
		if !b.sleepResponsive(220 * time.Millisecond) { return false }
	}
	if !armyArrowClicked {
		b.logger.Warn().Msg("could not find or click Army Arrow button")
		if screen, err := b.client.CaptureToMat(); err == nil {
			b.DumpDiagnostics("click_army_arrow_failed", screen, nil)
			screen.Close()
		}
		return false
	}
	if !b.sleepResponsive(220 * time.Millisecond) { return false }

	armyClicked := false
	for attempt := 0; attempt < 3; attempt++ {
		if b.selectArmySlot() {
			armyClicked = true
			break
		}
		if !b.sleepResponsive(220 * time.Millisecond) { return false }
	}
	if !armyClicked {
		b.logger.Warn().Int("army_slot", b.armySlot).Msg("army recipe card did not appear, continuing anyway")
		if screen, err := b.client.CaptureToMat(); err == nil {
			b.DumpDiagnostics("click_army_slot_not_found", screen, map[string]interface{}{"army_slot": b.armySlot})
			screen.Close()
		}
	}
	if !b.sleepResponsive(220 * time.Millisecond) { return false }

	// Pre-battle army gate. Require multi-frame consensus so one noisy OCR
	// read cannot launch a bad attack or create a bogus training deficit.
	if b.cfg.Training.Enabled &&
		b.cfg.Training.FullArmyBeforeAttack &&
		b.cfg.Automation.AutoArmyGuard {
		if profile, ok := b.cfg.Attack.Farm.ActiveProfile(); ok {
			guard := b.inspectArmyConsensus(profile, 3)

			for _, warning := range guard.Warnings {
				b.logger.Debug().Str("detail", warning).Msg("pre-battle army inspection")
			}

			switch guard.Decision {
				case attack.ArmyGuardNotReady:
					if b.cfg.Automation.Preferences.AutoRetrain {
						b.logger.Info().
							Int("warnings", len(guard.Warnings)).
							Msg("army recipe incomplete; attempting one verified instant recipe repair")
						if repairedGuard, repaired := b.reapplyActiveArmyRecipe(profile); repaired {
							guard = repairedGuard
							b.armyWaitUntil.Store(0)
							b.trainingItemsPending.Store(0)
							b.trainingHousingPending.Store(0)
							b.trainingPlanUncertain.Store(false)
							b.statusMu.Lock()
							b.trainingPending = nil
							b.statusMu.Unlock()
							_ = attack.WriteTrainingPlan(attack.BuildTrainingPlan(profile, guard))
							b.logger.Info().Msg("army recipe repair verified; continuing to Battle")
							break
						} else {
							guard = repairedGuard
							b.logger.Warn().Msg("army recipe repair did not reach a verified ready state")
						}
					}

					plan := attack.BuildTrainingPlan(profile, guard)
					if err := attack.ValidateTrainingPlan(plan, profile); err != nil {
						plan.HasUncertain = true
						b.logger.Error().Err(err).Msg("generated training plan failed safety validation; executor must not act on it")
					}
					actionable := attack.ActionableTrainingItems(plan)
					b.trainingItemsPending.Store(int32(len(actionable)))
					b.trainingHousingPending.Store(int32(plan.TotalHousing))
					b.trainingPlanUncertain.Store(plan.HasUncertain)
					b.statusMu.Lock()
					b.trainingPending = append([]attack.TrainingPlanItem(nil), plan.Items...)
					b.statusMu.Unlock()
					if err := attack.WriteTrainingPlan(plan); err != nil {
						b.logger.Warn().Err(err).Msg("could not persist pending training plan")
					} else {
						b.logger.Info().
							Int("items", len(plan.Items)).
							Int("actionable_items", len(actionable)).
							Int("housing_to_train", plan.TotalHousing).
							Bool("has_uncertain", plan.HasUncertain).
							Msg("training plan generated and safety-validated from live army deficits")
					}

					// Army recipes apply instantly in the modern game. This
					// delay is only a retry backoff after a failed repair.
					until := time.Now().Add(30 * time.Second)
					b.armyWaitUntil.Store(until.UnixNano())
					b.logger.Warn().
						Time("retry_after", until).
						Int("warnings", len(guard.Warnings)).
						Int("training_items", len(plan.Items)).
						Msg("army recipe still does not match configured farm profile; aborting matchmaking before Battle")

					// Return with proof after every Back press. This shares the
					// same bounded safety rule as donation cleanup and never
					// assumes that N Back presses are harmless.
					if b.returnToVillageVerified(3, "army readiness block") {
						b.logger.Info().Msg("returned to village after army readiness block")
					}
					return false

				case attack.ArmyGuardReady:
					b.armyWaitUntil.Store(0)
					b.trainingItemsPending.Store(0)
					b.trainingHousingPending.Store(0)
					b.trainingPlanUncertain.Store(false)
					b.statusMu.Lock()
					b.trainingPending = nil
					b.statusMu.Unlock()
					_ = attack.WriteTrainingPlan(attack.BuildTrainingPlan(profile, guard))
					b.logger.Info().Msg("pre-battle army guard: configured troops/spells ready")

			case attack.ArmyGuardUncertain:
				if b.cfg.Automation.Preferences.WaitForFullArmy {
					until := time.Now().Add(20 * time.Second)
					b.armyWaitUntil.Store(until.UnixNano())
					b.trainingPlanUncertain.Store(true)
					b.logger.Warn().
						Int("warnings", len(guard.Warnings)).
						Time("retry_after", until).
						Msg("army readiness stayed uncertain; Easy Mode refuses to launch an unverified attack")
					_ = b.returnToVillageVerified(3, "uncertain army readiness")
					return false
				}
				b.logger.Warn().
					Int("warnings", len(guard.Warnings)).
					Msg("army readiness uncertain; advanced mode permits attack")
			}
		}
	}

	battleClicked := false
	for attempt := 0; attempt < 3; attempt++ {
		if b.focusedButtonClick("Battle Attack", b.locateBattleButtonColor, 2) {
			battleClicked = true
			break
		}
		if b.findAndClick("btn_battle", "Battle", 1) {
			battleClicked = true
			break
		}
		if !b.sleepResponsive(220 * time.Millisecond) { return false }
	}
	if !battleClicked {
		b.logger.Warn().Msg("could not find or click Battle button")
		return false
	}

	b.logger.Info().Msg("waiting for battle state (searching)...")
	return b.waitForBattleState(60 * time.Second)
}

// selectArmySlot clicks the saved-recipe card for b.armySlot in the
// army-selection list opened by the Army Arrow. Each recipe card is a
// ~146px-tall row in a vertically scrolling list; the first card sits at
// ref-y 227 and cards stack every ~54px once the list is expanded
// (measured on the live 860x732 BlueStacks layout). Slot 1 keeps the
// legacy template path (btn_army_1) for backward compatibility with
// older game layouts; slots 2+ use the measured card geometry.
func (b *Bot) selectArmySlot() bool {
	slot := b.armySlot
	if slot <= 0 {
		slot = 1
	}

	if slot == 1 {
		before, err := b.client.CaptureToMat()
		if err != nil || before.Empty() {
			if !before.Empty() { before.Close() }
			return false
		}
		stateBefore, _ := b.classify(before)
		if stateBefore != game.StateArmySelection && stateBefore != game.StateArmyCamp {
			before.Close()
			b.logger.Warn().Str("state", stateBefore.String()).Msg("refusing Army 1 selection outside verified army menu")
			return false
		}

		if !b.findAndClick("btn_army_1", "Army 1", 1) {
			before.Close()
			return false
		}
		if !b.sleepResponsive(180 * time.Millisecond) {
			before.Close()
			return false
		}

		after, capErr := b.client.CaptureToMat()
		if capErr != nil || after.Empty() {
			before.Close()
			if !after.Empty() { after.Close() }
			return false
		}
		refX, refY := b.cal.ScaleRef(513, 230)
		delta := localVisualDelta(before, after, image.Pt(refX, refY), int(95*b.cal.ScaleX), int(42*b.cal.ScaleY))
		stateAfter, _ := b.classify(after)
		before.Close()
		after.Close()

		if delta < 0.012 && stateAfter == stateBefore {
			b.logger.Warn().
				Float64("visual_delta", delta).
				Str("state", stateAfter.String()).
				Msg("Army 1 tap produced no verified UI progress")
			return false
		}
		b.recordActivity()
		b.logger.Info().Float64("visual_delta", delta).Msg("Army 1 recipe selection verified")
		return true
	}

	// Slots 2+ do not have dedicated templates yet. Never treat a coordinate
	// tap alone as success: first prove we are still on an army menu, then
	// require a local visual change around the selected recipe card.
	before, err := b.client.CaptureToMat()
	if err != nil || before.Empty() {
		if !before.Empty() { before.Close() }
		b.logger.Warn().Err(err).Int("army_slot", slot).Msg("cannot verify army recipe before selection")
		return false
	}
	stateBefore, _ := b.classify(before)
	if stateBefore != game.StateArmySelection && stateBefore != game.StateArmyCamp {
		before.Close()
		b.logger.Warn().Str("state", stateBefore.String()).Int("army_slot", slot).Msg("refusing army recipe tap outside verified army menu")
		return false
	}

	cardY := 227 + (slot-1)*54
	if cardY < 180 || cardY > 590 {
		before.Close()
		b.logger.Warn().Int("army_slot", slot).Int("card_y", cardY).Msg("army recipe slot outside safe visible card range")
		return false
	}
	tapX, tapY := b.cal.ScaleRef(430, cardY)

	b.logger.Info().Int("army_slot", slot).Int("x", tapX).Int("y", tapY).Msg("selecting saved army recipe card with visual verification")
	if err := b.client.TapFast(tapX, tapY, 0.7); err != nil {
		before.Close()
		b.logger.Warn().Err(err).Msg("army recipe card tap failed")
		return false
	}
	if !b.sleepResponsive(180 * time.Millisecond) {
		before.Close()
		return false
	}

	after, capErr := b.client.CaptureToMat()
	if capErr != nil || after.Empty() {
		before.Close()
		if !after.Empty() { after.Close() }
		b.logger.Warn().Err(capErr).Int("army_slot", slot).Msg("army recipe selection could not be visually confirmed")
		return false
	}

	delta := localVisualDelta(before, after, image.Pt(tapX, tapY), int(70*b.cal.ScaleX), int(30*b.cal.ScaleY))
	stateAfter, _ := b.classify(after)
	before.Close()
	after.Close()

	// Either the card/toast changed materially, or the menu transitioned to a
	// different known army state. Anything else is an unproven tap.
	verified := delta >= 0.012 || stateAfter != stateBefore
	if !verified {
		b.logger.Warn().
			Int("army_slot", slot).
			Float64("visual_delta", delta).
			Str("state", stateAfter.String()).
			Msg("army recipe tap produced no verified UI progress")
		return false
	}

	b.recordActivity()
	b.logger.Info().Int("army_slot", slot).Float64("visual_delta", delta).Msg("army recipe selection verified")
	return true
}

// Pinpoint defines a precise location on the reference screen (860x732)
// and a color check to verify it before clicking.
type Pinpoint struct {
	X, Y int
	Name string
}

var villagePinpoints = map[string]Pinpoint{
	"btn_attack":      {X: 64, Y: 666, Name: "Attack"},
	"btn_find_match":  {X: 158, Y: 494, Name: "Find Match"},
	"btn_battle":      {X: 731, Y: 537, Name: "Battle"},
	"btn_army_arrow":  {X: 514, Y: 192, Name: "Army Arrow"},
	"btn_army_1":      {X: 513, Y: 230, Name: "Army 1"},
	"btn_next":        {X: 794, Y: 577, Name: "Next Match"},
	"btn_return_home": {X: 431, Y: 581, Name: "Return Home"},
	"btn_okay":        {X: 430, Y: 520, Name: "Okay"},
}

func (b *Bot) findAndClick(templateName, stepName string, maxRetries int) bool {
	// Never treat a hard-coded coordinate as a successful match. On Windows
	// the old fast path tapped the reference coordinate unconditionally and
	// returned true even when the expected screen was not visible. That made
	// clickSequence advance through Attack -> Find Match -> Army -> Battle on
	// the village screen and then falsely report "searching".
	//
	// Coordinates remain useful only as a last-resort diagnostic reference;
	// normal progression must be backed by an actual template/color match.
	tpl, ok := b.templates.Get(templateName)
	if !ok {
		b.logger.Error().Str("template", templateName).Msg("template not loaded")
		return false
	}

	roi := b.buttonROI(templateName)

	physROI := image.Rect(
		int(float64(roi.Min.X)*b.cal.ScaleX),
		int(float64(roi.Min.Y)*b.cal.ScaleY),
		int(float64(roi.Max.X)*b.cal.ScaleX),
		int(float64(roi.Max.Y)*b.cal.ScaleY),
	)

	for retry := 0; retry < maxRetries; retry++ {
		screen, err := b.client.CaptureToMat()
		if err != nil {
			b.logger.Warn().Err(err).Str("step", stepName).Msg("capture failed")
			if !b.sleepResponsive(250 * time.Millisecond) { return false }
			continue
		}

		if screen.Empty() {
			screen.Close()
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if templateName == "btn_battle" && retry == 0 {
			altX, altY := b.cal.ScaleRef(525, 247)
			if b.isGreen(screen, altX, altY) {
				screen.Close()
				b.logger.Info().Str("step", stepName).Msg("secondary pinpoint match (upper battle), clicking...")
				if err := b.client.TapFast(altX, altY, 0.6); err == nil {
					b.recordActivity()
					return true
				}
				var recaptureErr error
				screen, recaptureErr = b.client.CaptureToMat()
				if recaptureErr != nil || screen.Empty() {
					if !screen.Empty() { screen.Close() }
					b.logger.Warn().Err(recaptureErr).Str("step", stepName).Msg("battle fallback recapture failed")
					continue
				}
			}
		}

		threshold := float32(0.45)
		if templateName == "btn_attack" {
			threshold = 0.35
		}
		matches, err := vision.MatchMultiScaleROICached(screen, tpl, templateName, 0.2, 2.0, 5, threshold, physROI)

		if err != nil {
			screen.Close()
			b.logger.Warn().Err(err).Str("step", stepName).Msg("match error")
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if len(matches) == 0 {
			screen.Close()
			if retry == 0 {
				b.logger.Debug().Str("step", stepName).Msg("not found, retrying...")
			}
			b.dismissInterruptions()
			if !b.sleepResponsive(350 * time.Millisecond) { return false }
			continue
		}

		best := matches[0]
		px, py := best.Point.X, best.Point.Y

		if templateName == "btn_attack" {
			expectedX, expectedY := b.cal.ScaleRef(64, 666)
			dx := px - expectedX
			if dx < 0 {
				dx = -dx
			}
			dy := py - expectedY
			if dy < 0 {
				dy = -dy
			}
			if dx > 80 || dy > 60 {
				screen.Close()
				b.logger.Warn().
					Float64("conf", best.Confidence).
					Int("match_x", px).
					Int("match_y", py).
					Int("expected_x", expectedX).
					Int("expected_y", expectedY).
					Msg("rejected false Attack match outside safe button area")
				continue
			}

			// Once the template confirms the Attack button is present in its
			// tightly constrained ROI, tap the calibrated canonical center.
			// This prevents an imperfect template center from hitting a
			// neighboring HUD control.
			px, py = expectedX, expectedY
		}

		b.logger.Info().
			Str("step", stepName).
			Float64("conf", best.Confidence).
			Int("x", px).Int("y", py).
			Msg("clicking verified button")

		// IMPORTANT: SaveScreenshots previously called IMWrite after
		// screen.Close(), handing OpenCV a freed native cv::Mat*. On Windows
		// that is a process-level access violation (0xc0000005), which exactly
		// matched the crash immediately after "clicking (fallback match)".
		// Keep the Mat alive through the optional diagnostic write, then close.
		if b.cfg.Debug.SaveScreenshots {
			gocv.IMWrite(paths.ResolveConfig(fmt.Sprintf("diag_fallback_%s.png", templateName)), screen)
		}
		screen.Close()

		if err := b.client.TapFast(px, py, 0.7); err != nil {
			b.logger.Error().Err(err).Msg("tap failed")
			return false
		}
		b.recordActivity()

		return true
	}

	if pp, ok := villagePinpoints[templateName]; ok {
		px, py := b.cal.ScaleRef(pp.X, pp.Y)
		b.logger.Warn().
			Str("step", pp.Name).
			Int("reference_x", px).
			Int("reference_y", py).
			Msg("template/color verification failed; refusing blind tap")
	}

	b.logger.Error().Str("step", stepName).Int("retries", maxRetries).Msg("failed after retries")
	return false
}

// resultPanelHash returns a cheap content hash of the end-of-battle
// result panel region (star row + battle-loot and bonus columns). Two
// captures with an equal nonzero hash mean the panel has finished
// rendering — used to distinguish a still-counting-up overlay from a
// genuine 0-star/0-loot result.
func resultPanelHash(screen gocv.Mat, cal *game.Calibration) uint64 {
	// Reference (860x732) region covering the stars and both loot
	// columns; generous bounds tolerate small theme shifts.
	ref := image.Rect(300, 180, 690, 470)
	x0 := int(float64(ref.Min.X) * cal.ScaleX)
	y0 := int(float64(ref.Min.Y) * cal.ScaleY)
	x1 := int(float64(ref.Max.X) * cal.ScaleX)
	y1 := int(float64(ref.Max.Y) * cal.ScaleY)
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > screen.Cols() {
		x1 = screen.Cols()
	}
	if y1 > screen.Rows() {
		y1 = screen.Rows()
	}
	if x1-x0 < 2 || y1-y0 < 2 {
		return 0
	}

	var h uint64 = 14695981039346656037 // FNV-1a offset basis
	stride := 4                         // sample every 4th pixel — plenty for frame-diff detection
	for y := y0; y < y1; y += stride {
		for x := x0; x < x1; x += stride {
			b := uint64(screen.GetUCharAt(y, x*3))
			g := uint64(screen.GetUCharAt(y, x*3+1))
			r := uint64(screen.GetUCharAt(y, x*3+2))
			h ^= (r << 16) | (g << 8) | b
			h *= 1099511628211
		}
	}
	return h
}

func (b *Bot) isGreen(screen gocv.Mat, x, y int) bool {
	return b.colorCheck(screen, x, y,
		gocv.NewScalar(0, 150, 0, 0),
		gocv.NewScalar(120, 255, 120, 0),
		15)
}

func (b *Bot) colorCheck(screen gocv.Mat, x, y int, lower, upper gocv.Scalar, minPixels int) bool {
	if x < 0 || y < 0 || x >= screen.Cols() || y >= screen.Rows() {
		return false
	}
	region := image.Rect(x-10, y-10, x+11, y+11)
	if region.Min.X < 0 {
		region.Min.X = 0
	}
	if region.Min.Y < 0 {
		region.Min.Y = 0
	}
	if region.Max.X > screen.Cols() {
		region.Max.X = screen.Cols()
	}
	if region.Max.Y > screen.Rows() {
		region.Max.Y = screen.Rows()
	}

	sub := screen.Region(region)
	defer sub.Close()

	mask := vision.GetMat(sub.Rows(), sub.Cols(), gocv.MatTypeCV8UC1)
	defer vision.PutMat(mask)
	gocv.InRangeWithScalar(sub, lower, upper, &mask)

	return gocv.CountNonZero(mask) > minPixels
}

// dismissInterruptions taps its way through transient overlays. Note that
// these direct taps intentionally DO NOT call recordActivity() — if a dialog
// is truly stuck and refusing to dismiss, we want the global stuck watchdog
// to fire and cycle the game rather than mask the hang as forward progress.
//
// If the dismiss is actually working, the resulting state transition (caught
// by the captureLoop's next frame) will reset lastAction via recordActivity()
// on its own.
func (b *Bot) dismissInterruptions() {
	screen, err := b.client.CaptureToMat()
	if err != nil {
		return
	}
	state, _ := b.classify(screen)
	screen.Close()

	switch state {
	case game.StateObstacleDialog:
		b.client.TapRandomized(400, 300)
		time.Sleep(400 * time.Millisecond)
		b.client.Back()
	case game.StateGemDialog, game.StateShieldInfo:
		b.client.TapRandomized(175, 30)
	case game.StateWelcomeBack:

		ox, oy := b.cal.ScaleRef(430, 520)
		b.client.TapRandomized(ox, oy)
	case game.StateChatOpen:
		b.client.Back()
	case game.StateTapToContinue:
		// Post-boot "ТАР!" collect splash — tap the prompt text.
		px, py := b.cal.ScaleRef(450, 195)
		b.client.TapRandomized(px, py)
	case game.StateNewsSplash:
		// Post-boot news splash — tap the green Continue button.
		px, py := b.cal.ScaleRef(403, 535)
		b.client.TapRandomized(px, py)
	case game.StateConnectionLost:
		// Connection-lost dialog — tap TRY AGAIN to reconnect in place.
		px, py := b.cal.ScaleRef(300, 478)
		b.client.TapRandomized(px, py)
	case game.StateConfirmExit:
		// Quit-confirm dialog — tap Cancel to stay in the game.
		px, py := b.cal.ScaleRef(279, 429)
		b.client.TapRandomized(px, py)
	}
}

// dismissSelection taps in the background/empty space to close any active
// selection menus. Used as the production Dismiss hook for the wall-upgrade
// loop in RunWallUpgradeLoop. The 500ms settle waits for the menu
// close-animation; without it the next capture can race the menu's
// fade-out and confuse the next template match.
func (b *Bot) dismissSelection() {
	tx, ty := b.cal.ScaleRef(50, 450)
	_ = b.client.Tap(tx, ty)
	time.Sleep(500 * time.Millisecond)
}

func (b *Bot) waitForBattleState(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		screen, err := b.client.CaptureToMat()
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		state, _ := b.classify(screen)
		screen.Close()

		switch {
		case state == game.StateBattle:
			b.logger.Info().Msg("battle state detected, entering search loop")
			return true
		case state == game.StateSearchMap || state == game.StateLoading:
			b.logger.Debug().Msg("in clouds/loading...")
			if !b.sleepResponsive(250 * time.Millisecond) { return false }
			continue
		case state == game.StateArmySelection || state == game.StateArmyCamp:
			b.logger.Info().Msg("in army menu, retrying Battle Attack button...")
			if retryScreen, capErr := b.client.CaptureToMat(); capErr == nil {
				if x, y, ok := b.locateBattleButtonColor(retryScreen); ok {
					retryScreen.Close()
					b.logger.Info().Int("x", x).Int("y", y).Msg("retrying with detected Battle Attack button center")
					_ = b.client.TapFast(x, y, 0.6)
					b.recordActivity()
				} else {
					retryScreen.Close()
					b.findAndClick("btn_battle", "Battle Retry", 1)
				}
			}
			if !b.sleepResponsive(250 * time.Millisecond) { return false }
		default:
			b.logger.Debug().Str("state", state.String()).Msg("waiting for battle/search state...")
			b.dismissInterruptions()
			if !b.sleepResponsive(180 * time.Millisecond) { return false }
		}
	}

	b.logger.Warn().Dur("timeout", timeout).Msg("timed out waiting for battle")
	return false
}

func (b *Bot) deployTroops(screen gocv.Mat) (int, error) {
	strat, err := strategy.ParseYAML(b.cfg.Attack.StrategyFile)
	if err != nil {
		b.logger.Warn().Err(err).Str("path", b.cfg.Attack.StrategyFile).Msg("could not load strategy")
		return 0, err
	}

	b.logger.Info().
		Str("strategy", strat.Name).
		Int("phases", len(strat.Phases)).
		Msg("executing dynamic attack plan")

	if !b.sleepResponsive(100 * time.Millisecond) { return 0, b.ctx.Err() }

	remaining, err := b.attackExec.DeployDynamicV2(strat, screen, b.cfg.Attack.StrategyFile)
	if err != nil {
		b.logger.Error().Err(err).Msg("dynamic deploy failed")
		return remaining, err
	}
	return remaining, nil
}

// QuickDeploy is the manual single-shot deploy path for run_designed_attack.sh.
// The user is assumed to already be on the attack screen with a base loaded —
// there's no search loop, no attack-button discovery, no home → finds-match
// pipeline. We capture the current screen once, run deployTroops (which honors
// formula.json overrides if design_attack wrote one next to the strategy), and
// return.
//
// The captureLoop is intentionally NOT started; cli.go's --deploy-only branch
// calls this directly and bypasses b.Start() to avoid the Find-Attack-Button
// race that would otherwise kick the bot into the next-base search cycle.
//
// On non-Battle screens (e.g. user is still on home, or in clouds), we log a
// WARN and proceed anyway — the deployTroops path will see the absence of a
// troop bar and either error out cleanly or succeed-by-luck if the screen does
// actually contain a deployable base. Caller should surface the error.
func (b *Bot) QuickDeploy() error {
	b.logger.Info().Msg("deploy-only mode: capturing current screen once")

	screen, err := b.client.CaptureToMat()
	if err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	defer screen.Close()

	if screen.Empty() {
		return fmt.Errorf("captured screen is empty; device disconnected?")
	}

	state, _ := b.classify(screen)
	b.logger.Info().
		Str("state", state.String()).
		Int("w", screen.Cols()).
		Int("h", screen.Rows()).
		Msg("starting deploy from current screen")

	if state != game.StateBattle {
		b.logger.Error().
			Str("state", state.String()).
			Msg("refusing to deploy: screen is not in Battle state; wait for clouds/loading to clear or navigate to a base on the attack screen, then re-run")
		return fmt.Errorf("screen is in state %s; expected Battle — wait for the attack screen to be ready, then re-run", state.String())
	}

	remaining, deployErr := b.deployTroops(screen)
	if deployErr != nil {

		b.logger.Error().Err(deployErr).Int("undeployed", remaining).Msg("deploy failed")
		return fmt.Errorf("deployTroops: %w (undeployed=%d)", deployErr, remaining)
	}
	if remaining > 0 {
		b.logger.Warn().Int("undeployed", remaining).Msg("deploy finished with undeployed slots")
		return fmt.Errorf("deploy completed with %d undeployed slots", remaining)
	}
	b.logger.Info().Msg("deploy completed cleanly (0 undeployed)")
	return nil
}

func (b *Bot) Health() game.SystemHealth {
	return game.SystemHealth{
		ADBConnected:     b.client.IsConnected(),
		LastCapture:      b.lastCapture,
		AvgCaptureMs:     b.client.Health().AvgCaptureMs,
		ConsecutiveFails: b.client.Health().ConsecutiveFails,
		CPUTimeSec:       CPUTime().Seconds(),
		CPUCores:          b.cpuSampler.Usage(),
	}
}

func (b *Bot) UpdateConfig(cfg *config.BotConfig) {
	b.cfg = cfg
	if b.attackExec != nil {
		b.attackExec.UpdateConfig(&cfg.Attack)
	}

	if b.navigator != nil {
		b.navigator.SetDisableChestDismissal(cfg.Device.DisableChestDismissal)
	}
	b.logger.Info().Msg("bot configuration updated in real-time")
}

func (b *Bot) Stats() BotStats {
	now := time.Now()
	b.statusMu.RLock()
	trainingPending := append([]attack.TrainingPlanItem(nil), b.trainingPending...)
	villageReason := b.villageReason
	villageNextAt := b.villageNextAt
	lastDonationResult := b.lastDonationResult
	b.statusMu.RUnlock()
	state := game.GameState(b.runtimeState.Load())
	phase := RuntimePhase(b.runtimePhase.Load())
	stateAge := time.Duration(0)
	if n := b.runtimeStateSince.Load(); n > 0 {
		stateAge = now.Sub(time.Unix(0, n))
	}
	progressAge := time.Duration(0)
	if n := b.runtimeProgress.Load(); n > 0 {
		progressAge = now.Sub(time.Unix(0, n))
	}

	return BotStats{
		AttacksCompleted: b.attackCount.Load(),
		SearchSkips:      b.skipsCount.Load(),
		TotalGold:        b.totalGold.Load(),
		TotalElixir:      b.totalElixir.Load(),
		TotalDE:          b.totalDE.Load(),
		Stars0:           b.stars0.Load(),
		Stars1:           b.stars1.Load(),
		Stars2:           b.stars2.Load(),
		Stars3:           b.stars3.Load(),
		Uptime:           time.Since(b.startedAt),
		AdbHealth:          b.client.Health(),
		CPUTimeSec:         CPUTime().Seconds(),
		CPUCores:           b.cpuSampler.Usage(),
		RecoveryAttempts:   b.recoveryAttempts.Load(),
		RecoverySuccesses:  b.recoverySuccesses.Load(),
		BlueStacksRestarts: b.blueStacksRestarts.Load(),
		DonationChecks:     b.donationChecks.Load(),
		DonationsSent:      b.donationsSent.Load(),
		LastDonationUnix:      b.lastDonationUnix.Load(),
		LastDonationResult:    lastDonationResult,
		TrainingItemsPending:   b.trainingItemsPending.Load(),
		TrainingHousingPending: b.trainingHousingPending.Load(),
		TrainingPlanUncertain:  b.trainingPlanUncertain.Load(),
		ArmyRepairAttempts:     b.armyRepairAttempts.Load(),
		ArmyRepairSuccesses:    b.armyRepairSuccesses.Load(),
		TrainingPending:        trainingPending,
		VillageAction:         VillageAction(b.villageAction.Load()).String(),
		VillageReason:         villageReason,
		VillageNextUnix:       villageNextAt.Unix(),
		RuntimeState:          state.String(),
		RuntimePhase:       phase.String(),
		RuntimeStateAge:    stateAge,
		RuntimePhaseAge:    b.runtimePhaseAge(now),
		LastProgressAgo:    progressAge,
	}
}

type BotStats struct {
	AttacksCompleted int32         `json:"attacks_completed"`
	SearchSkips      int32         `json:"search_skips"`
	TotalGold        int64         `json:"total_gold"`
	TotalElixir      int64         `json:"total_elixir"`
	TotalDE          int64         `json:"total_de"`
	Stars0           int32         `json:"stars_0"`
	Stars1           int32         `json:"stars_1"`
	Stars2           int32         `json:"stars_2"`
	Stars3           int32         `json:"stars_3"`
	Uptime           time.Duration `json:"uptime"`
	AdbHealth        adb.Health    `json:"adb_health"`

	CPUTimeSec float64 `json:"cpu_time_sec"`

	CPUCores float64 `json:"cpu_cores"`

	RecoveryAttempts   int32 `json:"recovery_attempts"`
	RecoverySuccesses  int32 `json:"recovery_successes"`
	BlueStacksRestarts int32 `json:"bluestacks_restarts"`
	DonationChecks     int32 `json:"donation_checks"`
	DonationsSent      int32 `json:"donations_sent"`
	LastDonationUnix      int64  `json:"last_donation_unix"`
	LastDonationResult    string `json:"last_donation_result"`
	TrainingItemsPending   int32  `json:"training_items_pending"`
	TrainingHousingPending int32  `json:"training_housing_pending"`
	TrainingPlanUncertain  bool                      `json:"training_plan_uncertain"`
	ArmyRepairAttempts     int32                     `json:"army_repair_attempts"`
	ArmyRepairSuccesses    int32                     `json:"army_repair_successes"`
	TrainingPending        []attack.TrainingPlanItem `json:"training_pending"`
	VillageAction          string                    `json:"village_action"`
	VillageReason          string                    `json:"village_reason"`
	VillageNextUnix        int64                     `json:"village_next_unix"`

	RuntimeState    string        `json:"runtime_state"`
	RuntimePhase    string        `json:"runtime_phase"`
	RuntimeStateAge time.Duration `json:"runtime_state_age"`
	RuntimePhaseAge time.Duration `json:"runtime_phase_age"`
	LastProgressAgo time.Duration `json:"last_progress_ago"`
}

type AttackReport struct {
	Timestamp        string `json:"timestamp"`
	Strategy         string `json:"strategy"`
	TargetEdge       string `json:"target_edge"`
	DeploySuccess    bool   `json:"deploy_success"`
	UndeployedSlots  int    `json:"undeployed_slots"`
	DeployError      string `json:"deploy_error,omitempty"`
	ParsedResults    bool   `json:"parsed_results"`
	Stars            int    `json:"stars"`
	GoldStolen       int    `json:"gold_stolen"`
	ElixirStolen     int    `json:"elixir_stolen"`
	DarkElixirStolen int    `json:"dark_elixir_stolen"`
	BonusGold        int    `json:"bonus_gold"`
	BonusElixir      int    `json:"bonus_elixir"`
	BonusDE          int    `json:"bonus_de"`
	TotalAttacks     int32  `json:"total_attacks_session"`
}

type adbLogAdapter struct {
	log zerolog.Logger
}

func (a *adbLogAdapter) Debug() bool { return a.log.GetLevel() <= zerolog.DebugLevel }
func (a *adbLogAdapter) Debugf(format string, v ...any) {
	a.log.Debug().Msgf(format, v...)
}
func (a *adbLogAdapter) Info(msg string)  { a.log.Info().Msg(msg) }
func (a *adbLogAdapter) Warn(msg string)  { a.log.Warn().Msg(msg) }
func (a *adbLogAdapter) Error(msg string) { a.log.Error().Msg(msg) }
func (a *adbLogAdapter) WithFields(fields map[string]any) adb.Logger {
	return &adbLogAdapter{log: a.log.With().Fields(fields).Logger()}
}

func init() {
	runtime.GOMAXPROCS(0)
}


func shouldForceAttackAfterSkips(limit, skips int) bool {
	return limit > 0 && skips >= limit
}
