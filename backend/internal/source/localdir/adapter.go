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
	defaultCategory = "未分类"
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

		category := categoryFromRelPath(relPath)
		if meta.Keyword != "" {
			category = meta.Keyword
		} else if queueMeta.Keyword != "" {
			category = queueMeta.Keyword
		}

		item := source.MemeItem{
			SourceID:  sourceIDForItem(relPath, name, meta, queueMeta),
			URL:       path,
			LocalPath: path,
			Category:  category,
			Format:    format,
			Tags:      tagsForItem(a.sourceID, relPath, meta, queueMeta, category),
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

func categoryFromRelPath(relPath string) string {
	parts := strings.Split(relPath, "/")
	if len(parts) <= 1 || strings.TrimSpace(parts[0]) == "" {
		return defaultCategory
	}
	return parts[0]
}

func sourceIDForItem(relPath string, filename string, meta stage2Record, queueMeta queueRecord) string {
	noteID := firstNonEmpty(meta.NoteID, queueMeta.NoteID)
	if noteID != "" {
		return noteID + ":" + filename
	}
	return relPath
}

func tagsForItem(sourceID string, relPath string, meta stage2Record, queueMeta queueRecord, category string) []string {
	tags := make([]string, 0, 6)
	tags = appendUnique(tags, category)
	for _, tag := range tagsFromRelPath(relPath) {
		tags = appendUnique(tags, tag)
	}
	tags = appendUnique(tags, meta.Keyword)
	tags = appendUnique(tags, queueMeta.Keyword)
	for _, keyword := range queueMeta.Keywords {
		tags = appendUnique(tags, keyword)
	}
	if sourceID == "xiaohongshu" {
		tags = appendUnique(tags, "小红书")
	}
	return tags
}

func tagsFromRelPath(relPath string) []string {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	tags := make([]string, 0, len(parts)+2)
	for i, part := range parts {
		if i == len(parts)-1 {
			part = strings.TrimSuffix(part, filepath.Ext(part))
		}
		for _, token := range splitTagTokens(part) {
			tags = appendUnique(tags, token)
		}
	}
	return tags
}

func splitTagTokens(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == '_' || r == '-' || r == ' ' || r == '\t'
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if len([]rune(field)) > 1 && !isNumeric(field) {
			tokens = append(tokens, field)
		}
	}
	return tokens
}

func isNumeric(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func appendUnique(items []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" || item == defaultCategory {
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
