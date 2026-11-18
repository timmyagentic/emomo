-- statement
CREATE TABLE library_takedowns (
  sha256 TEXT PRIMARY KEY NOT NULL,
  targets TEXT NOT NULL CHECK(json_valid(targets)),
  taken_down_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
-- statement
CREATE TRIGGER library_takedown_remove AFTER INSERT ON library_takedowns BEGIN
  DELETE FROM memes WHERE json_extract(meme_json, '$.content_hash') = new.sha256;
END;
-- statement
CREATE TRIGGER memes_block_taken_down_insert BEFORE INSERT ON memes
WHEN EXISTS (SELECT 1 FROM library_takedowns WHERE sha256 = json_extract(new.meme_json, '$.content_hash'))
BEGIN
  SELECT RAISE(IGNORE);
END;
-- statement
CREATE TRIGGER memes_block_taken_down_update BEFORE UPDATE OF meme_json ON memes
WHEN EXISTS (SELECT 1 FROM library_takedowns WHERE sha256 = json_extract(new.meme_json, '$.content_hash'))
BEGIN
  SELECT RAISE(IGNORE);
END;
