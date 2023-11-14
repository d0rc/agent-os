package process_embeddings

import (
	"crypto/sha512"
	"fmt"
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/server"
	"github.com/d0rc/agent-os/settings"
	"github.com/d0rc/agent-os/vectors"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"time"
)

type VectorDBType string

const (
	VDB_QDRANT VectorDBType = "qdrant"
)

const defaultCollectionNamePrefix = "embeddings-llm-cache"

type EmbeddingsQueueRecord struct {
	Id           int64  `db:"id"`
	QueueName    string `db:"queue_name"`
	QueuePointer int64  `db:"queue_pointer"`
}

func BackgroundEmbeddingsWorker(ctx *server.Context, name string) {
	// let's see what we have in our vector DBs configs
	lg := ctx.Log.With().Str("bg-wrk", "embeddings").Logger()
	for _, vectorDB := range ctx.Config.VectorDBs {
		if vectorDB.Type == string(VDB_QDRANT) {
			err := startQdrant(&vectorDB, ctx)
			if err != nil {
				lg.Error().
					Err(err).
					Msgf("error starting qdrant server: %v", err)
			}
		}
	}

	if len(ctx.VectorDBs) == 0 {
		lg.Warn().Msg("exiting background vector-embedding thread, as no storage provided")
	}

	defaultVectorStorage := ctx.VectorDBs[0]
	defaultCollectionName := fmt.Sprintf("%s-%d",
		defaultCollectionNamePrefix,
		ctx.GetDefaultEmbeddingDims())
	// let's find out what models for embeddings do we have at hand
	// this can only be done by using completion command which will scan available models
	// let's start processing embeddings, first lets read our queue pointer
	// and then start processing embeddings
	pointers := make([]EmbeddingsQueueRecord, 0, 1)
	err := ctx.Storage.Db.GetStructsSlice("get-embeddings-queue-pointer",
		&pointers,
		defaultCollectionName)
	if err != nil {
		ctx.Log.Error().Err(err).Msg("error getting embeddings queue pointer")
		return
	}

	if ctx.GetDefaultEmbeddingDims() == 0 {
		ctx.Log.Error().Msg("no default embedding dims found")
		return
	}

	ctx.Log.Info().Msgf("default embedding dims: %d", ctx.GetDefaultEmbeddingDims())

	if len(pointers) == 0 || pointers[0].QueuePointer == 0 {
		// there's no queue, it means we've never processed embeddings
		// so, let's create a new collection in our vector storage
		err = defaultVectorStorage.CreateCollection(defaultCollectionName, &vectors.CollectionParameters{
			Dimensions:      ctx.GetDefaultEmbeddingDims(),
			DistanceMeasure: vectors.DistanceCosine,
		})
		if err != nil {
			lg.Error().Err(err).
				Str("collection", defaultCollectionName).
				Msg("error creating embeddings collection")
			return
		} else {
			lg.Info().Str("collection", defaultCollectionName).Msg("created embeddings collection")
		}
	}

	// let's wait for at least one compute node to appear
	// with embedding capabilities
	// and pick embedding model name from it
	modelName, modelDims, err := ctx.ComputeRouter.WaitForNodeWithEmbeddings()
	if err != nil {
		lg.Fatal().Err(err).Msg("error waiting for compute node with embeddings")
		return
	}

	processEmbeddings(defaultVectorStorage, defaultCollectionName, &pointers, ctx, lg, name, modelName, modelDims)
}

func processEmbeddings(vectorDb vectors.VectorDB, collection string, pointers *[]EmbeddingsQueueRecord, ctx *server.Context, lg zerolog.Logger, process string, modelName string, modelDims int) {
	pointersMap := make(map[string]*EmbeddingsQueueRecord)
	for _, pointer := range *pointers {
		pointersMap[pointer.QueueName] = &pointer
	}

	if pointersMap[collection] == nil {
		res, err := ctx.Storage.Db.Exec("set-embeddings-queue-pointer", collection, 0)
		if err != nil {
			lg.Error().Err(err).
				Str("collection", collection).
				Msg("error setting embeddings queue pointer")
			return
		}
		id, err := res.LastInsertId()
		if err != nil {
			lg.Fatal().Err(err).
				Str("collection", collection).
				Msg("error getting last insert id")
			return
		}
		pointersMap[collection] = &EmbeddingsQueueRecord{
			Id:           id,
			QueueName:    collection,
			QueuePointer: 0,
		}
	}

	for {
		batchSize := 128
		llmCacheRecords := make([]cmds.CompletionCacheRecord, 0, batchSize)
		err := ctx.Storage.Db.GetStructsSlice("query-llm-cache-by-ids-multi",
			&llmCacheRecords,
			pointersMap[collection].QueuePointer,
			batchSize)

		if err != nil {
			lg.Error().Err(err).Msg("error getting llm cache records")
			time.Sleep(1 * time.Second)
			continue
		}

		if len(llmCacheRecords) == 0 {
			time.Sleep(1 * time.Second)
			continue
		}

		// lg.Info().Msgf("processing %d records", len(llmCacheRecords))
		// first we need to check if we have embeddings already for this exact texts
		// so hash_sums has to be calculated

		jobs := make([]cmds.GetEmbeddingsRequest, 0, len(llmCacheRecords))
		maxId := int64(0)
		for _, llmCacheRecord := range llmCacheRecords {
			jobs = append(jobs, cmds.GetEmbeddingsRequest{
				Model:           modelName,
				RawPrompt:       llmCacheRecord.Prompt,
				MetaNamespace:   "llm-cache-prompt",
				MetaNamespaceId: llmCacheRecord.Id,
			})
			jobs = append(jobs, cmds.GetEmbeddingsRequest{
				Model:           modelName,
				RawPrompt:       llmCacheRecord.GenerationResult,
				MetaNamespace:   "llm-cache-generation",
				MetaNamespaceId: llmCacheRecord.Id,
			})

			if llmCacheRecord.Id > maxId {
				maxId = llmCacheRecord.Id
			}
		}

		ts := time.Now()
		response, err := cmds.ProcessGetEmbeddings(jobs, ctx, process, borrow_engine.PRIO_Background)
		if err != nil {
			lg.Error().Err(err).
				Msgf("error getting embeddings in %v", time.Since(ts))
			time.Sleep(1 * time.Second)
			continue
		}

		// let's write embeddings into our vector storage
		vectorsSlice := make([]*vectors.Vector, 0, len(response.GetEmbeddingsResponse))
		for _, embedding := range response.GetEmbeddingsResponse {
			vectorsSlice = append(vectorsSlice, &vectors.Vector{
				Id:     uuid.NewHash(sha512.New(), uuid.Nil, []byte(embedding.TextHash), 5).String(),
				VecF64: embedding.Embeddings,
				Payload: map[string]interface{}{
					"model": embedding.Model,
					"text":  embedding.Text,
				},
			})
		}
		err = vectorDb.InsertVectors(collection, vectorsSlice)
		if err != nil {
			ctx.Log.Error().Err(err).
				Msgf("error inserting vectors into collection %s", collection)
		}

		/*lg.Info().
		Msgf("maxId = %d, got embeddings for %d records, in %v",
			maxId,
			len(response.GetEmbeddingsResponse),
			time.Since(ts))*/
		pointersMap[collection].QueuePointer = maxId
		// set the queue pointer
		_, err = ctx.Storage.Db.Exec("set-embeddings-queue-pointer", collection, maxId)
		if err != nil {
			lg.Error().Err(err).
				Str("collection", collection).
				Msg("error setting embeddings queue pointer")
		}
	}
}

func startQdrant(vectorDB *settings.VectorDBConfigurationSection, ctx *server.Context) error {
	vectorDb, err := vectors.NewQdrantClient(vectorDB)
	if err != nil {
		return err
	}

	ctx.VectorDBs = append(ctx.VectorDBs, vectorDb)
	return nil
}

func hashSum(s string) string {
	// let's use sha512 for now
	sha512engine := sha512.New()
	sha512engine.Write([]byte(s))
	return string(sha512engine.Sum(nil))
}
