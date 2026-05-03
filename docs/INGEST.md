# Data Ingest

Emomo ingests meme resources from a local static image directory. GIF is not supported; only static `.jpg`, `.jpeg`, `.png`, and `.webp` files are scanned.

The default ingest profile is `qwen3vl`. It writes two vector routes:

- `image`: the image itself is embedded by the multimodal embedding model and stored in Qdrant. Text queries are embedded into the same space and can directly match these image vectors.
- `caption`: OCR/VLM-derived text, category, tags, and emotion words are embedded as an auxiliary caption vector. The same text also feeds the BM25 sparse route for keyword/exact-text recall.

VLM description and OCR are still generated and stored in `meme_annotations`, but they are auxiliary analysis signals; the default retrieval path no longer depends on converting every image into a text description before vector search.

The relational schema is intentionally small:

- `memes`: image identity, storage key, content hash, `image_info`, category, tags.
- `meme_annotations`: VLM/OCR description, OCR text, and structured labels such as `labels.text.present`.
- `meme_vectors`: Qdrant point index records keyed by `meme_id + collection + vector_type`.

Protobuf message schema lives in `backend/proto/emomo/v1/` (`types.proto` for closed cross-boundary enums and allowlisted JSON-column messages, `meme.proto` for API entity DTOs, `api.proto` for HTTP request/response/SSE messages); the generated Go code lives in `backend/gen/emomo/v1/`. `image_info` and `labels` are the only DB JSON columns currently allowed to use protobuf-backed `protojson` serialization via `backend/internal/persistence`; `vector_type` is a protobuf enum stored as an integer. Migrations and relational table shape remain owned by `backend/internal/repository/db.go` (GORM AutoMigrate + explicit helpers) — no separate SQL migration runner.

## Prepare Data

From `backend/`, place images under `data/memes`:

```text
backend/data/memes/
├── 猫猫/
│   ├── 无语.jpg
│   └── 开心.png
└── 狗狗/
    └── 柴犬.webp
```

The default source path is configured in `backend/configs/config.yaml`:

```yaml
sources:
  localdir:
    enabled: true
    root_path: ./data/memes
    source_id: localdir
```

You can also override the path with `LOCAL_MEMES_DIR` or the CLI `--path` flag. `source_id` is a runtime adapter identifier used by the source configuration; it is not persisted as a top-level column in `memes`.

## Run Ingest

```bash
cd backend
./scripts/import-data.sh -p ./data/memes -l 100
```

Equivalent direct command:

```bash
go run ./cmd/ingest --source=localdir --path=./data/memes --limit=100
```

Use `--embedding` to select a non-default embedding configuration:

```bash
./scripts/import-data.sh -p ./data/memes -e qwen3vl_image -l 100
./scripts/import-data.sh -p ./data/memes -e qwen3vl_caption -l 100
```

Use `--profile` to explicitly ingest all routes in a search profile:

```bash
./scripts/import-data.sh -p ./data/memes --profile qwen3vl -l 100
```

Use `--force` to reprocess existing vectors, or `--retry` to backfill missing vector records for existing memes:

```bash
./scripts/import-data.sh -p ./data/memes -f
./scripts/import-data.sh -r -l 100
```

## Metadata Rules

- `content_hash`: computed from the processed image bytes and used for deduplication.
- `storage_key`: object-storage key for the image.
- `image_info`: protobuf-defined structured value containing width, height, and image format.
- `category`: first-level directory name; files directly under the root use `未分类`.
- `tags`: generated from category, path/filename tokens, and optional manifest/queue keywords.
- `labels.text.present`: stored in `meme_annotations.labels` when OCR analysis can determine whether the image has visible text.
- `format`: detected by magic bytes during ingestion and stored inside `image_info.format` as protobuf `ImageFormat`; WebP is converted to JPEG before storage/model calls.
- unsupported formats, including GIF, are skipped or rejected before persistence.

Fields deliberately not stored on `memes`: `source_type`, `source_id`, `local_path`, `is_animated`, `file_size`, `md5_hash`, `perceptual_hash`, and lifecycle `status`.
