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

	RequestsRunning int
	LastIdleAt      time.Time
	RemoteEngine    *engines.RemoteInferenceEngine
}

func (n InferenceNode) RunBatch(cf ComputeFunction, jobs []*ComputeJob, nodeIdx int, f func(int, time.Time)) {
	// fmt.Printf("Running batch of %d jobs on node %s\n", len(jobs), n.EndpointUrl)
	// sleep for random time between 1 and 5 seconds
	ts := time.Now()

	cf[jobs[0].JobType](&n, jobs)

	//fmt.Printf("Batch of %d jobs on node %s finished\n", len(jobs), n.EndpointUrl)
	f(nodeIdx, ts)
}
