# REQ N GO — application Android native (V1 alpha)

**Une APK Android native, et non un programme Windows ou une interface web.**

### Disponible dans cette première version

- Interface locale Android en français, configuration du tag du clan.
- Raccourci vers la fiche du clan et ouverture du jeu Google Play.
- Service d'accessibilité **désactivé par défaut**, activé manuellement par l'utilisateur.
- Capture réelle de l'écran du jeu avec `AccessibilityService.takeScreenshot` sur **Android 11+**, stockée uniquement dans le stockage privé de l'APK.
- Calibration manuelle d'un point sur la capture et **un seul** geste tactile explicite, uniquement lorsque Clash of Clans est confirmé au premier plan.
- Journal local `READY → ARMED → DISPATCHED_UNVERIFIED`, sans répétition d'un geste au résultat inconnu. Confirmation et réarmement manuels.
- Arrêt qui annule tout geste en attente.
- Aucun SDK tiers et aucune permission réseau.

### Limites : ne pas confondre avec le bot terminé

**Cette version n'envoie pas automatiquement de demande de renforts.** Il faut d'abord collecter les captures authentiques d'avant/après et calibrer la détection fiable des états du jeu. Le raccourci de clan ouvre la fiche et **ne rejoint pas automatiquement** le clan. Aucune connexion ou rotation de compte, aucun import de session, aucun module de CA.apk. Pas de fonctionnement cloud 24 h/24 dans cette APK.

Les gestes ne sont **jamais** présentés comme une réussite du gameplay sur la seule base de `dispatchGesture`. À ce stade, le contrôle du résultat est réalisé par l'opérateur. La disponibilité peut varier selon MuMu, la version Android et le rendu de l'interface.

### Compiler

Sur GitHub : Actions → **Android APK — REQ N GO** → exécution de la branche → artefact **reqngo-android-apk-debug**. Décompresser le ZIP pour récupérer `app-debug.apk`. L'APK debug n'est pas signé avec une clé de publication et ne doit pas être distribué comme version finale.

Ou dans Android Studio : ouvrir le dossier `android-app` en tant que projet, synchroniser Gradle, puis Build → Build APK(s). Prérequis : Android SDK API 35, Java 17, Gradle 8.13 / Android Gradle Plugin 8.11.1.

### Tester sur MuMu

1. Utiliser un appareil virtuel **Android 11 ou supérieur** pour la capture native. L'APK est installable sur Android 8+, mais la capture est désactivée sur Android 8–10.
2. Installer `app-debug.apk`, ouvrir REQ N GO, entrer un tag de clan autorisé et l'enregistrer.
3. Activer volontairement le service REQ N GO dans les paramètres Android d'accessibilité.
4. Lancer Clash of Clans et atteindre le village (ne pas capturer un écran d'authentification).
5. Dans REQ N GO, choisir « Capturer le jeu » : l'application revient au jeu, le service effectue une capture locale puis attendre de revenir dans REQ N GO pour l'actualiser.
6. Cliquer sur la capture pour calibrer un point. Confirmer **un appui de test**, puis vérifier visuellement dans le jeu ce qui a changé.
7. Après une action incertaine, **ne pas réarmer** avant d'avoir inspecté l'écran réel.

La capture ne doit pas être envoyée publiquement si elle contient un nom, un tag de joueur ou un code de connexion.

### Tests et politique d'usage

`FlowChecks.java` teste la validation des tags et le journal à l'aide de Java pur. Le workflow GitHub compile la vraie APK ; il ne remplace pas un essai sur MuMu.

L'utilisation de bots et d'automatisation de gameplay peut enfreindre les conditions de Supercell et exposer les comptes à des sanctions.
