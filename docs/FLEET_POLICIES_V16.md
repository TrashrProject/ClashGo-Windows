# REQ N GO Manager — V16 : héritage et diagnostic individuel

La V16 prolonge la console groupée V15 sans la remplacer. La nouveauté est une **configuration technique persistante et hiérarchique** : paramètres de groupe, exceptions par profil, lecture du résultat effectif et création d'une sonde de diagnostic vers un worker ciblé.

## Héritage des préférences

Chaque groupe possède :

- Un **worker de diagnostic préféré** ou aucun : seules les identités de workers déjà enregistrées par un heartbeat peuvent être sélectionnées.
- Un **intervalle de diagnostic souhaité**, de 30 à 3 600 secondes (600 par défaut). **Attention : ce champ est préparatoire, il ne déclenche pas encore de scans périodiques automatiques.**

Chaque profil peut laisser ces deux champs sur « hériter du groupe » ou fournir sa propre valeur. Le résultat résolu est calculé à la demande : changer une politique de groupe peut donc modifier immédiatement les préférences visibles des profils qui n'ont pas d'exception.

Le choix d'un worker est **une affinité de traitement de sondes techniques**, ni une connexion d'un compte au jeu ni une affectation d'un émulateur. Un worker hors ligne peut rester configuré, mais toute nouvelle sonde ciblée est refusée tant que son dernier heartbeat date de plus de 180 secondes.

## Profils et diagnostics

Depuis `/fleet`, ouvrir **Profils → Consulter** pour voir :

- L'identifiant logique, le groupe, l'état d'activation et la provenance de la configuration effective.
- Le dernier heartbeat du worker préféré lorsqu'il existe.
- L'historique des **dix sondes techniques les plus récentes** (état, type, tentative, date). Aucun jeton de bail, secret, capture d'écran, série Android ou clé d'idempotence n'est affiché.
- Deux commandes explicites : **Tester le stockage** (sonde locale de worker) et **Vérifier Android (lecture seule)**.

La sonde Android requiert un worker explicitement affecté, en ligne et configuré **localement** pour le profil concerné. Elle repose sur le mécanisme préexistant V11 (disponibilité du device, processus du jeu seulement si activé), et n'exécute **aucun geste tactile**, aucune demande/donation ou opération de gameplay. Si le profil est désactivé, si le worker est inconnu/hors ligne ou si une sonde est déjà ouverte, le serveur refuse la création.

Une nouvelle sonde créée depuis la console est dirigée vers le worker effectif (`target_worker`). Les sondes déjà en file conservent leur cible, de même que les baux en cours. Les autres clients du planificateur V9–V14 peuvent encore ajouter des sondes directement sans passer par cette politique : **l'affinité n'est pas une réécriture globale de tous les jobs existants**.

## API

Toutes les routes utilisent les gardes Guardian existants et les règles d'écriture CSRF :

| Route | Méthode | Fonction |
| --- | --- | --- |
| `/api/fleet/console/groups/{key}/policy` | GET/PUT | Lire ou modifier les préférences de groupe |
| `/api/fleet/console/profiles/{key}/detail` | GET | Voir les paramètres résolus et les 10 dernières sondes |
| `/api/fleet/console/profiles/{key}/override` | PUT | Définir ou réinitialiser les exceptions par profil |
| `/api/fleet/console/profiles/{key}/diagnostic` | POST | Ajouter une sonde technique `storage` ou `android` avec vérification du worker |

Les mutations utilisent des transactions locales SQLite ou un verrou consultatif PostgreSQL déjà employé par la console. Chaque changement de configuration est ajouté au journal d'audit V15 (`group_policy` ou `profile_override`). Les schémas sont **additifs** : deux tables supplémentaires, sans suppression des profils historiques. Les préférences sont résolues par jointure au lieu d'être recopiées des milliers de fois.

## Limites à ne pas confondre

- Les « profils » restent des enregistrements logiques de test, **pas des milliers de sessions Clash of Clans**.
- La fréquence de diagnostic est enregistrée mais **aucun déclencheur automatique basé sur cette fréquence** n'a été activé.
- Les profils peuvent hériter d'un worker, mais il n'existe **aucune orchestration de téléphones cloud** ou réservation matérielle garantie.
- Les sondes déjà attribuées ne sont pas annulées ou migrées lors de changements de préférence.
- Les données de diagnostic sont un état technique local et des tests autorisés, **pas une preuve de dons/demandes réalisés**.

La prochaine étape sera de mesurer la fiabilité et la contention par appareil réel, ajouter les contrôles de réservations matériels et convertir les préférences en une politique d'ordonnancement cohérente pour tous les clients, avec quotas et métriques de succès.
