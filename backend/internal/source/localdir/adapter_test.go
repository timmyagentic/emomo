package localdir

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

// TestFetchBatchScansStaticImagesAndSkipsUnsupportedFiles verifies the
// hand-curated layout (no manifest): unsupported formats are skipped,
// memes.category and memes.tags are intentionally left empty, and per-file
// provenance is captured under MemeItem.Metadata so the path can still be
// traced via meme_metadata.
func TestFetchBatchScansStaticImagesAndSkipsUnsupportedFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "cat", "hello.jpg"), "jpg")
	writeFile(t, filepath.Join(root, "cat", "wave.png"), "png")
	writeFile(t, filepath.Join(root, "root.webp"), "webp")
	writeFile(t, filepath.Join(root, "cat", "animated.gif"), "gif")
	writeFile(t, filepath.Join(root, ".DS_Store"), "ignored")

	adapter := NewAdapter(Options{RootPath: root})
	items, nextCursor, err := adapter.FetchBatch(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("FetchBatch() error = %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("FetchBatch() nextCursor = %q, want empty", nextCursor)
	}
	if len(items) != 3 {
		t.Fatalf("FetchBatch() returned %d items, want 3", len(items))
	}

	byID := map[string]int{}
	for i, item := range items {
		byID[item.SourceID] = i
		if item.Format == "gif" {
			t.Fatalf("FetchBatch() returned GIF item: %+v", item)
		}
		if item.Category != "" {
			t.Fatalf("Category = %q, want empty (memes.category is intentionally left blank)", item.Category)
		}
		if len(item.Tags) != 0 {
			t.Fatalf("Tags = %v, want empty (memes.tags is intentionally left blank)", item.Tags)
		}
		if item.Metadata == nil {
			t.Fatalf("Metadata is nil; want a populated *source.Metadata for traceability")
		}
		if item.Metadata.Source != "localdir" {
			t.Fatalf("Metadata.Source = %q, want localdir", item.Metadata.Source)
		}
	}

	helloIdx, ok := byID["cat/hello.jpg"]
	if !ok {
		t.Fatalf("FetchBatch() missing cat/hello.jpg item; got keys %v", maps(byID))
	}
	hello := items[helloIdx]
	if hello.Metadata.SourceItemID != "cat/hello.jpg" {
		t.Fatalf("Metadata.SourceItemID = %q, want cat/hello.jpg", hello.Metadata.SourceItemID)
	}
	if hello.Metadata.Title != "hello" {
		t.Fatalf("Metadata.Title = %q, want hello (filename stem)", hello.Metadata.Title)
	}
	if hello.Metadata.SourceURL != "" {
		t.Fatalf("Metadata.SourceURL = %q, want empty for handcraft layout", hello.Metadata.SourceURL)
	}
	if len(hello.Metadata.SearchKeywords) != 0 {
		t.Fatalf("Metadata.SearchKeywords = %v, want empty", hello.Metadata.SearchKeywords)
	}
}

// TestFetchBatchUsesXiaohongshuManifestAndQueueMetadata verifies the crawler
// layout: items are filtered by stage2.keep, the search-facing fields stay
// empty, and all crawler provenance (note_id, search keywords, title, author,
// publish date, source URL) ends up under MemeItem.Metadata for persistence
// into meme_metadata.
func TestFetchBatchUsesXiaohongshuManifestAndQueueMetadata(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	keepImage := filepath.Join(root, "65d4a17900000000070079da_1.jpg")
	rejectImage := filepath.Join(root, "65d4a17900000000070079da_2.jpg")
	unlistedImage := filepath.Join(root, "unlisted_1.jpg")
	writeFile(t, keepImage, "webp bytes")
	writeFile(t, rejectImage, "webp bytes")
	writeFile(t, unlistedImage, "webp bytes")

	manifestPath := filepath.Join(root, "stage2_results.jsonl")
	writeFile(t, manifestPath,
		`{"note_id":"65d4a17900000000070079da","filename":"65d4a17900000000070079da_1.jpg","keyword":"学生党表情包","title":"考试周","confidence":0.9,"reason":"熊猫头配文字","keep":true}`+"\n"+
			`{"note_id":"65d4a17900000000070079da","filename":"65d4a17900000000070079da_2.jpg","keyword":"学生党表情包","confidence":0.4,"reason":"非表情包","keep":false}`+"\n")

	queuePath := filepath.Join(root, "stage1_queue.jsonl")
	writeFile(t, queuePath,
		`{"note_id":"65d4a17900000000070079da","filename":"65d4a17900000000070079da_1.jpg","keyword":"学生党表情包","keywords":["学生党表情包","考试表情包"],"title":"考试周","author":"alice","published_at":"2026-01-01"}`+"\n")

	adapter := NewAdapter(Options{
		RootPath:     root,
		SourceID:     "xiaohongshu",
		ManifestPath: manifestPath,
		QueuePath:    queuePath,
	})

	items, nextCursor, err := adapter.FetchBatch(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("FetchBatch() error = %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("FetchBatch() nextCursor = %q, want empty", nextCursor)
	}
	if len(items) != 1 {
		t.Fatalf("FetchBatch() returned %d items, want 1 (only stage2.keep=true should remain)", len(items))
	}

	item := items[0]
	if got := adapter.GetSourceID(); got != "xiaohongshu" {
		t.Fatalf("GetSourceID() = %q, want xiaohongshu", got)
	}
	if item.SourceID != "65d4a17900000000070079da:65d4a17900000000070079da_1.jpg" {
		t.Fatalf("SourceID = %q", item.SourceID)
	}
	if item.LocalPath != keepImage {
		t.Fatalf("LocalPath = %q, want %q", item.LocalPath, keepImage)
	}
	if item.Category != "" {
		t.Fatalf("Category = %q, want empty (crawler keywords are not semantic categories)", item.Category)
	}
	if len(item.Tags) != 0 {
		t.Fatalf("Tags = %v, want empty (crawler metadata never feeds memes.tags)", item.Tags)
	}

	if item.Metadata == nil {
		t.Fatalf("Metadata is nil; want a fully populated metadata struct")
	}
	md := item.Metadata
	if md.Source != "xiaohongshu" {
		t.Fatalf("Metadata.Source = %q, want xiaohongshu", md.Source)
	}
	if md.SourceItemID != "65d4a17900000000070079da" {
		t.Fatalf("Metadata.SourceItemID = %q, want note_id", md.SourceItemID)
	}
	if md.SourceURL != "https://www.xiaohongshu.com/explore/65d4a17900000000070079da" {
		t.Fatalf("Metadata.SourceURL = %q", md.SourceURL)
	}
	if md.Title != "考试周" {
		t.Fatalf("Metadata.Title = %q, want 考试周", md.Title)
	}
	if md.Author != "alice" {
		t.Fatalf("Metadata.Author = %q, want alice", md.Author)
	}
	if md.PublishedAt != "2026-01-01" {
		t.Fatalf("Metadata.PublishedAt = %q, want 2026-01-01", md.PublishedAt)
	}
	for _, kw := range []string{"学生党表情包", "考试表情包"} {
		if !slices.Contains(md.SearchKeywords, kw) {
			t.Fatalf("Metadata.SearchKeywords = %v, want %q", md.SearchKeywords, kw)
		}
	}
	if slices.Contains(md.SearchKeywords, "小红书") {
		t.Fatalf("Metadata.SearchKeywords should not contain the source name: %v", md.SearchKeywords)
	}
	_ = rejectImage
	_ = unlistedImage
}

func maps[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
