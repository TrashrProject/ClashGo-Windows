package com.reqngo.manager;

import android.content.Context;
import android.content.SharedPreferences;

/** Persistence wrapper; never stores arbitrary logs in the exportable trail. */
public final class DiagnosticStore {
    private static final String PREFS = "local_preferences";
    private static final String KEY = "safe_events_v1";

    private DiagnosticStore() { }

    public static synchronized void record(Context context, DiagnosticTrail.Event event) {
        SharedPreferences prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        int step = prefs.getInt("route_index", 0);
        if (step < 0 || step > GuidedRoute.SIZE) step = 0;
        String events = DiagnosticTrail.append(
                prefs.getString(KEY, ""), event, step, System.currentTimeMillis());
        prefs.edit().putString(KEY, events).apply();
    }

    public static String report(Context context) {
        SharedPreferences prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        return DiagnosticTrail.render(prefs.getString(KEY, ""),
                prefs.getString("route_phase", "READY"), prefs.getInt("route_index", 0));
    }
}
