# REQ N GO — application Android native (V1.4 alpha)

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
- Vérification visuelle supplémentaire **autour du bouton sélectionné**, afin qu'une petite superposition ne passe pas inaperçue dans la comparaison de l'écran entier.
- **Parcours guidé à quatre transitions** : village → fiche du clan → chat du clan → fenêtre de demande → demande visible. Chaque étape exige la reconnaissance du départ, un geste explicitement approuvé, une capture d'arrivée récente et une confirmation humaine. L'index et le verrou d'action sont persistants en cas de fermeture.
- **Boutons mémorisés par étape** : le point choisi est enregistré seulement si la capture de départ correspond à l'écran attendu. Chaque cible conserve son numéro d'étape, ses dimensions et l'empreinte SHA-256 de sa référence locale. Toute nouvelle capture de référence ou modification de résolution exige une recalibration. Le point n'est utilisé qu'après une confirmation explicite et une nouvelle vérification de l'écran.
- **Reconnaissance ciblée facultative** : possibilité de choisir sur une capture une zone d'interface stable pour chaque type d'écran. Le moteur compare alors cette petite région et ignore davantage d'animations ailleurs ; en cas d'ambiguïté, le classement reste « inconnu ». La reconnaissance ne déclenche toujours aucun geste automatiquement. Remplacer la capture de référence efface sa zone calibrée.
- **Diagnostics depuis l'APK** : un journal borné aux 60 derniers événements (captures, gestes, étapes, blocages) est consultable et copiable directement depuis l'interface. Ce rapport contient des codes d'événements et des horodatages, mais aucun tag de clan/joueur, image, nom de compte ou identifiant de session. Le presse-papiers Android doit rester privé une fois utilisé.

- **Reprise prudente** : pas d'avance ni de nouvelle tentative à partir d'un résultat ambigu. L'utilisateur doit bloquer puis examiner le jeu avant de réinitialiser la séquence. Pas de boucle automatique.
 Confirmation et réarmement manuels.
- Arrêt qui annule tout geste en attente.
- Aucun SDK tiers et aucune permission réseau.

### Limites : ne pas confondre avec le bot terminé

**Cette version n'envoie pas automatiquement de demande de renforts et ne rejoint pas automatiquement un clan.** Il faut d'abord collecter les captures authentiques d'avant/après et calibrer la détection fiable des états du jeu. Le raccourci de clan ouvre la fiche et **ne rejoint pas automatiquement** le clan. Aucune connexion ou rotation de compte, aucun import de session, aucun module de CA.apk. Pas de fonctionnement cloud 24 h/24 dans cette APK.

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
9. Dans « Parcours guidé », choisis un point sur la capture actuelle correspondant exactement à l'état de départ ; utilise « Mémoriser le bouton sélectionné pour cette étape ». Puis « Exécuter UN geste avec le bouton calibré ». Le moteur vérifie le hash de la référence et la résolution avant d'armer l'action. Les étapes suivantes nécessitent chacune leur propre calibration.
10. Après l'appui, actualise l'interface puis contrôle la photo « après » et l'état affiché. « Vérifier et valider l'étape » compare l'image de résultat à l'état d'arrivée calibré ; il te demande aussi de confirmer dans le jeu.
11. En cas d'écran ou résultat inattendu, bloque le parcours et examine le jeu. La réinitialisation est explicite, et **aucune demande incertaine n'est automatiquement répétée**.

La capture ne doit pas être envoyée publiquement si elle contient un nom, un tag de joueur ou un code de connexion.

### Tests et politique d'usage

`FlowChecks.java` teste la validation des tags et le journal à l'aide de Java pur. Le workflow GitHub compile la vraie APK ; il ne remplace pas un essai sur MuMu.

L'utilisation de bots et d'automatisation de gameplay peut enfreindre les conditions de Supercell et exposer les comptes à des sanctions.

### Périmètre du parcours guidé V1.2

La route guidée **n'est pas une démonstration que rejoindre le clan fonctionne en un seul clic** : les clans ouverts, fermés, sur invitation et les écrans de confirmation peuvent nécessiter des gestes intermédiaires non calibrés. Elle n'exécute qu'un geste à la fois et échoue en sécurité si l'étape attendue n'est pas observée. Le raccourci de clan continue d'ouvrir seulement la fiche. La reconnaissance des captures complètes est fragile en présence d'animations et doit être étalonnée sur MuMu.

La confirmation finale signifie uniquement qu'une **capture ressemblante a été examinée et acceptée par l'opérateur** ; elle ne prouve pas indépendamment l'envoi de la demande côté serveur, et ne valide ni l'arrivée de dons ni le changement de compte.

GitHub Actions vérifie la compilation APK et les règles Java hors jeu. Aucun test d'exécution sur MuMu n'a été effectué par ce workflow.

### Versions 1.3–1.4 — limitations connues

Les coordonnées préenregistrées ne sont **pas une preuve** d'identification du bon bouton. La validation reste conditionnée par une capture récente concordant avec l'état attendu et par un seul geste confirmé. La capture complète peut varier selon les animations du jeu et provoquer des blocages : ne pas abaisser automatiquement les seuils pour forcer le fonctionnement. Aucun test réel MuMu ou demande réussie côté serveur n'est garanti par GitHub Actions. Lorsqu'une étape est validée, le journal du geste passe à READY uniquement en même temps que le numéro de l'étape suivante, afin de pouvoir reprendre une nouvelle capture sans débloquer la précédente manuellement.

### Utilisation hors PC — préparation du diagnostic V1.4

Lorsque l'APK est installée et l'environnement de test disponible, mémoriser d'abord les cinq captures représentatives. Pour un écran très animé, toucher une zone stable **au centre** de sa capture, puis choisir « Choisir une zone stable pour identifier l'écran ». Enregistrer cette zone pour le même état. La ressemblance de cette zone **ne prouve pas l'envoi d'une demande** et ne permet aucun geste sans une confirmation de l'utilisateur.

Dans « Diagnostic local », le bouton permet de consulter les événements techniques et de copier un rapport sans données de compte. En cas de blocage, le rapport peut servir à identifier l'étape et le type d'échec. Aucun diagnostic réseau/cloud n'est activé dans la V1.4. Les événements sont conservés dans les préférences privées de l'application, sans permission Internet.

La vérification réelle de la prise de capture et des événements d'accessibilité sous MuMu reste un jalon distinct de la compilation Android dans GitHub Actions.
