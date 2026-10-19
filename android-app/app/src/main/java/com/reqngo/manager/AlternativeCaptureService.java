package com.reqngo.manager;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.Service;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.ServiceInfo;
import android.graphics.Bitmap;
import android.graphics.PixelFormat;
import android.hardware.display.DisplayManager;
import android.hardware.display.VirtualDisplay;
import android.media.Image;
import android.media.ImageReader;
import android.media.projection.MediaProjection;
import android.media.projection.MediaProjectionManager;
import android.os.Build;
import android.os.Handler;
import android.os.HandlerThread;
import android.os.IBinder;
import android.os.SystemClock;
import android.util.DisplayMetrics;
import android.view.WindowManager;

import java.io.File;
import java.io.FileOutputStream;
import java.nio.ByteBuffer;

/**
 * One-shot opt-in alternative to accessibility screenshots using Android's
 * system-authorized MediaProjection API. No indefinite recording, audio,
 * network transmission or automatic user-consent bypass.
 *
 * The system shows its screen-sharing prompt EVERY time. Captures are local.
 */
public final class AlternativeCaptureService extends Service {
    public static final String ACTION_CAPTURE = "com.reqngo.manager.CAPTURE_ONCE";
    public static final String EXTRA_RESULT_CODE = "projection_result_code";
    public static final String EXTRA_RESULT_DATA = "projection_result_data";
    private static final String CHANNEL = "reqngo_one_shot_screen_capture";
    private static final int NOTIFICATION_ID = 1142;
    private static final long TIMEOUT_MS = 13000;
    private static final long WAIT_FOR_GAME_MS = 3500;

    private HandlerThread worker;
    private Handler handler;
    private MediaProjection projection;
    private VirtualDisplay virtualDisplay;
    private ImageReader reader;
    private long started;
    private boolean finished;
    private long lastFrameAttempt;

    @Override public IBinder onBind(Intent intent) { return null; }

    private SharedPreferences preferences() {
        return getSharedPreferences("local_preferences", MODE_PRIVATE);
    }

    private void status(String value) {
        preferences().edit().putString("status", value).commit();
    }

    private void finish(String message) {
        if (finished) return;
        finished = true;
        if (message != null) status(message);
        stopSelf();
    }

    @Override public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent == null || !ACTION_CAPTURE.equals(intent.getAction()) || worker != null) {
            stopSelf(startId);
            return START_NOT_STICKY;
        }
        try {
            NotificationManager notifications =
                    (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
            notifications.createNotificationChannel(new NotificationChannel(
                    CHANNEL, "Capture temporaire du jeu", NotificationManager.IMPORTANCE_LOW));
            Notification notification = new Notification.Builder(this, CHANNEL)
                    .setSmallIcon(android.R.drawable.ic_menu_camera)
                    .setContentTitle("REQ N GO — capture autorisée")
                    .setContentText("Capture ponctuelle du jeu en cours")
                    .setOngoing(true)
                    .build();
            if (Build.VERSION.SDK_INT >= 29) {
                startForeground(NOTIFICATION_ID, notification,
                        ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PROJECTION);
            } else {
                startForeground(NOTIFICATION_ID, notification);
            }

            final int resultCode = intent.getIntExtra(EXTRA_RESULT_CODE, 0);
            final Intent token = intent.getParcelableExtra(EXTRA_RESULT_DATA);
            if (resultCode != android.app.Activity.RESULT_OK || token == null) {
                finish("Capture alternative : consentement Android absent.");
                return START_NOT_STICKY;
            }

            worker = new HandlerThread("reqngo-projection-once");
            worker.start();
            handler = new Handler(worker.getLooper());
            started = SystemClock.elapsedRealtime();

            MediaProjectionManager manager = (MediaProjectionManager)
                    getSystemService(MEDIA_PROJECTION_SERVICE);
            projection = manager.getMediaProjection(resultCode, token);
            if (projection == null) {
                finish("Capture alternative : MediaProjection indisponible.");
                return START_NOT_STICKY;
            }
            projection.registerCallback(new MediaProjection.Callback() {
                @Override public void onStop() {
                    if (!finished) finish("Capture alternative interrompue par Android.");
                }
            }, handler);

            DisplayMetrics metrics = new DisplayMetrics();
            WindowManager windows = (WindowManager) getSystemService(WINDOW_SERVICE);
            windows.getDefaultDisplay().getRealMetrics(metrics);
            final int width = metrics.widthPixels;
            final int height = metrics.heightPixels;
            if (width < 40 || height < 40) {
                finish("Capture alternative : résolution de MuMu invalide.");
                return START_NOT_STICKY;
            }
            reader = ImageReader.newInstance(width, height, PixelFormat.RGBA_8888, 3);
            reader.setOnImageAvailableListener(this::onFrame, handler);
            virtualDisplay = projection.createVirtualDisplay(
                    "REQNGO One Shot", width, height, metrics.densityDpi,
                    DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR,
                    reader.getSurface(), null, handler);
            handler.postDelayed(() -> finish("Capture alternative sans réponse d'Android après 13 secondes."),
                    TIMEOUT_MS);
            status("Capture alternative autorisée. Retour au jeu, analyse dans quelques secondes.");
        } catch (Exception e) {
            finish("Capture alternative indisponible : " + e.getClass().getSimpleName());
        }
        return START_NOT_STICKY;
    }

    private void onFrame(ImageReader input) {
        if (finished) return;
        Image image = null;
        Bitmap raw = null;
        Bitmap cropped = null;
        try {
            image = input.acquireLatestImage();
            if (image == null) return;
            long elapsed = SystemClock.elapsedRealtime() - started;
            if (elapsed < WAIT_FOR_GAME_MS) return;
            if (elapsed - lastFrameAttempt < 450) return;
            lastFrameAttempt = elapsed;
            if (!GameBridgeService.gameVisibleForCapture()) {
                if (elapsed >= 9700)
                    finish("Capture alternative annulée : Clash of Clans n'est pas confirmé "
                            + "au premier plan. Vérifie l'accessibilité et relance le jeu.");
                return;
            }

            Image.Plane plane = image.getPlanes()[0];
            ByteBuffer bytes = plane.getBuffer();
            int pixelStride = plane.getPixelStride();
            int rowStride = plane.getRowStride();
            if (pixelStride != 4 || rowStride < image.getWidth() * 4) {
                throw new IllegalStateException("Format image non pris en charge");
            }
            int expandedWidth = rowStride / pixelStride;
            raw = Bitmap.createBitmap(expandedWidth, image.getHeight(), Bitmap.Config.ARGB_8888);
            raw.copyPixelsFromBuffer(bytes);
            cropped = Bitmap.createBitmap(raw, 0, 0, image.getWidth(), image.getHeight());

            if (CaptureQuality.nearlyBlack(BitmapSamples.extract(cropped))) {
                if (elapsed >= 9700)
                    finish("Capture alternative également noire sur MuMu. "
                            + "Vérifie le moteur graphique et la compatibilité de l'émulateur.");
                return;
            }
            File output = new File(getFilesDir(), "latest_game_capture.png");
            File temporary = new File(getFilesDir(), "latest_projection_capture.pending");
            try (FileOutputStream stream = new FileOutputStream(temporary, false)) {
                if (!cropped.compress(Bitmap.CompressFormat.PNG, 100, stream))
                    throw new IllegalStateException("Impossible d'encoder l'image");
                stream.getFD().sync();
            }
            // Replace the reference only after a complete, nonblack screenshot.
            if (!temporary.renameTo(output))
                throw new IllegalStateException("Impossible de conserver la capture");
            preferences().edit()
                    .putLong("capture_time", System.currentTimeMillis())
                    .putInt("capture_width", cropped.getWidth())
                    .putInt("capture_height", cropped.getHeight())
                    .remove("comparison").commit();
            new File(getFilesDir(), "post_action_capture.png").delete();
            finish("Capture alternative réussie (" + cropped.getWidth() + " × "
                    + cropped.getHeight() + "). Vérifie que l'image représente le village.");
        } catch (Exception e) {
            finish("Échec MediaProjection : " + e.getClass().getSimpleName());
        } finally {
            if (cropped != null) cropped.recycle();
            if (raw != null) raw.recycle();
            if (image != null) image.close();
        }
    }

    @Override public void onDestroy() {
        finished = true;
        if (reader != null) {
            reader.setOnImageAvailableListener(null, null);
            reader.close();
            reader = null;
        }
        if (virtualDisplay != null) {
            virtualDisplay.release();
            virtualDisplay = null;
        }
        if (projection != null) {
            projection.stop();
            projection = null;
        }
        if (worker != null) {
            worker.quitSafely();
            worker = null;
        }
        stopForeground(STOP_FOREGROUND_REMOVE);
        super.onDestroy();
    }
}
