# Repository Guidelines

This is the emomo monorepo with an Agent CLI and the retained backend/web projects. This file captures repo-wide conventions; subproject-specific details live in each directory.

## Subproject Map

- [cli/AGENTS.md](cli/AGENTS.md) — Agent CLI + 搜索 skill
- [deployments/cloudflare/agent-search/AGENTS.md](deployments/cloudflare/agent-search/AGENTS.md) — 独立文字检索，不调用云端模型
- [backend/AGENTS.md](backend/AGENTS.md) — Go 后端（API + 摄入流水线）
- [frontend/AGENTS.md](frontend/AGENTS.md) — React + Vite 前端

When in doubt about which conventions apply to a file, follow the AGENTS.md nearest to that file.

## Project Structure

```
backend/      Go application (cmd/, internal/, gen/, proto/, configs/, Dockerfile)
cli/          Dependency-free Node CLI, packaged skill, and protocol/install tests
protocol/     Independent generation tools for current Worker and website DTOs
frontend/    React + Vite SPA (src/, gen/, e2e/, public/)
deployments/ Docker Compose orchestration (referenced by both backend and ops)
docs/        Cross-service design and ops documentation
scripts/     Cross-service helpers (currently: scripts/start.sh)
```

Single-language helpers (e.g. `import-data.sh`, Vite config) live inside their respective subproject directory, not in the root `scripts/`. Each subproject has a dedicated top-level `gen/` directory containing **only** generated protobuf code (Go in `backend/gen/`, TypeScript in `frontend/gen/`); these directories are committed to git, but flagged `linguist-generated=true` via [.gitattributes](.gitattributes) so PR diffs collapse them.

## Cloud development

Use `node scripts/dev.mjs bootstrap agent` then `node scripts/dev.mjs check agent` for the current CLI and deterministic search product. Use the `full` profile for the retained gateway, Go and frontend checks. Current Worker/website DTOs generate directly through `protocol/`; they do not copy old frontend output. Required CI scopes retained-product checks to changed paths and runs all on shared/schema changes. See [docs/CLOUD_DEVELOPMENT.md](docs/CLOUD_DEVELOPMENT.md) for requirements, isolation and verification boundaries. Never copy private image libraries or environment files into cloud development.

The native mobile app was retired on 2026-10-25. Do not restore mobile dependencies, CI or builds as part of normal development; see [retirement record](docs/MOBILE_RETIREMENT.md).

## Common Commands

- `./scripts/start.sh` — start backend (8080) + frontend (5173) for local development.
- `cd backend && go test ./... && go build ./...` — backend build + tests.
- `cd frontend && npm install && npm run lint && npm run build` — frontend lint + build.
- `cd backend && ./scripts/import-data.sh -p ./data/memes` — retained backend ingest only; this script owns the legacy model pipeline. Current reviewed-library maintenance uses `emomo-library` and never starts the old pipeline.
- `cd backend && GOTOOLCHAIN=go1.26.2 go run github.com/bufbuild/buf/cmd/buf@v1.69.0 generate` — regenerate Go code under `backend/gen/` after `.proto` changes.
- `cd frontend && npm run gen` — regenerate TS code under `frontend/gen/` after `.proto` changes.
- `docker compose --env-file backend/.env -f deployments/docker-compose.yml up -d` — run API container + Grafana Alloy locally.

## Coding Style & Naming

- Go: `gofmt` defaults (tabs for indentation); short, lowercase package names.
- TypeScript / React: 2 spaces, semicolons, single quotes; `PascalCase.tsx` + `PascalCase.module.css` per web component.
- Configuration: keep new keys grouped by subsystem, in the subproject's own config file.
- Protobuf's boundary is the API contract plus a small allowlist of structured values. Source `.proto` files live in `backend/proto/emomo/v1/` (`types.proto` / `meme.proto` / `api.proto`) and generate backend/frontend DTOs. Use protobuf for HTTP request/response/SSE bodies, closed cross-boundary enums, and the explicit DB JSON columns `memes.image_info` and `meme_annotations.labels`. Do **not** use protobuf as the owner of relational table shape, migrations, runtime config, open string sets (`category`, `tags`, `collection`, model names), repository internals, or React UI state. emomo's HTTP API stays RESTful via Gin; there is no protobuf `service`, gRPC, Connect, or `google.api.http` layer.
- Backend database migrations are owned by `backend/internal/repository/db.go` (GORM AutoMigrate + explicit migration helpers). No separate SQL migration runner; do not add files under `backend/migrations/`.

## Public product versioning

The current public Emomo version is owned by `cli/package.json`. The baseline is `0.1.0`; routine subsequent releases increment the patch component (`0.1.1`, `0.1.2`, ...). Changing the minor or major component requires an explicit product decision. Follow [docs/PUBLIC_RELEASE.md](docs/PUBLIC_RELEASE.md#version-policy) and synchronize every current install/release/website entry point. Schema/API/dependency versions and historical release evidence must not be mechanically renumbered.

## Commit & Pull Request Guidelines

- Use Conventional Commits (`feat:`, `fix:`, `chore:`, `docs:`, `refactor:`...). Keep the subject short and imperative.
- For changes that span subprojects, scope them by directory in the body (e.g. `backend: ...; frontend: ...`).
- PR descriptions should mention scope, link related issues, and add screenshots / curl examples for user-visible changes.

## Security & Configuration

- Never commit API keys or secrets. Each subproject has its own `.env.example`; copy to `.env` (gitignored).
- For production, prefer TLS-enabled endpoints (`QDRANT_USE_TLS=true`, `STORAGE_USE_SSL=true`).
- The Hugging Face Space deploy mirror only sees `backend/` (subtree split). Don't introduce runtime paths that escape that directory in the backend.

## Repository workflow and branch protection

- Confirm the current GitHub default branch before starting; it is currently `main`. Fetch it, use a separate `codex/` branch in an isolated worktree, and keep the primary checkout read-only. Under Tingjun Workspace, activate the Task and Session, acknowledge the current Context revision and hold the worktree's unique writer claim.
- All changes to the GitHub default branch must go through a PR. Direct pushes, force pushes and deletion of that branch are prohibited. Administrators and apps have no bypass. Never use `--admin`, disable protection or add a bypass actor to land a change.
- Only squash merging is allowed. Inspect the final diff, current head, review threads and actual verification results before an authorized merge. Resolve all review conversations and address substantive findings; zero mandatory approvals supports the single-maintainer workflow. Merge with `gh pr merge <PR> --squash --match-head-commit <verified-full-head-SHA>` and verify the merged state afterward.
- Keep `delete_branch_on_merge = true` and verify that GitHub automatically removes the merged PR's remote task branch when eligible. Do not use `--delete-branch`, which also removes the local branch. Under Tingjun Workspace, completion only releases the writer claim; local branches and worktrees remain for the existing archive workflow.
- The default-branch ruleset declaration is [`.github/rulesets/default-branch.json`](.github/rulesets/default-branch.json); repository merge and deletion settings are declared in [`.github/repository-settings.json`](.github/repository-settings.json). These files do not apply themselves. After an authorized configuration update, read back the GitHub repository settings, effective rules and any classic branch protection. Protect the actual default branch rather than assuming it is named `main`.
- Preserve the existing CI workflows, trigger conditions, required checks, app bindings and strictness. Existing stronger protections and immutable release tags remain in force. An empty or legitimately path-filtered checks list does not prove application tests passed; report the checks actually run and mark unavailable verification `UNVERIFIED`. Follow this repository's existing verification requirements and use proportionate document/configuration checks for documentation changes.
