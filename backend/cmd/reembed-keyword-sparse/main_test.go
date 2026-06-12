package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/persistence"
)

func TestBuildKeywordCandidateQuerySkipsBM25CompatibleVectors(t *testing.T) {
	t.Parallel()

	query, args := buildKeywordCandidateQuery("meme_caption_qwen3vl_1024", 50)

	if !strings.Contains(query, "mv.vector_type IN (?, ?)") {
		t.Fatalf("query does not filter compatible vector types: %s", query)
	}
	if strings.Contains(query, "VECTOR_TYPE_IMAGE") {
		t.Fatalf("query should not treat image vectors as keyword sparse coverage: %s", query)
	}
	if !strings.Contains(query, "LIMIT ?") {
		t.Fatalf("query does not include requested limit: %s", query)
	}

	want := []any{
		"meme_caption_qwen3vl_1024",
		int32(pb.VectorType_VECTOR_TYPE_CAPTION),
		int32(pb.VectorType_VECTOR_TYPE_KEYWORD),
		50,
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestBuildKeywordCandidateQueryOmitsLimitWhenUnlimited(t *testing.T) {
	t.Parallel()

	query, args := buildKeywordCandidateQuery("meme_caption_qwen3vl_1024", 0)

	if strings.Contains(query, "LIMIT ?") {
		t.Fatalf("query should not include a limit when limit=0: %s", query)
	}
	want := []any{
		"meme_caption_qwen3vl_1024",
		int32(pb.VectorType_VECTOR_TYPE_CAPTION),
		int32(pb.VectorType_VECTOR_TYPE_KEYWORD),
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestTextPresenceString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		candidate keywordCandidate
		want      string
	}{
		{
			name:      "missing annotation",
			candidate: keywordCandidate{},
			want:      persistence.TextPresenceToString(pb.TextPresence_TEXT_PRESENCE_UNKNOWN),
		},
		{
			name: "annotation with text",
			candidate: keywordCandidate{
				AnnotationID: "annotation-1",
				HasText:      true,
			},
			want: persistence.TextPresenceToString(pb.TextPresence_TEXT_PRESENCE_WITH_TEXT),
		},
		{
			name: "annotation without text",
			candidate: keywordCandidate{
				AnnotationID: "annotation-1",
				HasText:      false,
			},
			want: persistence.TextPresenceToString(pb.TextPresence_TEXT_PRESENCE_WITHOUT_TEXT),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := textPresenceString(tt.candidate); got != tt.want {
				t.Fatalf("textPresenceString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWithRetryRetriesTransientErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	attempts := 0
	err := withRetry(ctx, 3, time.Nanosecond, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withRetry() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestWithRetryStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	attempts := 0
	err := withRetry(ctx, 3, time.Nanosecond, func() error {
		attempts++
		return errors.New("temporary")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("withRetry() error = %v, want context.Canceled", err)
	}
	if attempts != 0 {
		t.Fatalf("attempts = %d, want 0", attempts)
	}
}
