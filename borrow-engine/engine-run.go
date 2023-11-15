package borrow_engine

import (
	"sync"
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
		for {
			ie.PrintTop(jobsBuffer, &jobsBufferLock)
			time.Sleep(ie.settings.TopInterval)
		}
	}()

	for {
		attemptProcessing := false

		timer := time.NewTimer(100 * time.Millisecond)
		if countMapValueLens(jobsBuffer, &jobsBufferLock) > 1024 {
			select {
			case <-timer.C:
				attemptProcessing = true
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
			case <-timer.C:
				attemptProcessing = true
			case node := <-ie.AddNodeChan:
				node.LastIdleAt = time.Now()
				ie.Nodes = append(ie.Nodes, node)
				// since new node is available, let's trigger the processing
				attemptProcessing = true
			case _ = <-ie.InferenceDone:
				attemptProcessing = true
			case jobs := <-ie.IncomingJobs:
				// fmt.Printf("Recieved %d jobs\n", len(jobs))
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
			if time.Since(ie.Nodes[nodeIdx].LastFailure) < 5*time.Second {
				continue
			}
			// we have an available node...! let's try to
			// get node.MaxBatchSize jobs from the buffer
			// but we can only run jobs of the same type
			batch := map[JobType][]*ComputeJob{}

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

			// fmt.Printf("canSend: %v, canSendJobType: %v, haveAtLeastOneJobType: %v\n",
			//	canSend, canSendJobType, haveAtLeastOneJobType)

			if !canSend {
				continue
			}
			// check if we need to switch job types because other one has lower priorities
			perBatchMinPrio := make(map[JobType]JobPriority)
			minPrioJobType := JT_NotAJob
			for jobType, jobs := range batch {
				if len(jobs) == 0 {
					perBatchMinPrio[jobType] = PRIO_Background
					continue
				}
				minPriority := jobs[0].Priority
				minPrioJobType = jobs[0].JobType
				for _, job := range jobs {
					if job.Priority < minPriority {
						minPriority = job.Priority
						minPrioJobType = job.JobType
					}
				}
			}
			if canSendJobType != minPrioJobType && minPrioJobType != JT_NotAJob {
				canSendJobType = minPrioJobType
			}

			// let's check node can run this types of jobs
			jobTypesSwitchedAlready := false
		switchJobTypes:
			if len(batch[canSendJobType]) == 0 {
				continue
			}
			nodeIsCompatible := false
			for _, jt := range ie.Nodes[nodeIdx].JobTypes {
				if jt == canSendJobType {
					nodeIsCompatible = true
					break
				}
			}
			if !nodeIsCompatible {
				if jobTypesSwitchedAlready {
					continue
				}
				// either pick another job type, or just continue
				// let's see if we have another job type
				if canSendJobType == JT_Embeddings {
					canSendJobType = JT_Completion
					jobTypesSwitchedAlready = true
					goto switchJobTypes
				} else if canSendJobType == JT_Completion {
					canSendJobType = JT_Embeddings
					jobTypesSwitchedAlready = true
					goto switchJobTypes
				}

				continue
			}

			// we have a batch to send
			// let's send it
			// fmt.Printf("Sending batch of %d jobs to node %s\n", len(batch[canSendJobType]), ie.Nodes[nodeIdx].EndpointUrl)
			if ie.Nodes[nodeIdx].RequestsRunning == 0 {
				ie.Nodes[nodeIdx].TotalTimeIdle += time.Since(ie.Nodes[nodeIdx].LastIdleAt)
				ie.TotalTimeIdle += time.Since(ie.Nodes[nodeIdx].LastIdleAt)
			}
			ie.Nodes[nodeIdx].RequestsRunning++

			go ie.Nodes[nodeIdx].RunBatch(ie.ComputeFunction, batch[canSendJobType], nodeIdx, func(nodeIdx int, ts time.Time) {
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
			}, func(nodeIdx int, ts time.Time, err error) {
				// fmt.Printf("Batch of %d jobs on node %s failed\n", len(batch[canSendJobType]), ie.Nodes[nodeIdx].EndpointUrl)
				ie.Nodes[nodeIdx].TotalTimeWaisted += time.Since(ts)
				ie.TotalTimeWaisted += time.Since(ts)
				ie.TotalRequestsFailed++

				ie.Nodes[nodeIdx].TotalRequestsFailed++
				ie.Nodes[nodeIdx].TotalJobsFailed += uint64(len(batch[canSendJobType]))

				ie.Nodes[nodeIdx].LastFailure = time.Now()
				go func() {
					ie.IncomingJobs <- batch[canSendJobType]
				}()

				ie.Nodes[nodeIdx].RequestsRunning--
				if ie.Nodes[nodeIdx].RequestsRunning == 0 {
					ie.Nodes[nodeIdx].LastIdleAt = time.Now()
				}
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
