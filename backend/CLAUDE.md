# CLAUDE.md (backend/)

This file provides guidance to Claude Code (claude.ai/code) when working in the Go backend of the emomo monorepo. For repo-wide context see [../CLAUDE.md](../CLAUDE.md); for the React frontend see [../frontend/CLAUDE.md](../frontend/CLAUDE.md).

All commands below assume `cd backend` unless stated otherwise.

## Project Overview

Emomo is an AI-powered meme/sticker semantic search system. Users can search for memes using natural language queries in Chinese.

**Tech Stack:** Go 1.26.2 + Gin, Qdrant (vector DB), S3-compatible storage (R2/S3), SQLite/PostgreSQL, protobuf message schema (shared with frontend via protobuf-es), Qwen3-VL multimodal embeddings, OpenAI-compatible VLM/OCR auxiliary analysis, Grafana Alloy + Loki (logging)

## Build & Run Commands

```bash
# Build binaries (optional, can use go run instead)
cd backend
go build -o api ./cmd/api

# Start infrastructure (API + Alloy; Qdrant/S3 are external)
# (run from repo root)
docker compose --env-file backend/.env -f deployments/docker-compose.yml up -d

# Logs only (Grafana Alloy)
docker compose --env-file backend/.env -f deployments/docker-compose.yml up -d alloy

# Data ingestion using the only supported entrypoint
cd backend
./scripts/import-data.sh -p ./data/memes            # Ingest all memes
./scripts/import-data.sh -r -l 100                  # Backfill missing vectors for existing memes
./scripts/import-data.sh -p ./data/memes -f         # Force re-process

# cmd/ingest is internal and must not be invoked directly

# Run API server (port 8080)
go run ./cmd/api

# Full stack (backend + frontend) — from repo root
../scripts/start.sh
```

## Architecture

```
backend/
├── cmd/
│   ├── api/main.go      # REST API server entry point
│   └── ingest/main.go   # Data ingestion CLI tool
├── internal/
│   ├── api/
│   │   ├── router.go    # Route configuration
│   │   └── handler/     # HTTP handlers (search, meme, health)
│   ├── service/
│   │   ├── search.go    # Semantic search (query → embedding → Qdrant)
│   │   ├── ingest.go    # Ingestion pipeline with worker pool
│   │   ├── vlm.go       # OpenAI-compatible VLM/OCR auxiliary analysis
│   │   └── embedding.go # Multimodal/text embeddings (multi-model registry)
│   ├── repository/
│   │   ├── meme_repo.go # Relational DB operations (memes)
│   │   ├── meme_annotation_repo.go # VLM/OCR annotations and structured labels
│   │   ├── meme_vector_repo.go # Qdrant point index records
│   │   └── qdrant_repo.go # Vector search operations (gRPC)
│   ├── storage/s3.go    # S3-compatible object storage (supports R2, S3, etc.)
│   ├── source/          # Data source adapters
│   │   └── localdir/    # Local static image directory source
│   ├── logger/          # Context-aware structured logging
│   └── domain/          # Data models (Meme, MemeAnnotation, MemeVector)
├── proto/               # hand-written protobuf message schema (types/meme/api .proto)
└── gen/                 # generated Go protobuf code (do not hand-edit)
```

### Data Flow

1. **Ingestion**: Local static image source → validate/normalize image → compute `content_hash` → upload to object storage → create/reuse `memes` → direct image embedding → Qdrant upsert → `meme_vectors` save. VLM/OCR writes `meme_annotations` as auxiliary description/OCR/labels.
2. **Search**: Query text → optional query expansion → multimodal query embedding → direct Qdrant image-vector search, optionally fused with caption dense and BM25 sparse routes → fetch metadata from `memes`.
3. **Schema boundary**: Core relational tables are `memes`, `meme_annotations`, and `meme_vectors`. Protobuf in `proto/emomo/v1/{types,meme,api}.proto` defines API DTOs, generated frontend/backend DTOs, closed cross-boundary enums, and the allowlisted structured DB JSON values `memes.image_info` / `meme_annotations.labels` (serialized via `protojson` with `UseEnumNumbers=true`). GORM models plus `internal/repository/db.go` own table shape, indexes, and migrations. Do not model runtime config, open business sets (`category`, `tags`, `collection`, model names), repository internals, or React UI state in protobuf. There is intentionally no `service` definition or RPC layer — handlers stay RESTful Gin handlers that call `protojson.Unmarshal` / `protojson.Marshal` directly.
4. **Migrations**: managed in code via GORM AutoMigrate plus the explicit helpers in `internal/repository/db.go` (`prepareLegacy*`, `migrate*`, `dropLegacy*`, `disableCoreTableRLS`). There is no separate SQL migration runner — do not introduce one without first replacing those Go helpers.
5. **Row Level Security**: the core tables intentionally run with RLS disabled; `disableCoreTableRLS` actively turns it off on every `InitDB`. Access control is enforced at the connection layer (service-role DSN, no Supabase Data API exposure for these tables) — do not enable RLS again unless you also commit explicit `REVOKE` / `CREATE POLICY` statements.

## API Endpoints

- `POST /api/v1/search` - Semantic meme search (`{"query": "text", "top_k": 20}`)
- `GET /api/v1/categories` - List categories
- `GET /api/v1/memes` - List memes (supports `category`, `limit`, `offset`)
- `GET /api/v1/memes/{id}` - Get meme details
- `GET /api/v1/stats` - System statistics
- `GET /health` - Health check

## Configuration

Environment variables (see `backend/.env.example`):
- **VLM**: `VLM_MODEL`, `OPENAI_API_KEY`, `OPENAI_BASE_URL`
- **Embeddings**: `SILICONFLOW_API_KEY`, `SILICONFLOW_BASE_URL` for the default Qwen3-VL profile; optional providers include `JINA_API_KEY`, `MODELSCOPE_API_KEY`, `MODELSCOPE_BASE_URL`
- **Search**: `QUERY_EXPANSION_MODEL`
- **Storage**: `STORAGE_TYPE`, `STORAGE_ENDPOINT`, `STORAGE_ACCESS_KEY`, `STORAGE_SECRET_KEY`, `STORAGE_PUBLIC_URL`
- **Qdrant**: `QDRANT_HOST`, `QDRANT_PORT`, `QDRANT_API_KEY`, `QDRANT_USE_TLS`
- **Database**: `DATABASE_DRIVER` (sqlite/postgres), `DATABASE_PATH` or `DATABASE_URL`
- **Monitoring**: `LOKI_URL`, `LOKI_USERNAME`, `LOKI_PASSWORD`, `CLUSTER_NAME`, `ENVIRONMENT`

Config file: `backend/configs/config.yaml`.

## Deployment & Monitoring

- **Compose**: `deployments/docker-compose.yml` for API + Alloy (Qdrant/S3 are external).
- **Render / Railway**: configured via `render.yaml` (`rootDir: backend`) and `railway.json` (`dockerfilePath: backend/Dockerfile`).
- **Hugging Face Space**: GitHub Actions splits `backend/` as a subtree and force-pushes it to the Space's `main`. The Space sees `backend/` contents as its root.
- **Logging**: Grafana Alloy collects Docker container logs and forwards to Grafana Cloud Loki. Alloy UI: `http://localhost:12345`.

## Key Patterns

- **Source Interface**: New data sources implement `Source` interface in `internal/source/`; source IDs are runtime adapter identifiers, not columns on `memes`.
- **Worker Pool**: Ingest service uses goroutine workers with configurable concurrency.
- **Layered Architecture**: Handler → Service → Repository → Storage.
- **Clean schema**: Do not reintroduce top-level `source_type`, `source_id`, `local_path`, `is_animated`, `md5_hash`, `status`, or per-vector provider/mode/dimension columns unless there is an implemented runtime need.
- **Structured labels**: "has visible text" lives at `meme_annotations.labels.has_text` (flat bool, EmitUnpopulated so every row stores an explicit value), and Qdrant `text_presence` is a derived payload/filter value.
- **Multi-embedding**: each embedding route is registered in `internal/service/embedding_registry.go` and stored as a separate `meme_vectors` row keyed by `meme_id + collection + vector_type`.
- **Protobuf code generation**: after changing any `.proto` under `proto/emomo/v1/`, run `GOTOOLCHAIN=go1.26.2 go run github.com/bufbuild/buf/cmd/buf@v1.69.0 generate` (Go output to `gen/`) and `cd ../frontend && npm run gen` (TS output to `frontend/gen/`). Both `gen/` directories are committed to git so deploy targets that don't have buf installed (Render / Railway / HuggingFace Space) can still build.
- **Migrations**: extend `internal/repository/db.go` (specifically `dropLegacyArtifacts`, `migrateMemes`, `prepareLegacy*` helpers). Add a regression test in `db_test.go` (SQLite) and, if it touches Postgres-specific behaviour, in `db_postgres_integration_test.go`.
