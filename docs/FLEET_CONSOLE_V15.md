# REQ N GO Fleet Console — V15 UI

Le but de cette version est de **ne pas afficher ni configurer des milliers de profils un par un**. Le nouvel écran est disponible sur `/fleet`, via le lien ajouté dans l'ancien Guardian. L'ancien panneau `/` reste disponible, sans suppression ni régression volontaire.

## Fonctionnement réel

- **Vue générale** : compteurs réels des profils logiques, workers, incidents et capacité, avec les groupes et leur proportion de profils activés.
- **Profils** : recherche par préfixe effectuée par le serveur, filtres de groupe et d'activation, **50 lignes au maximum par page**, sélection explicite de **200 profils au maximum**.
- **Groupes** : noms/descriptions persistants, ajout de profils sélectionnés à un groupe ou retour au groupe indéfini. Un profil appartient à un seul groupe.
- **Gestion groupée** : prévisualisation du nombre de profils modifiés dans **un groupe choisi**, puis demande de recopier `SUSPENDRE nom-du-groupe` ou `ACTIVER nom-du-groupe`. Si le nombre a changé entre prévisualisation et validation, le serveur refuse l'action.
- **Traçabilité** : nombre de profils affectés, nom de groupe, action et date sauvegardés dans un journal technique consultable.
- **Workers et incidents** : tableaux séparés affichant l'état déjà collecté par V12/V14 et la dernière mesure connue. Aucune fausse statistique n'est générée.
- **Import de test** : création de 1 à 10 000 **profils logiques fictifs** par lot depuis la console. Ce n'est pas une création de comptes de jeu.

## Protection des opérations

Les routes `/api/fleet/console/*` sont protégées par les gardes de lecture et d'écriture de Guardian. Les écritures continuent d'exiger `X-Guardian-Action: 1`, l'authentification administrateur et le contrôle d'origine. Les paramètres de recherche sont validés et liés aux requêtes SQL; les noms et messages de l'interface sont injectés exclusivement comme texte (jamais comme HTML renvoyé par l'API).

La nouvelle console ne dispose **d'aucune route de gameplay** ou de commande à distance. Suspendre un profil l'empêche de recevoir **de nouveaux baux**, mais ne peut ni annuler ni interrompre de façon sûre une opération déjà en cours; les baux existants sont laissés inchangés.

La création des groupes ajoute deux tables et une colonne nullable `group_key` à `fleet_profiles`. Les colonnes historiques, les anciens profils, les workers et les sondes ne sont pas supprimés. SQLite fonctionne sur une machine; PostgreSQL sert pour les instances distribuées.

## Accès et navigation

Démarrer Guardian comme d'habitude, avec un mot de passe administrateur correctement configuré. Ouvrir `http://127.0.0.1:8765/fleet` ou utiliser le lien **Ouvrir Fleet Manager** dans le tableau de bord d'origine.

La console est en français, responsive, sombre et construite en HTML/CSS/JavaScript natifs. Ce n'est ni une capture d'écran ni un faux écran statique. Chaque carte et tableau lit les API réelles. L'interface actualise la vue active toutes les 30 secondes quand elle est visible.

## Limites actuelles

- Les 50 anciens profils Guardian et les milliers de profils logiques Fleet ne sont **pas encore fusionnés**. L'ancienne gestion des simulations reste indépendante.
- Les groupes n'ont pas encore de configurations héritées ou de règles opérationnelles; la V15 implémente l'organisation et les actions d'activation **des seuls profils fictifs**.
- Les groupes n'ont pas encore de gestion complète des droits utilisateurs, d'étiquettes arbitraires ni de liste d'appareils attribués.
- Les workers de diagnostic possèdent des métriques techniques, mais la console ne valide aucune automatisation Clash of Clans.
- La recherche par préfixe et la pagination évitent de télécharger l'ensemble de l'inventaire. Une base gigantesque nécessitera à terme des indexes adaptés et une pagination par curseur pour naviguer très loin.
- Les journaux n'ont pas encore de politique de rétention. Les migrations ne sont pas encore versionnées indépendamment de l'initialisation du backend.
- La validation des actions repose sur les identifiants de groupe et sur les nombres prévisualisés; une authentification multi-administrateurs et une séparation des rôles seront nécessaires avant un déploiement public.

## Tests

`python -m unittest discover -s tests -v`

Les tests vérifient les groupes persistants, les confirmations obligatoires, l'inventaire paginé, la gestion des sélections, les opérations atomiques et l'écriture du journal. Le workflow PostgreSQL teste aussi la gestion de groupes depuis deux clients de base distincts.

Aucun nouveau serveur payant n'est déclenché par cette version.
