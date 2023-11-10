package borrow_engine

import (
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
	JobId      string
	JobType    JobType
	Priority   RequestPriority
	Process    string
	receivedAt time.Time
}

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
}
