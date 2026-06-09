---
name: emomo-product-doc-updater
description: Use when creating or updating emomo user-facing product documentation from the current commit, especially when screenshots, simulator checks, CDP/browser inspection, or ongoing feature records are needed.
---

# emomo Product Doc Updater

## Purpose

Create or update `docs/product-user-guide.md` from the product that actually runs in the current checkout. The output is user-facing Chinese copy plus evidence screenshots and a concise maintenance record.

## Standard Flow

1. Confirm repo state: read root/subproject `AGENTS.md`, record `git rev-parse --short HEAD`, and keep unrelated dirty files untouched.
2. Inspect product surfaces: read `frontend/src/App.tsx`, key `frontend/src/components/*`, `mobile/App.tsx`, and key `mobile/src/components/*` before writing.
3. Run the app:
   - Web: start Vite with an explicit `VITE_API_BASE`, then open it through the Browser plugin or Playwright/CDP.
   - iOS: prefer Expo Go with `EXPO_PUBLIC_API_BASE=... npx expo start --ios --go --localhost --port <port>`; use XcodeBuildMCP for simulator snapshots/screenshots.
4. Capture evidence under `docs/assets/product-guide/<commit>/`: home, search/results or error, detail modal, responsive/narrow layout, and simulator state when available.
5. Write/update `docs/product-user-guide.md`: explain pages, buttons, actions, empty/loading/error states, and why each feature helps. Keep technical reasons in “维护记录”, not in the user flow.
6. Verify: open screenshots, run relevant checks (`mobile` test/typecheck/lint when mobile is covered; frontend build/test when web behavior changed), inspect `git status`, and stop dev servers.

## Current emomo Pitfalls

- Do not assume the public API works in the local browser. If current UI falls back to examples or errors, document that as user-visible behavior and record the technical cause only in maintenance notes.
- Do not expose or print secrets from `backend/.env`.
- Do not use screenshots blocked by Expo developer menus as final evidence.
- Do not leave Vite, proxy, or Metro sessions running after the update.
- Keep the document appendable: add a new version row and only rewrite sections whose behavior changed.

