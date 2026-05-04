package localdir

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/timmy/emomo/internal/source"
)

const (
	defaultSourceID = "localdir"
)

// Options configures the local directory source adapter.
type Options struct {
	RootPath     string
	SourceID     string
	ManifestPath string
	QueuePath    string
}

// Adapter implements source.Source for a local static image directory.
type Adapter struct {
	rootPath     string
	sourceID     string
	manifestPath string
	queuePath    string

	items  []source.MemeItem
	loaded bool
}

type stage2Record struct {
	NoteID     string  `json:"note_id"`
	Filename   string  `json:"filename"`
	Keyword    string  `json:"keyword"`
	Title      string  `json:"title"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
	Keep       bool    `json:"keep"`
}

type queueRecord struct {
	NoteID      string   `json:"note_id"`
	Filename    string   `json:"filename"`
	Keyword     string   `json:"keyword"`
	Keywords    []string `json:"keywords"`
	Title       string   `json:"title"`
	Author      string   `json:"author"`
	PublishedAt string   `json:"published_at"`
}

// NewAdapter creates a local directory source adapter.
func NewAdapter(opts Options) *Adapter {
	sourceID := strings.TrimSpace(opts.SourceID)
	if sourceID == "" {
		sourceID = defaultSourceID
	}

	return &Adapter{
		rootPath:     opts.RootPath,
		sourceID:     sourceID,
		manifestPath: opts.ManifestPath,
		queuePath:    opts.QueuePath,
	}
}

// GetSourceID returns the runtime source identifier used in ingest logs.
func (a *Adapter) GetSourceID() string {
	return a.sourceID
}

// GetDisplayName returns a human-readable source name.
func (a *Adapter) GetDisplayName() string {
	rootPath := strings.TrimSpace(a.rootPath)
	if rootPath == "" {
		return a.sourceID
	}
	return fmt.Sprintf("%s (%s)", a.sourceID, rootPath)
}

// SupportsIncremental returns false because local directory scans are snapshot-based.
func (a *Adapter) SupportsIncremental() bool {
	return false
}

// FetchBatch fetches a page of local image items.
func (a *Adapter) FetchBatch(ctx context.Context, cursor string, limit int) ([]source.MemeItem, string, error) {
	if !a.loaded {
		if err := a.loadItems(); err != nil {
			return nil, "", err
		}
		a.loaded = true
	}

	startIndex := 0
	if cursor != "" {
		var err error
		startIndex, err = strconv.Atoi(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
	}
	if startIndex >= len(a.items) {
		return []source.MemeItem{}, "", nil
	}

	endIndex := startIndex + limit
	if limit <= 0 || endIndex > len(a.items) {
		endIndex = len(a.items)
	}

	nextCursor := ""
	if endIndex < len(a.items) {
		nextCursor = strconv.Itoa(endIndex)
	}

	return a.items[startIndex:endIndex], nextCursor, nil
}

func (a *Adapter) loadItems() error {
	rootPath := strings.TrimSpace(a.rootPath)
	if rootPath == "" {
		return fmt.Errorf("local directory root path is required")
	}
	if _, err := os.Stat(rootPath); err != nil {
		return fmt.Errorf("local directory root path is not accessible: %w", err)
	}

	manifest, err := loadStage2Manifest(a.manifestPath)
	if err != nil {
		return err
	}
	queue, err := loadStage1Queue(a.queuePath)
	if err != nil {
		return err
	}
	useManifest := strings.TrimSpace(a.manifestPath) != ""

	items := make([]source.MemeItem, 0)
	err = filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == rootPath {
			return nil
		}

		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}

		format, ok := formatFromFilename(name)
		if !ok {
			return nil
		}

		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		meta, hasManifest := manifest[name]
		if useManifest && (!hasManifest || !meta.Keep) {
			return nil
		}
		queueMeta := queue[name]

		// memes.category and memes.tags are intentionally left empty for
		// every localdir-sourced item: stage1/stage2 keywords and folder
		// names are crawler/operator metadata about *how* an image was
		// collected, not semantic descriptions of *what* the image depicts.
		// All such provenance is persisted under meme_metadata via
		// MemeItem.Metadata; semantic tagging is left to VLM/OCR
		// downstream in the ingest pipeline.
		item := source.MemeItem{
			SourceID:  sourceIDForItem(relPath, name, meta, queueMeta),
			URL:       path,
			LocalPath: path,
			Format:    format,
			Metadata:  buildMetadata(a.sourceID, relPath, name, meta, queueMeta),
		}
		items = append(items, item)
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan local directory: %w", err)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].SourceID < items[j].SourceID
	})
	a.items = items
	return nil
}

func formatFromFilename(name string) (string, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		return "jpeg", true
	case ".png":
		return "png", true
	case ".webp":
		return "webp", true
	default:
		return "", false
	}
}

func sourceIDForItem(relPath string, filename string, meta stage2Record, queueMeta queueRecord) string {
	noteID := firstNonEmpty(meta.NoteID, queueMeta.NoteID)
	if noteID != "" {
		return noteID + ":" + filename
	}
	return relPath
}

// buildMetadata returns a *source.Metadata populated from whatever provenance
// signals are available for an item. The shape differs by sourceID:
//
//   - sourceID == "xiaohongshu" (or any source that supplies stage1/stage2
//     manifest entries): SourceItemID = note_id, SourceURL points to the
//     xiaohongshu note page, Title/Author/PublishedAt come from stage1,
//     SearchKeywords merges stage1.keyword + stage1.keywords + stage2.keyword.
//
//   - sourceID == "localdir" (hand-curated directory layout, no manifest):
//     SourceItemID = relPath (stable per file), Title = filename stem,
//     no URL/Author/PublishedAt/SearchKeywords. This still gives downstream
//     consumers a way to trace a meme back to its on-disk origin without
//     polluting memes.category / memes.tags.
//
// Returns a non-nil pointer in both cases (a caller may always call .Source).
func buildMetadata(sourceID, relPath, filename string, meta stage2Record, queueMeta queueRecord) *source.Metadata {
	noteID := firstNonEmpty(meta.NoteID, queueMeta.NoteID)
	hasManifest := noteID != "" || queueMeta.Filename != "" || meta.Filename != ""

	md := &source.Metadata{Source: sourceID}

	if hasManifest {
		md.SourceItemID = noteID
		md.Title = firstNonEmpty(meta.Title, queueMeta.Title)
		md.Author = strings.TrimSpace(queueMeta.Author)
		md.PublishedAt = strings.TrimSpace(queueMeta.PublishedAt)
		md.SearchKeywords = collectSearchKeywords(meta, queueMeta)
		if sourceID == "xiaohongshu" && noteID != "" {
			md.SourceURL = "https://www.xiaohongshu.com/explore/" + noteID
		}
		return md
	}

	md.SourceItemID = relPath
	if stem := strings.TrimSuffix(filename, filepath.Ext(filename)); stem != "" {
		md.Title = stem
	}
	return md
}

func collectSearchKeywords(meta stage2Record, queueMeta queueRecord) []string {
	keywords := make([]string, 0, len(queueMeta.Keywords)+2)
	keywords = appendUnique(keywords, queueMeta.Keyword)
	for _, kw := range queueMeta.Keywords {
		keywords = appendUnique(keywords, kw)
	}
	keywords = appendUnique(keywords, meta.Keyword)
	if len(keywords) == 0 {
		return nil
	}
	return keywords
}

func appendUnique(items []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return items
	}
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func loadStage2Manifest(path string) (map[string]stage2Record, error) {
	records := make(map[string]stage2Record)
	if strings.TrimSpace(path) == "" {
		return records, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open stage2 manifest: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record stage2Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("failed to parse stage2 manifest line: %w", err)
		}
		if record.Filename == "" {
			continue
		}
		records[record.Filename] = record
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read stage2 manifest: %w", err)
	}

	return records, nil
}

func loadStage1Queue(path string) (map[string]queueRecord, error) {
	records := make(map[string]queueRecord)
	if strings.TrimSpace(path) == "" {
		return records, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open stage1 queue: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record queueRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("failed to parse stage1 queue line: %w", err)
		}
		if record.Filename == "" {
			continue
		}
		records[record.Filename] = record
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read stage1 queue: %w", err)
	}

	return records, nil
}
