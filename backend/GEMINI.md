# GEMINI.md - Backend Context for AI Assistants

This file describes the Go backend of the emomo monorepo. For repo-wide context see [../GEMINI.md](../GEMINI.md); for the React frontend see [../frontend/GEMINI.md](../frontend/GEMINI.md).

All commands below assume `cd backend` unless noted.

## 1. Project Overview

**Emomo** is a meme search engine that ingests memes from a local static image directory, indexes images with multimodal embeddings, and provides a semantic search API. VLM/OCR output is stored as auxiliary annotation data, not as the primary retrieval representation.

### Core Components
*   **Ingestion (`scripts/import-data.sh`):** the only supported data ingest entrypoint. It consumes a local static image directory, invokes the internal `cmd/ingest` worker, validates/normalizes images, uploads images to object storage (S3/R2), writes `memes`, embeds image/caption routes, indexes vectors in Qdrant, and stores Qdrant point records in `meme_vectors`.
*   **Annotations:** VLM description, OCR text, and structured labels live in `meme_annotations`. The "has visible text" tag is `labels.text.present`.
*   **API (Go, `cmd/api`):** REST API (Gin) for searching memes; uses optional query expansion, direct image-vector search, caption dense search, and BM25 sparse search.

## 2. Technology Stack

*   **Languages:** Go 1.26.2 (this directory; pinned in `go.mod`).
*   **Web Framework:** Gin (Go).
*   **Databases:** PostgreSQL (primary metadata, GORM) and Qdrant (vector search).
*   **Storage:** S3-compatible object storage (Cloudflare R2, AWS S3, or MinIO).
*   **Protobuf message schema:** hand-written `.proto` source lives in `proto/emomo/v1/` (`types.proto` / `meme.proto` / `api.proto`); generated Go code lands in `gen/emomo/v1/` (imported as `pb`). Protobuf defines API DTOs, generated frontend/backend DTOs, closed cross-boundary enums, and the allowlisted structured DB JSON values `memes.image_info` / `meme_annotations.labels`. It does not own relational table shape, migrations, runtime config, open business sets, repository internals, or React UI state. It is **not** an RPC contract — handlers under `internal/api/handler/` are still Gin handlers that read and write protobuf messages via `protojson`.
*   **AI Models:** Qwen3-VL multimodal embeddings are the default for image/caption/query vectors; OpenAI-compatible VLM/OCR is auxiliary.
*   **Infrastructure:** Docker Compose for local development (`../deployments/docker-compose.yml`).

## 3. Architecture & Data Flow

```mermaid
graph LR
    Local[Local Static Image Dir] --> Ingest[Ingest Service]

    Ingest -->|Upload| S3[Object Storage]
    Ingest -->|image/caption embeddings| AI[AI Services]
    Ingest -->|auxiliary VLM/OCR| AI
    Ingest -->|Metadata| DB[(PostgreSQL)]
    Ingest -->|Vectors| Vector[(Qdrant)]

    User -->|Search| API[API Service]
    API -->|Query| AI
    API -->|Search| Vector
    API -->|Fetch| DB
```

## 4. Key Directories (within backend/)

*   `cmd/api/`: REST API server entry (`main.go`).
*   `cmd/ingest/`: Internal ingestion worker used only by `scripts/import-data.sh`.
*   `internal/api/`: HTTP handlers and routers.
*   `proto/emomo/v1/`: hand-written `.proto` source (`types.proto` / `meme.proto` / `api.proto`).
*   `gen/emomo/v1/`: generated Go protobuf code; do not hand-edit.
*   `internal/service/`: Business logic (search, ingest, VLM/OCR, embedding, query expansion).
*   `internal/repository/`: Data access layer (DB, Qdrant). `db.go` owns every database migration in code (GORM AutoMigrate + `prepareLegacy*` / `migrate*` / `dropLegacy*` helpers); there is no SQL migration runner.
*   `internal/source/`: Adapters for ingestion sources (`localdir`).
*   `configs/`: `config.yaml`, `config.cloud.yaml.example`, `huggingface-spaces.env.example`.

## 5. Development & Usage

### Prerequisites
*   Go 1.26.2
*   Docker & Docker Compose

### Local Setup
1.  Configuration: `cp .env.example .env` and fill in API keys.
2.  Optional infra: `docker compose --env-file backend/.env -f deployments/docker-compose.yml up -d` (from repo root) to start API + Alloy.
3.  Prepare local static image data:
    ```bash
    mkdir -p ./data/memes
    ```
4.  Ingest all local images: `./scripts/import-data.sh -p ./data/memes`.
5.  API server: `go run ./cmd/api`. Defaults to `http://localhost:8080`.

### Common Tasks

*   **Add new ingestion source:**
    1.  Implement `internal/source/Source` interface.
    2.  Register in `cmd/ingest/main.go`; API routes must not become data ingest entrypoints.
*   **Add new embedding model:**
    1.  Add an entry under `embeddings:` in `configs/config.yaml` (provider, dimensions, collection, api_key_env).
    2.  Verify it loads via `internal/service/embedding_registry.go`.
*   **Change protobuf messages:**
    1.  Edit one of `proto/emomo/v1/types.proto` (closed cross-boundary enums + allowlisted JSON-column messages), `meme.proto` (API entity DTOs — Meme / MemeAnnotation / MemeVector / SearchResult), or `api.proto` (HTTP request/response + SSE event messages).
    2.  Run `GOTOOLCHAIN=go1.26.2 go run github.com/bufbuild/buf/cmd/buf@v1.69.0 generate`.
    3.  Keep the relational schema centered on `memes`, `meme_annotations`, and `meme_vectors`.
*   **Database migrations:** managed entirely in code via `internal/repository/db.go` (GORM AutoMigrate plus explicit `prepareLegacy*` / `migrate*` / `dropLegacy*` helpers). Add new migration logic and a regression test in `internal/repository/db_test.go`; do not introduce a parallel SQL migration runner.

## 6. Testing

*   **Go Tests:** `go test ./...`.
