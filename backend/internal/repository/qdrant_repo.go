package repository

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/google/uuid"
	pb "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	// DefaultVectorDimension is the default embedding dimension (Jina)
	DefaultVectorDimension = 1024
	DenseVectorName        = "dense"
	SparseVectorName       = "bm25"
	SparseVectorModel      = "qdrant/bm25"
)

// QdrantConnectionConfig holds configuration for Qdrant connection.
type QdrantConnectionConfig struct {
	Host            string
	Port            int
	Collection      string
	APIKey          string // Qdrant Cloud API Key (enables TLS automatically)
	UseTLS          bool   // Explicitly enable TLS without API Key
	VectorDimension int    // Vector dimension for this collection (default: 1024)
}

// apiKeyInterceptor creates a unary interceptor that adds API key to metadata
func apiKeyInterceptor(apiKey string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, "api-key", apiKey)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// QdrantRepository handles vector operations with Qdrant.
type QdrantRepository struct {
	conn            *grpc.ClientConn
	pointsClient    pb.PointsClient
	collectClient   pb.CollectionsClient
	collectionName  string
	vectorDimension int
}

// NewQdrantRepository creates a new QdrantRepository.
// Parameters:
//   - cfg: Qdrant connection settings including host, port, and collection.
//
// Returns:
//   - *QdrantRepository: initialized repository instance.
//   - error: non-nil if the connection cannot be established.
//
// Supports both local Qdrant (insecure) and Qdrant Cloud (TLS + API Key).
func NewQdrantRepository(cfg *QdrantConnectionConfig) (*QdrantRepository, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// Build gRPC dial options
	var opts []grpc.DialOption

	// Determine if TLS should be used
	// TLS is enabled if: APIKey is set OR UseTLS is explicitly true
	useTLS := cfg.UseTLS || cfg.APIKey != ""

	if useTLS {
		// Use TLS with system root certificates (TLS 1.3 minimum for Qdrant Cloud)
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS13,
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.WithTransportCredentials(creds))

		// Add API Key authentication if provided (using unary interceptor)
		if cfg.APIKey != "" {
			opts = append(opts, grpc.WithUnaryInterceptor(apiKeyInterceptor(cfg.APIKey)))
		}
	} else {
		// Local mode: no TLS, no authentication
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to qdrant: %w", err)
	}

	// Use default dimension if not specified
	vectorDim := cfg.VectorDimension
	if vectorDim <= 0 {
		vectorDim = DefaultVectorDimension
	}

	return &QdrantRepository{
		conn:            conn,
		pointsClient:    pb.NewPointsClient(conn),
		collectClient:   pb.NewCollectionsClient(conn),
		collectionName:  cfg.Collection,
		vectorDimension: vectorDim,
	}, nil
}

// Close closes the gRPC connection.
// Parameters: none.
// Returns:
//   - error: non-nil if closing the connection fails.
func (r *QdrantRepository) Close() error {
	return r.conn.Close()
}

// EnsureCollection creates the collection if it doesn't exist.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//
// Returns:
//   - error: non-nil if the collection check/create fails.
func (r *QdrantRepository) EnsureCollection(ctx context.Context) error {
	// Check if collection exists
	info, err := r.collectClient.Get(ctx, &pb.GetCollectionInfoRequest{
		CollectionName: r.collectionName,
	})
	if err == nil {
		if info != nil && info.Result != nil {
			var sparseConfig *pb.SparseVectorConfig
			if config := info.Result.GetConfig(); config != nil {
				if params := config.GetParams(); params != nil {
					sparseConfig = params.GetSparseVectorsConfig()
				}
			}
			return r.ensureSparseConfig(ctx, sparseConfig)
		}
		return nil
	}

	// Create collection with named vectors (dense + sparse)
	_, err = r.collectClient.Create(ctx, &pb.CreateCollection{
		CollectionName: r.collectionName,
		VectorsConfig: &pb.VectorsConfig{
			Config: &pb.VectorsConfig_ParamsMap{
				ParamsMap: &pb.VectorParamsMap{
					Map: map[string]*pb.VectorParams{
						DenseVectorName: {
							Size:     uint64(r.vectorDimension),
							Distance: pb.Distance_Cosine,
						},
					},
				},
			},
		},
		SparseVectorsConfig: pb.NewSparseVectorsConfig(map[string]*pb.SparseVectorParams{
			SparseVectorName: {},
		}),
		HnswConfig: &pb.HnswConfigDiff{
			M:                 optionalUint64(16),
			EfConstruct:       optionalUint64(128),
			FullScanThreshold: optionalUint64(10000),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	return nil
}

func (r *QdrantRepository) ensureSparseConfig(ctx context.Context, existing *pb.SparseVectorConfig) error {
	if existing != nil {
		if _, ok := existing.GetMap()[SparseVectorName]; ok {
			return nil
		}
	}

	paramsMap := make(map[string]*pb.SparseVectorParams)
	if existing != nil {
		for name, params := range existing.GetMap() {
			paramsMap[name] = params
		}
	}
	paramsMap[SparseVectorName] = &pb.SparseVectorParams{}

	_, err := r.collectClient.Update(ctx, &pb.UpdateCollection{
		CollectionName:      r.collectionName,
		SparseVectorsConfig: pb.NewSparseVectorsConfig(paramsMap),
	})
	if err != nil {
		return fmt.Errorf("failed to update sparse vectors config: %w", err)
	}

	return nil
}

// GetCollectionName returns the collection name.
// Parameters: none.
// Returns:
//   - string: configured collection name.
func (r *QdrantRepository) GetCollectionName() string {
	return r.collectionName
}

// GetVectorDimension returns the vector dimension for this collection.
// Parameters: none.
// Returns:
//   - int: embedding vector size for the collection.
func (r *QdrantRepository) GetVectorDimension() int {
	return r.vectorDimension
}

func optionalUint64(v uint64) *uint64 {
	return &v
}

func optionalUint32(v uint32) *uint32 {
	return &v
}

func optionalString(v string) *string {
	return &v
}

// MemePayload represents the payload stored with each vector.
type MemePayload struct {
	MemeID         string   `json:"meme_id"`
	Category       string   `json:"category"`
	Tags           []string `json:"tags"`
	VLMDescription string   `json:"vlm_description"`
	OCRText        string   `json:"ocr_text"`
	TextPresence   string   `json:"text_presence"`
	StorageURL     string   `json:"storage_url"`
}

func memePayloadToQdrant(payload *MemePayload) map[string]*pb.Value {
	return map[string]*pb.Value{
		"meme_id":         {Kind: &pb.Value_StringValue{StringValue: payload.MemeID}},
		"category":        {Kind: &pb.Value_StringValue{StringValue: payload.Category}},
		"vlm_description": {Kind: &pb.Value_StringValue{StringValue: payload.VLMDescription}},
		"ocr_text":        {Kind: &pb.Value_StringValue{StringValue: payload.OCRText}},
		"text_presence":   {Kind: &pb.Value_StringValue{StringValue: payload.TextPresence}},
		"storage_url":     {Kind: &pb.Value_StringValue{StringValue: payload.StorageURL}},
		"tags":            tagsToValue(payload.Tags),
	}
}

// Upsert inserts or updates a vector with payload.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - pointID: UUID string for the vector point.
//   - vector: embedding vector values.
//   - payload: metadata payload stored with the vector.
//
// Returns:
//   - error: non-nil if the upsert fails.
func (r *QdrantRepository) Upsert(ctx context.Context, pointID string, vector []float32, payload *MemePayload) error {
	// Parse UUID
	uid, err := uuid.Parse(pointID)
	if err != nil {
		return fmt.Errorf("invalid point ID: %w", err)
	}

	points := []*pb.PointStruct{
		{
			Id: &pb.PointId{
				PointIdOptions: &pb.PointId_Uuid{
					Uuid: uid.String(),
				},
			},
			Vectors: pb.NewVectorsMap(map[string]*pb.Vector{
				DenseVectorName: pb.NewVectorDense(vector),
			}),
			Payload: memePayloadToQdrant(payload),
		},
	}

	_, err = r.pointsClient.Upsert(ctx, &pb.UpsertPoints{
		CollectionName: r.collectionName,
		Points:         points,
	})
	if err != nil {
		return fmt.Errorf("failed to upsert point: %w", err)
	}

	return nil
}

// UpsertHybrid inserts a dense vector and sparse BM25 vector in a single atomic operation.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - pointID: UUID string for the vector point.
//   - vector: dense embedding vector values.
//   - bm25Text: text used for server-side BM25 sparse vector generation.
//   - payload: metadata payload stored with the vector.
//
// Returns:
//   - error: non-nil if the upsert fails.
func (r *QdrantRepository) UpsertHybrid(ctx context.Context, pointID string, vector []float32, bm25Text string, payload *MemePayload) error {
	uid, err := uuid.Parse(pointID)
	if err != nil {
		return fmt.Errorf("invalid point ID: %w", err)
	}

	// Build named vectors map
	vectorsMap := map[string]*pb.Vector{
		DenseVectorName: pb.NewVectorDense(vector),
	}

	// Add sparse BM25 vector if text provided
	bm25Text = strings.TrimSpace(bm25Text)
	if bm25Text != "" {
		doc := &pb.Document{
			Text:  bm25Text,
			Model: SparseVectorModel,
		}
		vectorsMap[SparseVectorName] = pb.NewVectorDocument(doc)
	}

	points := []*pb.PointStruct{
		{
			Id: &pb.PointId{
				PointIdOptions: &pb.PointId_Uuid{
					Uuid: uid.String(),
				},
			},
			Vectors: pb.NewVectorsMap(vectorsMap),
			Payload: memePayloadToQdrant(payload),
		},
	}

	_, err = r.pointsClient.Upsert(ctx, &pb.UpsertPoints{
		CollectionName: r.collectionName,
		Points:         points,
	})
	if err != nil {
		return fmt.Errorf("failed to upsert point: %w", err)
	}

	return nil
}

// UpsertSparse inserts or updates a sparse BM25-only point with payload.
func (r *QdrantRepository) UpsertSparse(ctx context.Context, pointID string, bm25Text string, payload *MemePayload) error {
	uid, err := uuid.Parse(pointID)
	if err != nil {
		return fmt.Errorf("invalid point ID: %w", err)
	}

	bm25Text = strings.TrimSpace(bm25Text)
	if bm25Text == "" {
		return fmt.Errorf("sparse vector text is empty")
	}

	doc := &pb.Document{
		Text:  bm25Text,
		Model: SparseVectorModel,
	}
	points := []*pb.PointStruct{
		{
			Id: &pb.PointId{
				PointIdOptions: &pb.PointId_Uuid{
					Uuid: uid.String(),
				},
			},
			Vectors: pb.NewVectorsMap(map[string]*pb.Vector{
				SparseVectorName: pb.NewVectorDocument(doc),
			}),
			Payload: memePayloadToQdrant(payload),
		},
	}

	_, err = r.pointsClient.Upsert(ctx, &pb.UpsertPoints{
		CollectionName: r.collectionName,
		Points:         points,
	})
	if err != nil {
		return fmt.Errorf("failed to upsert sparse point: %w", err)
	}

	return nil
}

// UpdateSparseVector updates the BM25 sparse vector for an existing point.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - pointID: UUID string for the vector point.
//   - text: BM25 document text.
//
// Returns:
//   - error: non-nil if the update fails.
func (r *QdrantRepository) UpdateSparseVector(ctx context.Context, pointID string, text string) error {
	uid, err := uuid.Parse(pointID)
	if err != nil {
		return fmt.Errorf("invalid point ID: %w", err)
	}

	doc := &pb.Document{
		Text:  text,
		Model: SparseVectorModel,
	}
	vectors := pb.NewVectorsMap(map[string]*pb.Vector{
		SparseVectorName: pb.NewVectorDocument(doc),
	})

	_, err = r.pointsClient.UpdateVectors(ctx, &pb.UpdatePointVectors{
		CollectionName: r.collectionName,
		Points: []*pb.PointVectors{
			{
				Id: &pb.PointId{
					PointIdOptions: &pb.PointId_Uuid{
						Uuid: uid.String(),
					},
				},
				Vectors: vectors,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to update sparse vector: %w", err)
	}

	return nil
}

func tagsToValue(tags []string) *pb.Value {
	values := make([]*pb.Value, len(tags))
	for i, tag := range tags {
		values[i] = &pb.Value{Kind: &pb.Value_StringValue{StringValue: tag}}
	}
	return &pb.Value{
		Kind: &pb.Value_ListValue{
			ListValue: &pb.ListValue{Values: values},
		},
	}
}

// SearchResult represents a search result from Qdrant.
type SearchResult struct {
	ID      string
	Score   float32
	Payload *MemePayload
}

// HybridSearchPlan defines the prefetch limits and routing for hybrid search.
type HybridSearchPlan struct {
	UseDense    bool
	UseSparse   bool
	DenseLimit  int
	SparseLimit int
	RRFK        uint32
}

// Search performs a vector similarity search.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - vector: query embedding vector.
//   - topK: maximum number of results to return.
//   - filters: optional filter criteria for the search.
//
// Returns:
//   - []SearchResult: ranked search results.
//   - error: non-nil if the search fails.
func (r *QdrantRepository) Search(ctx context.Context, vector []float32, topK int, filters *SearchFilters) ([]SearchResult, error) {
	req := &pb.SearchPoints{
		CollectionName: r.collectionName,
		Vector:         vector,
		VectorName:     optionalString(DenseVectorName),
		Limit:          uint64(topK),
		WithPayload: &pb.WithPayloadSelector{
			SelectorOptions: &pb.WithPayloadSelector_Enable{Enable: true},
		},
	}

	// Apply filters if provided
	if filters != nil {
		req.Filter = buildFilter(filters)
	}

	resp, err := r.pointsClient.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	results := make([]SearchResult, len(resp.Result))
	for i, scored := range resp.Result {
		results[i] = SearchResult{
			ID:      scored.Id.GetUuid(),
			Score:   scored.Score,
			Payload: parsePayload(scored.Payload),
		}
	}

	return results, nil
}

// SparseSearch performs a BM25 sparse-vector search.
func (r *QdrantRepository) SparseSearch(ctx context.Context, queryText string, topK int, filters *SearchFilters) ([]SearchResult, error) {
	queryText = strings.TrimSpace(queryText)
	if queryText == "" {
		return []SearchResult{}, nil
	}
	if topK <= 0 {
		topK = 20
	}

	req := &pb.QueryPoints{
		CollectionName: r.collectionName,
		Query: pb.NewQueryDocument(&pb.Document{
			Text:  queryText,
			Model: SparseVectorModel,
		}),
		Using:       optionalString(SparseVectorName),
		Limit:       optionalUint64(uint64(topK)),
		WithPayload: pb.NewWithPayload(true),
	}
	if filters != nil {
		req.Filter = buildFilter(filters)
	}

	resp, err := r.pointsClient.Query(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to sparse query: %w", err)
	}

	results := make([]SearchResult, len(resp.Result))
	for i, scored := range resp.Result {
		results[i] = SearchResult{
			ID:      scored.Id.GetUuid(),
			Score:   scored.Score,
			Payload: parsePayload(scored.Payload),
		}
	}
	return results, nil
}

// HybridSearch performs a hybrid search using dense embeddings and BM25 sparse vectors with RRF fusion.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - denseVector: dense embedding vector for semantic search.
//   - sparseQuery: query text for BM25 sparse search.
//   - topK: maximum number of results to return.
//   - plan: hybrid query plan (prefetch limits and routing).
//   - filters: optional filter criteria for the search.
//
// Returns:
//   - []SearchResult: ranked search results.
//   - error: non-nil if the search fails.
func (r *QdrantRepository) HybridSearch(
	ctx context.Context,
	denseVector []float32,
	sparseQuery string,
	topK int,
	plan *HybridSearchPlan,
	filters *SearchFilters,
) ([]SearchResult, error) {
	if plan == nil {
		return nil, fmt.Errorf("hybrid search plan is required")
	}
	if topK <= 0 {
		topK = 20
	}

	filter := buildFilter(filters)
	prefetch := make([]*pb.PrefetchQuery, 0, 2)

	if plan.UseDense && len(denseVector) > 0 {
		prefetch = append(prefetch, &pb.PrefetchQuery{
			Query:  pb.NewQueryDense(denseVector),
			Using:  optionalString(DenseVectorName),
			Filter: filter,
			Limit:  optionalUint64(uint64(plan.DenseLimit)),
		})
	}

	sparseQuery = strings.TrimSpace(sparseQuery)
	if plan.UseSparse && sparseQuery != "" {
		doc := &pb.Document{
			Text:  sparseQuery,
			Model: SparseVectorModel,
		}
		prefetch = append(prefetch, &pb.PrefetchQuery{
			Query:  pb.NewQueryDocument(doc),
			Using:  optionalString(SparseVectorName),
			Filter: filter,
			Limit:  optionalUint64(uint64(plan.SparseLimit)),
		})
	}

	if len(prefetch) == 0 {
		return nil, fmt.Errorf("hybrid search requires at least one prefetch query")
	}

	query := pb.NewQueryRRF(&pb.Rrf{K: optionalUint32(plan.RRFK)})

	req := &pb.QueryPoints{
		CollectionName: r.collectionName,
		Prefetch:       prefetch,
		Query:          query,
		Limit:          optionalUint64(uint64(topK)),
		WithPayload:    pb.NewWithPayload(true),
	}

	resp, err := r.pointsClient.Query(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}

	results := make([]SearchResult, len(resp.Result))
	for i, scored := range resp.Result {
		results[i] = SearchResult{
			ID:      scored.Id.GetUuid(),
			Score:   scored.Score,
			Payload: parsePayload(scored.Payload),
		}
	}

	return results, nil
}

// SearchFilters defines optional filters for search.
type SearchFilters struct {
	Category     *string
	TextPresence *string
}

func buildFilter(filters *SearchFilters) *pb.Filter {
	if filters == nil {
		return nil
	}

	var conditions []*pb.Condition

	if filters.Category != nil && *filters.Category != "" {
		conditions = append(conditions, &pb.Condition{
			ConditionOneOf: &pb.Condition_Field{
				Field: &pb.FieldCondition{
					Key: "category",
					Match: &pb.Match{
						MatchValue: &pb.Match_Keyword{Keyword: *filters.Category},
					},
				},
			},
		})
	}

	if filters.TextPresence != nil && *filters.TextPresence != "" {
		conditions = append(conditions, &pb.Condition{
			ConditionOneOf: &pb.Condition_Field{
				Field: &pb.FieldCondition{
					Key: "text_presence",
					Match: &pb.Match{
						MatchValue: &pb.Match_Keyword{Keyword: *filters.TextPresence},
					},
				},
			},
		})
	}

	if len(conditions) == 0 {
		return nil
	}

	return &pb.Filter{
		Must: conditions,
	}
}

func parsePayload(payload map[string]*pb.Value) *MemePayload {
	if payload == nil {
		return nil
	}

	p := &MemePayload{}
	if v, ok := payload["meme_id"]; ok {
		p.MemeID = v.GetStringValue()
	}
	if v, ok := payload["category"]; ok {
		p.Category = v.GetStringValue()
	}
	if v, ok := payload["vlm_description"]; ok {
		p.VLMDescription = v.GetStringValue()
	}
	if v, ok := payload["ocr_text"]; ok {
		p.OCRText = v.GetStringValue()
	}
	if v, ok := payload["text_presence"]; ok {
		p.TextPresence = v.GetStringValue()
	}
	if v, ok := payload["storage_url"]; ok {
		p.StorageURL = v.GetStringValue()
	}
	if v, ok := payload["tags"]; ok {
		if list := v.GetListValue(); list != nil {
			for _, item := range list.Values {
				p.Tags = append(p.Tags, item.GetStringValue())
			}
		}
	}

	return p
}

// PointExists checks if a point exists by ID.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - pointID: UUID string for the vector point.
//
// Returns:
//   - bool: true if the point exists.
//   - error: non-nil if the check fails.
func (r *QdrantRepository) PointExists(ctx context.Context, pointID string) (bool, error) {
	uid, err := uuid.Parse(pointID)
	if err != nil {
		return false, fmt.Errorf("invalid point ID: %w", err)
	}

	resp, err := r.pointsClient.Get(ctx, &pb.GetPoints{
		CollectionName: r.collectionName,
		Ids: []*pb.PointId{
			{PointIdOptions: &pb.PointId_Uuid{Uuid: uid.String()}},
		},
	})
	if err != nil {
		return false, fmt.Errorf("failed to check point existence: %w", err)
	}

	return len(resp.Result) > 0, nil
}

// Delete removes a point by ID.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - pointID: UUID string for the vector point.
//
// Returns:
//   - error: non-nil if the delete fails.
func (r *QdrantRepository) Delete(ctx context.Context, pointID string) error {
	uid, err := uuid.Parse(pointID)
	if err != nil {
		return fmt.Errorf("invalid point ID: %w", err)
	}

	_, err = r.pointsClient.Delete(ctx, &pb.DeletePoints{
		CollectionName: r.collectionName,
		Points: &pb.PointsSelector{
			PointsSelectorOneOf: &pb.PointsSelector_Points{
				Points: &pb.PointsIdsList{
					Ids: []*pb.PointId{
						{PointIdOptions: &pb.PointId_Uuid{Uuid: uid.String()}},
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete point: %w", err)
	}

	return nil
}
