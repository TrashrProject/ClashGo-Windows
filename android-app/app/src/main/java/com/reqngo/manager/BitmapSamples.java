package com.reqngo.manager;

import android.graphics.Bitmap;

/**
 * Fixed-size spatial sample from an Android screenshot.
 *
 * The comparison engine itself samples a 48x48 grid. Avoid allocating
 * full-screen int arrays (several MB per frame and per enrolled state).
 * This representation is for recognition ONLY, not a replacement for the
 * original, private PNG evidence.
 */
public final class BitmapSamples {
    public static final int WIDTH = 96;
    public static final int HEIGHT = 96;
    private BitmapSamples() { }

    public static int[] extract(Bitmap bitmap) {
        if (bitmap == null || bitmap.isRecycled()
                || bitmap.getWidth() < 20 || bitmap.getHeight() < 20)
            throw new IllegalArgumentException("Capture invalide");
        int[] pixels = new int[WIDTH * HEIGHT];
        for (int y = 0; y < HEIGHT; y++) {
            int sy = Math.min(bitmap.getHeight() - 1,
                    (int) ((y + .5) * bitmap.getHeight() / HEIGHT));
            for (int x = 0; x < WIDTH; x++) {
                int sx = Math.min(bitmap.getWidth() - 1,
                        (int) ((x + .5) * bitmap.getWidth() / WIDTH));
                pixels[y * WIDTH + x] = bitmap.getPixel(sx, sy);
            }
        }
        return pixels;
    }

    public static FrameFingerprint.Score full(Bitmap baseline, Bitmap current) {
        sameDimensions(baseline, current);
        return FrameFingerprint.compare(extract(baseline), extract(current), WIDTH, HEIGHT);
    }

    public static FrameFingerprint.Score local(Bitmap baseline, Bitmap current, float x, float y) {
        sameDimensions(baseline, current);
        return FrameFingerprint.compareNearTap(extract(baseline), extract(current),
                WIDTH, HEIGHT, x, y);
    }

    private static void sameDimensions(Bitmap a, Bitmap b) {
        if (a == null || b == null || a.getWidth() != b.getWidth()
                || a.getHeight() != b.getHeight())
            throw new IllegalArgumentException("Dimensions d'écran incompatibles");
    }
}
