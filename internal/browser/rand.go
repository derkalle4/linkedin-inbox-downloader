package browser

import (
	"math/rand"
	"sync"
	"time"
)

var (
	humanRand   = rand.New(rand.NewSource(time.Now().UnixNano()))
	humanRandMu sync.Mutex
)

func randInt63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	humanRandMu.Lock()
	defer humanRandMu.Unlock()
	return humanRand.Int63n(n)
}
