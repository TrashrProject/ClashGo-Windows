# REQ N GO Manager — AI Guardian (pilote)

Nouveau socle de supervision du projet. **Ce code n'est PAS un bot de farming ni une automatisation Clash of Clans.** Il fournit un tableau de bord, 50 profils fictifs, 3 comptes pilotes, un moteur de diagnostic et des reprises simulées. La connexion au jeu et aux comptes réels n'est pas implémentée.

## Fonctions disponibles
- 50 profils préparés, simulation de 3 pilotes par défaut (extensible à 150 dans une version future).
- Paramètres persistants : contrôle périodique (600 s), seuils de reprise, mode pilote.
- Incidents isolés par profil ; panne connue simulée → tentative limitée ; panne inconnue → escalade.
- Journaux consultables et export JSON sans identifiants ; SQLite local.
- Tableau de bord responsive sobre ; aucune statistique fictive présentée comme réelle.
- API sécurisée par un mot de passe et en-tête anti-CSRF pour les changements.

## Exécuter en local
Python 3.11+ :
\`\`\`bash
python -m pip install -r requirements.txt
# Configurer GUARDIAN_PASSWORD avec un secret long et unique
python -m uvicorn reqngo.server:app --host 127.0.0.1 --port 8765
\`\`\`
Ouvrir http://127.0.0.1:8765 ; identifiant \`admin\` et mot de passe GUARDIAN_PASSWORD.

Sur Windows, lancer \`DEMARRER-SUR-WINDOWS.bat\` : il génère au premier démarrage un secret local et installe les dépendances.

Tests : \`python -m unittest discover -s tests -v\`.

## Hébergement
Le service n'est **pas encore déployé**. Pour un hébergement 24/7, fournir un stockage persistant, une protection HTTPS/VPN, des sauvegardes et un superviseur de processus. Ne pas exposer directement le port 8765 sur Internet.

## Politique AI Guardian
- Détecter, isoler, journaliser puis réparer les incidents **reconnus et autorisés**, avec plafond de tentatives.
- Si le diagnostic est incertain : aucune commande arbitraire, escalade documentée.
- Les futures analyses IA de code devront travailler sur branches de test, avec validation et rollback ; pas d'accès aux mots de passe.
- Cette branche remplace le code applicatif dans un arbre Git propre. Le projet de farming d'origine est conservé dans \`archive/clashgo-before-reqngo-20261015\`.
- Les automatisations de gameplay peuvent enfreindre les conditions de Supercell et entraîner le bannissement de comptes.

## Organisation
\`reqngo/core.py\` : moteur SQLite indépendant. \`reqngo/server.py\` : API web et superviseur. \`reqngo/static/index.html\` : dashboard. \`tests/\` : scénarios automatisés.

## État
V1 pilote : **pas de comptes réels, pas d'émulateur, pas de cloud et pas d'IA capable de modifier le code seule**.