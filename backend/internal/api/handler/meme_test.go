package handler

import "testing"

func TestNormalizeListMemesParamsCapsLimitAndOffset(t *testing.T) {
	req := normalizeListMemesParams("120", "-12", "reaction", publicRequestLimits{
		ListLimitMax: 50,
	})

	if req.GetLimit() != 50 {
		t.Fatalf("limit = %d, want 50", req.GetLimit())
	}
	if req.GetOffset() != 0 {
		t.Fatalf("offset = %d, want 0", req.GetOffset())
	}
	if req.GetCategory() != "reaction" {
		t.Fatalf("category = %q, want reaction", req.GetCategory())
	}
}

func TestNormalizeListMemesParamsUsesDefaults(t *testing.T) {
	req := normalizeListMemesParams("", "", "", publicRequestLimits{
		ListLimitMax: 50,
	})

	if req.GetLimit() != 20 {
		t.Fatalf("limit = %d, want default 20", req.GetLimit())
	}
	if req.GetOffset() != 0 {
		t.Fatalf("offset = %d, want 0", req.GetOffset())
	}
}
