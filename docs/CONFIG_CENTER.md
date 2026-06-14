# Runtime Config Center

emomo can use a Cloudflare Worker + Workers KV + Cloudflare Secrets Store as
the backend configuration center.

## Strategy

- Hugging Face keeps only bootstrap config:
  `CONFIG_CENTER_ENABLED`, `CONFIG_CENTER_REQUIRED`, `CONFIG_CENTER_URL`,
  `CONFIG_CENTER_TOKEN`, polling interval, and timeout.
- The API loads local YAML/env first, then fetches the Worker config and lets
  the Worker config override local and Hugging Face env values.
- During migration, legacy Hugging Face variables can stay in place. When the
  same key exists in both places, the config center value wins.
- Workers KV stores non-secret config and Secrets Store binding names.
- Cloudflare Secrets Store stores high-sensitivity values.
- The Worker resolves `*_secret` bindings only for the high-sensitivity
  allowlist before returning config to the backend.

## Enabling and Disabling

- `CONFIG_CENTER_ENABLED` is the only switch. When it is `false`, the backend
  never fetches or polls the config center, even if `CONFIG_CENTER_URL` is set.
  To turn the config center off, set `CONFIG_CENTER_ENABLED=false`; you do not
  need to clear the URL.
- When `CONFIG_CENTER_ENABLED=true` but `CONFIG_CENTER_URL` is empty, the
  backend fails to start.
- `CONFIG_CENTER_REQUIRED` controls how a failed fetch is handled:
  - `true`: startup aborts if the config center cannot be loaded (fail-closed).
  - `false`: startup continues with local YAML/env config and the backend logs
    a warning (`Config center enabled but optional fetch failed`) so the
    fallback is visible instead of silent.
- Production should set `CONFIG_CENTER_REQUIRED=true` so a config center outage
  never silently falls back to stale local config.

## Config Tiers

Startup-applied config, requires restart after changes:

- `server`
- `database`
- `qdrant`
- `storage`
- `vlm`
- `embeddings`
- `ingest`
- `sources`
- `search` except the runtime hot fields below
- `logging`

Runtime hot config, applied on the next poll:

- `vlm.api_key`
- `vlm.base_url`
- `search.query_expansion.enabled`
- `search.query_expansion.model`
- `search.query_expansion.api_key`
- `search.query_expansion.base_url`

Bootstrap exception:

- `config_center.token` is not loaded from the config center because it is
  required before the backend can read the config center.
- The other `CONFIG_CENTER_*` bootstrap values also remain local/Hugging Face
  because they are needed before any remote config can be fetched.

## High-Sensitivity Fields

These fields must be stored as Secrets Store references in KV:

- `config.database.url`
- `config.database.password`
- `config.qdrant.api_key`
- `config.storage.access_key`
- `config.storage.secret_key`
- `config.vlm.api_key`
- `config.embeddings.*.api_key`
- `config.search.query_expansion.api_key`
- `config.logging.loki_password`

The Worker rejects raw values for those paths, and it also rejects `*_secret`
references outside this allowlist. It also rejects raw secret-like field names
such as `api_key`, `access_key`, `secret_key`, `password`, and `token` when
they are not on the allowlist. Add a new path to the Worker allowlist before
adding any new API key field. Use sibling `*_secret` fields instead:

```json
{
  "config": {
    "database": {
      "url_secret": "DATABASE_URL"
    },
    "qdrant": {
      "api_key_secret": "QDRANT_API_KEY"
    },
    "search": {
      "query_expansion": {
        "api_key_secret": "OPENAI_API_KEY"
      }
    }
  }
}
```

## Deploy

```sh
cd deployments/config-center
cp wrangler.toml.example wrangler.toml
npx wrangler kv namespace create emomo_config
```

Paste the KV namespace id into `wrangler.toml`.

Create read/write tokens:

```sh
npx wrangler secret put READ_TOKEN
npx wrangler secret put ADMIN_TOKEN
```

Create Secrets Store values and bind them in `wrangler.toml`:

```sh
npx wrangler secrets-store store list
# If needed:
# npx wrangler secrets-store store create emomo --remote
npx wrangler secrets-store secret create <STORE_ID> --name DATABASE_URL --scopes workers --remote
npx wrangler secrets-store secret create <STORE_ID> --name DATABASE_PASSWORD --scopes workers --remote
npx wrangler secrets-store secret create <STORE_ID> --name QDRANT_API_KEY --scopes workers --remote
npx wrangler secrets-store secret create <STORE_ID> --name STORAGE_ACCESS_KEY --scopes workers --remote
npx wrangler secrets-store secret create <STORE_ID> --name STORAGE_SECRET_KEY --scopes workers --remote
npx wrangler secrets-store secret create <STORE_ID> --name OPENAI_API_KEY --scopes workers --remote
npx wrangler secrets-store secret create <STORE_ID> --name SILICONFLOW_API_KEY --scopes workers --remote
npx wrangler secrets-store secret create <STORE_ID> --name LOKI_PASSWORD --scopes workers --remote
npx wrangler deploy
```

## Configure Hugging Face Once

```dotenv
CONFIG_CENTER_ENABLED=true
CONFIG_CENTER_REQUIRED=true
CONFIG_CENTER_URL=https://your-worker.example.workers.dev/v1/config/emomo/production/emomo-api
CONFIG_CENTER_TOKEN=the-read-token
CONFIG_CENTER_POLL_INTERVAL=60s
CONFIG_CENTER_TIMEOUT=5s
```

## Publish Local Config

In `backend/.env`, keep local raw values for development and add the matching
Secrets Store binding names:

```dotenv
DATABASE_URL=postgresql://...
DATABASE_URL_SECRET=DATABASE_URL
QDRANT_API_KEY=...
QDRANT_API_KEY_SECRET=QDRANT_API_KEY
STORAGE_ACCESS_KEY=...
STORAGE_ACCESS_KEY_SECRET=STORAGE_ACCESS_KEY
STORAGE_SECRET_KEY=...
STORAGE_SECRET_KEY_SECRET=STORAGE_SECRET_KEY
OPENAI_API_KEY=...
OPENAI_API_KEY_SECRET=OPENAI_API_KEY
SILICONFLOW_API_KEY=...
SILICONFLOW_API_KEY_SECRET=SILICONFLOW_API_KEY
LOKI_PASSWORD=...
LOKI_PASSWORD_SECRET=LOKI_PASSWORD
```

Preview:

```sh
cd backend
CONFIG_CENTER_URL=https://your-worker.example.workers.dev/v1/config/emomo/production/emomo-api \
CONFIG_CENTER_ADMIN_TOKEN=the-admin-token \
./scripts/publish-config-center.sh --dry-run
```

Publish:

```sh
CONFIG_CENTER_URL=https://your-worker.example.workers.dev/v1/config/emomo/production/emomo-api \
CONFIG_CENTER_ADMIN_TOKEN=the-admin-token \
./scripts/publish-config-center.sh
```

The script publishes the complete effective backend config. If it finds a raw
secret without a matching `*_SECRET` binding env var, it fails instead of
publishing.

## Security Notes

- `CONFIG_CENTER_TOKEN` is read-only, but it can read resolved secret values.
  Treat it as sensitive.
- `CONFIG_CENTER_ADMIN_TOKEN` should only exist locally or in trusted CI.
- KV stores binding names, not raw secrets.
- Secrets Store is the source of truth for high-sensitivity values.
- Secret references are allowlisted by config path; arbitrary `*_secret`
  fields are not resolved, and unallowlisted raw API key-like fields are
  rejected.
- The backend receives raw secrets at runtime because it must connect to the
  actual providers; do not log config payloads.
