//go:build integration

package edgefunc_test

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
)

// testPool and TestMain are defined in store_integration_test.go

func createTestFunction(t *testing.T) *edgefunc.EdgeFunction {
	t.Helper()
	store := edgefunc.NewPostgresStore(testPool)
	fn, err := store.Create(context.Background(), &edgefunc.EdgeFunction{
		Name:       "test-log-fn-" + uuid.New().String()[:8],
		EntryPoint: "handler",
		Source:     "function handler(req) { return { statusCode: 200, body: 'ok' }; }",
		CompiledJS: "function handler(req) { return { statusCode: 200, body: 'ok' }; }",
	})
	testutil.NoError(t, err)
	t.Cleanup(func() {
		store.Delete(context.Background(), fn.ID)
	})
	return fn
}

func TestLogStoreWriteLog(t *testing.T) {
	fn := createTestFunction(t)
	ctx := context.Background()
	logStore := edgefunc.NewPostgresLogStore(testPool)

	entry := &edgefunc.LogEntry{
		FunctionID:         fn.ID,
		InvocationID:       uuid.New(),
		Status:             "success",
		DurationMs:         42,
		Stdout:             "hello world\n",
		StdoutBytes:        len("hello world\n"),
		ResponseStatusCode: 200,
		RequestMethod:      "GET",
		RequestPath:        "/functions/v1/test",
	}

	err := logStore.WriteLog(ctx, entry)
	testutil.NoError(t, err)
	testutil.True(t, entry.ID != uuid.Nil, "should have assigned an ID")
	testutil.True(t, !entry.CreatedAt.IsZero(), "should have CreatedAt")
	testutil.Equal(t, "success", entry.Status)
	testutil.Equal(t, 42, entry.DurationMs)
	testutil.Equal(t, len("hello world\n"), entry.StdoutBytes)
	testutil.Equal(t, 200, entry.ResponseStatusCode)
}

func TestLogStoreWriteLog_Error(t *testing.T) {
	fn := createTestFunction(t)
	ctx := context.Background()
	logStore := edgefunc.NewPostgresLogStore(testPool)

	entry := &edgefunc.LogEntry{
		FunctionID:    fn.ID,
		InvocationID:  uuid.New(),
		Status:        "error",
		DurationMs:    15,
		Error:         "ReferenceError: foo is not defined",
		RequestMethod: "POST",
		RequestPath:   "/functions/v1/broken",
	}

	err := logStore.WriteLog(ctx, entry)
	testutil.NoError(t, err)
	testutil.Equal(t, "error", entry.Status)
	testutil.Equal(t, "ReferenceError: foo is not defined", entry.Error)
}

func TestLogStoreWriteLog_AutoGeneratesInvocationID(t *testing.T) {
	fn := createTestFunction(t)
	ctx := context.Background()
	logStore := edgefunc.NewPostgresLogStore(testPool)

	entry := &edgefunc.LogEntry{
		FunctionID: fn.ID,
		Status:     "success",
		DurationMs: 10,
	}

	err := logStore.WriteLog(ctx, entry)
	testutil.NoError(t, err)
	testutil.True(t, entry.InvocationID != uuid.Nil, "should auto-generate invocation ID")
}

func TestLogStoreListByFunction(t *testing.T) {
	fn := createTestFunction(t)
	ctx := context.Background()
	logStore := edgefunc.NewPostgresLogStore(testPool)

	// Write 3 log entries
	for i := 0; i < 3; i++ {
		err := logStore.WriteLog(ctx, &edgefunc.LogEntry{
			FunctionID:    fn.ID,
			InvocationID:  uuid.New(),
			Status:        "success",
			DurationMs:    i * 10,
			RequestMethod: "GET",
			RequestPath:   "/test",
		})
		testutil.NoError(t, err)
	}

	// List all
	logs, err := logStore.ListByFunction(ctx, fn.ID, edgefunc.LogListOptions{Page: 1, PerPage: 50})
	testutil.NoError(t, err)
	testutil.SliceLen(t, logs, 3)

	// Newest first
	testutil.True(t, !logs[0].CreatedAt.Before(logs[1].CreatedAt),
		"logs should be ordered newest first")
}

func TestLogStoreListByFunction_Pagination(t *testing.T) {
	fn := createTestFunction(t)
	ctx := context.Background()
	logStore := edgefunc.NewPostgresLogStore(testPool)

	for i := 0; i < 5; i++ {
		err := logStore.WriteLog(ctx, &edgefunc.LogEntry{
			FunctionID:   fn.ID,
			InvocationID: uuid.New(),
			Status:       "success",
			DurationMs:   i,
		})
		testutil.NoError(t, err)
	}

	// Page 1, 2 per page
	page1, err := logStore.ListByFunction(ctx, fn.ID, edgefunc.LogListOptions{Page: 1, PerPage: 2})
	testutil.NoError(t, err)
	testutil.SliceLen(t, page1, 2)

	// Page 2
	page2, err := logStore.ListByFunction(ctx, fn.ID, edgefunc.LogListOptions{Page: 2, PerPage: 2})
	testutil.NoError(t, err)
	testutil.SliceLen(t, page2, 2)

	// Page 3
	page3, err := logStore.ListByFunction(ctx, fn.ID, edgefunc.LogListOptions{Page: 3, PerPage: 2})
	testutil.NoError(t, err)
	testutil.SliceLen(t, page3, 1)
}

func TestLogStoreListByFunction_Empty(t *testing.T) {
	ctx := context.Background()
	logStore := edgefunc.NewPostgresLogStore(testPool)

	logs, err := logStore.ListByFunction(ctx, uuid.New(), edgefunc.LogListOptions{Page: 1, PerPage: 50})
	testutil.NoError(t, err)
	testutil.SliceLen(t, logs, 0)
}

func TestLogStoreListByFunction_IsolatedByFunction(t *testing.T) {
	fn1 := createTestFunction(t)
	fn2 := createTestFunction(t)
	ctx := context.Background()
	logStore := edgefunc.NewPostgresLogStore(testPool)

	// Write logs for both functions
	for i := 0; i < 3; i++ {
		logStore.WriteLog(ctx, &edgefunc.LogEntry{
			FunctionID: fn1.ID, InvocationID: uuid.New(), Status: "success", DurationMs: i,
		})
		logStore.WriteLog(ctx, &edgefunc.LogEntry{
			FunctionID: fn2.ID, InvocationID: uuid.New(), Status: "success", DurationMs: i,
		})
	}

	// Listing fn1 should only return fn1's logs
	logs, err := logStore.ListByFunction(ctx, fn1.ID, edgefunc.LogListOptions{})
	testutil.NoError(t, err)
	testutil.SliceLen(t, logs, 3)
	for _, l := range logs {
		testutil.Equal(t, fn1.ID, l.FunctionID)
	}
}
