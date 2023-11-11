package cmds

import (
	"encoding/json"
	borrowengine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/server"
	"github.com/d0rc/agent-os/storage"
)

func ProcessGetEmbeddings(request []GetEmbeddingsRequest, ctx *server.Context, process string, priority borrowengine.JobPriority) (response *ServerResponse, err error) {
	// I've found no evidence that vLLM supports batching for real
	// so, we can just launch parallel processing now
	// later comment: and it's not the right place to make automatic batching...:)
	results := make([]chan *GetEmbeddingsResponse, len(request))
	for idx, pr := range request {
		results[idx] = make(chan *GetEmbeddingsResponse, 1)
		go func(cr GetEmbeddingsRequest, ch chan *GetEmbeddingsResponse, idx int) {
			embeddingsResponse, err := processGetEmbeddings(cr, ctx, process, priority)
			if err != nil {
				ctx.Log.Error().Err(err).
					Msgf("Error processing embeddings request: ```%s```", cr.RawPrompt)
			}

			// ctx.Log.Info().Msgf("Got embeddings for prompt %d", idx)
			ch <- embeddingsResponse
		}(pr, results[idx], idx)
	}

	finalResults := make([]*GetEmbeddingsResponse, len(request))
	for idx, ch := range results {
		finalResults[idx] = <-ch
	}

	return &ServerResponse{
		GetEmbeddingsResponse: finalResults,
	}, nil
}

type EmbeddingsCacheRecord struct {
	Id          int64  `db:"id"`
	Model       string `db:"model"`
	Namespace   string `db:"namespace"`
	NamespaceId string `db:"namespace_id"`
	TextHash    string `db:"text_hash"`
	Embedding   []byte `db:"embedding"`
}

func processGetEmbeddings(cr GetEmbeddingsRequest, ctx *server.Context, process string, priority borrowengine.JobPriority) (*GetEmbeddingsResponse, error) {
	cachedResponse := make([]EmbeddingsCacheRecord, 0, 1)
	textHash := storage.GetHash(cr.RawPrompt)
	err := ctx.Storage.Db.GetStructsSlice("query-embeddings-cache", &cachedResponse,
		cr.Model, textHash)
	if err != nil {
		ctx.Log.Error().Err(err).
			Msgf("Failed to get cached response for prompt %s", cr.RawPrompt)
		// just continue...
	}

	response := &GetEmbeddingsResponse{}

	if len(cachedResponse) > 0 {
		decodedVector := make([]float64, 0)
		err := json.Unmarshal(cachedResponse[0].Embedding, &decodedVector)
		if err != nil {
			ctx.Log.Error().Err(err).
				Msgf("Failed to decode cached embeddings for prompt %s", cr.RawPrompt)
			// just continue...
		} else {
			response.Embeddings = decodedVector
			_, err := ctx.Storage.Db.Exec("make-embeddings-cache-hit", cachedResponse[0].Id)
			if err != nil {
				ctx.Log.Error().Err(err).
					Msgf("Failed to mark cache hit for prompt %s", cr.RawPrompt)
				// just continue...
			}

			return response, nil
		}
	}

	// once we're here, there were no embeddings in the cache
	// let's try to generate them
	computeResult := SendComputeRequest(ctx,
		process,
		borrowengine.JT_Embeddings,
		priority,
		&engines.GenerationSettings{
			RawPrompt: cr.RawPrompt,
		})
	embeddings := <-computeResult.EmbeddingChannel
	// ctx.Log.Info().Msgf("Got embeddings for prompt %d", len(cr.RawPrompt))

	// and now, need to save the result into the cache
	// but, first, need to serialize the embeddings
	serializedEmbeddings, err := json.Marshal(embeddings)
	if err != nil {
		return nil, err
	}

	_, err = ctx.Storage.Db.Exec("insert-embeddings-cache-record",
		*embeddings.Model,
		cr.MetaNamespace,
		cr.MetaNamespaceId,
		textHash,
		len(embeddings.VecF64),
		serializedEmbeddings)
	if err != nil {
		ctx.Log.Error().Err(err).
			Msgf("Failed to save embeddings to cache for prompt %s", cr.RawPrompt)
		// just continue...
	}

	response.Embeddings = embeddings.VecF64

	// ctx.Log.Info().Msgf("Got embeddings for prompt %d", len(cr.RawPrompt))
	return response, nil
}
