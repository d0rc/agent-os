package vectors

type Vector struct {
	Id      string                 `json:"id"`
	VecF64  []float64              `json:"vecF64"`
	Model   *string                `json:"model"`
	Payload map[string]interface{} `json:"payload"`
}

type SearchSettings struct {
	Radius float32
}

type DistanceMeasureType string

const (
	DistanceCosine    DistanceMeasureType = "Cosine"
	DistanceEuclidean DistanceMeasureType = "Euclid"
	DistanceDot       DistanceMeasureType = "Dot"
)

type CollectionParameters struct {
	Dimensions      uint64
	DistanceMeasure DistanceMeasureType
}

type VectorDB interface {
	CreateCollection(string, *CollectionParameters) error
	InsertVectors(string, []*Vector) error
	FindNeighborhoods(string, *Vector, *SearchSettings) ([]*Vector, error)
}
