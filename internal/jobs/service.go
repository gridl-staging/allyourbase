package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/adhocore/gronx"
)

// ServiceConfig holds runtime parameters for the job service.
type ServiceConfig struct {
	WorkerConcurrency int
	PollInterval      time.Duration
	LeaseDuration     time.Duration
	SchedulerEnabled  bool
	SchedulerTick     time.Duration
	ShutdownTimeout   time.Duration
	WorkerID          string // unique identifier for this instance
}

// DefaultServiceConfig returns production defaults.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		WorkerConcurrency: 4,
		PollInterval:      1 * time.Second,
		LeaseDuration:     5 * time.Minute,
		SchedulerEnabled:  true,
		SchedulerTick:     15 * time.Second,
		ShutdownTimeout:   30 * time.Second,
		WorkerID:          fmt.Sprintf("worker-%d", time.Now().UnixNano()),
	}
}

// Service orchestrates the job queue: worker loop, scheduler, handler dispatch.
type Service struct {
	store    *Store
	logger   *slog.Logger
	cfg      ServiceConfig
	handlers map[string]JobHandler
	mu       sync.RWMutex // protects handlers

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewService creates a new job Service.
func NewService(store *Store, logger *slog.Logger, cfg ServiceConfig) *Service {
	return &Service{
		store:    store,
		logger:   logger,
		cfg:      cfg,
		handlers: make(map[string]JobHandler),
	}
}

// RegisterHandler registers a handler for a job type.
func (s *Service) RegisterHandler(jobType string, handler JobHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[jobType] = handler
}

// Start launches worker goroutines and the scheduler loop.
func (s *Service) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)

	// Start worker goroutines.
	for i := 0; i < s.cfg.WorkerConcurrency; i++ {
		s.wg.Add(1)
		go s.workerLoop(ctx, i)
	}

	// Start scheduler goroutine when enabled.
	if s.cfg.SchedulerEnabled {
		s.wg.Add(1)
		go s.schedulerLoop(ctx)
	}

	// Start crash recovery goroutine.
	s.wg.Add(1)
	go s.recoveryLoop(ctx)

	s.logger.Info("job service started",
		"workers", s.cfg.WorkerConcurrency,
		"poll_interval", s.cfg.PollInterval,
		"scheduler_enabled", s.cfg.SchedulerEnabled,
		"scheduler_tick", s.cfg.SchedulerTick,
	)
}

// Stop signals all goroutines to stop and waits for in-progress jobs to finish.
func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.logger.Info("job service stopped")
}

func (s *Service) workerLoop(ctx context.Context, workerNum int) {
	defer s.wg.Done()
	workerID := fmt.Sprintf("%s-%d", s.cfg.WorkerID, workerNum)
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollAndProcess(ctx, workerID)
		}
	}
}

func (s *Service) pollAndProcess(ctx context.Context, workerID string) {
	job, err := s.store.Claim(ctx, workerID, s.cfg.LeaseDuration)
	if err != nil {
		if ctx.Err() != nil {
			return // shutting down
		}
		s.logger.Error("failed to claim job", "error", err, "worker", workerID)
		return
	}
	if job == nil {
		return // no jobs available
	}

	s.logger.Info("claimed job", "job_id", job.ID, "type", job.Type,
		"attempt", job.Attempts, "worker", workerID)

	s.mu.RLock()
	handler, ok := s.handlers[job.Type]
	s.mu.RUnlock()

	// Use a separate context for handler execution so that in-flight jobs
	// can finish their DB operations during graceful shutdown. The poll loop's
	// ctx may already be cancelled, but the handler needs a live context to
	// complete or fail the job cleanly. With lease renewal the handler is no
	// longer hard-capped at the lease duration — the shutdown timeout bounds
	// total in-flight execution instead.
	handlerCtx, handlerCancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer handlerCancel()

	// Start lease renewal goroutine. It extends the lease every half-period
	// so crash recovery won't reclaim the job while the handler is still running.
	renewCtx, renewCancel := context.WithCancel(handlerCtx)
	defer renewCancel()
	go s.renewLease(renewCtx, job.ID)

	var jobErr error
	if !ok {
		jobErr = fmt.Errorf("no handler registered for job type %q", job.Type)
	} else {
		jobErr = handler(handlerCtx, job.Payload)
	}

	// Stop lease renewal before updating final state.
	renewCancel()

	if jobErr != nil {
		backoff := ComputeBackoff(job.Attempts)
		_, failErr := s.store.Fail(handlerCtx, job.ID, jobErr.Error(), backoff)
		if failErr != nil {
			s.logger.Error("failed to record job failure",
				"job_id", job.ID, "error", failErr)
		} else {
			s.logger.Warn("job failed", "job_id", job.ID, "type", job.Type,
				"attempt", job.Attempts, "error", jobErr.Error())
		}
		return
	}

	_, completeErr := s.store.Complete(handlerCtx, job.ID)
	if completeErr != nil {
		s.logger.Error("failed to complete job",
			"job_id", job.ID, "error", completeErr)
	} else {
		s.logger.Info("job completed", "job_id", job.ID, "type", job.Type)
	}
}

// renewLease periodically extends a running job's lease until the context is cancelled.
// The renewal interval is half the configured lease duration, ensuring the lease is
// always refreshed well before expiry.
func (s *Service) renewLease(ctx context.Context, jobID string) {
	interval := s.cfg.LeaseDuration / 2
	if interval < 1*time.Second {
		interval = 1 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := s.store.ExtendLease(ctx, jobID, s.cfg.LeaseDuration)
			if err != nil {
				if ctx.Err() != nil {
					return // cancelled, expected during completion
				}
				s.logger.Error("failed to extend lease",
					"job_id", jobID, "error", err)
			}
		}
	}
}

func (s *Service) schedulerLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.cfg.SchedulerTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.schedulerTick(ctx)
		}
	}
}

func (s *Service) schedulerTick(ctx context.Context) {
	schedules, err := s.store.DueSchedules(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.logger.Error("failed to fetch due schedules", "error", err)
		return
	}

	for i := range schedules {
		sched := &schedules[i]
		nextRunAt, err := CronNextTime(sched.CronExpr, sched.Timezone, time.Now())
		if err != nil {
			s.logger.Error("failed to compute next run time",
				"schedule", sched.Name, "cron", sched.CronExpr, "error", err)
			continue
		}

		advanced, err := s.store.AdvanceScheduleAndEnqueue(
			ctx, sched.ID, nextRunAt, sched.JobType, sched.Payload, sched.MaxAttempts,
		)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Error("failed to advance schedule and enqueue job",
				"schedule", sched.Name, "error", err)
			continue
		}
		if !advanced {
			continue // another instance handled this tick
		}

		s.logger.Info("enqueued scheduled job",
			"schedule", sched.Name, "type", sched.JobType, "next_run", nextRunAt)
	}
}

func (s *Service) recoveryLoop(ctx context.Context) {
	defer s.wg.Done()
	// Run recovery at the lease duration interval (minimum 30s).
	interval := s.cfg.LeaseDuration
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			recovered, err := s.store.RecoverStalledJobs(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				s.logger.Error("failed to recover stalled jobs", "error", err)
				continue
			}
			if recovered > 0 {
				s.logger.Info("recovered stalled jobs", "count", recovered)
			}
		}
	}
}

// CronNextTime computes the next run time for a cron expression after refTime in the given timezone.
func CronNextTime(cronExpr, tz string, refTime time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timezone %q: %w", tz, err)
	}

	gron := gronx.New()
	if !gron.IsValid(cronExpr) {
		return time.Time{}, fmt.Errorf("invalid cron expression %q", cronExpr)
	}

	// Convert ref to the target timezone for computation.
	refInTZ := refTime.In(loc)
	next, err := gronx.NextTickAfter(cronExpr, refInTZ, false)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to compute next tick for %q: %w", cronExpr, err)
	}

	return next.UTC(), nil
}

// --- Delegate methods to Store for convenience ---

// Enqueue delegates to the underlying store.
func (s *Service) Enqueue(ctx context.Context, jobType string, payload json.RawMessage, opts EnqueueOpts) (*Job, error) {
	return s.store.Enqueue(ctx, jobType, payload, opts)
}

// Get delegates to the underlying store.
func (s *Service) Get(ctx context.Context, jobID string) (*Job, error) {
	return s.store.Get(ctx, jobID)
}

// List delegates to the underlying store.
func (s *Service) List(ctx context.Context, state, jobType string, limit, offset int) ([]Job, error) {
	return s.store.List(ctx, state, jobType, limit, offset)
}

// Stats delegates to the underlying store.
func (s *Service) Stats(ctx context.Context) (*QueueStats, error) {
	return s.store.Stats(ctx)
}

// Cancel delegates to the underlying store.
func (s *Service) Cancel(ctx context.Context, jobID string) (*Job, error) {
	return s.store.Cancel(ctx, jobID)
}

// RetryNow delegates to the underlying store.
func (s *Service) RetryNow(ctx context.Context, jobID string) (*Job, error) {
	return s.store.RetryNow(ctx, jobID)
}

// CreateSchedule delegates to the underlying store.
func (s *Service) CreateSchedule(ctx context.Context, sched *Schedule) (*Schedule, error) {
	return s.store.CreateSchedule(ctx, sched)
}

// GetSchedule delegates to the underlying store.
func (s *Service) GetSchedule(ctx context.Context, id string) (*Schedule, error) {
	return s.store.GetSchedule(ctx, id)
}

// GetScheduleByName delegates to the underlying store.
func (s *Service) GetScheduleByName(ctx context.Context, name string) (*Schedule, error) {
	return s.store.GetScheduleByName(ctx, name)
}

// ListSchedules delegates to the underlying store.
func (s *Service) ListSchedules(ctx context.Context) ([]Schedule, error) {
	return s.store.ListSchedules(ctx)
}

// UpdateSchedule delegates to the underlying store.
func (s *Service) UpdateSchedule(ctx context.Context, id string, cronExpr, timezone string, payload json.RawMessage, enabled bool, nextRunAt *time.Time) (*Schedule, error) {
	return s.store.UpdateSchedule(ctx, id, cronExpr, timezone, payload, enabled, nextRunAt)
}

// DeleteSchedule delegates to the underlying store.
func (s *Service) DeleteSchedule(ctx context.Context, id string) error {
	return s.store.DeleteSchedule(ctx, id)
}

// SetScheduleEnabled delegates to the underlying store.
func (s *Service) SetScheduleEnabled(ctx context.Context, id string, enabled bool) (*Schedule, error) {
	var nextRunAt *time.Time
	if enabled {
		// Recompute next_run_at from now.
		sched, err := s.store.GetSchedule(ctx, id)
		if err != nil {
			return nil, err
		}
		t, err := CronNextTime(sched.CronExpr, sched.Timezone, time.Now())
		if err != nil {
			return nil, err
		}
		nextRunAt = &t
	}
	return s.store.SetScheduleEnabled(ctx, id, enabled, nextRunAt)
}

// RegisterDefaultSchedules inserts the built-in schedule definitions (idempotent).
func (s *Service) RegisterDefaultSchedules(ctx context.Context) error {
	defaults := []Schedule{
		{
			Name:        "session_cleanup_hourly",
			JobType:     "stale_session_cleanup",
			CronExpr:    "0 * * * *",
			Timezone:    "UTC",
			Enabled:     true,
			MaxAttempts: 3,
		},
		{
			Name:        "webhook_delivery_prune_daily",
			JobType:     "webhook_delivery_prune",
			Payload:     json.RawMessage(`{"retention_hours": 168}`),
			CronExpr:    "0 3 * * *",
			Timezone:    "UTC",
			Enabled:     true,
			MaxAttempts: 3,
		},
		{
			Name:        "expired_oauth_cleanup_daily",
			JobType:     "expired_oauth_cleanup",
			CronExpr:    "0 4 * * *",
			Timezone:    "UTC",
			Enabled:     true,
			MaxAttempts: 3,
		},
		{
			Name:        "expired_auth_cleanup_daily",
			JobType:     "expired_auth_cleanup",
			CronExpr:    "0 5 * * *",
			Timezone:    "UTC",
			Enabled:     true,
			MaxAttempts: 3,
		},
	}

	for i := range defaults {
		sched := &defaults[i]
		// Compute initial next_run_at.
		next, err := CronNextTime(sched.CronExpr, sched.Timezone, time.Now())
		if err != nil {
			return fmt.Errorf("compute next_run_at for %s: %w", sched.Name, err)
		}
		sched.NextRunAt = &next

		if _, err := s.store.UpsertSchedule(ctx, sched); err != nil {
			return fmt.Errorf("upsert default schedule %s: %w", sched.Name, err)
		}
	}
	return nil
}
