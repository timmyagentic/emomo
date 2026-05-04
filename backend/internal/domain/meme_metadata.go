package domain

import "time"

// MemeMetadata records provenance metadata for a meme: where it came from,
// what it was indexed under at the source, the original title/author, etc.
//
// This table is intentionally separate from `memes` to keep the search-facing
// table (memes / meme_annotations / meme_vectors) free of crawler-specific
// fields. Metadata stored here:
//
//   - never feeds the search index (caption embedding, BM25 sparse, Qdrant
//     payload). The ingest pipeline reads only `memes.category` / `memes.tags`
//     / `meme_annotations` for those signals;
//   - is keyed by (source, source_item_id, meme_id), so:
//   - the same image (deduplicated by content hash → same `meme_id`) can
//     accumulate multiple metadata rows when crawled from multiple sources;
//   - re-importing the same source item updates in place rather than
//     creating a duplicate.
type MemeMetadata struct {
	ID             string      `gorm:"type:text;primaryKey" json:"id"`
	MemeID         string      `gorm:"type:text;not null;index:idx_meme_metadata_meme_id;uniqueIndex:idx_meme_metadata_source_item_meme,priority:3" json:"meme_id"`
	Source         string      `gorm:"type:text;not null;index:idx_meme_metadata_source;uniqueIndex:idx_meme_metadata_source_item_meme,priority:1" json:"source"`
	SourceItemID   string      `gorm:"type:text;uniqueIndex:idx_meme_metadata_source_item_meme,priority:2" json:"source_item_id"`
	SourceURL      string      `gorm:"type:text" json:"source_url"`
	Title          string      `gorm:"type:text" json:"title"`
	Author         string      `gorm:"type:text" json:"author"`
	PublishedAt    string      `gorm:"type:text" json:"published_at"`
	SearchKeywords StringArray `gorm:"type:text" json:"search_keywords"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// TableName returns the database table name for MemeMetadata.
func (MemeMetadata) TableName() string {
	return "meme_metadata"
}
