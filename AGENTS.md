# Repository Guidelines

This is the emomo monorepo containing two sibling subprojects. This file captures repo-wide conventions; subproject-specific details live in each directory.

## Subproject Map

- [backend/AGENTS.md](backend/AGENTS.md) — Go 后端（API + 摄入流水线）
- [frontend/AGENTS.md](frontend/AGENTS.md) — React + Vite 前端

When in doubt about which conventions apply to a file, follow the AGENTS.md nearest to that file.

## Project Structure

```
backend/      Go application (cmd/, internal/, gen/, proto/, configs/, Dockerfile)
frontend/    React + Vite SPA (src/, gen/, e2e/, public/)
deployments/ Docker Compose orchestration (referenced by both backend and ops)
docs/        Cross-service design and ops documentation
scripts/     Cross-service helpers (currently: scripts/start.sh)
```

Single-language helpers (e.g. `import-data.sh`, Vite config) live inside their respective subproject directory, not in the root `scripts/`. Each subproject has a dedicated top-level `gen/` directory containing **only** generated protobuf code (Go in `backend/gen/`, TypeScript in `frontend/gen/`); these directories are committed to git, but flagged `linguist-generated=true` via [.gitattributes](.gitattributes) so PR diffs collapse them.

## Common Commands

- `./scripts/start.sh` — start backend (8080) + frontend (5173) for local development.
- `cd backend && go test ./... && go build ./...` — backend build + tests.
- `cd frontend && npm install && npm run lint && npm run build` — frontend lint + build.
- `cd backend && ./scripts/import-data.sh -p ./data/memes -l 50` — ingest local static image data.
- `cd backend && GOTOOLCHAIN=go1.26.2 go run github.com/bufbuild/buf/cmd/buf@v1.69.0 generate` — regenerate Go code under `backend/gen/` after `.proto` changes.
- `cd frontend && npm run gen` — regenerate TS code under `frontend/gen/` after `.proto` changes.
- `docker compose --env-file backend/.env -f deployments/docker-compose.yml up -d` — run API container + Grafana Alloy locally.

## Coding Style & Naming

- Go: `gofmt` defaults (tabs for indentation); short, lowercase package names.
- TypeScript / React: 2 spaces, semicolons, single quotes; `PascalCase.tsx` + `PascalCase.module.css` per component.
- Configuration: keep new keys grouped by subsystem, in the subproject's own config file.
- Protobuf's boundary is the API contract plus a small allowlist of structured values. Source `.proto` files live in `backend/proto/emomo/v1/` (`types.proto` / `meme.proto` / `api.proto`) and generate backend/ frontend DTOs. Use protobuf for HTTP request/response/SSE bodies, closed cross-boundary enums, and the explicit DB JSON columns `memes.image_info` and `meme_annotations.labels`. Do **not** use protobuf as the owner of relational table shape, migrations, runtime config, open string sets (`category`, `tags`, `collection`, model names), repository internals, or React UI state. emomo's HTTP API stays RESTful via Gin; there is no protobuf `service`, gRPC, Connect, or `google.api.http` layer.
- Backend database migrations are owned by `backend/internal/repository/db.go` (GORM AutoMigrate + explicit migration helpers). No separate SQL migration runner; do not add files under `backend/migrations/`.

## Commit & Pull Request Guidelines

- Use Conventional Commits (`feat:`, `fix:`, `chore:`, `docs:`, `refactor:`...). Keep the subject short and imperative.
- For changes that span subprojects, scope them by directory in the body (e.g. `backend: ...; frontend: ...`).
- PR descriptions should mention scope, link related issues, and add screenshots / curl examples for user-visible changes.

## Security & Configuration

- Never commit API keys or secrets. Each subproject has its own `.env.example`; copy to `.env` (gitignored).
- For production, prefer TLS-enabled endpoints (`QDRANT_USE_TLS=true`, `STORAGE_USE_SSL=true`).
- The Hugging Face Space deploy mirror only sees `backend/` (subtree split). Don't introduce runtime paths that escape that directory in the backend.
