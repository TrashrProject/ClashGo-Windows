package com.reqngo.manager;

/** Pure readiness model for Android screenshot requests. */
public final class CaptureGate {
    public enum Reason {
        READY,
        ANDROID_TOO_OLD,
        SERVICE_NOT_CONNECTED,
        AUTOMATIC_CYCLE_ACTIVE,
        GESTURE_IN_PROGRESS,
        CAPTURE_IN_PROGRESS,
        GESTURE_ARMED
    }

    private CaptureGate() { }

    public static Reason check(boolean sdkSupported, boolean serviceConnected,
                               boolean autoRunning, boolean gesturePending,
                               boolean capturePending, boolean gestureArmed) {
        if (!sdkSupported) return Reason.ANDROID_TOO_OLD;
        if (!serviceConnected) return Reason.SERVICE_NOT_CONNECTED;
        if (autoRunning) return Reason.AUTOMATIC_CYCLE_ACTIVE;
        if (gesturePending) return Reason.GESTURE_IN_PROGRESS;
        if (capturePending) return Reason.CAPTURE_IN_PROGRESS;
        if (gestureArmed) return Reason.GESTURE_ARMED;
        return Reason.READY;
    }

    public static String explanation(Reason reason, boolean enabledInSettings) {
        switch (reason) {
            case READY: return "Prêt à capturer le jeu.";
            case ANDROID_TOO_OLD:
                return "La capture nécessite Android 11 ou supérieur.";
            case SERVICE_NOT_CONNECTED:
                return enabledInSettings
                    ? "Le service est autorisé dans Android mais n'est pas connecté à REQ N GO. "
                      + "Désactive puis réactive REQ N GO dans Accessibilité, puis reviens ici."
                    : "Le service REQ N GO n'est pas activé dans Accessibilité Android.";
            case AUTOMATIC_CYCLE_ACTIVE:
                return "Un cycle automatique est en cours : arrête-le avant de prendre une capture manuelle.";
            case GESTURE_IN_PROGRESS:
                return "Un geste Android est en cours : attends son résultat avant de recapturer.";
            case CAPTURE_IN_PROGRESS:
                return "Une capture est déjà en cours : attends sa fin. Elle sera automatiquement "
                    + "débloquée si Android ne répond pas.";
            case GESTURE_ARMED:
                return "Un geste est armé : contrôle ou annule ce geste avant de capturer à nouveau.";
            default: return "État de capture inconnu.";
        }
    }
}
