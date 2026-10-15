> Current product version: **0.1.4**. See [version policy](#version-policy).

# Emomo 0.1.4 stable release

The installation entry point is a copyable prompt linking to https://github.com/timmyagentic/emomo/releases/download/v0.1.4/INSTALL.md. Agents first review the versioned instructions and execute only the scoped install, Skill setup and live verification rather than asking users to paste shell commands.

The current product is the Agent CLI + skill and deterministic search service. The official site is https://emomo.timmyagentic.si; API is https://api.timmyagentic.si/agent/v1; original images use https://images.timmyagentic.si. The emomo.net domain-sale site remains independent.

## Version policy

`cli/package.json` is the source of truth for the public Emomo product version. The current release is `0.1.4`; the next ordinary release is `0.1.5`, then `0.1.6`, and so on. Increment the final (patch) component for routine fixes and updates. Changing the minor or major component requires an explicit product/version decision. API `/v1`, JSON `schemaVersion`, catalog schemas and dependency versions are separate contracts, not product release numbers.

For every release, synchronize package version, website labels, README, installation prompt/instructions, asset filenames, URLs, release title/tag, checksums and source provenance. HTTP User-Agent derives from the package version. Mark the current stable release explicitly as GitHub Latest. Preserve published package bytes and tag targets; document-only corrections must record their source and update the corresponding checksums.

## Distribution

The dependency-free Node.js >=22.13 CLI is distributed as `timmyagentic-emomo-cli-0.1.4.tgz` on GitHub Release `v0.1.4`, with SHA-256 checksums. It is not published to the npm registry. Install the release URL with `npm install -g --ignore-scripts`. Skill instructions ship in that archive and at the website's `/SKILL.md`.

Access control, issuance and operational limits are specified in [ACCESS_CONTROL](ACCESS_CONTROL.md). Hosted verification requires an issued key; missing-key requests must be rejected.

## Production components

- `website/`: static assets on Worker `emomo-official`, custom domain `emomo.timmyagentic.si`.
- `deployments/cloudflare/agent-search/`: Worker `emomo-agent-search`, `api.timmyagentic.si`, dedicated D1 `emomo-agent-index`.
- R2 `emomo-reviewed-public`: only this release's reviewed static primary images. Keys are collection + SHA-256 + original format. Public access goes through the rate-limited image Worker; see [RATE_LIMIT_ROLLOUT](RATE_LIMIT_ROLLOUT.md). Existing private/legacy buckets are not exposed by the new domain.
- As of the 2026-10-15 gallery restoration, the public index holds 7,833 primary records: 7,803 reactions and 30 object stickers. The 645 duplicate versions remain in the local library and are excluded from public import. The 709 approved first-batch inputs are fully accounted for: 531 new primary assets, 154 aliases of those assets, and 24 aliases of existing public records. Static first-frame previews are marked in descriptions. Dataset updates do not change the CLI package version. Uncertain OCR is omitted, with visual tags retained. No signed URLs, local paths, source-chat provenance or credentials are published in the search DTOs.

Local SQLite and hosted D1 share `cli/src/keyword-search.js`: keyword groups must all match; literal matches take precedence, with curated aliases used only when no complete literal match exists, and BM25 ranks results with OCR/description/tag weights 4/1/2. Private catalogs may retain their own vocabulary. Existing catalog and production data stay unchanged; local legacy indexes are read through an in-memory index. The API does not support local-only subject/media/intent options. A score is not semantic confidence; Agents must inspect the image and tone. The zero-model claim applies to this dedicated service, not arbitrary custom API endpoints or retained legacy source.

## Build and deployment

Run CLI `npm run check && npm test`; run Worker `npm ci && npm run check`. The latter directly regenerates canonical DTOs using `protocol/` (no old frontend dependency), checks types, local real-workerd/D1 integration, installed CLI, outbound-request blocking, and deployment dry-run. Website needs no build. Inspect desktop and narrow layouts, installation links, copy feedback, full image rendering, docs and 404.

Private snapshot/export/bundles remain outside Git. `static:prepare` verifies hashes, excludes duplicate aliases and animations, suppresses uncertain OCR, and emits objects plus SQL; its `publicReleaseReady=false` is a provenance statement and does not assert redistribution clearance. Public release must have explicit user authorization; do not relabel image rights as cleared. Code MIT does not license image content.

CLI-only maintenance releases leave the existing public index and images in place; no reimport is needed. Gallery updates must additionally follow the cumulative coverage and incremental publication checks in [LIBRARY_IMPORT](LIBRARY_IMPORT.md#当前图库的增量发布检查). After gates pass, publish the exact reviewed commit/tag/package and INSTALL.md, SHA256SUMS.txt and source provenance, deploy changed website assets using the pinned Wrangler, then test a clean installation from the public release URL. Only deploy the API if its source or configuration changed. Verify stats, multiple real queries, detail, download SHA-256, old-route 410, browser-Origin 403 and invalid method/body behavior. Record cloud version IDs separately from source SHA, CI and release asset evidence.

The old backend/Hugging Face push workflow is manual-only. Do not restart model services or repurpose the legacy API gateway as a fallback. On failure, the new API reports the failure explicitly. For rollback, use a verified previous Worker version and matching database snapshot; rollback and future production changes require authorization.

Production search and original-image downloads independently enforce at least 1 second between admissions per IP using Durable Objects, with approximate per-location 30/minute bulk protection. R2 public custom domains are removed and r2.dev is disabled. Public burst/CLI/direct-origin acceptance is documented in [RATE_LIMIT_ROLLOUT](RATE_LIMIT_ROLLOUT.md). This is not a global bandwidth or billing cap, and clients sharing an egress IP share their allowance.

The CLI matrix exercises Node 22 and 24 on Linux, macOS and Windows. Record the actual successful jobs before claiming platform support. These tests do not validate legacy mobile device builds, the old frontend UI, private catalogs or semantic accuracy of every library image. Public acceptance uses a clean child environment, an explicitly configured issued access key, explicit public API URL and a temporary CLI configuration. SHA-256 checksums demonstrate byte agreement with this release; they are not a separate signature or independent proof of publisher identity.
