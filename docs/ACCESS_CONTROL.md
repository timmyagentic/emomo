# Emomo 0.1.1 access and resource protection

The hosted search, detail, categories, stats and image services require an administrator-issued Emomo access key. Offline catalogs require no key. The CLI stores a key in a private file, binds it to an HTTPS API base, and sends the official API key only to that API and canonical HTTPS image paths on images.timmyagentic.si. Authenticated image redirects are rejected. No key appears in an image URL or response DTO.

## Admission order and quotas

Cloudflare WAF checks Bearer syntax and request shape before the Workers. A per-colocation 60 requests/10 seconds/IP rule guards the API/image path families. It is burst protection, not an exact global counter. The existing approximate 30/minute/IP Worker limiter remains.

The Worker verifies SHA-256 against its secret key registry, checks revocation/expiry, then applies the existing one-second/IP search or download interval. Only authenticated requests reach the per-key Durable Object. Search and download also have separate one-second/key intervals. A shared Admission class is owned by emomo-agent-search; the image Worker binds to that class remotely.

| Per-key operation | UTC calendar hour | UTC calendar day |
| --- | ---: | ---: |
| Search | 120 | 500 |
| Read: detail, categories, stats combined | 600 | 2,000 |
| Image GET or HEAD | 120 | 500 |

Counters are serialized and persisted per key/operation, independent of source IP or Worker isolate. Cached responses count. Failed requests after admission count; there is no refund. A CLI download resolves detail first, consuming one read and one image admission. Key policy overrides support smaller canary/test quotas. Reissuing a key uses a new ID; do not reuse IDs, which also identify counters.

Global limits use separate Durable Objects per operation:

| Operation | Per second | Per UTC day | Concurrent backend admissions |
| --- | ---: | ---: | ---: |
| Search | 10 | 5,000 | 10 |
| Read | 30 | 20,000 | 30 |
| Download | 10 | 5,000 | 10 |

These counters include cache hits and failures after key admission. Five backend failures within 30 seconds open that operation's circuit for 30 seconds. One subsequent probe may run; success closes it, failure reopens it. Backend leases expire after 30 seconds to recover crashed requests. Download leases cover R2 acquisition, not the client's entire response transfer; they are not a hard active-connection cap. Search request bodies are limited to 8 KiB and five seconds. Worker CPU limits are 100 ms for the API and 50 ms for images. Existing image size limit is 25 MiB. These controls bound admitted operations, not the Cloudflare account bill.

401 means missing, invalid, expired or revoked key. 429 means IP or key limit. 503 means missing policy/binding, paused service, global limit or open circuit. Admission responses are no-store; 429/503 include Retry-After where a retry time is known. WAF may return 403/429 before Worker JSON handling. Do not retry in a tight loop.

## Offline issuance and revocation

Run from deployments/cloudflare/agent-search. Use an operator-private directory outside any repository. Never paste keys into commands or output them into task evidence.

```sh
node scripts/access-keys.mjs create --store /private/emomo/registry.json --id client-unique-id --key-file /private/emomo/client.key
node scripts/access-keys.mjs list --store /private/emomo/registry.json
node scripts/access-keys.mjs revoke --store /private/emomo/registry.json --id client-unique-id
node scripts/access-keys.mjs export --store /private/emomo/registry.json --output /private/emomo/new-secrets.json
npx wrangler secret bulk /private/emomo/new-secrets.json
npx wrangler secret bulk /private/emomo/new-secrets.json --config wrangler.downloads.jsonc
```

Creation supports `--expires-days` and `--quota` (the latter sets all hour/day quotas, useful for canaries). The registry holds hashes, not raw keys. Key files, registry and exported secret files are mode 0600; do not commit them. Export refuses to overwrite an existing file. Every registry change must reach both Workers; until both secret updates complete, revocation is not complete. A lost key is revoked and replaced, never recovered from the registry.

Clients run `emomo auth configure --key-file /private/client.key`; `auth status` reports configuration without exposing the key. A saved local catalog remains selected. Use `--api-url https://api.timmyagentic.si/agent/v1` to explicitly verify hosted access.

## Rollout and operational controls

Deploy the Admission owner (API) before the image Worker; both require ACCESS_KEYS_JSON. Publish matching CLI/Skill and installation instructions. Apply policies/custom.json and policies/rate-limit.json to the zone only after reading current rules and preserving unrelated entries. Validate with the provider before applying; the rate expression uses only URI paths for the zone's Free-plan rule support. No bot challenge is placed in the CLI flow. Automatic Cloudflare DDoS protection remains enabled.

SERVICE_PAUSED=true fails closed before key/global admission. It must be set on both Workers to pause all data access. GLOBAL_LIMITS_JSON is versioned in each Wrangler configuration. Changes to keys, global limits, WAF or pause state are deliberate operator actions. Do not automatically deploy, rotate, pause or roll back from the CLI or a polling job.

Acceptance uses short-lived low-quota keys to verify all protected operations, missing/revoked access, and quota denial. Local workerd tests exercise circuit recovery, concurrency, global budgets, warm-cache authentication, actual D1 and R2, and real packaged CLI installation. Never deliberately overload production to prove a circuit. Record actual deployed version IDs, release checksums and live results separately from source/CI evidence.
