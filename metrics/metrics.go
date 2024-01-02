package metrics

import (
	"sync"
	"time"
)

type Counter struct {
	name string

	value int64
	ts    time.Time

	value1s int64
	ts1s    time.Time
	rate1s  float64
}

var counters = make(map[string]*Counter)
var countersLock = sync.RWMutex{}

func Tick(name string, value int64) {
	countersLock.Lock()
	if _, exists := counters[name]; exists {
		counters[name].value += value
		counters[name].value1s += value
		if time.Since(counters[name].ts1s) >= 1*time.Second {
			counters[name].rate1s = float64(counters[name].value1s) / float64(time.Since(counters[name].ts1s))
			counters[name].ts1s = time.Now()
			counters[name].value1s = 0
		}
	} else {
		counters[name] = &Counter{
			name:    name,
			value:   value,
			ts:      time.Now(),
			value1s: value,
			ts1s:    time.Now(),
		}
	}
	countersLock.Unlock()
}

func Get(name string) int64 {
	countersLock.RLock()
	defer countersLock.RUnlock()

	counter, exists := counters[name]
	if !exists {
		return 0
	}
	return counter.value
}

func GetPerformance(name string) float64 {
	countersLock.RLock()
	defer countersLock.RUnlock()

	counter, exists := counters[name]
	if !exists {
		return 0
	}
	return float64(counter.value) / float64(time.Since(counter.ts))
}

func GetRate1s(name string) float64 {
	countersLock.RLock()
	defer countersLock.RUnlock()

	counter, exists := counters[name]
	if !exists {
		return 0
	}
	return counter.rate1s
}
