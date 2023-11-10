package borrow_engine

import (
	"fmt"
	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/olekukonko/tablewriter"
	"math/rand"
	"os"
	"sort"
	"sync"
	"testing"
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

func (ie *InferenceEngine) Run() {
	jobsBuffer := map[RequestPriority][]*ComputeJob{
		PRIO_System:     []*ComputeJob{},
		PRIO_Kernel:     []*ComputeJob{},
		PRIO_User:       []*ComputeJob{},
		PRIO_Background: []*ComputeJob{},
	}
	jobsBufferLock := sync.RWMutex{}

	go func() {
		for {
			ie.PrintTop(jobsBuffer, &jobsBufferLock)
			time.Sleep(1 * time.Second)
		}
	}()

	for {
		attemptProcessing := false

		if countMapValueLens(jobsBuffer, &jobsBufferLock) > 1024 {
			select {
			case node := <-ie.AddNodeChan:
				node.LastIdleAt = time.Now()
				ie.Nodes = append(ie.Nodes, node)
				// since new node is available, let's trigger the processing
				attemptProcessing = true
			case _ = <-ie.InferenceDone:
				attemptProcessing = true
			}
		} else {
			select {
			case node := <-ie.AddNodeChan:
				node.LastIdleAt = time.Now()
				ie.Nodes = append(ie.Nodes, node)
				// since new node is available, let's trigger the processing
				attemptProcessing = true
			case _ = <-ie.InferenceDone:
				attemptProcessing = true
			case jobs := <-ie.IncomingJobs:
				for _, job := range jobs {
					jobsBufferLock.Lock()
					ie.ProcessesTotalJobs[job.Process]++
					jobsBuffer[job.Priority] = append(jobsBuffer[job.Priority], job)
					jobsBufferLock.Unlock()
				}
				attemptProcessing = true
			}
		}

		if !attemptProcessing {
			// idle cycle ended
			continue
		}

		// attempt to process the jobs
		tsScheduling := time.Now()
		for nodeIdx, _ := range ie.Nodes {
			if ie.Nodes[nodeIdx].RequestsRunning >= ie.Nodes[nodeIdx].MaxRequests {
				continue
			}
			// we have an available node...! let's try to
			// get node.MaxBatchSize jobs from the buffer
			// but we can only run jobs of the same type
			batch := map[JobType][]*ComputeJob{}
			type BatchMetaType struct {
				jobType JobType
				jobIdx  int
			}

			canSend := false
			var haveAtLeastOneJobType JobType = JT_NotAJob
			var canSendJobType JobType = JT_NotAJob

			for priority := PRIO_System; priority <= PRIO_Background; priority++ {
				jobsBufferLock.RLock()
				jobsByType := getJobsByType(jobsBuffer[priority], ie.Nodes[nodeIdx].JobTypes)
				jobsBufferLock.RUnlock()
				for jobType, jobs := range jobsByType {
					if len(jobs) == 0 {
						continue
					}
					// we have some jobs to run
					// let's try to fill our batch
					for _, job := range jobs {
						batch[jobType] = append(batch[jobType], job)
						haveAtLeastOneJobType = jobType
						if len(batch[jobType]) == ie.Nodes[nodeIdx].MaxBatchSize {
							canSend = true
							canSendJobType = jobType
							break
						}
					}
				}
			}

			if haveAtLeastOneJobType != JT_NotAJob && time.Since(ie.Nodes[nodeIdx].LastIdleAt) > 50*time.Millisecond {
				canSend = true

				if canSendJobType == JT_NotAJob {
					canSendJobType = haveAtLeastOneJobType
				}
			}

			if !canSend {
				continue
			}

			// we have a batch to send
			// let's send it
			if ie.Nodes[nodeIdx].RequestsRunning == 0 {
				ie.Nodes[nodeIdx].TotalTimeIdle += time.Since(ie.Nodes[nodeIdx].LastIdleAt)
				ie.TotalTimeIdle += time.Since(ie.Nodes[nodeIdx].LastIdleAt)
			}
			ie.Nodes[nodeIdx].RequestsRunning++

			go ie.Nodes[nodeIdx].RunBatch(batch[canSendJobType], nodeIdx, func(nodeIdx int, ts time.Time) {
				ie.Nodes[nodeIdx].TotalTimeConsumed += time.Since(ts)
				ie.TotalRequestsProcessed++
				ie.TotalJobsProcessed += uint64(len(batch[canSendJobType]))
				ie.TotalTimeConsumed += time.Since(ts)
				jobsBufferLock.Lock()
				for _, job := range batch[canSendJobType] {
					ie.ProcessesTotalTimeConsumed[job.Process] += time.Since(ts)
				}
				jobsBufferLock.Unlock()
				ie.Nodes[nodeIdx].RequestsRunning--
				ie.Nodes[nodeIdx].TotalRequestsProcessed++
				ie.Nodes[nodeIdx].TotalJobsProcessed += uint64(len(batch[canSendJobType]))

				if ie.Nodes[nodeIdx].RequestsRunning == 0 {
					ie.Nodes[nodeIdx].LastIdleAt = time.Now()
				}
				ie.InferenceDone <- ie.Nodes[nodeIdx]
			})

			// drop jobs from the buffer
			for _, job := range batch[canSendJobType] {
				for priority := PRIO_System; priority <= PRIO_Background; priority++ {
					jobsBufferLock.Lock()
					for idx, jobInBuffer := range jobsBuffer[priority] {
						if jobInBuffer.JobId == job.JobId {
							jobsBuffer[priority] = append(jobsBuffer[priority][:idx], jobsBuffer[priority][idx+1:]...)
							ie.ProcessesTotalTimeWaiting[job.Process] += time.Since(job.receivedAt)
							break
						}
					}
					jobsBufferLock.Unlock()
				}
			}
		}

		ie.TotalTimeScheduling += time.Since(tsScheduling)
	}
}

func (ie *InferenceEngine) PrintTop(jobsBuffer map[RequestPriority][]*ComputeJob, lock *sync.RWMutex) {
	topLines := fmt.Sprintf("Total jobs: %s, Total requests: %d, Total time consumed: %s, Total time idle: %s\n",
		aurora.BrightCyan(humanize.SIWithDigits(float64(ie.TotalJobsProcessed), 2, "j")),
		ie.TotalRequestsProcessed,
		ie.TotalTimeConsumed,
		ie.TotalTimeIdle)
	topLines = topLines + fmt.Sprintf("Total jobs in buffer: %d(+%d), Total time in scheduler: %s\n",
		countMapValueLens(jobsBuffer, lock),
		len(ie.IncomingJobs),
		ie.TotalTimeScheduling)
	fmt.Printf(topLines)
	tw := tablewriter.NewWriter(os.Stdout)
	tw.SetHeader([]string{"Endpoint", "Requests", "MaxRequests", "MaxBatchSize", "TotalJobsProcessed", "TotalRequestsProcessed", "TotalTimeConsumed", "TotalTimeIdle"})
	for _, node := range ie.Nodes {
		tw.Append([]string{
			node.EndpointUrl,
			fmt.Sprintf("%d", node.RequestsRunning),
			fmt.Sprintf("%d", node.MaxRequests),
			fmt.Sprintf("%d", node.MaxBatchSize),
			fmt.Sprintf("%d", node.TotalJobsProcessed),
			fmt.Sprintf("%d", node.TotalRequestsProcessed),
			fmt.Sprintf("%s", node.TotalTimeConsumed),
			fmt.Sprintf("%s", node.TotalTimeIdle),
		})
	}
	tw.Render()

	tw = tablewriter.NewWriter(os.Stdout)
	tw.SetHeader([]string{"Process", "TotalJobsProcessed", "TotalTimeConsumed", "AvgWait"})
	lock.RLock()

	type ProcessInfo struct {
		TotalJobs uint64
		Name      string
	}
	processInfo := make([]ProcessInfo, 0, len(ie.ProcessesTotalJobs))
	for process, tj := range ie.ProcessesTotalJobs {
		processInfo = append(processInfo, ProcessInfo{
			TotalJobs: tj,
			Name:      process,
		})
	}
	// sort processInfo, make process with most jobs first
	// use library sort
	sort.Slice(processInfo, func(i, j int) bool {
		return processInfo[i].TotalJobs > processInfo[j].TotalJobs
	})

	for _, processData := range processInfo {
		tw.Append([]string{
			processData.Name,
			fmt.Sprintf("%d", ie.ProcessesTotalJobs[processData.Name]),
			fmt.Sprintf("%s", ie.ProcessesTotalTimeConsumed[processData.Name]),
			fmt.Sprintf("%s", fmt.Sprintf("%4.4f", float64(ie.ProcessesTotalTimeWaiting[processData.Name]/time.Millisecond)/float64(ie.ProcessesTotalJobs[processData.Name]))),
		})
	}
	lock.RUnlock()
	tw.Render()
}

func countMapValueLens(buffer map[RequestPriority][]*ComputeJob, lock *sync.RWMutex) int {
	lock.RLock()
	cnt := 0
	for _, jobs := range buffer {
		cnt += len(jobs)
	}
	lock.RUnlock()

	return cnt
}

func getJobsByType(buffer []*ComputeJob, types []JobType) map[JobType][]*ComputeJob {
	jobsByType := make(map[JobType][]*ComputeJob)
	for _, jobType := range types {
		jobsByType[jobType] = []*ComputeJob{}
	}

	for _, job := range buffer {
		jobsByType[job.JobType] = append(jobsByType[job.JobType], job)
	}

	return jobsByType
}

func (ie *InferenceEngine) AddJob(job *ComputeJob) {
	job.receivedAt = time.Now()
	ie.IncomingJobs <- []*ComputeJob{job}
}

func TestComputeRoutingWorksTest(t *testing.T) {
	// create engine
	engine := NewInferenceEngine()
	go engine.Run()

	// add ten random nodes with
	// max requests 1-2
	// max batch size 70-100
	// job types: JT_Completion, JT_Embeddings
	for i := 0; i < 5; i++ {
		node := &InferenceNode{
			EndpointUrl:  fmt.Sprintf("http://127.0.0.1:800%d/v1/completions", i),
			MaxRequests:  1 + rand.Intn(2),
			MaxBatchSize: 128,
			JobTypes:     []JobType{JT_Completion, JT_Embeddings},
		}
		engine.AddNode(node)
	}

	processes := []string{
		"agent-test",
		"background[embeddings]",
		"agent-user-chat",
		"agent-project-manager",
		"agent-c++-developer",
		"agent-python-developer",
		"agent-frontend-developer",
		"agent-html5-developer",
		"agent-general-research",
		"background[default-mode-network]",
	}

	for {
		// add 1000 random jobs with random priority
		// and random job type
		for i := 0; i < 100_000; i++ {
			job := &ComputeJob{
				JobId:    fmt.Sprintf("job-%d", i),
				JobType:  JobType(rand.Intn(2)),
				Priority: RequestPriority(rand.Intn(4)),
				Process:  processes[(rand.Int()%len(processes)+rand.Int()%len(processes)+rand.Int()%len(processes))/3],
			}
			engine.AddJob(job)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
