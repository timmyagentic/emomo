# Repository Guidelines (frontend/)

> Frontend subproject of the emomo monorepo. Repo-wide conventions: [../AGENTS.md](../AGENTS.md). Backend conventions: [../backend/AGENTS.md](../backend/AGENTS.md).
> All commands below assume `cd frontend`.

## Project Structure & Module Organization
- `src/main.tsx` is the app entry; `src/App.tsx` is the root component.
- `src/components/` holds UI components; each uses `Component.tsx` with `Component.module.css`.
- `src/api/` contains the fetch wrapper that talks to the backend (request/response are protobuf messages serialized via `protojson`); `src/types/` holds UI projections such as `DisplayMeme` and accessor helpers. Generated protobuf types stop at the API boundary: do not use them as React component state, component props, or local fallback-data shapes once a value has been decoded.
- `gen/emomo/v1/` holds **generated** TypeScript message + schema descriptors produced from `../backend/proto/emomo/v1/` by `npm run gen`. Files there are committed but tagged `linguist-generated=true`; do not hand-edit. Import them via the `@gen/*` path alias (e.g. `from '@gen/emomo/v1/api_pb'`).
- `public/` stores static files served by Vite; `dist/` is the production build output.
- `e2e/` contains Playwright end-to-end tests (`*.spec.ts`).

## Build, Test, and Development Commands
- `npm install` installs dependencies.
- `npm run dev` starts the Vite dev server with HMR.
- `npm run gen` runs `buf generate --template buf.gen.yaml` to refresh `gen/` after `.proto` changes in `../backend/proto/`.
- `npm run build` runs TypeScript build (`tsc -b`) and outputs `dist/`.
- `npm run preview` serves the production build locally.
- `npm run lint` runs ESLint across the repo (the `gen/` directory is excluded via `eslint.config.js`).
- `npm run test` runs Playwright tests headless.
- `npm run test:ui` runs Playwright with the UI runner.
- `npm run test:headed` runs Playwright in headed mode.

## Coding Style & Naming Conventions
- Use TypeScript + React function components with hooks.
- Indentation: 2 spaces; use semicolons and single quotes to match existing files.
- Components use `PascalCase.tsx` and matching `PascalCase.module.css`.
- Hooks should be named `useSomething` in `src/hooks/` when added.
- Types live in `src/types/`; prefer explicit types at module boundaries. Do **not** hand-write Go/wire mirror types — anything that crosses the network boundary lives in `gen/` and is generated from the backend's `.proto`. After decoding, project protobuf DTOs into UI-owned types before passing data through the component tree.

## Testing Guidelines
- Framework: Playwright (`playwright.config.ts`).
- Place tests in `e2e/` and name them `*.spec.ts` (example: `e2e/app.spec.ts`).
- Add or update tests for UI behavior changes; no coverage gate is enforced.

## Commit & Pull Request Guidelines
- Commit history commonly uses Conventional Commits (`feat:`, `fix:`, `refactor:`) with optional scopes; keep subjects short and imperative.
- Non-English commit messages appear, but keep the style consistent within a series.
- PRs should include a concise description, linked issues if applicable, and before/after screenshots for UI changes.
- Call out any API or env changes in the PR description.

## Configuration Tips
- Copy `.env.example` to `.env` and set `VITE_API_BASE` for local development. Production builds use the Cloudflare API gateway. Do not add private upstream tokens to Vite env vars.
- Vite only exposes env vars prefixed with `VITE_`.
