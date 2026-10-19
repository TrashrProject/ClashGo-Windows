package com.reqngo.manager;

/**
 * Small deterministic visual preflight. This is NOT OCR and cannot verify a
 * troop request. It only rejects taps if the current frame differs strongly
 * from the operator-selected reference frame.
 *
 * Independent of Android APIs, allowing JVM regression tests.
 */
public final class FrameFingerprint {
    private FrameFingerprint() { }

    public static final class Score {
        public final double averageDifference;
        public final double changedFraction;
        public final int samples;

        Score(double averageDifference, double changedFraction, int samples) {
            this.averageDifference = averageDifference;
            this.changedFraction = changedFraction;
            this.samples = samples;
        }

        /** Conservative safety guard; tolerates only small animations. */
        public boolean similarEnoughForTap() {
            return samples >= 400 && averageDifference <= .065 && changedFraction <= .20;
        }
    }

    public static Score compare(int[] before, int[] after, int width, int height) {
        return compareRect(before, after, width, height, 0, 0, width, height);
    }

    /** Compare an area around the intended touch. A small popup near the
     *  button must block a tap even if the remainder of the screen is stable. */
    public static Score compareNearTap(int[] before, int[] after, int width, int height,
                                       double normalizedX, double normalizedY) {
        if (!Double.isFinite(normalizedX) || !Double.isFinite(normalizedY)
                || normalizedX <= 0 || normalizedX >= 1
                || normalizedY <= 0 || normalizedY >= 1) {
            throw new IllegalArgumentException("Coordonnées incorrectes");
        }
        int x = (int) (normalizedX * width);
        int y = (int) (normalizedY * height);
        int rx = Math.max(12, width / 10);
        int ry = Math.max(12, height / 10);
        return compareRect(before, after, width, height,
                Math.max(0, x - rx), Math.max(0, y - ry),
                Math.min(width, x + rx), Math.min(height, y + ry));
    }

    private static Score compareRect(int[] before, int[] after, int width, int height,
                                     int left, int top, int right, int bottom) {
        if (before == null || after == null || width < 20 || height < 20
                || (long) width * height > Integer.MAX_VALUE
                || before.length != width * height || after.length != width * height) {
            throw new IllegalArgumentException("Dimensions ou pixels incorrects");
        }
        long errors = 0;
        int changed = 0;
        int n = 0;
        // Grid stratification, stable across devices and camera resolutions.
        final int rows = 48;
        final int cols = 48;
        for (int y = 0; y < rows; y++) {
            int py = Math.min(height - 1, top + (int) ((y + .5) * (bottom - top) / rows));
            for (int x = 0; x < cols; x++) {
                int px = Math.min(width - 1, left + (int) ((x + .5) * (right - left) / cols));
                int a = before[py * width + px];
                int b = after[py * width + px];
                int r = Math.abs(((a >>> 16) & 255) - ((b >>> 16) & 255));
                int g = Math.abs(((a >>> 8) & 255) - ((b >>> 8) & 255));
                int bl = Math.abs((a & 255) - (b & 255));
                errors += r + g + bl;
                if (Math.max(r, Math.max(g, bl)) > 36) changed++;
                n++;
            }
        }
        return new Score(errors / (n * 3.0 * 255.0), changed / (double) n, n);
    }
}
