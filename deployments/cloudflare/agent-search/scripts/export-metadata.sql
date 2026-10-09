-- Read-only PostgreSQL export: one JSON line per meme, one existing annotation.
-- Run: psql -X -q -A -t -v ON_ERROR_STOP=1 -f scripts/export-metadata.sql
-- Use an existing secure connection; keep credentials and dumps outside Git.
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SET LOCAL statement_timeout = '60s';
SET LOCAL lock_timeout = '3s';
SELECT jsonb_build_object(
  'meme', jsonb_build_object(
    'id', m.id,
    'storage_key', m.storage_key,
    'content_hash', COALESCE(m.content_hash, ''),
    'image_info', COALESCE(NULLIF(m.image_info, '')::jsonb, '{}'::jsonb),
    'tags', COALESCE(NULLIF(m.tags, '')::jsonb, '[]'::jsonb),
    'category', COALESCE(m.category, '')
  ),
  'annotation', CASE WHEN a.meme_id IS NULL THEN NULL ELSE jsonb_build_object(
    'meme_id', a.meme_id,
    'description', COALESCE(a.description, ''),
    'ocr_text', COALESCE(a.ocr_text, ''),
    'labels', NULLIF(a.labels, '')::jsonb
  ) END
)::text
FROM memes m
LEFT JOIN LATERAL (
  SELECT meme_id, description, ocr_text, labels
  FROM meme_annotations WHERE meme_id=m.id
  ORDER BY updated_at DESC NULLS LAST, created_at DESC NULLS LAST, id ASC
  LIMIT 1
) a ON true
ORDER BY m.id;
COMMIT;
