# REQ N GO — véritable couche d'exécution Android (première étape)

Cette partie du dépôt est différente des anciens pilotes de simulation. Elle implémente de **vraies commandes Android Debug Bridge (ADB)** : découverte d'appareils, capture réelle de l'écran, détection de repères visuels locaux et gestes tactiles explicitement autorisés.

**Il n'y a pour le moment ni émulateur Android connecté, ni captures réelles de Clash of Clans, ni workflow de demande de troupes calibré et validé.** Ne pas confondre les tests de l'adaptateur avec une preuve du fonctionnement dans le jeu. Ces opérations risquent de contrevenir aux conditions du jeu.

## Architecture

- `reqngo/android/adb.py` : `adb devices`, `exec-out screencap -p`, `shell input tap/swipe/back` pour un **serial spécifié**, sans shell système ni exécution de commande arbitraire.
- `reqngo/android/vision.py` : détection réelle d'états d'écran à partir de modèles PNG fournis localement ; une détection absente ou ambiguë rend `unknown` et bloque le geste.
- `reqngo/android/runner.py` : transition `before -> after`, deux images concordantes avant puis après, journal SQLite de reprise. Aucun geste n'est automatiquement répété après un crash ambigu.
- `reqngo/android/cli.py` : interface de contrôle explicite, en lecture seule par défaut.

## Installation (lorsqu'un PC/émulateur sera disponible)

Il faudra disposer d'Android SDK Platform Tools (`adb`) et d'un appareil ou émulateur de test explicitement configuré, puis exécuter :

```bash
python -m pip install -r requirements-android.txt
python -m reqngo.android.cli devices
python -m reqngo.android.cli capture --serial emulator-5554 --output local-village.png
```

Les captures peuvent montrer des informations privées. Elles sont enregistrées **sur la machine locale**, uniquement sur commande explicite ; aucune capture n'est envoyée à GitHub ou à un modèle d'IA.

## Calibration indispensable

Avant d'effectuer le moindre geste :

1. Capturer plusieurs écrans authentiques de la **même résolution**, dans des états bien connus. Vérifier que la caméra, la langue, la mise à l'échelle et la version du jeu correspondent à la configuration.
2. Découper des repères graphiques **distinctifs** en PNG pour chacun des états, dans un dossier de travail local (exemple : `village.png`, `clan_window.png`). Ne pas inclure de noms de joueurs ni d'identifiants visibles.
3. Créer un fichier `states.json` décrivant chaque état reconnu :

```json
{"states":[
  {"name":"village","template":"village.png","threshold":0.92,"region":[0,0,1,1]},
  {"name":"clan_window","template":"clan_window.png","threshold":0.92,"region":[0,0,1,1]}
]}
```

4. Créer un fichier `workflow.json`, avec des positions **calibrées sur le véritable écran** (les nombres ci-dessous sont un exemple de format, pas les coordonnées de Clash of Clans) :

```json
{"steps":[
  {"name":"open_clan","before":"village","after":"clan_window",
   "action":"tap","points":[0.5,0.5],"timeout_seconds":12}
]}
```

5. Vérifier les états depuis l'appareil avant tout test :

```bash
python -m reqngo.android.cli inspect --serial emulator-5554 --manifest states.json
python -m reqngo.android.cli dry-run --serial emulator-5554 --manifest states.json --workflow workflow.json --journal ./data/pilot01-android.sqlite3
```

Sans modèles visuels et sans confirmation d'écran, la commande n'effectue **aucun geste**. Le processus n'envoie pas de demande de troupes et n'entre pas dans des clans sur la base de coordonnées inventées.

## Autoriser explicitement un geste en test contrôlé

Sur une machine Android de test configurée, l'opérateur peut activer `REQNGO_ANDROID_ACTIONS=I_ACCEPT_LOCAL_TOUCHES`, puis lancer :

```bash
python -m reqngo.android.cli execute --serial emulator-5554 --manifest states.json --workflow workflow.json --journal ./data/pilot01-android.sqlite3
```

La commande `execute` n'exécute **qu'une seule étape**. Si l'écran d'arrivée n'est pas reconnu, elle s'arrête et inscrit une transaction bloquée. Après crash, elle vérifie d'abord l'écran de sortie avant de conclure qu'une action a réussi ; elle **ne reproduit jamais aveuglément le geste**. Les mots de passe, sessions, connexions Supercell ID et mécanismes de contournement anti-bot ne sont pas gérés.

## Situation actuelle

**Fait :** adaptateur ADB, reconnaissance par modèles PNG, journal transactionnel, garde-fous, tests de communication et de reconnaissance synthétique.

**Pas encore fait :** calibration avec des images réelles du jeu, vérification du flux rejoindre/demander/quitter, association aux 3 vrais comptes, optimisation multi-appareils, hébergement d'émulateurs.

GitHub Actions peut vérifier la logique et les tests sans ordinateur personnel ; il ne remplace pas un émulateur réellement disponible pour une validation de gameplay.
