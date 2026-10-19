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
        report("Service interrompu.");
    }

    @Override public void onDestroy() {
        operationGeneration++;
        gesturePending = false;
        capturePending = false;
        if (active == this) active = null;
        main.removeCallbacksAndMessages(null);
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
                && SystemClock.elapsedRealtime() - lastWindowTime < 10000;
    }

    private RequestJournal.Phase phase() {
        return new RequestJournal(prefs().getString("journal", "READY")).phase();
    }

    private boolean setPhase(RequestJournal.Phase phase) {
        return prefs().edit().putString("journal", phase.name()).commit();
    }

    private void block(String explanation) {
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

    private FrameFingerprint.Score compareWithReference(Bitmap frame) {
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
            return FrameFingerprint.compare(left, right, width, height);
        } finally { reference.recycle(); }
    }

    /** Operator-requested capture: keep reference untouched during unresolved gestures. */
    public static boolean captureAfterReturnToGame() {
        GameBridgeService s = active;
        if (s == null || Build.VERSION.SDK_INT < 30 || s.capturePending || s.gesturePending
                || s.phase() == RequestJournal.Phase.ARMED) return false;
        s.capturePending = true;
        int generation = ++s.operationGeneration;
        s.report("Capture demandée : ouverture du jeu.");
        s.main.postDelayed(() -> s.captureNow(generation), 2200);
        return true;
    }

    private void captureNow(int generation) {
        if (generation != operationGeneration) return;
        if (Build.VERSION.SDK_INT < 30 || !gameForeground()) {
            capturePending = false;
            report("Capture annulée : jeu non confirmé au premier plan.");
            return;
        }
        takeScreenshot(Display.DEFAULT_DISPLAY, getMainExecutor(), new TakeScreenshotCallback() {
            @Override public void onFailure(int code) {
                if (generation != operationGeneration) return;
                capturePending = false;
                report("Capture Android refusée (code " + code + ").");
            }

            @Override public void onSuccess(ScreenshotResult result) {
                Bitmap frame = null;
                try {
                    if (generation != operationGeneration || !gameForeground()) return;
                    frame = bitmapFromResult(result);
                    if (phase() == RequestJournal.Phase.DISPATCHED_UNVERIFIED) {
                        store(frame, "post_action_capture.png");
                        FrameFingerprint.Score difference = compareWithReference(frame);
                        prefs().edit().putString("comparison", String.format(Locale.ROOT,
                                "Différence moyenne %.1f %% ; pixels modifiés %.1f %% — succès du jeu NON établi.",
                                difference.averageDifference * 100, difference.changedFraction * 100)).commit();
                        report("Capture après le geste enregistrée ; vérification humaine nécessaire.");
                    } else if (phase() != RequestJournal.Phase.ARMED) {
                        store(frame, "latest_game_capture.png");
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
                    capturePending = false;
                    if (frame != null) frame.recycle();
                    result.getHardwareBuffer().close();
                }
            }
        });
    }

    /** A tap is possible only after a new frame confirms the calibrated screen. */
    public static boolean armSingleTap(float x, float y) {
        GameBridgeService s = active;
        if (s == null || Build.VERSION.SDK_INT < 30 || s.capturePending || s.gesturePending
                || !Float.isFinite(x) || !Float.isFinite(y)
                || x <= 0 || x >= 1 || y <= 0 || y >= 1) return false;
        long captured = s.prefs().getLong("capture_time", 0L);
        if (captured <= 0 || System.currentTimeMillis() - captured > 120000
                || s.phase() != RequestJournal.Phase.READY
                || !s.setPhase(RequestJournal.Phase.ARMED)) return false;
        s.gesturePending = true;
        int generation = ++s.operationGeneration;
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
                    FrameFingerprint.Score comparison = compareWithReference(frame);
                    if (!comparison.similarEnoughForTap()) {
                        block(String.format(Locale.ROOT,
                                "Geste bloqué : écran changé (%.1f %% de pixels différents).",
                                comparison.changedFraction * 100));
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
                report("Geste envoyé. Capture de résultat en préparation...");
                capturePending = true;
                main.postDelayed(() -> captureNow(generation), 1000);
            }
            @Override public void onCancelled(GestureDescription description) {
                if (generation != operationGeneration) return;
                gesturePending = false;
                report("Geste annulé ; état du jeu incertain, aucune répétition.");
            }
        }, main)) {
            gesturePending = false;
            report("dispatchGesture refusé ; état conservé incertain.");
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
            s.report("Opérations en attente annulées.");
        }
    }
}
