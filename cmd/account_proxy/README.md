# ClashGO Account Service

This service is the only component that holds the official Clash of Clans developer API key.
Desktop users only enter their player tag.

## Production environment

- `COC_API_KEY`: official Clash of Clans developer API token.
- `CLASHGO_ACCOUNT_LISTEN`: listen address, defaults to `:8787`.

Run this service behind HTTPS (Caddy, nginx, Cloudflare Tunnel, etc.) and set the desktop
environment variable `CLASHGO_ACCOUNT_API_URL=https://your-account-service.example` at build/deployment time.

The desktop executable must never embed `COC_API_KEY`.

## Docker

```bash
docker build -f cmd/account_proxy/Dockerfile -t clashgo-account .
docker run -d --restart unless-stopped \
  -e COC_API_KEY='YOUR_REAL_KEY' \
  -p 127.0.0.1:8787:8787 \
  --name clashgo-account clashgo-account
```

Health check:

```bash
curl http://127.0.0.1:8787/healthz
```

The public reverse proxy should expose only `/healthz` and `/v1/player/*`.
