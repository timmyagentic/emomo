-- statement
CREATE TABLE memes (
  id TEXT PRIMARY KEY NOT NULL,
  meme_json TEXT NOT NULL CHECK(json_valid(meme_json)),
  category TEXT NOT NULL,
  description TEXT NOT NULL,
  ocr_text TEXT NOT NULL,
  text_presence INTEGER NOT NULL CHECK(text_presence BETWEEN 1 AND 3),
  ocr_terms TEXT NOT NULL,
  description_terms TEXT NOT NULL,
  tag_terms TEXT NOT NULL
);
-- statement
CREATE INDEX memes_category ON memes(category);
-- statement
CREATE INDEX memes_text_presence ON memes(text_presence);
-- statement
CREATE VIRTUAL TABLE meme_fts USING fts5(
  ocr_terms, description_terms, tag_terms,
  content='memes', content_rowid='rowid', tokenize='unicode61'
);
-- statement
CREATE TRIGGER memes_insert AFTER INSERT ON memes BEGIN
  INSERT INTO meme_fts(rowid, ocr_terms, description_terms, tag_terms)
  VALUES (new.rowid, new.ocr_terms, new.description_terms, new.tag_terms);
END;
-- statement
CREATE TRIGGER memes_delete AFTER DELETE ON memes BEGIN
  INSERT INTO meme_fts(meme_fts, rowid, ocr_terms, description_terms, tag_terms)
  VALUES ('delete', old.rowid, old.ocr_terms, old.description_terms, old.tag_terms);
END;
-- statement
CREATE TRIGGER memes_update AFTER UPDATE ON memes BEGIN
  INSERT INTO meme_fts(meme_fts, rowid, ocr_terms, description_terms, tag_terms)
  VALUES ('delete', old.rowid, old.ocr_terms, old.description_terms, old.tag_terms);
  INSERT INTO meme_fts(rowid, ocr_terms, description_terms, tag_terms)
  VALUES (new.rowid, new.ocr_terms, new.description_terms, new.tag_terms);
END;
