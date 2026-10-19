package com.reqngo.manager;

import android.content.Context;
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
        "village", "clan_profile", "clan_chat", "request_dialog", "request_posted"
    };
    public static final String[] LABELS = {
        "Village", "Fiche du clan", "Chat du clan",
        "Fenêtre de demande", "Demande visible dans le chat"
    };

    private ScreenCatalog() { }

    public static final class Result {
        public final String state;
        public final double difference;
        public final double runnerUpMargin;
        public final int enrolled;

        Result(String state, double difference, double margin, int enrolled) {
            this.state = state;
            this.difference = difference;
            this.runnerUpMargin = margin;
            this.enrolled = enrolled;
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
    }

    /**
     * A low score describes only image similarity. Two matches within 0.8%
     * average RGB difference are ambiguous and are reported as unknown.
     */
    public static Result identify(Context context) {
        File source = new File(context.getFilesDir(), "latest_game_capture.png");
        Bitmap current = BitmapFactory.decodeFile(source.getAbsolutePath());
        if (current == null) throw new IllegalStateException("Capture absente ou illisible");
        try {
            final int width = current.getWidth(), height = current.getHeight();
            if (width < 20 || height < 20) throw new IllegalStateException("Capture invalide");
            int[] currentPixels = new int[width * height];
            current.getPixels(currentPixels, 0, width, 0, 0, width, height);
            double best = Double.POSITIVE_INFINITY, next = Double.POSITIVE_INFINITY;
            String bestName = "inconnu";
            boolean bestValid = false;
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
                    FrameFingerprint.Score score = FrameFingerprint.compare(
                            referencePixels, currentPixels, width, height);
                    if (score.averageDifference < best) {
                        next = best;
                        best = score.averageDifference;
                        bestName = LABELS[i];
                        bestValid = score.similarEnoughForTap();
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
            }
            return new Result(bestName, best, margin, enrolled);
        } finally {
            current.recycle();
        }
    }
}
