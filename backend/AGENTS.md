# Repository Guidelines (backend/)

> 本文件描述 emomo monorepo 的 Go 后端子项目。仓库整体约定见 [../AGENTS.md](../AGENTS.md)；前端约定见 [../frontend/AGENTS.md](../frontend/AGENTS.md)。
> 所有命令默认在 `backend/` 目录下执行（`cd backend`）。

## Project Structure & Module Organization
- `cmd/`: Go entry points (`cmd/api`, `cmd/ingest`).
- `internal/`: Go application code (API handlers, services, repositories, sources, storage).
- `proto/`: protobuf value schema for column-level structured values and closed enums (not a wire/RPC contract).
- `internal/idl/`: generated Go protobuf code.
- `internal/repository/db.go`: owns all database migrations via GORM AutoMigrate + explicit `prepareLegacy*` / `migrate*` / `dropLegacy*` helpers. There is no separate SQL migration runner.
- `configs/`: YAML config files and examples.
- `scripts/`: Backend-only helper scripts (`import-data.sh`, `check-data-dir.sh`, `setup.sh`, `clear-qdrant.sh`).
- `data/`: Local data directories (gitignored except for `.gitkeep`).
- `Dockerfile`, `.dockerignore`: Container build definition (also pushed to Hugging Face Space via subtree split).

Sibling directories at repo root: `../frontend/` (React/Vite UI), `../deployments/` (cross-service compose), `../docs/`, `../scripts/start.sh`.

## Build, Test, and Development Commands
- `cd backend && go run ./cmd/api`: run the API server locally (port 8080 by default).
- `cd backend && go build ./... && go test ./...`: build and test all Go packages.
- `cd backend && ./scripts/import-data.sh -p ./data/memes -l 50`: ingest local static image memes (recommended).
- `cd backend && go run ./cmd/ingest --source=localdir --path=./data/memes --limit=50`: ingest local static image memes (alternative).
- `cd backend && GOTOOLCHAIN=go1.26.2 go run github.com/bufbuild/buf/cmd/buf@v1.69.0 generate`: regenerate Go code after schema-level type changes.
- `docker compose --env-file backend/.env -f deployments/docker-compose.yml up -d` (from repo root): start API + Grafana Alloy.

## Coding Style & Naming Conventions
- Go: follow `gofmt` defaults (tabs for indentation); package names short and lowercase.
- Config: keep new keys grouped by subsystem under `backend/configs/`.
- Schema: keep core relational data centered on `memes`, `meme_annotations`, and `meme_vectors`; update `proto/emomo/v1/schema.proto` only for column-level structured values and closed enums. Do not add top-level `Meme` / `MemeAnnotation` / `MemeVector` messages — the relational rows are GORM structs.
- Migrations: extend the helpers in `internal/repository/db.go` (`prepareLegacy*`, `migrate*`, `dropLegacy*`, `disableCoreTableRLS`) and add a regression test in `internal/repository/db_test.go`. Do not introduce a parallel SQL migration runner.
- Row Level Security: core tables intentionally run with RLS off (`disableCoreTableRLS` turns it off on every `InitDB`); access control is at the connection layer. Do not re-enable RLS without also committing explicit `REVOKE` / `CREATE POLICY` statements.
- Logging: prefer the helpers in `internal/logger` (context-aware fields).

## Testing Guidelines
- Go tests: `cd backend && go test ./...`.
- Add table-driven tests for service-layer logic when introducing new behavior.

## Commit & Pull Request Guidelines
- Commit messages follow Conventional Commits (`feat:`, `fix:`, `chore:` ...). Keep the subject short and imperative.
- For changes that touch multiple subprojects, scope by directory in the body (e.g. "backend: ...; frontend: ...").
- PRs should describe scope, link related issues, and include curl examples or screenshots for API-visible changes.

## Security & Configuration Tips
- Never commit API keys or secrets; use `backend/.env` (gitignored) or environment variables.
- For production, prefer TLS-enabled endpoints (`QDRANT_USE_TLS=true`, `STORAGE_USE_SSL=true`).
- The Hugging Face Space deploy mirror only sees `backend/`, so don't introduce paths that escape this directory at runtime.
