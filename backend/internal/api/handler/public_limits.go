package handler

// PublicRequestLimits constrains public API request cost before service calls.
type PublicRequestLimits struct {
	SearchTopKMax       int32
	SearchQueryMaxRunes int
	ListLimitMax        int32
}

type publicRequestLimits = PublicRequestLimits

func normalizePublicRequestLimits(limits PublicRequestLimits) PublicRequestLimits {
	if limits.SearchTopKMax <= 0 {
		limits.SearchTopKMax = 100
	}
	if limits.SearchQueryMaxRunes <= 0 {
		limits.SearchQueryMaxRunes = 160
	}
	if limits.ListLimitMax <= 0 {
		limits.ListLimitMax = 60
	}
	return limits
}
