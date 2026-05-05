package source

import "context"

// MemeItem represents a meme item from a data source.
type MemeItem struct {
	SourceID  string // Unique ID within the source
	URL       string // Image URL or local path
	Category  string // Category/folder name; empty for crawler sources whose grouping is metadata, not semantic
	Tags      []string
	Format    string // File format (jpg, png, webp, etc.)
	LocalPath string // Local file path (if available)

	// Metadata is provenance information about where this item came from
	// (crawler source, original title, author, search terms it was found
	// under, etc.). Persisted by ingest into the meme_metadata table; never
	// fed into the search index. Nil means "no extra provenance".
	Metadata *Metadata
}

// Metadata holds provenance fields for a meme item. All fields are optional
// (a fully zero value is meaningful — it means "no metadata"). Source must
// be set when a Metadata is non-nil so the meme_metadata row has a stable
// (source, source_item_id, meme_id) key.
type Metadata struct {
	// Source identifies the upstream system the item came from
	// (e.g. "xiaohongshu", "chinesebqb", "fabiaoqing", "localdir").
	Source string

	// SourceItemID is the item identifier inside Source's namespace
	// (e.g. xiaohongshu note_id). May be empty if the source has no
	// stable per-item ID; in that case the (source, "", meme_id) tuple
	// still uniquely identifies the metadata row.
	SourceItemID string

	// SourceURL is a best-effort URL pointing back to the original item.
	// Empty when not derivable.
	SourceURL string

	// Title is the upstream title for the post / album the image belongs
	// to. May be empty.
	Title string

	// Author is the upstream author / uploader display name. May be empty.
	Author string

	// PublishedAt is the original publish timestamp as a free-form string
	// (sources rarely agree on a format; the column stores the raw value).
	PublishedAt string

	// SearchKeywords records all crawler-side keywords / search terms that
	// caused this item to be collected. Useful for traceability ("why is
	// this in the corpus?") but intentionally not fed back into the
	// search index.
	SearchKeywords []string
}

// Source defines the interface for meme data sources.
type Source interface {
	// GetSourceID returns the unique identifier for this source.
	// Parameters: none.
	// Returns:
	//   - string: stable source identifier.
	GetSourceID() string

	// GetDisplayName returns a human-readable name for this source.
	// Parameters: none.
	// Returns:
	//   - string: display-friendly source name.
	GetDisplayName() string

	// FetchBatch fetches a batch of meme items starting from the given cursor.
	// Parameters:
	//   - ctx: context for cancellation and deadlines.
	//   - cursor: pagination cursor or empty for first page.
	//   - limit: maximum number of items to fetch.
	// Returns:
	//   - items: batch of meme items.
	//   - nextCursor: cursor for the next batch or empty if done.
	//   - err: non-nil if fetching fails.
	FetchBatch(ctx context.Context, cursor string, limit int) (items []MemeItem, nextCursor string, err error)

	// SupportsIncremental returns true if this source supports incremental updates.
	// Parameters: none.
	// Returns:
	//   - bool: true when incremental updates are supported.
	SupportsIncremental() bool
}
