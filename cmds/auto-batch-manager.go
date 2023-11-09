package cmds

import (
	"fmt"
	"github.com/d0rc/agent-os/engines"
	"github.com/logrusorgru/aurora"
	"time"
)

var jobsQueueChannel = make(chan *engines.JobQueueTask, 1000)
var inferenceEnginesDoneChannel = make(chan int, 1000)

func SendCompletionRequest(req *engines.GenerationSettings) chan *engines.Message {
	outputChannel := make(chan *engines.Message, 1)
	jobsQueueChannel <- &engines.JobQueueTask{
		Req: req,
		Res: outputChannel,
	}

	return outputChannel
}

func ProcessJobsQueue() {
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
		var batch []*engines.JobQueueTask

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
		for idx, inferenceEngine := range engines.GetInferenceEngines() {
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

		engines.InferenceEngines[bestEngineIdx].Busy = true
		go func(batch []*engines.JobQueueTask, bestEngineIdx int) {
			defer func(bestEngineIdx int) {
				engines.InferenceEngines[bestEngineIdx].Busy = false
				inferenceEnginesDoneChannel <- bestEngineIdx
			}(bestEngineIdx)

			fmt.Printf("[%s] sending request, batch_size = %d\n",
				aurora.BrightMagenta("BATCH"),
				len(batch))
			_, err := engines.RunCompletionRequest(engines.InferenceEngines[bestEngineIdx], batch)
			if err != nil {
				// things got wrong....
				go func(batch []*engines.JobQueueTask) {
					for _, job := range batch {
						jobsQueueChannel <- job // re-queue failed jobs
					}
				}(batch)
				return
			}
		}(batch, bestEngineIdx)
	}
}
