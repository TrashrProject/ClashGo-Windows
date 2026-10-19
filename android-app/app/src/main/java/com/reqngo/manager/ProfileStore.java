package com.reqngo.manager;

import android.content.Context;
import android.content.SharedPreferences;

/**
 * Local logical roster. Reconciliation runs once per Android app PROCESS,
 * rejecting any stale lease instead of assuming that the game session survived.
 */
public final class ProfileStore {
    private static ProfileQueue cache;
    private static final String KEY = "fleet_roster_v1";
    private ProfileStore() { }

    private static SharedPreferences prefs(Context context) {
        return context.getSharedPreferences("local_preferences", Context.MODE_PRIVATE);
    }

    public static synchronized ProfileQueue load(Context context) {
        if (cache == null) {
            try {
                cache = ProfileQueue.decode(prefs(context).getString(KEY, ""));
            } catch (RuntimeException error) {
                // Do not overwrite damaged data silently; require explicit review.
                throw new IllegalStateException("Profils illisibles : diagnostic nécessaire", error);
            }
            cache.reconcileAfterRestart();
            if (!prefs(context).edit().putString(KEY, cache.encode()).commit())
                throw new IllegalStateException("Impossible de sécuriser les sessions");
        }
        return cache;
    }

    public static synchronized void persist(Context context) {
        if (cache == null) throw new IllegalStateException("File non chargée");
        if (!prefs(context).edit().putString(KEY, cache.encode()).commit())
            throw new IllegalStateException("Stockage des profils indisponible");
    }

    public static synchronized String summary(Context context) {
        ProfileQueue fleet = load(context);
        int waiting = 0, active = 0, needsLogin = 0, done = 0, blocked = 0;
        StringBuilder detail = new StringBuilder();
        for (ProfileQueue.Item entry : fleet.items()) {
            switch (entry.status) {
                case WAITING: waiting++; break;
                case ACTIVE: active++; break;
                case NEEDS_OPERATOR_LOGIN: needsLogin++; break;
                case VISUAL_DONE: done++; break;
                case BLOCKED: blocked++; break;
            }
            if (detail.length() < 700 && (entry.status == ProfileQueue.Status.ACTIVE
                    || entry.status == ProfileQueue.Status.NEEDS_OPERATOR_LOGIN))
                detail.append("\n").append(entry.id).append(" : ").append(entry.status);
        }
        return "Profils logiques : " + fleet.items().size()
                + "\nEn attente : " + waiting + " | Actif : " + active
                + " | Connexion à vérifier : " + needsLogin
                + "\nRésultats visuels : " + done + " | Bloqués : " + blocked
                + detail;
    }
}
