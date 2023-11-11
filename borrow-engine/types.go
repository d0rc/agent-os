package borrow_engine

import (
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/vectors"
	"time"
)

type JobType int

const (
	JT_Completion JobType = iota
	JT_Embeddings
	JT_NotAJob
)

type JobPriority int

const (
	PRIO_System JobPriority = iota
	PRIO_Kernel
	PRIO_User
	PRIO_Background
)

type ComputeResult struct {
	CompletionChannel chan *engines.Message
	EmbeddingChannel  chan *vectors.Vector
}

type ComputeJob struct {
	JobId              string
	JobType            JobType
	Priority           JobPriority
	Process            string
	receivedAt         time.Time
	GenerationSettings *engines.GenerationSettings
	ComputeResult      *ComputeResult
}

type ComputeFunction map[JobType]func(*InferenceNode, []*ComputeJob) []*ComputeJob
