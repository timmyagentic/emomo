# emomo Mobile App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first public, no-login Expo React Native app for emomo, with search-first conversational AI search, masonry results, detail actions, local search history, and backend public API safeguards.

**Architecture:** Add a root-level `mobile/` Expo TypeScript app that talks only to the existing Go REST API through protobuf JSON DTOs generated from `backend/proto`. Keep mobile UI state in mobile-owned types, and add backend middleware/validation for public API cost control before store release.

**Tech Stack:** Expo, React Native, TypeScript, @bufbuild/protobuf, AsyncStorage, Expo MediaLibrary/Sharing/FileSystem/Clipboard, Go/Gin middleware, Go unit tests, Jest unit tests.

---

### Task 1: Backend Public API Guardrails

**Files:**
- Create: `backend/internal/api/middleware/public_guard.go`
- Create: `backend/internal/api/middleware/public_guard_test.go`
- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/config/config_test.go`
- Modify: `backend/internal/api/router.go`
- Modify: `backend/internal/api/handler/search.go`
- Modify: `backend/internal/api/handler/search_test.go`
- Modify: `backend/internal/api/handler/meme.go`

- [ ] Add config fields for public API rate limiting, request body size, maximum search `top_k`, maximum query length, and list page size.
- [ ] Write failing middleware tests for body-size rejection and per-client rate limiting.
- [ ] Implement in-memory public guard middleware using client IP plus route bucket keys.
- [ ] Add search validation tests for trimming, query length, and `top_k` capping.
- [ ] Add list validation for public `limit` capping and negative `offset` normalization.
- [ ] Wire guard middleware into `/api/v1`.
- [ ] Run `cd backend && go test ./...`.

### Task 2: Mobile Scaffold and Generated Protobuf

**Files:**
- Create: `mobile/` Expo app scaffold.
- Create: `mobile/AGENTS.md`
- Create: `mobile/buf.gen.yaml`
- Modify: `.gitattributes`
- Modify: `.gitignore`

- [ ] Generate an Expo TypeScript app under `mobile/`.
- [ ] Install mobile dependencies aligned with Expo.
- [ ] Configure TypeScript alias `@gen/*` to `mobile/gen/*`.
- [ ] Add protobuf generation using `../backend/proto`.
- [ ] Generate `mobile/gen/emomo/v1/*.ts`.
- [ ] Mark `mobile/gen/**` as linguist-generated.

### Task 3: Mobile Data Layer

**Files:**
- Create: `mobile/src/types/index.ts`
- Create: `mobile/src/storage/searchHistory.ts`
- Create: `mobile/src/storage/searchHistory.test.ts`
- Create: `mobile/src/api/index.ts`
- Create: `mobile/src/api/index.test.ts`

- [ ] Write failing tests for local search history normalization, dedupe, newest-first order, cap at 20, and clearing.
- [ ] Implement AsyncStorage-backed search history.
- [ ] Write failing tests for protobuf projection and non-streaming fallback progress events.
- [ ] Implement mobile API client for stats, memes, search, streaming fallback, and progress projection.
- [ ] Run mobile unit tests.

### Task 4: Mobile UI and Platform Actions

**Files:**
- Create: `mobile/src/components/*`
- Create: `mobile/src/utils/imageActions.ts`
- Modify: `mobile/App.tsx`
- Modify: `mobile/app.json`

- [ ] Build `SearchComposer`, `RecentSearchChips`, `SearchProgressPanel`, `MemeMasonryList`, `MemeTile`, `MemeDetailModal`, `ActionButton`, and `InlineState`.
- [ ] Build app state flow: load stats/feed, submit search, stream progress, cancel, retry, open detail.
- [ ] Implement native sharing, photo-library save, clipboard copy, and external URL open actions.
- [ ] Add responsive mobile styling with stable tile dimensions and no marketing landing page.
- [ ] Run typecheck/lint/build checks available in `mobile/`.

### Task 5: Docs and Developer Entry Points

**Files:**
- Modify: `README.md`
- Modify: `AGENTS.md`
- Modify: `scripts/start.sh` if needed.

- [ ] Document `mobile/` in the root project structure.
- [ ] Document mobile install, generate, test, and run commands.
- [ ] Avoid changing existing backend/frontend commands unless needed.

### Task 6: Verification

**Commands:**
- `cd backend && go test ./...`
- `cd mobile && npm test -- --runInBand`
- `cd mobile && npm run typecheck`
- `cd mobile && npm run lint`
- `cd frontend && npm run lint && npm run build`

- [ ] Confirm no private API/model/storage token is present in mobile code or config.
- [ ] Confirm generated protobuf compiles.
- [ ] Start Expo locally and report the URL/QR target.
- [ ] Summarize any environment-specific blockers, especially simulator availability.
