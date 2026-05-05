package repository

import "testing"

func TestBuildFilterIncludesTextPresence(t *testing.T) {
	t.Parallel()

	category := "reaction"
	textPresence := "with_text"

	filter := buildFilter(&SearchFilters{
		Category:     &category,
		TextPresence: &textPresence,
	})

	if filter == nil {
		t.Fatal("buildFilter() returned nil")
	}
	if len(filter.Must) != 2 {
		t.Fatalf("buildFilter() must conditions = %d, want 2", len(filter.Must))
	}

	got := map[string]string{}
	for _, condition := range filter.Must {
		field := condition.GetField()
		if field == nil || field.Match == nil {
			t.Fatalf("condition missing field match: %#v", condition)
		}
		got[field.Key] = field.Match.GetKeyword()
	}

	if got["category"] != category {
		t.Fatalf("category filter = %q, want %q", got["category"], category)
	}
	if got["text_presence"] != textPresence {
		t.Fatalf("text_presence filter = %q, want %q", got["text_presence"], textPresence)
	}
}
