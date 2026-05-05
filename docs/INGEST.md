# Data Ingest

Emomo ingests meme resources from a local static image directory. GIF is not supported; only static `.jpg`, `.jpeg`, `.png`, and `.webp` files are scanned.

The default ingest profile is `qwen3vl`. It writes two vector routes:

- `image`: the image itself is embedded by the multimodal embedding model and stored in Qdrant. Text queries are embedded into the same space and can directly match these image vectors.
- `caption`: OCR/VLM-derived text, category, tags, and emotion words are embedded as an auxiliary caption vector. The same text also feeds the BM25 sparse route for keyword/exact-text recall.

VLM description and OCR are still generated and stored in `meme_annotations`, but they are auxiliary analysis signals; the default retrieval path no longer depends on converting every image into a text description before vector search.

The relational schema is intentionally small:

- `memes`: image identity, storage key, content hash, `image_info`. `category` and `tags` are present on the table but the current ingest pipeline writes them as empty — see "Metadata Rules" below.
- `meme_annotations`: VLM/OCR description, OCR text, and structured labels such as `labels.has_text`.
- `meme_vectors`: Qdrant point index records keyed by `meme_id + collection + vector_type`.
- `meme_metadata`: provenance for each ingested item (source platform, original title, note id, search keywords). **Never read by the search pipeline**; pure traceability.

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

`backend/scripts/import-data.sh` is the only supported data ingest entrypoint. Pass the local image directory with `-p` / `--path`; the lower-level Go worker is internal to the script and should not be invoked directly. `source_id` is a runtime adapter identifier used by the source configuration; it is not persisted as a top-level column in `memes`.

## Run Ingest

```bash
cd backend
./scripts/import-data.sh -p ./data/memes
```

Use `--embedding` to select a non-default embedding configuration:

```bash
./scripts/import-data.sh -p ./data/memes -e qwen3vl_image
./scripts/import-data.sh -p ./data/memes -e qwen3vl_caption
```

Use `--profile` to explicitly ingest all routes in a search profile:

```bash
./scripts/import-data.sh -p ./data/memes --profile qwen3vl
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
- `memes.category`: present on the schema but **left empty** by the current ingest pipeline. Folder names and crawler keywords are not semantic categories — they describe how an image was *collected*, not what it *depicts*. See `meme_metadata` below for where that information actually goes.
- `memes.tags`: present on the schema but **left empty** by the current ingest pipeline, for the same reason. The field is reserved for genuine semantic tags (e.g. future VLM-extracted tags or manual labels).
- `labels.has_text`: stored in `meme_annotations.labels` as a single bool. The OCR analyzer always populates this; the protojson serializer is configured with `EmitUnpopulated=true` so every row carries an explicit `true`/`false` value at rest (no missing-key ambiguity).
- `format`: detected by magic bytes during ingestion and stored inside `image_info.format` as protobuf `ImageFormat`; WebP is converted to JPEG before storage/model calls.
- unsupported formats, including GIF, are skipped or rejected before persistence.

Fields deliberately not stored on `memes`: `source_type`, `source_id`, `local_path`, `is_animated`, `file_size`, `md5_hash`, `perceptual_hash`, and lifecycle `status`.

### Provenance via `meme_metadata`

Every ingested item produces (at most) one row in `meme_metadata`, keyed by `(source, source_item_id, meme_id)`. The shape varies by source adapter:

- **`source_id = "xiaohongshu"`** (or any source whose `localdir` adapter is fed `stage1_queue.jsonl` + `stage2_results.jsonl`):
  - `source = "xiaohongshu"`, `source_item_id = note_id`
  - `source_url = https://www.xiaohongshu.com/explore/<note_id>`
  - `title` from stage2/stage1, `author` and `published_at` from stage1
  - `search_keywords` is the union of `stage1.keyword + stage1.keywords + stage2.keyword`

- **`source_id = "localdir"`** (hand-curated `data/memes/猫/无语.jpg` style, no manifest):
  - `source = "localdir"`, `source_item_id = relative path`
  - `title = filename stem`
  - `source_url`, `author`, `published_at`, `search_keywords` all empty

The ingest service writes `meme_metadata` immediately after the `memes` row succeeds. Failure to upsert metadata is logged and **does not** abort the item — metadata is not on the search hot path.

Crucially, no part of the search pipeline (caption embedding, BM25 sparse, Qdrant payload) reads `meme_metadata`. If you ever want to surface the original source link in the UI, fetch it explicitly from this table by `meme_id`.
