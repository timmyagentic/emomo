package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/logger"
	"github.com/timmy/emomo/internal/repository"
)

func TestSearchServiceGetAvailableCollectionsUsesConfiguredKeys(t *testing.T) {
	t.Parallel()

	searchService := NewSearchService(nil, nil, nil, nil, nil, nil, nil, &SearchConfig{
		DefaultCollection: "qwen3",
	})

	searchService.RegisterCollection("jina", nil, nil)
	searchService.RegisterCollection("qwen3", nil, nil)
	searchService.RegisterCollection("alpha", nil, nil)

	got := searchService.GetAvailableCollections()
	want := []string{"qwen3", "alpha", "jina"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetAvailableCollections() = %v, want %v", got, want)
	}
}

func TestSearchServiceGetAvailableProfilesUsesConfiguredDefault(t *testing.T) {
	t.Parallel()

	searchService := NewSearchService(nil, nil, nil, nil, nil, nil, nil, &SearchConfig{
		DefaultProfile: "qwen3vl",
	})
	searchService.RegisterProfile("legacy", nil, nil, nil, nil)
	searchService.RegisterProfile("qwen3vl", nil, nil, nil, nil)
	searchService.RegisterProfile("alpha", nil, nil, nil, nil)

	got := searchService.GetAvailableProfiles()
	want := []string{"qwen3vl", "alpha", "legacy"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetAvailableProfiles() = %v, want %v", got, want)
	}
}

func TestResolveRequestedProfileFallsBackWhenDefaultProfileUnregistered(t *testing.T) {
	t.Parallel()

	searchService := NewSearchService(nil, nil, nil, nil, nil, nil, nil, &SearchConfig{
		DefaultProfile: "qwen3vl",
	})

	_, _, ok, err := searchService.resolveRequestedProfile(&pb.SearchRequest{})
	if err != nil {
		t.Fatalf("resolveRequestedProfile() error = %v, want nil", err)
	}
	if ok {
		t.Fatal("resolveRequestedProfile() ok = true, want false")
	}

	_, _, _, err = searchService.resolveRequestedProfile(&pb.SearchRequest{Profile: "qwen3vl"})
	if err == nil {
		t.Fatal("resolveRequestedProfile() explicit profile error = nil, want error")
	}
}

func TestApplyTopKDefaultsUses100WhenUnset(t *testing.T) {
	t.Parallel()

	for _, topK := range []int32{0, -1} {
		if got := applyTopKDefaults(topK); got != 100 {
			t.Fatalf("applyTopKDefaults(%d) = %d, want 100", topK, got)
		}
	}
}

func TestTextSearchUsesRequestIDAsSearchID(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&logger.Config{
		Level:       "info",
		Format:      "json",
		Output:      &buf,
		ServiceName: "search-test",
	})
	ctx := log.WithContext(context.Background())
	ctx = logger.SetRequestID(ctx, "req-123")

	searchService := NewSearchService(
		nil,
		nil,
		nil,
		&failingEmbeddingProvider{},
		nil,
		nil,
		log,
		&SearchConfig{},
	)

	_, err := searchService.TextSearch(ctx, &pb.SearchRequest{Query: "hello", TopK: 1})
	if err == nil {
		t.Fatal("TextSearch() error = nil, want embedding error")
	}

	entry := findSearchLogEntry(t, buf.Bytes(), "Performing text search")
	if got := entry[logger.FieldRequestID]; got != "req-123" {
		t.Fatalf("request_id = %#v, want req-123", got)
	}
	if got := entry[logger.FieldSearchID]; got != "req-123" {
		t.Fatalf("search_id = %#v, want req-123", got)
	}
}

func TestFuseProfileResultsCombinesRoutesByMemeID(t *testing.T) {
	t.Parallel()

	imageResults := []repository.SearchResult{
		{ID: "point-image-1", Payload: &repository.MemePayload{MemeID: "meme-a", StorageURL: "a.jpg"}},
		{ID: "point-image-2", Payload: &repository.MemePayload{MemeID: "meme-b", StorageURL: "b.jpg"}},
	}
	captionResults := []repository.SearchResult{
		{ID: "point-caption-1", Payload: &repository.MemePayload{MemeID: "meme-b", StorageURL: "b.jpg"}},
	}
	keywordResults := []repository.SearchResult{
		{ID: "point-keyword-1", Payload: &repository.MemePayload{MemeID: "meme-b", StorageURL: "b.jpg"}},
		{ID: "point-keyword-2", Payload: &repository.MemePayload{MemeID: "meme-c", StorageURL: "c.jpg"}},
	}

	results := fuseProfileResults(imageResults, captionResults, keywordResults, RetrievalWeights{
		Image:   0.6,
		Caption: 0.3,
		Keyword: 0.1,
	}, 20)

	if len(results) != 3 {
		t.Fatalf("fuseProfileResults returned %d results, want 3", len(results))
	}
	if results[0].GetMeme().GetId() != "meme-b" {
		t.Fatalf("first result ID = %q, want meme-b", results[0].GetMeme().GetId())
	}
	if results[0].GetScore() != 1 {
		t.Fatalf("first result score = %v, want normalized score 1", results[0].GetScore())
	}
}

type failingEmbeddingProvider struct{}

func (p *failingEmbeddingProvider) Embed(context.Context, string) ([]float32, error) {
	return nil, errors.New("unexpected Embed call")
}

func (p *failingEmbeddingProvider) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("unexpected EmbedBatch call")
}

func (p *failingEmbeddingProvider) EmbedQuery(context.Context, string) ([]float32, error) {
	return nil, errors.New("embedding failed")
}

func (p *failingEmbeddingProvider) EmbedDocument(context.Context, EmbeddingDocument) ([]float32, error) {
	return nil, errors.New("unexpected EmbedDocument call")
}

func (p *failingEmbeddingProvider) GetModel() string {
	return "failing-test"
}

func (p *failingEmbeddingProvider) GetDimensions() int {
	return 1
}

func findSearchLogEntry(t *testing.T, raw []byte, messagePrefix string) map[string]interface{} {
	t.Helper()

	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("failed to decode log line %q: %v", string(line), err)
		}
		message, _ := entry["message"].(string)
		if strings.HasPrefix(message, messagePrefix) {
			return entry
		}
	}
	t.Fatalf("log entry with prefix %q not found in %s", messagePrefix, raw)
	return nil
}
