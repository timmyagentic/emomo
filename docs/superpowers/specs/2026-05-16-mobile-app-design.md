# emomo Mobile App Design

Date: 2026-05-16

## Summary

Build a first-class iOS and Android app for emomo using Expo + React Native + TypeScript. The app is search-first, public, no-login, and focused on finding memes by describing a feeling or situation. It reuses the existing Go backend REST API and protobuf JSON contract, but owns its mobile UI, navigation, local state, and platform integrations.

## Product Decisions

- Platform: Expo managed React Native app under a new root-level `mobile/` subproject.
- Audience: public App Store / Google Play users.
- Access model: no login in v1.
- First screen: conversational AI search, not browse-first.
- Search mode: stream backend search progress where possible; fall back to non-streaming search if React Native streaming support is unavailable on a target runtime.
- Persistence: local search history only. No cloud favorites, user accounts, or cross-device sync in v1.
- Detail actions: share image, save image to local photo library, copy/open original image URL when available.
- Backend posture: do not embed any private API token, model key, object storage secret, or Qdrant credential in the app.

## Non-Goals

- No account system.
- No cloud favorites or cloud search history.
- No native iOS SwiftUI or Android Kotlin UI in v1.
- No direct mobile access to Qdrant, database, storage private APIs, or LLM providers.
- No rewrite of the existing web frontend.
- No separate SQL migration runner; backend database ownership remains in `backend/internal/repository/db.go`.

## Architecture

Add `mobile/` as a sibling of `backend/` and `frontend/`.

```text
mobile/
  app/                         Expo Router screens and route layout
  src/api/                     REST/protojson API client and SSE fallback adapter
  src/components/              Mobile UI components
  src/hooks/                   Search and local-history hooks
  src/storage/                 AsyncStorage-backed search history
  src/types/                   Mobile-owned view types
  src/utils/                   Image sharing/saving helpers
  gen/emomo/v1/                Generated protobuf TypeScript
```

The mobile app should generate protobuf TypeScript from `backend/proto/emomo/v1/` into `mobile/gen/`, matching the frontend convention that generated DTOs stop at the API boundary. React Native state and component props use mobile-owned projection types in `mobile/src/types/`.

The backend remains a Gin REST API. Protobuf continues to own request, response, and SSE payload shapes only.

## Mobile User Flow

1. User opens the app on the Search screen.
2. The top of the screen shows brand text, meme count from `GET /api/v1/stats`, a natural-language search composer, and local recent-search chips.
3. User submits a query.
4. App clears previous results, records the query locally, and starts `POST /api/v1/search/stream`.
5. While searching, a progress panel shows backend stages and streamed AI-thinking text in a compact mobile format.
6. On complete, results render as a two-column masonry-style list.
7. User taps a meme to open a full-screen detail route or modal.
8. Detail view shows the image, description, category/tags when present, and action buttons: share, save, copy/open source.
9. Saving requests platform photo-library permission only at action time.
10. Errors show inline retry affordances and never expose raw stack traces or provider details.

## Screens

### Search Home

- Primary screen and app entry.
- Shows search composer immediately.
- Shows recent searches stored on device.
- Shows initial browse fallback using `GET /api/v1/memes` or bundled curated examples if API is unavailable.
- Submitting a query transitions in-place to the results state; there is no separate marketing landing page.

### Search Results

- Same route as Search Home, with sticky compact composer at the top after results exist.
- Displays `SearchProgressView` while streaming.
- Uses a virtualized two-column list for performance.
- Supports canceling in-flight search.
- Supports retrying the last query after network or backend failure.

### Meme Detail

- Opens from any result tile.
- Uses image viewer layout optimized for one-handed mobile use.
- Provides buttons for share, save to photo library, and copy/open source URL.
- Shows metadata below the image without requiring scroll for the primary actions.

### Settings / About

- Minimal v1 settings reachable from the header.
- Contains clear search history, privacy note, app version, and backend status.
- No login, profile, subscriptions, or account controls.

## Components

- `SearchComposer`: query input, submit button, disabled/loading states.
- `RecentSearchChips`: local history chips and clear action entry.
- `SearchProgressPanel`: mobile presentation of backend `SearchProgressEvent`.
- `MemeMasonryList`: virtualized two-column result list.
- `MemeTile`: image tile with stable aspect ratio and short metadata.
- `MemeDetailView`: detail image, metadata, and platform actions.
- `ActionButton`: icon-first platform action control.
- `InlineState`: loading, empty, error, and retry states.

## Data Flow

The mobile API client mirrors the frontend's protobuf JSON pattern:

- Build `SearchRequest` using generated schemas.
- Encode with `toJson`.
- Decode `SearchResponse`, `ListMemesResponse`, `GetMemeResponse`, `GetStatsResponse`, and `SearchProgressEvent` with `fromJson`.
- Immediately project protobuf DTOs into mobile view types.

Search streaming should be implemented behind one function:

```ts
searchMemesStream(query, options, onProgress, signal)
```

If streaming is not supported in a runtime, the same function falls back to:

1. emit `query_expansion_start`;
2. call `POST /api/v1/search`;
3. emit `complete` with decoded results.

This keeps UI code independent from transport differences.

## Local Storage

Use AsyncStorage for search history:

- Key: `emomo.searchHistory.v1`
- Max entries: 20
- Deduplicate by normalized trimmed query.
- Newest first.
- Provide `addSearchHistory(query)`, `getSearchHistory()`, and `clearSearchHistory()`.
- Store only query text and timestamp.

No image files are cached as an app feature in v1. React Native image caching can rely on platform/runtime defaults.

## Public API Protection

Because v1 is public and no-login, the app must not ship a static private bearer token. Backend work should add:

- Configurable per-IP and per-route rate limiting.
- Conservative `top_k` cap for public clients.
- Request body size limit.
- Query length validation.
- Structured abuse-related log fields.
- Safe errors for public clients.
- Optional env-gated future app integrity hook for Apple App Attest / Google Play Integrity verification.

The first release can ship without full App Attest / Play Integrity enforcement if the backend has rate limits and cost caps. The API design should leave room for an `X-Emomo-App-Integrity` header later.

## Platform Permissions

- Photo library write permission is requested only when the user taps save.
- Sharing uses native share sheet.
- No push notifications, location, contacts, camera, microphone, or background tasks in v1.

## Testing

Mobile:

- Unit-test local search history normalization, ordering, dedupe, max-size trimming, and clearing.
- Unit-test API projection and streaming fallback behavior with mocked fetch responses.
- Smoke-test the app with Expo on iOS simulator and Android emulator when available.
- Run TypeScript and lint checks.

Backend:

- Unit-test rate limit middleware behavior.
- Unit-test public search request caps and validation.
- Run `cd backend && go test ./...`.

Frontend:

- Existing web frontend should not change for the mobile MVP. If generated proto changes occur, run `cd frontend && npm run gen && npm run build`.

## Release Preparation

- App name: `emomo`.
- Bundle/package IDs should be stable and configurable before store submission.
- API base URL is an Expo public config value, not a secret.
- Store submissions require screenshots, privacy disclosures, and notes that user search history is local-only.
- Google Play and App Store accounts are required before production submission.

## Acceptance Criteria

- `mobile/` can install, typecheck, lint, test, and run with Expo.
- App opens directly to the search experience.
- Search can call the existing backend, show progress, render results, cancel, retry, and fall back if streaming transport is unavailable.
- Tapping a result opens detail.
- Detail can share and save an image using platform APIs.
- Search history is local-only, deduped, capped at 20, and clearable.
- No private backend/model/storage token is shipped in the mobile app.
- Backend includes public API cost protection before public store release.
