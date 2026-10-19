# REQ N GO — application Android native (V1 alpha)

**Une APK Android native, et non un programme Windows ou une interface web.**

### Disponible dans cette première version

- Interface locale Android en français, configuration du tag du clan.
- Raccourci vers la fiche du clan et ouverture du jeu Google Play.
- Service d'accessibilité **désactivé par défaut**, activé manuellement par l'utilisateur.
- Capture réelle de l'écran du jeu avec `AccessibilityService.takeScreenshot` sur **Android 11+**, stockée uniquement dans le stockage privé de l'APK.
- Calibration manuelle d'un point sur une capture de référence, liée à cette capture précise. Juste avant **un seul** geste tactile explicite, l'APK prend une nouvelle capture et compare 2 304 points : si l'écran a trop changé, aucun geste n'est exécuté.
- Journal local `READY → ARMED → DISPATCHED_UNVERIFIED`, sans répétition d'un geste au résultat inconnu.
- Capture **après** le geste, comparaison avec la référence et affichage des deux images. Une différence visuelle ne prouve pas que la demande a réussi : la confirmation reste manuelle.
- Catalogue de cinq états visuels locaux : village, fiche clan, chat de clan, fenêtre de demande et demande visible dans le chat. L'utilisateur peut enregistrer les images de MuMu et tester leur reconnaissance ; une ambiguïté produit « inconnu ».
- Vérification visuelle supplémentaire **autour du bouton sélectionné**, afin qu'une petite superposition ne passe pas inaperçue dans la comparaison de l'écran entier. Confirmation et réarmement manuels.
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
6. Cliquer sur la capture pour calibrer un point. Confirmer **un appui de test**. L'APK recapture le jeu avant l'action et refuse d'appuyer si le contenu diffère trop de la référence ; après l'appui, il tente une capture du résultat.
7. Revenir dans REQ N GO et comparer les captures avant/après. La comparaison statistique n'est pas une preuve de demande réussie.
8. Pour préparer les cinq étapes du futur parcours, capturer successivement les écrans réels du village, de la fiche clan, du chat, de la fenêtre de demande et du chat contenant la demande. Après chaque nouvelle capture, choisir **Mémoriser la capture comme état connu** puis sélectionner le bon nom. **Identifier l'écran capturé** donne une correspondance indicative et bloque les classifications ambiguës.
9. Après une action incertaine, **ne pas réarmer** avant d'avoir inspecté l'écran réel.

La capture ne doit pas être envoyée publiquement si elle contient un nom, un tag de joueur ou un code de connexion.

### Tests et politique d'usage

`FlowChecks.java` teste la validation des tags et le journal à l'aide de Java pur. Le workflow GitHub compile la vraie APK ; il ne remplace pas un essai sur MuMu.

L'utilisation de bots et d'automatisation de gameplay peut enfreindre les conditions de Supercell et exposer les comptes à des sanctions.
