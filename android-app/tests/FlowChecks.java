import com.reqngo.manager.ClanTag;
import com.reqngo.manager.FrameFingerprint;
import com.reqngo.manager.RequestJournal;
import com.reqngo.manager.GuidedRoute;
import com.reqngo.manager.RouteTarget;
import com.reqngo.manager.DiagnosticTrail;

public final class FlowChecks {
    private static void check(boolean ok, String message) {
        if (!ok) throw new AssertionError(message);
    }

    private static void mustFail(Runnable action) {
        try { action.run(); throw new AssertionError("Une erreur était attendue"); }
        catch (IllegalArgumentException | IllegalStateException expected) { }
    }

    public static void main(String[] args) {
        check("#P0LY8".equals(ClanTag.normalize("  #p0ly8 ")), "normalisation");
        check(ClanTag.profileUrl("#P0LY8").endsWith("tag=%23P0LY8"), "lien du clan");
        mustFail(() -> ClanTag.normalize("https://other.site/"));
        mustFail(() -> ClanTag.normalize("####"));
        RequestJournal j = RequestJournal.fresh();
        mustFail(j::dispatched);
        j.arm();
        mustFail(j::arm);
        mustFail(j::reviewAndReset);
        j.dispatched();
        mustFail(j::reviewAndReset);
        j.confirmedByOperator();
        j.reviewAndReset();
        check(j.phase() == RequestJournal.Phase.READY, "revue puis réarmement");
        j.arm();
        j.block();
        j.reviewAndReset();
        check(j.phase() == RequestJournal.Phase.READY, "blocage puis revue");
        check(new RequestJournal("unknown").phase() == RequestJournal.Phase.BLOCKED,
                "journal corrompu : échec sûr");

        // New screen must match the calibrated reference before ANY input.
        int w = 48, h = 48;
        int[] before = new int[w * h];
        java.util.Arrays.fill(before, 0xff52657a);
        int[] unchanged = before.clone();
        FrameFingerprint.Score same = FrameFingerprint.compare(before, unchanged, w, h);
        check(same.similarEnoughForTap() && same.changedFraction == 0.0,
                "une capture identique doit être reconnue");
        int[] shifted = before.clone();
        java.util.Arrays.fill(shifted, 0xff101010);
        FrameFingerprint.Score different = FrameFingerprint.compare(before, shifted, w, h);
        check(!different.similarEnoughForTap() && different.changedFraction > .90,
                "un autre écran doit bloquer le geste");
        int[] animated = before.clone();
        for (int i = 0; i < 48; i++) animated[i] = 0xff101010;
        check(FrameFingerprint.compare(before, animated, w, h).similarEnoughForTap(),
                "une petite zone animée ne doit pas bloquer tous les gestes");
        mustFail(() -> FrameFingerprint.compare(before, shifted, w - 1, h));
        // Small popup near a selected button: global image looks stable,
        // but touching that button is unsafe.
        int[] popup = before.clone();
        for (int yy = 18; yy < 30; yy++)
            for (int xx = 18; xx < 30; xx++)
                popup[yy * w + xx] = 0xff101010;
        check(FrameFingerprint.compare(before, popup, w, h).similarEnoughForTap(),
                "global comparator intentionally overlooks a small popup");
        check(!FrameFingerprint.compareNearTap(before, popup, w, h, .5, .5)
                .similarEnoughForTap(),
                "local comparator must block a covered touch target");
        check(FrameFingerprint.compareNearTap(before, popup, w, h, .10, .10)
                .similarEnoughForTap(),
                "unrelated distant popup should not invalidate untouched corner");
        mustFail(() -> FrameFingerprint.compareNearTap(before, popup, w, h, -1, .5));
        // An accidental tap can NEVER advance a clan/request route.
        GuidedRoute route = GuidedRoute.fresh();
        check(route.index() == 0 && "village".equals(route.expectedBefore()),
                "première étape au village");
        mustFail(() -> route.start("clan_chat", true));
        mustFail(() -> route.start("village", false));
        route.start("village", true);
        check(route.phase() == GuidedRoute.Phase.AWAITING_REVIEW, "action incertaine");
        mustFail(() -> route.start("village", true));
        mustFail(() -> route.accept("clan_profile", false, true));
        mustFail(() -> route.accept("clan_profile", true, false));
        mustFail(() -> route.accept("request_posted", true, true));
        check(route.index() == 0, "aucun progrès sans preuve");
        route.accept("clan_profile", true, true);
        check(route.index() == 1 && route.phase() == GuidedRoute.Phase.READY,
                "une étape vérifiée uniquement");
        route.start("clan_profile", true);
        route.accept("clan_chat", true, true);
        route.start("clan_chat", true);
        route.accept("request_dialog", true, true);
        route.start("request_dialog", true);
        route.accept("request_posted", true, true);
        check(route.phase() == GuidedRoute.Phase.COMPLETE && route.index() == GuidedRoute.SIZE,
                "fin uniquement après les quatre étapes");
        mustFail(() -> route.start("request_posted", true));
        route.restartAfterReview();
        check(route.index() == 0 && route.phase() == GuidedRoute.Phase.READY,
                "reprise explicite après fin");
        route.start("village", true);
        route.block();
        mustFail(() -> route.accept("clan_profile", true, true));
        route.restartAfterReview();
        check(new GuidedRoute(-1, "READY").phase() == GuidedRoute.Phase.BLOCKED,
                "état hors limites bloqué");
        check(new GuidedRoute(2, "BAD").phase() == GuidedRoute.Phase.BLOCKED,
                "état corrompu bloqué");
        check(new GuidedRoute(GuidedRoute.SIZE, "READY").phase() == GuidedRoute.Phase.BLOCKED,
                "index terminé sans preuve bloqué");

        // A target is tied to one stage, screen resolution and reference image.
        String digest = "a".repeat(64), newDigest = "b".repeat(64);
        RouteTarget point = new RouteTarget(2, 720, 1280, .325f, .775f, digest);
        RouteTarget persisted = RouteTarget.decode(point.encode());
        check(persisted.step == 2 && Math.abs(persisted.x - .325f) < .00001f,
                "coordonnées enregistrées fidèlement");
        check(persisted.validFor(2, 720, 1280, digest), "référence inchangée");
        check(!persisted.validFor(2, 1080, 1920, digest), "résolution changée");
        check(!persisted.validFor(3, 720, 1280, digest), "autre étape");
        check(!persisted.validFor(2, 720, 1280, newDigest), "capture recalibrée");
        mustFail(() -> new RouteTarget(0, 720, 1280, Float.NaN, .5f, digest));
        mustFail(() -> new RouteTarget(0, 720, 1280, .5f, 1f, digest));
        mustFail(() -> new RouteTarget(-1, 720, 1280, .5f, .5f, digest));
        mustFail(() -> new RouteTarget(0, 720, 1280, .5f, .5f, "invalid"));
        mustFail(() -> RouteTarget.decode("1|bad|720|.5|.5|" + digest));
        mustFail(() -> RouteTarget.decode("0|720|1280|.5|.5|wrong"));
        mustFail(() -> RouteTarget.decode(null));
        // Shareable diagnostics allow only enumerated events and bounded history.
        String history = "";
        for (int i = 0; i < 100; i++) {
            history = DiagnosticTrail.append(history,
                    DiagnosticTrail.Event.CAPTURE_SAVED, i % (GuidedRoute.SIZE + 1),
                    1760000000000L + i);
        }
        check(DiagnosticTrail.parse(history).size() == DiagnosticTrail.MAX_EVENTS,
                "diagnostic history bounded to 60 events");
        check(!DiagnosticTrail.render(history, "READY", 1).contains("#P0LY"),
                "report cannot accidentally contain clan tag");
        check(DiagnosticTrail.parse("bad-data\n1760000000000|CAPTURE_SAVED|2")
                .size() == 1, "invalid history lines discarded");
        check(DiagnosticTrail.render("", "HACKED", -12).contains("BLOCKED"),
                "tampered state must fail closed");
        mustFail(() -> DiagnosticTrail.append("", null, 0, 100L));
        mustFail(() -> DiagnosticTrail.append("", DiagnosticTrail.Event.ROUTE_RESET, -1, 100L));
        mustFail(() -> DiagnosticTrail.append("", DiagnosticTrail.Event.ROUTE_RESET, 0, -1L));
        System.out.println("OK: tests de clan, journal, écran, parcours, calibrations et diagnostics");
    }
}
