# CLAUDE.md (frontend/)

This file provides guidance to Claude Code (claude.ai/code) when working in the React frontend of the emomo monorepo. For repo-wide context see [../CLAUDE.md](../CLAUDE.md); for the Go backend see [../backend/CLAUDE.md](../backend/CLAUDE.md).

All commands below assume `cd frontend`.

## Project Overview

Emomo is an AI-powered meme search engine frontend built with React 19, TypeScript, and Vite. It features semantic search, a responsive meme grid, and modal detail views with copy/download actions.

## Commands

- `npm run dev` - Start Vite dev server with HMR
- `npm run gen` - Run `buf generate` to refresh `gen/emomo/v1/*` from `../backend/proto/`
- `npm run build` - TypeScript build + Vite production build to `dist/`
- `npm run lint` - Run ESLint (skips `gen/`)
- `npm run test` - Run Playwright e2e tests headless
- `npm run test:ui` - Run Playwright with UI runner
- `npm run test:headed` - Run Playwright in headed browser mode

## Architecture

### Entry Flow
`index.html` -> `src/main.tsx` -> `src/App.tsx` (state hub) -> child components

### Key Directories
- `gen/emomo/v1/` — **generated** TypeScript protobuf message + schema descriptors (not hand-edited). Source `.proto` files live in `../backend/proto/emomo/v1/`; the `buf` config in `frontend/buf.gen.yaml` consumes the same source as the backend so wire types are guaranteed identical. Import via `@gen/*` path alias.
- `src/components/` - React components with co-located CSS modules (`Component.tsx` + `Component.module.css`)
- `src/api/index.ts` - Thin fetch wrapper that calls `protojson` (`fromJson` / `toJson`) against the schemas in `@gen/emomo/v1/` and projects results into `DisplayMeme` for the UI layer
- `src/types/index.ts` - UI-side projection (`DisplayMeme` + accessor helpers). Wire types are NOT mirrored here — they all come from `@gen/emomo/v1/*`. Generated protobuf types should not leak into React state, component props, or local fallback data once decoding is complete.
- `e2e/` - Playwright test specs (`*.spec.ts`)

### State Management
Uses React hooks only (useState, useEffect, useCallback, useMemo, useRef). App.tsx is the state hub managing memes, search state, loading states, and modal state. No Redux or external state library.

### Styling
- CSS Modules for component-scoped styles (import as `styles` object)
- Global CSS variables defined in `src/index.css` for theming
- Framer Motion for animations

### API Configuration
Environment variables (copy `.env.example` to `.env`):
- `VITE_API_BASE` - Local development API override (default: `http://localhost:8080/api/v1`; production builds use `https://api.emomo.net/api/v1`)
- Do not expose private upstream tokens in Vite env vars; production clients should call the Cloudflare API gateway.

Demo data fallback exists in App.tsx for offline development.

## Code Conventions

- 2 spaces indentation, semicolons, single quotes
- Components: PascalCase (`MemeCard.tsx` with `MemeCard.module.css`)
- Hooks: `useSomething` pattern in `src/hooks/` when added
- Commits: Conventional Commits format (`feat:`, `fix:`, `refactor:`)
