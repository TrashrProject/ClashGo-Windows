package com.reqngo.manager;

/** Detect completely black or nearly black captured frames without OCR. */
public final class CaptureQuality {
    private CaptureQuality() { }

    public static boolean nearlyBlack(int[] pixels) {
        if (pixels == null || pixels.length < 64)
            throw new IllegalArgumentException("Échantillon de capture trop petit");
        int dark = 0;
        for (int pixel : pixels) {
            int r = (pixel >> 16) & 255;
            int g = (pixel >> 8) & 255;
            int b = pixel & 255;
            if (r <= 22 && g <= 22 && b <= 22) dark++;
        }
        return dark >= Math.ceil(pixels.length * .98);
    }
}
