import com.reqngo.manager.AutoRoutePolicy;
import com.reqngo.manager.GuidedRoute;
import com.reqngo.manager.ProfileQueue;

/** Regression checks for automatic visual flow and multi-profile isolation. */
public final class AutomationFlowChecks {
    static void check(boolean ok, String message) {
        if (!ok) throw new AssertionError(message);
    }
    static void fail(Runnable action) {
        try { action.run(); throw new AssertionError("Expected failure"); }
        catch (IllegalArgumentException | IllegalStateException expected) { }
    }

    public static void main(String[] args) {
        AutoRoutePolicy p = AutoRoutePolicy.fresh();
        fail(() -> p.start(false, true));
        fail(() -> p.start(true, false));
        p.start(true, true);
        fail(() -> p.dispatched());
        fail(() -> p.verifiedBefore("wrong", true, true));
        fail(() -> p.verifiedBefore("village", false, true));
        fail(() -> p.verifiedBefore("village", true, false));
        for (int i = 0; i < GuidedRoute.SIZE; i++) {
            check(p.index() == i && p.state() == AutoRoutePolicy.State.CHECKING,
                    "expected step");
            p.verifiedBefore(GuidedRoute.FROM[i], true, true);
            p.dispatched();
            fail(() -> p.dispatched());
            fail(() -> p.verifiedAfter("unexpected"));
            check(p.index() == i, "no advancement without observed result");
            p.verifiedAfter(GuidedRoute.TO[i]);
        }
        check(p.index() == GuidedRoute.SIZE
                && p.state() == AutoRoutePolicy.State.COMPLETE_VISUAL,
                "full route finishes visually only");
        fail(() -> p.start(true, true));
        AutoRoutePolicy interrupted = AutoRoutePolicy.recover(2, "VERIFYING");
        check(interrupted.state() == AutoRoutePolicy.State.BLOCKED,
                "never auto resume after unknown gesture");
        AutoRoutePolicy linked = AutoRoutePolicy.fresh();
        linked.startAt(1, true, true);
        check(linked.index() == 1 && "clan_profile".equals(linked.before()),
                "deep link starts from clan-profile verification");
        linked.verifiedBefore("clan_profile", true, true);
        linked.dispatched();
        linked.block();
        fail(() -> linked.verifiedAfter("clan_chat"));
        fail(() -> AutoRoutePolicy.fresh().startAt(4, true, true));
        check(AutoRoutePolicy.recover(4, "COMPLETE_VISUAL").state()
                == AutoRoutePolicy.State.COMPLETE_VISUAL, "terminal visual record retained");
        check(AutoRoutePolicy.recover(3, "bogus").state() == AutoRoutePolicy.State.BLOCKED,
                "tampered state blocked");
        check(AutoRoutePolicy.recover(-1, "IDLE").state() == AutoRoutePolicy.State.BLOCKED,
                "corrupt index blocked");

        ProfileQueue fleet = new ProfileQueue();
        fleet.add(" acc-001 ");
        fleet.add("acc-002");
        fail(() -> fleet.add("ACC-001"));
        fail(() -> fleet.add("name|token"));
        ProfileQueue.Item first = fleet.acquire(10_000);
        check(first != null && first.id.equals("ACC-001")
                && first.status == ProfileQueue.Status.NEEDS_OPERATOR_LOGIN,
                "first profile reserved for operator login");
        check(fleet.acquire(10_001) == null, "isolate pending login");
        fail(() -> fleet.operatorConfirmedLogin("ACC-002", 10_002));
        fleet.operatorConfirmedLogin(first.id, 10_002);
        check(fleet.acquire(10_003) == null, "only one active profile on this worker");
        fleet.finish(first.id, true);
        ProfileQueue.Item second = fleet.acquire(10_004);
        check(second != null && second.id.equals("ACC-002"), "next after completion");
        ProfileQueue restored = ProfileQueue.decode(fleet.encode());
        restored.reconcileAfterRestart();
        check(restored.get("ACC-002").status == ProfileQueue.Status.BLOCKED,
                "crashed profile cannot be silently reused");
        check(restored.get("ACC-001").status == ProfileQueue.Status.VISUAL_DONE,
                "previous result preserved");
        ProfileQueue expired = new ProfileQueue();
        expired.add("T1"); expired.add("T2");
        expired.acquire(1000);
        check("T2".equals(expired.acquire(1000 + ProfileQueue.LEASE_MS + 1).id),
                "expired profile blocked, next available");
        check(expired.get("T1").status == ProfileQueue.Status.BLOCKED,
                "expired lease protected");
        fail(() -> ProfileQueue.decode("v1\nBAD|ACTIVE|oops\n"));
        fail(() -> ProfileQueue.decode("v1\nBAD|UNKNOWN|20\n"));

        ProfileQueue large = new ProfileQueue();
        for (int i = 0; i < 5000; i++) large.add("ACC_" + i);
        check(large.items().size() == 5000, "5000 logical profiles");
        fail(() -> large.add("EXTRA"));
        check(ProfileQueue.decode(large.encode()).items().size() == 5000,
                "5000 profiles survive serialization");
        System.out.println("OK: auto-route transitions and 5000 logical profiles");
    }
}
