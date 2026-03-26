package auth

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestTokenDenyListAddIsDenied(t *testing.T) {
	t.Parallel()

	list := NewTokenDenyList()
	list.Add("session-1", time.Hour)

	testutil.True(t, list.IsDenied("session-1"), "session should be denied after add")
	testutil.Equal(t, 1, list.Len())
}

func TestTokenDenyListZeroAndNegativeTTLNotDenied(t *testing.T) {
	t.Parallel()

	list := NewTokenDenyList()
	list.Add("session-zero", 0)
	list.Add("session-negative", -time.Second)

	testutil.False(t, list.IsDenied("session-zero"), "zero ttl should not deny")
	testutil.False(t, list.IsDenied("session-negative"), "negative ttl should not deny")
	testutil.Equal(t, 0, list.Len())
}

func TestTokenDenyListTracksEntriesIndependently(t *testing.T) {
	t.Parallel()

	list := NewTokenDenyList()
	list.Add("session-a", time.Hour)
	list.Add("session-b", time.Hour)

	testutil.True(t, list.IsDenied("session-a"), "session-a should be denied")
	testutil.True(t, list.IsDenied("session-b"), "session-b should be denied")
	testutil.False(t, list.IsDenied("session-c"), "unknown session should not be denied")
	testutil.Equal(t, 2, list.Len())
}

func TestTokenDenyListLazyEvictsExpiredEntryOnRead(t *testing.T) {
	t.Parallel()

	list := NewTokenDenyList()
	list.Add("session-expired", 5*time.Millisecond)

	time.Sleep(25 * time.Millisecond)

	testutil.False(t, list.IsDenied("session-expired"), "expired session should not be denied")
	testutil.Equal(t, 0, list.Len())
}

func TestTokenDenyListConcurrentAccess(t *testing.T) {
	t.Parallel()

	list := NewTokenDenyList()
	var wg sync.WaitGroup

	for i := 0; i < 32; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("session-%d", i)
			for j := 0; j < 100; j++ {
				list.Add(id, time.Second)
				_ = list.IsDenied(id)
			}
		}()
	}

	wg.Wait()
	testutil.True(t, list.Len() > 0, "deny list should contain entries")
}
