# emomo Config Center Worker

Cloudflare Worker + Workers KV + Cloudflare Secrets Store configuration center
for the emomo backend.

## Shape

KV stores a document like:

```json
{
  "version": "2026-06-14T10:00:00Z",
  "config": {
    "database": {
      "url_secret": "DATABASE_URL"
    },
    "qdrant": {
      "api_key_secret": "QDRANT_API_KEY"
    },
    "search": {
      "query_expansion": {
        "enabled": true,
        "model": "qwen/qwen-2.5-vl-7b-instruct:free",
        "api_key_secret": "OPENAI_API_KEY",
        "base_url": "https://openrouter.ai/api/v1"
      }
    }
  }
}
```

The Worker resolves `*_secret` fields from Cloudflare Secrets Store before
returning the config to the backend. Raw sensitive fields are rejected on write.

## Deploy

```sh
cd deployments/config-center
cp wrangler.toml.example wrangler.toml
npx wrangler kv namespace create emomo_config
# Paste the KV namespace id into wrangler.toml.

npx wrangler secrets-store store list
# Create the secrets listed in wrangler.toml.example, then paste the store id
# into each [[secrets_store_secrets]] block.

npx wrangler secret put READ_TOKEN
npx wrangler secret put ADMIN_TOKEN
npx wrangler deploy
```

Then add these once in Hugging Face Space variables/secrets:

```dotenv
CONFIG_CENTER_ENABLED=true
CONFIG_CENTER_REQUIRED=true
CONFIG_CENTER_URL=https://your-worker.example.workers.dev/v1/config/emomo/production/emomo-api
CONFIG_CENTER_TOKEN=the-read-token
```

Publish local config:

```sh
cd backend
CONFIG_CENTER_URL=https://your-worker.example.workers.dev/v1/config/emomo/production/emomo-api \
CONFIG_CENTER_ADMIN_TOKEN=the-admin-token \
./scripts/publish-config-center.sh
```

Full docs: `docs/CONFIG_CENTER.md`.
