package borrow_engine

import (
	"math/rand"
	"time"
)

type InferenceNode struct {
	EndpointUrl  string
	MaxRequests  int
	MaxBatchSize int
	JobTypes     []JobType

	TotalJobsProcessed     uint64
	TotalRequestsProcessed uint64
	TotalTimeConsumed      time.Duration
	TotalTimeIdle          time.Duration

	RequestsRunning int
	LastIdleAt      time.Time
}

func (n InferenceNode) RunBatch(jobs []*ComputeJob, nodeIdx int, f func(int, time.Time)) {
	// fmt.Printf("Running batch of %d jobs on node %s\n", len(jobs), n.EndpointUrl)
	// sleep for random time between 1 and 5 seconds
	ts := time.Now()
	time.Sleep(time.Duration(1+rand.Intn(1000)) * time.Millisecond)

	//fmt.Printf("Batch of %d jobs on node %s finished\n", len(jobs), n.EndpointUrl)
	f(nodeIdx, ts)
}
