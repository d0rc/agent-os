package vectors

import (
	"context"
	"github.com/d0rc/agent-os/settings"
	qdrantgo "github.com/henomis/qdrant-go"
	"github.com/henomis/qdrant-go/request"
	"github.com/henomis/qdrant-go/response"
)

type QdrantClient struct {
	client *qdrantgo.Client
}

func NewQdrantClient(config *settings.VectorDBConfigurationSection) (VectorDB, error) {
	client := qdrantgo.New(config.Endpoint, config.APIToken)

	return &QdrantClient{
		client: client,
	}, nil
}

func (q *QdrantClient) CreateCollection(collection string, params *CollectionParameters) error {
	//onDisk := true
	resp := &response.CollectionCreate{}
	err := q.client.CollectionCreate(
		context.Background(),
		&request.CollectionCreate{
			CollectionName: collection,
			Vectors: request.VectorsParams{
				Size:     params.Dimensions,
				Distance: request.Distance(params.DistanceMeasure),
				// OnDisk:   &onDisk,
			},
		},
		resp,
	)

	return err
}

func (q *QdrantClient) InsertVectors(collection string, vectors []*Vector) error {
	resp := &response.PointUpsert{
		Response: response.Response{},
		Result:   response.PointOperationResult{},
	}

	points := make([]request.Point, len(vectors))
	for idx, vector := range vectors {
		points[idx] = request.Point{
			ID:      vector.Id,
			Vector:  vector.VecF64,
			Payload: nil,
		}
	}

	wait := true
	err := q.client.PointUpsert(
		context.Background(),
		&request.PointUpsert{
			CollectionName: collection,
			Wait:           &wait,
			Points:         points,
		},
		resp)

	return err
}

func (q *QdrantClient) FindNeighborhoods(collection string, vector *Vector, params *SearchSettings) ([]*Vector, error) {
	resp := &response.PointSearch{}

	err := q.client.PointSearch(context.Background(), &request.PointSearch{
		CollectionName: collection,
		Consistency:    nil,
		Vector:         vector.VecF64,
		Filter:         request.Filter{},
		Params:         nil,
		Limit:          0,
		Offset:         0,
		WithPayload:    nil,
		WithVector:     nil,
		ScoreThreshold: nil,
	}, resp)
	if err != nil {
		return nil, err
	}

	result := make([]*Vector, len(resp.Result))
	for idx, point := range resp.Result {
		result[idx] = &Vector{
			Id:     point.ID,
			VecF64: point.Vector,
		}
	}

	return result, nil
}
