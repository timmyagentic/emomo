package service

import (
	"reflect"
	"testing"

	pb "github.com/timmy/emomo/gen/emomo/v1"
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
