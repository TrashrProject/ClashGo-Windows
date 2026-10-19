package com.reqngo.manager;

import java.util.Locale;

/**
 * One operator-calibrated touch point bound to a verified screen reference.
 *
 * No game coordinates are hard-coded. A target is rejected when its step,
 * resolution or referenced screenshot changes.
 */
public final class RouteTarget {
    public final int step;
    public final int width;
    public final int height;
    public final float x;
    public final float y;
    public final String referenceSha256;

    public RouteTarget(int step, int width, int height, float x, float y,
                       String referenceSha256) {
        if (step < 0 || step >= GuidedRoute.SIZE || width < 20 || height < 20
                || !Float.isFinite(x) || !Float.isFinite(y)
                || x <= 0 || x >= 1 || y <= 0 || y >= 1
                || referenceSha256 == null
                || !referenceSha256.matches("[0-9a-f]{64}")) {
            throw new IllegalArgumentException("Calibration non valide");
        }
        this.step = step;
        this.width = width;
        this.height = height;
        this.x = x;
        this.y = y;
        this.referenceSha256 = referenceSha256;
    }

    public String encode() {
        return String.format(Locale.ROOT, "%d|%d|%d|%.8f|%.8f|%s",
                step, width, height, x, y, referenceSha256);
    }

    public static RouteTarget decode(String raw) {
        if (raw == null) throw new IllegalArgumentException("Aucune calibration");
        String[] parts = raw.split("\\|", -1);
        if (parts.length != 6) throw new IllegalArgumentException("Calibration illisible");
        try {
            return new RouteTarget(
                    Integer.parseInt(parts[0]),
                    Integer.parseInt(parts[1]),
                    Integer.parseInt(parts[2]),
                    Float.parseFloat(parts[3]),
                    Float.parseFloat(parts[4]),
                    parts[5]);
        } catch (NumberFormatException e) {
            throw new IllegalArgumentException("Calibration corrompue", e);
        }
    }

    public boolean validFor(int requestedStep, int actualWidth, int actualHeight,
                            String currentReferenceSha256) {
        return step == requestedStep && width == actualWidth && height == actualHeight
                && referenceSha256.equals(currentReferenceSha256);
    }
}
