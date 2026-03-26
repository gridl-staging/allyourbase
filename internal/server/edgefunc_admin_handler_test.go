package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// --- Fake edgeFuncAdmin ---

type fakeEdgeFuncAdmin struct {
	functions              map[uuid.UUID]*edgefunc.EdgeFunction
	byName                 map[string]uuid.UUID
	logs                   map[uuid.UUID][]*edgefunc.LogEntry
	deployErr              error
	invokeErr              error
	listLogsErr            error
	listLogsOverrideSet    bool
	listLogsOverrideResult []*edgefunc.LogEntry
	lastListLogsFunctionID uuid.UUID
	lastListLogsOpts       edgefunc.LogListOptions
	lastInvokeReq          *edgefunc.Request // captures the last request passed to Invoke
}

func newFakeEdgeFuncAdmin() *fakeEdgeFuncAdmin {
	return &fakeEdgeFuncAdmin{
		functions: make(map[uuid.UUID]*edgefunc.EdgeFunction),
		byName:    make(map[string]uuid.UUID),
		logs:      make(map[uuid.UUID][]*edgefunc.LogEntry),
	}
}

func (f *fakeEdgeFuncAdmin) GetByName(_ context.Context, name string) (*edgefunc.EdgeFunction, error) {
	id, ok := f.byName[name]
	if !ok {
		return nil, edgefunc.ErrFunctionNotFound
	}
	return f.functions[id], nil
}

func (f *fakeEdgeFuncAdmin) Deploy(_ context.Context, name, source string, opts edgefunc.DeployOptions) (*edgefunc.EdgeFunction, error) {
	if f.deployErr != nil {
		return nil, f.deployErr
	}
	if _, exists := f.byName[name]; exists {
		return nil, edgefunc.ErrFunctionNameConflict
	}
	id := uuid.New()
	entryPoint := opts.EntryPoint
	if entryPoint == "" {
		entryPoint = "handler"
	}
	fn := &edgefunc.EdgeFunction{
		ID:         id,
		Name:       name,
		EntryPoint: entryPoint,
		Source:     source,
		CompiledJS: source,
		Timeout:    time.Duration(opts.TimeoutMs) * time.Millisecond,
		EnvVars:    opts.EnvVars,
		Public:     opts.Public,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	f.functions[id] = fn
	f.byName[name] = id
	return fn, nil
}

func (f *fakeEdgeFuncAdmin) Get(_ context.Context, id uuid.UUID) (*edgefunc.EdgeFunction, error) {
	fn, ok := f.functions[id]
	if !ok {
		return nil, edgefunc.ErrFunctionNotFound
	}
	return fn, nil
}

func (f *fakeEdgeFuncAdmin) List(_ context.Context, page, perPage int) ([]*edgefunc.EdgeFunction, error) {
	result := make([]*edgefunc.EdgeFunction, 0, len(f.functions))
	for _, fn := range f.functions {
		result = append(result, fn)
	}
	return result, nil
}

func (f *fakeEdgeFuncAdmin) Update(_ context.Context, id uuid.UUID, source string, opts edgefunc.DeployOptions) (*edgefunc.EdgeFunction, error) {
	fn, ok := f.functions[id]
	if !ok {
		return nil, edgefunc.ErrFunctionNotFound
	}
	fn.Source = source
	fn.CompiledJS = source
	fn.Public = opts.Public
	if opts.EntryPoint != "" {
		fn.EntryPoint = opts.EntryPoint
	}
	if opts.TimeoutMs > 0 {
		fn.Timeout = time.Duration(opts.TimeoutMs) * time.Millisecond
	}
	if opts.EnvVars != nil {
		fn.EnvVars = opts.EnvVars
	}
	fn.UpdatedAt = time.Now()
	return fn, nil
}

func (f *fakeEdgeFuncAdmin) Delete(_ context.Context, id uuid.UUID) error {
	fn, ok := f.functions[id]
	if !ok {
		return edgefunc.ErrFunctionNotFound
	}
	delete(f.byName, fn.Name)
	delete(f.functions, id)
	delete(f.logs, id)
	return nil
}

func (f *fakeEdgeFuncAdmin) Invoke(_ context.Context, name string, req edgefunc.Request) (edgefunc.Response, error) {
	f.lastInvokeReq = &req
	id, ok := f.byName[name]
	if !ok {
		return edgefunc.Response{}, edgefunc.ErrFunctionNotFound
	}
	if f.invokeErr != nil {
		return edgefunc.Response{}, f.invokeErr
	}
	// Seed a log entry for the invocation.
	entry := &edgefunc.LogEntry{
		ID:            uuid.New(),
		FunctionID:    id,
		InvocationID:  uuid.New(),
		Status:        "success",
		DurationMs:    42,
		RequestMethod: req.Method,
		RequestPath:   req.Path,
		CreatedAt:     time.Now(),
	}
	f.logs[id] = append(f.logs[id], entry)
	return edgefunc.Response{StatusCode: 200, Body: []byte("ok")}, nil
}

func (f *fakeEdgeFuncAdmin) ListLogs(_ context.Context, functionID uuid.UUID, opts edgefunc.LogListOptions) ([]*edgefunc.LogEntry, error) {
	f.lastListLogsFunctionID = functionID
	f.lastListLogsOpts = opts

	if f.listLogsErr != nil {
		return nil, f.listLogsErr
	}
	if f.listLogsOverrideSet {
		return f.listLogsOverrideResult, nil
	}
	if _, ok := f.functions[functionID]; !ok {
		return nil, edgefunc.ErrFunctionNotFound
	}

	entries, ok := f.logs[functionID]
	if !ok {
		return []*edgefunc.LogEntry{}, nil
	}
	return entries, nil
}

// seedFunction adds a function to the fake and returns its ID.
func seedFunction(f *fakeEdgeFuncAdmin, name, source string, public bool) uuid.UUID {
	id := uuid.New()
	fn := &edgefunc.EdgeFunction{
		ID:         id,
		Name:       name,
		EntryPoint: "handler",
		Source:     source,
		CompiledJS: source,
		Public:     public,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	f.functions[id] = fn
	f.byName[name] = id
	return id
}

// --- Test helpers ---

func adminEdgeFuncRouter(admin edgeFuncAdmin) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api/admin/functions", func(r chi.Router) {
		r.Get("/", handleAdminListFunctions(admin))
		r.Post("/", handleAdminDeployFunction(admin))
		r.Get("/by-name/{name}", handleAdminGetFunctionByName(admin))
		r.Get("/{id}", handleAdminGetFunction(admin))
		r.Put("/{id}", handleAdminUpdateFunction(admin))
		r.Delete("/{id}", handleAdminDeleteFunction(admin))
		r.Get("/{id}/logs", handleAdminFunctionLogs(admin))
		r.Post("/{id}/invoke", handleAdminInvokeFunction(admin))
	})
	return r
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	err := json.NewDecoder(w.Body).Decode(v)
	testutil.NoError(t, err)
}

// --- Admin List ---

func TestAdminListFunctions(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	seedFunction(admin, "fn1", "code1", true)
	seedFunction(admin, "fn2", "code2", false)

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var body []json.RawMessage
	decodeJSON(t, w, &body)
	testutil.Equal(t, 2, len(body))
}

func TestAdminListFunctions_Empty(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var body []json.RawMessage
	decodeJSON(t, w, &body)
	testutil.Equal(t, 0, len(body))
}

func TestAdminListFunctions_IncludesLastInvokedAt(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "fn1", "code1", true)
	lastInvokedAt := time.Date(2026, 2, 24, 4, 5, 6, 0, time.UTC)
	admin.functions[id].LastInvokedAt = &lastInvokedAt

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var body []edgefunc.EdgeFunction
	decodeJSON(t, w, &body)
	testutil.SliceLen(t, body, 1)
	testutil.True(t, body[0].LastInvokedAt != nil, "expected lastInvokedAt in response")
	testutil.Equal(t, lastInvokedAt, *body[0].LastInvokedAt)
}

// --- Admin Deploy ---

func TestAdminDeployFunction(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()

	r := adminEdgeFuncRouter(admin)
	body := `{"name":"hello","source":"function handler(req) { return new Response('ok'); }","public":true}`
	req := httptest.NewRequest("POST", "/api/admin/functions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusCreated, w.Code)
	var fn edgefunc.EdgeFunction
	decodeJSON(t, w, &fn)
	testutil.Equal(t, "hello", fn.Name)
	testutil.Equal(t, "handler", fn.EntryPoint)
	testutil.True(t, fn.Public)
}

func TestAdminDeployFunction_MissingName(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()

	r := adminEdgeFuncRouter(admin)
	body := `{"source":"code"}`
	req := httptest.NewRequest("POST", "/api/admin/functions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestAdminDeployFunction_MissingSource(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()

	r := adminEdgeFuncRouter(admin)
	body := `{"name":"hello"}`
	req := httptest.NewRequest("POST", "/api/admin/functions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestAdminDeployFunction_DuplicateName(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	seedFunction(admin, "hello", "code", true)

	r := adminEdgeFuncRouter(admin)
	body := `{"name":"hello","source":"code"}`
	req := httptest.NewRequest("POST", "/api/admin/functions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusConflict, w.Code)
}

func TestAdminDeployFunction_WithOptions(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()

	r := adminEdgeFuncRouter(admin)
	body := `{"name":"fn","source":"code","entry_point":"main","timeout_ms":3000,"env_vars":{"KEY":"val"},"public":false}`
	req := httptest.NewRequest("POST", "/api/admin/functions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusCreated, w.Code)
	var fn edgefunc.EdgeFunction
	decodeJSON(t, w, &fn)
	testutil.Equal(t, "main", fn.EntryPoint)
	testutil.False(t, fn.Public)
}

// --- Admin Get ---

func TestAdminGetFunction(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "source_code", true)

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var fn edgefunc.EdgeFunction
	decodeJSON(t, w, &fn)
	testutil.Equal(t, "hello", fn.Name)
	testutil.Equal(t, "source_code", fn.Source)
}

func TestAdminGetFunction_NotFound(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestAdminGetFunction_InvalidID(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid function id format")
}

func TestAdminGetFunctionByName(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	seedFunction(admin, "hello-by-name", "source_code", true)

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/by-name/hello-by-name", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var fn edgefunc.EdgeFunction
	decodeJSON(t, w, &fn)
	testutil.Equal(t, "hello-by-name", fn.Name)
	testutil.Equal(t, "source_code", fn.Source)
}

func TestAdminGetFunctionByName_NotFound(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/by-name/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

// --- Admin Update ---

func TestAdminUpdateFunction(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "old_code", false)

	r := adminEdgeFuncRouter(admin)
	body := `{"source":"new_code","public":true}`
	req := httptest.NewRequest("PUT", "/api/admin/functions/"+id.String(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var fn edgefunc.EdgeFunction
	decodeJSON(t, w, &fn)
	testutil.Equal(t, "new_code", fn.Source)
	testutil.True(t, fn.Public)
}

func TestAdminUpdateFunction_NotFound(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()

	r := adminEdgeFuncRouter(admin)
	body := `{"source":"code"}`
	req := httptest.NewRequest("PUT", "/api/admin/functions/"+uuid.New().String(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

// --- Admin Delete ---

func TestAdminDeleteFunction(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("DELETE", "/api/admin/functions/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNoContent, w.Code)
	// Verify deleted
	testutil.Equal(t, 0, len(admin.functions))
}

func TestAdminDeleteFunction_NotFound(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("DELETE", "/api/admin/functions/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

// --- Admin Logs ---

func TestAdminFunctionLogs(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)
	// Seed some logs.
	admin.logs[id] = []*edgefunc.LogEntry{
		{
			ID:                 uuid.New(),
			FunctionID:         id,
			InvocationID:       uuid.New(),
			Status:             "success",
			DurationMs:         10,
			TriggerType:        string(edgefunc.TriggerDB),
			TriggerID:          uuid.NewString(),
			ParentInvocationID: uuid.NewString(),
			CreatedAt:          time.Now(),
		},
		{ID: uuid.New(), FunctionID: id, InvocationID: uuid.New(), Status: "error", DurationMs: 50, Error: "boom", CreatedAt: time.Now()},
	}

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+id.String()+"/logs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var logs []edgefunc.LogEntry
	decodeJSON(t, w, &logs)
	testutil.Equal(t, 2, len(logs))
	testutil.Equal(t, string(edgefunc.TriggerDB), logs[0].TriggerType)
	testutil.True(t, logs[0].TriggerID != "", "triggerId should be serialized")
	testutil.True(t, logs[0].ParentInvocationID != "", "parentInvocationId should be serialized")
}

func TestAdminFunctionLogs_InvalidID(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/bad-id/logs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestAdminFunctionLogs_NotFound(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+uuid.New().String()+"/logs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestAdminFunctionLogs_ServiceError(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)
	admin.listLogsErr = errors.New("db unavailable")

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+id.String()+"/logs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusInternalServerError, w.Code)
}

func TestAdminFunctionLogs_NilLogsReturnsEmptyArray(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)
	admin.listLogsOverrideSet = true
	admin.listLogsOverrideResult = nil

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+id.String()+"/logs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Equal(t, "[]\n", w.Body.String())
}

func TestAdminFunctionLogs_ForwardsPagination(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+id.String()+"/logs?page=2&perPage=25", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Equal(t, id, admin.lastListLogsFunctionID)
	testutil.Equal(t, 2, admin.lastListLogsOpts.Page)
	testutil.Equal(t, 25, admin.lastListLogsOpts.PerPage)
}

func TestAdminFunctionLogs_ForwardsLimitAsPerPageWithCap(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+id.String()+"/logs?limit=1200", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Equal(t, id, admin.lastListLogsFunctionID)
	testutil.Equal(t, 1, admin.lastListLogsOpts.Page)
	testutil.Equal(t, 1000, admin.lastListLogsOpts.PerPage)
}

func TestAdminFunctionLogs_ForwardsFilters(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest(
		"GET",
		"/api/admin/functions/"+id.String()+"/logs?status=error&trigger_type=cron&since=2026-02-24T08:00:00Z&until=2026-02-24T09:00:00Z",
		nil,
	)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Equal(t, "error", admin.lastListLogsOpts.Status)
	testutil.Equal(t, "cron", admin.lastListLogsOpts.TriggerType)
	testutil.True(t, admin.lastListLogsOpts.Since != nil, "since should be forwarded")
	testutil.True(t, admin.lastListLogsOpts.Until != nil, "until should be forwarded")
}

func TestAdminFunctionLogs_InvalidStatusFilter(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+id.String()+"/logs?status=maybe", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestAdminFunctionLogs_InvalidTriggerTypeFilter(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+id.String()+"/logs?trigger_type=queue", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestAdminFunctionLogs_InvalidSinceFilter(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+id.String()+"/logs?since=not-a-time", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestAdminFunctionLogs_InvalidSinceUntilRange(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+id.String()+"/logs?since=2026-02-24T10:00:00Z&until=2026-02-24T09:00:00Z", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestAdminFunctionLogs_InvalidNegativeLimit(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+id.String()+"/logs?limit=-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestAdminFunctionLogs_NonNumericLimit(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+id.String()+"/logs?limit=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestAdminFunctionLogs_NonNumericPage(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("GET", "/api/admin/functions/"+id.String()+"/logs?page=xyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

// --- Admin Invoke ---

func TestAdminInvokeFunction(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)

	r := adminEdgeFuncRouter(admin)
	body := `{"method":"POST","path":"/test","body":"data"}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+id.String()+"/invoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	sc, _ := resp["statusCode"].(float64)
	testutil.Equal(t, float64(200), sc)

	// Verify request translation: method, path, body were forwarded.
	testutil.True(t, admin.lastInvokeReq != nil, "expected Invoke to be called")
	testutil.Equal(t, "POST", admin.lastInvokeReq.Method)
	testutil.Equal(t, "/test", admin.lastInvokeReq.Path)
	testutil.Equal(t, "data", string(admin.lastInvokeReq.Body))
}

func TestAdminInvokeFunction_NotFound(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()

	r := adminEdgeFuncRouter(admin)
	// Use a valid UUID for an ID that doesn't exist.
	body := `{}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+uuid.New().String()+"/invoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestAdminDeployFunction_InvalidJSON(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()

	r := adminEdgeFuncRouter(admin)
	req := httptest.NewRequest("POST", "/api/admin/functions", strings.NewReader("{broken"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestAdminUpdateFunction_MissingSource(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "hello", "code", true)

	r := adminEdgeFuncRouter(admin)
	body := `{"public":true}`
	req := httptest.NewRequest("PUT", "/api/admin/functions/"+id.String(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestAdminInvokeFunction_DefaultMethodAndPath(t *testing.T) {
	t.Parallel()
	admin := newFakeEdgeFuncAdmin()
	id := seedFunction(admin, "greet", "code", true)

	r := adminEdgeFuncRouter(admin)
	// Empty body — handler should default method to GET and path to /greet
	body := `{}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+id.String()+"/invoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	sc, _ := resp["statusCode"].(float64)
	testutil.Equal(t, float64(200), sc)

	// Verify the handler applied defaults: method→GET, path→/greet
	testutil.True(t, admin.lastInvokeReq != nil, "expected Invoke to be called")
	testutil.Equal(t, "GET", admin.lastInvokeReq.Method)
	testutil.Equal(t, "/greet", admin.lastInvokeReq.Path)
}

// Silence unused import warning.
var _ = httputil.WriteError
