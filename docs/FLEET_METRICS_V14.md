# REQ N GO Manager V14 — mesures CPU/RAM et inventaire ADB anonyme

Cette version ajoute des mesures **reelles de ressources techniques** dans les heartbeats des workers V10-V13, sans automatiser des comptes Clash of Clans ni lancer d'actions Android. Le serveur n'a jamais besoin d'identifiants de comptes, de captures, de numero de serie ADB ni de mot de passe des appareils.

## Valeurs mesurees

Chaque worker equipé de `psutil` (installe par `requirements.txt`) peut communiquer :

| Metrique | Interpretation | Limites |
| --- | --- | --- |
| `cpu_percent` | Utilisation CPU **du systeme vue par psutil** | 0–100 %, pas necessairement cgroup dans Docker |
| `memory_percent` | RAM **du systeme vue par psutil** | 0–100 %, pas necessairement memoire disponible du conteneur |
| `process_rss_mb` | RSS du processus Python worker | Megaoctets resident, pas total d'Android/emulateurs |
| `android_bound_devices` | Nombre de serials distincts configures **localement** | Optionnel, aucun serial transmis |
| `android_online_devices` | Nombre de ces appareils declares `device` dans `adb devices` | Optionnel, lecture seule; un appareil en ligne ne prouve pas un compte actif |

Les champs sont `null` si la mesure est absente. Ne jamais confondre `null` et `0`. Un ancien worker V10-V13 envoyant un heartbeat sans mesures continue a fonctionner et ses champs restent `null`.

Les valeurs transitent par l'endpoint de heartbeat **authentifie individuellement par worker** et sont refusees si elles sont hors limites, non numeriques ou si les nombres d'appareils sont incoherents. Les bases SQLite/PostgreSQL recoivent uniquement cinq colonnes optionnelles, ajoutees sans supprimer l'historique existant. Ce ne sont pas des series temporelles : **seule la derniere valeur connue** est conservée.

## Opt-in ADB

Le comptage Android est **desactive par defaut**. Pour compter les appareils connus d'un worker possedant ADB :

```bash
FLEET_MONITOR_ANDROID=1
FLEET_ANDROID_PROFILE_SERIALS_JSON='{"test-000001":"emulator-5554"}'
```

Ces exemples vont dans les variables privees **du worker**, pas dans GitHub. Les donnees de monitoring ne contiennent que `1/1`, jamais `emulator-5554`. La commande utilisee est `adb devices`, sans capture, geste tactile ou connexion de compte. Si ADB n'est pas accessible : le nombre d'appareils configures demeure connu mais la mesure des appareils en ligne reste **indeterminee**.

Les workers Docker du pilote V10 n'ont pas d'appareils Android connectes : leur valeur ADB reste donc inconnue.

## Visibilite et alertes

Dans `Comptes > Flotte grande echelle`, l'application affiche des metriques sur les workers disponibles et les incidents V12. Un worker est signale comme **fortement charge** si son dernier heartbeat recent fait etat de **CPU >= 95 % ou RAM >= 95 %**. L'API `GET /api/fleet/capacity` peut alors retourner `worker_overload`.

Ce seuil est indicatif, base sur la vue systeme mesuree, **pas une preuve de saturation Android ou de manque de puissance pour faire tourner le jeu**. La V14 ne commande ni redemarrage, ni changement de capacite, ni creation de worker payant. Une alerte de retard dans la file demeure prioritaire lorsqu'elle existe.

## Tests

- Controle strict des plages, rejection des valeurs NaN, Infinity, bool, strings et de tout champ non autorise.
- Opt-in ADB : aucune interrogation lorsque l'option n'est pas activee, et aucune fuite de serial dans la telemetrie.
- Migration additive SQLite de la V13 vers les colonnes V14.
- API authentifiee avec et sans metriques pour compatibilite ancienne.
- PostgreSQL réel : declaration sur un worker, lecture depuis une autre instance, saturation puis remise a zero des champs manquants.
- CI : les tests et le benchmark PostgreSQL V13 restent actifs.

**Prochaines étapes** : temps CPU par processus, memoire par conteneur (cgroup), quotas de ressources par worker, séries temporelles p95/p99, stress tests prolonges et verification sur appareils autorises. Un test logiciel avec des appareils reels reste necessaire pour estimer le nombre de sessions simultanées. Cette etape n'a aucun cout d'hebergement obligatoire.
