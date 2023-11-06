package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/logrusorgru/aurora"
	zlog "github.com/rs/zerolog/log"
	"io"
	"net/http"
	"agency/engine"
	"time"
)

type jobQueueTask struct {
	req *engine.GenerationSettings
	res chan *engine.Message
}

var jobsQueueChannel = make(chan *jobQueueTask, 1000)
var inferenceEnginesDoneChannel = make(chan int, 1000)

func SendCompletionRequest(req *engine.GenerationSettings) chan *engine.Message {
	outputChannel := make(chan *engine.Message, 1)
	jobsQueueChannel <- &jobQueueTask{
		req: req,
		res: outputChannel,
	}

	return outputChannel
}

func processJobsQueue() {
	for {
		// we should pick the most capable engine
		// find out what is its maximum batch size
		// try to collect the batch in next 50ms
		latencyTimeoutMs := 50 * time.Millisecond
		// send it and pick next inference engine

		// now we need to collect bestEngineBatchSize requests
		// but we can wait for them to arrive for not more then
		// latencyTimeoutMs
		// Create a slice to hold the batch of messages
		var batch []*jobQueueTask

		// Block until at least one message is received or inference engine freed
		select {
		case <-inferenceEnginesDoneChannel:
			// we've got inference server available...!
		case job := <-jobsQueueChannel:
			// we've got job
			batch = append(batch, job)
		}

		if len(batch) == 0 {
			continue
		}

	retryInferenceEngineSearch:
		bestEngineIdx := -1
		bestEngineBatchSize := 0
		for idx, inferenceEngine := range inferenceEngines {
			if !inferenceEngine.Busy && bestEngineBatchSize < inferenceEngine.MaxBatchSize {
				bestEngineIdx = idx
				bestEngineBatchSize = inferenceEngine.MaxBatchSize
			}
		}
		if bestEngineIdx == -1 {
			// No inference engine available, wait for one to become available
			// let's put it back to queue and try again
			<-inferenceEnginesDoneChannel
			goto retryInferenceEngineSearch
		}

		// Now start the timer
		timer := time.NewTimer(latencyTimeoutMs)

		// Collect additional messages in a loop
	loop:
		for len(batch) < bestEngineBatchSize { // Assume N is 10
			select {
			case msg := <-jobsQueueChannel:
				batch = append(batch, msg)
			case <-timer.C:
				// Timeout, break out of the loop
				break loop
			}
		}

		if len(batch) >= bestEngineBatchSize {
			// we should take only first bestEngineBatchSize elements
			// and send the rest back to the channel
			for i := bestEngineBatchSize - 1; i < len(batch); i++ {
				jobsQueueChannel <- batch[i]
			}
			batch = batch[bestEngineBatchSize:]
		}

		inferenceEngines[bestEngineIdx].Busy = true
		go func(batch []*jobQueueTask, bestEngineIdx int) {
			defer func(bestEngineIdx int) {
				inferenceEngines[bestEngineIdx].Busy = false
				inferenceEnginesDoneChannel <- bestEngineIdx
			}(bestEngineIdx)

			fmt.Printf("[%s] sending request, batch_size = %d\n",
				aurora.BrightMagenta("BATCH"),
				len(batch))
			err := runRequest(inferenceEngines[bestEngineIdx], batch)
			if err != nil {
				// things got wrong....
				go func(batch []*jobQueueTask) {
					for _, job := range batch {
						jobsQueueChannel <- job // re-queue failed jobs
					}
				}(batch)
				return
			}
		}(batch, bestEngineIdx)
	}
}

func runRequest(inferenceEngine *InferenceEngine, batch []*jobQueueTask) error {
	if len(batch) == 0 {
		return nil
	}
	client := http.Client{
		Timeout: InferenceTimeout,
	}

	type command struct {
		Prompts     []string `json:"prompt"`
		N           int      `json:"n"`
		Max         int      `json:"max_tokens"`
		Stop        []string `json:"stop"`
		Temperature float32  `json:"temperature"`
		Model       string   `json:"model"`
		BestOf      int      `json:"best_of"`
	}

	var stopTokens = []string{"###"}
	if batch[0].req.StopTokens != nil {
		stopTokens = append(stopTokens, batch[0].req.StopTokens...)
	}

	if batch[0].req.BestOf == 0 {
		batch[0].req.BestOf = 1
	}

	promptBodies := make([]string, len(batch))
	for i, b := range batch {
		promptBodies[i] = b.req.RawPrompt
	}

	cmd := &command{
		Prompts:     promptBodies,
		N:           1,
		Max:         4096,
		Stop:        stopTokens,
		Temperature: batch[0].req.Temperature,
		BestOf:      batch[0].req.BestOf,
	}

	commandBuffer, err := json.Marshal(cmd)
	if err != nil {
		zlog.Fatal().Err(err).Msg("error marshaling command")
	}

	// sending the request here...!
	resp, err := client.Post(inferenceEngine.EndpointUrl,
		"application/json",
		bytes.NewBuffer(commandBuffer))

	// whatever happened here, it's not of our business, we should just log it
	if err != nil {
		zlog.Error().Err(err).
			Interface("batch", batch).
			Msg("error sending request")
		return err
	}
	if resp.StatusCode != 200 {
		zlog.Error().Err(err).
			Interface("batch", batch).
			Msgf("error sending request http code is %d", resp.StatusCode)
		return err
	}
	// read resp.Body to result
	result, err := io.ReadAll(resp.Body)
	if err != nil {
		zlog.Error().Err(err).
			Interface("batch", batch).
			Msg("error reading response")
		return err
	}

	// now, let us parse all the response in choices
	type response struct {
		Choices []struct {
			Text string `json:"text"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	parsedResponse := &response{}
	err = json.Unmarshal(result, parsedResponse)
	if err != nil {
		zlog.Error().Err(err).
			Str("response", string(result)).
			Msg("error unmarshalling response")
	}

	// ok now each choice goes to its caller
	for idx, job := range batch {
		job.res <- &engine.Message{
			Role:    engine.ChatRoleAssistant,
			Content: parsedResponse.Choices[idx].Text,
		}
	}

	return nil
}

func init() {
	go processJobsQueue()
}
