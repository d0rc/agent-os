package borrow_engine

import (
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func (ie *InferenceEngine) Run() {
	jobsBuffer := map[JobPriority][]*ComputeJob{
		PRIO_System:     []*ComputeJob{},
		PRIO_Kernel:     []*ComputeJob{},
		PRIO_User:       []*ComputeJob{},
		PRIO_Background: []*ComputeJob{},
	}
	jobsBufferLock := sync.RWMutex{}

	go func() {
		if ie.settings.TermUI {
			ie.ui(jobsBuffer, &jobsBufferLock)
			os.Exit(0)
		} else {
			for {
				ie.PrintTop(jobsBuffer, &jobsBufferLock)
				time.Sleep(ie.settings.TopInterval)
			}
		}
	}()

	// first let's start our primary cycle....!
	jobQueues := make([]chan *ComputeJob, PRIO_Background+1)
	for i := 0; i < int(PRIO_Background)+1; i++ {
		jobQueues[i] = make(chan *ComputeJob, 1024)
	}
	for {
		select {
		case jobs := <-ie.IncomingJobs:
			for _, job := range jobs {
				jobQueues[job.Priority] <- job
			}
		case node := <-ie.AddNodeChan:
			node.LastIdleAt = time.Now()
			ie.Nodes = append(ie.Nodes, node)
			nodeIdx := len(ie.Nodes) - 1
			// since we have added a new node, let's start the feeders for it
			for idx := 0; idx < node.MaxRequests; idx++ {
				go func() {
					batch := make([]*ComputeJob, 0, node.MaxBatchSize)
					firstElementTs := time.Now()
					batchIsReady := false
					for {
						for _, ch := range jobQueues {
							for {
								gotTheJob := false
								select {
								case job := <-ch:
									if job != nil {
										// let's check if this job is compatible with our engine at all..!
										// run the job...!
										gotTheJob = true
										batch = append(batch, job)
										if len(batch) == 1 {
											firstElementTs = time.Now()
										}
										if len(batch) == node.MaxBatchSize || (time.Since(firstElementTs) > 50*time.Millisecond && len(batch) > 0) {
											batchIsReady = true
											break
										}
									}
								default:
								}

								if !gotTheJob || batchIsReady {
									break
								}
							}

							if batchIsReady {
								break
							}
						}

						if len(batch) > 0 {
							// we have a batch of jobs to run...!
							atomic.AddInt32(&ie.Nodes[nodeIdx].RequestsRunning, 1)
							node.RunBatch(ie.ComputeFunction, batch, nodeIdx, func(nodeIdx int, ts time.Time) {
								ie.Nodes[nodeIdx].TotalTimeConsumed += time.Since(ts)
								ie.TotalRequestsProcessed++
								ie.TotalJobsProcessed += uint64(len(batch))
								ie.TotalTimeConsumed += time.Since(ts)
								jobsBufferLock.Lock()
								for _, job := range batch {
									ie.ProcessesTotalTimeConsumed[job.Process] += time.Since(ts)
								}
								jobsBufferLock.Unlock()
								//ie.Nodes[nodeIdx].RequestsRunning--
								ie.Nodes[nodeIdx].TotalRequestsProcessed++
								ie.Nodes[nodeIdx].TotalJobsProcessed += uint64(len(batch))

								if ie.Nodes[nodeIdx].RequestsRunning == 0 {
									ie.Nodes[nodeIdx].LastIdleAt = time.Now()
								}
							}, func(nodeIdx int, ts time.Time, err error) {
								// fmt.Printf("Batch of %d jobs on node %s failed\n", len(batch[canSendJobType]), ie.Nodes[nodeIdx].EndpointUrl)
								ie.Nodes[nodeIdx].TotalTimeWaisted += time.Since(ts)
								ie.TotalTimeWaisted += time.Since(ts)
								ie.TotalRequestsFailed++

								ie.Nodes[nodeIdx].TotalRequestsFailed++
								ie.Nodes[nodeIdx].TotalJobsFailed += uint64(len(batch))

								ie.Nodes[nodeIdx].LastFailure = time.Now()
								go func() {
									ie.IncomingJobs <- batch
								}()

								//ie.Nodes[nodeIdx].RequestsRunning--
								if ie.Nodes[nodeIdx].RequestsRunning == 0 {
									ie.Nodes[nodeIdx].LastIdleAt = time.Now()
								}
							})
							batch = []*ComputeJob{}
							batchIsReady = false

							atomic.AddInt32(&ie.Nodes[nodeIdx].RequestsRunning, -1)
						} else {
							time.Sleep(10 * time.Millisecond)
						}
					}
				}()
			}
		}
	}
}

func jobTypeName(jobType JobType) string {
	switch jobType {
	case JT_Embeddings:
		return "embeddings"
	case JT_Completion:
		return "completion"
	default:
		return "unknown"
	}
}
