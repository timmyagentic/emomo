package repository

import (
	"context"
	"testing"

	pb "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
)

type recordingPointsClient struct {
	pb.PointsClient
	createFieldIndexes []*pb.CreateFieldIndexCollection
}

func (c *recordingPointsClient) CreateFieldIndex(ctx context.Context, req *pb.CreateFieldIndexCollection, opts ...grpc.CallOption) (*pb.PointsOperationResponse, error) {
	c.createFieldIndexes = append(c.createFieldIndexes, req)
	return &pb.PointsOperationResponse{}, nil
}

type existingCollectionClient struct {
	pb.CollectionsClient
	info *pb.GetCollectionInfoResponse
}

func (c *existingCollectionClient) Get(ctx context.Context, req *pb.GetCollectionInfoRequest, opts ...grpc.CallOption) (*pb.GetCollectionInfoResponse, error) {
	return c.info, nil
}

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

func TestEnsureCollectionCreatesMissingTextPresencePayloadIndex(t *testing.T) {
	t.Parallel()

	pointsClient := &recordingPointsClient{}
	collectionClient := &existingCollectionClient{
		info: &pb.GetCollectionInfoResponse{
			Result: &pb.CollectionInfo{
				Config: &pb.CollectionConfig{
					Params: &pb.CollectionParams{
						SparseVectorsConfig: pb.NewSparseVectorsConfig(map[string]*pb.SparseVectorParams{
							SparseVectorName: {},
						}),
					},
				},
				PayloadSchema: map[string]*pb.PayloadSchemaInfo{
					"category": {DataType: pb.PayloadSchemaType_Keyword},
				},
			},
		},
	}
	repo := &QdrantRepository{
		pointsClient:   pointsClient,
		collectClient:  collectionClient,
		collectionName: "memes",
	}

	if err := repo.EnsureCollection(context.Background()); err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}

	var textPresenceIndex *pb.CreateFieldIndexCollection
	for _, req := range pointsClient.createFieldIndexes {
		if req.GetFieldName() == "text_presence" {
			textPresenceIndex = req
			break
		}
	}
	if textPresenceIndex == nil {
		t.Fatalf("EnsureCollection() did not create text_presence payload index; got %d index requests", len(pointsClient.createFieldIndexes))
	}
	if got := textPresenceIndex.GetCollectionName(); got != "memes" {
		t.Fatalf("text_presence index collection = %q, want memes", got)
	}
	if got := textPresenceIndex.GetFieldType(); got != pb.FieldType_FieldTypeKeyword {
		t.Fatalf("text_presence index type = %v, want keyword", got)
	}
	if !textPresenceIndex.GetWait() {
		t.Fatal("text_presence index wait = false, want true")
	}
}
