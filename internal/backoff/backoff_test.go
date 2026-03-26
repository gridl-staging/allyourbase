package backoff

import (
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestExponentialWithConfig(t *testing.T) {
	t.Parallel()

	t.Run("attempt less than one clamps to one", func(t *testing.T) {
		t.Parallel()
		got := Exponential(0, Config{Base: 2 * time.Second, Cap: 30 * time.Second})
		testutil.Equal(t, 2*time.Second, got)
	})

	t.Run("caps at configured maximum", func(t *testing.T) {
		t.Parallel()
		got := Exponential(10, Config{Base: time.Second, Cap: 5 * time.Second})
		testutil.Equal(t, 5*time.Second, got)
	})

	t.Run("honors cap when lower than base", func(t *testing.T) {
		t.Parallel()
		got := Exponential(3, Config{Base: 5 * time.Second, Cap: 2 * time.Second})
		testutil.Equal(t, 2*time.Second, got)
	})

	t.Run("applies jitter with computed delay", func(t *testing.T) {
		t.Parallel()
		got := Exponential(4, Config{
			Base: time.Second,
			Cap:  30 * time.Second,
			Jitter: func(delay time.Duration) time.Duration {
				testutil.Equal(t, 8*time.Second, delay)
				return 250 * time.Millisecond
			},
		})
		testutil.Equal(t, 8250*time.Millisecond, got)
	})
}

func TestComputeBackoffWithRandDeterministic(t *testing.T) {
	t.Parallel()
	got := ComputeBackoffWithRand(3, func(n int64) int64 {
		return n - 1
	})
	want := 20*time.Second + (time.Second - time.Nanosecond)
	testutil.Equal(t, want, got)
}

func TestComputeBackoffWithRandClampsAttemptToOne(t *testing.T) {
	t.Parallel()
	got := ComputeBackoffWithRand(0, func(int64) int64 { return 0 })
	testutil.Equal(t, 5*time.Second, got)
}

func TestComputeBackoffWithRandCapsAtFiveMinutes(t *testing.T) {
	t.Parallel()
	got := ComputeBackoffWithRand(99, func(int64) int64 { return 0 })
	testutil.Equal(t, 5*time.Minute, got)
}
