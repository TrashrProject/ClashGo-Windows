# REQ N GO V13 — mesure de charge et diagnostic de capacite

La V13 conserve les moteurs et protections V9–V12. Elle ajoute deux outils **en lecture seule pour le systeme de supervision** : un conseil de capacite base sur des compteurs techniques et un benchmark synthetique d'operations en base. Aucune commande de jeu, session Clash of Clans ni geste Android n'est effectuee.

## Diagnostic de capacite

Nouvelle route admin authentifiee : `GET /api/fleet/capacity`.

Le calcul n'expose aucun profil ni secret et distingue :

- `nominal` : aucune pression identifiee selon les seuils decrits ci-dessous.
- `idle_without_workers` : aucun worker recent, sans tache prete.
- `no_capacity` : une ou plusieurs sondes attendent, mais aucun slot declare actif.
- `backlog_pressure` : au moins une sonde attend depuis **120 secondes** ou plus, OU le nombre de sondes pretes depasse **20 par slot declare**.
- `worker_incidents` : un incident de deconnexion demeure ouvert.

Les regles sont heuristiques et reposent sur les heartbeats et les compteurs existants. Les **slots sont declares par les workers**, pas mesures comme appareils ou sessions reels. Cet outil ne provisionne pas de machines, ne relance pas de services et ne remedie pas seul aux problemes.

Le tableau de bord affiche l'etat observe et une suggestion de diagnostic, sans modifier la configuration.

## Benchmark de concurrence

Le script `tools/fleet_load_benchmark.py` requiert :

1. Une base PostgreSQL **jetable** dont le nom se termine par `_ci` ou `_test`, verifie sur la connexion.
2. La variable `FLEET_BENCH_DISPOSABLE=1`, afin d'eviter les executions accidentelles.
3. `FLEET_TEST_PG_DSN` pour cette base et ses identifiants, qui **ne figurent jamais** dans le rapport.

Exemple dans la CI : **5 000 profils logiques**, **1 200 sondes de diagnostic**, **6 workers synthetiques**, lots de **8 taches**. Chaque worker reclame et confirme ses baux depuis sa propre instance de la couche PostgreSQL. Sont verifies : comptage de tous les jobs termines, absence de double attribution, pas de jobs en revue, respect d'un delai maximum. L'outil mesure :

- Debit du traitement des sondes (`throughput_jobs_per_second`);
- Latences p50, p95 et p99 (millisecondes) pour la mise en file, l'attribution et la confirmation;
- Temps total du lot, taches terminees, nombre de doublons.

Les nombres obtenus dependent de la machine de CI, de la contention, du reseau local et du disque. **Ils ne correspondent ni a la vitesse de Xingchen, ni a des actions en jeu, ni au nombre maximal de sessions Android.**

Le rapport `artifacts/fleet-benchmark.json` est conserve pendant 14 jours dans les artefacts GitHub Actions et contient exclusivement les valeurs de benchmark (aucun DSN, secret, nom de joueur ou serial Android). Le benchmark n'est pas une charge longue de 24 h.

## Utilisation et securite

Pour lancer les tests locaux standards sans serveur :

```sh
python -m unittest discover -s tests -v
```

Pour utiliser une vraie base locale jetable sous controle de l'operateur :

```sh
FLEET_TEST_PG_DSN="postgresql://<service-user>:<secret>@127.0.0.1:5432/fleet_ci" FLEET_BENCH_DISPOSABLE=1 python tools/fleet_load_benchmark.py --profiles 3000 --jobs 1000 --workers 6
```

Il ne faut **jamais** diriger ce script vers une base en production. Le benchmark insere de nouveaux profils et jobs, sans effacer l'historique existant. Un test de longue duree demandera un banc distinct, des donnees reinitialisables, des quotas et une retention des historiques et des metriques.

## Etapes restantes avant une grande flotte active

1. Metriques systeme **mesurees** (CPU/RAM/IO et charge reelle par appareil) et comparaison de materiels.
2. Limitation centralisee des demandes/quotas et gestion des appareils independante des profils.
3. Tests de reprise apres coupures reseau et redemarrages, backup/restauration PostgreSQL et rotation des secrets.
4. Tests repetes de charge puis de stabilite sur des workers Android autorises.
5. Migration vers une architecture cloud durable et surveillee, apres choix d'un budget.

**Aucun deploiement payant n'est declenche par les tests CI.**
