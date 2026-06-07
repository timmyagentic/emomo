# emomo Cloudflare API Gateway

This Worker exposes the public API surface used by the emomo web/mobile clients
while keeping the Hugging Face Space private. The Worker stores the Hugging Face
token as a Cloudflare secret and injects it only on server-side upstream
requests.

## Public Endpoint

- `https://api.emomo.net/api/v1`

Allowed upstream routes:

- `GET /health`
- `GET /api/v1/stats`
- `GET /api/v1/categories`
- `GET /api/v1/memes?limit=&offset=&category=`
- `GET /api/v1/memes/:id`
- `POST /api/v1/search`
- `POST /api/v1/search/stream`

All other paths, methods, and unsupported query parameters are rejected at the
gateway.

## Setup

```bash
cd deployments/cloudflare/api-gateway
npm install
npx wrangler login
npm run secret:put:hf-token
npm run deploy
```

Use a rotated Hugging Face token. Do not paste a token into source code,
`wrangler.jsonc`, Expo config, or any `EXPO_PUBLIC_*` / `VITE_*` variable.

The route in `wrangler.jsonc` uses a Cloudflare Workers Custom Domain for
`api.emomo.net`. The `emomo.net` zone must be active in Cloudflare, and
`api.emomo.net` must not already have a conflicting CNAME record.

## Validation

```bash
npm run typecheck
npm run check
curl https://api.emomo.net/api/v1/stats
```

After deployment, mobile production builds should use:

```text
EXPO_PUBLIC_API_BASE=https://api.emomo.net/api/v1
```
