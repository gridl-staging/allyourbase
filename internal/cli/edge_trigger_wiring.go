// Package cli This file wires the edge trigger runtime and provides scheduler adapters for cron and database-backed job scheduling.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"time"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

type edgeTriggerJobService interface {
	edgefunc.JobsScheduler
	RegisterHandler(jobType string, handler jobs.JobHandler)
}

type storageEventRegistrar interface {
	RegisterEventHandler(handler storage.StorageEventHandler)
}

type dbTriggerWorkerRunner interface {
	Start(ctx context.Context) error
}

func isNilStorageEventRegistrar(registrar storageEventRegistrar) bool {
	if registrar == nil {
		return true
	}
	value := reflect.ValueOf(registrar)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

var newDBTriggerWorkerRunner = func(
	eventStore edgefunc.DBTriggerEventStore,
	dispatcher *edgefunc.DBTriggerDispatcher,
	connString string,
	logger *slog.Logger,
) dbTriggerWorkerRunner {
	return edgefunc.NewDBTriggerWorker(eventStore, dispatcher, connString, logger)
}

var newCronFallbackScheduler = func(dbPool *pgxpool.Pool) edgefunc.JobsScheduler {
	if dbPool == nil {
		return nil
	}
	return &storeBackedCronScheduler{store: jobs.NewStore(dbPool)}
}

// wireEdgeTriggerRuntime initializes and wires the edge trigger runtime, setting up database, cron, and storage trigger services, registering event handlers, and starting a background worker for database triggers.
func wireEdgeTriggerRuntime(
	ctx context.Context,
	dbPool *pgxpool.Pool,
	dbConnString string,
	storageSvc storageEventRegistrar,
	jobSvc edgeTriggerJobService,
	invoker edgefunc.FunctionInvoker,
	logger *slog.Logger,
	setServices func(db *edgefunc.DBTriggerService, cron *edgefunc.CronTriggerService, st *edgefunc.StorageTriggerService, invoker edgefunc.FunctionInvoker),
) {
	if setServices == nil || invoker == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}

	dbStore := edgefunc.NewDBTriggerPostgresStore(dbPool)
	dbSvc := edgefunc.NewDBTriggerService(dbStore)

	storageStore := edgefunc.NewStorageTriggerPostgresStore(dbPool)
	storageTriggerSvc := edgefunc.NewStorageTriggerService(storageStore)

	var cronScheduler edgefunc.JobsScheduler
	var cronSvc *edgefunc.CronTriggerService
	if jobSvc != nil {
		cronScheduler = jobSvc
		jobSvc.RegisterHandler(edgefunc.CronJobType, edgefunc.NewCronJobHandler(invoker))
	} else {
		cronScheduler = newCronFallbackScheduler(dbPool)
	}
	if cronScheduler != nil {
		cronStore := edgefunc.NewCronTriggerPostgresStore(dbPool)
		cronSvc = edgefunc.NewCronTriggerService(cronStore, cronScheduler)
	}

	setServices(dbSvc, cronSvc, storageTriggerSvc, invoker)

	if !isNilStorageEventRegistrar(storageSvc) {
		storageSvc.RegisterEventHandler(edgefunc.NewStorageTriggerDispatcher(
			storageStore,
			invoker,
			edgefunc.WithDispatcherLogger(logger),
		))
	}

	worker := newDBTriggerWorkerRunner(
		edgefunc.NewDBTriggerEventPostgresStore(dbPool),
		edgefunc.NewDBTriggerDispatcher(dbStore, invoker, edgefunc.WithDBDispatcherLogger(logger)),
		dbConnString,
		logger,
	)
	go func() {
		if err := worker.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("db trigger worker stopped", "error", err)
		}
	}()
}

type cronJobsSchedulerAdapter struct {
	svc *jobs.Service
}

func newCronJobsSchedulerAdapter(svc *jobs.Service) edgeTriggerJobService {
	if svc == nil {
		return nil
	}
	return &cronJobsSchedulerAdapter{svc: svc}
}

// CreateSchedule creates a new cron schedule with the given parameters, delegating to the underlying jobs service and returning the generated schedule ID.
func (a *cronJobsSchedulerAdapter) CreateSchedule(
	ctx context.Context,
	name, jobType string,
	payload json.RawMessage,
	cronExpr, timezone string,
	maxAttempts int,
	nextRunAt *time.Time,
) (string, error) {
	sched, err := a.svc.CreateSchedule(ctx, &jobs.Schedule{
		Name:        name,
		JobType:     jobType,
		Payload:     payload,
		CronExpr:    cronExpr,
		Timezone:    timezone,
		Enabled:     true,
		MaxAttempts: maxAttempts,
		NextRunAt:   nextRunAt,
	})
	if err != nil {
		return "", err
	}
	return sched.ID, nil
}

func (a *cronJobsSchedulerAdapter) DeleteSchedule(ctx context.Context, id string) error {
	return a.svc.DeleteSchedule(ctx, id)
}

func (a *cronJobsSchedulerAdapter) SetScheduleEnabled(ctx context.Context, id string, enabled bool) error {
	_, err := a.svc.SetScheduleEnabled(ctx, id, enabled)
	return err
}

func (a *cronJobsSchedulerAdapter) RegisterHandler(jobType string, handler jobs.JobHandler) {
	a.svc.RegisterHandler(jobType, handler)
}

type storeBackedCronScheduler struct {
	store *jobs.Store
}

// CreateSchedule creates a new cron schedule, providing defaults for timezone (UTC), payload (empty JSON object), and max attempts (3), and computing the next run time from the cron expression if not provided.
func (s *storeBackedCronScheduler) CreateSchedule(
	ctx context.Context,
	name, jobType string,
	payload json.RawMessage,
	cronExpr, timezone string,
	maxAttempts int,
	nextRunAt *time.Time,
) (string, error) {
	if timezone == "" {
		timezone = "UTC"
	}
	if payload == nil {
		payload = json.RawMessage(`{}`)
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	resolvedNextRun := nextRunAt
	if resolvedNextRun == nil {
		next, err := jobs.CronNextTime(cronExpr, timezone, time.Now())
		if err != nil {
			return "", err
		}
		resolvedNextRun = &next
	}

	sched, err := s.store.CreateSchedule(ctx, &jobs.Schedule{
		Name:        name,
		JobType:     jobType,
		Payload:     payload,
		CronExpr:    cronExpr,
		Timezone:    timezone,
		Enabled:     true,
		MaxAttempts: maxAttempts,
		NextRunAt:   resolvedNextRun,
	})
	if err != nil {
		return "", err
	}
	return sched.ID, nil
}

func (s *storeBackedCronScheduler) DeleteSchedule(ctx context.Context, id string) error {
	return s.store.DeleteSchedule(ctx, id)
}

// SetScheduleEnabled enables or disables a schedule; when enabling, recalculates the next run time based on the schedule's cron expression and timezone.
func (s *storeBackedCronScheduler) SetScheduleEnabled(ctx context.Context, id string, enabled bool) error {
	var nextRunAt *time.Time
	if enabled {
		sched, err := s.store.GetSchedule(ctx, id)
		if err != nil {
			return err
		}
		next, err := jobs.CronNextTime(sched.CronExpr, sched.Timezone, time.Now())
		if err != nil {
			return err
		}
		nextRunAt = &next
	}
	_, err := s.store.SetScheduleEnabled(ctx, id, enabled, nextRunAt)
	return err
}
