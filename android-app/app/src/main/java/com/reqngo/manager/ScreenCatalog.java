package com.reqngo.manager;

import android.content.Context;
import android.content.SharedPreferences;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.util.Arrays;

/**
 * Private on-device reference screenshots. This catalog provides only a
 * diagnostic classification, NEVER an authorization for an automated gesture
 * or proof of a successful request.
 */
public final class ScreenCatalog {
    public static final String[] IDS = {
        "village", "clan_profile", "clan_chat", "request_dialog", "request_posted",
        "clan_closed", "clan_invitation", "join_pending",
        "request_cooldown", "castle_full", "connection_error"
    };
    public static final String[] LABELS = {
        "Village", "Fiche du clan", "Chat du clan",
        "Fenêtre de demande", "Demande visible dans le chat",
        "Clan fermé", "Invitation / autorisation requise", "Adhésion en attente",
        "Délai de demande actif", "Château plein", "Erreur de connexion"
    };

    private ScreenCatalog() { }

    private static SharedPreferences prefs(Context context) {
        return context.getSharedPreferences("local_preferences", Context.MODE_PRIVATE);
    }

    public static final class Result {
        public final String state;
        public final String id;
        public final double difference;
        public final double runnerUpMargin;
        public final int enrolled;
        public final String method;

        Result(String state, String id, double difference, double margin,
               int enrolled, String method) {
            this.state = state;
            this.id = id;
            this.difference = difference;
            this.runnerUpMargin = margin;
            this.enrolled = enrolled;
            this.method = method;
        }
    }

    public static File referenceFile(Context context, String id) {
        if (!Arrays.asList(IDS).contains(id)) {
            throw new IllegalArgumentException("État non autorisé");
        }
        return new File(context.getFilesDir(), "reference_" + id + ".png");
    }

    /** Called ONLY after the operator has inspected a fresh game screenshot. */
    public static void save(Context context, String id) throws Exception {
        File source = new File(context.getFilesDir(), "latest_game_capture.png");
        if (!source.isFile()) throw new IllegalStateException("Aucune capture du jeu disponible");
        File destination = referenceFile(context, id);
        File pending = new File(context.getFilesDir(), "reference_" + id + ".pending");
        byte[] buffer = new byte[8192];
        try (FileInputStream in = new FileInputStream(source);
             FileOutputStream out = new FileOutputStream(pending, false)) {
            int len;
            while ((len = in.read(buffer)) >= 0) {
                if (len > 0) out.write(buffer, 0, len);
            }
            out.getFD().sync();
        } catch (Exception e) {
            pending.delete();
            throw e;
        }
        if (!pending.renameTo(destination)) {
            pending.delete();
            throw new IllegalStateException("Impossible de conserver cet état");
        }
        // Reference replacement changes the meaning of its ROI.
        prefs(context).edit().remove("roi_" + id + "_x")
                .remove("roi_" + id + "_y").commit();
    }

    /**
     * Anchor on a visually stable area selected by the operator; used only
     * for classification, never to authorize or target a touch directly.
     */
    public static void setAnchor(Context context, String id, float x, float y) throws Exception {
        File reference = referenceFile(context, id);
        if (!reference.isFile() || !Float.isFinite(x) || !Float.isFinite(y)
                || x <= .10f || x >= .90f || y <= .10f || y >= .90f) {
            throw new IllegalArgumentException("Zone ou référence invalide (éviter les bords)");
        }
        // Confirm that the selected point was taken on this exact reference.
        Bitmap saved = BitmapFactory.decodeFile(reference.getAbsolutePath());
        Bitmap current = BitmapFactory.decodeFile(
                new File(context.getFilesDir(), "latest_game_capture.png").getAbsolutePath());
        if (saved == null || current == null)
            throw new IllegalStateException("Capture de référence non disponible");
        try {
            int w = saved.getWidth(), h = saved.getHeight();
            if (w != current.getWidth() || h != current.getHeight() || w < 20 || h < 20)
                throw new IllegalStateException("Dimensions incompatibles");
            int[] a = new int[w * h], b = new int[w * h];
            saved.getPixels(a, 0, w, 0, 0, w, h);
            current.getPixels(b, 0, w, 0, 0, w, h);
            if (!FrameFingerprint.compare(a, b, w, h).similarEnoughForTap())
                throw new IllegalStateException("Recapture l'écran mémorisé avant de choisir sa zone");
        } finally {
            saved.recycle();
            current.recycle();
        }
        if (!prefs(context).edit().putFloat("roi_" + id + "_x", x)
                .putFloat("roi_" + id + "_y", y).commit()) {
            throw new IllegalStateException("Enregistrement de zone refusé");
        }
    }

    /**
     * A low score describes only image similarity. Two matches within 0.8%
     * average RGB difference are ambiguous and are reported as unknown.
     */
    public static Result identify(Context context) {
        return identifyFile(context, "latest_game_capture.png");
    }

    public static Result identifyPostAction(Context context) {
        return identifyFile(context, "post_action_capture.png");
    }

    /** Dedicated transient image; never overrides operator reference screenshots. */
    public static Result identifyAutomatic(Context context) {
        return identifyFile(context, "auto_frame.png");
    }

    private static Result identifyFile(Context context, String filename) {
        File source = new File(context.getFilesDir(), filename);
        Bitmap current = BitmapFactory.decodeFile(source.getAbsolutePath());
        if (current == null) throw new IllegalStateException("Capture absente ou illisible");
        try {
            final int width = current.getWidth(), height = current.getHeight();
            if (width < 20 || height < 20) throw new IllegalStateException("Capture invalide");
            int[] currentPixels = new int[width * height];
            current.getPixels(currentPixels, 0, width, 0, 0, width, height);
            double best = Double.POSITIVE_INFINITY, next = Double.POSITIVE_INFINITY;
            String bestName = "inconnu";
            String bestId = "unknown";
            boolean bestValid = false;
            String bestMethod = "globale";
            int enrolled = 0;
            for (int i = 0; i < IDS.length; i++) {
                File reference = referenceFile(context, IDS[i]);
                if (!reference.isFile()) continue;
                enrolled++;
                Bitmap stored = BitmapFactory.decodeFile(reference.getAbsolutePath());
                if (stored == null) continue;
                try {
                    if (stored.getWidth() != width || stored.getHeight() != height) continue;
                    int[] referencePixels = new int[width * height];
                    stored.getPixels(referencePixels, 0, width, 0, 0, width, height);
                    // Only after explicit ROI calibration. The fallback remains
                    // full-frame comparison for uncalibrated screen types.
                    SharedPreferences p = prefs(context);
                    boolean roi = p.contains("roi_" + IDS[i] + "_x")
                            && p.contains("roi_" + IDS[i] + "_y");
                    FrameFingerprint.Score score = roi
                            ? FrameFingerprint.compareNearTap(
                                referencePixels, currentPixels, width, height,
                                p.getFloat("roi_" + IDS[i] + "_x", -1f),
                                p.getFloat("roi_" + IDS[i] + "_y", -1f))
                            : FrameFingerprint.compare(
                                referencePixels, currentPixels, width, height);
                    if (score.averageDifference < best) {
                        next = best;
                        best = score.averageDifference;
                        bestName = LABELS[i];
                        bestId = IDS[i];
                        bestValid = score.similarEnoughForTap();
                        bestMethod = roi ? "zone stable" : "écran entier";
                    } else if (score.averageDifference < next) {
                        next = score.averageDifference;
                    }
                } finally {
                    stored.recycle();
                }
            }
            double margin = next - best;
            if (!bestValid || (!Double.isInfinite(margin) && margin < .008)) {
                bestName = "inconnu / ambigu";
                bestId = "unknown";
            }
            return new Result(bestName, bestId, best, margin, enrolled, bestMethod);
        } finally {
            current.recycle();
        }
    }
}
