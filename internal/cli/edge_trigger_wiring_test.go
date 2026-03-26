package cli

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeTriggerJobService struct {
	registeredType    string
	registeredHandler jobs.JobHandler
}

func (f *fakeTriggerJobService) CreateSchedule(context.Context, string, string, json.RawMessage, string, string, int, *time.Time) (string, error) {
	return "sched-1", nil
}

func (f *fakeTriggerJobService) DeleteSchedule(context.Context, string) error {
	return nil
}

func (f *fakeTriggerJobService) SetScheduleEnabled(context.Context, string, bool) error {
	return nil
}

func (f *fakeTriggerJobService) RegisterHandler(jobType string, handler jobs.JobHandler) {
	f.registeredType = jobType
	f.registeredHandler = handler
}

type fakeStorageEventRegistrar struct {
	handlers []storage.StorageEventHandler
}

func (f *fakeStorageEventRegistrar) RegisterEventHandler(h storage.StorageEventHandler) {
	f.handlers = append(f.handlers, h)
}

type fakeDBTriggerWorker struct {
	startCalls atomic.Int32
	err        error
}

func (w *fakeDBTriggerWorker) Start(context.Context) error {
	w.startCalls.Add(1)
	return w.err
}

type fakeFunctionInvoker struct{}

func (f *fakeFunctionInvoker) InvokeByID(_ context.Context, _ string, _ edgefunc.Request) (edgefunc.Response, error) {
	return edgefunc.Response{StatusCode: 200, Body: []byte("ok")}, nil
}

func waitForWorkerStart(t *testing.T, worker *fakeDBTriggerWorker) {
	t.Helper()
	deadline := time.Now().Add(200 * time.Millisecond)
	for worker.startCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWireEdgeTriggerRuntime_WiresServicesAndHandlers(t *testing.T) {
	invoker := &fakeFunctionInvoker{}
	jobSvc := &fakeTriggerJobService{}
	storageRegistrar := &fakeStorageEventRegistrar{}
	worker := &fakeDBTriggerWorker{}

	origFactory := newDBTriggerWorkerRunner
	t.Cleanup(func() { newDBTriggerWorkerRunner = origFactory })
	newDBTriggerWorkerRunner = func(edgefunc.DBTriggerEventStore, *edgefunc.DBTriggerDispatcher, string, *slog.Logger) dbTriggerWorkerRunner {
		return worker
	}

	var gotDB *edgefunc.DBTriggerService
	var gotCron *edgefunc.CronTriggerService
	var gotStorage *edgefunc.StorageTriggerService
	var gotInvoker edgefunc.FunctionInvoker

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wireEdgeTriggerRuntime(ctx, nil, "postgres://test", storageRegistrar, jobSvc, invoker, slog.Default(),
		func(db *edgefunc.DBTriggerService, cron *edgefunc.CronTriggerService, st *edgefunc.StorageTriggerService, fnInv edgefunc.FunctionInvoker) {
			gotDB = db
			gotCron = cron
			gotStorage = st
			gotInvoker = fnInv
		},
	)

	testutil.NotNil(t, gotDB)
	testutil.NotNil(t, gotCron)
	testutil.NotNil(t, gotStorage)
	testutil.True(t, gotInvoker == invoker, "expected registered invoker to match")
	testutil.Equal(t, edgefunc.CronJobType, jobSvc.registeredType)
	testutil.NotNil(t, jobSvc.registeredHandler)
	testutil.SliceLen(t, storageRegistrar.handlers, 1)
	waitForWorkerStart(t, worker)
	testutil.Equal(t, int32(1), worker.startCalls.Load())
}

func TestWireEdgeTriggerRuntime_WorkerCancellationIsNotLoggedAsError(t *testing.T) {
	invoker := &fakeFunctionInvoker{}
	worker := &fakeDBTriggerWorker{err: context.Canceled}

	origFactory := newDBTriggerWorkerRunner
	t.Cleanup(func() { newDBTriggerWorkerRunner = origFactory })
	newDBTriggerWorkerRunner = func(edgefunc.DBTriggerEventStore, *edgefunc.DBTriggerDispatcher, string, *slog.Logger) dbTriggerWorkerRunner {
		return worker
	}

	ctx := context.Background()
	var gotCron *edgefunc.CronTriggerService
	wireEdgeTriggerRuntime(ctx, nil, "postgres://test", nil, nil, invoker, slog.Default(),
		func(_ *edgefunc.DBTriggerService, cron *edgefunc.CronTriggerService, _ *edgefunc.StorageTriggerService, _ edgefunc.FunctionInvoker) {
			gotCron = cron
		},
	)

	waitForWorkerStart(t, worker)

	testutil.Nil(t, gotCron)
	testutil.Equal(t, int32(1), worker.startCalls.Load())
	testutil.True(t, errors.Is(worker.err, context.Canceled))
}

func TestWireEdgeTriggerRuntime_UsesFallbackCronSchedulerWhenJobsDisabled(t *testing.T) {
	invoker := &fakeFunctionInvoker{}
	worker := &fakeDBTriggerWorker{}
	var pool pgxpool.Pool

	origFactory := newDBTriggerWorkerRunner
	origFallback := newCronFallbackScheduler
	t.Cleanup(func() {
		newDBTriggerWorkerRunner = origFactory
		newCronFallbackScheduler = origFallback
	})

	newDBTriggerWorkerRunner = func(edgefunc.DBTriggerEventStore, *edgefunc.DBTriggerDispatcher, string, *slog.Logger) dbTriggerWorkerRunner {
		return worker
	}

	fallbackCalled := false
	newCronFallbackScheduler = func(dbPool *pgxpool.Pool) edgefunc.JobsScheduler {
		fallbackCalled = true
		testutil.True(t, dbPool == &pool, "expected db pool to be forwarded to fallback scheduler")
		return &fakeTriggerJobService{}
	}

	var gotCron *edgefunc.CronTriggerService
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wireEdgeTriggerRuntime(ctx, &pool, "postgres://test", nil, nil, invoker, slog.Default(),
		func(_ *edgefunc.DBTriggerService, cron *edgefunc.CronTriggerService, _ *edgefunc.StorageTriggerService, _ edgefunc.FunctionInvoker) {
			gotCron = cron
		},
	)

	waitForWorkerStart(t, worker)
	testutil.True(t, fallbackCalled, "expected fallback cron scheduler to be used when jobs service is nil")
	testutil.NotNil(t, gotCron)
}

func TestWireEdgeTriggerRuntime_TypedNilStorageRegistrarDoesNotPanic(t *testing.T) {
	invoker := &fakeFunctionInvoker{}
	worker := &fakeDBTriggerWorker{}

	origFactory := newDBTriggerWorkerRunner
	t.Cleanup(func() { newDBTriggerWorkerRunner = origFactory })
	newDBTriggerWorkerRunner = func(edgefunc.DBTriggerEventStore, *edgefunc.DBTriggerDispatcher, string, *slog.Logger) dbTriggerWorkerRunner {
		return worker
	}

	var typedNilRegistrar storageEventRegistrar = (*fakeStorageEventRegistrar)(nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("wireEdgeTriggerRuntime panicked with typed nil storage registrar: %v", r)
		}
	}()

	wireEdgeTriggerRuntime(ctx, nil, "postgres://test", typedNilRegistrar, nil, invoker, slog.Default(),
		func(_ *edgefunc.DBTriggerService, _ *edgefunc.CronTriggerService, _ *edgefunc.StorageTriggerService, _ edgefunc.FunctionInvoker) {
		},
	)

	waitForWorkerStart(t, worker)
	testutil.Equal(t, int32(1), worker.startCalls.Load())
}
