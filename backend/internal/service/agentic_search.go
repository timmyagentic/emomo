package service

// AgenticSearchService groups LLM planner/reranker dependencies.
type AgenticSearchService struct {
	enabled         bool
	fallbackOnError bool
	rerankTopK      int
	planner         SearchPlanner
	reranker        SearchReranker
}

// AgenticSearchConfig configures the service-level agentic behavior.
type AgenticSearchConfig struct {
	Enabled         bool
	FallbackOnError bool
	RerankTopK      int
}

// NewAgenticSearchService creates an agentic search coordinator.
func NewAgenticSearchService(planner SearchPlanner, reranker SearchReranker, cfg AgenticSearchConfig) *AgenticSearchService {
	return &AgenticSearchService{
		enabled:         cfg.Enabled && planner != nil,
		fallbackOnError: cfg.FallbackOnError,
		rerankTopK:      cfg.RerankTopK,
		planner:         planner,
		reranker:        reranker,
	}
}

func (s *AgenticSearchService) IsEnabled() bool {
	return s != nil && s.enabled && s.planner != nil
}

func (s *AgenticSearchService) FallbackOnError() bool {
	return s == nil || s.fallbackOnError
}

func (s *AgenticSearchService) RerankTopK() int {
	if s == nil || s.rerankTopK <= 0 {
		return 40
	}
	return s.rerankTopK
}
