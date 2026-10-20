# Emomo website and account service

The static public site and the account API run on `emomo-official`. The existing 0.1.1 CLI accepts self-service keys without a client upgrade.

`/account` combines registration and login through a six-digit email code. A code expires after ten minutes, permits five attempts, and is consumed atomically. A successful verification creates a 14-day Secure, HttpOnly, SameSite=Lax host-only session. POST requests require the same Origin. Account JSON DTOs come from `backend/proto/emomo/v1/account.proto`; regenerate frontend/mobile/Go outputs, then run `node scripts/sync-proto.mjs` here.

Account storage is a SQLite Durable Object per pseudonymous account ID. `ACCOUNT_SECRET` is the stable HMAC identity/OTP secret; generate at least 32 random bytes and keep it outside the repository. **Do not rotate it without an account-ID migration**, because it maps email addresses to existing account objects. Store it as a Worker secret. Codes, session secrets and raw access keys are never logged. Only hashes are persisted for authentication secrets; the email address is stored for account access.

Each account can retain three active keys and create ten keys per UTC day. Raw keys are returned exactly once. All its keys share the existing search/read/download quotas through the API-owned Admission namespace; creating or revoking a key never resets usage. Key IDs beginning with `web-` are reserved. Existing operator-issued keys remain supported by the API/image secret registry.

Email sends use Cloudflare Email Service from `login@mail.timmyagentic.si`. Limits: one code per email per minute, five per hour and ten per day; each IP five per hour/twenty per day; global one hundred per hour/five hundred per day. An approximate 30/minute edge binding guards account API requests. These limits reduce abuse but do not establish one-account-per-person or an account-wide billing cap. No timer polls the server; the resend countdown is local UI state.

Run `npm ci && npm run check`. The repository's `node scripts/dev.mjs bootstrap agent` / `check agent` also include this service. Tests replace only email delivery with a local recorder; account, session, key and quota operations use real workerd Durable Objects. No test sends live email or provides a production authentication bypass.

Rollout requires explicit production authorization. Publish the website Worker first with `ACCOUNTS_ENABLED=false` to register Account, then API/image Workers with their `ACCOUNT_STORE` remote binding, then publish the website with the normal enabled configuration. The website binds back to the existing API-owned Admission for usage display. Keep manual registry secrets unchanged. Verify a real email code, browser session, key creation, CLI search/download, revocation, logout and missing-key denial. Never include a real code, session cookie or key in screenshots or task records.

Keep current CLI version/download links aligned with the published release; `public/SKILL.md` mirrors the packaged Skill. Validate desktop/mobile layouts, links, complete example images, and the real CLI flow. Static-only preview can use `python3 -m http.server 8765 --directory public`; authentication needs a local workerd fixture or the deployed service. Production assets provide clean `/docs` and `/account` URLs.


## timmyagentic visual system

The product website follows the approved timmyagentic main-site visual language: Instrument Sans and Noto Sans SC, paper/ink colors, blue emphasis, quiet image backgrounds, generous spacing, and a shared parent-brand navigation and footer. Product content, languages, existing links and JavaScript interactions stay owned by this website.

The self-hosted visual assets live under `brand/` in the static directory. Font licenses are included in `brand/fonts/`; Chinese subsets cover the current HTML/JavaScript copy (800 characters). Regenerate those subsets when adding new characters; system fallbacks remain configured. The ambient image is the same approved asset used on the main site. No third-party font request is made by visitors.

See `docs/website-design-qa.md` and `docs/website-brand-preview.jpg` for the validated design.
