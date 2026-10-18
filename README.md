# REQ N GO Manager — AI Guardian (pilote)

Socle de supervision avec tableau de bord, 50 profils fictifs, 3 pilotes techniques et diagnostics persistants. **Le moteur de jeu complet n'est pas encore opérationnel.** Une couche Android expérimentale sait désormais communiquer avec un appareil ADB et reconnaître des écrans calibrés, mais aucun émulateur ni compte réel n'est connecté aux tests.

## Fonctions disponibles
- 50 profils préparés, simulation de 3 pilotes par défaut (extensible à 150 dans une version future).
- Paramètres persistants : contrôle périodique (600 s), seuils de reprise, mode pilote.
- Incidents isolés par profil ; panne connue simulée → tentative limitée ; panne inconnue → escalade.
- Journaux consultables et export JSON sans identifiants ; SQLite local.
- Tableau de bord responsive sobre ; aucune statistique fictive présentée comme réelle.
- API sécurisée par un mot de passe et en-tête anti-CSRF pour les changements.

## Exécuter en local
Python 3.11+ :
```bash
python -m pip install -r requirements.txt
# Configurer GUARDIAN_PASSWORD avec un secret long et unique
python -m uvicorn reqngo.server:app --host 127.0.0.1 --port 8765
```
Ouvrir http://127.0.0.1:8765 ; identifiant `admin` et mot de passe GUARDIAN_PASSWORD.

Sur Windows, lancer `DEMARRER-SUR-WINDOWS.bat` : il génère au premier démarrage un secret local et installe les dépendances.

Tests : `python -m unittest discover -s tests -v`.

## Hébergement
Le service n'est **pas encore déployé**. Pour un hébergement 24/7, fournir un stockage persistant, une protection HTTPS/VPN, des sauvegardes et un superviseur de processus. Ne pas exposer directement le port 8765 sur Internet.

## Politique AI Guardian
- Détecter, isoler, journaliser puis réparer les incidents **reconnus et autorisés**, avec plafond de tentatives.
- Si le diagnostic est incertain : aucune commande arbitraire, escalade documentée.
- Les futures analyses IA de code devront travailler sur branches de test, avec validation et rollback ; pas d'accès aux mots de passe.
- Cette branche remplace le code applicatif dans un arbre Git propre. Le projet de farming d'origine est conservé dans `archive/clashgo-before-reqngo-20261015`.
- Les automatisations de gameplay peuvent enfreindre les conditions de Supercell et entraîner le bannissement de comptes.


## Nouveauté V2 : diagnostics à distance (pilote)

Un service de test externe peut envoyer un **heartbeat** et des incidents structurés au serveur. Les données persistent dans `/data/telemetry.sqlite3` ; un agent sans nouvelles depuis trois minutes est signalé comme déconnecté.

- Tableau de bord : ouvrir **Incidents**, puis **Diagnostics distants** (section repliable).
- Export : `GET /api/telemetry/export` nécessite la connexion administrateur.
- Émission : `POST /api/telemetry/heartbeat` et `POST /api/telemetry/report` nécessitent l'en-tête `Authorization: Bearer <GUARDIAN_AGENT_TOKEN>`.
- Démarrage : définir `GUARDIAN_AGENT_TOKEN` avec une valeur aléatoire de **32 caractères minimum** dans l'environnement du serveur **et** de l'agent.
- Exemple d'agent : `python tools/demo_diagnostic_worker.py --once --demo-error` (variables `GUARDIAN_URL`, `GUARDIAN_AGENT_TOKEN` et éventuellement `GUARDIAN_AGENT_ID`).

Sur Windows PowerShell, créer un jeton local avec `python -c "import secrets; print(secrets.token_urlsafe(36))"`, puis le stocker dans des variables d'environnement/secrets d'hébergement ; **jamais dans GitHub ni dans le code**. Pour une machine distante, utiliser HTTPS vérifié ou un VPN. L'exemple refuse HTTP hors localhost. Le jeton d'ingestion est partagé entre agents de test : ne pas l'utiliser comme système d'identité multi-tenant en production.

Exemple d'événement autorisé :

```json
{"agent_id":"pilot_01","event_key":"evt_123","code":"worker_timeout",
 "level":"warning","profile_id":3,
 "metadata":{"component":"session","retry_count":1}}
```

Les diagnostics n'acceptent pas de logs libres, mots de passe, emails, captures d'écran ou autres pièces jointes : seules quelques données techniques structurées sont autorisées, afin de minimiser les risques de fuite. Les événements sont plafonnés à 2 000 entrées. Les appels répétés avec la même clé d'événement sont idempotents.

**Limites :** ce module ne contrôle ni Clash of Clans ni des émulateurs, ne redémarre pas les machines distantes et ne donne pas automatiquement accès à ChatGPT aux données du serveur. Ce dernier point nécessite un connecteur autorisé. L'auto-réparation actuelle concerne toujours uniquement les scénarios simulés.

Tests automatisés GitHub Actions : `.github/workflows/guardian-tests.yml`.

## V3 : triage et vérification des incidents distants

La surveillance classe les événements techniques des services **pilotes** et consolide les répétitions dans un même dossier : service concerné, profil, gravité, nombre d'occurrences et action **conseillée**. Les erreurs non reconnues sont placées en revue plutôt que de déclencher des commandes hasardeuses.

- Dans **Incidents > Diagnostics distants**, les dossiers ouverts apparaissent sans ajouter d'écran principal.
- Un nouveau heartbeat marque uniquement une **perte de connexion** comme rétablie ; il ne prétend pas résoudre les autres erreurs.
- Pour une erreur technique réellement retestée avec succès, l'agent peut envoyer un événement `probe_success` avec `level="info"`, le même `profile_id` et `metadata.error_type` contenant le code précis de l'erreur.
- Un dossier n'est alors marqué `verified` que sur la base du **signal de succès de l'agent de test**. Ce n'est pas une validation indépendante par ChatGPT.
- Les diagnostics et leur classement restent déterministes : **aucun modèle d'IA externe et aucune correction automatique de code ne sont activés** à ce stade.

Exemple d'un succès de vérification remonté par un agent autorisé :

```json
{"agent_id":"pilot_01","event_key":"evt_probe_456","code":"probe_success",
 "level":"info","profile_id":3,
 "metadata":{"error_type":"worker_timeout","component":"test_worker"}}
```

## V4 : trois processus pilotes réellement exécutables

La V4 fournit **trois processus Python indépendants**, pilot_01, pilot_02 et pilot_03, démarrables dans des conteneurs Docker distincts. Ils **ne se connectent pas au jeu**. Ils contrôlent seulement leur propre environnement technique : lecture/écriture dans un dossier dédié, émission de signaux de vie et envoi d'incidents structurés au serveur Guardian.

Chaque pilote possède un volume de données et une file SQLite distincts. Si Guardian est momentanément indisponible, les événements attendent localement, puis sont réémis avec un identifiant unique ; le serveur évite les doublons. Les processus continuent indépendamment. Si le dossier de contrôle du pilote manque, il est recréé puis testé avant d'envoyer le signal de récupération. Aucun autre fichier n'est effacé et aucune commande distante n'est exécutée.

### Démarrage local via Docker Compose

1. Installer Docker et son module Compose sur l'ordinateur ou la VM que tu contrôles.
2. Définir **deux secrets distincts** via l'environnement ou un fichier `.env` **non versionné** : `GUARDIAN_PASSWORD` (16 caractères min.) et `GUARDIAN_AGENT_TOKEN` (32 caractères min.). Pour générer un secret : `python -c "import secrets; print(secrets.token_urlsafe(36))"`. Ne jamais transmettre ces valeurs dans le chat, ni les publier sur GitHub.
3. Lancer `docker compose up --build -d`.
4. Ouvrir **http://127.0.0.1:8765**, s'identifier en tant que `admin`, puis **Incidents > Diagnostics distants**. Les trois services devraient apparaître après leur premier envoi.
5. Voir l'état des processus : `docker compose ps`. Afficher leurs sorties techniques : `docker compose logs --tail=30 pilot01 pilot02 pilot03`. Terminer la démonstration : `docker compose down` (les volumes persistants sont conservés).

Le fichier Compose ne publie l'interface Guardian **que sur localhost**. Entre conteneurs d'une même machine, les pilotes utilisent le nom réseau interne `guardian`; cette exception HTTP est explicitement limitée à ce nom et au port 8765. Pour un serveur distant, ajouter **HTTPS derrière une passerelle authentifiée ou un VPN** ; ne pas exposer directement le port Guardian. Les trois conteneurs ont `restart: unless-stopped`, qui relance un processus **s'il se termine**, mais ne garantit pas la réparation de toute panne ni le redémarrage automatique d'un conteneur seulement marqué « unhealthy ».

### Vérification

La suite `python -m unittest discover -s tests -v` inclut un essai d'intégration qui démarre **trois vrais sous-processus** Python contre un serveur HTTP local de test, puis vérifie les trois signaux de vie et les événements enregistrés. Les tests unitaires vérifient la restauration du dossier de travail, l'isolation et la transmission différée après coupure.

**Limites :** les pilotes n'effectuent aucune demande de troupes ; les 50 profils restent fictifs, aucune VM cloud n'est déployée et aucune IA externe ne modifie le code automatiquement.

## V5 : préparation au cloud sans PC personnel

Cette version reste un **outil de supervision de trois pilotes techniques**. Elle ne contient pas de bot Clash of Clans ni de connexion aux comptes.

### Fiabilité supplémentaire

- `GET /health/live` : vérifie que l'API répond, sans informations sensibles.
- `GET /health/ready` : vérifie que les deux bases SQLite du serveur sont accessibles ; renvoie HTTP 503 sinon.
- Chaque pilote met à jour un signal de vie local toutes les 30 secondes, **même si le serveur central est momentanément indisponible**.
- Docker Compose vérifie séparément Guardian et chaque pilote. Un conteneur signalé `unhealthy` ne redémarre **pas automatiquement** avec Docker Compose ; `restart: unless-stopped` redémarre seulement les processus qui se terminent.
- `tools/compose_smoke.py` vérifie en CI que les **trois vrais processus Docker** démarrent, transmettent leur état et rapportent leurs réparations **simulées**.
- Après fusion dans `main`, le workflow `.github/workflows/guardian-image.yml` peut fabriquer et publier une image OCI versionnée dans GitHub Container Registry, sans ordinateur local.

### Hébergement sur un serveur cloud

Le fichier `compose.cloud.yaml` emploie l'image construite sur GitHub au lieu de compiler sur la machine cible. Un hébergeur **compatible Docker Compose, volumes persistants et connexion sécurisée** est nécessaire. GitHub ne fait pas tourner ce programme 24 h/24.

Sur un serveur Docker correctement configuré, avec `GUARDIAN_PASSWORD` et `GUARDIAN_AGENT_TOKEN` distincts (32 caractères aléatoires recommandés chacun), les commandes sont :

```bash
docker compose -f compose.cloud.yaml pull
docker compose -f compose.cloud.yaml up -d
docker compose -f compose.cloud.yaml ps
```

La publication de l'image ne garantit pas sa visibilité publique : il peut être nécessaire d'accorder au serveur un accès GitHub Container Registry avant le premier `pull`. Fixer `GUARDIAN_IMAGE_TAG=sha-<commit complet>` permet de déployer une version déterminée plutôt que `main`. Ne jamais stocker les secrets dans GitHub, dans le dépôt ou dans des captures partagées. Stocker les données dans les volumes et prévoir une sauvegarde chiffrée.

Le port HTTP reste lié à `127.0.0.1` **sur le serveur**. Pour y accéder depuis un téléphone, déployer **un VPN ou un accès HTTPS avec authentification**. Ne pas ouvrir directement le port 8765 sur Internet. Aucune infrastructure cloud n'a été provisionnée au moment de cette modification.

Les vérifications de santé ne sont pas une IA réparatrice universelle : elles servent à détecter les blocages et à établir un diagnostic fiable, avant des réparations ciblées et testées.


Pour preparer un hebergement depuis un telephone, consulter [le guide Railway](deploy/RAILWAY.md) : quatre services independants et quatre volumes persistants, avec facturation et autorisations a verifier avant de deployer.

## V6 : fiabilisation gratuite (GitHub Actions)

Sans payer d'hebergement, nous pouvons encore tester et corriger le code dans GitHub :

- **Controle general** : reste configure a 600 secondes (10 minutes) pour les profils fictifs et leurs reprises de simulation.
- **Surveillance distante** : un second controle des heartbeats tourne **toutes les 30 secondes** lorsque le serveur fonctionne. Avec le seuil de 180 secondes sans heartbeat, l'alerte arrive generalement sous 180–210 secondes si le processus Guardian reste actif.
- **Isolation** : si le controle des profils locaux echoue, celui des pilotes continue (et inversement) ; les erreurs de diagnostic sont limitees a leur categorie technique.
- **Rapport automatique de CI** : le test Docker des trois pilotes produit `artifacts/guardian-smoke.json`. GitHub Actions conserve ce rapport durant 7 jours, accessible dans les artefacts du workflow de test. Le fichier est genere avec une liste **strictement autorisee** de statuts et de compteurs : aucun mot de passe, jeton, capture d'ecran, log complet ou information de compte de jeu.
- **Tests de securite** : la suite verifie que les erreurs non fiables et les faux champs secrets ne peuvent pas se retrouver dans le rapport.

**Important :** GitHub Actions execute ponctuellement les tests, mais n'heberge pas Guardian en continu. Les scans toutes les 30 secondes et 10 minutes ne sont actifs que lorsque Guardian tourne effectivement sur une machine. Aucune depense cloud n'est engagee par cette etape.

## V7 : chaos tests, reprises durables et analyse de pannes multiples

**Aucun serveur payant ni service de jeu n'est nécessaire pour exécuter les tests.**

- Les pilotes persistent maintenant leur dernier type d'incident dans une table SQLite locale. Lors d'un redémarrage, un contrôle réellement réussi peut confirmer la réparation de l'incident qui existait avant le crash.
- Pour éviter les avalanches d'événements, une panne locale persistante ne génère pas la même alerte à chaque cycle. Les messages en attente restent durables dans l'outbox.
- Les interruptions réseau répétées utilisent un numéro de génération de heartbeat : les reconnexions produisent de nouveaux épisodes de panne même si l'horloge système change. Les anciennes bases SQLite sont migrées sans suppression des incidents.
- La section **Incidents > Diagnostics distants > Analyse des incidents** regroupe les alertes : plusieurs pilotes déconnectés, même signature d'erreur, panne isolée ou panne répétée. Ce sont des **hypothèses déterministes**, pas des diagnostics d'un modèle de langage.
- Les recommandations sont en lecture seule. Il n'y a pas de commande distante, pas de déploiement automatique de code, pas de suppression de fichiers et pas d'accès à des comptes Clash of Clans.
- `tests/test_chaos_recovery.py` couvre les pannes en parallèle, l'interruption au milieu des accusés de réception, l'état durable après crash, la migration et des incidents simultanés sur **50 profils fictifs**. `tests/test_correlation.py` vérifie l'isolation, les pannes partagées et les recommandations non dangereuses.

Commande d'essai dans GitHub Actions : les workflows existants exécutent automatiquement `python -m unittest discover -s tests -v` et le test de démarrage des quatre conteneurs. Les résultats restent inspectables depuis un téléphone dans GitHub ; la supervision réelle toutes les 30 secondes nécessite toujours un serveur en fonctionnement.

## V8 — Début du vrai moteur d'exécution Android

**C'est une nouveauté distincte des scénarios AI Guardian.** La couche `reqngo/android/` implémente une connexion ADB à un appareil Android sélectionné, une capture d'écran PNG et une reconnaissance visuelle par OpenCV. Elle peut exécuter un geste de test uniquement avec une confirmation d'état avant/après et l'autorisation explicite de l'opérateur. Le journal SQLite empêche toute répétition silencieuse d'une action après crash.

Les scripts sont en **lecture seule par défaut** ; le simple lancement ne clique pas dans le jeu. Les tests GitHub utilisent un faux appareil pour les commandes Android et de véritables PNG de synthèse pour la reconnaissance visuelle. Cela valide le code, **pas les interactions avec Clash of Clans**.

La calibration des écrans et des gestes du jeu n'est pas encore possible sans appareil/émulateur et captures authentiques. Voir [Installation, calibration et limites](docs/ANDROID_PILOT.md). Les dépendances Android sont séparées dans `requirements-android.txt`, pour ne pas alourdir le serveur Guardian.

## Organisation
`reqngo/core.py` : moteur SQLite indépendant. `reqngo/server.py` : API web et superviseur. `reqngo/static/index.html` : dashboard. `tests/` : scénarios automatisés.

## État
**Pilotage Android expérimental** : transport ADB et automate à états ajoutés, sans test sur appareil réel. Les 50 comptes fictifs de Guardian ne sont pas des comptes de jeu. Pas d'émulateur, pas de cloud, pas de réparation autonome de code sur les systèmes réels.

## V9 : planificateur de flotte pour de nombreux profils logiques

Une nouvelle base `fleet.sqlite3` et un module `reqngo/fleet.py` gerent des profils **fictifs**, distincts des 50 profils historiques de Guardian. Import par lots de 10 000 maximum, consultation paginee, taches de diagnostic idempotentes, baux avec jeton de confirmation et recuperation bornee apres interruption. L'interface **Comptes > Flotte grande echelle (V9 · test)** presente les compteurs et permet de creer des profils de test.

La suite teste notamment 5 000 profils en base et deux connexions concurrentes. **Cela ne valide ni 5 000 appareils/connexions simultanes, ni une automatisation du jeu.** Les workers de test V4 et le pilote Android V8 ne sont pas encore relies a ce planificateur. SQLite reste un choix de prototype mono-stockage ; une migration PostgreSQL, des identities worker distinctes et des tests de charge seront indispensables pour une execution cloud distribuee.

Lire [Architecture et limites de la flotte](docs/FLEET_SCALE.md). Aucun cout de serveur ni deploiement n'est declenche par ce changement.

## V10 : file de flotte PostgreSQL et workers de diagnostic distribues

**La V10 ne pilote pas encore des comptes de jeu.** Elle introduit un backend PostgreSQL optionnel avec attribution transactionnelle de taches, protections contre les doublons, baux et horloges de base de donnees, ainsi que des workers techniques identifiables individuellement par un secret distinct.

Le tableau de bord affiche les workers de diagnostic, leur capacite et les etats de file. Le fichier `compose.fleet-demo.yaml` permet de tester en local Guardian + PostgreSQL + 2 workers. Les anciens pilotes V4 restent disponibles via `compose.yaml`; le module Android V8 reste volontairement separe.

Tests GitHub : workflow `Fleet PostgreSQL tests` en plus des tests Guardian. La capacite **5 000 profils en base** n'est pas une mesure de 5 000 sessions simultanees. Les bases Guardian et Telemetry restent locales; avant la haute disponibilite, il faudra aussi les rendre partagees.

Voir [Guide V10 et limites de securite](docs/FLEET_V10.md). Aucun serveur payant n'est cree automatiquement.


## V11 : workers Android en lecture seule (routage explicite)

La V11 permet d'affecter une **sonde Android de diagnostic** à un worker déterminé ; le numéro de série ADB reste dans la configuration locale du worker. La base refuse qu'un autre worker prenne cette tâche et préserve les données existantes grâce à des colonnes ajoutées sans suppression. Les sondes classiques et les trois processus pilotes V4 restent inchangés.

Ce **n'est pas une automatisation de Clash of Clans** : le contrôle est limité à `adb devices` et, sur activation locale, à la présence du processus. Aucune capture ni geste tactile, ni connexion de comptes. Voir [Guide V11 : ADB et routage](docs/ANDROID_WORKER_V11.md). Sans appareil Android autorisé et connecté, la validation CI reste simulée.


## V12 : surveillance des incidents à l'échelle d'une flotte

Guardian détecte automatiquement les workers de diagnostic n'ayant pas envoyé de heartbeat depuis 180 secondes et enregistre un incident durable, sans doublons à chaque contrôle. Le retour d'un worker clôt l'incident. Le dashboard affiche en complément la capacité déclarée des workers actifs, l'âge de la file prête et les incidents récents. L'API `GET /api/fleet/incidents` est protégée et paginée par une limite stricte.

Cette surveillance fonctionne avec SQLite local ou PostgreSQL partagé. Elle ne redémarre aucun compte et n'effectue aucune action dans Clash of Clans. Les contrôles sont exécutés à intervalle régulier, mais cela ne prouve pas une disponibilité cloud 24h/24 tant que le service n'est pas déployé et surveillé. Voir [Guide V12](docs/FLEET_HEALTH_V12.md).

## V13 — capacite diagnostique et benchmark multi-workers

Un diagnostic de charge consultable dans **Comptes > Flotte grande echelle** (et via `GET /api/fleet/capacity`) donne une alerte non-intrusive lorsque des sondes techniques attendent trop longtemps ou qu'aucun worker n'est actif. Les seuils sont documentes; il ne s'agit **pas** d'une commande d'auto-scaling.

Un benchmark en CI sur une base PostgreSQL jetable mesure les latences p50/p95/p99 et le debit de sondes synthetiques sur **six workers**. Les rapports de mesure sont conserves en artefacts GitHub Actions, sans mots de passe. Cette etape ne prouve pas la capacite de faire fonctionner des milliers de sessions Android ou Clash of Clans simultanement. Voir [Guide capacite V13](docs/FLEET_CAPACITY_V13.md).

## V14 — metriques par worker et inventaire Android anonyme

Les workers remontent CPU, charge RAM hote et consommation RSS du processus avec `psutil`; le comptage des appareils ADB connectes est un **opt-in explicite**, limite aux serials deja lies localement au worker. Aucune adresse, identifiant d'appareil, capture ou compte ne remonte vers Guardian. Les donnees existantes et les anciens workers restent compatibles avec une migration additive SQLite/PostgreSQL.

Le tableau de bord presente ces mesures et un indicateur de saturation (CPU/RAM vus par le systeme >=95 %). **Un appareil Android detecte ne signifie pas une session de jeu active et la charge hote Docker ne correspond pas necessairement aux limites d'un conteneur.** Le systeme ne fait pas d'auto-scaling et ne commande aucune action dans le jeu.

Voir [Guide des mesures V14](docs/FLEET_METRICS_V14.md).

## V15 — nouvelle interface Fleet Manager

Une console responsive en français, accessible sur **`/fleet`**, organise les profils logiques fictifs par groupes persistants. Recherche et filtres sont executés coté serveur, avec 50 lignes par page et une sélection maximale de 200 profils. Des actions groupées d'activation/suspension exigent une prévisualisation et une confirmation explicite, puis sont enregistrées dans un journal d'audit. La page présente également les statistiques, workers, ressources et incidents déjà fournis par V9–V14.

L'ancien écran Guardian `/` reste disponible. Les fonctions de contrôle Android ou de gameplay ne sont pas déclenchées par cette console. Voir [Guide V15](docs/FLEET_CONSOLE_V15.md).

## V16 — politiques de groupe et diagnostic individuel

La console `/fleet` permet désormais d'enregistrer une **affinité de worker technique** et un intervalle souhaité au niveau du groupe, puis de les laisser en héritage ou de définir des exceptions pour chaque profil. En ouvrant un profil, tu retrouves le worker effectif, les sources de ses réglages, les derniers résultats de diagnostics et des boutons de sondes de stockage ou Android en lecture seule. Les nouveaux diagnostics créés par la console ciblent le worker effectif et utilisent la file transactionnelle existante.

L'intervalle enregistré est un paramètre préparatoire : il ne déclenche **pas** encore de scans automatiques. Aucun compte de jeu, émulateur ou téléphone cloud n'est piloté par cette interface. Les routes administratives sont protégées et les modifications sont journalisées.

Voir [Documentation V16](docs/FLEET_POLICIES_V16.md).
