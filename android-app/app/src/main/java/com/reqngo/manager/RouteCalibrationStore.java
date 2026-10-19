package com.reqngo.manager;

import android.content.Context;
import android.content.SharedPreferences;

import java.io.File;
import java.io.FileInputStream;
import java.security.MessageDigest;
import java.util.Locale;

/** Private, read-only screen authentication for operator-approved touch presets. */
public final class RouteCalibrationStore {
    private RouteCalibrationStore() { }

    private static SharedPreferences prefs(Context context) {
        return context.getSharedPreferences("local_preferences", Context.MODE_PRIVATE);
    }

    private static String key(int step) {
        if (step < 0 || step >= GuidedRoute.SIZE)
            throw new IllegalArgumentException("Étape de parcours inconnue");
        return "route_target_" + step;
    }

    private static String sha256(File file) throws Exception {
        if (!file.isFile()) throw new IllegalStateException("Référence d'écran absente");
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        try (FileInputStream input = new FileInputStream(file)) {
            byte[] bytes = new byte[8192];
            int count;
            while ((count = input.read(bytes)) != -1) {
                if (count > 0) digest.update(bytes, 0, count);
            }
        }
        StringBuilder hex = new StringBuilder(64);
        for (byte value : digest.digest()) hex.append(String.format(Locale.ROOT, "%02x", value & 255));
        return hex.toString();
    }

    private static void ensureFreshCapture(Context context) {
        long time = prefs(context).getLong("capture_time", 0L);
        long now = System.currentTimeMillis();
        if (time <= 0 || time > now || now - time > 120000L)
            throw new IllegalStateException("Capture récente requise pour cette étape");
    }

    public static RouteTarget store(Context context, GuidedRoute route,
                                    float x, float y, long selectionTime) throws Exception {
        if (route.phase() != GuidedRoute.Phase.READY)
            throw new IllegalStateException("Étape non prête");
        ensureFreshCapture(context);
        SharedPreferences preferences = prefs(context);
        if (selectionTime <= 0 || selectionTime != preferences.getLong("capture_time", -1L))
            throw new IllegalStateException("Sélectionne un point sur la capture actuelle");
        if (!route.expectedBefore().equals(ScreenCatalog.identify(context).id))
            throw new IllegalStateException("Écran de départ non reconnu");
        String hash = sha256(ScreenCatalog.referenceFile(context, route.expectedBefore()));
        RouteTarget target = new RouteTarget(route.index(),
                preferences.getInt("capture_width", 0),
                preferences.getInt("capture_height", 0), x, y, hash);
        if (!preferences.edit().putString(key(route.index()), target.encode()).commit())
            throw new IllegalStateException("Enregistrement impossible");
        return target;
    }

    public static RouteTarget loadForCurrentScreen(Context context, GuidedRoute route) throws Exception {
        if (route.phase() != GuidedRoute.Phase.READY)
            throw new IllegalStateException("Étape non prête");
        ensureFreshCapture(context);
        SharedPreferences preferences = prefs(context);
        RouteTarget target = RouteTarget.decode(preferences.getString(key(route.index()), null));
        String digest = sha256(ScreenCatalog.referenceFile(context, route.expectedBefore()));
        if (!target.validFor(route.index(),
                preferences.getInt("capture_width", 0),
                preferences.getInt("capture_height", 0), digest)) {
            throw new IllegalStateException("Référence ou résolution modifiée : recalibre le bouton");
        }
        if (!route.expectedBefore().equals(ScreenCatalog.identify(context).id))
            throw new IllegalStateException("Écran différent de l'étape attendue");
        return target;
    }

    /**
     * Read a previously operator-calibrated point for an automatic test.
     * Full-screen recognition and current-frame button checks happen in the
     * Android service immediately before every dispatched gesture.
     */
    public static RouteTarget loadForAutomaticStep(Context context, int step,
                                                    int width, int height) throws Exception {
        if (step < 0 || step >= GuidedRoute.SIZE)
            throw new IllegalArgumentException("Étape inconnue");
        SharedPreferences p = prefs(context);
        RouteTarget target = RouteTarget.decode(p.getString(key(step), null));
        String referenceHash = sha256(ScreenCatalog.referenceFile(context, GuidedRoute.FROM[step]));
        if (!target.validFor(step, width, height, referenceHash))
            throw new IllegalStateException("Écran ou référence du bouton modifié");
        return target;
    }

    public static boolean stored(Context context, int step) {
        return prefs(context).contains(key(step));
    }
}
