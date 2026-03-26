package server

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// TestRequestLoggerConcurrentWrites spawns 50 goroutines each logging 100 entries
// to a single RequestLogger and verifies:
//   - No deadlocks or panics.
//   - drop_count + flushed_count == total_sent.
//   - The test passes cleanly under -race.
func TestRequestLoggerConcurrentWrites(t *testing.T) {
	t.Parallel()

	const goroutines = 50
	const entriesPerGoroutine = 100
	const totalSent = goroutines * entriesPerGoroutine

	var flushedCount atomic.Int64
	flush := func(_ context.Context, entries []RequestLogEntry) error {
		flushedCount.Add(int64(len(entries)))
		return nil
	}

	cfg := RequestLogConfig{
		Enabled:   true,
		BatchSize: 50,
		QueueSize: 500, // smaller than total so some drops are expected under contention
	}
	rl := newRequestLoggerWithFlush(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), flush)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	rl.Start(ctx)

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range entriesPerGoroutine {
				rl.Log(RequestLogEntry{Method: "GET", Path: "/concurrent", StatusCode: 200})
			}
		}()
	}
	wg.Wait()

	testutil.NoError(t, rl.Shutdown(context.Background()))

	dropped := rl.DropCount()
	flushed := flushedCount.Load()

	// Every entry must be accounted for: either flushed or dropped.
	if flushed+dropped != int64(totalSent) {
		t.Errorf("flushed(%d) + dropped(%d) = %d, want %d",
			flushed, dropped, flushed+dropped, totalSent)
	}
}
