package borrow_engine

import "sync"

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
