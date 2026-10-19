# REQ N GO Android V2 — fonctions automatiques et limites de validation

## Ce que fait réellement le code

1. **Entrée par clan :** ouvrir la fiche d'un clan via un deep-link construit à partir d'un tag validé. Une autre voie conserve l'ouverture du profil par un geste calibré. L'adhésion elle-même passe par des gestes calibrés avec examen d'images ; les clubs fermés, les invitations et les modales inattendues arrêtent le cycle.
2. **Demande de renforts :** le service Android peut tenter un parcours prédéfini à travers le village, le profil, le chat et la fenêtre de demande. Avant tout geste : le jeu doit être au premier plan, la résolution identique à la calibration, l'image attendue reconnue **sur deux captures**, et la région du bouton stable. Après le geste : reconnaissance de l'état attendu **sur deux captures**. Une nouvelle capture est autorisée pendant une courte fenêtre, mais **aucun geste n'est répété**.
3. **Multicomptes :** file de 5 000 **profils logiques** au maximum, indépendante du nombre d'appareils. Un seul profil peut obtenir le bail local ; un profil non vérifié nécessite un changement de compte via les mécanismes officiels par son propriétaire, puis une confirmation explicite. Le code ne restaure aucun token, mot de passe, fichier privé ou session copiée.

## Non implémenté / non démontré

- La connexion automatique / authentification à plusieurs Supercell IDs **n'est pas implémentée**. Les profils logiques ne sont pas des sessions authentifiées.
- La boucle sans opérateur sur de multiples comptes et l'orchestration distribuée 24/7 **ne fonctionnent pas encore**.
- L'adhésion au clan n'a pas été validée en conditions réelles. L'interface peut demander d'autres interactions que les quatre étapes actuellement calibrables.
- Le résultat `COMPLETE_VISUAL` est un **résultat fondé sur les images**, pas une preuve du serveur Supercell. Les demandes rejetées, les cooldowns, un château plein et les erreurs réseau restent à caractériser.
- Le moteur de reconnaissance par similarité nécessite des captures réellement enregistrées sur MuMu avec l'interface courante du jeu. Ni les 5 images ni les 4 boutons ne sont livrés par défaut dans le code.
- Aucun accès distant à MuMu n'était disponible lors de la compilation. La CI utilise des tests Java synthétiques et l'assemblage Android, pas le jeu réel.

## Tests automatisés livrés

- Quatre étapes, transitions invalides, double envoi interdit, reprise après crash bloquée.
- Raccourci clan : l'ouverture de la fiche n'équivaut pas à adhésion confirmée.
- 5 000 profils logiques sérialisés sans sessions privées, double réservation interdite, bail expiré bloqué.
- Tests antérieurs : normalisation du tag de clan, journal d'action, comparateur d'écran global/local, diagnostic privé.
- GitHub Actions : JVM tests, Android debug APK, vérification du paquet et de la signature.

## Tests MuMu restant indispensables

1. Installation de l'APK debug sur un MuMu Android 11+.
2. Consentement aux capacités d'accessibilité sur le compte de test autorisé.
3. Capture de référence des cinq écrans et calibration des régions stables/boutons.
4. Ouverture du clan ouvert ciblé par deep-link (recherche par tag en secours) ; mesure du succès/échec.
5. Parcours : sans clan, déjà membre, clan sur invitation, clan fermé, rôle clan empêchant sortie.
6. Demande : disponible, cooldown, château plein, demande déjà envoyée, manque de réseau.
7. Deux captures avant/après, rotation et changements de résolution, arrêt d'urgence, redémarrage entre dispatch et confirmation.
8. Changement de deux comptes de test via connexion officielle, confirmation du profil actif, séparation des résultats entre profils.
9. Mesurer taux de succès réel, latence, faux positifs, cas de double demande. Une compilation CI réussie ne constitue pas cette preuve.

## Contraintes de sécurité

Les outils d'automatisation peuvent contrevenir aux conditions de Supercell et entraîner des sanctions. Ne pas utiliser des comptes dont on ne détient pas l'autorisation. Ne pas importer les ZIP de `shared_prefs`, tokens ou sessions observés dans CA.apk. Aucun contournement du login ni des règles d'adhésion aux clans n'est implémenté.
