package com.reqngo.manager;

import android.accessibilityservice.AccessibilityService;
import android.accessibilityservice.GestureDescription;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.Path;
import android.graphics.Point;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import android.util.Log;
import android.view.Display;
import android.view.accessibility.AccessibilityEvent;
import android.view.accessibility.AccessibilityNodeInfo;
import android.content.SharedPreferences;

import java.io.File;
import java.io.FileOutputStream;
import java.util.Locale;

/**
 * Explicit opt-in local service. Limited to one Google Play Clash of Clans
 * foreground package. No root, network, account data extraction or hidden input.
 */
public final class GameBridgeService extends AccessibilityService {
    public static final String GAME = "com.supercell.clashofclans";
    private static volatile GameBridgeService active;
    private static final String PREFS = "local_preferences";
    private static final String LOG_TAG = "REQNGO";
    private final Handler main = new Handler(Looper.getMainLooper());
    private String windowPackage = "";
    private long lastWindowTime = 0L;
    private boolean gesturePending = false;
    private boolean capturePending = false;
    private int operationGeneration = 0;
    private boolean autoRunning = false;
    private AutoRoutePolicy automatic = AutoRoutePolicy.fresh();
    private int beforeMatches = 0;
    private int afterMatches = 0;

    public static boolean connected() { return active != null; }

    private SharedPreferences prefs() {
        return getSharedPreferences(PREFS, MODE_PRIVATE);
    }

    private void report(String message) {
        prefs().edit().putString("status", message).apply();
        Log.i(LOG_TAG, message);
    }

    @Override protected void onServiceConnected() {
        active = this;
        // An unexpected process restart must not replay an unverified touch.
        String previous = prefs().getString("auto_phase", "IDLE");
        if (previous.equals("CHECKING") || previous.equals("IN_FLIGHT")
                || previous.equals("VERIFYING")) {
            prefs().edit().putString("auto_phase", "BLOCKED")
                    .putString("journal", "BLOCKED")
                    .putString("route_phase", "BLOCKED").commit();
        }
        DiagnosticStore.record(this, DiagnosticTrail.Event.SERVICE_CONNECTED);
        report("Service Android activé. Aucun geste automatique.");
    }

    @Override public void onAccessibilityEvent(AccessibilityEvent event) {
        if (event == null || event.getPackageName() == null) return;
        final int type = event.getEventType();
        if (type == AccessibilityEvent.TYPE_WINDOW_STATE_CHANGED
                || type == AccessibilityEvent.TYPE_WINDOWS_CHANGED) {
            windowPackage = event.getPackageName().toString();
            lastWindowTime = SystemClock.elapsedRealtime();
        }
    }

    @Override public void onInterrupt() {
        DiagnosticStore.record(this, DiagnosticTrail.Event.SERVICE_INTERRUPTED);
        report("Service interrompu.");
    }

    @Override public void onDestroy() {
        if (autoRunning) stopAutomaticInternal("Service Android arrêté pendant le parcours.");
        operationGeneration++;
        gesturePending = false;
        capturePending = false;
        if (active == this) active = null;
        main.removeCallbacksAndMessages(null);
        DiagnosticStore.record(this, DiagnosticTrail.Event.SERVICE_STOPPED);
        report("Service arrêté.");
        super.onDestroy();
    }

    private boolean gameForeground() {
        AccessibilityNodeInfo root = getRootInActiveWindow();
        if (root != null) {
            try {
                CharSequence pkg = root.getPackageName();
                if (pkg != null) return GAME.contentEquals(pkg);
            } finally { root.recycle(); }
        }
        // Some games expose no accessibility tree. In this case require
        // a very recent WINDOW transition explicitly identifying the game.
        return GAME.equals(windowPackage)
                && SystemClock.elapsedRealtime() - lastWindowTime < 30000;
    }

    private RequestJournal.Phase phase() {
        return new RequestJournal(prefs().getString("journal", "READY")).phase();
    }

    private boolean setPhase(RequestJournal.Phase phase) {
        return prefs().edit().putString("journal", phase.name()).commit();
    }

    private void block(String explanation) {
        DiagnosticStore.record(this, DiagnosticTrail.Event.GESTURE_BLOCKED);
        gesturePending = false;
        setPhase(RequestJournal.Phase.BLOCKED);
        report(explanation);
    }

    private static Bitmap bitmapFromResult(ScreenshotResult result) {
        Bitmap hw = Bitmap.wrapHardwareBuffer(result.getHardwareBuffer(), result.getColorSpace());
        if (hw == null) throw new IllegalStateException("Capture vide");
        try {
            Bitmap bitmap = hw.copy(Bitmap.Config.ARGB_8888, false);
            if (bitmap == null) throw new IllegalStateException("Capture illisible");
            return bitmap;
        } finally { hw.recycle(); }
    }

    private void store(Bitmap bitmap, String filename) throws Exception {
        try (FileOutputStream out = new FileOutputStream(new File(getFilesDir(), filename), false)) {
            if (!bitmap.compress(Bitmap.CompressFormat.PNG, 100, out)) {
                throw new IllegalStateException("Compression impossible");
            }
            out.getFD().sync();
        }
    }

    private FrameFingerprint.Score[] compareWithReference(Bitmap frame, double normalizedX,
                                                         double normalizedY) {
        Bitmap reference = BitmapFactory.decodeFile(
                new File(getFilesDir(), "latest_game_capture.png").getAbsolutePath());
        if (reference == null) throw new IllegalStateException("Capture de référence absente");
        try {
            int width = frame.getWidth(), height = frame.getHeight();
            if (reference.getWidth() != width || reference.getHeight() != height) {
                throw new IllegalStateException("Orientation modifiée");
            }
            int[] left = new int[width * height];
            int[] right = new int[width * height];
            reference.getPixels(left, 0, width, 0, 0, width, height);
            frame.getPixels(right, 0, width, 0, 0, width, height);
            FrameFingerprint.Score overall = FrameFingerprint.compare(left, right, width, height);
            FrameFingerprint.Score near = normalizedX > 0 && normalizedY > 0
                    ? FrameFingerprint.compareNearTap(left, right, width, height,
                            normalizedX, normalizedY) : null;
            return new FrameFingerprint.Score[]{ overall, near };
        } finally { reference.recycle(); }
    }

    /** Operator-requested capture: keep reference untouched during unresolved gestures. */
    public static boolean captureAfterReturnToGame() {
        GameBridgeService s = active;
        if (startIndex < 0 || startIndex > 1
                || s == null || Build.VERSION.SDK_INT < 30 || s.autoRunning
                || s.capturePending || s.gesturePending
                || s.phase() == RequestJournal.Phase.ARMED) return false;
        s.capturePending = true;
        int generation = ++s.operationGeneration;
        DiagnosticStore.record(s, DiagnosticTrail.Event.CAPTURE_REQUESTED);
        s.report("Capture demandée : ouverture du jeu.");
        s.main.postDelayed(() -> s.captureNow(generation, false), 2200);
        return true;
    }

    private void captureNow(int generation, boolean afterGestureOnly) {
        if (generation != operationGeneration) return;
        if (Build.VERSION.SDK_INT < 30 || !gameForeground()) {
            capturePending = false;
            DiagnosticStore.record(this, DiagnosticTrail.Event.CAPTURE_REJECTED);
            report("Capture annulée : jeu non confirmé au premier plan.");
            return;
        }
        takeScreenshot(Display.DEFAULT_DISPLAY, getMainExecutor(), new TakeScreenshotCallback() {
            @Override public void onFailure(int code) {
                if (generation != operationGeneration) return;
                capturePending = false;
                DiagnosticStore.record(GameBridgeService.this, DiagnosticTrail.Event.CAPTURE_REJECTED);
                report("Capture Android refusée (code " + code + ").");
            }

            @Override public void onSuccess(ScreenshotResult result) {
                Bitmap frame = null;
                try {
                    if (generation != operationGeneration || !gameForeground()) return;
                    frame = bitmapFromResult(result);
                    if (phase() == RequestJournal.Phase.DISPATCHED_UNVERIFIED || afterGestureOnly) {
                        store(frame, "post_action_capture.png");
                        DiagnosticStore.record(GameBridgeService.this, DiagnosticTrail.Event.CAPTURE_SAVED);
                        FrameFingerprint.Score difference = compareWithReference(frame, -1, -1)[0];
                        prefs().edit().putString("comparison", String.format(Locale.ROOT,
                                "Différence moyenne %.1f %% ; pixels modifiés %.1f %% — succès du jeu NON établi.",
                                difference.averageDifference * 100, difference.changedFraction * 100)).commit();
                        report("Capture après le geste enregistrée ; vérification humaine nécessaire.");
                    } else if (phase() != RequestJournal.Phase.ARMED) {
                        store(frame, "latest_game_capture.png");
                        DiagnosticStore.record(GameBridgeService.this, DiagnosticTrail.Event.CAPTURE_SAVED);
                        new File(getFilesDir(), "post_action_capture.png").delete();
                        prefs().edit().putLong("capture_time", System.currentTimeMillis())
                                .putInt("capture_width", frame.getWidth())
                                .putInt("capture_height", frame.getHeight())
                                .remove("comparison").commit();
                        report("Référence capturée localement (" + frame.getWidth()
                                + " × " + frame.getHeight() + ").");
                    }
                } catch (Exception e) {
                    report("Échec capture : " + e.getClass().getSimpleName());
                } finally {
                    if (generation == operationGeneration) capturePending = false;
                    if (frame != null) frame.recycle();
                    result.getHardwareBuffer().close();
                }
            }
        });
    }

    /** A tap is possible only after a new frame confirms the calibrated screen. */
    public static boolean armSingleTap(float x, float y) {
        GameBridgeService s = active;
        if (s == null || Build.VERSION.SDK_INT < 30 || s.autoRunning
                || s.capturePending || s.gesturePending
                || !Float.isFinite(x) || !Float.isFinite(y)
                || x <= 0 || x >= 1 || y <= 0 || y >= 1) return false;
        long captured = s.prefs().getLong("capture_time", 0L);
        if (captured <= 0 || System.currentTimeMillis() - captured > 120000
                || s.phase() != RequestJournal.Phase.READY
                || !s.setPhase(RequestJournal.Phase.ARMED)) return false;
        s.gesturePending = true;
        int generation = ++s.operationGeneration;
        DiagnosticStore.record(s, DiagnosticTrail.Event.GESTURE_ARMED);
        s.report("Geste armé : contrôle de l'écran avant le toucher.");
        s.main.postDelayed(() -> s.verifyBeforeTap(generation, x, y), 2200);
        return true;
    }

    private void verifyBeforeTap(int generation, float x, float y) {
        if (generation != operationGeneration || phase() != RequestJournal.Phase.ARMED) return;
        if (!gameForeground()) { block("Geste bloqué : jeu non confirmé au premier plan."); return; }
        Point display = new Point();
        if (getDisplay() == null) { block("Geste bloqué : affichage inconnu."); return; }
        getDisplay().getRealSize(display);
        if (display.x != prefs().getInt("capture_width", 0)
                || display.y != prefs().getInt("capture_height", 0)) {
            block("Geste bloqué : résolution ou orientation modifiée.");
            return;
        }
        takeScreenshot(Display.DEFAULT_DISPLAY, getMainExecutor(), new TakeScreenshotCallback() {
            @Override public void onFailure(int code) {
                if (generation == operationGeneration) block("Geste bloqué : capture de contrôle refusée.");
            }

            @Override public void onSuccess(ScreenshotResult result) {
                Bitmap frame = null;
                try {
                    if (generation != operationGeneration || phase() != RequestJournal.Phase.ARMED
                            || !gameForeground()) return;
                    frame = bitmapFromResult(result);
                    FrameFingerprint.Score[] measurements = compareWithReference(frame, x, y);
                    FrameFingerprint.Score overall = measurements[0], aroundTap = measurements[1];
                    if (!overall.similarEnoughForTap() || !aroundTap.similarEnoughForTap()) {
                        block(String.format(Locale.ROOT,
                                "Geste bloqué : écran modifié (%.1f %% global, %.1f %% près du bouton).",
                                overall.changedFraction * 100, aroundTap.changedFraction * 100));
                        return;
                    }
                    dispatchVerifiedTap(generation, x, y, display);
                } catch (Exception e) {
                    if (generation == operationGeneration) {
                        block("Geste bloqué : analyse visuelle invalide.");
                    }
                } finally {
                    if (frame != null) frame.recycle();
                    result.getHardwareBuffer().close();
                }
            }
        });
    }

    private void dispatchVerifiedTap(int generation, float x, float y, Point display) {
        if (generation != operationGeneration || phase() != RequestJournal.Phase.ARMED
                || !gameForeground()) { block("Geste annulé : contexte modifié."); return; }
        Path path = new Path();
        path.moveTo(x * display.x, y * display.y);
        GestureDescription gesture = new GestureDescription.Builder().addStroke(
                new GestureDescription.StrokeDescription(path, 0, 90)).build();
        // Persist uncertainty BEFORE dispatch: never replay on crash.
        if (!setPhase(RequestJournal.Phase.DISPATCHED_UNVERIFIED)) {
            block("Geste bloqué : journal inaccessible."); return;
        }
        if (!dispatchGesture(gesture, new GestureResultCallback() {
            @Override public void onCompleted(GestureDescription description) {
                if (generation != operationGeneration) return;
                gesturePending = false;
                DiagnosticStore.record(GameBridgeService.this, DiagnosticTrail.Event.GESTURE_SENT);
                report("Geste envoyé. Capture de résultat en préparation...");
                capturePending = true;
                main.postDelayed(() -> captureNow(generation, true), 1000);
            }
            @Override public void onCancelled(GestureDescription description) {
                if (generation != operationGeneration) return;
                gesturePending = false;
                DiagnosticStore.record(GameBridgeService.this, DiagnosticTrail.Event.GESTURE_CANCELLED);
                report("Geste annulé ; état du jeu incertain, aucune répétition.");
            }
        }, main)) {
            gesturePending = false;
            report("dispatchGesture refusé ; état conservé incertain.");
        }
    }

    /**
     * EXPERIMENTAL: executes one previously calibrated four-transition route
     * with a single explicit session-wide opt-in. Never handles login or
     * account switching. Stops on any unknown/ambiguous screen.
     */
    public static boolean startAutomaticRoute() {
        return startAutomaticRoute(0);
    }

    /** startIndex=1 is allowed only after opening the clan profile deep link. */
    public static boolean startAutomaticRoute(int startIndex) {
        GameBridgeService s = active;
        if (s == null || Build.VERSION.SDK_INT < 30 || s.autoRunning
                || s.capturePending || s.gesturePending
                || s.phase() != RequestJournal.Phase.READY
                || s.prefs().getInt("route_index", -1) != 0
                || !s.prefs().getString("route_phase", "").equals("READY")
                || !s.prefs().getString("auto_phase", "IDLE").equals("IDLE")) return false;
        try {
            int width = s.prefs().getInt("capture_width", 0);
            int height = s.prefs().getInt("capture_height", 0);
            if (width < 20 || height < 20) return false;
            for (int step = startIndex; step < GuidedRoute.SIZE; step++) {
                RouteCalibrationStore.loadForAutomaticStep(s, step, width, height);
                if (!ScreenCatalog.referenceFile(s, GuidedRoute.TO[step]).isFile())
                    return false;
            }
            s.automatic = AutoRoutePolicy.fresh();
            s.beforeMatches = 0;
            s.afterMatches = 0;
            s.automatic.startAt(startIndex, true, true);
            if (!s.prefs().edit().putInt("auto_index", s.automatic.index())
                    .putString("auto_phase", s.automatic.state().name())
                    .putInt("route_index", startIndex).commit()) return false;
            s.autoRunning = true;
            int generation = ++s.operationGeneration;
            s.report("Cycle automatique de test autorisé. Arrêt sur toute incertitude.");
            s.main.postDelayed(() -> s.autoCapture(generation, false, 0), 2200);
            return true;
        } catch (Exception failure) {
            s.report("Parcours automatique non calibré : vérifie les cinq écrans et boutons.");
            return false;
        }
    }

    public static void resetAutomaticAfterReview() {
        GameBridgeService s = active;
        if (s == null || s.autoRunning) return;
        s.automatic = AutoRoutePolicy.fresh();
        s.prefs().edit().putInt("auto_index", 0).putString("auto_phase", "IDLE").commit();
    }

    private boolean persistAutomatic() {
        return prefs().edit().putInt("auto_index", automatic.index())
                .putString("auto_phase", automatic.state().name()).commit();
    }

    private void stopAutomaticInternal(String cause) {
        autoRunning = false;
        automatic.block();
        operationGeneration++; // invalidate ALL async screenshot/gesture callbacks
        main.removeCallbacksAndMessages(null);
        gesturePending = false;
        capturePending = false;
        prefs().edit().putInt("auto_index", automatic.index())
                .putString("auto_phase", "BLOCKED")
                .putString("route_phase", "BLOCKED")
                .putString("journal", "BLOCKED").commit();
        DiagnosticStore.record(this, DiagnosticTrail.Event.ROUTE_BLOCKED);
        report(cause);
    }

    public static void stopAutomatic() {
        GameBridgeService s = active;
        if (s != null && s.autoRunning) s.stopAutomaticInternal(
                "Cycle automatique interrompu par l'opérateur.");
    }

    private void autoCapture(int generation, boolean afterGesture, int attempt) {
        if (!autoRunning || generation != operationGeneration) return;
        if (Build.VERSION.SDK_INT < 30 || !gameForeground()) {
            stopAutomaticInternal("Arrêt auto : Clash of Clans n'est pas au premier plan.");
            return;
        }
        takeScreenshot(Display.DEFAULT_DISPLAY, getMainExecutor(), new TakeScreenshotCallback() {
            @Override public void onFailure(int errorCode) {
                if (autoRunning && generation == operationGeneration)
                    stopAutomaticInternal("Arrêt auto : capture Android refusée.");
            }
            @Override public void onSuccess(ScreenshotResult result) {
                Bitmap screenshot = null;
                try {
                    if (!autoRunning || generation != operationGeneration || !gameForeground()) return;
                    screenshot = bitmapFromResult(result);
                    store(screenshot, "auto_frame.png");
                    ScreenCatalog.Result detected = ScreenCatalog.identifyAutomatic(
                            GameBridgeService.this);
                    if (afterGesture) {
                        autoVerifyAfter(generation, detected, attempt);
                    } else {
                        autoVerifyBefore(generation, screenshot, detected);
                    }
                } catch (Exception failure) {
                    if (autoRunning && generation == operationGeneration)
                        stopAutomaticInternal("Arrêt auto : image illisible ou calibration incompatible.");
                } finally {
                    if (screenshot != null) screenshot.recycle();
                    result.getHardwareBuffer().close();
                }
            }
        });
    }

    private boolean targetRegionMatches(Bitmap screenshot, String expectedState,
                                         float x, float y) {
        Bitmap reference = BitmapFactory.decodeFile(
                ScreenCatalog.referenceFile(this, expectedState).getAbsolutePath());
        if (reference == null) return false;
        try {
            int width = screenshot.getWidth(), height = screenshot.getHeight();
            if (reference.getWidth() != width || reference.getHeight() != height)
                return false;
            int[] baseline = new int[width * height];
            int[] live = new int[width * height];
            reference.getPixels(baseline, 0, width, 0, 0, width, height);
            screenshot.getPixels(live, 0, width, 0, 0, width, height);
            return FrameFingerprint.compareNearTap(baseline, live, width, height,
                    x, y).similarEnoughForTap();
        } finally { reference.recycle(); }
    }

    private void autoVerifyBefore(int generation, Bitmap screenshot,
                                  ScreenCatalog.Result detected) throws Exception {
        if (automatic.state() != AutoRoutePolicy.State.CHECKING) {
            stopAutomaticInternal("Arrêt auto : transition inattendue."); return;
        }
        int index = automatic.index();
        Point display = new Point();
        if (getDisplay() == null) {
            stopAutomaticInternal("Arrêt auto : dimensions inconnues."); return;
        }
        getDisplay().getRealSize(display);
        if (display.x != screenshot.getWidth() || display.y != screenshot.getHeight()) {
            stopAutomaticInternal("Arrêt auto : affichage ou rotation modifiée."); return;
        }
        RouteTarget target = RouteCalibrationStore.loadForAutomaticStep(
                this, index, display.x, display.y);
        boolean allowed = automatic.before().equals(detected.id)
                && targetRegionMatches(screenshot, automatic.before(), target.x, target.y)
                && gameForeground();
        if (!allowed) {
            stopAutomaticInternal("Arrêt auto : état ou bouton non reconnu (" + detected.state + ").");
            return;
        }
        // Two separate frames must agree before any automated gesture.
        if (beforeMatches == 0) {
            beforeMatches = 1;
            main.postDelayed(() -> autoCapture(generation, false, 0), 350);
            return;
        }
        beforeMatches = 0;
        afterMatches = 0;
        automatic.verifiedBefore(detected.id, true, true);
        if (!persistAutomatic()) {
            stopAutomaticInternal("Arrêt auto : journal inaccessible."); return;
        }
        Path path = new Path();
        path.moveTo(target.x * display.x, target.y * display.y);
        GestureDescription gesture = new GestureDescription.Builder().addStroke(
                new GestureDescription.StrokeDescription(path, 0, 85)).build();
        // Persist in-flight and unverified BEFORE the Android gesture.
        automatic.dispatched();
        if (!prefs().edit().putInt("auto_index", automatic.index())
                .putString("auto_phase", automatic.state().name())
                .putInt("route_index", index).putString("route_phase", "AWAITING_REVIEW")
                .putString("journal", "DISPATCHED_UNVERIFIED").commit()) {
            stopAutomaticInternal("Arrêt auto : impossible de journaliser le geste."); return;
        }
        if (!gameForeground()) {
            stopAutomaticInternal("Arrêt auto : fenêtre du jeu modifiée."); return;
        }
        boolean accepted = dispatchGesture(gesture, new GestureResultCallback() {
            @Override public void onCompleted(GestureDescription desc) {
                if (!autoRunning || generation != operationGeneration) return;
                DiagnosticStore.record(GameBridgeService.this, DiagnosticTrail.Event.GESTURE_SENT);
                main.postDelayed(() -> autoCapture(generation, true, 0), 1400);
            }
            @Override public void onCancelled(GestureDescription desc) {
                if (autoRunning && generation == operationGeneration)
                    stopAutomaticInternal("Arrêt auto : geste Android annulé, résultat inconnu.");
            }
        }, main);
        if (!accepted) stopAutomaticInternal("Arrêt auto : geste refusé, résultat inconnu.");
    }

    private void autoVerifyAfter(int generation, ScreenCatalog.Result detected, int attempt) {
        if (automatic.state() != AutoRoutePolicy.State.VERIFYING) {
            stopAutomaticInternal("Arrêt auto : résultat dans une phase inattendue."); return;
        }
        if (!automatic.after().equals(detected.id)) {
            afterMatches = 0;
            // Re-observe only: NEVER repeat a tap whose effect is uncertain.
            if (attempt < 4) {
                main.postDelayed(() -> autoCapture(generation, true, attempt + 1), 800);
            } else {
                stopAutomaticInternal("Arrêt auto : écran d'arrivée non confirmé (" + detected.state + ").");
            }
            return;
        }
        if (afterMatches == 0) {
            if (attempt >= 4) {
                stopAutomaticInternal("Arrêt auto : état d'arrivée non stabilisé."); return;
            }
            afterMatches = 1;
            main.postDelayed(() -> autoCapture(generation, true, attempt + 1), 350);
            return;
        }
        afterMatches = 0;
        Bitmap evidence = BitmapFactory.decodeFile(
                new File(getFilesDir(), "auto_frame.png").getAbsolutePath());
        if (evidence == null) {
            stopAutomaticInternal("Arrêt auto : capture de preuve indisponible."); return;
        }
        try {
            store(evidence, "post_action_capture.png");
        } catch (Exception failure) {
            stopAutomaticInternal("Arrêt auto : impossible de conserver la preuve."); return;
        } finally {
            evidence.recycle();
        }
        automatic.verifiedAfter(detected.id);
        boolean finished = automatic.state() == AutoRoutePolicy.State.COMPLETE_VISUAL;
        if (!prefs().edit().putInt("auto_index", automatic.index())
                .putString("auto_phase", automatic.state().name())
                .putInt("route_index", automatic.index())
                .putString("route_phase", finished ? "COMPLETE" : "READY")
                .putString("journal", finished ? "CONFIRMED" : "READY").commit()) {
            stopAutomaticInternal("Arrêt auto : résultat non enregistré."); return;
        }
        DiagnosticStore.record(this, DiagnosticTrail.Event.ROUTE_STEP_APPROVED);
        if (finished) {
            autoRunning = false;
            report("Cycle terminé VISUELLEMENT : demande serveur NON vérifiée.");
        } else {
            main.postDelayed(() -> autoCapture(generation, false, 0), 800);
        }
    }

    /** Invalidates late screenshot callbacks as well as delayed gestures. */
    public static void cancelPending() {
        GameBridgeService s = active;
        if (s != null) {
            s.operationGeneration++;
            s.main.removeCallbacksAndMessages(null);
            s.gesturePending = false;
            s.capturePending = false;
            if (s.autoRunning) s.stopAutomaticInternal("Arrêt explicite du cycle automatique.");
            s.report("Opérations en attente annulées.");
        }
    }
}
