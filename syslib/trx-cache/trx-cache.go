package trx_cache

import (
	"sync"
	"time"
)

type ttlPair struct {
	trx      string
	deadLine time.Time
}

type TrxCache struct {
	pendingResults  map[string][]chan []byte
	finalResults    map[string][]byte
	lock            sync.RWMutex
	expirationQueue []ttlPair
}

const expirationTimeout = 5 * time.Minute

func NewTrxCache() *TrxCache {
	return &TrxCache{
		pendingResults:  make(map[string][]chan []byte),
		finalResults:    make(map[string][]byte),
		expirationQueue: make([]ttlPair, 0),
	}
}

func (t *TrxCache) GetValue(trx string, f func() []byte) []byte {
	t.lock.Lock()

	if _, ok := t.finalResults[trx]; ok {
		t.lock.Unlock()
		return t.finalResults[trx]
	}

	if _, ok := t.pendingResults[trx]; ok {
		// something is cooking already...!
		responseChannel := make(chan []byte)
		t.pendingResults[trx] = append(t.pendingResults[trx], responseChannel)
		t.lock.Unlock()

		return <-responseChannel
	}

	t.pendingResults[trx] = make([]chan []byte, 0)
	t.lock.Unlock()

	result := f()

	go func() {
		t.lock.Lock()
		t.finalResults[trx] = result
		channels := t.pendingResults[trx]
		delete(t.pendingResults, trx)

		t.expirationQueue = append(t.expirationQueue, ttlPair{
			trx:      trx,
			deadLine: time.Now().Add(expirationTimeout),
		})

		for len(t.expirationQueue) > 1 && time.Since(t.expirationQueue[0].deadLine) > 0 {
			// we need to pop ttlPairs from the top of t.expirationQueue and delete their data
			// remove them from the t.expirationQueue when done
			t.expirationQueue = t.expirationQueue[1:]
			delete(t.finalResults, trx)
		}

		t.lock.Unlock()

		for _, channel := range channels {
			channel <- result
		}
	}()

	return result
}
