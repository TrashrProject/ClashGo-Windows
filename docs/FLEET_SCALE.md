# REQ N GO V9 — socle de gestion de flotte

**Etat : implementé sur une branche de test.** Le planificateur gere des **profils logiques de test**, pas des comptes Clash of Clans connectes. Aucune authentification au jeu ni automatisation de gameplay n'est fournie par ce module.

## Pourquoi il ne faut pas un processus Android par compte

Un **profil logique** represente une identite de travail, avec son etat et ses taches. Un **worker** represente une ressource d'execution qui peut servir successivement plusieurs profils. Ainsi 5 000 profils repertories ne signifient jamais 5 000 emulateurs actifs, ni 5 000 operations simultanees.

Les ressources executrices doivent etre dimensionnees separement, suivant les mesures reelles de memoire, CPU, debit, contention et contraintes du fournisseur. Les 3 pilotes V4 et l'Android experimental V8 ne sont pas encore raccordes a la file V9.

## Ce qui fonctionne deja

- Import par lots de 1 a 10 000 **profils fictifs** a la fois, sans mot de passe ni identifiant de jeu.
- Liste paginee (200 maximum par appel), compteur global et etat de la file sans charger tous les profils dans le navigateur.
- File SQLite durable avec contrainte SQL interdisant deux taches ouvertes simultanees sur le meme profil.
- Attribution atomique a un worker et **bail avec jeton aleatoire** : apres expiration ou reattribution, l'ancien worker ne peut pas confirmer la tache.
- Cles d'idempotence pour eviter les demandes repetees.
- Retentative bornee (**3 prises en charge**) des seules sondes techniques en lecture seule. A l'echec definitif ou si une sonde signale un echec : etat `needs_review`, sans action dangereuse.
- Isolation : un profil desactive ne recevra plus de nouveaux baux.
- Tests : 5 000 profils en base, 100 taches reparties entre deux connexions concurrentes, reprise apres interruption, verification de bail, limites HTTP.

## API admin de test

Toutes les routes sont protegees par les gardes deja presents dans Guardian. Les ecritures demandent en plus l'en-tete `X-Guardian-Action: 1`.

| Methode | Route | Usage |
| --- | --- | --- |
| GET | `/api/fleet/summary` | Compteurs de la flotte logique |
| GET | `/api/fleet/profiles?offset=0&limit=100` | Pagination |
| POST | `/api/fleet/profiles/batch` | `{"prefix":"test","start":1,"count":5000}` |
| POST | `/api/fleet/probes` | `{"profile_key":"test-000001","dedupe_key":"read-only-check-0001"}` |
| POST | `/api/fleet/profiles/test-000001/enabled` | `{"enabled":false}` |

**Important :** ces routes n'exposent **pas** la prise en charge des taches par un worker externe. Aucun jeton de bail n'est accessible par HTTP. Cette integration exigera une authentification **par worker**, des permissions limitees, une rotation des secrets et un journal d'audit avant de l'exposer.

## Limites de la V9

SQLite prend en charge les tests concurrents sur un **seul stockage local**; ses ecritures sont serialisees. Ne pas partager le fichier SQLite par reseau entre plusieurs serveurs. **Un test de 5 000 lignes ne certifie pas 5 000 connexions actives simultanement.**

Avant une exploitation de grande taille : migration vers PostgreSQL avec `FOR UPDATE SKIP LOCKED` ou equivalent, partitionnement/retention des historiques, comptes de service par worker, capacite configurable par groupe, quotas, metrics p95/p99, backpressure, sauvegardes/restauration et tests de charge en environnement isole.

Ne jamais reexecuter automatiquement apres expiration du bail une action ayant des effets externes dont l'issue est inconnue. Dans un futur moteur d'execution, ces actions devront passer en `needs_review` ou etre reconciliees sur une preuve d'etat validee.

## Verification

`python -m unittest discover -s tests -v`

La CI existante inclut automatiquement `tests/test_fleet.py` et `tests/test_fleet_api.py` au prochain push/PR. Aucun abonnement ni deploiement Railway n'est declenche par ces modifications.
