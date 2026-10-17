# V11 — Android en lecture seule et routage des appareils (pilote)

**Statut : intégration de diagnostic ; aucune action de jeu automatisée.** La V11 réutilise les bases V9/V10, les workers et l'adaptateur ADB de la V8 sans reconstruction. Elle permet de demander une **vérification de présence d'un appareil Android** sur un worker déterminé, sans envoyer son numéro de série au serveur.

## Problème résolu

Un worker technique générique pouvait recevoir n'importe quelle sonde de stockage. Cela convient à des milliers de profils logiques, mais une sonde d'un appareil Android doit être dirigée vers **la machine sur laquelle cet appareil est accessible**.

Chaque tâche comporte désormais :
- `probe_mode=storage` (ancienne sonde, comportement par défaut) ou `probe_mode=android`.
- `target_worker` (optionnel pour stockage, **obligatoire pour Android**).

Les deux moteurs SQLite et PostgreSQL appliquent la contrainte d'aiguillage dans leur requête d'attribution : un autre worker **ne peut pas revendiquer la tâche**. Les colonnes sont ajoutées aux bases existantes de manière additive ; les anciennes tâches sont automatiquement considérées comme `storage`.

Les clés d'idempotence vérifient désormais aussi le mode et le worker cible. Un changement de worker sur une tâche existante avec la même clé est rejeté.

## Vérification ADB

L'appareil doit avoir été autorisé pour ADB et son numéro de série configuré localement. Les contrôles possibles sont :

1. **Présence** : `adb devices` doit répertorier précisément cet appareil comme connecté.
2. **Optionnel, processus de jeu** : `pidof com.supercell.clashofclans` vérifie si le processus est actif.

Aucune capture d'écran, entrée tactile, exécution de workflow, connexion Supercell ID, installation d'APK ou extraction de données privées n'a lieu. `game_running()` ne prouve pas que le jeu fonctionne correctement ni que les renforts sont reçus.

Si l'appareil n'est pas associé au profil, s'il est hors ligne ou si ADB est indisponible, la sonde est **en échec**, puis placée en `needs_review`. Aucun essai de reconnexion de compte ni automatisme risqué ne se déclenche. Les sondes Android et les sondes de stockage restent des opérations techniques répétables après expiration d'un bail.

## Configurer un worker Windows/Android autorisé

Configurer sur **la machine du worker**, jamais dans GitHub ou dans un message, les variables :

```powershell
$env:FLEET_WORKER_ID = "worker_adb"
$env:FLEET_WORKER_TOKEN = "<secret-individuel-depuis-votre-gestionnaire-de-secrets>"
$env:GUARDIAN_URL = "https://votre-guardian-protege.example"
$env:FLEET_ANDROID_PROFILE_SERIALS_JSON = '{"test-000001":"emulator-5554"}'
$env:FLEET_ANDROID_REQUIRE_GAME = "0"
python -m reqngo.fleet_worker --once
```

*Ces valeurs sont illustratives ; le numéro de série doit être obtenu avec `adb devices`, et le worker doit être autorisé dans `FLEET_WORKER_TOKENS_JSON` côté Guardian. Ne pas diffuser de secrets ou de liste d'appareils dans des journaux publics.*

- Installer Android SDK Platform Tools sur le worker et s'assurer que `adb` est accessible par le `PATH` (ou `ADB_BINARY`).
- Si `FLEET_ANDROID_REQUIRE_GAME=1`, l'application doit être démarrée, mais aucun geste ne sera effectué.
- Le JSON peut lier plusieurs profils logiques à un même appareil ; les requêtes de cet appareil sont sérialisées par un verrou local pour éviter des interrogations ADB simultanées. Ce mécanisme n'assure pas l'isolation entre plusieurs **processus** utilisant le même appareil : en production, attribuer un appareil à un seul processus à la fois.
- Sans mappage explicite, le worker ne peut pas déclarer une sonde Android réussie.
- Les workers génériques Docker déjà existants continuent de traiter seulement leurs tâches `storage`. Les fonctions ADB **ne sont pas installées dans les conteneurs par défaut** ; exécuter un worker ADB sur un hôte possédant les outils et appareils.

## Lancer une sonde

Dans le tableau de bord **Comptes > Flotte grande échelle**, choisir un profil déjà créé, sélectionner **ADB : appareil associé (lecture seule)** et entrer l'identifiant exact du worker qui détient l'appareil.

Alternativement, depuis une session administrateur HTTPS authentifiée :

```json
{
  "profile_key": "test-000001",
  "dedupe_key": "adb-readonly-test-001",
  "probe_mode": "android",
  "target_worker": "worker_adb"
}
```

`POST /api/fleet/probes` nécessite les protections administrateur existantes, dont `X-Guardian-Action: 1`.

## Validation et limites

Les tests V11 couvrent : validation des requêtes, ajout non destructif des colonnes SQLite, distribution vers le bon worker, refus des idempotences contradictoires, simulation ADB hors ligne, blocage des commandes d'écriture et reprise du comportement V10. PostgreSQL est testé sur une véritable instance PostgreSQL dans GitHub Actions.

**Aucune session Android réelle n'était disponible dans l'environnement GitHub CI.** Il est donc impossible d'affirmer que ces sondes fonctionnent sur l'appareil du joueur sans exécution contrôlée sur un téléphone/émulateur autorisé.

**Grandes flottes :** ces améliorations conservent la séparation entre milliers de profils enregistrés et workers actifs. Avant un fonctionnement 24 h/24 à grande échelle : ajouter la supervision de capacité par appareil, une gestion persistante des affectations, tests de charge longs, quotas, alertes et métriques p95/p99.
