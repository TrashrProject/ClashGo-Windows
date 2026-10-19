import com.reqngo.manager.ClanTag;
import com.reqngo.manager.FrameFingerprint;
import com.reqngo.manager.RequestJournal;

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
        System.out.println("OK: tests de clan, journal et écran avant geste");
    }
}
