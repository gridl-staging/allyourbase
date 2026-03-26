package realtime_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/testutil"
)

// fastCM creates a ConnectionManager with short timeouts suitable for unit tests.
func fastCM(maxPerUser int, idleTimeout time.Duration) *realtime.ConnectionManager {
	return realtime.NewConnectionManager(realtime.ConnectionManagerOptions{
		MaxConnectionsPerUser: maxPerUser,
		IdleTimeout:           idleTimeout,
		SweepInterval:         idleTimeout / 4,
	})
}

// noopMeta returns a minimal ConnectionMeta with a no-op CloseFunc.
func noopMeta(clientID, userID, transport string) realtime.ConnectionMeta {
	return realtime.ConnectionMeta{
		ClientID:         clientID,
		UserID:           userID,
		Transport:        transport,
		CloseFunc:        func() {},
		HasSubscriptions: func() bool { return false },
	}
}

func TestRegisterAndDeregister(t *testing.T) {
	t.Parallel()
	cm := fastCM(10, time.Minute)
	defer cm.Stop()

	testutil.NoError(t, cm.Register(noopMeta("c1", "user-1", "ws")))
	snap := cm.Snapshot()
	testutil.Equal(t, 1, len(snap))
	testutil.Equal(t, "c1", snap[0].ClientID)
	testutil.Equal(t, "user-1", snap[0].UserID)
	testutil.Equal(t, "ws", snap[0].Transport)
	testutil.True(t, !snap[0].ConnectedAt.IsZero(), "ConnectedAt should be set")

	cm.Deregister("c1")
	testutil.Equal(t, 0, len(cm.Snapshot()))
}

func TestDeregisterMissingIDIsNoOp(t *testing.T) {
	t.Parallel()
	cm := fastCM(10, time.Minute)
	defer cm.Stop()

	// Should not panic or error.
	cm.Deregister("nonexistent")
}

func TestPerUserLimitEnforced(t *testing.T) {
	t.Parallel()
	cm := fastCM(2, time.Minute)
	defer cm.Stop()

	testutil.NoError(t, cm.Register(noopMeta("c1", "user-1", "ws")))
	testutil.NoError(t, cm.Register(noopMeta("c2", "user-1", "ws")))

	err := cm.Register(noopMeta("c3", "user-1", "ws"))
	testutil.True(t, errors.Is(err, realtime.ErrLimitExceeded), "expected ErrLimitExceeded, got: %v", err)
}

func TestPerUserLimitIsPerUser(t *testing.T) {
	t.Parallel()
	cm := fastCM(1, time.Minute)
	defer cm.Stop()

	testutil.NoError(t, cm.Register(noopMeta("c1", "user-1", "ws")))
	// Different user — should not be rejected.
	testutil.NoError(t, cm.Register(noopMeta("c2", "user-2", "ws")))
}

func TestPerUserLimitReleasedOnDeregister(t *testing.T) {
	t.Parallel()
	cm := fastCM(1, time.Minute)
	defer cm.Stop()

	testutil.NoError(t, cm.Register(noopMeta("c1", "user-1", "ws")))
	cm.Deregister("c1")
	// Slot freed — next registration should succeed.
	testutil.NoError(t, cm.Register(noopMeta("c2", "user-1", "ws")))
}

func TestCrossTransportLimit(t *testing.T) {
	t.Parallel()
	// Register two WS connections to fill user-1's limit, then attempt SSE — should fail.
	cm := fastCM(2, time.Minute)
	defer cm.Stop()

	testutil.NoError(t, cm.Register(noopMeta("ws-1", "user-1", "ws")))
	testutil.NoError(t, cm.Register(noopMeta("ws-2", "user-1", "ws")))

	sseMeta := realtime.ConnectionMeta{
		ClientID:         "sse-1",
		UserID:           "user-1",
		Transport:        "sse",
		CloseFunc:        func() {},
		HasSubscriptions: func() bool { return true },
	}
	err := cm.Register(sseMeta)
	testutil.True(t, errors.Is(err, realtime.ErrLimitExceeded), "expected ErrLimitExceeded for cross-transport, got: %v", err)
}

func TestAnonymousUserKeyPooled(t *testing.T) {
	t.Parallel()
	// All anonymous connections share a single pool via "__anonymous__".
	cm := fastCM(1, time.Minute)
	defer cm.Stop()

	testutil.NoError(t, cm.Register(noopMeta("c1", "__anonymous__", "ws")))
	err := cm.Register(noopMeta("c2", "__anonymous__", "ws"))
	testutil.True(t, errors.Is(err, realtime.ErrLimitExceeded), "anonymous connections should share limit pool")
}

func TestTouchActivity(t *testing.T) {
	t.Parallel()
	cm := fastCM(10, time.Minute)
	defer cm.Stop()

	testutil.NoError(t, cm.Register(noopMeta("c1", "user-1", "ws")))
	before := cm.Snapshot()[0].LastActivityAt

	time.Sleep(5 * time.Millisecond)
	cm.TouchActivity("c1")

	after := cm.Snapshot()[0].LastActivityAt
	testutil.True(t, after.After(before), "TouchActivity should advance LastActivityAt")
}

func TestTouchActivityMissingIDIsNoOp(t *testing.T) {
	t.Parallel()
	cm := fastCM(10, time.Minute)
	defer cm.Stop()

	// Should not panic.
	cm.TouchActivity("nonexistent")
}

func TestSnapshot(t *testing.T) {
	t.Parallel()
	cm := fastCM(10, time.Minute)
	defer cm.Stop()

	testutil.NoError(t, cm.Register(noopMeta("c1", "user-1", "ws")))
	testutil.NoError(t, cm.Register(noopMeta("c2", "user-2", "sse")))

	snaps := cm.Snapshot()
	testutil.Equal(t, 2, len(snaps))

	// Snapshot should not expose CloseFunc — verify it's just the value struct.
	ids := map[string]bool{}
	for _, s := range snaps {
		ids[s.ClientID] = true
	}
	testutil.True(t, ids["c1"] && ids["c2"], "both clients should appear in snapshot")
}

func TestIdleTimeoutClosesUnsubscribedConnection(t *testing.T) {
	t.Parallel()
	idleTimeout := 30 * time.Millisecond
	cm := fastCM(10, idleTimeout)
	defer cm.Stop()

	var closed atomic.Bool
	meta := realtime.ConnectionMeta{
		ClientID:         "ws-idle",
		UserID:           "user-1",
		Transport:        "ws",
		CloseFunc:        func() { closed.Store(true) },
		HasSubscriptions: func() bool { return false }, // no subscriptions
	}
	testutil.NoError(t, cm.Register(meta))

	deadline := time.Now().Add(idleTimeout * 10)
	for time.Now().Before(deadline) {
		if closed.Load() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	testutil.True(t, closed.Load(), "idle unsubscribed connection should be closed by sweep")
	testutil.Equal(t, 0, len(cm.Snapshot()))
}

func TestIdleTimeoutSpareSubscribedConnection(t *testing.T) {
	t.Parallel()
	idleTimeout := 20 * time.Millisecond
	cm := fastCM(10, idleTimeout)
	defer cm.Stop()

	var closed atomic.Bool
	meta := realtime.ConnectionMeta{
		ClientID:         "ws-sub",
		UserID:           "user-1",
		Transport:        "ws",
		CloseFunc:        func() { closed.Store(true) },
		HasSubscriptions: func() bool { return true }, // has subscriptions
	}
	testutil.NoError(t, cm.Register(meta))

	// Wait longer than the idle timeout — subscribed conn must NOT be closed.
	time.Sleep(idleTimeout * 4)
	testutil.True(t, !closed.Load(), "subscribed connection must not be idle-closed")
	testutil.Equal(t, 1, len(cm.Snapshot()))
}

func TestIdleTimeoutSpareConnectionWithRecentActivity(t *testing.T) {
	t.Parallel()
	idleTimeout := 30 * time.Millisecond
	cm := fastCM(10, idleTimeout)
	defer cm.Stop()

	var closed atomic.Bool
	meta := realtime.ConnectionMeta{
		ClientID:         "ws-active",
		UserID:           "user-1",
		Transport:        "ws",
		CloseFunc:        func() { closed.Store(true) },
		HasSubscriptions: func() bool { return false },
	}
	testutil.NoError(t, cm.Register(meta))

	// Keep touching activity to prevent idle expiry.
	stop := make(chan struct{})
	var touchWg sync.WaitGroup
	touchWg.Add(1)
	go func() {
		defer touchWg.Done()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				cm.TouchActivity("ws-active")
			}
		}
	}()

	time.Sleep(idleTimeout * 3)
	close(stop)
	touchWg.Wait()

	testutil.True(t, !closed.Load(), "actively-touched connection must not be idle-closed")
}

// --- Drain tests ---

func TestRegisterReturnsErrDrainingWhileDraining(t *testing.T) {
	t.Parallel()
	cm := fastCM(10, time.Minute)
	// Don't call Stop; Drain calls it.

	go cm.Drain(time.Second)
	time.Sleep(10 * time.Millisecond) // let drain set draining flag

	err := cm.Register(noopMeta("c1", "user-1", "ws"))
	testutil.True(t, errors.Is(err, realtime.ErrDraining), "expected ErrDraining, got: %v", err)
}

func TestDrainNaturalDeregisterNotForceClosed(t *testing.T) {
	t.Parallel()
	cm := fastCM(10, time.Minute)

	var forceClosed atomic.Bool
	meta := realtime.ConnectionMeta{
		ClientID:         "c1",
		UserID:           "user-1",
		Transport:        "ws",
		CloseFunc:        func() { forceClosed.Store(true) },
		HasSubscriptions: func() bool { return false },
	}
	testutil.NoError(t, cm.Register(meta))

	// Start drain with a generous timeout.
	done := make(chan struct{})
	go func() {
		defer close(done)
		cm.Drain(2 * time.Second)
	}()

	// Deregister naturally before timeout.
	time.Sleep(20 * time.Millisecond)
	cm.Deregister("c1")

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Drain did not return after all connections deregistered")
	}
	testutil.True(t, !forceClosed.Load(), "naturally-deregistered connection should not have CloseFunc called")
}

func TestDrainForceClosesAtTimeout(t *testing.T) {
	t.Parallel()
	cm := fastCM(10, time.Minute)

	var closed atomic.Bool
	meta := realtime.ConnectionMeta{
		ClientID:         "stubborn",
		UserID:           "user-1",
		Transport:        "ws",
		CloseFunc:        func() { closed.Store(true) },
		HasSubscriptions: func() bool { return false },
	}
	testutil.NoError(t, cm.Register(meta))

	// Short drain timeout — connection never self-deregisters.
	cm.Drain(50 * time.Millisecond)

	testutil.True(t, closed.Load(), "stubborn connection should be force-closed after drain timeout")
	testutil.Equal(t, 0, len(cm.Snapshot()))
}

func TestForceDisconnect(t *testing.T) {
	t.Parallel()
	cm := fastCM(10, time.Minute)
	defer cm.Stop()

	var closed atomic.Bool
	meta := realtime.ConnectionMeta{
		ClientID:         "c1",
		UserID:           "user-1",
		Transport:        "ws",
		CloseFunc:        func() { closed.Store(true) },
		HasSubscriptions: func() bool { return false },
	}
	testutil.NoError(t, cm.Register(meta))

	found := cm.ForceDisconnect("c1")
	testutil.True(t, found, "ForceDisconnect should return true for existing connection")
	testutil.True(t, closed.Load(), "CloseFunc should be called")
	testutil.Equal(t, 0, len(cm.Snapshot()))
}

func TestForceDisconnectNotFound(t *testing.T) {
	t.Parallel()
	cm := fastCM(10, time.Minute)
	defer cm.Stop()

	found := cm.ForceDisconnect("nonexistent")
	testutil.True(t, !found, "ForceDisconnect should return false for unknown ID")
}
