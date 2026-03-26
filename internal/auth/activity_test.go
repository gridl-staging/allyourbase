package auth

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func waitForWrite(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case sid := <-ch:
		return sid
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("timed out waiting for async activity write")
		return ""
	}
}

func TestSessionActivityTrackerDebounceBehavior(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 3, 12, 0, 0, 0, time.UTC)
	writes := make(chan string, 4)
	var count int64

	tracker := NewSessionActivityTracker(nil, 5*time.Minute, testutil.DiscardLogger())
	tracker.nowFn = func() time.Time { return now }
	tracker.updateFn = func(_ context.Context, sessionID string) error {
		atomic.AddInt64(&count, 1)
		writes <- sessionID
		return nil
	}

	tracker.Touch(context.Background(), "session-1")
	testutil.Equal(t, "session-1", waitForWrite(t, writes))
	testutil.Equal(t, int64(1), atomic.LoadInt64(&count))

	now = now.Add(1 * time.Minute)
	tracker.Touch(context.Background(), "session-1")
	select {
	case sid := <-writes:
		t.Fatalf("unexpected debounced write for session %s", sid)
	case <-time.After(75 * time.Millisecond):
	}
	testutil.Equal(t, int64(1), atomic.LoadInt64(&count))

	now = now.Add(6 * time.Minute)
	tracker.Touch(context.Background(), "session-1")
	testutil.Equal(t, "session-1", waitForWrite(t, writes))
	testutil.Equal(t, int64(2), atomic.LoadInt64(&count))
}

func TestNewServiceActivityTrackerInitialization(t *testing.T) {
	t.Parallel()

	svcNoPool := NewService(nil, testSecret, time.Hour, 7*24*time.Hour, 8, testutil.DiscardLogger())
	testutil.Nil(t, svcNoPool.activityTracker)

	svcWithPool := NewService(&pgxpool.Pool{}, testSecret, time.Hour, 7*24*time.Hour, 8, testutil.DiscardLogger())
	testutil.NotNil(t, svcWithPool.activityTracker)
}
