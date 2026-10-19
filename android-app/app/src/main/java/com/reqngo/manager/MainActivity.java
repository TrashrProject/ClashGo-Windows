package com.reqngo.manager;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.Color;
import android.graphics.Matrix;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.provider.Settings;
import android.view.MotionEvent;
import android.view.View;
import android.widget.Button;
import android.widget.EditText;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;
import java.io.File;

/** Native Android MVP. No simulated victory or account switching. */
public final class MainActivity extends Activity {
    private static final int BG = Color.rgb(16, 18, 22);
    private static final int SURFACE = Color.rgb(28, 31, 37);
    private static final int TEXT = Color.rgb(236, 234, 230);
    private static final int MUTED = Color.rgb(165, 169, 177);
    private static final int RED = Color.rgb(169, 75, 80);
    private LinearLayout screen;
    private TextView status;
    private TextView pointLabel;
    private EditText clan;
    private ImageView screenshot;
    private ImageView postScreenshot;
    private TextView comparison;
    private TextView detectedScreen;
    private TextView routeStatus;
    private long shownReferenceTime = -1L;
    private long selectedReferenceTime = -1L;
    private float selectedX = -1, selectedY = -1;

    private SharedPreferences prefs() {
        return getSharedPreferences("local_preferences", MODE_PRIVATE);
    }

    private int dp(float value) {
        return (int) (value * getResources().getDisplayMetrics().density + .5f);
    }

    private GradientDrawable background(int color, int stroke) {
        GradientDrawable d = new GradientDrawable();
        d.setColor(color); d.setCornerRadius(dp(12));
        if (stroke != 0) d.setStroke(dp(1), stroke);
        return d;
    }

    private TextView text(String value, float size, int color) {
        TextView t = new TextView(this);
        t.setText(value); t.setTextSize(size); t.setTextColor(color);
        t.setLineSpacing(dp(3), 1f);
        return t;
    }

    private void section(String heading, String description) {
        TextView title = text(heading, 18, TEXT);
        title.setTypeface(null, 1);
        LinearLayout.LayoutParams a = new LinearLayout.LayoutParams(-1, -2);
        a.topMargin = dp(24);
        screen.addView(title, a);
        TextView desc = text(description, 13, MUTED);
        LinearLayout.LayoutParams b = new LinearLayout.LayoutParams(-1, -2);
        b.topMargin = dp(5);
        screen.addView(desc, b);
    }

    private Button action(String label, boolean primary, View.OnClickListener listener) {
        Button b = new Button(this);
        b.setAllCaps(false); b.setText(label); b.setTextColor(TEXT);
        b.setTextSize(14);
        b.setBackground(background(primary ? RED : SURFACE, primary ? 0 : Color.rgb(63,67,72)));
        LinearLayout.LayoutParams p = new LinearLayout.LayoutParams(-1, dp(52));
        p.topMargin = dp(11);
        screen.addView(b, p);
        b.setOnClickListener(listener);
        return b;
    }

    private void toast(String value) { Toast.makeText(this, value, Toast.LENGTH_LONG).show(); }

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        getWindow().setStatusBarColor(BG);
        getWindow().setNavigationBarColor(BG);
        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        scroll.setBackgroundColor(BG);
        screen = new LinearLayout(this);
        screen.setOrientation(LinearLayout.VERTICAL);
        screen.setPadding(dp(22), dp(25), dp(22), dp(36));
        scroll.addView(screen);
        setContentView(scroll);

        TextView brand = text("REQ N GO", 30, TEXT);
        brand.setTypeface(null, 1);
        screen.addView(brand);
        TextView alpha = text("ANDROID  /  PREMIER PILOTE", 12, RED);
        screen.addView(alpha);
        TextView lead = text("Capture locale, accès rapide au clan et geste de test contrôlé. Aucune demande automatisée n'est encore validée.", 14, MUTED);
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(-1, -2);
        lp.topMargin = dp(14);
        screen.addView(lead, lp);

        section("01  /  Autorisations", "Active volontairement le service Android. Il n'envoie aucun geste tant que tu ne confirmes pas une opération.");
        action("Ouvrir les paramètres d'accessibilité", false, v ->
                startActivity(new Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS)));
        status = text("", 13, TEXT);
        LinearLayout.LayoutParams sp = new LinearLayout.LayoutParams(-1, -2);
        sp.topMargin = dp(12); sp.bottomMargin = dp(4);
        status.setPadding(dp(14), dp(14), dp(14), dp(14));
        status.setBackground(background(SURFACE, 0));
        screen.addView(status, sp);

        section("02  /  Clan cible", "Saisis le tag exact du clan. Le raccourci ouvre sa fiche : il ne contourne pas les conditions d'adhésion.");
        clan = new EditText(this);
        clan.setSingleLine(true);
        clan.setTextSize(17);
        clan.setTextColor(TEXT);
        clan.setHintTextColor(MUTED);
        clan.setHint("#TAG_DU_CLAN");
        clan.setBackground(background(SURFACE, Color.rgb(63,67,72)));
        clan.setPadding(dp(14), dp(12), dp(14), dp(12));
        clan.setText(prefs().getString("clan_tag", ""));
        LinearLayout.LayoutParams cp = new LinearLayout.LayoutParams(-1, dp(54));
        cp.topMargin = dp(12);
        screen.addView(clan, cp);
        action("Enregistrer le clan", false, v -> saveTag());
        action("Ouvrir la fiche du clan", true, v -> openClan());
        action("Ouvrir Clash of Clans", false, v -> openGame());

        section("03  /  Capture du jeu", "Android 11 minimum pour la capture par accessibilité. L'image reste dans le stockage privé de cette APK, jamais sur un serveur.");
        action("Capturer le jeu (retour automatique)", true, v -> {
            if (Build.VERSION.SDK_INT < 30) {
                toast("La capture exige Android 11 ou plus sur MuMu."); return;
            }
            if (!GameBridgeService.captureAfterReturnToGame()) {
                toast("Active d'abord le service Android."); return;
            }
            if (!openGame()) {
                GameBridgeService.cancelPending();
                toast("Clash of Clans Google Play introuvable.");
            }
        });
        action("Actualiser la capture", false, v -> refresh());
        screenshot = new ImageView(this);
        screenshot.setAdjustViewBounds(true);
        screenshot.setScaleType(ImageView.ScaleType.FIT_CENTER);
        screenshot.setBackground(background(SURFACE, Color.rgb(63,67,72)));
        LinearLayout.LayoutParams ip = new LinearLayout.LayoutParams(-1, dp(240));
        ip.topMargin = dp(12);
        screen.addView(screenshot, ip);
        screenshot.setOnTouchListener((view, event) -> {
            if (event.getAction() != MotionEvent.ACTION_UP || screenshot.getDrawable() == null) return true;
            Matrix inverse = new Matrix();
            if (!screenshot.getImageMatrix().invert(inverse)) return true;
            float[] mapped = {event.getX() - screenshot.getPaddingLeft(),
                    event.getY() - screenshot.getPaddingTop()};
            inverse.mapPoints(mapped);
            float width = screenshot.getDrawable().getIntrinsicWidth();
            float height = screenshot.getDrawable().getIntrinsicHeight();
            if (width <= 0 || height <= 0 || mapped[0] <= 0 || mapped[0] >= width
                    || mapped[1] <= 0 || mapped[1] >= height) return true;
            selectedX = mapped[0] / width;
            selectedY = mapped[1] / height;
            selectedReferenceTime = prefs().getLong("capture_time", -1L);
            pointLabel.setText(String.format(java.util.Locale.ROOT,
                    "Point choisi : %.1f %% × %.1f %%", selectedX * 100, selectedY * 100));
            return true;
        });

        section("04  /  Calibration des cinq écrans",
                "Sur MuMu, capture d'abord chaque étape réelle. Mémorise localement : village, fiche clan, chat, fenêtre de demande et demande publiée. L'application signale les ambiguïtés.");
        action("Mémoriser la capture comme état connu", false, v -> chooseScreenState());
        action("Identifier l'écran capturé", false, v -> identifyScreen());
        detectedScreen = text("Aucun écran analysé.", 13, MUTED);
        LinearLayout.LayoutParams detectedLp = new LinearLayout.LayoutParams(-1, -2);
        detectedLp.topMargin = dp(10);
        screen.addView(detectedScreen, detectedLp);

        section("05  /  Contrôle avant et après l'appui",
                "Touche un point de la capture. Avant le geste, l'APK recapture le jeu et bloque l'action si l'écran diffère trop. Le résultat sera photographié sans être déclaré réussi.");
        pointLabel = text("Aucun point sélectionné", 13, MUTED);
        screen.addView(pointLabel);
        action("Envoyer un seul appui de test", true, v -> confirmTap());
        comparison = text("Aucune mesure visuelle après action.", 12, MUTED);
        LinearLayout.LayoutParams comp = new LinearLayout.LayoutParams(-1, -2);
        comp.topMargin = dp(16);
        screen.addView(comparison, comp);
        TextView afterTitle = text("CAPTURE APRÈS LE GESTE (PREUVE VISUELLE)", 12, MUTED);
        LinearLayout.LayoutParams afterLp = new LinearLayout.LayoutParams(-1, -2);
        afterLp.topMargin = dp(14);
        screen.addView(afterTitle, afterLp);
        postScreenshot = new ImageView(this);
        postScreenshot.setAdjustViewBounds(true);
        postScreenshot.setScaleType(ImageView.ScaleType.FIT_CENTER);
        postScreenshot.setBackground(background(SURFACE, Color.rgb(63,67,72)));
        LinearLayout.LayoutParams postLp = new LinearLayout.LayoutParams(-1, dp(210));
        postLp.topMargin = dp(9);
        screen.addView(postScreenshot, postLp);
        action("Confirmer manuellement l'effet constaté", false, v -> review(true));
        action("Signaler échec / résultat incertain", false, v -> review(false));
        action("Réarmer après examen", false, v -> resetJournal());
        action("ARRÊT : annuler toute action en attente", false, v -> {
            GameBridgeService.cancelPending();
            prefs().edit().putString("journal", "BLOCKED").commit();
            refresh(); toast("Actions en attente annulées.");
        });
        section("06  /  Parcours guidé : clan → renforts",
                "Quatre transitions encadrées : village → fiche du clan → chat du clan → fenêtre de demande → demande visible. Chaque geste exige un écran de départ reconnu et chaque passage à l'étape suivante exige une capture d'arrivée et ta confirmation. Aucune action n'est répétée automatiquement.");
        routeStatus = text("Parcours en attente de vérification.", 14, TEXT);
        routeStatus.setPadding(dp(13), dp(13), dp(13), dp(13));
        routeStatus.setBackground(background(SURFACE, 0));
        screen.addView(routeStatus);
        action("Exécuter UN geste de l'étape guidée", true, v -> prepareGuidedStep());
        action("Vérifier et valider l'étape par la capture d'arrivée", false, v -> approveGuidedStep());
        action("Bloquer le parcours et annuler toute action en attente", false, v -> stopGuidedRoute());
        action("Réinitialiser le parcours après examen", false, v -> restartGuidedRoute());

        section("État du pilote", "Ce parcours est guidé, PAS autonome : aucune adhésion au clan, demande réussie ou changement de compte ne sont supposés. Les écrans réels doivent d'abord être calibrés sur MuMu. Automatiser le gameplay peut enfreindre les règles de Supercell.");
        refresh();
    }

    private GuidedRoute route() {
        return new GuidedRoute(prefs().getInt("route_index", 0),
                prefs().getString("route_phase", "READY"));
    }

    private boolean saveRoute(GuidedRoute route) {
        return prefs().edit().putInt("route_index", route.index())
                .putString("route_phase", route.phase().name()).commit();
    }

    private void prepareGuidedStep() {
        GuidedRoute route = route();
        if (route.phase() != GuidedRoute.Phase.READY) {
            toast("Parcours non prêt : vérifie ou réinitialise l'étape en cours."); return;
        }
        if (selectedX < 0 || selectedY < 0 || selectedReferenceTime <= 0
                || selectedReferenceTime > System.currentTimeMillis()
                || selectedReferenceTime != prefs().getLong("capture_time", -1L)
                || System.currentTimeMillis() - selectedReferenceTime > 120000) {
            toast("Capture récente et point de geste requis pour cette étape."); return;
        }
        if (new RequestJournal(prefs().getString("journal", "READY")).phase()
                != RequestJournal.Phase.READY) {
            toast("Journal de geste non prêt : examine l'action précédente."); return;
        }
        final ScreenCatalog.Result identified;
        try {
            identified = ScreenCatalog.identify(this);
            if (!route.expectedBefore().equals(identified.id)) {
                toast("Écran de départ non identifié : attendu « "
                        + route.expectedBefore() + " », obtenu « " + identified.state + " ».");
                return;
            }
        } catch (RuntimeException e) {
            toast("Capture ou références absentes : impossible d'armer l'étape."); return;
        }
        int currentStep = route.index();
        new AlertDialog.Builder(this)
                .setTitle("Étape " + (currentStep + 1) + " / " + GuidedRoute.SIZE)
                .setMessage(GuidedRoute.TITLE[currentStep]
                        + "\nÉcran reconnu : " + identified.state
                        + "\nDestination attendue : " + route.expectedAfter()
                        + "\nAutoriser exactement un appui sur le point choisi ? "
                        + "Le résultat ne sera jamais considéré comme réussi automatiquement.")
                .setNegativeButton("Annuler", null)
                .setPositiveButton("Autoriser un geste", (dialog, which) -> {
                    GuidedRoute live = route();
                    if (live.phase() != GuidedRoute.Phase.READY
                            || live.index() != currentStep
                            || selectedReferenceTime != prefs().getLong("capture_time", -1L)) {
                        toast("Contexte modifié : capture à recommencer."); return;
                    }
                    try {
                        // Durable marker is stored BEFORE possible dispatch.
                        live.start(identified.id, true);
                        if (!prefs().edit().putInt("route_index", live.index())
                                .putString("route_phase", live.phase().name())
                                .putLong("route_started_ms", System.currentTimeMillis())
                                .commit()) throw new IllegalStateException("Journal inaccessible");
                        if (!GameBridgeService.armSingleTap(selectedX, selectedY)) {
                            live.block(); saveRoute(live);
                            toast("Action refusée : parcours bloqué, aucune répétition automatique.");
                            refresh(); return;
                        }
                        if (!openGame()) {
                            GameBridgeService.cancelPending();
                            prefs().edit().putString("journal", "BLOCKED").commit();
                            live.block(); saveRoute(live);
                            toast("Jeu inaccessible : parcours arrêté.");
                        }
                        refresh();
                    } catch (RuntimeException error) {
                        live.block(); saveRoute(live);
                        toast("Erreur de parcours : " + error.getMessage());
                        refresh();
                    }
                }).show();
    }

    private void approveGuidedStep() {
        GuidedRoute route = route();
        if (route.phase() != GuidedRoute.Phase.AWAITING_REVIEW) {
            toast("Aucune étape guidée en attente."); return;
        }
        if (new RequestJournal(prefs().getString("journal", "BLOCKED")).phase()
                != RequestJournal.Phase.DISPATCHED_UNVERIFIED) {
            toast("Le journal ne confirme pas l'envoi d'un geste : parcours bloqué."); return;
        }
        File evidence = new File(getFilesDir(), "post_action_capture.png");
        long started = prefs().getLong("route_started_ms", Long.MAX_VALUE);
        if (!evidence.isFile() || evidence.lastModified() < started) {
            toast("Capture après-action absente ou périmée. Ne pas supposer la réussite."); return;
        }
        ScreenCatalog.Result detected;
        try {
            detected = ScreenCatalog.identifyPostAction(this);
            if (!route.expectedAfter().equals(detected.id)) {
                toast("Écran d'arrivée non confirmé : attendu " + route.expectedAfter()
                        + ", détecté " + detected.state); return;
            }
        } catch (RuntimeException error) {
            toast("Écran après-action impossible à reconnaître."); return;
        }
        final int currentStep = route.index();
        new AlertDialog.Builder(this)
                .setTitle("Confirmer le résultat dans Clash of Clans")
                .setMessage("Capture d'arrivée : " + detected.state
                        + "\nVérifie MANUELLEMENT le clan, le bouton ou la demande dans le jeu. "
                        + "Une ressemblance visuelle ne prouve ni l'adhésion ni l'envoi. "
                        + "Confirmer explicitement que l'étape est réussie ?")
                .setNegativeButton("Non, résultat incertain", null)
                .setPositiveButton("J'ai vérifié dans le jeu", (dialog, which) -> {
                    try {
                        GuidedRoute live = route();
                        if (live.index() != currentStep
                                || live.phase() != GuidedRoute.Phase.AWAITING_REVIEW
                                || new RequestJournal(prefs().getString("journal", "BLOCKED"))
                                .phase() != RequestJournal.Phase.DISPATCHED_UNVERIFIED
                                || evidence.lastModified() < started) {
                            toast("Situation modifiée : validation annulée."); return;
                        }
                        // Recheck after user confirmation. Never trust a stale preview.
                        ScreenCatalog.Result last = ScreenCatalog.identifyPostAction(this);
                        live.accept(last.id, true, true);
                        if (!prefs().edit().putInt("route_index", live.index())
                                .putString("route_phase", live.phase().name())
                                .putString("journal", "CONFIRMED").commit()) {
                            toast("Enregistrement impossible : ne pas continuer."); return;
                        }
                        toast("Étape revue et enregistrée. Capture le prochain écran pour continuer.");
                        refresh();
                    } catch (RuntimeException e) {
                        toast("Validation refusée : résultat incertain."); 
                    }
                }).show();
    }

    private void stopGuidedRoute() {
        new AlertDialog.Builder(this)
                .setTitle("Interrompre le parcours")
                .setMessage("Annuler toute action en attente et bloquer le parcours. "
                        + "Un geste déjà envoyé ne peut pas être annulé dans le jeu.")
                .setNegativeButton("Retour", null)
                .setPositiveButton("Bloquer", (dialog, which) -> {
                    GameBridgeService.cancelPending();
                    GuidedRoute progress = route();
                    progress.block();
                    prefs().edit().putInt("route_index", progress.index())
                            .putString("route_phase", progress.phase().name())
                            .putString("journal", "BLOCKED").commit();
                    refresh();
                }).show();
    }

    private void restartGuidedRoute() {
        GuidedRoute progress = route();
        RequestJournal j = new RequestJournal(prefs().getString("journal", "BLOCKED"));
        if (progress.phase() != GuidedRoute.Phase.BLOCKED
                && progress.phase() != GuidedRoute.Phase.COMPLETE) {
            toast("Bloque ou termine d'abord le parcours."); return;
        }
        if (j.phase() != RequestJournal.Phase.BLOCKED
                && j.phase() != RequestJournal.Phase.CONFIRMED) {
            toast("Une action est incertaine : bloque-la et examine le jeu."); return;
        }
        new AlertDialog.Builder(this).setTitle("Nouvelle tentative")
                .setMessage("Tu confirmes avoir vérifié le jeu après toute action incertaine ? "
                        + "La reprise repartira du village, sans répéter automatiquement l'action précédente.")
                .setNegativeButton("Annuler", null)
                .setPositiveButton("J'ai vérifié", (dialog, which) -> {
                    GuidedRoute live = route();
                    try {
                        live.restartAfterReview();
                        GameBridgeService.cancelPending();
                        if (!prefs().edit().putInt("route_index", live.index())
                                .putString("route_phase", live.phase().name())
                                .putString("journal", "READY")
                                .remove("route_started_ms").commit()) {
                            throw new IllegalStateException("Stockage inaccessible");
                        }
                        refresh();
                    } catch (RuntimeException error) {
                        toast("Impossible de redémarrer la séquence.");
                    }
                }).show();
    }

    private void chooseScreenState() {
        long time = prefs().getLong("capture_time", 0L);
        RequestJournal.Phase phase = new RequestJournal(
                prefs().getString("journal", "BLOCKED")).phase();
        if (time <= 0 || time > System.currentTimeMillis()
                || System.currentTimeMillis() - time > 120000
                || phase == RequestJournal.Phase.ARMED
                || phase == RequestJournal.Phase.DISPATCHED_UNVERIFIED) {
            toast("Capture trop ancienne ou geste incertain : vérifie le jeu et recapture."); return;
        }
        new AlertDialog.Builder(this).setTitle("Quel écran du jeu ?")
                .setItems(ScreenCatalog.LABELS, (dialog, which) -> {
                    final String id = ScreenCatalog.IDS[which];
                    new AlertDialog.Builder(this)
                            .setTitle("Enregistrer cet état")
                            .setMessage("La capture actuelle remplacera l'ancienne référence de « "
                                    + ScreenCatalog.LABELS[which] +
                                    " ». Vérifie qu'elle ne contient aucune connexion ou donnée sensible.")
                            .setNegativeButton("Annuler", null)
                            .setPositiveButton("Enregistrer", (d, index) -> {
                                try {
                                    if (time != prefs().getLong("capture_time", -1L))
                                        throw new IllegalStateException("La capture a changé");
                                    ScreenCatalog.save(this, id);
                                    detectedScreen.setText("État enregistré localement : "
                                            + ScreenCatalog.LABELS[which]);
                                } catch (Exception e) {
                                    toast("Enregistrement impossible : " + e.getClass().getSimpleName());
                                }
                            }).show();
                }).show();
    }

    private void identifyScreen() {
        RequestJournal.Phase phase = new RequestJournal(
                prefs().getString("journal", "BLOCKED")).phase();
        if (phase == RequestJournal.Phase.ARMED
                || phase == RequestJournal.Phase.DISPATCHED_UNVERIFIED) {
            toast("Termine la revue du geste avant une autre calibration."); return;
        }
        try {
            ScreenCatalog.Result result = ScreenCatalog.identify(this);
            if (result.enrolled == 0) {
                detectedScreen.setText("Aucun état enregistré : capture et mémorise les écrans de MuMu.");
                return;
            }
            String difference = Double.isFinite(result.difference)
                    ? String.format(java.util.Locale.ROOT, "%.2f %%", result.difference * 100)
                    : "inconnue";
            detectedScreen.setText("Écran probable : " + result.state
                    + "\nÉcart visuel : " + difference + " | Références : " + result.enrolled
                    + "\nIdentification indicative, pas une preuve de demande réussie.");
        } catch (Exception e) {
            toast("Identification impossible : " + e.getClass().getSimpleName());
        }
    }

    private boolean saveTag() {
        try {
            String tag = ClanTag.normalize(clan.getText().toString());
            if (!prefs().edit().putString("clan_tag", tag).commit()) throw new IllegalStateException("Stockage indisponible");
            clan.setText(tag);
            toast("Clan enregistré : " + tag);
            return true;
        } catch (RuntimeException e) {
            toast(e.getMessage()); return false;
        }
    }

    private void openClan() {
        if (!saveTag()) return;
        try {
            Uri uri = Uri.parse(ClanTag.profileUrl(clan.getText().toString()));
            startActivity(new Intent(Intent.ACTION_VIEW, uri).addCategory(Intent.CATEGORY_BROWSABLE));
        } catch (Exception e) {
            toast("Impossible d'ouvrir le lien. Utilise la recherche par tag dans le jeu.");
        }
    }

    private boolean openGame() {
        Intent launch = getPackageManager().getLaunchIntentForPackage(GameBridgeService.GAME);
        if (launch == null) return false;
        try { startActivity(launch); return true; }
        catch (RuntimeException e) { return false; }
    }

    private void confirmTap() {
        if (route().phase() == GuidedRoute.Phase.AWAITING_REVIEW) {
            toast("Étape guidée en cours : utilise sa validation, pas le geste libre."); return;
        }
        if (selectedX < 0 || selectedY < 0 || selectedReferenceTime <= 0
                || selectedReferenceTime != prefs().getLong("capture_time", -1L)) {
            toast("Choisis un point sur la capture de référence actuelle."); return;
        }
        new AlertDialog.Builder(this)
                .setTitle("Un seul geste Android")
                .setMessage("Autoriser un appui sur le point choisi ? Le jeu sera ouvert. Un geste envoyé ne signifie PAS qu'une demande de renforts a réussi.")
                .setNegativeButton("Annuler", null)
                .setPositiveButton("Autoriser", (dialog, which) -> {
                    if (!GameBridgeService.armSingleTap(selectedX, selectedY)) {
                        toast("Geste bloqué : service, capture récente ou journal non prêt."); return;
                    }
                    if (!openGame()) {
                        GameBridgeService.cancelPending();
                        prefs().edit().putString("journal", "BLOCKED").commit();
                        toast("Clash of Clans introuvable.");
                    }
                }).show();
    }

    private void review(boolean success) {
        if (route().phase() == GuidedRoute.Phase.AWAITING_REVIEW) {
            toast("Valide ou bloque le parcours guidé dans sa section."); return;
        }
        RequestJournal j = new RequestJournal(prefs().getString("journal", "BLOCKED"));
        try {
            if (success) j.confirmedByOperator();
            else j.block();
            prefs().edit().putString("journal", j.phase().name()).commit();
            refresh();
        } catch (IllegalStateException e) { toast(e.getMessage()); }
    }

    private void resetJournal() {
        if (route().phase() == GuidedRoute.Phase.AWAITING_REVIEW) {
            toast("Ne réarme pas pendant une étape guidée incertaine."); return;
        }
        RequestJournal j = new RequestJournal(prefs().getString("journal", "BLOCKED"));
        try {
            j.reviewAndReset();
            prefs().edit().putString("journal", j.phase().name()).commit();
            refresh();
        } catch (IllegalStateException e) { toast(e.getMessage()); }
    }

    private void refresh() {
        if (status == null || screenshot == null) return;
        if (routeStatus != null) {
            GuidedRoute current = route();
            String states = current.index() < GuidedRoute.SIZE
                    ? "\nÉcran requis : " + current.expectedBefore()
                        + " → " + current.expectedAfter()
                    : "\\nAucune autre étape";
            routeStatus.setText(current.summary() + states);
        }
        long referenceTime = prefs().getLong("capture_time", -1L);
        if (referenceTime != shownReferenceTime) {
            shownReferenceTime = referenceTime;
            selectedReferenceTime = -1L;
            selectedX = -1f;
            selectedY = -1f;
            if (pointLabel != null) pointLabel.setText("Choisis un nouveau point sur cette capture.");
        }
        if (comparison != null) comparison.setText(
                prefs().getString("comparison", "Aucune mesure après geste — confirmation manuelle requise."));
        if (postScreenshot != null) {
            File post = new File(getFilesDir(), "post_action_capture.png");
            Bitmap later = post.isFile() ? BitmapFactory.decodeFile(post.getAbsolutePath()) : null;
            postScreenshot.setImageBitmap(later);
        }
        String availability = GameBridgeService.connected() ? "Service activé" : "Service inactif";
        String journal = prefs().getString("journal", "READY");
        String message = prefs().getString("status", "En attente d'une première capture.");
        status.setText(availability + "\nJournal : " + journal + "\n" + message);
        File file = new File(getFilesDir(), "latest_game_capture.png");
        if (!file.isFile()) return;
        Bitmap bitmap = BitmapFactory.decodeFile(file.getAbsolutePath());
        if (bitmap != null) screenshot.setImageBitmap(bitmap);
    }

    @Override protected void onResume() {
        super.onResume();
        if (status != null && screenshot != null) refresh();
    }
}
