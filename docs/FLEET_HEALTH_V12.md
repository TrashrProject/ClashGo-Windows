# REQ N GO V12 — supervision de flotte et incidents workers

Cette version complète la V11 sans la remplacer. Elle n'automatise aucun compte de jeu et ne commande aucun appareil Android. Les workers restent des exécutants de **sondes de diagnostic** sur des ressources autorisées.

## Ce qui change

- **Détection automatique des absences** : le serveur lance une vérification de la flotte toutes les 30 secondes, indépendamment des contrôles historiques de Guardian. Une absence supérieure à **180 secondes** depuis le dernier heartbeat ouvre un incident.
- **Pas d'alertes en boucle** : un worker absent génère un seul incident `open`, conservé tant qu'il ne revient pas. Son heartbeat ferme l'incident (`resolved`). Si la panne réapparaît, le compteur `occurrences` augmente.
- **Résistance à plusieurs processus** : PostgreSQL conserve les incidents dans la base partagée, verrouille les lignes des workers pendant la réconciliation et utilise une clé unique par worker. Les interruptions sont donc dédupliquées au niveau de la base. SQLite conserve une version transactionnelle pour les tests locaux.
- **Charge de travail visible** : le tableau de bord affiche le nombre de slots signalés par les workers connectés, les sondes prêtes en attente et le temps écoulé depuis que la plus ancienne sonde est devenue disponible.
- **Historique limité** : `GET /api/fleet/incidents?limit=50` renvoie uniquement un maximum de 200 enregistrements (ID technique, statut, nombre d'occurrences, dates). Il exige l'authentification du tableau de bord ; aucun jeton, numéro de série ADB ou mot de passe n'est renvoyé.

## Comportement de Guardian

L'ordre de supervision est indépendant : une erreur dans le contrôle historique de Guardian ou la télémétrie n'empêche pas le contrôle de flotte. Une erreur de base dans le contrôle de flotte est enregistrée sous forme de **classe d'erreur**, sans URL de base de données ou trace contenant des secrets.

La reprise est **observée**, pas devinée : seul un nouveau heartbeat prouve qu'un worker est de retour. Cette étape n'exécute ni commande à distance, ni redémarrage de compte, ni correctif de code. Les redémarrages de services déjà gérés par Docker sont distincts des incidents logiques.

## Mesures disponibles

`GET /api/fleet/summary` conserve ses anciens champs et ajoute :

| Champ | Sens |
| --- | --- |
| `total_workers` | Nombre de workers ayant déjà envoyé un heartbeat |
| `online_workers` | Workers avec heartbeat récent (moins de 180 s) |
| `online_slots` | Somme des capacités **déclarées** par ces workers |
| `open_worker_incidents` | Nombre de workers identifiés comme absents lors du dernier contrôle |
| `oldest_ready_wait_seconds` | Temps d'attente de la plus ancienne tâche **déjà disponible** |

**Limites :** `online_slots` est une capacité déclarée par les workers, **pas un nombre de sessions Android prouvées**. Le compteur d'incidents dépend de la fréquence du superviseur. Un worker dégradé mais connecté n'est pas considéré comme absent. Le temps d'attente ne compte pas les jobs différés par un délai de disponibilité.

Un très grand parc demandera aussi des indicateurs p95/p99, un stockage des séries temporelles, des politiques de rétention et des alertes externes. Les 50 000 profils testés précédemment représentent un **inventaire logique**, pas des téléphones actifs.

## Vérifier

```bash
python -m unittest discover -s tests -v
```

Le workflow `Fleet PostgreSQL tests` exécute les tests de concurrence et d'incidents contre un PostgreSQL 16 jetable. Le scénario Docker précédent continue de tester Guardian, PostgreSQL et deux workers. Le système ne déploie aucun abonnement.
