package borrow_engine

import (
	"github.com/d0rc/agent-os/engines"
	zlog "github.com/rs/zerolog/log"
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

	ComputeFunction     ComputeFunction
	TotalTimeWaisted    time.Duration
	TotalRequestsFailed uint64
	settings            *InferenceEngineSettings
}

type InferenceEngineSettings struct {
	TopInterval time.Duration
	TermUI      bool
	LogChan     chan []byte
}

func NewInferenceEngine(f ComputeFunction, settings *InferenceEngineSettings) *InferenceEngine {
	return &InferenceEngine{
		Nodes:                      []*InferenceNode{},
		AddNodeChan:                make(chan *InferenceNode, 16384),
		IncomingJobs:               make(chan []*ComputeJob, 16384),
		InferenceDone:              make(chan *InferenceNode, 16384),
		ProcessesTotalJobs:         make(map[string]uint64),
		ProcessesTotalTimeWaiting:  make(map[string]time.Duration),
		ProcessesTotalTimeConsumed: make(map[string]time.Duration),
		ComputeFunction:            f,
		settings:                   settings,
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

	go func(node *InferenceNode) {
		<-doneChannel
		if node.RemoteEngine.CompletionFailed &&
			node.RemoteEngine.EmbeddingsFailed {
			// engine failed to run completion and embeddings
			// we can't use it
			zlog.Info().Str("url", node.EndpointUrl).Msg("compute node failed to run completion and embeddings")
			autodetectFinished <- node
		} else {
			autodetectFinished <- node
			ie.AddNodeChan <- node
			ie.InferenceDone <- node
		}
	}(node)

	return autodetectFinished
}

func (ie *InferenceEngine) AddJob(job *ComputeJob) {
	job.receivedAt = time.Now()
	ie.IncomingJobs <- []*ComputeJob{job}
}

func (ie *InferenceEngine) WaitForNodeWithEmbeddings() (string, int, error) {
	for {
		for _, node := range ie.Nodes {
			if node.RemoteEngine.EmbeddingsDims != nil {
				return node.RemoteEngine.Models[0], int(*node.RemoteEngine.EmbeddingsDims), nil
			}
		}
		time.Sleep(1 * time.Second)
	}
}
