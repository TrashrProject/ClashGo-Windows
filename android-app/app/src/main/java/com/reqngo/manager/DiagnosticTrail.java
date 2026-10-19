package com.reqngo.manager;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

/**
 * Bounded privacy-first device diagnostic trail.
 *
 * It deliberately accepts only an enum, timestamp and route index. Never
 * capture clan tags, player identities, screenshot pixels, account sessions
 * or free-form exception text in the shareable diagnostic report.
 */
public final class DiagnosticTrail {
    public enum Event {
        SERVICE_CONNECTED, SERVICE_STOPPED, SERVICE_INTERRUPTED,
        CAPTURE_REQUESTED, CAPTURE_SAVED, CAPTURE_REJECTED,
        GESTURE_ARMED, GESTURE_SENT, GESTURE_CANCELLED, GESTURE_BLOCKED,
        ROUTE_STEP_STARTED, ROUTE_STEP_APPROVED, ROUTE_BLOCKED, ROUTE_RESET,
        REFERENCE_RECORDED, BUTTON_CALIBRATED,
        GAME_CLAN_CLOSED, GAME_INVITATION, GAME_JOIN_PENDING,
        GAME_REQUEST_COOLDOWN, GAME_CASTLE_FULL, GAME_CONNECTION_ERROR
    }

    public static final int MAX_EVENTS = 60;
    private DiagnosticTrail() { }

    public static String append(String existing, Event event, int step, long timestampMs) {
        if (event == null || timestampMs <= 0 || step < 0 || step > GuidedRoute.SIZE)
            throw new IllegalArgumentException("Diagnostic invalide");
        List<String> valid = parse(existing);
        valid.add(timestampMs + "|" + event.name() + "|" + step);
        if (valid.size() > MAX_EVENTS)
            valid = valid.subList(valid.size() - MAX_EVENTS, valid.size());
        return String.join("\n", valid);
    }

    public static List<String> parse(String saved) {
        ArrayList<String> records = new ArrayList<>();
        if (saved == null || saved.isEmpty()) return records;
        String[] lines = saved.split("\n", -1);
        // Process only a bounded tail even when prior storage was corrupted.
        for (int i = Math.max(0, lines.length - MAX_EVENTS); i < lines.length; i++) {
            String line = lines[i];
            String[] pieces = line.split("\\|", -1);
            if (pieces.length != 3) continue;
            try {
                long timestamp = Long.parseLong(pieces[0]);
                int step = Integer.parseInt(pieces[2]);
                Event.valueOf(pieces[1]);
                if (timestamp <= 0 || step < 0 || step > GuidedRoute.SIZE) continue;
                records.add(line);
            } catch (RuntimeException ignored) { /* discard tampered record */ }
        }
        return records;
    }

    public static String render(String saved, String phase, int index) {
        // Never copy raw stored phase without validating its shape.
        String safePhase;
        try { safePhase = GuidedRoute.Phase.valueOf(phase).name(); }
        catch (RuntimeException e) { safePhase = "BLOCKED"; }
        int safeIndex = (index >= 0 && index <= GuidedRoute.SIZE) ? index : -1;
        StringBuilder builder = new StringBuilder();
        builder.append("REQ N GO / DIAGNOSTICS LOCAUX\n")
                .append("Format: timestamp_ms | code | step\n")
                .append("Route: ").append(safePhase).append(" / ")
                .append(safeIndex).append("/").append(GuidedRoute.SIZE)
                .append("\nCaptures et identifiants NON inclus.\n");
        for (String line : parse(saved)) builder.append(line).append("\n");
        return builder.toString();
    }
}
