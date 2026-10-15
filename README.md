# REQ N GO Manager — AI Guardian (pilote)

Nouveau socle de supervision du projet. **Ce code n'est PAS un bot de farming ni une automatisation Clash of Clans.** Il fournit un tableau de bord, 50 profils fictifs, 3 comptes pilotes, un moteur de diagnostic et des reprises simulées. La connexion au jeu et aux comptes réels n'est pas implémentée.

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


## V5 : préparation cloud sans ordinateur personnel

**Cette étape prépare le déploiement, mais ne loue ni ne démarre de serveur cloud.** Le dépôt contient désormais :

- `GET /health/live` : vérifie que le processus web répond, sans exposer de données.
- `GET /health/ready` : vérifie l'accès aux deux bases SQLite, sans exposer de données.
- Une sonde locale par pilote qui détecte si sa propre boucle n'a plus progressé.
- `compose.yaml` : construction et test des 4 conteneurs à partir du code.
- `compose.cloud.yaml` : même topologie, mais téléchargement d'une image versionnée depuis GitHub Container Registry (GHCR).
- `.github/workflows/guardian-image.yml` : après fusion dans `main`, tests puis publication des images `main` et `sha-<commit>` si les permissions GitHub le permettent.
- `tools/compose_smoke.py` : test d'intégration des 3 pilotes contre le vrai serveur dans GitHub Actions, **sans connexion au jeu**.

### Quand un serveur sera connecté

L'hébergeur devra disposer de Docker Compose (un hébergement de service web simple ne suffit pas forcément), d'un stockage persistant pour **quatre volumes**, et d'une entrée sécurisée HTTPS/VPN. Dans les secrets de l'hébergeur, définir `GUARDIAN_PASSWORD` (au moins 16 caractères) et `GUARDIAN_AGENT_TOKEN` (au moins 32 caractères), différents et aléatoires.

```bash
docker compose -f compose.cloud.yaml pull
docker compose -f compose.cloud.yaml up -d
docker compose -f compose.cloud.yaml ps
```

**Ne pas exposer directement le port 8765.** Il reste lié à localhost dans les configurations fournies. L'accès depuis un iPhone exigera une passerelle HTTPS authentifiée ou un VPN. Une image GHCR n'est téléchargeable sans identifiants que si sa visibilité et les droits d'accès le permettent. Épingler `GUARDIAN_IMAGE_TAG=sha-<commit>` pour une mise en production vérifiable plutôt que d'utiliser systématiquement `main`.

Les sondes Docker marquent un conteneur `unhealthy` sans automatiquement le relancer si son processus reste vivant. Le redémarrage sur sortie de processus est assuré par `restart: unless-stopped`. Un opérateur de remédiation ultérieur devra valider les actions, conserver un historique, appliquer des limites de fréquence et permettre un retour arrière.

**Statut réel :** aucun serveur 24 h/24 ni domaine n'est encore connecté ; il n'existe aucune connexion à Clash of Clans ou à des émulateurs, aucune fonction d'IA capable de modifier seule le code. Les profils de comptes restent des simulations de gestion.

## Organisation
`reqngo/core.py` : moteur SQLite indépendant. `reqngo/server.py` : API web et superviseur. `reqngo/static/index.html` : dashboard. `tests/` : scénarios automatisés.

## État
Pilote cloud V5 : **pas de comptes réels, pas d'émulateur, pas de cloud et pas d'IA capable de modifier le code seule**.