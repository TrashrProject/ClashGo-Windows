# REQ N GO V19 — premier test Android réel (MuMu / BlueStacks)

**Portée :** un seul émulateur et un compte de test autorisé. Cette livraison ne prouve pas le farming, la rotation des comptes ni une flotte de 50 ou 1 000 comptes. La suite GitHub Actions utilise des doubles de test : son statut vert n'atteste pas que Clash of Clans répond sur ton PC.

## Préparation sans écrire dans le jeu

1. Sur Windows, mettre à jour cette branche et lancer **un** émulateur compatible ADB. Ouvrir Clash of Clans manuellement, au premier plan, dans le village. Fermer les fenêtres et superpositions qui cachent le jeu.
2. Lancer `DEMARRER-ANDROID-STUDIO.bat`. Le script cherche `adb` dans le PATH puis dans quelques dossiers **existants** de MuMu et BlueStacks. Pour toute installation différente, définir `ADB_BINARY` avant le lancement.
3. Si `devices` ne montre aucun appareil en ligne, **ne pas saisir un serial inventé**. Dans le gestionnaire de l'émulateur, identifier **son propre port ADB**, effectuer une connexion explicite et relancer. Les ports dépendent de la version et de l'instance.
4. Copier exactement le serial retourné par ADB. Le nouveau `preflight` effectue uniquement `adb devices`, `pidof`, `dumpsys window` et **deux** `screencap` sans sauvegarder les PNG. Un jeu masqué, absent, un appareil hors ligne ou une résolution changeante bloquent l'essai.
5. L'interface locale `http://127.0.0.1:8766` apparaît après le diagnostic. Choisir **non** pour les gestes tactiles jusqu'à validation de la calibration.

### Diagnostic en ligne de commande

```powershell
python -m reqngo.android.cli devices
python -m reqngo.android.cli preflight --serial "SERIAL_AFFICHE_PAR_ADB"
```

Si ADB n'est pas dans le PATH, ajouter `--adb "CHEMIN_EXISTANT_VERS_ADB.exe"`. Ces commandes ne touchent pas l'écran du jeu.

## Ordre de qualification, sans raccourci

| Niveau | Test vérifiable | Critère d'acceptation | Ce que cela **ne** prouve pas |
|---|---|---|---|
| P0 | ADB + preflight | Serial sélectionné en ligne, jeu au premier plan, deux PNG de même résolution, aucune action | Détection du village |
| P1 | Calibration | Repères enregistrés sur deux **vrais** états visuellement distincts | Bouton correctement associé |
| P2 | Mode `Vérifier sans toucher` | Deux lectures consécutives de l'état initial ; aucun tap | Bonne transition |
| P3 | Un geste volontaire | Un unique tap vers la cible ; deux lectures de l'état d'arrivée ; journal mis à jour | Demande de troupes serveur confirmée |
| P4 | Parcours rapide | Deux à dix transitions préalablement calibrées, chacune vérifiée et chronométrée | Rotation de comptes et boucle de farming |
| P5 | Échec volontaire | Focus perdu ou état inconnu => **zéro nouveau geste**, erreur lisible, journal bloqué si le geste est incertain | Reprise multicomptes automatique |
| P6 | Cycle fonctionnel | Dans le jeu, demande effectivement envoyée et résultat observé (pas seulement bouton cliqué) | 50 sessions fiables |
| P7 | Répétabilité/performance | Plusieurs cycles **indépendants** instrumentés avec des métriques anonymisées | 1 000 comptes/24 h validés |

**Ne pas contourner la phase `armed`/`blocked` du journal en supprimant la base SQLite.** Le studio n'a pas encore de commande officielle pour repartir sur un nouveau cycle après un parcours achevé : ne pas présenter des cycles répétés comme disponibles en V19.

Pour un premier parcours, prévoir les véritables états : `village`, `clan`, `demande_possible`, `demande_confirmee`. Les noms sont indicatifs ; seuls les PNG du jeu font foi. La transition `demande_possible -> demande_confirmee` n'est pas validée tant que la demande effective n'a pas été observée dans le jeu.

## Erreurs à distinguer

- **ADB absent** : outil Android introuvable ; vérifier `ADB_BINARY` et les emplacements de l'installation.
- **Aucun appareil ADB en ligne** : émulateur éteint, ADB désactivé, port non connecté ou appareil `unauthorized`.
- **Jeu non lancé** : `pidof` n'a pas trouvé `com.supercell.clashofclans` ; ne pas automatiser un lancement ou une connexion.
- **Jeu non confirmé au premier plan** : affichage Android différent, superposition ou format `dumpsys` non reconnu ; le contrôle refuse le clic.
- **PNG incohérent** : impossible d'établir une calibration sûre.
- **État visuel inconnu** : recadrer un repère sur une zone stable et discriminante ; ne pas abaisser arbitrairement les seuils pour forcer une action.
- **Transaction ambiguë** : aucune répétition aveugle du geste ; vérifier l'état manuellement.

## Périmètre de sécurité et confidentialité

- Ne jamais charger `1601.zip`, des `shared_prefs`, des sessions Supercell ID ou des jetons issus de CA dans le dépôt.
- Ne pas exporter des captures contenant des noms de joueurs ou informations personnelles.
- Aucun processus de fond sur ton PC ni gestion 24/7 n'est mis en place par ce script.
- Le bot peut enfreindre les règles de Supercell et entraîner des sanctions de compte.

## Éléments nécessaires pour conclure

Relever localement, pour chaque transition, le type de l'étape, le succès/échec, les temps écoulés, l'état visuel avant/après et le statut du journal. Utiliser les métriques produites par V18 lorsque disponibles. Ne pas transmettre les sauvegardes d'identification ni les captures complètes non expurgées.

**Décision :** cette phase traite le raccordement réel et la sûreté d'un appareil. La vitesse et l'architecture 1 000 profils ne seront évaluées qu'après une démonstration répétable de la boucle de demande sur un profil autorisé.
