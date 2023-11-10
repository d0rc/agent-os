package borrow_engine

import (
	"github.com/d0rc/agent-os/engines"
	"time"
)

type JobType int

const (
	JT_Completion JobType = iota
	JT_Embeddings
	JT_NotAJob
)

type RequestPriority int

const (
	PRIO_System RequestPriority = iota
	PRIO_Kernel
	PRIO_User
	PRIO_Background
)

type ComputeJob struct {
	JobId              string
	JobType            JobType
	Priority           RequestPriority
	Process            string
	receivedAt         time.Time
	GenerationSettings *engines.GenerationSettings
}

type ComputeFunction map[JobType]func(*InferenceNode, []*ComputeJob) []*ComputeJob
