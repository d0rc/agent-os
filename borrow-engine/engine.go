package borrow_engine

import (
	"github.com/d0rc/agent-os/engines"
	"time"
)

type InferenceEngine struct {
	Nodes []*InferenceNode

	// statistics
	TotalJobsProcessed         uint64
	TotalRequestsProcessed     uint64
	TotalTimeConsumed          time.Duration
	TotalTimeIdle              time.Duration
	ProcessesTotalJobs         map[string]uint64
	ProcessesTotalRequests     map[string]uint64
	ProcessesTotalTimeConsumed map[string]time.Duration
	ProcessesTotalTimeWaiting  map[string]time.Duration

	// control channels
	AddNodeChan         chan *InferenceNode
	IncomingJobs        chan []*ComputeJob
	InferenceDone       chan *InferenceNode
	TotalTimeScheduling time.Duration

	ComputeFunction ComputeFunction
}

func NewInferenceEngine(f ComputeFunction) *InferenceEngine {
	return &InferenceEngine{
		Nodes:                      []*InferenceNode{},
		AddNodeChan:                make(chan *InferenceNode, 16384),
		IncomingJobs:               make(chan []*ComputeJob, 16384),
		InferenceDone:              make(chan *InferenceNode, 16384),
		ProcessesTotalJobs:         make(map[string]uint64),
		ProcessesTotalTimeWaiting:  make(map[string]time.Duration),
		ProcessesTotalTimeConsumed: make(map[string]time.Duration),
		ComputeFunction:            f,
	}
}

func (ie *InferenceEngine) AddNode(node *InferenceNode) chan *InferenceNode {
	doneChannel := make(chan struct{}, 1)
	newRemoteEngine := &engines.RemoteInferenceEngine{
		EndpointUrl:           node.EndpointUrl,
		EmbeddingsEndpointUrl: node.EmbeddingsEndpointUrl,
		MaxBatchSize:          node.MaxBatchSize,
		Performance:           0,
		MaxRequests:           node.MaxRequests,
		Models:                nil,
		RequestsServed:        0,
		TimeConsumed:          0,
		TokensProcessed:       0,
		TokensGenerated:       0,
		PromptTokens:          0,
		LeasedAt:              time.Time{},
		Busy:                  false,
		EmbeddingsDims:        nil,
	}
	autodetectFinished := make(chan *InferenceNode, 1)
	go func(node *InferenceNode) {
		engines.StartInferenceEngine(newRemoteEngine, doneChannel)
		node.RemoteEngine = newRemoteEngine
	}(node)

	go func() {
		<-doneChannel
		autodetectFinished <- node
		ie.AddNodeChan <- node
	}()

	return autodetectFinished
}

func (ie *InferenceEngine) AddJob(job *ComputeJob) {
	job.receivedAt = time.Now()
	ie.IncomingJobs <- []*ComputeJob{job}
}
