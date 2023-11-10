package borrow_engine

import "time"

func NewInferenceEngine() *InferenceEngine {
	return &InferenceEngine{
		Nodes:                      []*InferenceNode{},
		AddNodeChan:                make(chan *InferenceNode, 16384),
		IncomingJobs:               make(chan []*ComputeJob, 16384),
		InferenceDone:              make(chan *InferenceNode, 16384),
		ProcessesTotalJobs:         make(map[string]uint64),
		ProcessesTotalTimeWaiting:  make(map[string]time.Duration),
		ProcessesTotalTimeConsumed: make(map[string]time.Duration),
	}
}

func (ie *InferenceEngine) AddNode(node *InferenceNode) {
	ie.AddNodeChan <- node
}

func (ie *InferenceEngine) AddJob(job *ComputeJob) {
	job.receivedAt = time.Now()
	ie.IncomingJobs <- []*ComputeJob{job}
}
