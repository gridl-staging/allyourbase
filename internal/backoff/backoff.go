package backoff

import (
	"math/rand/v2"
	"time"
)

const (
	jobsBackoffBase      = 5 * time.Second
	jobsBackoffCap       = 5 * time.Minute
	jobsBackoffMaxJitter = 1 * time.Second
)

// Config controls exponential backoff calculation.
type Config struct {
	Base   time.Duration
	Cap    time.Duration
	Jitter func(delay time.Duration) time.Duration
}

// Exponential computes bounded exponential backoff with optional jitter.
func Exponential(attempt int, cfg Config) time.Duration {
	base := cfg.Base
	if base <= 0 {
		base = time.Second
	}

	capDelay := cfg.Cap
	if capDelay <= 0 {
		capDelay = base
	}

	if attempt < 1 {
		attempt = 1
	}

	delay := base
	if delay > capDelay {
		delay = capDelay
	}
	for i := 1; i < attempt && delay < capDelay; i++ {
		if delay > capDelay/2 {
			delay = capDelay
			break
		}
		delay *= 2
	}

	if cfg.Jitter != nil {
		delay += cfg.Jitter(delay)
	}

	if delay < 0 {
		return 0
	}
	return delay
}

// ComputeBackoff returns the jobs backoff profile using process randomness.
func ComputeBackoff(attempt int) time.Duration {
	return ComputeBackoffWithRand(attempt, rand.Int64N)
}

// ComputeBackoffWithRand returns the jobs backoff profile using caller-provided randomness.
func ComputeBackoffWithRand(attempt int, randInt63n func(int64) int64) time.Duration {
	delay := Exponential(attempt, Config{Base: jobsBackoffBase, Cap: jobsBackoffCap})
	if randInt63n == nil {
		return delay
	}
	return delay + time.Duration(randInt63n(int64(jobsBackoffMaxJitter)))
}
