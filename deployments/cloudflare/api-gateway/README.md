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

## Anti-crawling Controls

The gateway is the public boundary for web and mobile clients. It intentionally
does not expose an unbounded meme catalog export:

- `GET /api/v1/memes` is limited to the first `MAX_LIST_WINDOW` items
  (`120` by default), so callers cannot page through the entire catalog by
  increasing `offset`.
- Search requests are capped by `MAX_SEARCH_TOP_K` (`100` by default).
- POST bodies are read and rejected at `MAX_REQUEST_BODY_BYTES`, even when the
  client omits `Content-Length`.
- All public data routes use the `EMOMO_RATE_LIMITER` Cloudflare Rate Limiting
  binding. The default `wrangler.jsonc` setting allows 120 requests per 60
  seconds per route family and `CF-Connecting-IP`.

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
npm test
npm run typecheck
npm run check
curl https://api.emomo.net/api/v1/stats
```

After deployment, mobile production builds should use:

```text
EXPO_PUBLIC_API_BASE=https://api.emomo.net/api/v1
```
