package realtime

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSweepIdleKeepsConnectionIfActivityArrivesDuringSweep(t *testing.T) {
	t.Parallel()

	const clientID = "race-conn"
	idleTimeout := 50 * time.Millisecond

	cm := &ConnectionManager{
		idleTimeout: idleTimeout,
		conns:       make(map[string]*ConnectionMeta),
	}

	var closed atomic.Bool
	var touchStart sync.Once
	var touchWg sync.WaitGroup
	touchWg.Add(1)

	cm.conns[clientID] = &ConnectionMeta{
		ClientID:       clientID,
		UserID:         "user-1",
		LastActivityAt: time.Now().Add(-time.Minute), // definitely idle before sweep starts
		CloseFunc:      func() { closed.Store(true) },
		HasSubscriptions: func() bool {
			// Start a touch while sweepIdle still holds cm.mu. The touch blocks
			// until sweep releases the lock.
			touchStart.Do(func() {
				go func() {
					defer touchWg.Done()
					cm.TouchActivity(clientID)
				}()
			})
			return false
		},
	}

	cm.sweepIdle()
	touchWg.Wait()

	_, stillPresent := cm.conns[clientID]
	testutil.True(t, stillPresent, "connection touched during sweep should remain registered")
	testutil.True(t, !closed.Load(), "connection touched during sweep should not be closed")
}
