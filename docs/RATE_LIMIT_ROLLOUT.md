# Search and original-image protection

Production protection is active as of 2026-10-11. Search and original-image downloads independently enforce a 1,000 ms per-IP interval. The CLI 0.1.0 protocol and canonical image URLs are unchanged.

## Deployed state and acceptance

- Search Worker version: `091e966b-6d17-49ae-9755-f50684523091`.
- Image Worker version: `3b36bfb5-9c5d-4ec8-8ced-a6fca4349344`.
- `images.timmyagentic.si` is attached to the image Worker. The R2 bucket has no public custom domains; its managed r2.dev endpoint is disabled.
- Public tests admitted one of six simultaneous searches and one of six downloads; the remainder returned 429 with Retry-After: 1. Requests succeeded again after the interval. Cached queries, changing image keys and HEAD requests did not bypass the guard. Range returned 416.
- Released CLI search and full-image download passed canonical SHA-256 verification. Known-object r2.dev requests returned 401; unsigned S3 requests returned an Authorization error, without image bytes.
- The recorded public checks used the NRT edge. Independent second-egress/cross-region production tests remain unverified.

The steps below are the runbook for future redeployment or recovery, not pending work for the current release.

## Policy

- Each Cloudflare-provided client IP gets one search admission per 1,000 ms. A SQLite Durable Object persists the next admission time; requests from different Cloudflare locations reach the same object. Reservations use the object's clock and storage gates.
- A separate image Worker applies an independent 1,000 ms interval to GET and HEAD, across all image keys. Search then download is allowed immediately. Invalid image paths never access R2; valid-but-missing images and ranges consume an admission. Range is rejected; conditional headers do not bypass the guard.
- Both services retain the approximate per-location 30 requests/60 seconds bulk guard before the durable check. A rejection gives HTTP 429 and Retry-After (1 or 60 seconds); protection failures give 503. There is no automatic retry or unprotected fallback.
- Search cache lookup happens after admission. External responses use no-store; the successful search response is cached internally for 60 seconds. Downloads stream complete bytes from the R2 binding and do not use a public upstream or public CDN cache.
- Object names are SHA-256 of the trusted IP, not the raw IP. Idle state is deleted after about a minute. X-Forwarded-For, user agent and Referer cannot grant an exception. Cloudflare's own platform logging/privacy settings remain separate.
- Shared egress IPs share an allowance. Distributed clients with many IPs can still download; this is neither authentication nor a global bandwidth/billing cap. Already downloaded or client-cached public images cannot be revoked.

## Before cutover

Run `node scripts/dev.mjs check agent` from the repository root. It includes CLI tests, real-workerd concurrency tests, a packaged CLI searching D1 and downloading through the protected R2 Worker, type checking, and dry runs of both Workers. Inspect the website's two local example images. The website and documentation must ship together with protection, not ahead of it.

The pre-cutover inventory had one R2 custom domain, `images.timmyagentic.si`. After the completed cutover, `emomo-reviewed-public` has no public custom domains and its managed r2.dev endpoint remains disabled. Refresh that inventory immediately before writing; check for any additional public aliases. Do not touch legacy/private buckets or emomo.net.

## Authorized cutover

1. Record current search/website Worker versions, DNS/domain mappings, R2 custom-domain settings and cache rules. Keep verified source and rollback versions. Preserve the database and all original objects.
2. Deploy `emomo-image-downloads` with the bindings/migration from `wrangler.downloads.jsonc` but temporarily no routes. Keep workers.dev and preview URLs disabled. Confirm deployment success and R2/SQLite bindings before exposing it; production request behavior is still UNVERIFIED at this stage.
3. In a maintenance window, remove the R2 custom-domain attachment and attach that same hostname as the image Worker's custom domain using the committed config. DNS/domain propagation can cause a brief download outage. Do not keep a public R2 alias as a fallback. Keep the managed r2.dev endpoint disabled and close any additional public aliases on this bucket. If attachment fails, keep the bucket private and report the outage; do not automatically reopen its public origin.
4. Deploy the search Worker with its durable binding/migration, then the website with the local examples and updated policy. Purge the old image-host cache (use hostname-scoped purge if available; otherwise list and purge this bucket's exact previously published image URLs in batches). Inspect any cache rules/overrides that could serve images or API responses without executing their guards. Do not purge unrelated hostnames.
5. Verify the checks below from the public routes. Record exact source SHA, deployed Worker versions, inventory and live results separately. Local tests are not live production evidence.

## Live acceptance

- From one egress IP, issue concurrent searches: exactly one succeeds, the rest return 429 with Retry-After. Repeat using the same query to exercise cached results, and different queries to prove a shared interval. Wait at least a second and verify success again.
- Immediately after search, retrieve metadata and download one full image with the released CLI. Confirm bytes and SHA-256 against the public metadata. A second immediate download (including a different image or HEAD) must get 429. Wait and retry. Verify Range cannot serve bytes outside the guard.
- A second independently available egress IP should succeed during the first IP's interval. Cross-location checks are UNVERIFIED unless actually performed; do not manufacture another device identity or copy credentials.
- Re-read R2 public-domain state: no R2 custom domains and r2.dev disabled. Test the old managed endpoint and known aliases using a known object key; none may return image bytes. Confirm image hostname routes to the Worker. Test fresh requests, not a browser's prior local cache.
- Check old API routes remain 410 and browser Origin requests 403. Check both website examples render at desktop/mobile widths and the installation prompt still points to stable CLI 0.1.0.

If any guard or binding fails, keep the affected endpoint closed (503/outage) and repair forward. Reopening R2 public access or rolling back to unprotected versions requires explicit authorization and must be reported as loss of protection.
