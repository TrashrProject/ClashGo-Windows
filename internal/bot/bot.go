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
	autopolicy "github.com/Ducky705/ClashGO/internal/automation"
	"github.com/Ducky705/ClashGO/internal/config"
	"github.com/Ducky705/ClashGO/internal/game"
	"github.com/Ducky705/ClashGO/internal/intelligence"
	"github.com/Ducky705/ClashGO/internal/multiaccount"
	"github.com/Ducky705/ClashGO/internal/paths"
	"github.com/Ducky705/ClashGO/internal/telemetry"
	"github.com/Ducky705/ClashGO/internal/vision"
	"github.com/Ducky705/ClashGO/pkg/strategy"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type PreparationTimings struct {
	AttackButtonMS      int64
	FindMatchMS         int64
	ArmyMenuMS          int64
	ArmySlotMS          int64
	BattleButtonMS      int64
	MatchmakingReadyMS  int64
}

type Bot struct {
	client     *adb.Client
	cal        *game.Calibration
	classifier *game.Classifier
	navigator  *game.Navigator
	graph      *game.StateGraph
	templates  *game.TemplateStore
	recognizer     *game.Recognizer
	resourceReader *game.VillageResourceReader
	searchLootRec  *game.LootRecognizer
	cfg            *config.BotConfig

	classify func(gocv.Mat) (game.GameState, int)

	attackExec *attack.Executor
	governor   *autopolicy.Governor
	adaptive       *intelligence.AdaptiveEngine
	contextual     *intelligence.ContextualEngine
	villageMemory  *intelligence.VillageMemory
	multiAccount   *multiaccount.Manager

	ctx         context.Context
	cancel      context.CancelFunc
	done        chan struct{}
	captureDone chan struct{}
	frameBroker *FrameBroker
	brokerActive atomic.Bool
	frameSeq     atomic.Uint64
	logger      zerolog.Logger

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
	seqStartedAtUnix  atomic.Int64
	paused            atomic.Bool
	zoomedOut         atomic.Bool
	recoveryAttempts  atomic.Int32
	recoverySuccesses atomic.Int32
	blueStacksRestarts atomic.Int32
	cleanAttackStreak   atomic.Int32
	soakValidated       atomic.Bool
	returnHomeCount     atomic.Int64
	returnHomeMicros    atomic.Int64
	lastReturnHomeUS    atomic.Int64
	safePacingUntilUS   atomic.Int64
	wallUpgradePending  atomic.Bool

	// Xingchen-style runtime supervision: independent heartbeat, phase/state
	// tracking, and single-flight recovery/restart guards.
	captureHeartbeat atomic.Int64
	runtimeState atomic.Int32
	runtimeStateSince atomic.Int64
	runtimeProgress atomic.Int64
	runtimePhase atomic.Int32
	runtimePhaseSince atomic.Int64
	recoveryInFlight atomic.Bool
	restartInFlight atomic.Bool

	chestDismissInFlight  atomic.Bool
	rewardDismissInFlight atomic.Bool
	splashDismissInFlight atomic.Bool
	connLostDismissInFlight atomic.Bool
	lastArmyCampGuardLog    time.Time
	startedAt             time.Time
	watchdogMu           sync.RWMutex
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

	historyMu    sync.RWMutex
	historyCache []AttackReport
	telemetry    *telemetry.Bus
	lastPrepTimings PreparationTimings
	lastPlanningUS  atomic.Int64

	uiAnchorMu        sync.RWMutex
	uiAnchors         map[string]image.Point
	uiAnchorAttempts        atomic.Int64
	uiAnchorHits            atomic.Int64
	uiAnchorFallbacks       atomic.Int64
	uiAnchorDisabledUntilUS atomic.Int64

	diagMu          sync.Mutex
	lastDiagnostics map[string]time.Time

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
	vision.ResetPreferredScaleStats()

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
	attackExec.SetArmyGuardEnabled(cfg.Automation.AutoArmyGuard)

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
	searchLootRec := game.NewLootRecognizer(cal, templates, log.Logger)
	attackExec.SetLootRecognizer(searchLootRec)

	b = &Bot{
		client:            client,
		cal:               cal,
		graph:             graph,
		templates:         templates,
		recognizer:        recognizer,
		resourceReader:    resourceReader,
		searchLootRec:     searchLootRec,
		cfg:               cfg,
		attackExec:        attackExec,
		governor:          autopolicy.NewGovernor(cfg.Automation),
		ctx:               ctx,
		cancel:            cancel,
		done:              make(chan struct{}),
		captureDone:       make(chan struct{}),
		frameBroker:       NewFrameBroker(),
		logger:            log.With().Str("bot", "orchestrator").Logger(),
		startedAt:         startedWall,
		lastAction:        time.Now(),
		lastSequenceStart: time.Now(),
		lastIdlePan:       time.Now(),
		stuckTimeout:      35 * time.Second,
		cpuSampler:        newCPUSampler(),
		dukePicksFile:     dukePicksFile,
		telemetry:          telemetry.New(paths.ResolveConfig("telemetry/events.ndjson")),
		lastDiagnostics:    make(map[string]time.Time),
		uiAnchors:          make(map[string]image.Point),
	}

	multiMgr, multiErr := multiaccount.NewManager(
		multiAccountStatePath(cfg),
		cfg.Account.MultiAccount,
		cfg.Account.PlayerTag,
	)
	if multiErr != nil {
		b.logger.Warn().Err(multiErr).Msg("multi-account scheduler unavailable; continuing single-account")
	} else {
		b.multiAccount = multiMgr
		if active, ok := multiMgr.Active(); ok {
			b.logger.Info().
				Bool("enabled", multiMgr.Enabled()).
				Str("account_id", active.ID).
				Str("account_label", active.Label).
				Msg("multi-account scheduler initialized")
		}
	}

	emulatorKind := "adb"
	if cfg.Device.BlueStacksInstance != "" || strings.Contains(strings.ToLower(cfg.Device.DeviceID), "localhost") {
		emulatorKind = "bluestacks"
	}
	adaptive, adaptiveErr := intelligence.NewAdaptiveEngine(
		learningAccountStatePath(cfg, "adaptive_learning.json"),
		intelligence.EnvironmentFingerprint{
			OS: runtime.GOOS,
			Emulator: emulatorKind,
			DeviceID: cfg.Device.DeviceID,
			Width: cfg.Device.Width,
			Height: cfg.Device.Height,
			DPI: cfg.Device.DPI,
			Strategy: filepath.Base(cfg.Attack.StrategyFile),
			TownHall: cfg.Attack.Farm.TownHall,
			AccountScope: learningScopeKey(cfg),
		},
	)
	if adaptiveErr != nil {
		b.logger.Warn().Err(adaptiveErr).Msg("adaptive intelligence unavailable; continuing without learning")
	} else {
		b.adaptive = adaptive
		b.logger.Info().
			Str("mode", string(adaptive.Mode())).
			Msg("adaptive intelligence active")
	}

	contextual, contextualErr := intelligence.NewContextualEngine(
		learningAccountStatePath(cfg, "contextual_learning_v3.json"),
	)
	if contextualErr != nil {
		b.logger.Warn().Err(contextualErr).Msg("Intelligence V3 contextual learning unavailable; continuing with legacy adaptive engine")
	} else {
		b.contextual = contextual
		b.logger.Info().
			Int("experiences", contextual.TotalSamples()).
			Msg("Intelligence V3 contextual learning active")
	}

	villageMemory, villageErr := intelligence.NewVillageMemory(learningEnvironmentStatePath(cfg, "village_model.json"))
	if villageErr != nil {
		b.logger.Warn().Err(villageErr).Msg("village memory unavailable; continuing without persistent village model")
	} else {
		b.villageMemory = villageMemory
		b.logger.Info().
			Int("known_entities", len(villageMemory.Snapshot().Entities)).
			Msg("persistent village memory loaded")
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
			// Bootstrap V3 from the history ClashGO already collected. Only do
			// this for an empty V3 state so restarting the app never double-counts
			// the same historical attacks.
			if b.contextual != nil && b.contextual.TotalSamples() == 0 && len(seeded) > 0 {
				limit := len(seeded)
				if limit > 200 {
					limit = 200
				}
				outcomes := make([]intelligence.ContextualOutcome, 0, limit)
				// History is newest-first. Feed oldest-first so EWMA ends weighted
				// toward the most recent real attacks.
				for i := limit - 1; i >= 0; i-- {
					outcomes = append(outcomes, contextualOutcomeFromReport(seeded[i], cfg.Attack.Farm.TownHall, 0, 0))
				}
				if err := b.contextual.ObserveMany(outcomes); err != nil {
					b.logger.Warn().Err(err).Msg("Intelligence V3 history bootstrap failed")
				} else {
					b.logger.Info().Int("replayed_attacks", len(outcomes)).Msg("Intelligence V3 learned from existing attack history")
				}
			}
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
	b.attackExec.SetFrameProvider(b.runtimeFrameFresh)
	b.attackExec.OnPlanReady = func(duration time.Duration, edge string) {
		b.lastPlanningUS.Store(duration.Microseconds())
		b.logger.Info().Dur("planning", duration).Str("edge", edge).Msg("attack plan committed; starting deployment")
		b.setRuntimePhase(PhaseDeploying)
	}

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
	b.logger.Debug().Int("x", focusX).Int("y", focusY).Msg("performing initial focus click")
	b.client.Tap(focusX, focusY)
	b.client.JitteredSleep(250 * time.Millisecond)

	now := time.Now()
	b.captureHeartbeat.Store(now.UnixNano())
	b.runtimeState.Store(int32(game.StateUnknown))
	b.runtimeStateSince.Store(now.UnixNano())
	b.runtimeProgress.Store(now.UnixNano())
	b.runtimePhase.Store(int32(PhaseIdle))
	b.runtimePhaseSince.Store(now.UnixNano())

	b.brokerActive.Store(true)
	go func() {
		defer close(b.captureDone)
		defer b.brokerActive.Store(false)
		b.captureLoop()
	}()
	go b.runtimeSupervisorLoop()
	return nil
}

func (b *Bot) Stop() {
	b.cancel()
	if b.telemetry != nil {
		b.telemetry.Close()
	}

	// Cut ADB first so no further taps/captures can leave the process after
	// Cancel. Wait for the capture loop and active sequence to leave their
	// OpenCV code before releasing native matrices.
	b.client.Close()

	select {
	case <-b.captureDone:
	case <-time.After(3 * time.Second):
		b.logger.Warn().Msg("capture loop did not stop within teardown window; keeping shared templates alive")
	}

	deadline := time.Now().Add(3 * time.Second)
	for b.seqRunning.Load() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	captureStopped := false
	select {
	case <-b.captureDone:
		captureStopped = true
	default:
	}
	if captureStopped && b.frameBroker != nil {
		b.frameBroker.Close()
	}

	// Persist a compact human-readable session summary only after the active
	// sequence has had a chance to finish updating its final attack report.
	// This is off the farming hot path and never delays taps/captures.
	if !b.seqRunning.Load() {
		report := b.CurrentSessionReport()
		if report.Attacks > 0 {
			if err := saveSessionReport(report); err != nil {
				b.logger.Warn().Err(err).Msg("failed to persist session report")
			} else {
				b.logger.Info().
					Int("attacks", report.Attacks).
					Float64("gold_per_hour", report.GoldPerHour).
					Float64("zero_touch_rate", report.ZeroTouchRate).
					Str("bottleneck", report.Bottleneck).
					Msg("session report saved")
			}
		}
	}

	if !b.seqRunning.Load() && captureStopped {
		if b.attackExec != nil {
			b.attackExec.Close()
		}
		if b.resourceReader != nil {
			b.resourceReader.Close()
		}
		if b.searchLootRec != nil {
			b.searchLootRec.Close()
		}
		if b.templates != nil {
			b.templates.Close()
		}
		vision.CloseTemplateCache()
	} else {
		// Safety wins over eager cleanup: if any goroutine may still be inside
		// CGO/OpenCV, let the process/GC reclaim at final exit rather than close
		// a Mat underneath active native code.
		b.logger.Warn().
			Bool("sequence_running", b.seqRunning.Load()).
			Bool("capture_stopped", captureStopped).
			Msg("native template cleanup deferred to process exit")
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
		// captureLoop is the ONLY runtime screenshot owner. Attack/search code
		// consumes broker frames, so cadence can be tuned per phase without
		// ever creating a second ADB screencap stream.
		if b.seqRunning.Load() {
			switch RuntimePhase(b.runtimePhase.Load()) {
			case PhaseAttackNavigation:
				return 220 * time.Millisecond
			case PhaseSearching:
				return 700 * time.Millisecond
			case PhasePlanning:
				// Planning is intentionally fast: one broker stream at ~4 FPS is
				// enough for zoom/red-zone/bar analysis without recreating the old
				// concurrent screencap pressure.
				return 250 * time.Millisecond
			case PhaseDeploying:
				return 350 * time.Millisecond
			case PhaseBattle, PhaseParsingResult, PhaseReturningHome:
				return 650 * time.Millisecond
			default:
				return 500 * time.Millisecond
			}
		}
		switch gc.State {
		case game.StateMainVillage, game.StateArmySelection, game.StateArmyCamp:
			return 250 * time.Millisecond
		case game.StateBattle, game.StateSearchMap, game.StateLoading:
			return 500 * time.Millisecond
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
			if b.telemetry != nil {
				b.telemetry.RecordCaptureMicros(dur.Microseconds())
			}
			lastCapture = time.Now()
			b.lastCapture = lastCapture
			if err == nil && !screen.Empty() && screen.Cols() >= 2 && screen.Rows() >= 2 {
				b.captureHeartbeat.Store(lastCapture.UnixNano())
				if b.frameBroker != nil {
					b.frameBroker.Publish(screen, lastCapture)
				}
			}

			if err != nil || screen.Empty() || screen.Cols() < 2 || screen.Rows() < 2 {
				screen.Close()
				b.logger.Debug().Err(err).Msg("empty/degenerate capture dropped")
				continue
			}

			if b.seqRunning.Load() {
				// FrameBroker already owns a clone of this frame. During an active
				// attack, do not enqueue the same Mat into the idle UI processor.
				screen.Close()
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
			// During an active attack, the state machine owns UI decisions. The
			// capture loop still feeds FrameBroker, but it must not dismiss/tap
			// anything in parallel.
			if b.seqRunning.Load() {
				if !f.mat.Empty() {
					f.mat.Close()
				}
				continue
			}
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
	b.watchdogMu.Lock()
	b.lastAction = now
	b.watchdogMu.Unlock()
	b.runtimeProgress.Store(now.UnixNano())
}

func (b *Bot) recordSequenceStart() {
	b.watchdogMu.Lock()
	b.lastSequenceStart = time.Now()
	b.watchdogMu.Unlock()
}

func (b *Bot) watchdogTimes() (lastAction, lastSequenceStart time.Time) {
	b.watchdogMu.RLock()
	lastAction = b.lastAction
	lastSequenceStart = b.lastSequenceStart
	b.watchdogMu.RUnlock()
	return lastAction, lastSequenceStart
}

func (b *Bot) recordWatchdogIncident(kind string, state game.GameState, stuck time.Duration) {
	if b.telemetry == nil {
		return
	}
	b.telemetry.Emit(telemetry.EventAnomaly, map[string]any{
		"kind":       "watchdog_" + kind,
		"state":      state.String(),
		"duration_ms": stuck.Milliseconds(),
	})
	b.telemetry.WriteIncident("watchdog_" + kind)
}

// checkStuck enforces a global watchdog: if the capture pipeline is dead,
// or if the bot is in-progress for absurdly long, or has been sitting in
// one place doing nothing for too long, we cycle the game to recover from
// hangs / dialogs / out-of-game screens without requiring user intervention.
func (b *Bot) checkStuck(gc *game.GameContext) {
	lastAction, lastSequenceStart := b.watchdogTimes()

	if gc.ReadHealth().ConsecutiveFails >= 10 {
		b.recordWatchdogIncident("capture_dead", gc.State, 0)
		b.logger.Error().
			Int("consecutive_fails", gc.ReadHealth().ConsecutiveFails).
			Str("state", gc.State.String()).
			Msg("capture pipeline appears dead, beginning device recovery ladder...")
		b.recoverEmulator()
		b.recordSequenceStart()
		return
	}

	if b.seqRunning.Load() {
		if time.Since(lastSequenceStart) > 15*time.Minute {
			seqStuck := time.Since(lastSequenceStart)
			b.recordWatchdogIncident("sequence_timeout", gc.State, seqStuck)
			b.logger.Warn().
				Dur("seq_time", time.Since(lastSequenceStart)).
				Msg("attack sequence exceeded maximum duration, triggering emergency restart...")
			b.restartGame()
			b.recordSequenceStart()
		}
		return
	}

	state, _, _ := gc.ReadState()

	// Windows/BlueStacks can spend a while in StateUnknown immediately after
	// the game becomes visually usable (localized HUD, animated overlays, first
	// template-cache warmup). The old 35s generic watchdog restarted Clash
	// before the bot had a chance to obtain a stable village classification,
	// producing the exact launch -> 35s -> restart loop seen on Windows.
	// Give only the initial Unknown phase a bounded grace period; once a real
	// state is observed the normal watchdog rules apply.
	if state == game.StateUnknown && time.Since(b.startedAt) < 2*time.Minute {
		return
	}

	// Post-boot splash states (ТАР! collect splash, castle logo, news)
	// legitimately sit static for 1-3 minutes while the game connects — the
	// castle logo has no progress indicator at all. The generic stuck timeout
	// below (35s) would force-restart mid-boot, which previously caused an
	// endless force-stop/relaunch loop on the collect splash. Give the whole
	// boot-splash chain a generous window; the dismiss taps in processFrame
	// advance through it.
	if state == game.StateLogo || state == game.StateTapToContinue || state == game.StateNewsSplash {
		bootStuck := time.Since(lastAction)
		const bootSplashTimeout = 5 * time.Minute
		if bootStuck > bootSplashTimeout {
			b.recordWatchdogIncident("boot_splash", state, bootStuck)
			b.logger.Warn().
				Str("state", state.String()).
				Time("last_action", lastAction).
				Dur("stuck_time", bootStuck).
				Dur("timeout", bootSplashTimeout).
				Msg("boot splash stuck too long, triggering emergency restart...")
			b.restartGame()
			b.recordSequenceStart()
		}
		return
	}

	if state == game.StateBattle ||
		state == game.StateSearchMap ||
		state == game.StateLoading {
		attackPhaseStuck := time.Since(lastAction)
		const attackPhaseTimeout = 30 * time.Second
		if attackPhaseStuck > attackPhaseTimeout {
			b.recordWatchdogIncident("attack_phase", state, attackPhaseStuck)
			b.logger.Warn().
				Str("state", state.String()).
				Time("last_action", lastAction).
				Dur("stuck_time", attackPhaseStuck).
				Dur("timeout", attackPhaseTimeout).
				Msg("attack-phase state without active sequence, triggering emergency restart...")
			b.restartGame()
			b.recordSequenceStart()
		}
		return
	}

	timeout := b.stuckTimeout

	stuckTime := time.Since(lastAction)
	if stuckTime > timeout {
		b.recordWatchdogIncident("idle", state, stuckTime)
		b.logger.Warn().
			Str("state", state.String()).
			Time("last_action", lastAction).
			Dur("stuck_time", stuckTime).
			Dur("timeout", timeout).
			Msg("bot appears stuck without meaningful action, triggering emergency restart...")

		b.restartGame()
		b.recordSequenceStart()
	}
}

func (b *Bot) restartGame() {
	if b.seqRunning.Load() {
		b.resetAttackSoak("runtime_restart")
	}
	if !b.restartInFlight.CompareAndSwap(false, true) {
		b.logger.Debug().Msg("restart already in progress; suppressing duplicate request")
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

	b.client.JitteredSleep(2 * time.Second)

	if err := b.client.StartApp(pkg); err != nil {
		b.logger.Error().Err(err).Msg("failed to start app")
	}

	b.client.JitteredSleep(15 * time.Second)
	b.zoomedOut.Store(false)

	b.recordActivity()
	b.lastNav = time.Now()
	b.recordSequenceStart()
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
func safePacingActive(untilUS, nowUS int64) bool {
	return untilUS > nowUS
}

func (b *Bot) forceSafePacing(reason string, duration time.Duration) {
	if duration <= 0 {
		duration = 2 * time.Minute
	}
	until := time.Now().Add(duration).UnixMicro()
	for {
		current := b.safePacingUntilUS.Load()
		if current >= until || b.safePacingUntilUS.CompareAndSwap(current, until) {
			break
		}
	}
	if b.telemetry != nil {
		b.telemetry.Emit(telemetry.EventSpeedProfile, map[string]any{
			"mode": "Safe",
			"reason": "safety_governor",
			"incident": reason,
			"safe_until_unix_us": until,
		})
	}
}

func (b *Bot) safePacingForced() bool {
	return safePacingActive(b.safePacingUntilUS.Load(), time.Now().UnixMicro())
}

func (b *Bot) recoverEmulator() {
	b.resetAttackSoak("device_recovery")
	if !b.recoveryInFlight.CompareAndSwap(false, true) {
		b.logger.Debug().Msg("device recovery already in progress; suppressing duplicate request")
		return
	}
	defer func() {
		// Recovery itself may take well over the stale-heartbeat threshold.
		// Stamp a fresh grace period when it finishes so the supervisor does
		// not immediately start a second recovery wave against BlueStacks.
		b.captureHeartbeat.Store(time.Now().UnixNano())
		b.recoveryInFlight.Store(false)
	}()

	b.recoveryAttempts.Add(1)
	b.forceSafePacing("device_recovery", 2*time.Minute)
	if b.telemetry != nil {
		b.telemetry.Emit(telemetry.EventRecovery, map[string]any{"stage": "start", "attempt": b.recoveryAttempts.Load()})
		b.telemetry.WriteIncident("device_recovery")
	}
	b.logger.Warn().Msg("capture pipeline dead; beginning device recovery ladder")

	deviceOK := func() bool {
		_, _, err := b.client.ScreenSize()
		return err == nil
	}

	if deviceOK() {
		b.logger.Info().Msg("device still responsive; restarting game only")
		b.restartGame()
		b.recoverySuccesses.Add(1)
		if b.telemetry != nil {
			b.telemetry.Emit(telemetry.EventRecovery, map[string]any{"stage": "success", "method": "game_restart"})
		}
		return
	}

	b.logger.Warn().Msg("device unresponsive to wm size; reconnecting ADB transport")
	if err := b.client.Reconnect(); err != nil {
		b.logger.Warn().Err(err).Msg("transport reconnect failed")
	}
	if deviceOK() {
		b.restartGame()
		b.recoverySuccesses.Add(1)
		if b.telemetry != nil {
			b.telemetry.Emit(telemetry.EventRecovery, map[string]any{"stage": "success", "method": "adb_reconnect"})
		}
		return
	}

	b.logger.Warn().Msg("device still unreachable; resetting adb server (drops ALL adb connections on this host)")
	if err := b.client.ResetAdbServer(); err != nil {
		b.logger.Warn().Err(err).Msg("adb server reset failed")
	}
	time.Sleep(2 * time.Second)
	_ = b.client.Reconnect()
	if deviceOK() {
		b.restartGame()
		b.recoverySuccesses.Add(1)
		if b.telemetry != nil {
			b.telemetry.Emit(telemetry.EventRecovery, map[string]any{"stage": "success", "method": "adb_server_reset"})
		}
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
		time.Sleep(2 * time.Second)
	}
	if !recovered {
		b.logger.Error().Msg("device remained unreachable after BlueStacks recovery window; deferring until next watchdog cycle")
		if b.telemetry != nil {
			b.telemetry.Emit(telemetry.EventRecovery, map[string]any{
				"stage": "failed", "method": "bluestacks_relaunch",
			})
		}
		return
	}
	b.restartGame()
	b.recoverySuccesses.Add(1)
	if b.telemetry != nil {
		b.telemetry.Emit(telemetry.EventRecovery, map[string]any{
			"stage": "success", "method": "bluestacks_relaunch", "bluestacks_restart": true,
		})
	}
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

	// Keep normal output action-oriented. State changes are INFO; a stable
	// classifier heartbeat is DEBUG-only and heavily throttled. The old 750ms
	// INFO heartbeat flooded Wails/logBuffer even while nothing changed.
	stateChanged := state != gc.State
	bootVerbose := time.Since(b.startedAt) < 3*time.Second
	if stateChanged || bootVerbose {
		b.lastVisionLog = time.Now()
		b.logger.Info().
			Str("vision_state", state.String()).
			Int("score", score).
			Int("capture_w", screen.Cols()).
			Int("capture_h", screen.Rows()).
			Msg("vision state")
	} else if time.Since(b.lastVisionLog) >= 5*time.Second {
		b.lastVisionLog = time.Now()
		b.logger.Debug().
			Str("vision_state", state.String()).
			Int("score", score).
			Int("capture_w", screen.Cols()).
			Int("capture_h", screen.Rows()).
			Msg("vision heartbeat")
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
		previousState := gc.State
		gc.UpdateState(state, now)
		b.observeRuntimeState(state, now)
		if b.telemetry != nil && previousState != state {
			b.telemetry.Emit(telemetry.EventStateChanged, map[string]any{"from": previousState.String(), "to": state.String(), "score": score})
		}

		select {
		case gc.StateChange <- game.StateChange{From: gc.PrevState(), To: state, At: now}:
		default:
		}

		b.logger.Debug().
			Str("state", state.String()).
			Int("score", score).
			Msg("state detected")
	}

	if !b.seqRunning.Load() && (state == game.StateMainVillage || gc.State == game.StateMainVillage) {
		b.maybeScanVillageResources(screen)
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
				time.Sleep(1200 * time.Millisecond)

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
				time.Sleep(800 * time.Millisecond)
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
				time.Sleep(800 * time.Millisecond)
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

	if b.paused.Load() {
		// A pause requested during an attack takes effect naturally once the
		// active sequence returns home. Keep the observer alive, but never
		// start another farming cycle until ResumeAutomation is called.
		return
	}

	if b.zoomedOut.Load() && (gc.State == game.StateMainVillage || gc.State == game.StateUnknown) && b.findAttackButton(screen, 0.30) {
		b.logger.Info().Msg("attack button detected, starting sequence")
		b.recordSequenceStart()
		go b.executeAttackSequence(gc)
		return
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

func (b *Bot) findAttackButton(screen gocv.Mat, threshold float32) bool {
	// Prefer locating the actual orange button body in the tight bottom-left
	// HUD ROI. This is robust across language and avoids assuming one fixed
	// center coordinate.
	if x, y, ok := b.locateAttackButtonColor(screen); ok {
		b.logger.Debug().Int("x", x).Int("y", y).Msg("attack button verified via localized orange region")
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

	b.logger.Debug().
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

	b.logger.Debug().
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

	b.logger.Debug().
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

	b.logger.Debug().
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
	defer b.setRuntimePhase(PhaseIdle)
	sequenceRecoveryStart := b.recoveryAttempts.Load()
	sequenceBlueStacksRestartStart := b.blueStacksRestarts.Load()
	b.lastPlanningUS.Store(0)
	b.seqStartedAtUnix.Store(time.Now().Unix())
	defer b.seqStartedAtUnix.Store(0)
	defer b.seqRunning.Store(false)

	if b.cfg.Debug.UseShellPipe && runtime.GOOS != "windows" {
		b.client.EnablePersistentShell(b.cfg.Debug.ShellPipeSyncFlush)
		defer b.client.ClosePersistentShell()
	} else if b.cfg.Debug.UseShellPipe && runtime.GOOS == "windows" {
		// The persistent interactive ADB shell is not reliable enough on
		// BlueStacks/Windows yet. Live runs showed the pipe closing mid-sequence
		// ("use of closed network connection"), followed by capture/tap failures.
		// Use the proven one-shot transport path on Windows until the pipe has a
		// dedicated Windows implementation.
		b.logger.Debug().Msg("persistent adb shell pipe disabled on Windows-safe path")
	}

	if b.attackCount.Load() >= int32(b.cfg.Attack.MaxAttackPerSession) {
		return
	}

	// Long-session governor: combine hourly rate limits, scheduled rest and
	// recovery circuit-breaking before touching the village Attack button.
	// Re-evaluate after every wait because more than one policy can become
	// active at the same boundary (for example scheduled break + hourly cap).
	for b.governor != nil {
		gate := b.governor.Gate(time.Now(), b.attackCount.Load(), b.recoveryAttempts.Load())
		if gate.Wait <= 0 {
			break
		}
		b.logger.Info().
			Str("reason", gate.Reason).
			Dur("wait", gate.Wait).
			Int32("attacks", b.attackCount.Load()).
			Int32("recoveries", b.recoveryAttempts.Load()).
			Msg("automation governor pausing before next attack")
		if b.telemetry != nil {
			b.telemetry.Emit(telemetry.EventSpeedProfile, map[string]any{
				"mode": "Paused",
				"reason": "automation_governor",
				"policy": gate.Reason,
				"wait_ms": gate.Wait.Milliseconds(),
			})
		}
		timer := time.NewTimer(gate.Wait)
		select {
		case <-timer.C:
		case <-b.ctx.Done():
			if !timer.Stop() {
				select { case <-timer.C: default: }
			}
			return
		}
	}

	sequenceStartedAt := time.Now()
	var cooldownDurationMS int64
	var preparationDurationMS int64

	// A failed post-battle wall pass is never forgotten. Retry it once the bot
	// is safely back on the Main Village, before spending time on another
	// matchmaking cycle. Failure here does not deadlock farming: the pending
	// bit remains set and the next home cycle gets another chance.
	if b.cfg.Upgrade.UpgradeWalls && b.wallUpgradePending.Load() {
		screen, err := b.runtimeFrameFresh(2 * time.Second)
		if err == nil && !screen.Empty() {
			state, _ := b.classify(screen)
			screen.Close()
			if state == game.StateMainVillage {
				b.logger.Info().Msg("pending wall-upgrade stage detected; retrying before next attack")
				if b.UpgradeWalls(gc) {
					b.wallUpgradePending.Store(false)
				}
			}
		} else if err == nil {
			screen.Close()
		}
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
			cooldownStarted := time.Now()
			b.logger.Info().
				Dur("wait", wait).
				Int("min_gap_s", b.cfg.Attack.MinSecondsBetweenAttacks).
				Msg("waiting out inter-attack cooldown before next search")
			select {
			case <-time.After(wait):
				cooldownDurationMS = time.Since(cooldownStarted).Milliseconds()
			case <-b.ctx.Done():
				return
			}
		}
	}

	preparationStarted := time.Now()
	b.lastPrepTimings = PreparationTimings{}
	if !b.clickSequence() {
		b.logger.Warn().Msg("attack click sequence failed, restarting game to recover...")
		b.restartGame()
		return
	}
	preparationDurationMS = time.Since(preparationStarted).Milliseconds()

	b.setRuntimePhase(PhaseSearching)
	b.logger.Info().
		Int64("prep_ms", preparationDurationMS).
		Msg("search started")
	if runtime.GOOS == "windows" {
		// Give BlueStacks one quiet render window after entering matchmaking.
		// Capturing immediately while HD-Player is switching from clouds/menu
		// to the first opponent has produced native memory crashes on Pie64.
		b.logger.Debug().Msg("Windows matchmaking settle: pausing before first search capture")
		time.Sleep(1200 * time.Millisecond)
	}
	if b.telemetry != nil {
		b.telemetry.Emit(telemetry.EventSearchStarted, nil)
	}

	lootRec := b.searchLootRec
	closeLootRec := false
	if lootRec == nil {
		// Defensive fallback for tests/legacy constructors. Production bots
		// keep one recognizer hot for the whole session.
		lootRec = game.NewLootRecognizer(b.cal, b.templates, b.logger)
		closeLootRec = true
	}
	if closeLootRec {
		defer lootRec.Close()
	}

	var remainingUndeployed int
	var deployErr error
	var deployDurationMS int64
	var acceptedTargetGold, acceptedTargetElixir, acceptedTargetDE int
	var acceptedTargetScore int
	var stratName string = "Unknown"
	var targetEdge string = "Unknown"
	var deploySide string = "Unknown"
	searchStrategyName := filepath.Base(b.cfg.Attack.StrategyFile)
	if searchStrat, searchStratErr := strategy.ParseYAML(b.cfg.Attack.StrategyFile); searchStratErr == nil && strings.TrimSpace(searchStrat.Name) != "" {
		searchStrategyName = searchStrat.Name
	}

	searchStart := time.Now()
	attackStartedAt := time.Time{}
	sequenceSkips := 0

	// Xingchen-style search loop: one authoritative capture per cycle,
	// one decision, one transition action. No multi-probe Next verifier and
	// no adaptive capture bursts around the most fragile BlueStacks phase.
	for {
		select {
		case <-b.ctx.Done():
			b.logger.Info().Msg("search loop cancelled by stop, abandoning attack sequence")
			return
		default:
		}

		if time.Since(searchStart) > 5*time.Minute {
			b.logger.Error().Msg("search exceeded five minutes; restarting game")
			b.restartGame()
			return
		}

		if !b.sleepResponsive(120 * time.Millisecond) {
			return
		}

		screen, err := b.runtimeFrameFresh(2 * time.Second)
		if err != nil {
			b.logger.Warn().Err(err).Msg("search capture failed")
			if !b.sleepResponsive(1200 * time.Millisecond) {
				return
			}
			continue
		}
		if screen.Empty() {
			screen.Close()
			continue
		}
		b.captureHeartbeat.Store(time.Now().UnixNano())

		state, _ := b.classify(screen)
		if state != game.StateBattle {
			if state == game.StateSearchMap || state == game.StateLoading {
				b.logger.Debug().Str("state", state.String()).Msg("matchmaking in progress")
			} else {
				b.logger.Debug().Str("state", state.String()).Msg("waiting for searchable base")
				if isTransientRuntimeState(state) {
					b.dismissInterruptions()
				}
			}
			screen.Close()
			continue
		}

		b.logger.Info().Msg("base found; reading loot")
		scanStarted := time.Now()
		loot, lootErr := lootRec.ReadAvailableLoot(screen)
		targetScanUS := time.Since(scanStarted).Microseconds()
		if lootErr != nil {
			b.logger.Warn().Err(lootErr).Msg("loot read failed; treating target conservatively")
		}

		target := intelligence.Target{
			Gold: loot.Gold, Elixir: loot.Elixir, DarkElixir: loot.DarkElixir,
		}
		rules := intelligence.TargetRules{
			MinGold:       b.cfg.Search.MinLootGold,
			MinElixir:     b.cfg.Search.MinLootElixir,
			MinDarkElixir: b.cfg.Search.MinLootDarkElixir,
			DarkOverride:  b.cfg.Search.AttackIfDarkElixirGT,
			SearchEnabled: b.cfg.Search.Enabled,
		}
		decision := intelligence.EvaluateTarget(target, rules)

		// V3 learns the value of continuing matchmaking versus attacking this
		// concrete base. No extra screenshot is requested: it consumes the loot
		// OCR already read from the current broker frame and historical outcomes.
		if b.contextual != nil && lootErr == nil {
			allowAggressiveReject := !b.safePacingForced() &&
				b.client.Health().ConsecutiveFails == 0 &&
				!b.recoveryInFlight.Load()
			farmDecision := b.contextual.RecommendTarget(
				searchStrategyName,
				b.cfg.Attack.Farm.TownHall,
				target,
				rules,
				decision,
				time.Since(searchStart),
				sequenceSkips,
				allowAggressiveReject,
			)
			if farmDecision.Apply && farmDecision.Accept != decision.Accept {
				b.logger.Info().
					Bool("legacy_accept", decision.Accept).
					Bool("v3_accept", farmDecision.Accept).
					Float64("predicted_farm_rate", farmDecision.PredictedFarmRate).
					Float64("baseline_farm_rate", farmDecision.BaselineFarmRate).
					Float64("capture_efficiency", farmDecision.CaptureEfficiency).
					Int("samples", farmDecision.Samples).
					Int("skips", sequenceSkips).
					Str("reason", farmDecision.Reason).
					Msg("Intelligence V3 overrode target decision for farm throughput")
				decision.Accept = farmDecision.Accept
				decision.Reason = farmDecision.Reason
			} else if farmDecision.Apply {
				decision.Reason = farmDecision.Reason
			}
		}

		if b.telemetry != nil {
			if decision.Accept {
				b.telemetry.Emit(telemetry.EventTargetFound, map[string]any{
					"gold": loot.Gold, "elixir": loot.Elixir, "de": loot.DarkElixir,
					"score": decision.Score, "accept": true, "reason": decision.Reason,
					"scan_us": targetScanUS,
				})
			} else {
				b.telemetry.RecordRejectedTarget(
					loot.Gold, loot.Elixir, loot.DarkElixir, decision.Score, targetScanUS,
					b.cfg.Search.MinLootGold, b.cfg.Search.MinLootElixir, b.cfg.Search.MinLootDarkElixir,
				)
			}
		}

		if decision.Accept {
			attackStartedAt = time.Now()
			acceptedTargetGold = loot.Gold
			acceptedTargetElixir = loot.Elixir
			acceptedTargetDE = loot.DarkElixir
			acceptedTargetScore = decision.Score

			b.logger.Info().
				Int("score", decision.Score).
				Int("gold", loot.Gold).
				Int("elixir", loot.Elixir).
				Int("de", loot.DarkElixir).
				Msg("target accepted — attacking")

			if b.telemetry != nil {
				b.telemetry.Emit(telemetry.EventAttackStarted, map[string]any{
					"gold": loot.Gold, "elixir": loot.Elixir, "de": loot.DarkElixir,
					"search_ms": attackStartedAt.Sub(searchStart).Milliseconds(),
					"skips": sequenceSkips,
				})
			}

			b.attackExec.SetInitialLoot(loot.Gold, loot.Elixir, loot.DarkElixir)
			// Reset the per-attack learned exit policy before planning. A previous
			// attack's policy must never leak into a strategy that cannot be parsed.
			b.attackExec.SetAdaptiveFarmExit(false, 0, 12*time.Second)
			b.setRuntimePhase(PhasePlanning)
			deployStarted := time.Now()

			if strat, stratErr := strategy.ParseYAML(b.cfg.Attack.StrategyFile); stratErr == nil {
				stratName = strat.Name
				targetEdge = strat.TargetEdge

				// Intelligence V3 operates only on the frame/data already present.
				// Fixed YAML edges remain authoritative. Rotate/Random/Adaptive may
				// be replaced by a learned champion; challenger exploration is
				// disabled whenever the BlueStacks safety governor is active.
				if b.contextual != nil {
					ctx := intelligence.AttackContext{
						Strategy: strat.Name,
						TownHall: b.cfg.Attack.Farm.TownHall,
						TargetScore: acceptedTargetScore,
						TargetGold: acceptedTargetGold,
						TargetElixir: acceptedTargetElixir,
						TargetDE: acceptedTargetDE,
					}
					allowExplore := !b.safePacingForced() &&
						b.client.Health().ConsecutiveFails == 0 &&
						!b.recoveryInFlight.Load()
					rec := b.contextual.RecommendEdge(ctx, strat.TargetEdge,
						[]string{"TopLeft", "TopRight", "BottomLeft", "BottomRight"}, allowExplore)
					if rec.Apply {
						original := strat.TargetEdge
						strat.TargetEdge = rec.Edge
						targetEdge = rec.Edge
						b.logger.Info().
							Str("original_edge", original).
							Str("learned_edge", rec.Edge).
							Bool("challenger", rec.Exploratory).
							Float64("confidence", rec.Confidence).
							Int("context_samples", rec.ContextSamples).
							Str("scope", rec.ProfileScope).
							Str("reason", rec.Reason).
							Msg("Intelligence V3 selected attack edge")
					}

					exitRec := b.contextual.RecommendFarmExit(strat.Name, b.cfg.Attack.Farm.TownHall)
					exitEnabled := exitRec.Enabled &&
						!b.safePacingForced() &&
						b.client.Health().ConsecutiveFails == 0 &&
						!b.recoveryInFlight.Load()
					b.attackExec.SetAdaptiveFarmExit(
						exitEnabled,
						exitRec.MinLootPercent,
						time.Duration(exitRec.StallSeconds)*time.Second,
					)
					if exitRec.Enabled {
						b.logger.Info().
							Bool("enabled", exitEnabled).
							Int("min_loot_percent", exitRec.MinLootPercent).
							Int("stall_seconds", exitRec.StallSeconds).
							Int("samples", exitRec.Samples).
							Str("reason", exitRec.Reason).
							Msg("Intelligence V3 prepared farm-throughput battle exit")
					}
				}

				remainingUndeployed, deployErr = b.deployParsedStrategy(screen, strat)
			} else {
				deployErr = stratErr
				b.logger.Warn().Err(stratErr).Str("path", b.cfg.Attack.StrategyFile).Msg("could not load strategy")
			}
			deployDurationMS = time.Since(deployStarted).Milliseconds()

			if resolved := b.attackExec.LastResolvedEdge(); resolved != "" {
				targetEdge = resolved
			}
			if side := b.attackExec.LastDeploySide(); side != "" {
				deploySide = side
			}
			b.attackExec.SetEarlyExitAllowed(deployErr == nil && remainingUndeployed == 0)

			if deployErr != nil || remainingUndeployed > 0 {
				if b.telemetry != nil {
					b.telemetry.WriteIncident("deployment_failed")
				}
				b.logger.Warn().
					Err(deployErr).
					Int("remaining", remainingUndeployed).
					Msg("deployment incomplete; battle remains active")
			}

			b.setRuntimePhase(PhaseBattle)
			screen.Close()
			break
		}

		sequenceSkips++
		b.skipsCount.Add(1)
		b.logger.Info().
			Int("skip", sequenceSkips).
			Int("gold", loot.Gold).
			Int("elixir", loot.Elixir).
			Int("de", loot.DarkElixir).
			Msg("target rejected — requesting next base")

		// Use the frame already in memory. Only if the button cannot be found
		// do we allow one evidence-gated retry. This prevents the old
		// capture->tap->capture->verify->capture retry burst.
		nextClicked := false
		if x, y, ok := b.locateNextButtonColor(screen); ok {
			if err := b.client.TapRandomized(x, y); err == nil {
				b.recordActivity()
				nextClicked = true
			}
		}
		screen.Close()

		if !nextClicked {
			nextClicked = b.waitAndClickButton("btn_next", "Next Match", 1800*time.Millisecond)
		}
		if !nextClicked {
			b.logger.Error().Msg("Next button not visually confirmed; restarting game instead of blind tapping")
			b.forceSafePacing("next_unresponsive", 2*time.Minute)
			b.restartGame()
			return
		}

		if b.telemetry != nil {
			b.telemetry.Emit(telemetry.EventTargetSkipped, map[string]any{
				"sequence_skips": sequenceSkips,
			})
		}

		// Xingchen-style transition settle: do not poll repeatedly while
		// BlueStacks is animating clouds / loading the next opponent.
		if !b.sleepResponsive(450 * time.Millisecond) {
			return
		}
	}

	if deployErr != nil || remainingUndeployed > 0 {
		b.forceSafePacing("incomplete_deployment", 2*time.Minute)
		b.logger.Warn().
			Int("remaining", remainingUndeployed).
			Msg("deployment ended with units still unverified; battle continues but deployment is NOT marked complete")
	} else {
		b.logger.Info().Msg("battle deployment complete: all live deployable units verified, waiting for battle to end naturally...")
	}

	var battleStars int = 0
	var battleGold int = 0
	var battleElixir int = 0
	var battleDE int = 0
	var bonusGold, bonusElixir, bonusDE int = 0, 0, 0
	var parsedResults bool = false
	starsSource := "unknown"
	lootSource := "unknown"
	resultConfidence := "low"

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
		resultSettleDuration, resultPanelStable := b.waitForStableResultPanel(1800 * time.Millisecond)
		b.logger.Debug().
			Dur("duration", resultSettleDuration).
			Bool("stable", resultPanelStable).
			Msg("result panel settle completed")

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
			resultScreen, err := b.runtimeFrameFresh(2 * time.Second)
			if err != nil {
				b.logger.Warn().Err(err).Msg("battle result capture failed; retrying")
				time.Sleep(500 * time.Millisecond)
				continue
			}
			gocv.IMWrite(paths.ResolveConfig("last_battle_result.png"), resultScreen)
			b.logger.Debug().Msg("saved battle result screenshot to last_battle_result.png")

			res, rerr := lootRec.ReadBattleResult(resultScreen)
			hash := resultPanelHash(resultScreen, b.cal)
			resultScreen.Close()

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
			starsSource = "result_ocr"
			lootSource = "result_ocr"
			resultConfidence = "medium"
			battleGold = parsedResult.Loot.Gold
			battleElixir = parsedResult.Loot.Elixir
			battleDE = parsedResult.Loot.DarkElixir
			bonusGold = parsedResult.Bonus.Gold
			bonusElixir = parsedResult.Bonus.Elixir
			bonusDE = parsedResult.Bonus.DarkElixir
			parsedResults = true

			// Reconcile stars against live battle facts. Visual OCR can still
			// reveal a TH star when the optional TH banner detector missed it,
			// but it may never fall BELOW a confirmed live minimum (e.g. TH
			// destroyed => at least 1★; >=50% + TH => at least 2★) or claim an
			// impossible 3★ below 100%.
			var overridden bool
			battleStars, overridden = intelligence.ReconcileBattleStars(
				visualStars,
				finalPct,
				b.attackExec.ThDestroyed(),
			)
			if finalPct >= 100 {
				starsSource = "battle_outcome"
			} else if overridden {
				starsSource = "reconciled_outcome"
				b.logger.Warn().
					Int("visual_stars", visualStars).
					Int("reconciled_stars", battleStars).
					Int("destruction_pct", finalPct).
					Bool("th_destroyed", b.attackExec.ThDestroyed()).
					Msg("result-screen stars rejected as inconsistent with battle outcome")
			}

			// Prefer the battle's live Available-Loot delta over themed
			// result-screen OCR whenever two stable live reads were accepted.
			// This directly measures what disappeared from the enemy's loot
			// counters and is substantially more stable across CoC themes.
			if liveLoot, ok := b.attackExec.EstimatedLootStolen(); ok {
				lootSource = "live_delta"
				if finalPct > 0 {
					resultConfidence = "high"
				}
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
				starsSource = "battle_outcome"
				resultConfidence = "medium"
				b.logger.Warn().
					Int("stars", battleStars).
					Int("destruction_pct", finalPct).
					Msg("result-screen OCR failed; stars derived from measured battle outcome")
			} else {
				b.logger.Error().Msg("battle result OCR failed after retries; no reliable destruction read available")
			}

			if liveLoot, ok := b.attackExec.EstimatedLootStolen(); ok {
				lootSource = "live_delta"
				if finalPct > 0 {
					resultConfidence = "high"
				}
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
	if b.multiAccount != nil {
		if err := b.multiAccount.ObserveAttack(); err != nil {
			b.logger.Warn().Err(err).Msg("could not persist multi-account attack counter")
		}
	}
	if b.governor != nil {
		b.governor.RecordAttack(time.Now())
	}

	depErrStr := ""
	if deployErr != nil {
		depErrStr = deployErr.Error()
	}

	attackHealth := b.client.Health()
	attackMode := chooseSearchPacing(attackHealth).Mode
	liveBarRescans, avgLiveBarRescanMS, avgSlotDetectMS, avgSlotClassifyMS, templatesTried, templatesMatched, avgSelectedCardOCRMS := b.attackExec.LiveBarMetrics()
	battleLootOCRSamples, avgBattleLootOCRMS := b.attackExec.BattleLootOCRMetrics()
	battleEndWaitMS, lootExitPercent := b.attackExec.BattleExitMetrics()
	deploySafety := b.attackExec.DeploymentSafety()
	attackTelemetry := telemetry.Snapshot{}
	sessionID := ""
	if b.telemetry != nil {
		attackTelemetry = b.telemetry.Snapshot()
		sessionID = b.telemetry.SessionID()
	}

	activeAccountID := ""
	activePlayerTag := strings.ToUpper(strings.TrimSpace(b.cfg.Account.PlayerTag))
	if b.multiAccount != nil {
		if active, ok := b.multiAccount.Active(); ok {
			activeAccountID = active.ID
			if strings.TrimSpace(active.PlayerTag) != "" {
				activePlayerTag = strings.ToUpper(strings.TrimSpace(active.PlayerTag))
			}
		}
	}

	rep := AttackReport{
		Timestamp:        time.Now().Format(time.RFC3339),
		SessionID:        sessionID,
		AccountID:        activeAccountID,
		PlayerTag:        activePlayerTag,
		Strategy:         stratName,
		TargetEdge:       targetEdge,
		DeploySide:       deploySide,
		DeploySuccess:    deployErr == nil && remainingUndeployed == 0,
		UndeployedSlots:  remainingUndeployed,
		DeployError:      depErrStr,
		ParsedResults:    parsedResults,
		StarsSource:      starsSource,
		LootSource:       lootSource,
		ResultConfidence: resultConfidence,
		Stars:            battleStars,
		GoldStolen:       battleGold,
		ElixirStolen:     battleElixir,
		DarkElixirStolen: battleDE,
		BonusGold:        bonusGold,
		BonusElixir:      bonusElixir,
		BonusDE:          bonusDE,
		TotalAttacks:     b.attackCount.Load(),
		SearchSkips:      sequenceSkips,
		SearchDurationMS: func() int64 { if attackStartedAt.IsZero() { return 0 }; return attackStartedAt.Sub(searchStart).Milliseconds() }(),
		CycleDurationMS:  time.Since(searchStart).Milliseconds(),
		DeployDurationMS: deployDurationMS,
		BattleDurationMS: func() int64 { if attackStartedAt.IsZero() { return 0 }; return time.Since(attackStartedAt).Milliseconds() }(),
		TargetGold:       acceptedTargetGold,
		TargetElixir:     acceptedTargetElixir,
		TargetDE:         acceptedTargetDE,
		TargetScore:      acceptedTargetScore,
		RuntimeMode:      attackMode,
		CaptureMS:        attackHealth.AvgCaptureMs,
		TargetScanMS:     attackTelemetry.AvgTargetScanMS,
		LiveBarRescans:  liveBarRescans,
		AvgLiveBarRescanMS: avgLiveBarRescanMS,
		AvgSlotDetectMS: avgSlotDetectMS,
		AvgSlotClassifyMS: avgSlotClassifyMS,
		TemplatesTried: templatesTried,
		TemplatesMatched: templatesMatched,
		AvgSelectedCardOCRMS: avgSelectedCardOCRMS,
		BattleLootOCRSamples: battleLootOCRSamples,
		AvgBattleLootOCRMS: avgBattleLootOCRMS,
		BattleEndWaitMS: battleEndWaitMS,
		LootExitPercent: lootExitPercent,
		PreparationDurationMS: preparationDurationMS,
		PrepAttackButtonMS:    b.lastPrepTimings.AttackButtonMS,
		PrepFindMatchMS:       b.lastPrepTimings.FindMatchMS,
		PrepArmyMenuMS:        b.lastPrepTimings.ArmyMenuMS,
		PrepArmySlotMS:        b.lastPrepTimings.ArmySlotMS,
		PrepBattleButtonMS:    b.lastPrepTimings.BattleButtonMS,
		PrepMatchmakingReadyMS: b.lastPrepTimings.MatchmakingReadyMS,
		CooldownDurationMS:    cooldownDurationMS,
		BattleEndReason:  b.attackExec.LastBattleEndReason(),
		DestructionPct:   b.attackExec.LastDestructionPercent(),
		TownHallDestroyed: b.attackExec.ThDestroyed(),
		SafetyMode: deploySafety.Mode,
		RedZoneValid: deploySafety.RedZoneValid,
		CorridorVerified: deploySafety.CorridorVerified,
		HUDSafe: deploySafety.HUDSafe,
		RedZoneX1: deploySafety.RedZoneX1,
		RedZoneY1: deploySafety.RedZoneY1,
		RedZoneX2: deploySafety.RedZoneX2,
		RedZoneY2: deploySafety.RedZoneY2,
		DeployLineX1: deploySafety.DeployX1,
		DeployLineY1: deploySafety.DeployY1,
		DeployLineX2: deploySafety.DeployX2,
		DeployLineY2: deploySafety.DeployY2,
		DeployFreeSpace: deploySafety.FreeSpace,
		ReturnHomeSuccess: false,
	}

	if b.telemetry != nil {
		b.telemetry.Emit(telemetry.EventAttackFinished, map[string]any{"strategy": rep.Strategy, "edge": rep.TargetEdge, "deploy_side": rep.DeploySide, "stars": rep.Stars, "gold": rep.GoldStolen + rep.BonusGold, "elixir": rep.ElixirStolen + rep.BonusElixir, "de": rep.DarkElixirStolen + rep.BonusDE, "deploy_success": rep.DeploySuccess, "cooldown_ms": rep.CooldownDurationMS, "prep_ms": rep.PreparationDurationMS, "search_ms": rep.SearchDurationMS, "deploy_ms": rep.DeployDurationMS, "battle_ms": rep.BattleDurationMS, "cycle_ms": rep.CycleDurationMS, "target_score": rep.TargetScore, "live_bar_rescans": rep.LiveBarRescans, "live_bar_rescan_ms": rep.AvgLiveBarRescanMS, "slot_detect_ms": rep.AvgSlotDetectMS, "slot_classify_ms": rep.AvgSlotClassifyMS, "templates_tried": rep.TemplatesTried, "templates_matched": rep.TemplatesMatched, "selected_card_ocr_ms": rep.AvgSelectedCardOCRMS, "battle_loot_ocr_samples": rep.BattleLootOCRSamples, "battle_loot_ocr_ms": rep.AvgBattleLootOCRMS, "battle_end_wait_ms": rep.BattleEndWaitMS, "loot_exit_percent": rep.LootExitPercent, "stars_source": rep.StarsSource, "loot_source": rep.LootSource, "result_confidence": rep.ResultConfidence, "end_reason": rep.BattleEndReason, "destruction_pct": rep.DestructionPct, "town_hall_destroyed": rep.TownHallDestroyed, "safety_mode": rep.SafetyMode, "red_zone_valid": rep.RedZoneValid, "corridor_verified": rep.CorridorVerified, "hud_safe": rep.HUDSafe, "red_zone_x1": rep.RedZoneX1, "red_zone_y1": rep.RedZoneY1, "red_zone_x2": rep.RedZoneX2, "red_zone_y2": rep.RedZoneY2, "deploy_line_x1": rep.DeployLineX1, "deploy_line_y1": rep.DeployLineY1, "deploy_line_x2": rep.DeployLineX2, "deploy_line_y2": rep.DeployLineY2, "deploy_free_space": rep.DeployFreeSpace})
	}

	if repBytes, err := json.MarshalIndent(rep, "", "  "); err == nil {
		_ = AsyncWriteFileSoon(paths.ResolveConfig("last_attack_report.json"), repBytes, 0644)
	}

	history := b.HistorySnapshot()
	if history == nil {
		if histData, err := os.ReadFile(paths.ResolveConfig("attack_history.json")); err == nil {
			_ = json.Unmarshal(histData, &history)
		}
	}
	history = append([]AttackReport{rep}, history...)
	if len(history) > 500 {
		history = history[:500]
	}
	b.historyMu.Lock()
	b.historyCache = append([]AttackReport(nil), history...)
	b.historyMu.Unlock()
	if histBytes, err := json.MarshalIndent(history, "", "  "); err == nil {
		_ = AsyncWriteFileSoon(paths.ResolveConfig("attack_history.json"), histBytes, 0644)
	}

	// Multi-account sessions retain the global history for the dashboard while
	// also writing an account-scoped history used for account-specific analysis.
	if strings.TrimSpace(rep.PlayerTag) != "" {
		accountPath := accountAttackHistoryPath(b.cfg)
		var accountHistory []AttackReport
		if data, err := os.ReadFile(accountPath); err == nil {
			_ = json.Unmarshal(data, &accountHistory)
		}
		accountHistory = append([]AttackReport{rep}, accountHistory...)
		if len(accountHistory) > 500 {
			accountHistory = accountHistory[:500]
		}
		if data, err := json.MarshalIndent(accountHistory, "", "  "); err == nil {
			_ = AsyncWriteFileSoon(accountPath, data, 0644)
		}
	}

	// Notify the UI after historyCache is updated. Wails mirrors this cache
	// directly, so persistence can flush independently without delaying
	// ReturnHome or making the dashboard wait on filesystem I/O.
	if b.OnStatsUpdate != nil {
		b.OnStatsUpdate()
	}

	b.setRuntimePhase(PhaseReturningHome)
	returnHomeStarted := time.Now()
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
	returnHomeDur := time.Since(returnHomeStarted)
	b.returnHomeCount.Add(1)
	b.returnHomeMicros.Add(returnHomeDur.Microseconds())
	b.lastReturnHomeUS.Store(returnHomeDur.Microseconds())
	// Enrich the already-published attack report with post-battle overhead.
	// The initial row is intentionally saved before ReturnHome so battle loot
	// appears immediately; this second lightweight write adds the true
	// ready-to-ready timing once ReturnHome finishes.
	rep.ReturnHomeDurationMS = returnHomeDur.Milliseconds()
	rep.ReturnHomeSuccess = returnedHome
	rep.FullRoutineDurationMS = time.Since(sequenceStartedAt).Milliseconds()
	recoveryDelta := b.recoveryAttempts.Load() - sequenceRecoveryStart
	blueStacksRestartDelta := b.blueStacksRestarts.Load() - sequenceBlueStacksRestartStart
	b.recordAttackSoak(rep, recoveryDelta, blueStacksRestartDelta)

	if b.contextual != nil {
		outcome := contextualOutcomeFromReport(rep, b.cfg.Attack.Farm.TownHall, int(recoveryDelta), int(blueStacksRestartDelta))
		reward, learnErr := b.contextual.Observe(outcome)
		if learnErr != nil {
			b.logger.Warn().Err(learnErr).Msg("Intelligence V3 attack observation could not be saved")
		} else {
			b.logger.Info().
				Float64("context_reward", reward).
				Float64("farm_rate_per_hour", intelligence.FarmResourcesPerHour(outcome)).
				Int("gold", rep.GoldStolen).
				Int("elixir", rep.ElixirStolen).
				Int("dark_elixir", rep.DarkElixirStolen).
				Int64("routine_ms", rep.FullRoutineDurationMS).
				Str("edge", rep.TargetEdge).
				Msg("Intelligence V3 learned from farm throughput")
		}
	}

	if b.adaptive != nil {
		clean := rep.DeploySuccess && rep.ReturnHomeSuccess && rep.ParsedResults &&
			recoveryDelta == 0 && blueStacksRestartDelta == 0
		reward, learnErr := b.adaptive.Observe(intelligence.LearningOutcome{
			Domain: "attack",
			Parameters: map[string]float64{
				"search_capture_ms":   700,
				"planning_capture_ms": 250,
				"deploy_capture_ms":   350,
				"card_settle_ms":      150,
				"camera_zoom_steps":   3,
				"camera_pan_steps":    2,
			},
			Clean:             clean,
			DeploySuccess:     rep.DeploySuccess,
			ReturnHomeSuccess: rep.ReturnHomeSuccess,
			SafeDeployment:    rep.RedZoneValid,
			ParsedResults:     rep.ParsedResults,
			RecoveryCount:     int(recoveryDelta),
			BlueStacksRestart: int(blueStacksRestartDelta),
			PlanningMS:        b.lastPlanningUS.Load() / 1000,
			DeployMS:          rep.DeployDurationMS,
			CaptureMS:         rep.CaptureMS,
		})
		if learnErr != nil {
			b.logger.Warn().Err(learnErr).Msg("adaptive intelligence observation could not be saved")
		} else {
			b.logger.Info().
				Float64("reward", reward).
				Str("mode", string(b.adaptive.Mode())).
				Bool("clean", clean).
				Msg("adaptive intelligence learned from attack")
		}
	}

	b.historyMu.Lock()
	if len(b.historyCache) > 0 && b.historyCache[0].Timestamp == rep.Timestamp {
		b.historyCache[0] = rep
	}
	postReturnHistory := make([]AttackReport, len(b.historyCache))
	copy(postReturnHistory, b.historyCache)
	b.historyMu.Unlock()
	if histBytes, err := json.MarshalIndent(postReturnHistory, "", "  "); err == nil {
		_ = AsyncWriteFileSoon(paths.ResolveConfig("attack_history.json"), histBytes, 0644)
	}
	if repBytes, err := json.MarshalIndent(rep, "", "  "); err == nil {
		_ = AsyncWriteFileSoon(paths.ResolveConfig("last_attack_report.json"), repBytes, 0644)
	}
	if b.OnStatsUpdate != nil {
		b.OnStatsUpdate()
	}

	// Native regression guard: compare the newest attacks against the recent
	// baseline after the true routine duration is known. Diagnostics only —
	// it never changes strategy, target thresholds or deployment geometry.
	if b.telemetry != nil {
		perfHistory := b.HistorySnapshot()
		limit := len(perfHistory)
		if limit > 20 { limit = 20 }
		samples := make([]intelligence.PerformanceSample, 0, limit)
		for i := 0; i < limit; i++ {
			h := perfHistory[i]
			samples = append(samples, intelligence.PerformanceSample{
				SearchMS: h.SearchDurationMS,
				DeployMS: h.DeployDurationMS,
				RoutineMS: h.FullRoutineDurationMS,
				CaptureMS: h.CaptureMS,
				TargetScanMS: h.TargetScanMS,
				DeploySuccess: h.DeploySuccess,
				ReturnHomeOK: h.ReturnHomeSuccess,
				SafeDeployment: h.RedZoneValid && h.CorridorVerified && h.HUDSafe,
			})
		}
		assessment := intelligence.AnalyzePerformance(samples)
		if assessment.Status == "watch" {
			b.telemetry.Emit(telemetry.EventAnomaly, map[string]any{
				"kind": "performance_regression",
				"recent": assessment.RecentCount,
				"baseline": assessment.BaseCount,
				"regressions": assessment.Regressions,
			})
		}
	}
	if b.telemetry != nil {
		b.telemetry.Emit(telemetry.EventReturnHome, map[string]any{
			"success": returnedHome,
			"duration_ms": returnHomeDur.Milliseconds(),
		})
		if returnHomeDur >= 3*time.Second {
			b.telemetry.Emit(telemetry.EventAnomaly, map[string]any{
				"kind": "slow_return_home",
				"duration_ms": returnHomeDur.Milliseconds(),
				"success": returnedHome,
			})
		}
	}

	if !returnedHome {
		if b.cfg.Upgrade.UpgradeWalls {
			b.wallUpgradePending.Store(true)
		}
		b.logger.Error().Msg("failed to return home after battle, restarting game...")
		b.restartGame()
		return
	}

	// Stamp the attack boundary so the inter-attack cooldown has a clean
	// reference point (set only on a real return home, not on a restart).
	b.lastAttackEnd = time.Now()

	sideX := int(537 * b.cal.ScaleX)
	sideY := int(693 * b.cal.ScaleY)
	b.logger.Debug().Msg("dismissing potential post-attack popup")
	_ = b.client.Tap(sideX, sideY)
	if b.cfg.Upgrade.UpgradeWalls {
		b.wallUpgradePending.Store(true)
		// ReturnHome already verified the village; a short settle is sufficient
		// before the evidence-driven wall stage starts.
		time.Sleep(450 * time.Millisecond)
		if b.UpgradeWalls(gc) {
			b.wallUpgradePending.Store(false)
		}
	} else {
		b.wallUpgradePending.Store(false)
		// ReturnHome already verified MainVillage. For pure farming, only a
		// short acknowledgement window is needed for the side tap itself.
		time.Sleep(300 * time.Millisecond)
	}

	if b.multiAccount != nil {
		if next, due := b.multiAccount.NextDue(); due {
			b.logger.Info().
				Str("next_account_id", next.ID).
				Str("next_account_label", next.Label).
				Int("switch_slot", next.SwitchSlot).
				Msg("multi-account rotation is due; waiting for safe calibrated switch")
			// Actual Supercell-ID navigation is fail-closed and lives in
			// switchMultiAccountIfReady. If calibration is unavailable, the
			// current account keeps farming rather than receiving blind taps.
			if err := b.switchMultiAccountIfReady(next); err != nil {
				_ = b.multiAccount.MarkSwitchFailed(err)
				b.logger.Warn().Err(err).Msg("multi-account switch deferred safely")
			}
		}
	}

	// Cap check stays after wall upgrades so the graceful shutdown (2s
	// grace then cancel) never interrupts an in-progress wall loop; the
	// count itself was already incremented when the report was recorded.
	if int(b.attackCount.Load()) >= b.cfg.Attack.MaxAttackPerSession {
		attacks := b.attackCount.Load()
		cap := b.cfg.Attack.MaxAttackPerSession
		b.logger.Info().
			Int32("attacks", attacks).
			Int("cap", cap).
			Msg("attack cap reached, scheduling graceful shutdown...")
		if b.telemetry != nil {
			b.telemetry.Emit(telemetry.EventSessionComplete, map[string]any{
				"reason":  "attack_cap",
				"attacks": attacks,
				"cap":     cap,
			})
		}
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

	b.logger.Info().
		Str("strategy", rep.Strategy).
		Int("stars", rep.Stars).
		Int("gold", rep.GoldStolen+rep.BonusGold).
		Int("elixir", rep.ElixirStolen+rep.BonusElixir).
		Int("de", rep.DarkElixirStolen+rep.BonusDE).
		Str("deploy", deployStatus).
		Int64("cycle_ms", rep.CycleDurationMS).
		Str("end_reason", rep.BattleEndReason).
		Msg("battle complete")

	b.logger.Info().
		Int32("attacks", b.attackCount.Load()).
		Str("stars", fmt.Sprintf("3⭐:%d | 2⭐:%d | 1⭐:%d | 0⭐:%d", b.stars3.Load(), b.stars2.Load(), b.stars1.Load(), b.stars0.Load())).
		Str("loot", fmt.Sprintf("Gold: %d | Elixir: %d | DE: %d", b.totalGold.Load(), b.totalElixir.Load(), b.totalDE.Load())).
		Dur("uptime", time.Since(b.startedAt)).
		Msg("=== SESSION SUMMARY ===")

	b.zoomedOut.Store(false)
}

type buttonColorSpec struct {
	low, high    gocv.Scalar
	minW, minH   int
	minArea      float64
	halfW, halfH int
}

func (b *Bot) buttonColorSpec(name string) (buttonColorSpec, bool) {
	switch name {
	case "Attack":
		return buttonColorSpec{
			low: gocv.NewScalar(0, 70, 110, 0),
			high: gocv.NewScalar(200, 255, 255, 0),
			minW: 18, minH: 12, minArea: 180,
			halfW: 70, halfH: 55,
		}, true
	case "Find Match":
		return buttonColorSpec{
			low: gocv.NewScalar(0, 70, 110, 0),
			high: gocv.NewScalar(210, 255, 255, 0),
			minW: 55, minH: 24, minArea: 900,
			halfW: 150, halfH: 90,
		}, true
	case "Battle Attack":
		return buttonColorSpec{
			low: gocv.NewScalar(0, 110, 70, 0),
			high: gocv.NewScalar(170, 255, 210, 0),
			minW: 70, minH: 24, minArea: 1100,
			halfW: 150, halfH: 90,
		}, true
	case "Next":
		return buttonColorSpec{
			low: gocv.NewScalar(0, 85, 145, 0),
			high: gocv.NewScalar(190, 255, 255, 0),
			minW: 45, minH: 22, minArea: 700,
			halfW: 120, halfH: 95,
		}, true
	default:
		return buttonColorSpec{}, false
	}
}

func locateColoredButtonNear(screen gocv.Mat, center image.Point, spec buttonColorSpec) (int, int, bool) {
	if screen.Empty() {
		return 0, 0, false
	}
	x0 := center.X - spec.halfW
	x1 := center.X + spec.halfW
	y0 := center.Y - spec.halfH
	y1 := center.Y + spec.halfH
	if x0 < 0 { x0 = 0 }
	if y0 < 0 { y0 = 0 }
	if x1 > screen.Cols() { x1 = screen.Cols() }
	if y1 > screen.Rows() { y1 = screen.Rows() }
	if x1-x0 < spec.minW || y1-y0 < spec.minH {
		return 0, 0, false
	}

	roi := screen.Region(image.Rect(x0, y0, x1, y1))
	defer roi.Close()
	mask := vision.GetMat(roi.Rows(), roi.Cols(), gocv.MatTypeCV8UC1)
	defer vision.PutMat(mask)

	gocv.InRangeWithScalar(roi, spec.low, spec.high, &mask)
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
		if rect.Dx() < spec.minW || rect.Dy() < spec.minH {
			continue
		}
		bestArea = area
		bestRect = rect
	}
	if bestArea < spec.minArea || bestRect.Empty() {
		return 0, 0, false
	}
	return x0 + bestRect.Min.X + bestRect.Dx()/2,
		y0 + bestRect.Min.Y + bestRect.Dy()/2,
		true
}

func (b *Bot) rememberedUIAnchor(name string) (image.Point, bool) {
	b.uiAnchorMu.RLock()
	defer b.uiAnchorMu.RUnlock()
	pt, ok := b.uiAnchors[name]
	return pt, ok
}

func (b *Bot) rememberUIAnchor(name string, pt image.Point) {
	b.uiAnchorMu.Lock()
	b.uiAnchors[name] = pt
	b.uiAnchorMu.Unlock()
}

func (b *Bot) locateRememberedButton(name string, screen gocv.Mat) (int, int, bool) {
	spec, ok := b.buttonColorSpec(name)
	if !ok {
		return 0, 0, false
	}

	// A poor local hit-rate means the current UI layout/animation no longer
	// matches what was learned. Temporarily bypass the cache; the full locator
	// remains authoritative and will keep updating anchors after verified hits.
	if until := b.uiAnchorDisabledUntilUS.Load(); until > time.Now().UnixMicro() {
		return 0, 0, false
	}

	pt, ok := b.rememberedUIAnchor(name)
	if !ok {
		return 0, 0, false
	}
	attempts := b.uiAnchorAttempts.Add(1)
	x, y, found := locateColoredButtonNear(screen, pt, spec)
	if found {
		b.uiAnchorHits.Add(1)
		return x, y, true
	}

	fallbacks := b.uiAnchorFallbacks.Add(1)
	hits := b.uiAnchorHits.Load()
	if attempts >= 20 {
		hitRate := float64(hits) * 100 / float64(attempts)
		if hitRate < 50 {
			b.uiAnchorDisabledUntilUS.Store(time.Now().Add(2 * time.Minute).UnixMicro())
			b.logger.Debug().
				Int64("attempts", attempts).
				Int64("hits", hits).
				Int64("fallbacks", fallbacks).
				Float64("hit_rate", hitRate).
				Msg("verified UI anchor cache temporarily disabled; full locator remains authoritative")
		}
	}
	return 0, 0, false
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
		first, err := b.runtimeFrameFresh(2 * time.Second)
		if err != nil || first.Empty() {
			if !first.Empty() { first.Close() }
			time.Sleep(70 * time.Millisecond)
			continue
		}
		x1, y1, ok1 := b.locateRememberedButton(name, first)
		if !ok1 {
			x1, y1, ok1 = locator(first)
		}
		first.Close()
		if !ok1 {
			time.Sleep(70 * time.Millisecond)
			continue
		}

		// Let the button finish a few animation frames, then confirm its center.
		time.Sleep(85 * time.Millisecond)
		second, err := b.runtimeFrameFresh(2 * time.Second)
		if err != nil || second.Empty() {
			if !second.Empty() { second.Close() }
			continue
		}
		x2, y2, ok2 := 0, 0, false
		if spec, specOK := b.buttonColorSpec(name); specOK {
			x2, y2, ok2 = locateColoredButtonNear(second, image.Pt(x1, y1), spec)
		}
		if !ok2 {
			x2, y2, ok2 = locator(second)
		}
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
		b.rememberUIAnchor(name, image.Pt(x, y))
		b.logger.Debug().
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
func (b *Bot) waitForStableLocator(name string, locator func(gocv.Mat) (int, int, bool), timeout time.Duration) (int, int, bool) {
	deadline := time.Now().Add(timeout)
	pollPause := chooseSearchPacing(b.client.Health()).PrepPollPause
	if pollPause <= 0 {
		pollPause = 100 * time.Millisecond
	}
	var lastX, lastY int
	stable := 0

	for time.Now().Before(deadline) {
		screen, err := b.runtimeFrameFresh(2 * time.Second)
		if err != nil || screen.Empty() {
			if !screen.Empty() { screen.Close() }
			time.Sleep(pollPause)
			continue
		}
		x, y, ok := locator(screen)
		screen.Close()
		if !ok {
			stable = 0
			time.Sleep(pollPause)
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
			b.logger.Debug().Str("target", name).Int("x", x).Int("y", y).Msg("next UI target is stable and ready")
			return x, y, true
		}
		time.Sleep(pollPause)
	}
	b.logger.Warn().Str("target", name).Dur("timeout", timeout).Msg("next UI target did not become stable in time")
	return 0, 0, false
}

func (b *Bot) clickSequence() bool {
	// Xingchen-style navigation: every action requires fresh visual evidence.
	// No blind coordinate progression, no stacked retry loops, no duplicate
	// capture owner while the attack sequence is active.
	b.lastPrepTimings = PreparationTimings{}

	stepStarted := time.Now()
	if !b.waitAndClickButton("btn_attack", "Attack", 2500*time.Millisecond) {
		b.captureFailureDiagnostic("click_attack_failed", nil)
		return false
	}
	b.lastPrepTimings.AttackButtonMS = time.Since(stepStarted).Milliseconds()

	stepStarted = time.Now()
	if !b.waitAndClickButton("btn_find_match", "Find Match", 4500*time.Millisecond) {
		b.captureFailureDiagnostic("click_find_match_failed", nil)
		return false
	}
	b.lastPrepTimings.FindMatchMS = time.Since(stepStarted).Milliseconds()

	// Some current CoC layouts enter matchmaking directly after Find Match.
	// Preserve the Xingchen evidence-first flow, but do not insist on Army
	// Arrow when the game has already progressed into clouds/base search.
	probeDeadline := time.Now().Add(3500 * time.Millisecond)
	for time.Now().Before(probeDeadline) {
		screen, err := b.runtimeFrameFresh(2 * time.Second)
		if err != nil {
			if !b.sleepResponsive(250 * time.Millisecond) {
				return false
			}
			continue
		}
		if screen.Empty() {
			screen.Close()
			if !b.sleepResponsive(250 * time.Millisecond) {
				return false
			}
			continue
		}
		state, _ := b.classify(screen)
		screen.Close()

		switch state {
		case game.StateBattle, game.StateSearchMap, game.StateLoading:
			b.logger.Info().Str("state", state.String()).Msg("matchmaking transition confirmed; skipping army picker")
			stepStarted = time.Now()
			ready := b.waitForBattleState(60 * time.Second)
			b.lastPrepTimings.MatchmakingReadyMS = time.Since(stepStarted).Milliseconds()
			return ready
		case game.StateArmySelection, game.StateArmyCamp:
			probeDeadline = time.Now()
		default:
			if !b.sleepResponsive(250 * time.Millisecond) {
				return false
			}
		}
	}

	stepStarted = time.Now()
	if !b.waitAndClickButton("btn_army_arrow", "Army Arrow", 4500*time.Millisecond) {
		b.captureFailureDiagnostic("click_army_arrow_failed", nil)
		return false
	}
	b.lastPrepTimings.ArmyMenuMS = time.Since(stepStarted).Milliseconds()

	stepStarted = time.Now()
	armyClicked := false
	if b.armySlot <= 1 {
		armyClicked = b.waitAndClickButton("btn_army_1", "Army 1", 4000*time.Millisecond)
	} else if b.waitForUIEvidence("btn_army_1", game.StateArmySelection, 4000*time.Millisecond) {
		armyClicked = b.selectArmySlot()
	}
	if !armyClicked {
		b.captureFailureDiagnostic("click_army_slot_not_found", map[string]interface{}{"army_slot": b.armySlot})
		return false
	}
	b.lastPrepTimings.ArmySlotMS = time.Since(stepStarted).Milliseconds()

	stepStarted = time.Now()
	if !b.waitAndClickButton("btn_battle", "Battle", 5000*time.Millisecond) {
		b.captureFailureDiagnostic("click_battle_failed", nil)
		return false
	}
	b.lastPrepTimings.BattleButtonMS = time.Since(stepStarted).Milliseconds()

	b.logger.Info().Msg("battle requested; waiting for search/base state...")
	stepStarted = time.Now()
	ready := b.waitForBattleState(60 * time.Second)
	b.lastPrepTimings.MatchmakingReadyMS = time.Since(stepStarted).Milliseconds()
	return ready
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
		return b.findAndClick("btn_army_1", "Army 1", 1)
	}

	// Card rows in the saved-recipes list (reference resolution).
	// cardY is the vertical center of the Nth card body; tapping the
	// card body sets it as the active army (verified live: slot 4 at
	// y≈403 fired the "Recipe ... set as active army!" toast).
	cardY := 227 + (slot-1)*54
	tapX, tapY := b.cal.ScaleRef(430, cardY)

	b.logger.Debug().Int("army_slot", slot).Int("x", tapX).Int("y", tapY).Msg("selecting saved army recipe card")
	if err := b.client.TapFast(tapX, tapY, 1.2); err != nil {
		b.logger.Warn().Err(err).Msg("army recipe card tap failed")
		return false
	}
	// Do not sleep here: clickSequence immediately waits for Battle to become
	// stable on two fresh frames before clicking it. An extra fixed delay here
	// would only slow slots 2+ without adding another verification boundary.
	b.recordActivity()
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
		screen, err := b.runtimeFrameFresh(2 * time.Second)
		if err != nil {
			b.logger.Warn().Err(err).Str("step", stepName).Msg("capture failed")
			time.Sleep(500 * time.Millisecond)
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
				b.logger.Debug().Str("step", stepName).Msg("secondary pinpoint match (upper battle), clicking...")
				if err := b.client.TapFast(altX, altY, 0.6); err == nil {
					b.recordActivity()
					return true
				}
				screen, _ = b.runtimeFrameFresh(2 * time.Second)
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
			time.Sleep(800 * time.Millisecond)
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

		b.logger.Debug().
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

// waitForStableResultPanel replaces the old blind 1.8s result-screen sleep
// with a bounded visual settle. It can finish sooner on fast devices, but it
// never waits longer than maxWait and never changes result parsing rules.
func (b *Bot) waitForStableResultPanel(maxWait time.Duration) (time.Duration, bool) {
	started := time.Now()
	if maxWait <= 0 {
		return 0, false
	}

	// The overlay needs a short guaranteed paint window before frame stability
	// is meaningful. This is still far below the historical 1.8s blind sleep.
	initial := 600 * time.Millisecond
	if initial > maxWait {
		initial = maxWait
	}
	select {
	case <-time.After(initial):
	case <-b.ctx.Done():
		return time.Since(started), false
	}

	var previous uint64
	for time.Since(started) < maxWait {
		screen, err := b.runtimeFrameFresh(2 * time.Second)
		if err == nil && !screen.Empty() {
			hash := resultPanelHash(screen, b.cal)
			screen.Close()
			if hash != 0 && hash == previous {
				return time.Since(started), true
			}
			previous = hash
		} else if !screen.Empty() {
			screen.Close()
		}

		remaining := maxWait - time.Since(started)
		if remaining <= 0 {
			break
		}
		pause := 250 * time.Millisecond
		if pause > remaining {
			pause = remaining
		}
		select {
		case <-time.After(pause):
		case <-b.ctx.Done():
			return time.Since(started), false
		}
	}
	return time.Since(started), false
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
	screen, err := b.runtimeFrameFresh(2 * time.Second)
	if err != nil {
		return
	}
	state, _ := b.classify(screen)
	screen.Close()
	b.dismissInterruptionState(state)
}

func (b *Bot) dismissInterruptionState(state game.GameState) {
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
		screen, err := b.runtimeFrameFresh(2 * time.Second)
		if err != nil {
			b.logger.Debug().Err(err).Msg("battle-state capture unavailable")
			if !b.sleepResponsive(350 * time.Millisecond) {
				return false
			}
			continue
		}
		if screen.Empty() {
			screen.Close()
			if !b.sleepResponsive(350 * time.Millisecond) {
				return false
			}
			continue
		}

		b.captureHeartbeat.Store(time.Now().UnixNano())
		state, _ := b.classify(screen)
		screen.Close()

		switch state {
		case game.StateBattle:
			b.logger.Info().Msg("battle state detected, entering search loop")
			return true
		case game.StateMainVillage:
			b.logger.Warn().Msg("returned to village while waiting for battle; aborting navigation")
			return false
		case game.StateConnectionLost:
			b.logger.Warn().Msg("connection lost while entering battle; clearing dialog")
			b.dismissInterruptions()
			if !b.sleepResponsive(500 * time.Millisecond) {
				return false
			}
		case game.StateSearchMap, game.StateLoading:
			b.logger.Debug().Str("state", state.String()).Msg("matchmaking in progress")
			if !b.sleepResponsive(650 * time.Millisecond) {
				return false
			}
		case game.StateArmySelection, game.StateArmyCamp:
			b.logger.Info().Msg("army UI still visible; retrying verified Battle button")
			_ = b.waitAndClickButton("btn_battle", "Battle Retry", 1500*time.Millisecond)
			if !b.sleepResponsive(400 * time.Millisecond) {
				return false
			}
		default:
			b.logger.Debug().Str("state", state.String()).Msg("waiting for battle/search state")
			if isTransientRuntimeState(state) {
				b.dismissInterruptions()
			}
			if !b.sleepResponsive(400 * time.Millisecond) {
				return false
			}
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
	return b.deployParsedStrategy(screen, strat)
}

func (b *Bot) deployParsedStrategy(screen gocv.Mat, strat *strategy.DynamicStrategy) (int, error) {
	if strat == nil {
		return 0, fmt.Errorf("nil dynamic strategy")
	}

	b.logger.Info().
		Str("strategy", strat.Name).
		Int("phases", len(strat.Phases)).
		Msg("executing dynamic attack plan")

	time.Sleep(150 * time.Millisecond)

	remaining, err := b.attackExec.DeployDynamicV2(strat, screen, b.cfg.Attack.StrategyFile)
	if err != nil {
		b.logger.Error().Err(err).Msg("dynamic deploy failed: " + err.Error())
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

	screen, err := b.runtimeFrameFresh(2 * time.Second)
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

type MultiAccountRuntimeStatus struct {
	Enabled            bool      `json:"enabled"`
	ActiveAccountID    string    `json:"active_account_id,omitempty"`
	ActiveAccountLabel string    `json:"active_account_label,omitempty"`
	AttacksThisTurn    int       `json:"attacks_this_turn"`
	NextAccountID      string    `json:"next_account_id,omitempty"`
	NextAccountLabel   string    `json:"next_account_label,omitempty"`
	RotationDue        bool      `json:"rotation_due"`
	TotalSwitches      int       `json:"total_switches"`
	LastSwitchAt       time.Time `json:"last_switch_at,omitempty"`
	LastError          string    `json:"last_error,omitempty"`
}

func (b *Bot) MultiAccountStatus() MultiAccountRuntimeStatus {
	if b == nil || b.multiAccount == nil {
		return MultiAccountRuntimeStatus{}
	}
	st := b.multiAccount.State()
	out := MultiAccountRuntimeStatus{
		Enabled:         b.multiAccount.Enabled(),
		ActiveAccountID: st.ActiveAccountID,
		AttacksThisTurn: st.AttacksThisTurn,
		TotalSwitches:   st.TotalSwitches,
		LastSwitchAt:    st.LastSwitchAt,
		LastError:       st.LastError,
	}
	if active, ok := b.multiAccount.Active(); ok {
		out.ActiveAccountLabel = active.Label
	}
	if next, due := b.multiAccount.NextDue(); due {
		out.RotationDue = true
		out.NextAccountID = next.ID
		out.NextAccountLabel = next.Label
	}
	return out
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
	if cfg == nil {
		return
	}
	b.cfg = cfg
	if b.multiAccount != nil {
		if err := b.multiAccount.UpdateConfig(cfg.Account.MultiAccount, cfg.Account.PlayerTag); err != nil {
			b.logger.Warn().Err(err).Msg("multi-account scheduler config update failed")
		}
	}
	if b.attackExec != nil {
		b.attackExec.UpdateConfig(&cfg.Attack)
		b.attackExec.SetArmyGuardEnabled(cfg.Automation.AutoArmyGuard)
	}
	if b.governor != nil {
		// Preserve the rolling attack timestamps and circuit-breaker state
		// while applying member pacing changes live.
		b.governor.UpdateConfig(cfg.Automation)
	}
	if !cfg.Upgrade.UpgradeWalls {
		b.wallUpgradePending.Store(false)
	}

	if b.navigator != nil {
		b.navigator.SetDisableChestDismissal(cfg.Device.DisableChestDismissal)
	}
	b.logger.Info().Msg("bot configuration updated in real-time")
}


func (b *Bot) IsSequenceRunning() bool {
	if b == nil {
		return false
	}
	return b.seqRunning.Load()
}

func (b *Bot) SequenceStartedAtUnix() int64 {
	if b == nil {
		return 0
	}
	return b.seqStartedAtUnix.Load()
}

func (b *Bot) PauseAutomation() {
	if b == nil {
		return
	}
	b.paused.Store(true)
	b.logger.Info().Bool("sequence_running", b.seqRunning.Load()).Msg("automation pause requested")
}

func (b *Bot) ResumeAutomation() {
	if b == nil {
		return
	}
	if b.paused.Swap(false) {
		b.recordActivity()
		b.logger.Info().Msg("automation resumed")
	}
}

func (b *Bot) IsPaused() bool {
	return b != nil && b.paused.Load()
}

func contextualOutcomeFromReport(rep AttackReport, townHall, recoveryCount, blueStacksRestart int) intelligence.ContextualOutcome {
	at, _ := time.Parse(time.RFC3339, rep.Timestamp)
	return intelligence.ContextualOutcome{
		Context: intelligence.AttackContext{
			Strategy: rep.Strategy,
			TownHall: townHall,
			TargetScore: rep.TargetScore,
			TargetGold: rep.TargetGold,
			TargetElixir: rep.TargetElixir,
			TargetDE: rep.TargetDE,
		},
		Edge: rep.TargetEdge,
		Stars: rep.Stars,
		DestructionPct: rep.DestructionPct,
		GoldStolen: rep.GoldStolen,
		ElixirStolen: rep.ElixirStolen,
		DarkElixirStolen: rep.DarkElixirStolen,
		CycleDurationMS: rep.CycleDurationMS,
		FullRoutineDurationMS: rep.FullRoutineDurationMS,
		SearchDurationMS: rep.SearchDurationMS,
		SearchSkips: rep.SearchSkips,
		BattleDurationMS: rep.BattleDurationMS,
		DeploySuccess: rep.DeploySuccess,
		ReturnHomeSuccess: rep.ReturnHomeSuccess,
		SafeDeployment: rep.RedZoneValid && rep.HUDSafe,
		ParsedResults: rep.ParsedResults,
		RecoveryCount: recoveryCount,
		BlueStacksRestart: blueStacksRestart,
		At: at,
	}
}

// HistorySnapshot returns an immutable copy of the bot's authoritative
// in-memory attack history. The App uses this to update React immediately at
// attack boundaries without writing then re-reading attack_history.json.
func (b *Bot) HistorySnapshot() []AttackReport {
	if b == nil {
		return []AttackReport{}
	}
	b.historyMu.RLock()
	defer b.historyMu.RUnlock()
	out := make([]AttackReport, len(b.historyCache))
	copy(out, b.historyCache)
	return out
}

// RecordMemberSettingsChange adds one low-volume, user-initiated settings
// event to the activity feed. It does not affect the farming state machine.
func (b *Bot) RecordMemberSettingsChange(profile string, maxAttacksPerHour, maxAttacksPerSession, breakEvery, breakMinutes int) {
	if b == nil || b.telemetry == nil {
		return
	}
	b.telemetry.Emit(telemetry.EventSpeedProfile, map[string]any{
		"profile":                 profile,
		"max_attacks_per_hour":    maxAttacksPerHour,
		"max_attacks_per_session": maxAttacksPerSession,
		"break_every_attacks":     breakEvery,
		"break_minutes":        breakMinutes,
	})
}

// RecentActivity returns a compact high-level activity feed for the UI.
// It deliberately excludes per-frame telemetry and verbose diagnostic logs.
func (b *Bot) RecentActivity(limit int) []telemetry.Event {
	if b == nil || b.telemetry == nil {
		return []telemetry.Event{}
	}
	return b.telemetry.Recent(limit)
}

func (b *Bot) Stats() BotStats {
	uptime := time.Since(b.startedAt)
	hours := uptime.Hours()
	attacks := b.attackCount.Load()
	var goldPerHour, elixirPerHour, dePerHour, avgStars, threeStarRate float64
	if hours > 0 {
		goldPerHour = float64(b.totalGold.Load()) / hours
		elixirPerHour = float64(b.totalElixir.Load()) / hours
		dePerHour = float64(b.totalDE.Load()) / hours
	}
	if attacks > 0 {
		avgStars = float64(b.totalStars.Load()) / float64(attacks)
		threeStarRate = float64(b.stars3.Load()) * 100 / float64(attacks)
	}
	tm := telemetry.Snapshot{}
	if b.telemetry != nil { tm = b.telemetry.Snapshot() }
	scaleStats := vision.PreferredScaleRuntimeStats()
	scaleHitRate := 0.0
	if scaleStats.Attempts > 0 {
		scaleHitRate = float64(scaleStats.Hits) * 100 / float64(scaleStats.Attempts)
	}
	avgReturnHomeMS := 0.0
	if count := b.returnHomeCount.Load(); count > 0 {
		avgReturnHomeMS = float64(b.returnHomeMicros.Load()) / float64(count) / 1000.0
	}
	var targetAcceptanceRate, avgSkipsPerAttack, recoverySuccessRate float64
	if tm.TargetsFound > 0 {
		targetAcceptanceRate = float64(tm.TargetsAccepted) * 100 / float64(tm.TargetsFound)
	}
	if attacks > 0 {
		avgSkipsPerAttack = float64(tm.TargetsSkipped) / float64(attacks)
	}
	if attempts := b.recoveryAttempts.Load(); attempts > 0 {
		recoverySuccessRate = float64(b.recoverySuccesses.Load()) * 100 / float64(attempts)
	}
	adbHealth := b.client.Health()
	uiAnchorAttempts := b.uiAnchorAttempts.Load()
	uiAnchorHits := b.uiAnchorHits.Load()
	uiAnchorDisabled := b.uiAnchorDisabledUntilUS.Load() > time.Now().UnixMicro()
	uiAnchorHitRate := 0.0
	if uiAnchorAttempts > 0 {
		uiAnchorHitRate = float64(uiAnchorHits) * 100 / float64(uiAnchorAttempts)
	}
	healthScore := 100
	healthScore -= adbHealth.ConsecutiveFails * 8
	failedRecoveries := int(b.recoveryAttempts.Load() - b.recoverySuccesses.Load())
	if failedRecoveries > 0 { healthScore -= failedRecoveries * 6 }
	healthScore -= int(b.blueStacksRestarts.Load()) * 2
	captureHealthMS := adbHealth.AvgCaptureMs
	if adbHealth.FastCaptureMs > captureHealthMS {
		captureHealthMS = adbHealth.FastCaptureMs
	}
	if captureHealthMS > 1200 { healthScore -= 15 } else if captureHealthMS > 700 { healthScore -= 7 }
	if healthScore < 0 { healthScore = 0 }
	if healthScore > 100 { healthScore = 100 }

	runtimeSearchMode := chooseSearchPacing(adbHealth).Mode
	if b.safePacingForced() {
		runtimeSearchMode = "Safe"
	}
	memberSpeedProfile := strings.ToLower(strings.TrimSpace(b.cfg.Automation.SpeedProfile))
	switch memberSpeedProfile {
	case "cautious", "fast":
	default:
		memberSpeedProfile = "normal"
	}

	return BotStats{
		AttacksCompleted: b.attackCount.Load(),
		SessionAttacks:   b.attackCount.Load(),
		SessionAttackCap: b.cfg.Attack.MaxAttackPerSession,
		SearchSkips:      b.skipsCount.Load(),
		TotalGold:        b.totalGold.Load(),
		TotalElixir:      b.totalElixir.Load(),
		TotalDE:          b.totalDE.Load(),
		Stars0:           b.stars0.Load(),
		Stars1:           b.stars1.Load(),
		Stars2:           b.stars2.Load(),
		Stars3:           b.stars3.Load(),
		Uptime:           uptime,
		AdbHealth:          adbHealth,
		CPUTimeSec:         CPUTime().Seconds(),
		CPUCores:           b.cpuSampler.Usage(),
		RecoveryAttempts:   b.recoveryAttempts.Load(),
		RecoverySuccesses:  b.recoverySuccesses.Load(),
		BlueStacksRestarts: b.blueStacksRestarts.Load(),
		CleanAttackStreak:  b.cleanAttackStreak.Load(),
		AttackSoakValidated: b.soakValidated.Load(),
		GoldPerHour:        goldPerHour,
		ElixirPerHour:      elixirPerHour,
		DEPerHour:          dePerHour,
		AverageStars:       avgStars,
		ThreeStarRate:      threeStarRate,
		AverageCaptureMS:   tm.AvgCaptureMS,
		LastCaptureMS:      tm.LastCaptureMS,
		TelemetryEvents:    tm.Events,
		Anomalies:          tm.Anomalies,
		TargetsSkipped:     tm.TargetsSkipped,
		HealthScore:          healthScore,
		SpeedProfile:         runtimeSearchMode,
		MemberSpeedProfile:   memberSpeedProfile,
		TargetsSeen:          tm.TargetsFound,
		TargetsAccepted:      tm.TargetsAccepted,
		TargetAcceptanceRate: targetAcceptanceRate,
		AvgSkipsPerAttack:    avgSkipsPerAttack,
		RecoverySuccessRate:  recoverySuccessRate,
		AverageTargetScanMS:  tm.AvgTargetScanMS,
		LastTargetScanMS:     tm.LastTargetScanMS,
		AverageReturnHomeMS:  avgReturnHomeMS,
		LastReturnHomeMS:     float64(b.lastReturnHomeUS.Load()) / 1000.0,
		AverageNextTransitionMS: tm.AvgNextTransitionMS,
		LastNextTransitionMS:    tm.LastNextTransitionMS,
		NextTransitions:         tm.NextTransitions,
		NextRetries:             tm.NextRetries,
		NextFirstPassRate:       tm.NextFirstPassRate,
		AvgNextVerifyProbes:     tm.AvgNextVerifyProbes,
		AvgAcceptedGE:           tm.AvgAcceptedGE,
		AvgRejectedGE:           tm.AvgRejectedGE,
		AvgAcceptedDE:           tm.AvgAcceptedDE,
		AvgRejectedDE:           tm.AvgRejectedDE,
		AvgAcceptedScore:        tm.AvgAcceptedScore,
		AvgRejectedScore:        tm.AvgRejectedScore,
		PreferredScaleAttempts:  scaleStats.Attempts,
		PreferredScaleHits:      scaleStats.Hits,
		PreferredScaleFallbacks: scaleStats.Fallbacks,
		PreferredScaleHitRate:   scaleHitRate,
		PreferredScaleEnabled:   scaleStats.Enabled,
		UIAnchorAttempts:        uiAnchorAttempts,
		UIAnchorHits:            uiAnchorHits,
		UIAnchorFallbacks:       b.uiAnchorFallbacks.Load(),
		UIAnchorHitRate:         uiAnchorHitRate,
		UIAnchorEnabled:         !uiAnchorDisabled,
		NearMissTargets:         tm.NearMissTargets,
		NearMiss5Targets:        tm.NearMiss5Targets,
		NearMiss10Targets:       tm.NearMiss10Targets,
		NearMiss15Targets:       tm.NearMiss15Targets,
		TopRejectedTargets:      tm.TopRejectedTargets,
	}
}

type BotStats struct {
	AttacksCompleted int32         `json:"attacks_completed"`
	SessionAttacks   int32         `json:"session_attacks"`
	SessionAttackCap int           `json:"session_attack_cap"`
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

	RecoveryAttempts    int32 `json:"recovery_attempts"`
	RecoverySuccesses   int32 `json:"recovery_successes"`
	BlueStacksRestarts  int32 `json:"bluestacks_restarts"`
	CleanAttackStreak   int32 `json:"clean_attack_streak"`
	AttackSoakValidated bool  `json:"attack_soak_validated"`

	GoldPerHour      float64 `json:"gold_per_hour"`
	ElixirPerHour    float64 `json:"elixir_per_hour"`
	DEPerHour        float64 `json:"de_per_hour"`
	AverageStars     float64 `json:"average_stars"`
	ThreeStarRate    float64 `json:"three_star_rate"`
	AverageCaptureMS float64 `json:"average_capture_ms"`
	LastCaptureMS    float64 `json:"last_capture_ms"`
	TelemetryEvents  int64   `json:"telemetry_events"`
	Anomalies        int64   `json:"anomalies"`
	TargetsSkipped   int64   `json:"targets_skipped"`
	HealthScore          int     `json:"health_score"`
	SpeedProfile         string  `json:"speed_profile"`
	MemberSpeedProfile   string  `json:"member_speed_profile"`
	TargetsSeen          int64   `json:"targets_seen"`
	TargetsAccepted      int64   `json:"targets_accepted"`
	TargetAcceptanceRate float64 `json:"target_acceptance_rate"`
	AvgSkipsPerAttack    float64 `json:"avg_skips_per_attack"`
	RecoverySuccessRate  float64 `json:"recovery_success_rate"`
	AverageTargetScanMS  float64 `json:"average_target_scan_ms"`
	LastTargetScanMS     float64 `json:"last_target_scan_ms"`
	AverageReturnHomeMS  float64 `json:"average_return_home_ms"`
	LastReturnHomeMS        float64 `json:"last_return_home_ms"`
	AverageNextTransitionMS float64 `json:"average_next_transition_ms"`
	LastNextTransitionMS    float64 `json:"last_next_transition_ms"`
	NextTransitions         int64   `json:"next_transitions"`
	NextRetries             int64   `json:"next_retries"`
	NextFirstPassRate       float64 `json:"next_first_pass_rate"`
	AvgNextVerifyProbes     float64 `json:"avg_next_verify_probes"`
	AvgAcceptedGE           float64 `json:"avg_accepted_ge"`
	AvgRejectedGE           float64 `json:"avg_rejected_ge"`
	AvgAcceptedDE           float64 `json:"avg_accepted_de"`
	AvgRejectedDE           float64 `json:"avg_rejected_de"`
	AvgAcceptedScore        float64 `json:"avg_accepted_score"`
	AvgRejectedScore        float64 `json:"avg_rejected_score"`
	PreferredScaleAttempts  int64   `json:"preferred_scale_attempts"`
	PreferredScaleHits      int64   `json:"preferred_scale_hits"`
	PreferredScaleFallbacks int64   `json:"preferred_scale_fallbacks"`
	PreferredScaleHitRate   float64 `json:"preferred_scale_hit_rate"`
	PreferredScaleEnabled   bool    `json:"preferred_scale_enabled"`
	UIAnchorAttempts        int64   `json:"ui_anchor_attempts"`
	UIAnchorHits            int64   `json:"ui_anchor_hits"`
	UIAnchorFallbacks       int64   `json:"ui_anchor_fallbacks"`
	UIAnchorHitRate         float64                          `json:"ui_anchor_hit_rate"`
	UIAnchorEnabled         bool                             `json:"ui_anchor_enabled"`
	NearMissTargets         int64                            `json:"near_miss_targets"`
	NearMiss5Targets        int64                            `json:"near_miss_5_targets"`
	NearMiss10Targets       int64                            `json:"near_miss_10_targets"`
	NearMiss15Targets       int64                            `json:"near_miss_15_targets"`
	TopRejectedTargets      []telemetry.RejectedTargetSample `json:"top_rejected_targets,omitempty"`
}

type AttackReport struct {
	Timestamp        string `json:"timestamp"`
	SessionID        string `json:"session_id,omitempty"`
	AccountID        string `json:"account_id,omitempty"`
	PlayerTag        string `json:"player_tag,omitempty"`
	Strategy         string `json:"strategy"`
	TargetEdge       string `json:"target_edge"`
	DeploySide       string `json:"deploy_side"`
	DeploySuccess    bool   `json:"deploy_success"`
	UndeployedSlots  int    `json:"undeployed_slots"`
	DeployError      string `json:"deploy_error,omitempty"`
	ParsedResults    bool   `json:"parsed_results"`
	StarsSource      string `json:"stars_source"`
	LootSource       string `json:"loot_source"`
	ResultConfidence string `json:"result_confidence"`
	Stars            int    `json:"stars"`
	GoldStolen       int    `json:"gold_stolen"`
	ElixirStolen     int    `json:"elixir_stolen"`
	DarkElixirStolen int    `json:"dark_elixir_stolen"`
	BonusGold        int    `json:"bonus_gold"`
	BonusElixir      int    `json:"bonus_elixir"`
	BonusDE          int    `json:"bonus_de"`
	TotalAttacks     int32  `json:"total_attacks_session"`
	SearchSkips      int    `json:"search_skips"`
	SearchDurationMS int64  `json:"search_duration_ms"`
	CycleDurationMS  int64  `json:"cycle_duration_ms"`
	DeployDurationMS int64  `json:"deploy_duration_ms"`
	BattleDurationMS int64  `json:"battle_duration_ms"`
	TargetGold       int    `json:"target_gold"`
	TargetElixir     int    `json:"target_elixir"`
	TargetDE         int    `json:"target_de"`
	TargetScore      int     `json:"target_score"`
	RuntimeMode      string  `json:"runtime_mode"`
	CaptureMS        float64 `json:"capture_ms"`
	TargetScanMS          float64 `json:"target_scan_ms"`
	LiveBarRescans       int     `json:"live_bar_rescans"`
	AvgLiveBarRescanMS   float64 `json:"avg_live_bar_rescan_ms"`
	AvgSlotDetectMS       float64 `json:"avg_slot_detect_ms"`
	AvgSlotClassifyMS     float64 `json:"avg_slot_classify_ms"`
	TemplatesTried        int     `json:"templates_tried"`
	TemplatesMatched      int     `json:"templates_matched"`
	AvgSelectedCardOCRMS float64 `json:"avg_selected_card_ocr_ms"`
	BattleLootOCRSamples int     `json:"battle_loot_ocr_samples"`
	AvgBattleLootOCRMS   float64 `json:"avg_battle_loot_ocr_ms"`
	BattleEndWaitMS      int64   `json:"battle_end_wait_ms"`
	LootExitPercent      int     `json:"loot_exit_percent"`
	PreparationDurationMS  int64 `json:"preparation_duration_ms"`
	PrepAttackButtonMS     int64 `json:"prep_attack_button_ms"`
	PrepFindMatchMS        int64 `json:"prep_find_match_ms"`
	PrepArmyMenuMS         int64 `json:"prep_army_menu_ms"`
	PrepArmySlotMS         int64 `json:"prep_army_slot_ms"`
	PrepBattleButtonMS     int64 `json:"prep_battle_button_ms"`
	PrepMatchmakingReadyMS int64 `json:"prep_matchmaking_ready_ms"`
	CooldownDurationMS     int64 `json:"cooldown_duration_ms"`
	BattleEndReason   string  `json:"battle_end_reason"`
	DestructionPct    int     `json:"destruction_pct"`
	TownHallDestroyed     bool  `json:"town_hall_destroyed"`
	SafetyMode            string `json:"safety_mode"`
	RedZoneValid          bool   `json:"red_zone_valid"`
	CorridorVerified      bool   `json:"corridor_verified"`
	HUDSafe               bool   `json:"hud_safe"`
	RedZoneX1             int    `json:"red_zone_x1"`
	RedZoneY1             int    `json:"red_zone_y1"`
	RedZoneX2             int    `json:"red_zone_x2"`
	RedZoneY2             int    `json:"red_zone_y2"`
	DeployLineX1          int    `json:"deploy_line_x1"`
	DeployLineY1          int    `json:"deploy_line_y1"`
	DeployLineX2          int    `json:"deploy_line_x2"`
	DeployLineY2          int    `json:"deploy_line_y2"`
	DeployFreeSpace       int    `json:"deploy_free_space"`
	ReturnHomeDurationMS  int64 `json:"return_home_duration_ms"`
	ReturnHomeSuccess     bool  `json:"return_home_success"`
	FullRoutineDurationMS int64 `json:"full_routine_duration_ms"`
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
