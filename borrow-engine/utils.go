package borrow_engine

import "sync"

func countMapValueLens(buffer map[JobPriority][]*ComputeJob, lock *sync.RWMutex) int {
	lock.RLock()
	cnt := 0
	for _, jobs := range buffer {
		cnt += len(jobs)
	}
	lock.RUnlock()

	return cnt
}
