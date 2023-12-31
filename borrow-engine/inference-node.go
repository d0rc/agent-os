package borrow_engine

import (
	"github.com/d0rc/agent-os/engines"
	"time"
)

type InferenceNode struct {
	EndpointUrl           string
	EmbeddingsEndpointUrl string
	MaxRequests           int
	MaxBatchSize          int
	JobTypes              []JobType

	TotalJobsProcessed     uint64
	TotalRequestsProcessed uint64
	TotalTimeConsumed      time.Duration
	TotalTimeIdle          time.Duration

	RequestsRunning     int32
	LastIdleAt          time.Time
	RemoteEngine        *engines.RemoteInferenceEngine
	TotalTimeWaisted    time.Duration
	TotalRequestsFailed uint64
	TotalJobsFailed     uint64
	LastFailure         time.Time
	Protocol            string
	Token               string
}

func (n InferenceNode) RunBatch(cf ComputeFunction, jobs []*ComputeJob, nodeIdx int,
	f func(int, time.Time),
	failFunc func(int, time.Time, error)) {
	// fmt.Printf("Running batch of %d jobs on node %s\n", len(jobs), n.EndpointUrl)
	// sleep for random time between 1 and 5 seconds
	ts := time.Now()

	_, err := cf[jobs[0].JobType](&n, jobs)
	if err != nil {
		// we need to retry the jobs, or send jobs back to the general queue
		// also it's a good idea to account engine failure at this point...
		failFunc(nodeIdx, ts, err)
		return
	}

	//fmt.Printf("Batch of %d jobs on node %s finished\n", len(jobs), n.EndpointUrl)
	f(nodeIdx, ts)
}
