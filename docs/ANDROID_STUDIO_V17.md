# REQ N GO — essai Android réel sur Windows (V17)

**Objectif : obtenir une première interaction vérifiée sur UN appareil Android réel.** La V17 ne développe pas de nouvelles métriques fictives et ne dépend pas du cloud. Elle permet de capturer l'écran du jeu par ADB, de tracer des repères et un point tactile dans un navigateur local, puis de tester un seul clic vérifié visuellement, sans coordonnées inventées.

Ce que nous avons codé est un **outil capable d'envoyer une vraie commande Android**. Nous n'avons **pas encore connecté d'appareil/emulateur ni vérifié une demande de troupes Clash of Clans**. Il faut effectuer cette validation localement ; les tests CI ne disposent pas de téléphone.

## Préparation

Sur ton PC Windows 11 :

1. Installer Python 3.12+ si absent.
2. Pour **BlueStacks 5** : démarrer une instance, ouvrir **Paramètres → Avancé → Android Debug Bridge**, activer ADB et **Enregistrer les modifications**. Avec l'installation BlueStacks standard, le lanceur détecte directement `C:\\Program Files\\BlueStacks_nxt\\HD-Adb.exe` et tente la connexion locale `127.0.0.1:5555`. Aucun Android Studio complet n'est nécessaire. Ce port peut différer selon l'instance. Si BlueStacks n'est pas installé au chemin standard, définir `ADB_BINARY` sur le chemin de son `HD-Adb.exe`.
3. Sinon, installer **Android SDK Platform Tools** officiels (contenant `adb.exe`) et les ajouter au PATH Windows : <https://developer.android.com/tools/releases/platform-tools>.
4. Préparer un **appareil Android** ou un **émulateur Android compatible ADB**, activer le débogage USB pour un téléphone, ou démarrer l'émulateur.
5. Ouvrir Clash of Clans **manuellement** sur l'appareil, le laisser au premier plan.
6. Dans le dossier racine du dépôt, double-cliquer **`DEMARRER-ANDROID-STUDIO.bat`**. Le lanceur vérifie Python, ADB et les bibliothèques visuelles, affiche les appareils, te demande **le numéro de série ADB** et si tu souhaites activer les gestes pour cet essai.

Le navigateur s'ouvre automatiquement sur **<http://127.0.0.1:8766>**. Seul ton PC peut accéder à cet outil ; il est distinct du tableau de bord Guardian (`/fleet`).

Si tu préfères PowerShell :

```powershell
python -m pip install -r requirements-android.txt
python -m reqngo.android.cli devices
$env:REQNGO_ANDROID_ACTIONS="I_ACCEPT_LOCAL_TOUCHES"
python -m reqngo.android.studio --serial "emulator-5554"
```

Remplace `emulator-5554` par le serial réellement renvoyé par ADB. Ne partage pas de capture montrant des identifiants de compte.

## Premier parcours (1 appareil, 1 clic)

1. Ouvre le jeu manuellement sur l'appareil. Clique **Capturer l'écran** dans le studio.
2. Avec le mode **Dessiner un repère**, encadre à la souris un élément graphique distinctif et stable, donne-lui le nom `village`, puis clique **Enregistrer ce repère**. Le studio découpe un PNG local et le calibre.
3. Sur le téléphone / émulateur, ouvre **manuellement** une autre fenêtre identifiable. Clique **Capturer l'écran** et enregistre un deuxième repère, par exemple `clan_window`. La fenêtre d'arrivée doit être reconnaissable, différente de l'écran de départ.
4. Sur le téléphone, retourne **manuellement** à l'état de départ, puis capture à nouveau l'écran. Active **Placer un clic** et clique sur le vrai bouton visible qui permet d'ouvrir la deuxième fenêtre.
5. Dans **Construire une transition**, choisis un nom d'étape comme `open_clan`, l'état de départ `village` et l'état d'arrivée `clan_window`. Clique **Enregistrer cette action**.
6. Clique **Vérifier sans toucher** : le studio confirme le repère de départ à partir de deux captures successives sans envoyer de clic.
7. Si le test est concluant, et si tu as volontairement autorisé les gestes dans le lanceur, clique **Exécuter 1 geste réel**, accepte l'avertissement puis saisis `EXECUTER UNE ACTION`. Le studio revalide l'écran de départ, envoie **un** `adb shell input tap` au point calibré, puis attend **deux nouvelles captures confirmant l'état d'arrivée**.

Il n'y a **aucune boucle de farm**, de connexion à des comptes, de demande automatique de troupes ou d'enchaînement aveugle dans cette version. Le moteur existant peut stocker jusqu'à 30 étapes, mais chaque activation n'effectue qu'**une étape** et nécessite un contrôle utilisateur explicite.

## Fichiers locaux

Dans un sous-dossier isolé `data/android-studio/device-<empreinte-du-serial>/` (séparé par appareil, sans afficher le serial en clair dans le chemin), le studio enregistre `states.json`, les repères `<nom>.png`, `workflow.json` et `execution.sqlite3`. Aucun écran complet n'est persisté par le serveur, sauf si tu utilises séparément la commande explicite `capture` de l'ancien CLI. Ces fichiers peuvent contenir des détails privés ; **ne les ajoute pas à Git** et ne les envoie pas à un vendeur.

## Sécurité et incidents

- HTTP sur `127.0.0.1` uniquement, protection Origin/Host + jeton local aléatoire obligatoire pour les opérations d'écriture, sans accès distant ni route du Guardian.
- Toutes les commandes Android sont émises vers un serial explicitement choisi ; pas de shell arbitraire, pas de mot de passe ni de gestion d'identifiants.
- Les gestes sont désactivés par défaut et nécessitent la variable `REQNGO_ANDROID_ACTIONS=I_ACCEPT_LOCAL_TOUCHES`, puis une confirmation manuelle pour **chaque** clic.
- La reconnaissance compare de vrais PNG avec OpenCV et refuse les repères uniformes, les états inconnus ou ambigus. Elle attend deux captures concordantes avant **et** après un clic.
- Un journal SQLite marque l'exécution comme potentiellement effectuée **avant** le geste. Après coupure ou échec, aucun clic n'est rejoué automatiquement. Fermer puis relancer le studio permet de reprendre le diagnostic d'état, sans répéter le geste.
- Si le jeu n'est pas au premier plan, si ADB ne répond pas ou si l'état n'est pas identifié, l'outil refuse de lancer un clic.

**Attention :** Supercell interdit certaines formes de bots et logiciels d'automatisation non autorisés ; l'emploi de macros sur un compte réel peut conduire à une sanction. Ce pilote est conçu pour des tests locaux explicitement autorisés, sans contournement de mécanismes anti-bot.

## En cas de blocage

Le navigateur est un outil **local** : il faut le lancer **sur ton PC** avec un Android ou émulateur ADB opérationnel. GitHub Actions ne sait ni ouvrir l'écran de jeu sur ton PC ni vérifier son contenu réel.

Si un test bloque, lis le message de refus dans l'interface et vérifie les deux captures réelles correspondantes. Ne supprime pas le journal d'exécution pour contourner une action ambiguë ; identifie d'abord l'état réel de l'appareil.

## Vérification automatisée

`python -m unittest tests.test_android_studio -v`

Les tests synthétiques valident le découpage PNG, la persistance des repères, une commande tactile vérifiée sur faux appareil, l'absence de répétition et le refus des requêtes de navigateur non authentifiées. **Ils ne constituent pas une preuve de fonctionnement dans Clash of Clans.**
