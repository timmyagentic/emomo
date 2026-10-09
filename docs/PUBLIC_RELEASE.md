# Emomo 1.0 public release

The current product is the Agent CLI + skill and deterministic search service. The official site is https://timmyagentic.si; API is https://api.timmyagentic.si/agent/v1; original images use https://images.timmyagentic.si. The emomo.net domain-sale site remains independent.

## Distribution

The dependency-free Node.js >=22.13 CLI is distributed as `timmyagentic-emomo-cli-1.0.0.tgz` on GitHub Release `v1.0.0`, with SHA-256 checksums. It is not published to the npm registry. Install the release URL with `npm install -g --ignore-scripts`. Skill instructions ship in that archive and at the website's `/SKILL.md`.

## Production components

- `website/`: static assets on Worker `emomo-official`, custom domains `timmyagentic.si` and `www.timmyagentic.si`.
- `deployments/cloudflare/agent-search/`: Worker `emomo-agent-search`, `api.timmyagentic.si`, dedicated D1 `emomo-agent-index`.
- R2 `emomo-reviewed-public`: only this release's reviewed static primary images. Keys are collection + SHA-256 + original format; cache immutable. Existing private/legacy buckets are not exposed by the new domain.
- The public index holds 7,302 primary records. The 467 duplicate versions remain in the local library and are excluded from public import. Uncertain OCR is omitted, with visual tags retained. No signed URLs, local paths, source-chat provenance or credentials are published in the search DTOs.

Search is keyword FTS5/BM25 with OR recall, not model inference. Local curated facet/synonym matching is a different retrieval implementation. The API does not support local-only subject/media/intent options. A score is not semantic confidence; Agents must inspect the image and tone. The zero-model claim applies to this dedicated service, not arbitrary custom API endpoints or retained legacy source.

## Build and deployment

Run CLI `npm run check && npm test`; run Worker `npm ci && npm run check`. The latter checks generated DTO sync, types, local real-workerd/D1 integration, installed CLI, outbound-request blocking, and deployment dry-run. Website needs no build. Inspect desktop and narrow layouts, installation links, copy feedback, full image rendering, docs and 404.

Private snapshot/export/bundles remain outside Git. `static:prepare` verifies hashes, excludes duplicate aliases and animations, suppresses uncertain OCR, and emits objects plus SQL; its `publicReleaseReady=false` is a provenance statement and does not assert redistribution clearance. Public release must have explicit user authorization; do not relabel image rights as cleared. Code MIT does not license image content.

After authorization: upload images with byte/hash readback, apply D1 schema and import, deploy dedicated API and website using their pinned Wrangler, publish an exact reviewed commit/tag/package, then test a clean installation from the public release URL. Verify stats, multiple real queries, detail, download SHA-256, old-route 410, browser-Origin 403 and invalid method/body behavior. Record cloud version IDs separately from source SHA, CI and release asset evidence.

The old backend/Hugging Face push workflow is manual-only. Do not restart model services or repurpose the legacy API gateway as a fallback. On failure, the new API reports the failure explicitly. For rollback, use a verified previous Worker version and matching database snapshot; rollback and future production changes require authorization.

Rate limiting uses Cloudflare per-location, eventually consistent counters. It is not a global hard quota or billing cap. A production search test observed HTTP 429 with Retry-After at request 32 in one location; a very short GET burst did not hit the threshold. See [Cloudflare rate-limit accuracy](https://developers.cloudflare.com/workers/runtime-apis/bindings/rate-limit/#accuracy).
