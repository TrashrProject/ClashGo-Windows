# REQ N GO — parcours Android rapide (prototype V18)

## Pourquoi ne pas simplement rebadger CA.apk ?

CA.apk est un programme tiers. Nous pouvons prendre les fonctions *observables* comme référence (demandes de troupes, transition entre comptes, temps d'attente), mais **nous ne réutilisons pas ses scripts protégés, ses identifiants ni ses sessions Supercell ID**. Cette version est développée indépendamment dans REQ N GO.

## Ce qui fonctionne dans le code

Le Studio Android local (`http://127.0.0.1:8766`) propose désormais **Parcours rapide**, une séquence de **1 à 10 actions tactiles calibrées**, sur **un seul appareil explicitement choisi**, après confirmation `EXECUTER LE PARCOURS` et activation des gestes dans le lanceur Windows.

- Avant chaque action, l'état visuel est reconnu sur **deux captures consécutives**.
- Le geste ADB ne part que si Clash of Clans est au premier plan.
- Après chaque geste, l'état attendu doit être observé **deux fois** ; ensuite l'action suivante peut commencer **sans attente fixe supplémentaire**.
- Le processus se bloque sur état inconnu, perte de focus, ADB indisponible ou transition non vérifiée.
- Le journal SQLite empêche la réémission aveugle d'une action après crash ; chaque étape est effectuée au plus une fois.
- **Arrêter après ce geste** demande un arrêt après l'opération en cours, et jamais au milieu d'une commande tactile.
- Limite stricte : **10 gestes** par lancement volontaire et **90 secondes** de temps global indicatif. Une commande ADB individuelle peut encore attendre son propre délai d'expiration.
- Le rapport donne le temps total et le **temps de chaque transition**, pour identifier la plus lente. Aucun nom de compte ni capture d'écran n'est envoyé sur GitHub.

Le système ne gère **pas encore** les 50 comptes, leurs connexions ni la réouverture automatique du jeu : ces comportements demandent des états visuels réels, des gestes adaptés à la version de MuMu et un essai manuel sur un seul compte. Les tests CI simulés ne prouvent pas son fonctionnement réel sur MuMu.

## Essai sur MuMu

1. Démarrer MuMu et **Clash of Clans au premier plan**, sans superposition CA pouvant masquer le focus.
2. Activer le débogage ADB dans les paramètres MuMu si l'option existe. Le chemin vers ADB et le port de connexion varient selon la version de MuMu. Vérifier les appareils visibles avec l'outil ADB de l'émulateur ou les Android Platform Tools officiels, sans deviner le port.
3. Si l'ADB de MuMu ne se trouve pas dans le PATH, ouvrir un CMD et définir son chemin avant de lancer le BAT :

   ```bat
   cd /d "C:\Users\grime\OneDrive\Desktop\ClashGo-Windows-Live"
   set "ADB_BINARY=C:\CHEMIN_REEL_VERS_ADB\adb.exe"
   DEMARRER-ANDROID-STUDIO.bat
   ```

   Remplacer le chemin fictif par le chemin réel de l'exécutable ADB de **ton** installation. BlueStacks 5 est détecté automatiquement quand installé à son emplacement standard.
4. Choisir le serial ADB explicitement affiché et confirmer les gestes uniquement après avoir prévu un essai sur compte de test.
5. Dans le studio, **calibrer visuellement** chaque écran et son bouton réel : village → écran de clan → demande envoyée → écran de changement → nouvel écran (selon les véritables captures). Il faut plusieurs étapes si le chemin réel en comporte plusieurs.
6. Tester chaque transition séparément et vérifier qu'elle conduit au bon écran. Ensuite, utiliser **Parcours rapide**, au plus 10 actions, et lire le chronométrage.

Ne jamais charger `1601.zip` ou des données `shared_prefs` dans le dépôt. Le studio n'importe aucune session CA ou Supercell ID et n'extrait aucune clé.

## Mesure des gains

Comparer sur un même appareil et une même résolution :
- Le temps complet « demande envoyée → nouveau compte prêt » dans CA.apk.
- Le temps de la transition équivalente, **après calibration**, avec notre parcours rapide.

Une optimisation de temps ne peut être revendiquée qu'après un test comparatif sur MuMu. Les performances sont souvent dominées par les chargements et transitions du jeu et de l'authentification, pas par la vitesse des clics.

## Tests

```powershell
python -m unittest tests.test_android_studio tests.test_android_studio_html tests.test_android_vision tests.test_android_runner -v
```

Cette implémentation reste un outil de test local : **pas encore un bot autonome multicomptes**, pas de récupération des sessions d'un tiers, pas de contournement anti-bot. L'automatisation peut enfreindre les règles de Supercell et entraîner des sanctions de comptes.
