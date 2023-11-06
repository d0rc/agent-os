package cmds

import (
	"fmt"
	"github.com/logrusorgru/aurora"
	"math/rand"
	"sync"
	"time"
)

/*

Description is generated by bloop.ai from vllm source code:

JSON exmaple:

{
  "prompts": ["San Francisco is a", "New York is a"],
  "n": 1,
  "stream": false,
  "log_level": "info",
  "logprobs": 10,
  "echo": false,
  "max_tokens": 16,
  "temperature": 0.0,
  "top_p": 1.0,
  "presence_penalty": 0.0,
  "frequency_penalty": 0.0,
  "best_of": 1,
  "stop_sequences": ["\n"]
}

In this example:

"prompts" is a list of texts you want the model to continue.
"n" is the number of completions to generate for each prompt.
"stream" is a boolean indicating whether to stream the results or not.
"log_level" is the logging level.
"logprobs" is the number of most probable tokens to log for each token.
"echo" is a boolean indicating whether to include the prompt in the response.
"max_tokens" is the maximum number of tokens in the generated text.
"temperature" controls the randomness of the model's output. A higher value (closer to 1) makes the output more random, while a lower value (closer to 0) makes it more deterministic.
"top_p" is used for nucleus sampling and controls the cumulative probability cutoff.
"presence_penalty" and "frequency_penalty" are advanced parameters that control the model's output.
"best_of" is the number of times to run the model and keep the best result.
"stop_sequences" is a list of sequences where the API will stop generating further tokens.
Please note that the actual parameters may vary depending on the specific implementation of the vLLM engine.

*/

type EndpointProtocol int

const (
	EP_OpenAI = iota
	EP_VLLM
	EP_Custom
)

const A6000vLLMBatchSize = 128
const InferenceTimeout = 600 * time.Second

type InferenceEngine struct {
	EndpointUrl     string
	Protocol        EndpointProtocol
	MaxBatchSize    int
	Performance     float32 // tokens per second
	MaxRequests     int
	Models          []string // supported models
	RequestsServed  uint64
	TimeConsumed    time.Duration
	TokensProcessed uint64
	TokensGenerated uint64
	PromptTokens    uint64
	LeasedAt        time.Time
	Busy            bool
}

var inferenceEngines []*InferenceEngine

func init() {
	localLLM := &InferenceEngine{
		EndpointUrl:  "http://localhost:8000/v1/completions",
		Protocol:     EP_OpenAI,
		MaxBatchSize: 1,
		Performance:  0,
		MaxRequests:  1,
		Models:       []string{""},
	}

	remoteVLLM1 := &InferenceEngine{
		EndpointUrl:  "http://127.0.0.1:8001/v1/completions",
		Protocol:     EP_OpenAI,
		MaxBatchSize: A6000vLLMBatchSize,
		Performance:  0,
		MaxRequests:  1,
		Models:       []string{""},
	}

	remoteVLLM2 := &InferenceEngine{
		EndpointUrl:  "http://127.0.0.1:8002/v1/completions",
		Protocol:     EP_OpenAI,
		MaxBatchSize: A6000vLLMBatchSize,
		Performance:  0,
		MaxRequests:  1,
		Models:       []string{""},
	}
	remoteVLLM3 := &InferenceEngine{
		EndpointUrl:  "http://127.0.0.1:8003/v1/completions",
		Protocol:     EP_OpenAI,
		MaxBatchSize: A6000vLLMBatchSize,
		Performance:  0,
		MaxRequests:  1,
		Models:       []string{""},
	}
	remoteVLLM4 := &InferenceEngine{
		EndpointUrl:  "http://127.0.0.1:8004/v1/completions",
		Protocol:     EP_OpenAI,
		MaxBatchSize: A6000vLLMBatchSize,
		Performance:  0,
		MaxRequests:  1,
		Models:       []string{""},
	}

	fmt.Printf("localLLM: %v\n", localLLM)
	inferenceEngines = []*InferenceEngine{
		//localLLM,
		remoteVLLM1,
		remoteVLLM2,
		remoteVLLM3,
		remoteVLLM4,
	}

	// inferenceEngines = []*InferenceEngine{inferenceEngines[1]}
}

var inferenceEnginesMap = make(map[int]int)
var inferenceEnginesMapLock = sync.RWMutex{}

func pickInferenceEngine() *InferenceEngine {
pickDifferentEngine:
	engineId := rand.Intn(len(inferenceEngines))
	inferenceEnginesMapLock.Lock()

	if inferenceEnginesMap[engineId] >= inferenceEngines[engineId].MaxRequests {
		time.Sleep(100 * time.Millisecond)
		inferenceEnginesMapLock.Unlock()
		goto pickDifferentEngine
	}

	inferenceEnginesMap[engineId]++
	inferenceEngines[engineId].LeasedAt = time.Now()
	inferenceEnginesMapLock.Unlock()

	return inferenceEngines[engineId]
}

func releaseInferenceEngine(engine *InferenceEngine) {
	for idx, engineInfo := range inferenceEngines {
		if engineInfo.EndpointUrl == engine.EndpointUrl {
			// we've found it...
			inferenceEnginesMapLock.Lock()
			engine.TimeConsumed = engine.TimeConsumed + time.Since(engine.LeasedAt)
			inferenceEnginesMap[idx]--
			inferenceEnginesMapLock.Unlock()
			break
		}
	}
}

func GetInferenceEngines() []*InferenceEngine {
	return inferenceEngines
}

func ProcessGetCompletions(request []GetCompletionRequest, storage *Storage) (response *ServerResponse, err error) {
	// I've found no evidence that vllm supports batching for real
	// so we can just launch parallel processing now
	// later comment: and it's not the right place to make automatic batching...:)
	results := make([]chan *GetCompletionResponse, len(request))
	for idx, pr := range request {
		results[idx] = make(chan *GetCompletionResponse, 1)
		go func(cr GetCompletionRequest, ch chan *GetCompletionResponse) {
			completionResponse, err := processGetCompletion(cr, storage)
			if err != nil {
				storage.lg.Error().Err(err).
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

func processGetCompletion(cr GetCompletionRequest, storage *Storage) (*GetCompletionResponse, error) {
	cachedResponse := make([]CompletionCacheRecord, 0, 1)
	err := storage.db.GetStructsSlice("query-llm-cache", &cachedResponse,
		len(cr.RawPrompt), cr.RawPrompt)

	if err != nil {
		storage.lg.Error().Err(err).
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
			_, err := storage.db.Exec("make-llm-cache-hit", cacheRecord.Id)
			if err != nil {
				storage.lg.Error().Err(err).Msgf("error updating cache-hit counter: %v", err)
			}
		}

		if len(response.Choices) >= cr.MinResults {
			return response, nil
		}
	}

	message := <-SendCompletionRequest(&GenerationSettings{
		Messages:        nil,
		AfterJoinPrefix: "",
		RawPrompt:       cr.RawPrompt,
		NoCache:         false,
		Temperature:     cr.Temperature,
		StopTokens:      cr.StopTokens,
		BestOf:          cr.BestOf,
		StatisticsCallback: func(info *StatisticsInfo) {

		},
		MaxRetries: 1,
	})

	_, err = storage.db.Exec("insert-llm-cache-record",
		cr.Model,
		cr.RawPrompt,
		len(cr.RawPrompt),
		time.Now(),
		"",
		0,
		message.Content)
	if err != nil {
		storage.lg.Error().Err(err).
			Msgf("error creating new llm cache record: %v", err)
	}

	response.Choices = append(response.Choices, message.Content)

	return response, nil
}
