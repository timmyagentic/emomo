# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working in this repository. It covers the monorepo layout; subproject-specific instructions live in each subdirectory.

## Subproject Map

Open the nearest CLAUDE.md to whatever you are touching:

- [backend/CLAUDE.md](backend/CLAUDE.md) — Go backend (REST API + ingestion pipeline + Qdrant/storage integration)
- [frontend/CLAUDE.md](frontend/CLAUDE.md) — React 19 + Vite frontend

## Repo Layout

```
backend/      Go application (cmd/, internal/, gen/, proto/, configs/, Dockerfile)
frontend/    React + Vite SPA (src/, gen/)
deployments/ Docker Compose orchestration
docs/        Cross-service docs
scripts/     Cross-service helpers (start.sh)
```

`backend/gen/` and `frontend/gen/` contain only protobuf-generated code (Go and TypeScript respectively); both are committed to git and tagged `linguist-generated=true` in [.gitattributes](.gitattributes) so review tools auto-fold them.

## Common Commands

```bash
# Start backend + frontend for local development
./scripts/start.sh

# Backend build / test
cd backend && go build ./... && go test ./...

# Backend protobuf code generation (Go → backend/gen/)
cd backend && GOTOOLCHAIN=go1.26.2 go run github.com/bufbuild/buf/cmd/buf@v1.69.0 generate

# Frontend protobuf code generation (TS → frontend/gen/)
cd frontend && npm run gen

# Frontend lint / build
cd frontend && npm install && npm run lint && npm run build

# Ingest all local static image data; import-data.sh is the only supported ingest entrypoint
cd backend && ./scripts/import-data.sh -p ./data/memes

# Containerized API + Grafana Alloy
docker compose --env-file backend/.env -f deployments/docker-compose.yml up -d
```

## Deployment

- Render: configured via [render.yaml](render.yaml) (`rootDir: backend`).
- Railway: [railway.json](railway.json) points at `backend/Dockerfile`.
- Hugging Face Space: [.github/workflows/sync_to_hf.yml](.github/workflows/sync_to_hf.yml) splits `backend/` as a subtree and force-pushes it to the Space's `main`. Anything outside `backend/` does not reach the Space.

## Tips When Editing

- Keep changes scoped to one subproject when possible; cross-cutting commits should clearly call out which directory each hunk belongs to.
- The Go module path is `github.com/timmy/emomo` and is independent of the on-disk path; do not rewrite imports just because files moved.
- Backend runtime expects `cwd = backend/` (so `./configs`, `./data` resolve correctly). Don't introduce code that assumes the repo root is the cwd.
- Protobuf's boundary is intentionally narrow: it defines backend HTTP request/response/SSE DTOs, generated frontend/backend DTOs, closed cross-boundary enums, and the allowlisted DB JSON values `memes.image_info` and `meme_annotations.labels`. Source `.proto` files live in `backend/proto/emomo/v1/` (`types.proto` / `meme.proto` / `api.proto`); generated Go lands in `backend/gen/`, generated TS in `frontend/gen/`. Do not use protobuf to own relational table shape, migrations, runtime config, open string sets, repository internals, or React UI state. The schema is **not** an RPC contract — emomo's HTTP API is RESTful via Gin, with handlers (de)serializing message bodies via `protojson`.
- Backend database migrations are managed entirely in code: GORM AutoMigrate plus the explicit migration helpers in `backend/internal/repository/db.go`. There is no separate SQL migration runner; do not add files under `backend/migrations/`.
