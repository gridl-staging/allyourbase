//go:build integration

package jobs_test

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func setupDB(t *testing.T) *jobs.Store {
	t.Helper()
	ctx := context.Background()

	// Reset schema and run migrations.
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err = runner.Bootstrap(ctx)
	testutil.NoError(t, err)
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)

	return jobs.NewStore(sharedPG.Pool)
}

// --- State Machine Tests ---

func TestEnqueueClaimComplete(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	// Enqueue
	job, err := store.Enqueue(ctx, "test_job", json.RawMessage(`{"key":"value"}`), jobs.EnqueueOpts{})
	testutil.NoError(t, err)
	testutil.Equal(t, jobs.StateQueued, job.State)
	testutil.Equal(t, "test_job", job.Type)
	testutil.Equal(t, 0, job.Attempts)
	testutil.Equal(t, 3, job.MaxAttempts)

	// Claim
	claimed, err := store.Claim(ctx, "worker-1", 5*time.Minute)
	testutil.NoError(t, err)
	testutil.NotNil(t, claimed)
	testutil.Equal(t, job.ID, claimed.ID)
	testutil.Equal(t, jobs.StateRunning, claimed.State)
	testutil.Equal(t, 1, claimed.Attempts)
	testutil.NotNil(t, claimed.LeaseUntil)
	testutil.NotNil(t, claimed.WorkerID)
	testutil.Equal(t, "worker-1", *claimed.WorkerID)

	// Complete
	completed, err := store.Complete(ctx, claimed.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, jobs.StateCompleted, completed.State)
	testutil.NotNil(t, completed.CompletedAt)
	testutil.True(t, completed.LeaseUntil == nil, "lease_until should be cleared")
	testutil.True(t, completed.WorkerID == nil, "worker_id should be cleared")
}

func TestEnqueueClaimFailRetry(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	job, err := store.Enqueue(ctx, "fail_job", nil, jobs.EnqueueOpts{MaxAttempts: 3})
	testutil.NoError(t, err)

	// First attempt: claim + fail (should re-queue).
	claimed, err := store.Claim(ctx, "w1", 5*time.Minute)
	testutil.NoError(t, err)
	testutil.Equal(t, job.ID, claimed.ID)
	testutil.Equal(t, 1, claimed.Attempts)

	failed, err := store.Fail(ctx, claimed.ID, "attempt 1 error", 1*time.Second)
	testutil.NoError(t, err)
	testutil.Equal(t, jobs.StateQueued, failed.State) // re-queued for retry
	testutil.NotNil(t, failed.LastError)
	testutil.Equal(t, "attempt 1 error", *failed.LastError)

	// Wait for run_at to pass.
	time.Sleep(1100 * time.Millisecond)

	// Second attempt: claim + fail (should re-queue again).
	claimed2, err := store.Claim(ctx, "w1", 5*time.Minute)
	testutil.NoError(t, err)
	testutil.NotNil(t, claimed2)
	testutil.Equal(t, 2, claimed2.Attempts)

	failed2, err := store.Fail(ctx, claimed2.ID, "attempt 2 error", 1*time.Second)
	testutil.NoError(t, err)
	testutil.Equal(t, jobs.StateQueued, failed2.State)

	// Wait for run_at to pass.
	time.Sleep(1100 * time.Millisecond)

	// Third attempt: claim + fail (should be terminal).
	claimed3, err := store.Claim(ctx, "w1", 5*time.Minute)
	testutil.NoError(t, err)
	testutil.NotNil(t, claimed3)
	testutil.Equal(t, 3, claimed3.Attempts)

	failed3, err := store.Fail(ctx, claimed3.ID, "attempt 3 terminal", 1*time.Second)
	testutil.NoError(t, err)
	testutil.Equal(t, jobs.StateFailed, failed3.State) // terminal
	testutil.Equal(t, "attempt 3 terminal", *failed3.LastError)
}

func TestEnqueueCancel(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	job, err := store.Enqueue(ctx, "cancel_job", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	canceled, err := store.Cancel(ctx, job.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, jobs.StateCanceled, canceled.State)
	testutil.NotNil(t, canceled.CanceledAt)
}

func TestCancelRunningJobFails(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	job, err := store.Enqueue(ctx, "running_job", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	_, err = store.Claim(ctx, "w1", 5*time.Minute)
	testutil.NoError(t, err)

	_, err = store.Cancel(ctx, job.ID)
	testutil.NotNil(t, err) // Can't cancel a running job.
}

func TestRetryNow(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	// Enqueue with max_attempts=1 so it fails terminally.
	job, err := store.Enqueue(ctx, "retry_test", nil, jobs.EnqueueOpts{MaxAttempts: 1})
	testutil.NoError(t, err)

	claimed, err := store.Claim(ctx, "w1", 5*time.Minute)
	testutil.NoError(t, err)

	_, err = store.Fail(ctx, claimed.ID, "failed", 0)
	testutil.NoError(t, err)

	// Verify it's failed.
	got, err := store.Get(ctx, job.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, jobs.StateFailed, got.State)

	// Admin retry.
	retried, err := store.RetryNow(ctx, job.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, jobs.StateQueued, retried.State)
	testutil.Equal(t, 0, retried.Attempts)
}

// --- Concurrency Tests ---

func TestClaimSkipLocked(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	// Enqueue 1 job.
	_, err := store.Enqueue(ctx, "concurrent_job", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	// Two concurrent claims: exactly 1 should succeed.
	var wg sync.WaitGroup
	results := make(chan *jobs.Job, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(workerID string) {
			defer wg.Done()
			j, err := store.Claim(ctx, workerID, 5*time.Minute)
			if err != nil {
				return
			}
			results <- j
		}("worker-" + string(rune('A'+i)))
	}

	wg.Wait()
	close(results)

	claimed := 0
	for j := range results {
		if j != nil {
			claimed++
		}
	}
	testutil.Equal(t, 1, claimed)
}

// --- Crash Recovery Tests ---

func TestRecoverStalledJobs(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	// Enqueue and claim a job.
	job, err := store.Enqueue(ctx, "stalled_job", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	claimed, err := store.Claim(ctx, "w1", 1*time.Second)
	testutil.NoError(t, err)
	testutil.Equal(t, job.ID, claimed.ID)

	// Wait for lease to expire.
	time.Sleep(1100 * time.Millisecond)

	// Recover stalled jobs.
	recovered, err := store.RecoverStalledJobs(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(1), recovered)

	// Verify the job is back in queued state.
	got, err := store.Get(ctx, job.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, jobs.StateQueued, got.State)
	testutil.True(t, got.LeaseUntil == nil, "lease_until should be cleared")

	// Should be claimable again.
	reClaimed, err := store.Claim(ctx, "w2", 5*time.Minute)
	testutil.NoError(t, err)
	testutil.NotNil(t, reClaimed)
	testutil.Equal(t, job.ID, reClaimed.ID)
}

// --- Idempotency Tests ---

func TestIdempotencyKey(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	_, err := store.Enqueue(ctx, "idem_job", nil, jobs.EnqueueOpts{IdempotencyKey: "unique-key-1"})
	testutil.NoError(t, err)

	// Second enqueue with same key should fail.
	_, err = store.Enqueue(ctx, "idem_job", nil, jobs.EnqueueOpts{IdempotencyKey: "unique-key-1"})
	testutil.NotNil(t, err)
}

// --- Delayed Job Test ---

func TestDelayedJob(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	future := time.Now().Add(10 * time.Second)
	job, err := store.Enqueue(ctx, "delayed_job", nil, jobs.EnqueueOpts{RunAt: &future})
	testutil.NoError(t, err)
	testutil.Equal(t, jobs.StateQueued, job.State)

	// Claim should return nil (not yet eligible).
	claimed, err := store.Claim(ctx, "w1", 5*time.Minute)
	testutil.NoError(t, err)
	testutil.True(t, claimed == nil, "delayed job should not be claimable yet")
}

// --- List and Stats Tests ---

func TestListAndStats(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	// Enqueue several jobs.
	_, err := store.Enqueue(ctx, "type_a", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)
	_, err = store.Enqueue(ctx, "type_b", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)
	_, err = store.Enqueue(ctx, "type_a", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	// List all.
	all, err := store.List(ctx, "", "", 10, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, len(all))

	// List by type.
	typeA, err := store.List(ctx, "", "type_a", 10, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(typeA))

	// List by state.
	queued, err := store.List(ctx, "queued", "", 10, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, len(queued))

	// Stats.
	stats, err := store.Stats(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, stats.Queued)
	testutil.Equal(t, 0, stats.Running)
}

// --- Schedule Tests ---

func TestScheduleCRUD(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	sched, err := store.CreateSchedule(ctx, &jobs.Schedule{
		Name:        "test_schedule",
		JobType:     "test_job",
		CronExpr:    "0 * * * *",
		Timezone:    "UTC",
		Enabled:     true,
		MaxAttempts: 3,
	})
	testutil.NoError(t, err)
	testutil.Equal(t, "test_schedule", sched.Name)

	// Get by ID.
	got, err := store.GetSchedule(ctx, sched.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, sched.ID, got.ID)

	// Get by name.
	byName, err := store.GetScheduleByName(ctx, "test_schedule")
	testutil.NoError(t, err)
	testutil.Equal(t, sched.ID, byName.ID)

	// List.
	list, err := store.ListSchedules(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(list))

	// Update.
	nextRun := time.Now().Add(1 * time.Hour)
	updated, err := store.UpdateSchedule(ctx, sched.ID, "*/5 * * * *", "America/New_York", nil, true, &nextRun)
	testutil.NoError(t, err)
	testutil.Equal(t, "*/5 * * * *", updated.CronExpr)
	testutil.Equal(t, "America/New_York", updated.Timezone)

	// Delete.
	err = store.DeleteSchedule(ctx, sched.ID)
	testutil.NoError(t, err)

	_, err = store.GetSchedule(ctx, sched.ID)
	testutil.NotNil(t, err)
}

func TestUpsertSchedule(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	sched1, err := store.UpsertSchedule(ctx, &jobs.Schedule{
		Name:        "upsert_test",
		JobType:     "cleanup",
		CronExpr:    "0 * * * *",
		Timezone:    "UTC",
		Enabled:     true,
		MaxAttempts: 3,
	})
	testutil.NoError(t, err)

	// Second upsert should not error, returns existing.
	sched2, err := store.UpsertSchedule(ctx, &jobs.Schedule{
		Name:        "upsert_test",
		JobType:     "cleanup",
		CronExpr:    "0 */2 * * *", // different cron
		Timezone:    "UTC",
		Enabled:     true,
		MaxAttempts: 3,
	})
	testutil.NoError(t, err)
	testutil.Equal(t, sched1.ID, sched2.ID)
	testutil.Equal(t, "0 * * * *", sched2.CronExpr) // should keep original
}

func TestAdvanceScheduleAndEnqueue(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	past := time.Now().Add(-1 * time.Minute)
	sched, err := store.CreateSchedule(ctx, &jobs.Schedule{
		Name:        "advance_test",
		JobType:     "test_job",
		CronExpr:    "0 * * * *",
		Timezone:    "UTC",
		Enabled:     true,
		MaxAttempts: 3,
		NextRunAt:   &past,
	})
	testutil.NoError(t, err)

	future := time.Now().Add(1 * time.Hour)
	advanced, err := store.AdvanceScheduleAndEnqueue(ctx, sched.ID, future, "test_job", nil, 3)
	testutil.NoError(t, err)
	testutil.True(t, advanced, "should advance schedule and enqueue job")

	// Verify a job was enqueued.
	jobList, err := store.List(ctx, "queued", "test_job", 10, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(jobList))
	testutil.NotNil(t, jobList[0].ScheduleID)
	testutil.Equal(t, sched.ID, *jobList[0].ScheduleID)

	// Second advance should fail (next_run_at is now in the future).
	advanced2, err := store.AdvanceScheduleAndEnqueue(ctx, sched.ID, future.Add(1*time.Hour), "test_job", nil, 3)
	testutil.NoError(t, err)
	testutil.False(t, advanced2, "second advance should fail (duplicate prevention)")

	// Still only 1 job — no duplicate.
	jobList2, err := store.List(ctx, "queued", "test_job", 10, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(jobList2))
}

func TestAdvanceScheduleAndEnqueueSkipsDisabledSchedule(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	past := time.Now().Add(-1 * time.Minute)
	sched, err := store.CreateSchedule(ctx, &jobs.Schedule{
		Name:        "advance_disabled_test",
		JobType:     "disabled_job",
		CronExpr:    "0 * * * *",
		Timezone:    "UTC",
		Enabled:     false,
		MaxAttempts: 3,
		NextRunAt:   &past,
	})
	testutil.NoError(t, err)

	future := time.Now().Add(1 * time.Hour)
	advanced, err := store.AdvanceScheduleAndEnqueue(ctx, sched.ID, future, "disabled_job", nil, 3)
	testutil.NoError(t, err)
	testutil.False(t, advanced, "disabled schedules must not advance or enqueue")

	jobList, err := store.List(ctx, "queued", "disabled_job", 10, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(jobList))
}

// --- Lease Renewal Tests ---

func TestExtendLease(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	job, err := store.Enqueue(ctx, "lease_job", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	// Claim with a short lease.
	claimed, err := store.Claim(ctx, "w1", 10*time.Second)
	testutil.NoError(t, err)
	testutil.NotNil(t, claimed.LeaseUntil)
	originalLease := *claimed.LeaseUntil

	// Extend the lease.
	extended, err := store.ExtendLease(ctx, job.ID, 5*time.Minute)
	testutil.NoError(t, err)
	testutil.NotNil(t, extended.LeaseUntil)
	testutil.True(t, extended.LeaseUntil.After(originalLease),
		"extended lease should be later than original")
	testutil.Equal(t, jobs.StateRunning, extended.State)
}

func TestExtendLeaseNonRunningFails(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	job, err := store.Enqueue(ctx, "queued_job", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	// Extending lease on a queued job should fail.
	_, err = store.ExtendLease(ctx, job.ID, 5*time.Minute)
	testutil.NotNil(t, err)
}

// --- CHECK Constraint Tests ---

func TestInvalidStateRejected(t *testing.T) {
	store := setupDB(t)
	ctx := context.Background()

	// Try to insert with invalid state via raw SQL.
	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_jobs (type, state) VALUES ('test', 'invalid_state')`)
	testutil.NotNil(t, err)

	// Valid state works.
	_, err = store.Enqueue(ctx, "valid_test", nil, jobs.EnqueueOpts{})
	testutil.NoError(t, err)
}

func TestMaxAttemptsCheckConstraint(t *testing.T) {
	setupDB(t) // ensure migrations are applied (test isolation)
	ctx := context.Background()

	// max_attempts = 0 should be rejected.
	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_jobs (type, max_attempts) VALUES ('test', 0)`)
	testutil.NotNil(t, err)
}
