package jobs

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestStartWithSchedulerDisabledDoesNotRunSchedulerLoop(t *testing.T) {
	cfg := DefaultServiceConfig()
	cfg.WorkerConcurrency = 0
	cfg.SchedulerEnabled = false
	cfg.SchedulerTick = 10 * time.Millisecond
	cfg.LeaseDuration = time.Hour // keep recovery loop asleep during the test

	svc := NewService(&Store{}, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	svc.Stop()
}
