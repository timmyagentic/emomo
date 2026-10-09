# Emomo Agent API Gateway

This Worker exposes a small retrieval API for the Emomo CLI and skill, while
keeping the Hugging Face backend private. `HF_TOKEN` stays in a Cloudflare secret
and is injected only into server-side upstream requests.

## Agent mode and paused service

The checked-in configuration uses `SERVICE_MODE=agent` and
`AGENT_API_ENABLED=false`. Deploying these defaults keeps searches paused.
Enabling the Agent API requires an authorized production activation and a
working private backend; local tests do not prove live library availability.

When enabled, these routes map to the backend's existing protojson API:

| Public route | Private upstream route |
| --- | --- |
| `POST /agent/v1/search` | `POST /api/v1/search` |
| `GET /agent/v1/memes/:id` | `GET /api/v1/memes/:id` |
| `GET /agent/v1/categories` | `GET /api/v1/categories` |
| `GET /agent/v1/stats` | `GET /api/v1/stats` |

Old `/api/v1` requests always return 410 in Agent mode, even with the Agent API
enabled. Root/admin pages, `/health`, bulk meme listing, and streaming search
are not exposed. The configured CORS allowlist is empty, so browser origins
receive no cross-origin access grant. CLI requests do not require CORS.

Only explicit `SERVICE_MODE=legacy` restores the previous web/mobile routing
behavior for a separately authorized operation. Missing or misspelled mode
variables retain the Agent boundary. Legacy mode is not used by this migration.

## Request bounds

- Search queries contain 1–160 characters. Candidates default to 8 and must be a
  positive integer no larger than `MAX_SEARCH_TOP_K` (100 by default).
- POST bodies are bounded by `MAX_REQUEST_BODY_BYTES` (65536), including bodies
  without `Content-Length`.
- Existing `EMOMO_RATE_LIMITER` settings allow 120 requests per 60 seconds per
  route family and `CF-Connecting-IP`.
- Stats and categories use the existing short-lived edge cache. Detailed meme
  retrieval and searches remain uncached.
- Unsupported methods and query parameters are rejected before upstream access.

## Local validation

```sh
cd deployments/cloudflare/api-gateway
npm ci
npm test
npm run typecheck
npm run deploy:dry-run
```

Tests cover the legacy guards, Agent-only routes, paused defaults, validation
and rate limiting, plus a real CLI process calling the Worker handler and a
local private-backend fixture. Fixtures are not real-library search evidence.

`wrangler types` generates `Env` from the config. The dry run builds the Worker
without deploying it. Do not resume the Hugging Face Space or publish a Worker
as part of local validation.

## Production activation

Follow [the repository activation checklist](../../../docs/AGENT_NATIVE.md)
after authorization. Restore and verify the private search backend first,
preserve the existing secret and rate-limit binding, and explicitly verify the
final mode variables: `SERVICE_MODE=agent`, `AGENT_API_ENABLED=true`.

The Custom Domain stays `api.emomo.net`. A live acceptance run must check
installed-CLI health, real semantic search, and a real image download, then
confirm that old `/api/v1` requests still return 410. Keep R2 image URLs
available to the CLI. Website, mobile app, and email services have independent
lifecycles.

Never put a Hugging Face token in source, config, CLI packages, task documents,
or frontend environment variables. Public npm publication and GitHub unarchive
are separate delivery actions.
