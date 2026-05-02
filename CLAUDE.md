# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working in this repository. It covers the monorepo layout; subproject-specific instructions live in each subdirectory.

## Subproject Map

Open the nearest CLAUDE.md to whatever you are touching:

- [backend/CLAUDE.md](backend/CLAUDE.md) — Go backend (REST API + ingestion pipeline + Qdrant/storage integration)
- [frontend/CLAUDE.md](frontend/CLAUDE.md) — React 19 + Vite frontend

## Repo Layout

```
backend/      Go application (cmd/, internal/, configs/, Dockerfile)
frontend/    React + Vite SPA
deployments/ Docker Compose orchestration
docs/        Cross-service docs
scripts/     Cross-service helpers (start.sh)
```

## Common Commands

```bash
# Start backend + frontend for local development
./scripts/start.sh

# Backend build / test
cd backend && go build ./... && go test ./...

# Backend protobuf value schema generation
cd backend && GOTOOLCHAIN=go1.26.2 go run github.com/bufbuild/buf/cmd/buf@v1.69.0 generate

# Frontend lint / build
cd frontend && npm install && npm run lint && npm run build

# Ingest local static image data
cd backend && ./scripts/import-data.sh -p ./data/memes -l 50

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
- Backend schema-level enums and structured values are protobuf-defined in `backend/proto/emomo/v1/schema.proto` (only column-level structured values + closed enums; not a wire/RPC contract). The core relational tables are `memes`, `meme_annotations`, and `meme_vectors`.
- Backend database migrations are managed entirely in code: GORM AutoMigrate plus the explicit migration helpers in `backend/internal/repository/db.go`. There is no separate SQL migration runner; do not add files under `backend/migrations/`.
