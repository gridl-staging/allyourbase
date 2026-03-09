package jobs

import (
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestComputeBackoffWithRandDeterministic(t *testing.T) {
	// attempt=3 => base*2^(3-1)=20s, plus deterministic jitter.
	got := ComputeBackoffWithRand(3, func(n int64) int64 {
		// max jitter value for deterministic assertion.
		return n - 1
	})
	want := 20*time.Second + (time.Second - time.Nanosecond)
	testutil.Equal(t, want, got)
}

func TestComputeBackoffWithRandClampsAttemptToOne(t *testing.T) {
	got := ComputeBackoffWithRand(0, func(int64) int64 { return 0 })
	testutil.Equal(t, 5*time.Second, got)
}

func TestComputeBackoffWithRandCapsAtFiveMinutes(t *testing.T) {
	got := ComputeBackoffWithRand(99, func(int64) int64 { return 0 })
	testutil.Equal(t, 5*time.Minute, got)
}
