-- Exact duplicate checks also include records published by the older tooling.
CREATE INDEX IF NOT EXISTS memes_content_hash ON memes(json_extract(meme_json, '$.content_hash'));
