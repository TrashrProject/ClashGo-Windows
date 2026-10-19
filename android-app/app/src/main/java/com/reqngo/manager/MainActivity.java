package com.reqngo.manager;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.ClipboardManager;
import android.content.ClipData;
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
import android.media.projection.MediaProjectionManager;
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
import java.io.InputStream;
import java.io.OutputStream;

/** Native Android MVP. No simulated victory or account switching. */
public final class MainActivity extends Activity {
    private static final int PROJECTION_REQUEST = 4120;
    private static final int CALIBRATION_EXPORT_REQUEST = 5130;
    private static final int CALIBRATION_IMPORT_REQUEST = 5131;
    private static final int AUTO_PROJECTION_REQUEST = 4121;
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
    private TextView captureNotice;
    private ImageView postScreenshot;
    private TextView comparison;
    private TextView detectedScreen;
    private TextView routeStatus;
    private TextView referencesStatus;
    private TextView fleetStatus;
    private TextView autoStatus;
    private EditText profileIdInput;
    private int calibrationPreviewStep = -1;
    private int pendingAutoStartIndex = -1;
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
            CaptureGate.Reason availability = GameBridgeService.captureReadiness(this);
            if (availability != CaptureGate.Reason.READY) {
                toast(CaptureGate.explanation(availability,
                        GameBridgeService.enabledInSettings(this)));
                refresh();
                return;
            }
            if (!GameBridgeService.captureAfterReturnToGame()) {
                toast(CaptureGate.explanation(GameBridgeService.captureReadiness(this),
                        GameBridgeService.enabledInSettings(this)));
                refresh();
                return;
            }
            if (!openGame()) {
                GameBridgeService.cancelPending();
                toast("Clash of Clans Google Play introuvable.");
            }
        });
        action("Capture alternative — partage d'écran Android", false,
                v -> requestAlternativeCapture());
        action("Actualiser la capture", false, v -> refresh());
        captureNotice = text("État de la capture : en attente.", 13, MUTED);
        LinearLayout.LayoutParams noticeParams = new LinearLayout.LayoutParams(-1, -2);
        noticeParams.topMargin = dp(10);
        screen.addView(captureNotice, noticeParams);
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

        section("04  /  Calibration du parcours et des erreurs",
                "Sur MuMu, enregistre les cinq écrans du parcours et, lorsque tu les rencontres, les six états bloquants : clan fermé, invitation, adhésion en attente, délai actif, château plein et connexion perdue. Un écran inconnu arrête le cycle.");
        action("Mémoriser la capture comme état connu", false, v -> chooseScreenState());
        action("Choisir une zone stable pour identifier l'écran", false, v -> chooseStableRegion());
        action("Identifier l'écran capturé", false, v -> identifyScreen());
        referencesStatus = text("Références enregistrées : vérification en cours.", 13, TEXT);
        referencesStatus.setPadding(dp(13), dp(12), dp(13), dp(12));
        referencesStatus.setBackground(background(SURFACE, 0));
        screen.addView(referencesStatus);
        action("Sauvegarder mes images et boutons (.zip)", false, v -> exportCalibration());
        action("Restaurer mes images et boutons (.zip)", false, v -> importCalibration());
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
            GuidedRoute route = route();
            route.block();
            prefs().edit().putString("journal", "BLOCKED")
                    .putInt("route_index", route.index())
                    .putString("route_phase", route.phase().name()).commit();
            refresh(); toast("Actions en attente annulées ; parcours bloqué.");
        });
        section("06  /  Parcours guidé : clan → renforts",
                "Quatre transitions encadrées : village → fiche du clan → chat du clan → fenêtre de demande → demande visible. Chaque geste exige un écran de départ reconnu et chaque passage à l'étape suivante exige une capture d'arrivée et ta confirmation. Aucune action n'est répétée automatiquement.");
        routeStatus = text("Parcours en attente de vérification.", 14, TEXT);
        routeStatus.setPadding(dp(13), dp(13), dp(13), dp(13));
        routeStatus.setBackground(background(SURFACE, 0));
        screen.addView(routeStatus);
        action("Choisir une référence et son bouton (1 à 4)", false,
                v -> pickCalibrationPreview());
        action("Enregistrer le bouton sélectionné", false, v -> saveGuidedTarget());
        action("Revenir à la dernière capture", false, v -> {
            calibrationPreviewStep = -1;
            selectedX = -1;
            selectedY = -1;
            refresh();
        });
        action("Exécuter UN geste avec le bouton calibré", true, v -> prepareGuidedStep());
        action("Vérifier et valider l'étape par la capture d'arrivée", false, v -> approveGuidedStep());
        action("Bloquer le parcours et annuler toute action en attente", false, v -> stopGuidedRoute());
        action("Réinitialiser le parcours après examen", false, v -> restartGuidedRoute());

        section("07  /  Gestion des profils logiques",
                "Une seule session Android active. Les profils ne contiennent que des identifiants locaux ; aucune connexion, sauvegarde privée ou session Supercell n'est copiée.");
        profileIdInput = new EditText(this);
        profileIdInput.setSingleLine(true);
        profileIdInput.setTextColor(TEXT);
        profileIdInput.setHintTextColor(MUTED);
        profileIdInput.setHint("Ex. FARM_001");
        profileIdInput.setBackground(background(SURFACE, Color.rgb(63,67,72)));
        profileIdInput.setPadding(dp(14), dp(12), dp(14), dp(12));
        screen.addView(profileIdInput, new LinearLayout.LayoutParams(-1, dp(55)));
        action("Ajouter le profil logique", false, v -> addLogicalProfile());
        action("Préparer le prochain profil", false, v -> acquireNextProfile());
        action("Confirmer manuellement la connexion de ce profil", false, v -> approveProfileLogin());
        action("Terminer / bloquer le profil actif", false, v -> finishProfile());
        fleetStatus = text("Aucun profil chargé.", 13, MUTED);
        fleetStatus.setPadding(dp(12), dp(12), dp(12), dp(12));
        fleetStatus.setBackground(background(SURFACE, 0));
        screen.addView(fleetStatus);

        section("08  /  Exécution automatique expérimentale",
                "Un seul cycle autorisé par l'utilisateur, jusqu'à quatre gestes calibrés ou trois via la fiche du clan. Le moteur exige deux captures concordantes et s'arrête sur un état inattendu. Succès visuel ≠ succès confirmé par Supercell.");
        autoStatus = text("Moteur automatique à l'arrêt.", 13, MUTED);
        screen.addView(autoStatus);
        action("Lancer UN cycle automatique calibré", true, v -> startOneAutomaticCycle(false));
        action("Cycle rapide : ouvrir directement la fiche du clan", false,
                v -> startOneAutomaticCycle(true));
        action("ARRÊTER immédiatement le cycle automatique", false, v -> {
            GameBridgeService.stopAutomatic();
            refresh();
        });

        section("09  /  Diagnostic local", "Historique limité des captures, gestes et changements d'étape. Le rapport ne contient ni captures, ni tags de joueurs, ni données de connexion.");
        action("Voir le rapport de diagnostic", false, v -> showDiagnostics());

        section("État du pilote", "Cycle automatique expérimental disponible, mais ni l'adhésion au clan, ni l'envoi de renforts côté serveur, ni la rotation des sessions Supercell ne sont validés. Un test réel MuMu reste indispensable. L'automatisation peut contrevenir aux règles de Supercell.");
        refresh();
    }

    private ProfileQueue.Item findCurrentProfile(ProfileQueue queue) {
        for (ProfileQueue.Item item : queue.items()) {
            if (item.status == ProfileQueue.Status.ACTIVE
                    || item.status == ProfileQueue.Status.NEEDS_OPERATOR_LOGIN) return item;
        }
        return null;
    }

    private void addLogicalProfile() {
        try {
            ProfileQueue q = ProfileStore.load(this);
            q.add(profileIdInput.getText().toString());
            ProfileStore.persist(this);
            refresh();
        } catch (RuntimeException error) { toast(error.getMessage()); }
    }

    private void acquireNextProfile() {
        try {
            ProfileQueue q = ProfileStore.load(this);
            ProfileQueue.Item item = q.acquire(System.currentTimeMillis());
            ProfileStore.persist(this);
            if (item == null) {
                toast("Aucun profil libre : termine d'abord la session en cours."); return;
            }
            toast("Profil réservé : " + item.id
                    + ". Connecte-le dans le jeu par la méthode officielle.");
            refresh();
        } catch (RuntimeException error) { toast(error.getMessage()); }
    }

    private void approveProfileLogin() {
        try {
            ProfileQueue q = ProfileStore.load(this);
            ProfileQueue.Item pending = findCurrentProfile(q);
            if (pending == null || pending.status != ProfileQueue.Status.NEEDS_OPERATOR_LOGIN) {
                toast("Aucun profil en attente de connexion."); return;
            }
            new AlertDialog.Builder(this).setTitle("Confirmer le profil actif")
                    .setMessage("Vérifie dans Clash of Clans que le compte AUTORISÉ "
                            + pending.id + " est bien connecté. Cette application ne peut pas "
                            + "vérifier une connexion uniquement avec le nom logique.")
                    .setNegativeButton("Annuler", null)
                    .setPositiveButton("Connexion vérifiée", (d, w) -> {
                        try {
                            q.operatorConfirmedLogin(pending.id, System.currentTimeMillis());
                            ProfileStore.persist(this);
                            refresh();
                        } catch (RuntimeException e) { toast(e.getMessage()); }
                    }).show();
        } catch (RuntimeException error) { toast(error.getMessage()); }
    }

    private void finishProfile() {
        try {
            ProfileQueue q = ProfileStore.load(this);
            ProfileQueue.Item current = findCurrentProfile(q);
            if (current == null) { toast("Aucun profil actif ou réservé."); return; }
            new AlertDialog.Builder(this).setTitle("Fin de la session")
                    .setMessage("Le résultat 'visuel' n'est pas une preuve serveur. "
                            + "Marquer ce profil comme terminé visuellement si le cycle "
                            + "affiché a été vérifié, sinon le bloquer.")
                    .setNegativeButton("Bloquer", (d, w) -> {
                        q.block(current.id);
                        ProfileStore.persist(this);
                        refresh();
                    })
                    .setPositiveButton("Terminé visuellement", (d, w) -> {
                        try {
                            if (!q.holdsActive(current.id, System.currentTimeMillis())
                                    || !current.id.equals(prefs().getString("auto_profile_id", null))
                                    || !prefs().getString("auto_phase", "").equals("COMPLETE_VISUAL"))
                                throw new IllegalStateException("Aucun cycle automatique terminé");
                            q.finish(current.id, true);
                            ProfileStore.persist(this);
                            prefs().edit().remove("auto_profile_id").commit();
                            refresh();
                        } catch (RuntimeException error) { toast(error.getMessage()); }
                    }).show();
        } catch (RuntimeException error) { toast(error.getMessage()); }
    }

    private void startOneAutomaticCycle(boolean directClanLink) {
        try {
            ProfileQueue q = ProfileStore.load(this);
            ProfileQueue.Item current = findCurrentProfile(q);
            if (current == null || current.status != ProfileQueue.Status.ACTIVE) {
                toast("Prépare et confirme d'abord un profil actif."); return;
            }
            if (route().index() != 0 || route().phase() != GuidedRoute.Phase.READY
                    || new RequestJournal(prefs().getString("journal", "BLOCKED")).phase()
                        != RequestJournal.Phase.READY) {
                toast("Réinitialise d'abord le parcours après examen du jeu."); return;
            }
            if (directClanLink) {
                try { ClanTag.normalize(clan.getText().toString()); }
                catch (RuntimeException error) {
                    toast("Tag de clan invalide : raccourci indisponible."); return;
                }
            }
            new AlertDialog.Builder(this).setTitle("Cycle automatique expérimental")
                    .setMessage("Profil logique : " + current.id
                            + (directClanLink ? "\\nFiche du clan par lien, puis 3 gestes au maximum. "
                                : "\\nQuatre gestes au maximum. ")
                            + "Les écrans et boutons nécessaires doivent être calibrés. "
                            + "Arrêt à la première différence ou erreur. "
                            + "Ne confirme jamais la réussite côté serveur. "
                            + "L'utilisation peut contrevenir aux règles de Supercell. "
                            + "Autoriser ce cycle unique ?")
                    .setNegativeButton("Annuler", null)
                    .setPositiveButton("Autoriser le cycle", (d, w) -> {
                        if (!ProfileStore.load(this).holdsActive(current.id,
                                System.currentTimeMillis())) {
                            toast("Bail de profil expiré : reconnecte et vérifie le compte.");
                            return;
                        }
                        if (!prefs().edit().putString("auto_profile_id", current.id).commit()) {
                            toast("Impossible de verrouiller le profil actif.");
                            return;
                        }
                        // A separate system permission is required for EVERY auto
                        // session. The accessibility API returns black on MuMu.
                        try {
                            if (!GameBridgeService.connected())
                                throw new IllegalStateException("Accessibilité non connectée");
                            pendingAutoStartIndex = directClanLink ? 1 : 0;
                            MediaProjectionManager manager = (MediaProjectionManager)
                                    getSystemService(MEDIA_PROJECTION_SERVICE);
                            startActivityForResult(manager.createScreenCaptureIntent(),
                                    AUTO_PROJECTION_REQUEST);
                        } catch (RuntimeException error) {
                            pendingAutoStartIndex = -1;
                            prefs().edit().remove("auto_profile_id").commit();
                            toast("Autorisation de capture impossible : " + error.getMessage());
                        }
                    }).show();
        } catch (RuntimeException error) { toast(error.getMessage()); }
    }

    private void requestAlternativeCapture() {
        CaptureGate.Reason readiness = GameBridgeService.captureReadiness(this);
        if (readiness != CaptureGate.Reason.READY) {
            toast(CaptureGate.explanation(readiness,
                    GameBridgeService.enabledInSettings(this)));
            return;
        }
        new AlertDialog.Builder(this)
                .setTitle("Capture alternative — autorisation Android")
                .setMessage("Cette méthode utilise MediaProjection au lieu de la capture "
                        + "par accessibilité. Android demandera de partager l'écran. "
                        + "Choisis « Écran entier » s'il propose plusieurs modes. "
                        + "Le service prend une seule image après le retour dans Clash of Clans, "
                        + "puis s'arrête. Aucune vidéo ni image n'est envoyée à un serveur.")
                .setNegativeButton("Annuler", null)
                .setPositiveButton("Autoriser via Android", (dialog, which) -> {
                    try {
                        MediaProjectionManager projection =
                                (MediaProjectionManager)
                                        getSystemService(MEDIA_PROJECTION_SERVICE);
                        startActivityForResult(projection.createScreenCaptureIntent(),
                                PROJECTION_REQUEST);
                    } catch (RuntimeException error) {
                        toast("Capture alternative indisponible : "
                                + error.getClass().getSimpleName());
                    }
                }).show();
    }

    @Override protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == CALIBRATION_EXPORT_REQUEST
                || requestCode == CALIBRATION_IMPORT_REQUEST) {
            onArchiveResult(requestCode, resultCode, data);
            return;
        }
        if (requestCode != PROJECTION_REQUEST
                && requestCode != AUTO_PROJECTION_REQUEST) return;
        final boolean automatic = requestCode == AUTO_PROJECTION_REQUEST;
        int startIndex = automatic ? pendingAutoStartIndex : -1;
        pendingAutoStartIndex = -1;
        if (resultCode != RESULT_OK || data == null) {
            if (automatic) prefs().edit().remove("auto_profile_id").commit();
            toast("Partage d'écran refusé : aucune capture effectuée.");
            return;
        }
        if (automatic && (startIndex < 0 || startIndex > 1)) {
            prefs().edit().remove("auto_profile_id").commit();
            toast("Parcours automatique annulé : étape inconnue.");
            return;
        }
        Intent capture = new Intent(this, AlternativeCaptureService.class)
                .setAction(automatic
                        ? AlternativeCaptureService.ACTION_AUTO
                        : AlternativeCaptureService.ACTION_CAPTURE)
                .putExtra(AlternativeCaptureService.EXTRA_AUTO_START_INDEX, startIndex)
                .putExtra(AlternativeCaptureService.EXTRA_RESULT_CODE, resultCode)
                .putExtra(AlternativeCaptureService.EXTRA_RESULT_DATA, data);
        try {
            if (Build.VERSION.SDK_INT >= 26) startForegroundService(capture);
            else startService(capture);
            boolean opened = automatic && startIndex == 1
                    ? launchClanLink() : openGame();
            if (!opened) {
                stopService(new Intent(this, AlternativeCaptureService.class));
                if (automatic) prefs().edit().remove("auto_profile_id").commit();
                toast("Clash of Clans introuvable : capture annulée.");
            }
        } catch (RuntimeException error) {
            stopService(new Intent(this, AlternativeCaptureService.class));
            if (automatic) prefs().edit().remove("auto_profile_id").commit();
            toast("Partage d'écran impossible : " + error.getClass().getSimpleName());
        }
    }

    /** Optional backup via Android's file picker. No network and no account sessions. */
    private void exportCalibration() {
        if (CalibrationArchive.flowReferenceCount(this) == 0) {
            toast("Aucune référence enregistrée. Il n'y a rien à sauvegarder.");
            return;
        }
        new AlertDialog.Builder(this).setTitle("Sauvegarder tes calibrations")
                .setMessage("Crée un ZIP avec tes références et tes boutons calibrés. "
                        + "Les captures peuvent montrer des noms de joueurs ou de clans. "
                        + "Le ZIP n'est PAS chiffré : conserve-le dans un dossier privé. "
                        + "Les comptes et identifiants de connexion ne sont pas exportés.")
                .setNegativeButton("Annuler", null)
                .setPositiveButton("Choisir où sauvegarder", (dialog, which) -> {
                    Intent create = new Intent(Intent.ACTION_CREATE_DOCUMENT)
                            .addCategory(Intent.CATEGORY_OPENABLE)
                            .setType("application/zip")
                            .putExtra(Intent.EXTRA_TITLE, "REQNGO_calibrations.zip");
                    try { startActivityForResult(create, CALIBRATION_EXPORT_REQUEST); }
                    catch (RuntimeException error) {
                        toast("Sélecteur de fichiers indisponible : "
                                + error.getClass().getSimpleName());
                    }
                }).show();
    }

    private void importCalibration() {
        new AlertDialog.Builder(this).setTitle("Restaurer tes références")
                .setMessage("Sélectionne une sauvegarde REQ N GO créée précédemment. "
                        + "Les images et boutons sauvegardés remplaceront ceux de cette installation. "
                        + "Le parcours restera BLOQUÉ jusqu'à vérification manuelle des écrans. "
                        + "Ce bouton ne peut pas récupérer des données d'une application "
                        + "déjà désinstallée sans sauvegarde.")
                .setNegativeButton("Annuler", null)
                .setPositiveButton("Sélectionner le ZIP", (dialog, which) -> {
                    Intent open = new Intent(Intent.ACTION_OPEN_DOCUMENT)
                            .addCategory(Intent.CATEGORY_OPENABLE)
                            .setType("application/zip");
                    try { startActivityForResult(open, CALIBRATION_IMPORT_REQUEST); }
                    catch (RuntimeException error) {
                        toast("Sélecteur de fichiers indisponible : "
                                + error.getClass().getSimpleName());
                    }
                }).show();
    }

    private void onArchiveResult(int requestCode, int resultCode, Intent data) {
        if (resultCode != RESULT_OK || data == null || data.getData() == null) {
            toast("Opération de sauvegarde annulée.");
            return;
        }
        try {
            if (requestCode == CALIBRATION_EXPORT_REQUEST) {
                try (OutputStream out = getContentResolver().openOutputStream(data.getData())) {
                    if (out == null) throw new IllegalStateException("Fichier non accessible");
                    CalibrationArchive.exportTo(this, out);
                }
                toast("Calibrations sauvegardées. Conserve le ZIP avant de changer d'APK.");
            } else {
                int imported;
                try (InputStream in = getContentResolver().openInputStream(data.getData())) {
                    if (in == null) throw new IllegalStateException("Archive illisible");
                    imported = CalibrationArchive.importFrom(this, in);
                }
                calibrationPreviewStep = -1;
                shownReferenceTime = -1;
                selectedX = -1;
                selectedY = -1;
                refresh();
                toast(imported + " références restaurées. "
                        + "Réactive l'accessibilité puis examine le jeu avant tout geste.");
            }
        } catch (Exception error) {
            toast("Sauvegarde/restauration refusée : " + error.getMessage());
        }
    }

    private void showDiagnostics() {
        final String report = DiagnosticStore.report(this);
        ScrollView view = new ScrollView(this);
        TextView content = text(report, 12, TEXT);
        content.setTextIsSelectable(true);
        content.setPadding(dp(15), dp(14), dp(15), dp(14));
        view.addView(content);
        new AlertDialog.Builder(this)
                .setTitle("Diagnostics REQ N GO (sans données de compte)")
                .setView(view)
                .setNegativeButton("Fermer", null)
                .setPositiveButton("Copier le rapport", (dialog, which) -> {
                    ClipboardManager clip = (ClipboardManager)
                            getSystemService(CLIPBOARD_SERVICE);
                    if (clip != null) {
                        clip.setPrimaryClip(ClipData.newPlainText("REQ N GO diagnostic", report));
                        toast("Rapport technique copié ; aucun identifiant ni image inclus.");
                    }
                }).show();
    }

    private GuidedRoute route() {
        return new GuidedRoute(prefs().getInt("route_index", 0),
                prefs().getString("route_phase", "READY"));
    }

    private boolean saveRoute(GuidedRoute route) {
        return prefs().edit().putInt("route_index", route.index())
                .putString("route_phase", route.phase().name()).commit();
    }

    private void pickCalibrationPreview() {
        new AlertDialog.Builder(this)
                .setTitle("Sélectionner le bouton")
                .setItems(GuidedRoute.TITLE, (dialog, which) -> {
                    File source = ScreenCatalog.referenceFile(this, GuidedRoute.FROM[which]);
                    Bitmap preview = BitmapFactory.decodeFile(source.getAbsolutePath());
                    if (preview == null) {
                        toast("Référence absente : " + GuidedRoute.FROM[which]);
                        return;
                    }
                    calibrationPreviewStep = which;
                    selectedX = -1;
                    selectedY = -1;
                    selectedReferenceTime = -1;
                    screenshot.setImageBitmap(preview);
                    pointLabel.setText("Étape " + (which + 1) + " : "
                            + GuidedRoute.TITLE[which]
                            + ". Choisis le centre du bouton sur l'image en section 03.");
                    toast("Image affichée en section 03 : choisis le bouton, puis enregistre.");
                }).show();
    }

    private void saveGuidedTarget() {
        if (selectedX <= 0 || selectedY <= 0) {
            toast("Choisis d'abord le centre du bouton sur la capture."); return;
        }
        try {
            RouteTarget point;
            if (calibrationPreviewStep >= 0) {
                point = RouteCalibrationStore.saveFromReference(
                        this, calibrationPreviewStep, selectedX, selectedY);
            } else {
                GuidedRoute current = route();
                point = RouteCalibrationStore.store(
                        this, current, selectedX, selectedY, selectedReferenceTime);
            }
            DiagnosticStore.record(this, DiagnosticTrail.Event.BUTTON_CALIBRATED);
            toast("Bouton enregistré pour l'étape " + (point.step + 1)
                    + " (" + Math.round(point.x * 100) + " %, "
                    + Math.round(point.y * 100) + " %).");
            refresh();
        } catch (Exception e) {
            toast("Calibration refusée : " + e.getMessage());
        }
    }

    private void prepareGuidedStep() {
        GuidedRoute route = route();
        if (route.phase() != GuidedRoute.Phase.READY) {
            toast("Parcours non prêt : vérifie ou réinitialise l'étape en cours."); return;
        }
        final RouteTarget target;
        try {
            target = RouteCalibrationStore.loadForCurrentScreen(this, route);
        } catch (Exception error) {
            toast("Calibre le bouton de cette étape : " + error.getMessage()); return;
        }
        if (new RequestJournal(prefs().getString("journal", "READY")).phase()
                != RequestJournal.Phase.READY) {
            toast("Journal de geste non prêt : examine l'action précédente."); return;
        }
        final ScreenCatalog.Result identified;
        final long referenceTime = prefs().getLong("capture_time", -1L);
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
                            || referenceTime != prefs().getLong("capture_time", -1L)) {
                        toast("Contexte modifié : capture à recommencer."); return;
                    }
                    try {
                        RouteTarget currentTarget = RouteCalibrationStore.loadForCurrentScreen(
                                this, live);
                        if (!currentTarget.encode().equals(target.encode())) {
                            toast("Bouton recalibré pendant la confirmation : action refusée.");
                            return;
                        }
                        // Durable marker is stored BEFORE possible dispatch.
                        live.start(identified.id, true);
                        if (!prefs().edit().putInt("route_index", live.index())
                                .putString("route_phase", live.phase().name())
                                .putLong("route_started_ms", System.currentTimeMillis())
                                .commit()) throw new IllegalStateException("Journal inaccessible");
                        DiagnosticStore.record(this, DiagnosticTrail.Event.ROUTE_STEP_STARTED);
                        if (!GameBridgeService.armSingleTap(target.x, target.y)) {
                            live.block(); saveRoute(live);
                            DiagnosticStore.record(this, DiagnosticTrail.Event.ROUTE_BLOCKED);
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
                    } catch (Exception error) {
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
                                .putString("journal", "READY")
                                .remove("route_started_ms").commit()) {
                            toast("Enregistrement impossible : ne pas continuer."); return;
                        }
                        DiagnosticStore.record(this, DiagnosticTrail.Event.ROUTE_STEP_APPROVED);
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
                    DiagnosticStore.record(this, DiagnosticTrail.Event.ROUTE_BLOCKED);
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
                && j.phase() != RequestJournal.Phase.CONFIRMED
                && !(progress.phase() == GuidedRoute.Phase.COMPLETE
                     && j.phase() == RequestJournal.Phase.READY)) {
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
                        DiagnosticStore.record(this, DiagnosticTrail.Event.ROUTE_RESET);
                        GameBridgeService.resetAutomaticAfterReview();
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
                                    DiagnosticStore.record(this, DiagnosticTrail.Event.REFERENCE_RECORDED);
                                    detectedScreen.setText("État enregistré localement : "
                                            + ScreenCatalog.LABELS[which]);
                                } catch (Exception e) {
                                    toast("Enregistrement impossible : " + e.getClass().getSimpleName());
                                }
                            }).show();
                }).show();
    }

    private void chooseStableRegion() {
        long capture = prefs().getLong("capture_time", -1L);
        long now = System.currentTimeMillis();
        RequestJournal.Phase journal = new RequestJournal(
                prefs().getString("journal", "BLOCKED")).phase();
        if (capture <= 0 || capture > now || now - capture > 120000
                || selectedReferenceTime != capture || selectedX <= .10f || selectedX >= .90f
                || selectedY <= .10f || selectedY >= .90f
                || journal == RequestJournal.Phase.ARMED
                || journal == RequestJournal.Phase.DISPATCHED_UNVERIFIED) {
            toast("Choisis un point stable loin des bords, sur une capture récente et résolue.");
            return;
        }
        final float x = selectedX, y = selectedY;
        new AlertDialog.Builder(this).setTitle("Zone visuelle stable")
                .setItems(ScreenCatalog.LABELS, (dialog, which) -> {
                    new AlertDialog.Builder(this)
                            .setTitle("Identifier « " + ScreenCatalog.LABELS[which] + " »")
                            .setMessage("Utiliser une zone autour du point sélectionné pour reconnaître "
                                    + "cet écran ? Choisis une icône ou un titre stable, "
                                    + "pas une animation. Aucune action n'en découlera automatiquement.")
                            .setNegativeButton("Annuler", null)
                            .setPositiveButton("Enregistrer", (confirm, unused) -> {
                                try {
                                    if (capture != prefs().getLong("capture_time", -1L))
                                        throw new IllegalStateException("Capture périmée");
                                    ScreenCatalog.setAnchor(this, ScreenCatalog.IDS[which], x, y);
                                    detectedScreen.setText("Zone stable enregistrée pour : "
                                            + ScreenCatalog.LABELS[which]);
                                } catch (Exception e) {
                                    toast("Zone refusée : " + e.getMessage());
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
                    + "\nMéthode : " + result.method
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

    private boolean launchClanLink() {
        try {
            Uri uri = Uri.parse(ClanTag.profileUrl(clan.getText().toString()));
            startActivity(new Intent(Intent.ACTION_VIEW, uri)
                    .addCategory(Intent.CATEGORY_BROWSABLE));
            return true;
        } catch (Exception e) { return false; }
    }

    private void openClan() {
        if (!saveTag()) return;
        if (!launchClanLink())
            toast("Impossible d'ouvrir le lien. Utilise la recherche par tag dans le jeu.");
    }

    private boolean openGame() {
        Intent launch = getPackageManager().getLaunchIntentForPackage(GameBridgeService.GAME);
        if (launch == null) return false;
        try { startActivity(launch); return true; }
        catch (RuntimeException e) { return false; }
    }

    private void confirmTap() {
        if (calibrationPreviewStep >= 0) {
            toast("Reviens à la dernière capture avant d'essayer un geste libre.");
            return;
        }
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
        if (referencesStatus != null) {
            int count = CalibrationArchive.flowReferenceCount(this);
            int total = CalibrationArchive.referenceCount(this);
            referencesStatus.setText("Références du parcours : " + count
                    + "/5 · total (avec états d'erreur) : " + total
                    + "\nSi tu viens d'installer une nouvelle APK et vois 0/5, "
                    + "la précédente calibration n'est pas présente dans ce stockage. "
                    + "Restaure un ZIP sauvegardé ou recrée les captures.");
        }
        if (fleetStatus != null) {
            try { fleetStatus.setText(ProfileStore.summary(this)); }
            catch (RuntimeException error) { fleetStatus.setText("Profils bloqués : " + error.getMessage()); }
        }
        if (autoStatus != null) autoStatus.setText(
                "Cycle auto : " + prefs().getString("auto_phase", "IDLE")
                + " | Étape " + prefs().getInt("auto_index", 0) + "/" + GuidedRoute.SIZE
                + "\\nUn résultat visuel n'est pas une preuve d'envoi serveur.");
        if (routeStatus != null) {
            GuidedRoute current = route();
            String states = current.index() < GuidedRoute.SIZE
                    ? "\nÉcran requis : " + current.expectedBefore()
                        + " → " + current.expectedAfter()
                    : "\\nAucune autre étape";
            String calibrated = current.index() < GuidedRoute.SIZE
                    ? "\nBouton : " + (RouteCalibrationStore.stored(this, current.index())
                        ? "enregistré (à vérifier pour l'écran actuel)"
                        : "à calibrer pour cette étape")
                    : "";
            routeStatus.setText(current.summary() + states + calibrated);
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
        boolean enabled = GameBridgeService.enabledInSettings(this);
        String availability = GameBridgeService.connected()
                ? "Service connecté à REQ N GO"
                : (enabled ? "Service autorisé, mais non connecté"
                           : "Service non activé dans Android");
        String journal = prefs().getString("journal", "READY");
        String message = prefs().getString("status", "En attente d'une première capture.");
        status.setText(availability + "\nJournal : " + journal + "\n" + message);
        if (captureNotice != null) {
            CaptureGate.Reason readiness = GameBridgeService.captureReadiness(this);
            captureNotice.setText("Capture : "
                    + CaptureGate.explanation(readiness, enabled)
                    + "\nDernier diagnostic : " + message);
        }
        // The current screenshot can be absent while previously saved
        // references still exist. The preview must work in that situation.
        if (calibrationPreviewStep >= 0) {
            File reference = ScreenCatalog.referenceFile(this,
                    GuidedRoute.FROM[calibrationPreviewStep]);
            Bitmap preview = BitmapFactory.decodeFile(reference.getAbsolutePath());
            if (preview != null) {
                screenshot.setImageBitmap(preview);
                return;
            }
            calibrationPreviewStep = -1;
        }
        File file = new File(getFilesDir(), "latest_game_capture.png");
        Bitmap bitmap = file.isFile()
                ? BitmapFactory.decodeFile(file.getAbsolutePath()) : null;
        screenshot.setImageBitmap(bitmap);
    }

    @Override protected void onResume() {
        super.onResume();
        if (status != null && screenshot != null) refresh();
    }
}
