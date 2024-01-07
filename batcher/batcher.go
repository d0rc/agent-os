package batcher

import (
	"github.com/rs/zerolog/log"
	"sync"
	"time"
)

type Batcher[T any] struct {
	op        func([]T) error
	tasksChan chan T
	stopChan  chan struct{}
	batchSize int
	latency   time.Duration
}

var batchers = make(map[string]interface{})
var batchersLock = sync.RWMutex{}

func NewBatcher[T any](id string, op func([]T) error, batchSize int, latency time.Duration) *Batcher[T] {
	batchersLock.Lock()
	defer batchersLock.Unlock()
	if b, exists := batchers[id]; exists {
		return b.(*Batcher[T])
	}

	b := &Batcher[T]{
		op:        op,
		tasksChan: make(chan T),
		stopChan:  make(chan struct{}, 1),
		batchSize: batchSize,
	}
	batchers[id] = b

	go func(b *Batcher[T]) {
		tasks := make([]T, 0)
		timer := time.NewTimer(b.latency)
		for {
			timerAlarm := false
			select {
			case <-b.stopChan:
				return
			case task := <-b.tasksChan:
				tasks = append(tasks, task)
				timer.Reset(b.latency)
			case <-timer.C:
				if len(tasks) > 0 {
					timerAlarm = true
				}
			}

			if len(tasks) > b.batchSize || timerAlarm {
				err := b.op(tasks)
				if err != nil {
					log.Error().Msgf("got error running batcher: %v", err)
				}
				tasks = make([]T, 0)
			}
		}
	}(b)

	return b
}

func (b *Batcher[T]) RunTask(task T) {
	b.tasksChan <- task
}

func (b *Batcher[T]) Stop() {
	b.stopChan <- struct{}{}
}
