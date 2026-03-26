package edgefunc_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
)

// --- Mock Store ---

type mockStore struct {
	functions map[uuid.UUID]*edgefunc.EdgeFunction
	byName    map[string]uuid.UUID
}

func newMockStore() *mockStore {
	return &mockStore{
		functions: make(map[uuid.UUID]*edgefunc.EdgeFunction),
		byName:    make(map[string]uuid.UUID),
	}
}

func (m *mockStore) Create(_ context.Context, fn *edgefunc.EdgeFunction) (*edgefunc.EdgeFunction, error) {
	if _, exists := m.byName[fn.Name]; exists {
		return nil, edgefunc.ErrFunctionNameConflict
	}
	fn.ID = uuid.New()
	fn.CreatedAt = time.Now()
	fn.UpdatedAt = fn.CreatedAt
	m.functions[fn.ID] = fn
	m.byName[fn.Name] = fn.ID
	return fn, nil
}

func (m *mockStore) Get(_ context.Context, id uuid.UUID) (*edgefunc.EdgeFunction, error) {
	fn, ok := m.functions[id]
	if !ok {
		return nil, edgefunc.ErrFunctionNotFound
	}
	return fn, nil
}

func (m *mockStore) GetByName(_ context.Context, name string) (*edgefunc.EdgeFunction, error) {
	id, ok := m.byName[name]
	if !ok {
		return nil, edgefunc.ErrFunctionNotFound
	}
	return m.functions[id], nil
}

func (m *mockStore) List(_ context.Context, page, perPage int) ([]*edgefunc.EdgeFunction, error) {
	result := make([]*edgefunc.EdgeFunction, 0, len(m.functions))
	for _, fn := range m.functions {
		result = append(result, fn)
	}
	return result, nil
}

func (m *mockStore) Update(_ context.Context, fn *edgefunc.EdgeFunction) (*edgefunc.EdgeFunction, error) {
	if _, ok := m.functions[fn.ID]; !ok {
		return nil, edgefunc.ErrFunctionNotFound
	}
	fn.UpdatedAt = time.Now()
	m.functions[fn.ID] = fn
	return fn, nil
}

func (m *mockStore) Delete(_ context.Context, id uuid.UUID) error {
	fn, ok := m.functions[id]
	if !ok {
		return edgefunc.ErrFunctionNotFound
	}
	delete(m.byName, fn.Name)
	delete(m.functions, id)
	return nil
}

// --- Mock LogStore ---

type mockLogStore struct {
	entries []*edgefunc.LogEntry
}

func newMockLogStore() *mockLogStore {
	return &mockLogStore{}
}

func (m *mockLogStore) WriteLog(_ context.Context, entry *edgefunc.LogEntry) error {
	entry.ID = uuid.New()
	entry.CreatedAt = time.Now()
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockLogStore) ListByFunction(_ context.Context, functionID uuid.UUID, _ edgefunc.LogListOptions) ([]*edgefunc.LogEntry, error) {
	var result []*edgefunc.LogEntry
	for _, e := range m.entries {
		if e.FunctionID == functionID {
			result = append(result, e)
		}
	}
	if result == nil {
		result = []*edgefunc.LogEntry{}
	}
	return result, nil
}

type mockInvocationLogWriter struct {
	calls []mockInvocationLogEntry
}

type mockInvocationLogEntry struct {
	functionName string
	invocationID string
	status       string
	durationMs   int
	stdout       string
	errMsg       string
}

func (m *mockInvocationLogWriter) WriteLog(_ context.Context, functionName, invocationID, status string, durationMs int, stdout, errMsg string) {
	m.calls = append(m.calls, mockInvocationLogEntry{
		functionName: functionName,
		invocationID: invocationID,
		status:       status,
		durationMs:   durationMs,
		stdout:       stdout,
		errMsg:       errMsg,
	})
}

type mockQueryExecutor struct {
	lastQuery edgefunc.Query
	result    edgefunc.QueryResult
	err       error
}

func (m *mockQueryExecutor) Execute(_ context.Context, query edgefunc.Query) (edgefunc.QueryResult, error) {
	m.lastQuery = query
	if m.err != nil {
		return edgefunc.QueryResult{}, m.err
	}
	return m.result, nil
}

// --- Service Tests ---

func TestServiceDeploy_JS(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	fn, err := svc.Deploy(context.Background(), "hello", `function handler(req) { return { statusCode: 200, body: "hello" }; }`, edgefunc.DeployOptions{
		EntryPoint: "handler",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, "hello", fn.Name)
	testutil.Equal(t, "handler", fn.EntryPoint)
	testutil.True(t, fn.Source != "", "source should be stored")
	testutil.True(t, fn.CompiledJS != "", "compiled JS should be stored")
	testutil.True(t, fn.ID != uuid.Nil, "should have an ID")
}

func TestServiceDeploy_DefaultEntryPoint(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	fn, err := svc.Deploy(context.Background(), "default-ep", `function handler(req) { return { statusCode: 200 }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)
	testutil.Equal(t, "handler", fn.EntryPoint)
}

func TestServiceDeploy_WithOptions(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	fn, err := svc.Deploy(context.Background(), "opts-fn", `function main(req) { return { statusCode: 200 }; }`, edgefunc.DeployOptions{
		EntryPoint: "main",
		TimeoutMs:  3000,
		EnvVars:    map[string]string{"API_KEY": "secret"},
		Public:     true,
	})
	testutil.NoError(t, err)
	testutil.Equal(t, "main", fn.EntryPoint)
	testutil.Equal(t, 3*time.Second, fn.Timeout)
	testutil.Equal(t, "secret", fn.EnvVars["API_KEY"])
	testutil.True(t, fn.Public, "should be public")
}

func TestServiceDeploy_UsesConfiguredDefaultTimeout(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore, edgefunc.WithDefaultTimeout(9*time.Second))

	fn, err := svc.Deploy(context.Background(), "default-timeout", `function handler(req) { return { statusCode: 200 }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)
	testutil.Equal(t, 9*time.Second, fn.Timeout)
}

func TestServiceDeploy_CompileError(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "bad-js", `function handler( { broken syntax`, edgefunc.DeployOptions{})
	testutil.True(t, err != nil, "should fail on compile error")
}

func TestServiceDeploy_DuplicateName(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "dup", `function handler(req) { return { statusCode: 200 }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	_, err = svc.Deploy(context.Background(), "dup", `function handler(req) { return { statusCode: 200 }; }`, edgefunc.DeployOptions{})
	testutil.True(t, errors.Is(err, edgefunc.ErrFunctionNameConflict), "should return name conflict error, got: %v", err)
}

func TestServiceInvoke(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "greet", `function handler(req) { return { statusCode: 200, body: "hi " + req.method }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	resp, err := svc.Invoke(context.Background(), "greet", edgefunc.Request{
		Method: "GET",
		Path:   "/functions/v1/greet",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "hi GET", string(resp.Body))
}

func TestServiceInvoke_LogsSuccess(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	deployed, err := svc.Deploy(context.Background(), "logged", `function handler(req) { return { statusCode: 200, body: "ok" }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	_, err = svc.Invoke(context.Background(), "logged", edgefunc.Request{
		Method: "POST",
		Path:   "/functions/v1/logged",
	})
	testutil.NoError(t, err)

	testutil.SliceLen(t, logStore.entries, 1)
	entry := logStore.entries[0]
	testutil.Equal(t, deployed.ID, entry.FunctionID)
	testutil.Equal(t, "success", entry.Status)
	testutil.True(t, entry.DurationMs >= 0, "duration should be non-negative")
	testutil.Equal(t, "POST", entry.RequestMethod)
	testutil.Equal(t, "/functions/v1/logged", entry.RequestPath)
	testutil.True(t, entry.InvocationID != uuid.Nil, "should have invocation ID")
}

func TestServiceInvoke_LogsError(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "fail", `function handler(req) { throw new Error("boom"); }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	_, err = svc.Invoke(context.Background(), "fail", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.True(t, err != nil, "should return error from failed handler")

	testutil.SliceLen(t, logStore.entries, 1)
	entry := logStore.entries[0]
	testutil.Equal(t, "error", entry.Status)
	testutil.True(t, entry.Error != "", "should capture error message")
}

func TestServiceInvoke_LogsStdoutBytesAndResponseStatusCode(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "metrics", `function handler(req) { console.log("abc"); return { statusCode: 201, body: "ok" }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	resp, err := svc.Invoke(context.Background(), "metrics", edgefunc.Request{Method: "GET", Path: "/metrics"})
	testutil.NoError(t, err)
	testutil.Equal(t, 201, resp.StatusCode)

	testutil.SliceLen(t, logStore.entries, 1)
	entry := logStore.entries[0]
	testutil.Equal(t, len(resp.Stdout), entry.StdoutBytes)
	testutil.Equal(t, 201, entry.ResponseStatusCode)
}

func TestServiceInvoke_LogsStdoutOnError(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "stdout-err", `function handler(req) { console.log("before crash"); throw new Error("boom"); }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	_, err = svc.Invoke(context.Background(), "stdout-err", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.True(t, err != nil, "should return error from failed handler")

	testutil.SliceLen(t, logStore.entries, 1)
	entry := logStore.entries[0]
	testutil.Equal(t, "error", entry.Status)
	testutil.True(t, strings.Contains(entry.Stdout, "before crash"), "log should capture stdout even on error, got: %q", entry.Stdout)
}

func TestServiceInvoke_LogsStdoutOnAsyncError(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "stdout-async-err", `function handler(req) { console.log("before async crash"); return Promise.reject(new Error("boom")); }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	_, err = svc.Invoke(context.Background(), "stdout-async-err", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.True(t, err != nil, "should return error from failed handler")

	testutil.SliceLen(t, logStore.entries, 1)
	entry := logStore.entries[0]
	testutil.Equal(t, "error", entry.Status)
	testutil.True(t, strings.Contains(entry.Stdout, "before async crash"), "log should capture stdout even on async error, got: %q", entry.Stdout)
}

func TestServiceInvoke_ForwardsInvocationLogToWriter(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	writer := &mockInvocationLogWriter{}
	svc := edgefunc.NewService(
		store,
		pool,
		logStore,
		edgefunc.WithInvocationLogWriter(writer),
	)

	_, err := svc.Deploy(context.Background(), "writer-log-success", `function handler(req) {
		console.log("hello world");
		return { statusCode: 200, body: "ok" };
	}`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	resp, err := svc.Invoke(context.Background(), "writer-log-success", edgefunc.Request{
		Method: "GET",
		Path:   "/functions/v1/writer-log-success",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, "ok", string(resp.Body))

	testutil.Equal(t, 1, len(writer.calls))
	call := writer.calls[0]
	testutil.Equal(t, "writer-log-success", call.functionName)
	testutil.Equal(t, "success", call.status)
	testutil.Equal(t, "", call.errMsg)
	testutil.True(t, call.durationMs >= 0, "duration should be non-negative")
	_, parseErr := uuid.Parse(call.invocationID)
	testutil.NoError(t, parseErr)
}

func TestServiceInvoke_ForwardsInvocationLogWriterErrorStatusAndErrorMessage(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	writer := &mockInvocationLogWriter{}
	svc := edgefunc.NewService(
		store,
		pool,
		logStore,
		edgefunc.WithInvocationLogWriter(writer),
	)

	_, err := svc.Deploy(context.Background(), "writer-log-error", `function handler(req) { throw new Error("boom"); }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	_, err = svc.Invoke(context.Background(), "writer-log-error", edgefunc.Request{
		Method: "POST",
		Path:   "/functions/v1/writer-log-error",
	})
	testutil.True(t, err != nil, "invocation should fail")

	testutil.Equal(t, 1, len(writer.calls))
	call := writer.calls[0]
	testutil.Equal(t, "writer-log-error", call.functionName)
	testutil.Equal(t, "error", call.status)
	testutil.True(t, strings.Contains(call.errMsg, "Error"), "error message should include invocation error")
	testutil.True(t, call.durationMs >= 0, "duration should be non-negative")
}

func TestServiceInvoke_LogsStdoutOnResponseConversionError(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "stdout-conv-err", `function handler(req) { console.log("before conversion crash"); return 42; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	_, err = svc.Invoke(context.Background(), "stdout-conv-err", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.True(t, err != nil, "should return error from invalid handler response")

	testutil.SliceLen(t, logStore.entries, 1)
	entry := logStore.entries[0]
	testutil.Equal(t, "error", entry.Status)
	testutil.True(t, strings.Contains(entry.Stdout, "before conversion crash"), "log should capture stdout on response conversion error, got: %q", entry.Stdout)
}

func TestServiceInvoke_NotFound(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Invoke(context.Background(), "nonexistent", edgefunc.Request{})
	testutil.True(t, errors.Is(err, edgefunc.ErrFunctionNotFound), "should return not found, got: %v", err)
	// No log entry for not-found
	testutil.SliceLen(t, logStore.entries, 0)
}

func TestServiceInvoke_CapturesStdout(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "stdout-fn", `function handler(req) { console.log("debug info"); return { statusCode: 200, body: "ok" }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	resp, err := svc.Invoke(context.Background(), "stdout-fn", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, err)
	testutil.True(t, strings.Contains(resp.Stdout, "debug info"), "response should contain stdout")

	entry := logStore.entries[0]
	testutil.True(t, strings.Contains(entry.Stdout, "debug info"), "log entry should capture stdout")
}

func TestServiceInvoke_WithEnvVars(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "env-fn", `function handler(req) { return { statusCode: 200, body: ayb.env.get("SECRET") }; }`, edgefunc.DeployOptions{
		EnvVars: map[string]string{"SECRET": "s3cret"},
	})
	testutil.NoError(t, err)

	resp, err := svc.Invoke(context.Background(), "env-fn", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, err)
	testutil.Equal(t, "s3cret", string(resp.Body))
}

func TestServiceInvoke_WithDBBridge(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	qe := &mockQueryExecutor{
		result: edgefunc.QueryResult{
			Rows: []map[string]interface{}{
				{"name": "Ada"},
			},
		},
	}
	svc := edgefunc.NewService(store, pool, logStore, edgefunc.WithServiceQueryExecutor(qe))

	_, err := svc.Deploy(context.Background(), "db-fn", `function handler(req) { const rows = ayb.db.from("users").select("name").eq("id", 7).execute(); return { statusCode: 200, body: rows[0].name }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	resp, err := svc.Invoke(context.Background(), "db-fn", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, err)
	testutil.Equal(t, "Ada", string(resp.Body))
	testutil.Equal(t, "users", qe.lastQuery.Table)
	testutil.Equal(t, "select", qe.lastQuery.Action)
	testutil.Equal(t, "name", qe.lastQuery.Columns)
	testutil.SliceLen(t, qe.lastQuery.Filters, 1)
	testutil.Equal(t, "id", qe.lastQuery.Filters[0].Column)
	testutil.Equal(t, "eq", qe.lastQuery.Filters[0].Op)
	filterValue, ok := qe.lastQuery.Filters[0].Value.(int64)
	testutil.True(t, ok, "expected filter value to be int64, got %T", qe.lastQuery.Filters[0].Value)
	testutil.Equal(t, int64(7), filterValue)
}

func TestServiceGet(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	deployed, err := svc.Deploy(context.Background(), "get-fn", `function handler(req) { return { statusCode: 200 }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	fn, err := svc.Get(context.Background(), deployed.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, "get-fn", fn.Name)
}

func TestServiceGetByName(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "by-name-fn", `function handler(req) { return { statusCode: 200 }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	fn, err := svc.GetByName(context.Background(), "by-name-fn")
	testutil.NoError(t, err)
	testutil.Equal(t, "by-name-fn", fn.Name)
}

func TestServiceList(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	svc.Deploy(context.Background(), "list-a", `function handler(req) { return { statusCode: 200 }; }`, edgefunc.DeployOptions{})
	svc.Deploy(context.Background(), "list-b", `function handler(req) { return { statusCode: 200 }; }`, edgefunc.DeployOptions{})

	fns, err := svc.List(context.Background(), 0, 0)
	testutil.NoError(t, err)
	testutil.True(t, len(fns) >= 2, "should list at least 2 functions, got %d", len(fns))
}

func TestServiceList_PopulatesLastInvokedAt(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	deployed, err := svc.Deploy(context.Background(), "list-last-invoked", `function handler(req) { return { statusCode: 200 }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	invokedAt := time.Date(2026, 2, 24, 1, 2, 3, 0, time.UTC)
	logStore.entries = append(logStore.entries, &edgefunc.LogEntry{
		ID:         uuid.New(),
		FunctionID: deployed.ID,
		Status:     "success",
		DurationMs: 12,
		CreatedAt:  invokedAt,
	})

	fns, err := svc.List(context.Background(), 1, 10)
	testutil.NoError(t, err)

	var got *edgefunc.EdgeFunction
	for _, fn := range fns {
		if fn.ID == deployed.ID {
			got = fn
			break
		}
	}
	testutil.True(t, got != nil, "expected deployed function in list")
	testutil.True(t, got.LastInvokedAt != nil, "expected lastInvokedAt to be populated")
	testutil.Equal(t, invokedAt, *got.LastInvokedAt)
}

func TestServiceUpdate(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	deployed, err := svc.Deploy(context.Background(), "update-fn", `function handler(req) { return { statusCode: 200, body: "v1" }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	updated, err := svc.Update(context.Background(), deployed.ID, `function handler(req) { return { statusCode: 200, body: "v2" }; }`, edgefunc.DeployOptions{
		EntryPoint: "handler",
		Public:     true,
	})
	testutil.NoError(t, err)
	testutil.True(t, updated.Public, "should be public after update")

	// Invoke should use updated code
	resp, err := svc.Invoke(context.Background(), "update-fn", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, err)
	testutil.Equal(t, "v2", string(resp.Body))
}

func TestServiceUpdate_CompileError(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	deployed, err := svc.Deploy(context.Background(), "update-compile-err", `function handler(req) { return { statusCode: 200 }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	// Update with broken JS should be rejected at compile time.
	_, err = svc.Update(context.Background(), deployed.ID, `function handler( { broken syntax`, edgefunc.DeployOptions{})
	testutil.True(t, err != nil, "should fail on compile error during update")

	// Original source should be unchanged.
	fn, err := svc.Get(context.Background(), deployed.ID)
	testutil.NoError(t, err)
	testutil.True(t, strings.Contains(fn.Source, "statusCode: 200"), "source should be unchanged after failed update")
}

func TestServiceUpdate_TranspileFallbackReportsTranspileError(t *testing.T) {
	t.Parallel()
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	deployed, err := svc.Deploy(context.Background(), "update-fallback-bad", `function handler(req) { return { statusCode: 200, body: "ok" }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	// Invalid as JS and invalid as TS. Update fallback should return transpile error.
	badSource := `function handler(req) { return <></>; }`
	_, err = svc.Update(context.Background(), deployed.ID, badSource, edgefunc.DeployOptions{})
	testutil.True(t, err != nil, "should fail on invalid source during update")
	testutil.Contains(t, err.Error(), "transpile failed")

	current, getErr := svc.Get(context.Background(), deployed.ID)
	testutil.NoError(t, getErr)
	testutil.Equal(t, `function handler(req) { return { statusCode: 200, body: "ok" }; }`, current.Source)
}

func TestServiceDelete(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	deployed, err := svc.Deploy(context.Background(), "delete-fn", `function handler(req) { return { statusCode: 200 }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	err = svc.Delete(context.Background(), deployed.ID)
	testutil.NoError(t, err)

	_, err = svc.Get(context.Background(), deployed.ID)
	testutil.True(t, errors.Is(err, edgefunc.ErrFunctionNotFound), "should be not found after delete")
}

func TestServiceListLogs(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	deployed, err := svc.Deploy(context.Background(), "list-logs-fn", `function handler(req) { return { statusCode: 200, body: "ok" }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	_, err = svc.Invoke(context.Background(), "list-logs-fn", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, err)

	logs, err := svc.ListLogs(context.Background(), deployed.ID, edgefunc.LogListOptions{Page: 1, PerPage: 20})
	testutil.NoError(t, err)
	testutil.SliceLen(t, logs, 1)
	testutil.Equal(t, deployed.ID, logs[0].FunctionID)
	testutil.Equal(t, "success", logs[0].Status)
}

func TestServiceDeploy_TypeScript(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	tsSource := `function handler(request: { path: string }): { statusCode: number; body: string } {
		return { statusCode: 200, body: "hello from ts: " + request.path };
	}`

	fn, err := svc.Deploy(context.Background(), "ts-fn", tsSource, edgefunc.DeployOptions{
		EntryPoint:   "handler",
		IsTypeScript: true,
	})
	testutil.NoError(t, err)
	testutil.Equal(t, "ts-fn", fn.Name)
	testutil.Equal(t, tsSource, fn.Source)
	testutil.True(t, !strings.Contains(fn.CompiledJS, ": string"), "CompiledJS should not contain TS type annotations")
	testutil.True(t, !strings.Contains(fn.CompiledJS, ": number"), "CompiledJS should not contain TS type annotations")
	testutil.True(t, fn.CompiledJS != "", "CompiledJS should be non-empty")
}

func TestServiceDeploy_TypeScript_Invocable(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	tsSource := `function handler(request: { path: string }): { statusCode: number; body: string } {
		return { statusCode: 200, body: "ts:" + request.path };
	}`

	_, err := svc.Deploy(context.Background(), "ts-invoke", tsSource, edgefunc.DeployOptions{
		IsTypeScript: true,
	})
	testutil.NoError(t, err)

	resp, err := svc.Invoke(context.Background(), "ts-invoke", edgefunc.Request{
		Method: "GET",
		Path:   "/hello",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "ts:/hello", string(resp.Body))
}

func TestServiceDeploy_TypeScript_CompileError(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "ts-bad", `function handler(req: string) { return req.`, edgefunc.DeployOptions{
		IsTypeScript: true,
	})
	testutil.True(t, err != nil, "should fail on TS syntax error")
	testutil.True(t, strings.Contains(err.Error(), "transpile"), "error should mention transpilation")
}

func TestServiceDeploy_TypeScript_AutoDetectWhenFlagOmitted(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	tsSource := `function handler(req: { path: string }): { statusCode: number; body: string } {
		return { statusCode: 200, body: "auto-ts:" + req.path };
	}`

	// Intentionally omit IsTypeScript to simulate admin/dashboard callers.
	fn, err := svc.Deploy(context.Background(), "ts-auto-deploy", tsSource, edgefunc.DeployOptions{})
	testutil.NoError(t, err)
	testutil.Equal(t, tsSource, fn.Source)
	testutil.True(t, !strings.Contains(fn.CompiledJS, ": string"), "CompiledJS should not contain TS annotations")

	resp, err := svc.Invoke(context.Background(), "ts-auto-deploy", edgefunc.Request{Method: "GET", Path: "/auto"})
	testutil.NoError(t, err)
	testutil.Equal(t, "auto-ts:/auto", string(resp.Body))
}

func TestServiceUpdate_TypeScript(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	// Deploy as JS first
	deployed, err := svc.Deploy(context.Background(), "ts-update", `function handler(req) { return { statusCode: 200, body: "js" }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	// Update with TS source
	tsSource := `function handler(req: { path: string }): { statusCode: number; body: string } {
		return { statusCode: 200, body: "ts-updated:" + req.path };
	}`
	updated, err := svc.Update(context.Background(), deployed.ID, tsSource, edgefunc.DeployOptions{
		IsTypeScript: true,
	})
	testutil.NoError(t, err)
	testutil.Equal(t, tsSource, updated.Source)
	testutil.True(t, !strings.Contains(updated.CompiledJS, ": string"), "CompiledJS should not contain TS annotations after update")

	// Invoke should use transpiled code
	resp, err := svc.Invoke(context.Background(), "ts-update", edgefunc.Request{Method: "GET", Path: "/up"})
	testutil.NoError(t, err)
	testutil.Equal(t, "ts-updated:/up", string(resp.Body))
}

func TestServiceUpdate_TypeScript_AutoDetectWhenFlagOmitted(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	deployed, err := svc.Deploy(context.Background(), "ts-auto-update", `function handler(req) { return { statusCode: 200, body: "js" }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	tsSource := `function handler(req: { path: string }): { statusCode: number; body: string } {
		return { statusCode: 200, body: "auto-updated:" + req.path };
	}`

	// Intentionally omit IsTypeScript to simulate admin/dashboard callers.
	updated, err := svc.Update(context.Background(), deployed.ID, tsSource, edgefunc.DeployOptions{})
	testutil.NoError(t, err)
	testutil.Equal(t, tsSource, updated.Source)
	testutil.True(t, !strings.Contains(updated.CompiledJS, ": string"), "CompiledJS should not contain TS annotations after auto-detected TS update")

	resp, err := svc.Invoke(context.Background(), "ts-auto-update", edgefunc.Request{Method: "GET", Path: "/updated"})
	testutil.NoError(t, err)
	testutil.Equal(t, "auto-updated:/updated", string(resp.Body))
}

func TestServiceListLogs_NotFound(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	logs, err := svc.ListLogs(context.Background(), uuid.New(), edgefunc.LogListOptions{Page: 1, PerPage: 10})
	testutil.True(t, errors.Is(err, edgefunc.ErrFunctionNotFound), "should return not found, got: %v", err)
	testutil.SliceLen(t, logs, 0)
}

func TestServiceInvokeByID(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	deployed, err := svc.Deploy(context.Background(), "invoke-by-id", `function handler(req) { return { statusCode: 200, body: "id:" + req.method }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	resp, err := svc.InvokeByID(context.Background(), deployed.ID.String(), edgefunc.Request{
		Method: "POST",
		Path:   "/cron",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "id:POST", string(resp.Body))

	// Should log the invocation
	testutil.SliceLen(t, logStore.entries, 1)
	testutil.Equal(t, "success", logStore.entries[0].Status)
}

func TestServiceInvokeByID_NotFound(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.InvokeByID(context.Background(), uuid.New().String(), edgefunc.Request{})
	testutil.True(t, errors.Is(err, edgefunc.ErrFunctionNotFound), "should return not found, got: %v", err)
}

func TestServiceInvokeByID_InvalidUUID(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.InvokeByID(context.Background(), "not-a-uuid", edgefunc.Request{})
	testutil.True(t, err != nil, "should return error for invalid UUID")
	testutil.True(t, strings.Contains(err.Error(), "invalid function ID"), "error should mention invalid ID, got: %v", err)
}

func TestServiceInvoke_LogsTriggerMetadata_DBTrigger(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	deployed, err := svc.Deploy(context.Background(), "db-triggered", `function handler(req) { return { statusCode: 200, body: "ok" }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	triggerID := uuid.New().String()
	ctx := edgefunc.WithTriggerMeta(context.Background(), edgefunc.TriggerDB, triggerID)

	_, err = svc.InvokeByID(ctx, deployed.ID.String(), edgefunc.Request{Method: "POST", Path: "/db-event"})
	testutil.NoError(t, err)

	testutil.SliceLen(t, logStore.entries, 1)
	entry := logStore.entries[0]
	testutil.Equal(t, string(edgefunc.TriggerDB), entry.TriggerType)
	testutil.Equal(t, triggerID, entry.TriggerID)
}

func TestServiceInvoke_LogsTriggerMetadata_CronTrigger(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	deployed, err := svc.Deploy(context.Background(), "cron-triggered", `function handler(req) { return { statusCode: 200, body: "ok" }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	triggerID := uuid.New().String()
	ctx := edgefunc.WithTriggerMeta(context.Background(), edgefunc.TriggerCron, triggerID)

	_, err = svc.InvokeByID(ctx, deployed.ID.String(), edgefunc.Request{Method: "POST", Path: "/cron"})
	testutil.NoError(t, err)

	testutil.SliceLen(t, logStore.entries, 1)
	entry := logStore.entries[0]
	testutil.Equal(t, string(edgefunc.TriggerCron), entry.TriggerType)
	testutil.Equal(t, triggerID, entry.TriggerID)
}

func TestServiceInvoke_LogsTriggerMetadata_StorageTrigger(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	deployed, err := svc.Deploy(context.Background(), "storage-triggered", `function handler(req) { return { statusCode: 200, body: "ok" }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	triggerID := uuid.New().String()
	ctx := edgefunc.WithTriggerMeta(context.Background(), edgefunc.TriggerStorage, triggerID)

	_, err = svc.InvokeByID(ctx, deployed.ID.String(), edgefunc.Request{Method: "POST", Path: "/storage"})
	testutil.NoError(t, err)

	testutil.SliceLen(t, logStore.entries, 1)
	entry := logStore.entries[0]
	testutil.Equal(t, string(edgefunc.TriggerStorage), entry.TriggerType)
	testutil.Equal(t, triggerID, entry.TriggerID)
}

func TestServiceInvoke_LogsTriggerMetadata_ParentInvocationID(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(4)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	// Deploy helper + caller
	_, err := svc.Deploy(context.Background(), "child-fn", `function handler(req) { return { statusCode: 200, body: "child" }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	_, err = svc.Deploy(context.Background(), "parent-fn", `function handler(req) {
		var result = ayb.functions.invoke("child-fn", { method: "GET", path: "/nested" });
		return { statusCode: 200, body: "parent" };
	}`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	_, err = svc.Invoke(context.Background(), "parent-fn", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, err)

	// Should have 2 log entries: child first (inner invocation completes first), then parent
	testutil.SliceLen(t, logStore.entries, 2)

	childEntry := logStore.entries[0]
	parentEntry := logStore.entries[1]
	testutil.Equal(t, "", parentEntry.ParentInvocationID)
	testutil.Equal(t, parentEntry.InvocationID.String(), childEntry.ParentInvocationID)
}

func TestServiceInvoke_LogsTriggerMetadata_HTTPDefault(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "http-fn", `function handler(req) { return { statusCode: 200, body: "ok" }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	// No trigger context = plain HTTP invocation, no trigger metadata
	_, err = svc.Invoke(context.Background(), "http-fn", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, err)

	testutil.SliceLen(t, logStore.entries, 1)
	entry := logStore.entries[0]
	testutil.Equal(t, "", entry.TriggerType)
	testutil.Equal(t, "", entry.TriggerID)
	testutil.Equal(t, "", entry.ParentInvocationID)
}

// --- Failing LogStore for Bug 1 ---

type failingLogStore struct {
	writeErr error
}

func (f *failingLogStore) WriteLog(_ context.Context, _ *edgefunc.LogEntry) error {
	return f.writeErr
}

func (f *failingLogStore) ListByFunction(_ context.Context, _ uuid.UUID, _ edgefunc.LogListOptions) ([]*edgefunc.LogEntry, error) {
	return []*edgefunc.LogEntry{}, nil
}

func TestServiceDeploy_TranspileFallbackReportsTranspileError(t *testing.T) {
	t.Parallel()
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	// This source is invalid JS (goja will fail) and also invalid TS (esbuild will fail).
	// The fallback path in prepareCompiledJS tries transpilation; when that fails too,
	// the error should reference the transpile failure, not the original JS compile error.
	badSource := `function handler(req) { return <></>; }`

	_, err := svc.Deploy(context.Background(), "bad-fallback", badSource, edgefunc.DeployOptions{
		// IsTypeScript: false — triggers the fallback path
	})
	testutil.True(t, err != nil, "should fail on bad source")
	testutil.Contains(t, err.Error(), "transpile failed")
}

func TestServiceInvoke_LogWriteFailureIsLogged(t *testing.T) {
	// Not parallel: this test swaps slog.SetDefault (global state).
	store := newMockStore()
	logStore := &failingLogStore{writeErr: errors.New("disk full")}
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "log-fail", `function handler(req) { return { statusCode: 200, body: "ok" }; }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	// Capture slog output to verify error is logged.
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldLogger)

	resp, err := svc.Invoke(context.Background(), "log-fail", edgefunc.Request{
		Method: "GET",
		Path:   "/functions/v1/log-fail",
	})
	// The invocation itself should succeed — only the log write fails.
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)

	// The log write error must appear in slog output.
	logged := buf.String()
	testutil.Contains(t, logged, "disk full")
}

func TestServiceInvoke_ExecutionErrorWithLogWriteFailurePreservesExecutionError(t *testing.T) {
	// Not parallel: this test swaps slog.SetDefault (global state).
	store := newMockStore()
	logStore := &failingLogStore{writeErr: errors.New("disk full")}
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "exec-and-log-fail", `function handler(req) { throw new Error("boom"); }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldLogger)

	_, err = svc.Invoke(context.Background(), "exec-and-log-fail", edgefunc.Request{
		Method: "GET",
		Path:   "/functions/v1/exec-and-log-fail",
	})
	testutil.True(t, err != nil, "execution should fail")
	testutil.Contains(t, err.Error(), "boom")

	logged := buf.String()
	testutil.Contains(t, logged, "disk full")
}

// --- Mock Vault Provider ---

type mockVaultProvider struct {
	secrets map[string]string
	err     error
}

func (m *mockVaultProvider) GetAllSecretsDecrypted(_ context.Context) (map[string]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.secrets, nil
}

func TestServiceInvoke_VaultSecretsAvailable(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	vp := &mockVaultProvider{
		secrets: map[string]string{
			"VAULT_SECRET": "vault-value-42",
		},
	}

	svc := edgefunc.NewService(store, pool, logStore, edgefunc.WithVaultProvider(vp))

	_, err := svc.Deploy(context.Background(), "vault-fn", `function handler(req) {
		return { statusCode: 200, body: ayb.env.get("VAULT_SECRET") };
	}`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	resp, err := svc.Invoke(context.Background(), "vault-fn", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, err)
	testutil.Equal(t, "vault-value-42", string(resp.Body))
}

func TestServiceInvoke_FunctionEnvVarsTakePrecedenceOverVaultSecrets(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	vp := &mockVaultProvider{
		secrets: map[string]string{
			"SHARED_KEY": "from-vault",
			"VAULT_ONLY": "vault-only-value",
		},
	}

	svc := edgefunc.NewService(store, pool, logStore, edgefunc.WithVaultProvider(vp))

	_, err := svc.Deploy(context.Background(), "precedence-fn", `function handler(req) {
		var shared = ayb.env.get("SHARED_KEY");
		var vaultOnly = ayb.env.get("VAULT_ONLY");
		return { statusCode: 200, body: shared + "|" + vaultOnly };
	}`, edgefunc.DeployOptions{
		EnvVars: map[string]string{"SHARED_KEY": "from-function"},
	})
	testutil.NoError(t, err)

	resp, err := svc.Invoke(context.Background(), "precedence-fn", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, err)
	// Function env var wins over vault secret for SHARED_KEY
	testutil.Equal(t, "from-function|vault-only-value", string(resp.Body))
}

func TestServiceInvoke_VaultProviderErrorDoesNotBlockInvocation(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	vp := &mockVaultProvider{
		err: errors.New("vault unavailable"),
	}

	svc := edgefunc.NewService(store, pool, logStore, edgefunc.WithVaultProvider(vp))

	_, err := svc.Deploy(context.Background(), "vault-err-fn", `function handler(req) {
		return { statusCode: 200, body: "ok" };
	}`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	// Should still execute; vault errors are logged but don't fail invocation
	resp, err := svc.Invoke(context.Background(), "vault-err-fn", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, err)
	testutil.Equal(t, "ok", string(resp.Body))
}

func TestServiceInvoke_NoVaultProviderStillWorks(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	// No vault provider set — should work as before
	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "no-vault-fn", `function handler(req) {
		return { statusCode: 200, body: ayb.env.get("KEY") || "none" };
	}`, edgefunc.DeployOptions{
		EnvVars: map[string]string{"KEY": "fn-val"},
	})
	testutil.NoError(t, err)

	resp, err := svc.Invoke(context.Background(), "no-vault-fn", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, err)
	testutil.Equal(t, "fn-val", string(resp.Body))
}
