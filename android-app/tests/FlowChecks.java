import com.reqngo.manager.ClanTag;
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
        System.out.println("OK: tests de clan et de journal");
    }
}
