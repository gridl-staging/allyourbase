package jobs

import (
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestComputeBackoffUsesConfiguredBounds(t *testing.T) {
	t.Parallel()
	got := ComputeBackoff(3)
	testutil.True(t, got >= 20*time.Second, "expected attempt 3 delay to be at least 20s")
	testutil.True(t, got <= 21*time.Second, "expected attempt 3 delay to include at most 1s jitter")
}
