//go:build integration

package jobs_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/testutil"
)

func setupService(t *testing.T, opts ...func(*jobs.ServiceConfig)) *jobs.Service {
	t.Helper()
	store := setupDB(t)

	cfg := jobs.DefaultServiceConfig()
	cfg.PollInterval = 100 * time.Millisecond
	cfg.LeaseDuration = 5 * time.Second
	cfg.WorkerConcurrency = 2
	cfg.SchedulerTick = 200 * time.Millisecond

	for _, o := range opts {
		o(&cfg)
	}

	svc := jobs.NewService(store, testutil.DiscardLogger(), cfg)
	return svc
}

// --- Backoff Tests ---

func TestBackoffExponentialWithJitter(t *testing.T) {
	// backoff = min(base * 2^(attempt-1), cap) + jitter
	// base=5s, cap=5min, jitter=0..1s
	for attempt := 1; attempt <= 5; attempt++ {
		d := jobs.ComputeBackoff(attempt)
		minExpected := 5 * time.Second * time.Duration(1<<(attempt-1))
		if minExpected > 5*time.Minute {
			minExpected = 5 * time.Minute
		}
		maxExpected := minExpected + 1*time.Second

		if d < minExpected || d > maxExpected {
			t.Errorf("attempt %d: backoff %v not in [%v, %v]", attempt, d, minExpected, maxExpected)
		}
	}

	// Verify cap: attempt=10 should be capped at 5min + jitter
	d := jobs.ComputeBackoff(10)
	if d < 5*time.Minute || d > 5*time.Minute+1*time.Second {
		t.Errorf("attempt 10: backoff %v should be capped at ~5min", d)
	}
}

// --- Worker Loop Tests ---

func TestWorkerProcessesJob(t *testing.T) {
	svc := setupService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var processed atomic.Int32
	svc.RegisterHandler("test_worker", func(ctx context.Context, payload json.RawMessage) error {
		processed.Add(1)
		return nil
	})

	// Enqueue a job before starting workers.
	_, err := svc.Enqueue(ctx, "test_worker", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	svc.Start(ctx)
	defer svc.Stop()

	// Wait for job to be processed.
	deadline := time.After(3 * time.Second)
	for processed.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for job to be processed")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	testutil.Equal(t, int32(1), processed.Load())

	// Verify job is completed in DB.
	time.Sleep(100 * time.Millisecond) // let Complete() finish
	all, err := svc.List(ctx, "completed", "", 10, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(all))
}

func TestWorkerRetriesFailedJob(t *testing.T) {
	svc := setupService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var attempts atomic.Int32
	svc.RegisterHandler("fail_then_pass", func(ctx context.Context, payload json.RawMessage) error {
		n := attempts.Add(1)
		if n < 2 {
			return fmt.Errorf("deliberate failure attempt %d", n)
		}
		return nil
	})

	_, err := svc.Enqueue(ctx, "fail_then_pass", nil, jobs.EnqueueOpts{MaxAttempts: 2})
	testutil.NoError(t, err)

	svc.Start(ctx)
	defer svc.Stop()

	// Wait for 2 attempts.
	deadline := time.After(8 * time.Second)
	for attempts.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("timed out: only %d attempts", attempts.Load())
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Let final processing complete.
	time.Sleep(200 * time.Millisecond)

	all, err := svc.List(ctx, "completed", "", 10, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(all))
}

func TestWorkerTerminalFailure(t *testing.T) {
	svc := setupService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	svc.RegisterHandler("always_fail", func(ctx context.Context, payload json.RawMessage) error {
		return fmt.Errorf("permanent failure")
	})

	_, err := svc.Enqueue(ctx, "always_fail", nil, jobs.EnqueueOpts{MaxAttempts: 1})
	testutil.NoError(t, err)

	svc.Start(ctx)
	defer svc.Stop()

	deadline := time.After(3 * time.Second)
	for {
		failed, err := svc.List(ctx, "failed", "", 10, 0)
		testutil.NoError(t, err)
		if len(failed) == 1 {
			testutil.Equal(t, "permanent failure", *failed[0].LastError)
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for terminal failure")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestWorkerUnknownJobTypeFails(t *testing.T) {
	svc := setupService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Enqueue a job type with no handler registered.
	_, err := svc.Enqueue(ctx, "nonexistent_type", nil, jobs.EnqueueOpts{MaxAttempts: 1})
	testutil.NoError(t, err)

	svc.Start(ctx)
	defer svc.Stop()

	deadline := time.After(3 * time.Second)
	for {
		failed, err := svc.List(ctx, "failed", "", 10, 0)
		testutil.NoError(t, err)
		if len(failed) == 1 {
			testutil.Contains(t, *failed[0].LastError, "no handler registered")
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for unknown-type failure")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestConcurrentWorkers(t *testing.T) {
	svc := setupService(t, func(cfg *jobs.ServiceConfig) {
		cfg.WorkerConcurrency = 4
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var maxConcurrent atomic.Int32
	var current atomic.Int32
	var total atomic.Int32

	svc.RegisterHandler("slow_job", func(ctx context.Context, payload json.RawMessage) error {
		c := current.Add(1)
		total.Add(1)
		// Track max concurrency.
		for {
			old := maxConcurrent.Load()
			if c <= old || maxConcurrent.CompareAndSwap(old, c) {
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
		current.Add(-1)
		return nil
	})

	// Enqueue 8 jobs.
	for i := 0; i < 8; i++ {
		_, err := svc.Enqueue(ctx, "slow_job", nil, jobs.EnqueueOpts{})
		testutil.NoError(t, err)
	}

	svc.Start(ctx)
	defer svc.Stop()

	deadline := time.After(8 * time.Second)
	for total.Load() < 8 {
		select {
		case <-deadline:
			t.Fatalf("timed out: only %d of 8 jobs processed", total.Load())
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	// With 4 workers processing 200ms jobs, we should see >1 concurrent.
	testutil.True(t, maxConcurrent.Load() > 1,
		"expected concurrent execution, got max=%d", maxConcurrent.Load())
}

// --- Lease Renewal Tests ---

func TestLeaseRenewalExtendsLease(t *testing.T) {
	// Use a very short lease so we can observe renewal within the test window.
	svc := setupService(t, func(cfg *jobs.ServiceConfig) {
		cfg.WorkerConcurrency = 1
		cfg.LeaseDuration = 2 * time.Second // short lease
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store := jobs.NewStore(sharedPG.Pool)

	var jobID string
	svc.RegisterHandler("long_running", func(ctx context.Context, payload json.RawMessage) error {
		// Simulate a handler that takes longer than half the lease duration.
		// Lease is 2s, renewal should fire at ~1s. We sleep for 3s to ensure
		// at least one renewal has happened.
		time.Sleep(3 * time.Second)
		return nil
	})

	job, err := svc.Enqueue(ctx, "long_running", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)
	jobID = job.ID

	svc.Start(ctx)
	defer svc.Stop()

	// Wait for job to be claimed.
	time.Sleep(500 * time.Millisecond)

	// Check the lease_until — it should have been extended beyond the original
	// claim time. The original lease was 2s from claim time; if renewal worked,
	// lease_until should be pushed further out.
	got, err := store.Get(ctx, jobID)
	testutil.NoError(t, err)
	testutil.Equal(t, jobs.StateRunning, got.State)
	testutil.NotNil(t, got.LeaseUntil)

	// Save the initial lease_until for comparison.
	firstLease := *got.LeaseUntil

	// Wait for renewal to fire (half of 2s = 1s, so after another second or so).
	time.Sleep(1500 * time.Millisecond)

	got2, err := store.Get(ctx, jobID)
	testutil.NoError(t, err)
	testutil.Equal(t, jobs.StateRunning, got2.State)
	testutil.NotNil(t, got2.LeaseUntil)
	testutil.True(t, got2.LeaseUntil.After(firstLease),
		"lease_until should have been extended: first=%v, current=%v",
		firstLease, *got2.LeaseUntil)

	// Wait for job to complete.
	deadline := time.After(5 * time.Second)
	for {
		completed, err := svc.List(ctx, "completed", "long_running", 10, 0)
		testutil.NoError(t, err)
		if len(completed) == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for long-running job to complete")
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func TestLeaseRenewalStopsOnCompletion(t *testing.T) {
	svc := setupService(t, func(cfg *jobs.ServiceConfig) {
		cfg.WorkerConcurrency = 1
		cfg.LeaseDuration = 2 * time.Second
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var handlerDone atomic.Int32
	svc.RegisterHandler("quick_job", func(ctx context.Context, payload json.RawMessage) error {
		handlerDone.Store(1)
		return nil
	})

	_, err := svc.Enqueue(ctx, "quick_job", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	svc.Start(ctx)
	defer svc.Stop()

	// Wait for completion.
	deadline := time.After(3 * time.Second)
	for handlerDone.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Job completed quickly — no crash, no goroutine leak. This test mainly
	// ensures the renewal goroutine doesn't panic or prevent completion.
	completed, err := svc.List(ctx, "completed", "quick_job", 10, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(completed))
}

// --- Scheduler Tests ---

func TestSchedulerEnqueuesJob(t *testing.T) {
	svc := setupService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var processed atomic.Int32
	svc.RegisterHandler("sched_job", func(ctx context.Context, payload json.RawMessage) error {
		processed.Add(1)
		return nil
	})

	// Create schedule with past next_run_at.
	past := time.Now().Add(-1 * time.Minute)
	_, err := svc.CreateSchedule(ctx, &jobs.Schedule{
		Name:        "test_sched",
		JobType:     "sched_job",
		CronExpr:    "* * * * *", // every minute
		Timezone:    "UTC",
		Enabled:     true,
		MaxAttempts: 3,
		NextRunAt:   &past,
	})
	testutil.NoError(t, err)

	svc.Start(ctx)
	defer svc.Stop()

	// Wait for scheduler to tick and worker to process.
	deadline := time.After(3 * time.Second)
	for processed.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for scheduled job")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Verify schedule's next_run_at was advanced.
	sched, err := svc.GetScheduleByName(ctx, "test_sched")
	testutil.NoError(t, err)
	testutil.NotNil(t, sched.NextRunAt)
	testutil.True(t, sched.NextRunAt.After(time.Now()),
		"next_run_at should be in the future after tick")
}

func TestSchedulerDuplicatePrevention(t *testing.T) {
	svc := setupService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var processed atomic.Int32
	svc.RegisterHandler("dedup_job", func(ctx context.Context, payload json.RawMessage) error {
		processed.Add(1)
		return nil
	})

	past := time.Now().Add(-1 * time.Minute)
	_, err := svc.CreateSchedule(ctx, &jobs.Schedule{
		Name:        "dedup_sched",
		JobType:     "dedup_job",
		CronExpr:    "0 0 1 1 *", // far future after first tick
		Timezone:    "UTC",
		Enabled:     true,
		MaxAttempts: 3,
		NextRunAt:   &past,
	})
	testutil.NoError(t, err)

	svc.Start(ctx)
	defer svc.Stop()

	// Wait enough for multiple scheduler ticks.
	time.Sleep(1 * time.Second)

	// Should have exactly 1 job enqueued (not duplicated).
	testutil.Equal(t, int32(1), processed.Load())
}

func TestSchedulerTimezone(t *testing.T) {
	svc := setupService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create schedule with timezone.
	past := time.Now().Add(-1 * time.Minute)
	_, err := svc.CreateSchedule(ctx, &jobs.Schedule{
		Name:        "tz_sched",
		JobType:     "tz_job",
		CronExpr:    "* * * * *",
		Timezone:    "America/New_York",
		Enabled:     true,
		MaxAttempts: 3,
		NextRunAt:   &past,
	})
	testutil.NoError(t, err)

	svc.RegisterHandler("tz_job", func(ctx context.Context, payload json.RawMessage) error {
		return nil
	})

	svc.Start(ctx)
	defer svc.Stop()

	time.Sleep(500 * time.Millisecond)

	// Verify the next_run_at was computed (schedule was advanced).
	sched, err := svc.GetScheduleByName(ctx, "tz_sched")
	testutil.NoError(t, err)
	testutil.NotNil(t, sched.NextRunAt)
	testutil.True(t, sched.NextRunAt.After(time.Now()),
		"next_run_at should be in the future")
}

func TestSchedulerDisabledDoesNotEnqueue(t *testing.T) {
	svc := setupService(t, func(cfg *jobs.ServiceConfig) {
		cfg.SchedulerEnabled = false
		cfg.SchedulerTick = 100 * time.Millisecond
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var processed atomic.Int32
	svc.RegisterHandler("disabled_sched_job", func(ctx context.Context, payload json.RawMessage) error {
		processed.Add(1)
		return nil
	})

	past := time.Now().Add(-1 * time.Minute)
	sched, err := svc.CreateSchedule(ctx, &jobs.Schedule{
		Name:        "disabled_sched",
		JobType:     "disabled_sched_job",
		CronExpr:    "0 0 1 1 *", // far future after one tick if scheduler runs
		Timezone:    "UTC",
		Enabled:     true,
		MaxAttempts: 3,
		NextRunAt:   &past,
	})
	testutil.NoError(t, err)

	svc.Start(ctx)
	defer svc.Stop()

	time.Sleep(500 * time.Millisecond)

	testutil.Equal(t, int32(0), processed.Load())

	// next_run_at should not be advanced when scheduler is disabled.
	after, err := svc.GetSchedule(ctx, sched.ID)
	testutil.NoError(t, err)
	testutil.NotNil(t, after.NextRunAt)
	testutil.Equal(t, sched.NextRunAt.UTC(), after.NextRunAt.UTC())
}

// --- Graceful Shutdown ---

func TestGracefulShutdown(t *testing.T) {
	svc := setupService(t, func(cfg *jobs.ServiceConfig) {
		cfg.WorkerConcurrency = 1
	})
	ctx := context.Background()

	var started atomic.Int32
	var finished atomic.Int32

	svc.RegisterHandler("long_job", func(ctx context.Context, payload json.RawMessage) error {
		started.Add(1)
		time.Sleep(500 * time.Millisecond)
		finished.Add(1)
		return nil
	})

	_, err := svc.Enqueue(ctx, "long_job", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	svc.Start(ctx)

	// Wait for job to start.
	deadline := time.After(2 * time.Second)
	for started.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for job to start")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Stop should wait for in-progress job.
	svc.Stop()
	testutil.Equal(t, int32(1), finished.Load())
}

// --- CronNextTime Tests ---

func TestCronNextTime(t *testing.T) {
	ref := time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC)

	// "0 * * * *" = every hour on the hour
	next, err := jobs.CronNextTime("0 * * * *", "UTC", ref)
	testutil.NoError(t, err)
	testutil.Equal(t, time.Date(2026, 2, 22, 11, 0, 0, 0, time.UTC), next)

	// "*/5 * * * *" = every 5 minutes, from 10:00 should be 10:05
	next, err = jobs.CronNextTime("*/5 * * * *", "UTC", ref)
	testutil.NoError(t, err)
	testutil.Equal(t, time.Date(2026, 2, 22, 10, 5, 0, 0, time.UTC), next)

	// With timezone
	next, err = jobs.CronNextTime("0 * * * *", "America/New_York", ref)
	testutil.NoError(t, err)
	testutil.True(t, next.After(ref), "next should be after ref")
}

func TestCronNextTimeInvalidExpr(t *testing.T) {
	ref := time.Now()
	_, err := jobs.CronNextTime("invalid cron", "UTC", ref)
	testutil.NotNil(t, err)
}

func TestCronNextTimeInvalidTimezone(t *testing.T) {
	ref := time.Now()
	_, err := jobs.CronNextTime("0 * * * *", "Invalid/Zone", ref)
	testutil.NotNil(t, err)
}

// --- Default Schedules ---

func TestRegisterDefaultSchedules(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	err := svc.RegisterDefaultSchedules(ctx)
	testutil.NoError(t, err)

	schedules, err := svc.ListSchedules(ctx)
	testutil.NoError(t, err)
	testutil.True(t, len(schedules) >= 4,
		"expected at least 4 default schedules, got %d", len(schedules))

	names := map[string]bool{}
	for _, s := range schedules {
		names[s.Name] = true
	}
	testutil.True(t, names["session_cleanup_hourly"], "missing session_cleanup_hourly")
	testutil.True(t, names["webhook_delivery_prune_daily"], "missing webhook_delivery_prune_daily")
	testutil.True(t, names["expired_oauth_cleanup_daily"], "missing expired_oauth_cleanup_daily")
	testutil.True(t, names["expired_auth_cleanup_daily"], "missing expired_auth_cleanup_daily")

	// Idempotent: running again should not error or create duplicates.
	err = svc.RegisterDefaultSchedules(ctx)
	testutil.NoError(t, err)

	schedules2, err := svc.ListSchedules(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, len(schedules), len(schedules2))
}
