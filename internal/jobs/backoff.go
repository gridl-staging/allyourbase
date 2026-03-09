package jobs

import (
	"math/rand"
	"time"
)

const (
	backoffBase      = 5 * time.Second
	backoffCap       = 5 * time.Minute
	backoffMaxJitter = 1 * time.Second
)

// ComputeBackoff returns a bounded exponential backoff with jitter.
// Formula: min(base * 2^(attempt-1), cap) + random(0..maxJitter).
func ComputeBackoff(attempt int) time.Duration {
	return ComputeBackoffWithRand(attempt, rand.Int63n)
}

// ComputeBackoffWithRand returns a bounded exponential backoff with jitter
// using the provided jitter source for deterministic tests.
func ComputeBackoffWithRand(attempt int, randInt63n func(int64) int64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	delay := backoffBase
	for i := 1; i < attempt && delay < backoffCap; i++ {
		delay *= 2
		if delay >= backoffCap {
			delay = backoffCap
			break
		}
	}

	if randInt63n == nil {
		return delay
	}
	jitter := time.Duration(randInt63n(int64(backoffMaxJitter)))
	return delay + jitter
}
