# Keyword Sparse Backfill

`reembed-keyword-sparse` backfills missing BM25 sparse vectors for an existing search profile.

Use this command when a profile's keyword route points at a Qdrant collection that already contains caption hybrid points, but some memes still have no BM25-compatible vector in that collection.

The command does not call an embedding model. It builds BM25 text from the latest annotation OCR text, compact VLM description, and tags, then writes a sparse-only Qdrant point plus a `meme_vectors` row with:

- `vector_type = KEYWORD`
- `embedding_model = qdrant/bm25`
- `collection = <profile keyword_embedding collection>`

Memes that already have either a caption hybrid vector or a keyword sparse vector in the target collection are skipped. This prevents duplicate sparse recall for memes whose caption vector already carries a BM25 vector.

## Recommended Flow

Run from `backend/`.

```bash
LOKI_ENABLED=false LOG_LEVEL=info go run ./cmd/reembed-keyword-sparse \
  --profile qwen3vl \
  --dry-run \
  --workers 2
```

Then smoke test a small batch:

```bash
LOKI_ENABLED=false LOG_LEVEL=info go run ./cmd/reembed-keyword-sparse \
  --profile qwen3vl \
  --limit 100 \
  --workers 2 \
  --retries 5
```

After verifying Postgres/Qdrant coverage and sparse-only point shape, run the remainder:

```bash
LOKI_ENABLED=false LOG_LEVEL=info go run ./cmd/reembed-keyword-sparse \
  --profile qwen3vl \
  --workers 2 \
  --retries 5
```

Re-run dry-run at the end. A clean completion should report `planned=0`. Remaining rows with `skipped_empty_bm25` have no OCR, description, or tags available for sparse indexing and should not be forced into empty sparse points.

## Flags

- `--profile`: search profile whose keyword embedding collection should be backfilled.
- `--limit`: maximum missing memes to process; `0` means no limit.
- `--workers`: concurrent workers.
- `--retries`: retry attempts for transient Qdrant writes.
- `--retry-delay`: initial retry delay; retries use exponential backoff.
- `--dry-run`: plan only; do not write Qdrant or Postgres.
- `--check-storage`: verify source object existence before writing the sparse point.
