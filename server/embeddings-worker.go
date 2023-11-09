package server

import (
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/settings"
	"github.com/d0rc/agent-os/vectors"
)

type VectorDBType string

const (
	VDB_QDRANT VectorDBType = "qdrant"
)

type EmbeddingsQueueRecord struct {
	Id           int64  `db:"id"`
	QueueName    string `db:"queue_name"`
	QueuePointer int64  `db:"queue_pointer"`
}

func (ctx *Context) backgroundEmbeddingsWorker() {
	// let's see what we have in our vector DBs configs

	for _, vectorDB := range ctx.Config.VectorDBs {
		if vectorDB.Type == string(VDB_QDRANT) {
			err := ctx.startQdrant(&vectorDB)
			if err != nil {
				ctx.Log.Error().
					Err(err).
					Msgf("error starting qdrant server: %v", err)
			}
		}
	}

	if len(ctx.VectorDBs) == 0 {
		ctx.Log.Warn().Msg("exiting background vector-embedding thread, as no storage provided")
	}

	defaultVectorStorage := ctx.VectorDBs[0]
	// let's find out what models for embeddings do we have at hand
	// this can only be done by using completion command which will scan available models
	embeddingEngines := make([]*engines.InferenceEngine, 0)
	for idx, inferenceEngine := range engines.GetInferenceEngines() {
		if inferenceEngine.EmbeddingsDims != nil && *inferenceEngine.EmbeddingsDims > 0 {
			embeddingEngines = append(embeddingEngines, engines.GetInferenceEngines()[idx])
		}
	}

	defaultEmbeddingEngine := embeddingEngines[0]
	// let's start processing embeddings, first lets read our queue pointer
	// and then start processing embeddings
	pointers := make([]EmbeddingsQueueRecord, 0, 1)
	err := ctx.Storage.Db.GetStructsSlice("get-embeddings-queue-pointer",
		&pointers,
		"embeddings-llm-cache")
	if err != nil {
		ctx.Log.Error().Err(err).Msg("error getting embeddings queue pointer")
		return
	}
	if len(pointers) == 0 {
		// there's no queue, it means we've never processed embeddings
		// so, let's create a new collection in our vector storage
		err = defaultVectorStorage.CreateCollection("embeddings-llm-cache", &vectors.CollectionParameters{
			Dimensions: *defaultEmbeddingEngine.EmbeddingsDims,
		})
		if err != nil {
			ctx.Log.Error().Err(err).
				Str("collection", "embeddings-llm-cache").
				Msg("error creating embeddings collection")
			return
		} else {
			ctx.Log.Info().Str("collection", "embeddings-llm-cache").Msg("created embeddings collection")
		}
	}
}

func (ctx *Context) startQdrant(vectorDB *settings.VectorDBConfigurationSection) error {
	vectorDb, err := vectors.NewQdrantClient(vectorDB)
	if err != nil {
		return err
	}

	ctx.VectorDBs = append(ctx.VectorDBs, vectorDb)
	return nil
}
