# REQ N GO — MuMu Player et diagnostic de vitesse (V19)

Cette version ajoute un **lanceur MuMu** et une mesure de la latence de capture ADB **sans modifier CA.apk**, sans authentification de comptes et sans accès aux données Supercell ID. Elle s'ajoute au parcours visuellement vérifié V18.

## Sur Windows avec MuMu

1. Démarrer MuMu puis ouvrir Clash of Clans manuellement.
2. Dans le dossier de travail `ClashGo-Windows-Live`, double-cliquer **`DEMARRER-MUMU.bat`**.
3. Le lanceur cherche `adb.exe`/ `adb_server.exe` dans plusieurs emplacements courants de MuMu. Si le chemin de ton installation est différent, il demande le chemin complet de l'exécutable ADB.
4. Il demande ensuite le **port local ADB** de l'instance (Entrée pour essayer **7555** indiqué dans le manuel officiel MuMu classique). Le port peut être différent sous **MuMu Player 12** ou avec plusieurs instances. Le lanceur ne scanne pas ton réseau. En cas d'échec, retrouver le port dans les réglages/gestionnaire de ton MuMu puis relancer.
5. Lancer le Studio sur `http://127.0.0.1:8766`. Sélectionner explicitement le **serial** réellement affiché par ADB et activer les gestes uniquement quand tu veux tester tes propres actions.
6. Dans l'interface, cliquer sur **Mesurer la latence ADB**. Cinq captures d'écran sont chronométrées ; seuls les temps et la résolution sont retournés.
7. Le **Parcours rapide** de V18 enchaîne des transitions calibrées, vérifiées et bornées, et affiche maintenant le temps de l'étape la plus lente.

Les captures utilisées par la mesure ne sont **ni enregistrées ni envoyées au cloud**. La durée ADB n'est **pas** la durée d'une demande de troupes, d'un chargement de village ni d'un changement de compte. Pour prouver un gain contre CA.apk, chronométrer le même scénario complet (demande -> compte suivant prêt) sur la même instance MuMu, avec un compte de test autorisé.

## Chemin ADB personnalisé

Si MuMu est installé hors des dossiers détectés :

```bat
cd /d "C:\Users\grime\OneDrive\Desktop\ClashGo-Windows-Live"
set "ADB_BINARY=C:\DOSSIER_MUMU\CHEMIN_REEL\adb.exe"
DEMARRER-MUMU.bat
```

Remplacer le chemin fictif par le véritable emplacement. Le script peut également accepter un `adb_server.exe` compatible. MuMu classique documente `C:\Program Files (x86)\Nemu\vmonitor\bin\adb_server.exe`.

Source officielle : https://www.mumuplayer.com/fr/help/win/developers-essentials-manual.html

## Limites

- Aucun contournement des scripts protégés de CA.apk ni extraction d'identifiants provenant de `1601.zip`.
- Pas de preuve de vitesse réelle avant une mesure sur ton émulateur.
- Le studio existant refuse toute interaction si Clash of Clans n'est pas au premier plan, et le parcours V18 nécessite la calibration préalable de chaque écran/action.
- Les tests du dépôt utilisent un faux Android, pas ton MuMu.
- L'usage de bots non autorisés peut entraîner une sanction Supercell.

## Tests

```cmd
python -m unittest tests.test_android_benchmark tests.test_android_studio tests.test_android_studio_html -v
```
