# REQ N GO Manager V10 — orchestration distribuee (pilote)

**Portee : profils logiques et diagnostics techniques sur des ressources controlees.**
Aucune interaction Clash of Clans, session de jeu ou automatisation de gameplay n'est branchee. Les changements V10 n'executent pas de programme tiers.

## Architecture

- `reqngo/fleet.py` — moteur SQLite de la V9, conserve pour les tests sur **une machine**.
- `reqngo/fleet_postgres.py` — moteur PostgreSQL 16 avec transactions partagees, verrous `FOR UPDATE SKIP LOCKED`, exclusions par profil, idempotence et baux limites.
- `reqngo/fleet_factory.py` — choisit PostgreSQL **uniquement** si `FLEET_DATABASE_URL` est defini.
- `reqngo/fleet_api.py` — endpoints admin et endpoints worker independants, avec cle d'acces distincte par worker.
- `reqngo/fleet_worker.py` — processus autorise qui effectue une sonde I/O sur **son propre dossier**, puis renvoie un resultat; aucune commande arbitraire.
- `compose.fleet-demo.yaml` — un Guardian, une instance PostgreSQL et **deux workers techniques**.

Un profil n'est pas un appareil. `5 000 profils` representent une base de donnees de 5 000 objets geres; la capacite de sessions actives depend des CPU, RAM, I/O, quotas et tests reels.

## Garanties implementees

1. Les demandes portent une cle d'idempotence; une meme cle ne cree jamais deux taches.
2. Une contrainte unique SQL empeche deux taches ouvertes pour un meme profil.
3. Les prises en charge utilisent un bail avec jeton aleatoire; le serveur lie le worker a une identite authentifiee. Un autre worker ne peut pas terminer sa tache.
4. PostgreSQL verifie les dates avec **son horloge**, pas les horloges divergentes des workers.
5. Si un worker meurt, un bail expire est recupere progressivement avec 3 tentatives maximales **uniquement sur la sonde technique**, qui peut etre repetee sans effet sur un jeu.
6. Apres un echec de sonde, la tache passe en `needs_review`. Un futur moteur d'actions non idempotentes ne doit **jamais** reexecuter automatiquement une action dont l'issue reste inconnue.
7. Capacite active borne par worker et lots de taches plafonnes a 8 depuis l'API HTTP; identifiants et cles des workers distincts.
8. Tableau de bord : compteurs, pagination, workers recents, type de stockage et bouton de sonde. Les identifiants sensibles ne figurent pas dans la sortie du tableau de bord.

## Tester en local (sans hebergement)

Sur une machine avec Docker Compose, creer un fichier **`.env` non versionne**, en remplissant avec **5 secrets differents**, longs et aleatoires. Ne jamais mettre ces valeurs dans GitHub ou une conversation.

```dotenv
GUARDIAN_PASSWORD=REPLACE_WITH_UNIQUE_RANDOM_32_PLUS
GUARDIAN_AGENT_TOKEN=REPLACE_WITH_DIFFERENT_RANDOM_32_PLUS
FLEET_DB_PASSWORD=REPLACE_WITH_THIRD_RANDOM_32_PLUS
FLEET_WORKER_01_TOKEN=REPLACE_WITH_FOURTH_RANDOM_32_PLUS
FLEET_WORKER_02_TOKEN=REPLACE_WITH_FIFTH_RANDOM_32_PLUS
```

Les textes `REPLACE_...` sont des exemples, **pas des secrets de production**. Generer de vraies valeurs avec `python -c "import secrets; print(secrets.token_urlsafe(36))"` pour chacune. Ne pas publier `docker compose config` : sa sortie peut afficher des secrets d'environnement.

Puis :

```bash
docker compose -f compose.fleet-demo.yaml up --build -d
docker compose -f compose.fleet-demo.yaml ps
```

L'interface Guardian est liee a `http://127.0.0.1:8765`. Dans **Comptes > Flotte grande echelle**, creer des profils de test, puis utiliser **Ajouter une sonde technique** en indiquant un profil existant. La sonde passe de `queued` a `leased` puis `done` quand un worker teste son stockage.

```bash
docker compose -f compose.fleet-demo.yaml logs --tail=30 fleet01 fleet02
docker compose -f compose.fleet-demo.yaml down
```

Ne pas utiliser `down -v` sauf si les volumes **de demonstration** doivent reellement etre detruits. Cette configuration de demonstration n'expose ni PostgreSQL ni les workers sur Internet. Pour un usage sur serveur, securiser Guardian derriere HTTPS avec authentification forte et controle d'acces. La simple authentification Basic ne suffit pas comme solution production publique.

## Backend en cloud

Pour passer d'une installation SQLite a PostgreSQL : definir `FLEET_DATABASE_URL` sur le processus Guardian avec une chaine PostgreSQL privee et certifiee TLS, puis installer les dependances `requirements.txt`. Aucune migration de donnees SQLite vers PostgreSQL n'est effectuee automatiquement; les donnees historiques V9 demeurent dans SQLite tant qu'une migration explicite n'est pas ecrite et validee.

Chaque worker a `FLEET_WORKER_ID` et `FLEET_WORKER_TOKEN`; Guardian recoit leur table de correspondance `FLEET_WORKER_TOKENS_JSON` depuis un gestionnaire de secrets. Les requetes HTTP ajoutent `X-Fleet-Worker-Id` et un Bearer distinct, avec TLS ou un reseau Docker prive. Les cles peuvent etre remplacees par changement de configuration, mais la rotation sans interruption et une gestion des identites a grande echelle restent a realiser.

PostgreSQL peut coordonner plusieurs replicas du serveur *pour la file de flotte*, mais les anciennes bases Guardian/Telemetry restent SQLite locales : **elles ne doivent pas etre reparties sans migration supplementaire**. Cette V10 est un premier socle distribue, pas un cluster Guardian completement haute disponibilite.

## Validation et limitations

- `tests/test_fleet_workers.py` : authentification par worker, refus de prise de controle, confirmation protegee, 2 slots paralleles, detection de tache inconnue.
- `tests/test_postgres_fleet.py` : vraie PostgreSQL, 5 000 profils de test, deux connexions distinctes revendiquant 100 taches sans duplication, expiration de bail et plafond de capacite.
- Workflow `.github/workflows/fleet-postgres-tests.yml` : PostgreSQL 16 jetable en CI. **Il n'est pas un hebergement permanent.**
- En production, prevoir migrations versionnees, backups testees, quotas, retention des jobs, secrets par service hors environnement si necessaire, metrics p95/p99, alerting, supervision de PostgreSQL, rotation des jetons, audit et simulation de pannes.
- Ne pas confondre tests sur 5 000 profils **enregistres** et execution simultanee de 5 000 sessions; cela n'a pas ete mesure ni garanti.
