package retry

import (
	"math/rand"
	"time"
)

func Backoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 6 {
		attempt = 6
	}
	base := time.Second * time.Duration(1<<attempt)
	return base + time.Duration(rand.Int63n(int64(base/2+1)))
}
