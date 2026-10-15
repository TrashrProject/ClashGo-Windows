# Deployer AI Guardian sur Railway, depuis un telephone

**Statut : guide uniquement. Aucun service ni abonnement n'est cree par ce fichier.**
Il s'agit de trois pilotes techniques et d'un tableau de bord ; aucun compte Clash of Clans ou emulateur n'est connecte.

Railway ne lance pas directement les fichiers Docker Compose. La topologie actuelle correspond a **quatre services Railway distincts** relies au meme depot GitHub :

| Service | Source | Demarrage | Volume |
| --- | --- | --- | --- |
| `guardian` | `TrashrProject/ClashGo-Windows` (Dockerfile) | Commande par defaut du Dockerfile, port 8765 | `/data` |
| `pilot01` | meme depot, meme Dockerfile | `python -m reqngo.pilot_worker --id pilot_01 --profile 1` | `/data` |
| `pilot02` | meme depot, meme Dockerfile | `python -m reqngo.pilot_worker --id pilot_02 --profile 2` | `/data` |
| `pilot03` | meme depot, meme Dockerfile | `python -m reqngo.pilot_worker --id pilot_03 --profile 3` | `/data` |

## Variables de service

Sur `guardian` :
- `GUARDIAN_PASSWORD` : secret aleatoire unique de 16 caracteres minimum.
- `GUARDIAN_AGENT_TOKEN` : **autre** secret aleatoire, de 32 caracteres minimum.
- `GUARDIAN_DATA_DIR=/data`.
- `PORT=8765` ; le serveur ecoute actuellement sur le port fixe 8765.

Sur chaque pilote :
- `GUARDIAN_URL=http://guardian.railway.internal:8765`.
- `GUARDIAN_PRIVATE_RAILWAY=1` (autorise uniquement le domaine prive **exact** `guardian.railway.internal`).
- `GUARDIAN_AGENT_TOKEN` : **reference securisee** vers le meme secret que le serveur.
- `PILOT_DATA_DIR=/data`.

Ne jamais saisir de mots de passe ou jetons dans GitHub, une issue ou une discussion publique. Utiliser les variables privees de Railway.

## Acces depuis l'iPhone

- Declarer **un domaine HTTPS public uniquement sur le service `guardian`**, pas sur les trois pilotes.
- Acceder a la page Guardian avec l'utilisateur `admin` et le secret `GUARDIAN_PASSWORD`.
- Le tableau de bord utilise encore une authentification Basic ; pour une exposition internet durable, prevoir une protection complementaire en amont (controle d'acces, rate limiting), et surtout un mot de passe unique tres long.
- Ne pas publier d'API d'ingestion ou de diagnostic sans TLS et jetons.

## Stockage et couts : verifier avant tout lancement

Il faut un volume persistant **par service**, donc quatre volumes pour cette topologie. Selon la documentation Railway, les limites des plans peuvent rendre ce montage indisponible sur le plan gratuit ou l'essai. Les services sont factures selon leurs ressources et le stockage. Verifier les tarifs et limites actuels dans Railway avant approbation.

**Important :** les images de ce projet s'executent en utilisateur non-root, tandis que Railway indique que les volumes peuvent etre montes avec proprietaire root. Sa documentation recommande `RAILWAY_RUN_UID=0` lorsque les droits bloquent l'acces. Cette option fait tourner le processus en root ; examiner les risques et limiter les droits avant un usage sensible. Ne pas mettre de donnees privees de comptes de jeu dans ces services.

## Mise en place assistee

Une fois l'application Railway connectee et un projet autorise :
1. Creer les quatre services depuis le depot GitHub et leurs volumes respectifs.
2. Ajouter les variables et les references partagees sans communiquer leurs valeurs.
3. Deployer en premier `guardian`, puis les pilotes ; ils savent resynchroniser les evenements apres coupure.
4. Lire l'etat `/health/ready`, puis verifier les trois pilotes dans **Incidents > Diagnostics distants**.
5. Examiner facturation, alertes, sauvegardes et disponibilite avant de le laisser fonctionner en permanence.

### Limites techniques connues

- Les controles de sante detectent une panne de processus ou de bases, mais ne garantissent pas qu'une erreur sera toujours reparee.
- Un pilote inaccessible ne bloque pas les deux autres.
- Aucun mode d'IA capable de modifier puis deployer automatiquement du code n'est encore actif.
- Aucune machine n'est deja hebergee : le depot est seulement pret pour configurer ce scenario.
