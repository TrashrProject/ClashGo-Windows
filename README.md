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

## Organisation
`reqngo/core.py` : moteur SQLite indépendant. `reqngo/server.py` : API web et superviseur. `reqngo/static/index.html` : dashboard. `tests/` : scénarios automatisés.

## État
V1 pilote : **pas de comptes réels, pas d'émulateur, pas de cloud et pas d'IA capable de modifier le code seule**.