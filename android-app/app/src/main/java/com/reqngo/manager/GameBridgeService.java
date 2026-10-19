package com.reqngo.manager;

import android.accessibilityservice.AccessibilityService;
import android.accessibilityservice.GestureDescription;
import android.graphics.Bitmap;
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

    public static boolean captureAfterReturnToGame() {
        GameBridgeService s = active;
        if (s == null || Build.VERSION.SDK_INT < 30 || s.capturePending) return false;
        s.capturePending = true;
        s.report("Capture autorisée : ouverture du jeu.");
        s.main.postDelayed(() -> s.captureNow(), 2200);
        return true;
    }

    private void captureNow() {
        if (Build.VERSION.SDK_INT < 30 || !gameForeground()) {
            capturePending = false;
            report("Capture annulée : jeu non confirmé au premier plan.");
            return;
        }
        takeScreenshot(Display.DEFAULT_DISPLAY, getMainExecutor(), new TakeScreenshotCallback() {
            @Override public void onFailure(int code) {
                capturePending = false;
                report("Capture Android refusée (code " + code + ").");
            }

            @Override public void onSuccess(ScreenshotResult result) {
                capturePending = false;
                if (!gameForeground()) {
                    result.getHardwareBuffer().close();
                    report("Capture rejetée : fenêtre changée.");
                    return;
                }
                Bitmap hardware = null;
                Bitmap copy = null;
                try {
                    hardware = Bitmap.wrapHardwareBuffer(result.getHardwareBuffer(), result.getColorSpace());
                    if (hardware == null) throw new IllegalStateException("Bitmap absent");
                    copy = hardware.copy(Bitmap.Config.ARGB_8888, false);
                    if (copy == null) throw new IllegalStateException("Bitmap illisible");
                    File dest = new File(getFilesDir(), "latest_game_capture.png");
                    try (FileOutputStream out = new FileOutputStream(dest, false)) {
                        if (!copy.compress(Bitmap.CompressFormat.PNG, 100, out)) {
                            throw new IllegalStateException("Compression impossible");
                        }
                        out.getFD().sync();
                    }
                    prefs().edit().putLong("capture_time", System.currentTimeMillis())
                            .putInt("capture_width", copy.getWidth())
                            .putInt("capture_height", copy.getHeight()).commit();
                    report("Capture du jeu enregistrée localement (" + copy.getWidth()
                            + " × " + copy.getHeight() + ").");
                } catch (Exception e) {
                    report("Échec de capture : " + e.getClass().getSimpleName());
                } finally {
                    result.getHardwareBuffer().close();
                    if (copy != null) copy.recycle();
                }
            }
        });
    }

    public static boolean armSingleTap(float x, float y) {
        GameBridgeService s = active;
        if (s == null || s.gesturePending || !Float.isFinite(x) || !Float.isFinite(y)
                || x <= 0 || x >= 1 || y <= 0 || y >= 1) return false;
        SharedPreferences prefs = s.prefs();
        long captured = prefs.getLong("capture_time", 0);
        if (captured == 0 || System.currentTimeMillis() - captured > 120000) return false;
        if (!new RequestJournal(prefs.getString("journal", "READY"))
                .phase().equals(RequestJournal.Phase.READY)) return false;
        RequestJournal journal = RequestJournal.fresh();
        journal.arm();
        if (!prefs.edit().putString("journal", journal.phase().name()).commit()) return false;
        s.gesturePending = true;
        s.report("Un geste armé : ouverture du jeu, exécution dans 2,2 s.");
        s.main.postDelayed(() -> s.dispatchOnce(x, y), 2200);
        return true;
    }

    private void dispatchOnce(float x, float y) {
        gesturePending = false;
        RequestJournal journal = new RequestJournal(prefs().getString("journal", "BLOCKED"));
        if (journal.phase() != RequestJournal.Phase.ARMED || !gameForeground()) {
            journal.block();
            prefs().edit().putString("journal", journal.phase().name()).commit();
            report("Geste annulé : jeu non confirmé au premier plan.");
            return;
        }
        Point display = new Point();
        getDisplay().getRealSize(display);
        if (display.x != prefs().getInt("capture_width", 0)
                || display.y != prefs().getInt("capture_height", 0)) {
            journal.block();
            prefs().edit().putString("journal", journal.phase().name()).commit();
            report("Geste bloqué : résolution différente de la capture.");
            return;
        }
        Path path = new Path();
        path.moveTo(x * display.x, y * display.y);
        GestureDescription.StrokeDescription stroke =
                new GestureDescription.StrokeDescription(path, 0, 90);
        GestureDescription gesture = new GestureDescription.Builder().addStroke(stroke).build();
        // Persist the uncertain state BEFORE dispatch, to avoid replay after crash.
        journal.dispatched();
        if (!prefs().edit().putString("journal", journal.phase().name()).commit()) {
            report("Journal inaccessible : aucun geste.");
            return;
        }
        if (!dispatchGesture(gesture, new GestureResultCallback() {
            @Override public void onCompleted(GestureDescription description) {
                report("Geste Android exécuté — résultat dans le jeu NON VÉRIFIÉ.");
            }
            @Override public void onCancelled(GestureDescription description) {
                report("Geste Android annulé — état du jeu inconnu.");
            }
        }, main)) {
            report("dispatchGesture refusé — état conservé incertain.");
        }
    }

    public static void cancelPending() {
        GameBridgeService s = active;
        if (s != null) {
            s.main.removeCallbacksAndMessages(null);
            s.gesturePending = false;
            s.capturePending = false;
        }
    }
}
