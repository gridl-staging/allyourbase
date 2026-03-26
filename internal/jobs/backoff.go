package jobs

import (
	"time"

	sharedbackoff "github.com/allyourbase/ayb/internal/backoff"
)

// ComputeBackoff returns a bounded exponential backoff with jitter.
// Formula: min(base * 2^(attempt-1), cap) + random(0..maxJitter).
func ComputeBackoff(attempt int) time.Duration {
	return sharedbackoff.ComputeBackoff(attempt)
}
