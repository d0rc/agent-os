package cmds

import (
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/server"
	"github.com/logrusorgru/aurora"
	"time"
)

func ProcessGetCompletions(request []GetCompletionRequest, ctx *server.Context) (response *ServerResponse, err error) {
	// I've found no evidence that vllm supports batching for real
	// so we can just launch parallel processing now
	// later comment: and it's not the right place to make automatic batching...:)
	results := make([]chan *GetCompletionResponse, len(request))
	for idx, pr := range request {
		results[idx] = make(chan *GetCompletionResponse, 1)
		go func(cr GetCompletionRequest, ch chan *GetCompletionResponse) {
			completionResponse, err := processGetCompletion(cr, ctx)
			if err != nil {
				ctx.Log.Error().Err(err).
					Msgf("Error processing completion request: ```%s```", aurora.Cyan(cr.RawPrompt))
			}

			ch <- completionResponse
		}(pr, results[idx])
	}

	finalResults := make([]*GetCompletionResponse, len(request))
	for idx, ch := range results {
		finalResults[idx] = <-ch
	}

	return &ServerResponse{
		GetCompletionResponse: finalResults,
	}, nil
}

type CompletionCacheRecord struct {
	Id                           int64     `db:"id"`
	Model                        string    `db:"model"`
	Prompt                       string    `db:"prompt"`
	PromptLength                 int       `db:"prompt_length"`
	CreatedAt                    time.Time `db:"created_at"`
	SerializedGenerationSettings []byte    `db:"generation_settings"`
	CacheHits                    uint64    `db:"cache_hits"`
	GenerationResult             string    `db:"generation_result"`
}

func processGetCompletion(cr GetCompletionRequest, ctx *server.Context) (*GetCompletionResponse, error) {
	cachedResponse := make([]CompletionCacheRecord, 0, 1)
	err := ctx.Storage.Db.GetStructsSlice("query-llm-cache", &cachedResponse,
		len(cr.RawPrompt), cr.RawPrompt)

	if err != nil {
		ctx.Log.Error().Err(err).
			Msgf("Failed to get cached response for prompt %s", cr.RawPrompt)
		// just continue...
	}

	response := &GetCompletionResponse{
		Choices: make([]string, 0, len(cachedResponse)),
	}

	if len(cachedResponse) > 0 {
		// we have some cache hits, let's check if it's enough...!
		for _, cacheRecord := range cachedResponse {
			response.Choices = append(response.Choices, cacheRecord.GenerationResult)
			_, err := ctx.Storage.Db.Exec("make-llm-cache-hit", cacheRecord.Id)
			if err != nil {
				ctx.Log.Error().Err(err).Msgf("error updating cache-hit counter: %v", err)
			}
		}

		if len(response.Choices) >= cr.MinResults {
			return response, nil
		}
	}

	message := <-SendComputeRequest(ctx, &engines.GenerationSettings{
		Messages:        nil,
		AfterJoinPrefix: "",
		RawPrompt:       cr.RawPrompt,
		NoCache:         false,
		Temperature:     cr.Temperature,
		StopTokens:      cr.StopTokens,
		BestOf:          cr.BestOf,
		StatisticsCallback: func(info *engines.StatisticsInfo) {

		},
		MaxRetries: 1,
	})

	_, err = ctx.Storage.Db.Exec("insert-llm-cache-record",
		cr.Model,
		cr.RawPrompt,
		len(cr.RawPrompt),
		time.Now(),
		"",
		0,
		message.Content)
	if err != nil {
		ctx.Log.Error().Err(err).
			Msgf("error creating new llm cache record: %v", err)
	}

	response.Choices = append(response.Choices, message.Content)

	return response, nil
}
