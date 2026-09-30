# ClashGO Control API — Cloudflare

Backend serverless pour les licences et le support ClashGO.

## Architecture

- Cloudflare Worker : API HTTPS
- Cloudflare D1 : licences, machines, incidents
- GitHub Pages : interface publique + Control Center
- ClashGO Windows : activation et remontée automatique des erreurs

## Secrets

Ne jamais committer les secrets.

Le Worker attend :

- `CLASHGO_ADMIN_KEY` : secret du propriétaire pour le Control Center
- `CLASHGO_WEB_ORIGIN` : origine autorisée pour le navigateur

## Première installation

1. Créer un compte Cloudflare gratuit.
2. Créer la base D1 :
   `npx wrangler d1 create clashgo-control`
3. Copier l'ID de base dans `wrangler.jsonc`.
4. Initialiser :
   `npm install`
   `npm run db:init:remote`
5. Ajouter le secret :
   `npx wrangler secret put CLASHGO_ADMIN_KEY`
6. Déployer :
   `npm run deploy`

Le Worker fournit ensuite une URL du type :
`https://clashgo-control-api.<compte>.workers.dev`

Cette URL devient la valeur de l'API ClashGO.
