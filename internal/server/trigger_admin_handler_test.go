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
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

// --- Fake trigger admin implementations ---

type fakeDBTriggerAdmin struct {
	triggers map[string]*edgefunc.DBTrigger
	createFn func(ctx context.Context, input edgefunc.CreateDBTriggerInput) (*edgefunc.DBTrigger, error)
}

func newFakeDBTriggerAdmin() *fakeDBTriggerAdmin {
	return &fakeDBTriggerAdmin{triggers: make(map[string]*edgefunc.DBTrigger)}
}

func (f *fakeDBTriggerAdmin) Create(ctx context.Context, input edgefunc.CreateDBTriggerInput) (*edgefunc.DBTrigger, error) {
	if f.createFn != nil {
		return f.createFn(ctx, input)
	}
	t := &edgefunc.DBTrigger{
		ID:            "dbt-001",
		FunctionID:    input.FunctionID,
		TableName:     input.TableName,
		Schema:        input.Schema,
		Events:        input.Events,
		FilterColumns: input.FilterColumns,
		Enabled:       true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if t.Schema == "" {
		t.Schema = "public"
	}
	f.triggers[t.ID] = t
	return t, nil
}

func (f *fakeDBTriggerAdmin) Get(_ context.Context, id string) (*edgefunc.DBTrigger, error) {
	t, ok := f.triggers[id]
	if !ok {
		return nil, edgefunc.ErrDBTriggerNotFound
	}
	return t, nil
}

func (f *fakeDBTriggerAdmin) List(_ context.Context, functionID string) ([]*edgefunc.DBTrigger, error) {
	var result []*edgefunc.DBTrigger
	for _, t := range f.triggers {
		if t.FunctionID == functionID {
			result = append(result, t)
		}
	}
	return result, nil
}

func (f *fakeDBTriggerAdmin) Delete(_ context.Context, id string) error {
	if _, ok := f.triggers[id]; !ok {
		return edgefunc.ErrDBTriggerNotFound
	}
	delete(f.triggers, id)
	return nil
}

func (f *fakeDBTriggerAdmin) SetEnabled(_ context.Context, id string, enabled bool) (*edgefunc.DBTrigger, error) {
	t, ok := f.triggers[id]
	if !ok {
		return nil, edgefunc.ErrDBTriggerNotFound
	}
	t.Enabled = enabled
	return t, nil
}

type fakeCronTriggerAdmin struct {
	triggers map[string]*edgefunc.CronTrigger
	createFn func(ctx context.Context, input edgefunc.CreateCronTriggerInput) (*edgefunc.CronTrigger, error)
	invoker  *fakeManualRunInvoker
}

func newFakeCronTriggerAdmin() *fakeCronTriggerAdmin {
	return &fakeCronTriggerAdmin{
		triggers: make(map[string]*edgefunc.CronTrigger),
		invoker:  &fakeManualRunInvoker{},
	}
}

type fakeManualRunInvoker struct {
	lastFunctionID string
	lastRequest    edgefunc.Request
	lastTrigger    edgefunc.TriggerMeta
	hasTriggerMeta bool
	resp           edgefunc.Response
	hasResp        bool
	err            error
}

func (f *fakeManualRunInvoker) InvokeByID(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
	f.lastFunctionID = functionID
	f.lastRequest = req
	f.lastTrigger, f.hasTriggerMeta = edgefunc.GetTriggerMeta(ctx)
	if f.err != nil {
		return edgefunc.Response{}, f.err
	}
	if f.hasResp {
		return f.resp, nil
	}
	return edgefunc.Response{StatusCode: 200, Body: []byte("ok")}, nil
}

func (f *fakeCronTriggerAdmin) Create(ctx context.Context, input edgefunc.CreateCronTriggerInput) (*edgefunc.CronTrigger, error) {
	if f.createFn != nil {
		return f.createFn(ctx, input)
	}
	tz := input.Timezone
	if tz == "" {
		tz = "UTC"
	}
	payload := input.Payload
	if payload == nil {
		payload = json.RawMessage(`{}`)
	}
	t := &edgefunc.CronTrigger{
		ID:         "ct-001",
		FunctionID: input.FunctionID,
		ScheduleID: "sched-001",
		CronExpr:   input.CronExpr,
		Timezone:   tz,
		Payload:    payload,
		Enabled:    true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	f.triggers[t.ID] = t
	return t, nil
}

func (f *fakeCronTriggerAdmin) Get(_ context.Context, id string) (*edgefunc.CronTrigger, error) {
	t, ok := f.triggers[id]
	if !ok {
		return nil, edgefunc.ErrCronTriggerNotFound
	}
	return t, nil
}

func (f *fakeCronTriggerAdmin) List(_ context.Context, functionID string) ([]*edgefunc.CronTrigger, error) {
	var result []*edgefunc.CronTrigger
	for _, t := range f.triggers {
		if t.FunctionID == functionID {
			result = append(result, t)
		}
	}
	return result, nil
}

func (f *fakeCronTriggerAdmin) Delete(_ context.Context, id string) error {
	if _, ok := f.triggers[id]; !ok {
		return edgefunc.ErrCronTriggerNotFound
	}
	delete(f.triggers, id)
	return nil
}

func (f *fakeCronTriggerAdmin) SetEnabled(_ context.Context, id string, enabled bool) (*edgefunc.CronTrigger, error) {
	t, ok := f.triggers[id]
	if !ok {
		return nil, edgefunc.ErrCronTriggerNotFound
	}
	t.Enabled = enabled
	return t, nil
}

type fakeStorageTriggerAdmin struct {
	triggers map[string]*edgefunc.StorageTrigger
	createFn func(ctx context.Context, input edgefunc.CreateStorageTriggerInput) (*edgefunc.StorageTrigger, error)
}

func newFakeStorageTriggerAdmin() *fakeStorageTriggerAdmin {
	return &fakeStorageTriggerAdmin{triggers: make(map[string]*edgefunc.StorageTrigger)}
}

func (f *fakeStorageTriggerAdmin) Create(ctx context.Context, input edgefunc.CreateStorageTriggerInput) (*edgefunc.StorageTrigger, error) {
	if f.createFn != nil {
		return f.createFn(ctx, input)
	}
	t := &edgefunc.StorageTrigger{
		ID:           "st-001",
		FunctionID:   input.FunctionID,
		Bucket:       input.Bucket,
		EventTypes:   input.EventTypes,
		PrefixFilter: input.PrefixFilter,
		SuffixFilter: input.SuffixFilter,
		Enabled:      true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	f.triggers[t.ID] = t
	return t, nil
}

func (f *fakeStorageTriggerAdmin) Get(_ context.Context, id string) (*edgefunc.StorageTrigger, error) {
	t, ok := f.triggers[id]
	if !ok {
		return nil, edgefunc.ErrStorageTriggerNotFound
	}
	return t, nil
}

func (f *fakeStorageTriggerAdmin) List(_ context.Context, functionID string) ([]*edgefunc.StorageTrigger, error) {
	var result []*edgefunc.StorageTrigger
	for _, t := range f.triggers {
		if t.FunctionID == functionID {
			result = append(result, t)
		}
	}
	return result, nil
}

func (f *fakeStorageTriggerAdmin) Delete(_ context.Context, id string) error {
	if _, ok := f.triggers[id]; !ok {
		return edgefunc.ErrStorageTriggerNotFound
	}
	delete(f.triggers, id)
	return nil
}

func (f *fakeStorageTriggerAdmin) SetEnabled(_ context.Context, id string, enabled bool) (*edgefunc.StorageTrigger, error) {
	t, ok := f.triggers[id]
	if !ok {
		return nil, edgefunc.ErrStorageTriggerNotFound
	}
	t.Enabled = enabled
	return t, nil
}

// --- Test router helpers ---

func triggerAdminRouter(db dbTriggerAdmin, cron cronTriggerAdmin, cronInvoker edgefunc.FunctionInvoker, st storageTriggerAdmin) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api/admin/functions/{id}/triggers", func(r chi.Router) {
		r.Route("/db", func(r chi.Router) {
			r.Get("/", handleListDBTriggers(db))
			r.Post("/", handleCreateDBTrigger(db))
			r.Get("/{triggerId}", handleGetDBTrigger(db))
			r.Delete("/{triggerId}", handleDeleteDBTrigger(db))
			r.Post("/{triggerId}/enable", handleEnableDBTrigger(db))
			r.Post("/{triggerId}/disable", handleDisableDBTrigger(db))
		})
		r.Route("/cron", func(r chi.Router) {
			r.Get("/", handleListCronTriggers(cron))
			r.Post("/", handleCreateCronTrigger(cron))
			r.Get("/{triggerId}", handleGetCronTrigger(cron))
			r.Delete("/{triggerId}", handleDeleteCronTrigger(cron))
			r.Post("/{triggerId}/enable", handleEnableCronTrigger(cron))
			r.Post("/{triggerId}/disable", handleDisableCronTrigger(cron))
			r.Post("/{triggerId}/run", handleManualRunCronTrigger(cron, cronInvoker))
		})
		r.Route("/storage", func(r chi.Router) {
			r.Get("/", handleListStorageTriggers(st))
			r.Post("/", handleCreateStorageTrigger(st))
			r.Get("/{triggerId}", handleGetStorageTrigger(st))
			r.Delete("/{triggerId}", handleDeleteStorageTrigger(st))
			r.Post("/{triggerId}/enable", handleEnableStorageTrigger(st))
			r.Post("/{triggerId}/disable", handleDisableStorageTrigger(st))
		})
	})
	return r
}

const testFunctionID = "550e8400-e29b-41d4-a716-446655440000"

// ========== DB Trigger Tests ==========

func TestCreateDBTrigger(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	r := triggerAdminRouter(db, nil, nil, nil)

	body := `{"table_name":"users","events":["INSERT","UPDATE"]}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/db", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusCreated, w.Code)
	var trigger edgefunc.DBTrigger
	decodeJSON(t, w, &trigger)
	testutil.Equal(t, "users", trigger.TableName)
	testutil.Equal(t, "public", trigger.Schema)
	testutil.Equal(t, testFunctionID, trigger.FunctionID)
	testutil.SliceLen(t, trigger.Events, 2)
}

func TestCreateDBTrigger_MissingTableName(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	r := triggerAdminRouter(db, nil, nil, nil)

	body := `{"events":["INSERT"]}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/db", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCreateDBTrigger_MissingEvents(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	r := triggerAdminRouter(db, nil, nil, nil)

	body := `{"table_name":"users"}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/db", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCreateDBTrigger_InvalidEvent(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	db.createFn = func(_ context.Context, _ edgefunc.CreateDBTriggerInput) (*edgefunc.DBTrigger, error) {
		return nil, edgefunc.ErrInvalidDBEvent
	}
	r := triggerAdminRouter(db, nil, nil, nil)

	body := `{"table_name":"users","events":["TRUNCATE"]}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/db", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCreateDBTrigger_InvalidIdentifier(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	db.createFn = func(_ context.Context, _ edgefunc.CreateDBTriggerInput) (*edgefunc.DBTrigger, error) {
		return nil, edgefunc.ErrInvalidIdentifier
	}
	r := triggerAdminRouter(db, nil, nil, nil)

	body := `{"table_name":"bad-table","events":["INSERT"]}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/db", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCreateDBTrigger_Duplicate(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	db.createFn = func(_ context.Context, _ edgefunc.CreateDBTriggerInput) (*edgefunc.DBTrigger, error) {
		return nil, edgefunc.ErrDBTriggerDuplicate
	}
	r := triggerAdminRouter(db, nil, nil, nil)

	body := `{"table_name":"users","events":["INSERT"]}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/db", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusConflict, w.Code)
}

func TestCreateDBTrigger_WithSchemaAndFilterColumns(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	r := triggerAdminRouter(db, nil, nil, nil)

	body := `{"table_name":"orders","schema":"sales","events":["UPDATE"],"filter_columns":["status","total"]}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/db", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusCreated, w.Code)
	var trigger edgefunc.DBTrigger
	decodeJSON(t, w, &trigger)
	testutil.Equal(t, "sales", trigger.Schema)
	testutil.SliceLen(t, trigger.FilterColumns, 2)
}

func TestListDBTriggers(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	db.triggers["t1"] = &edgefunc.DBTrigger{ID: "t1", FunctionID: testFunctionID, TableName: "users", Events: []edgefunc.DBTriggerEvent{"INSERT"}}
	db.triggers["t2"] = &edgefunc.DBTrigger{ID: "t2", FunctionID: testFunctionID, TableName: "orders", Events: []edgefunc.DBTriggerEvent{"DELETE"}}
	r := triggerAdminRouter(db, nil, nil, nil)

	req := httptest.NewRequest("GET", "/api/admin/functions/"+testFunctionID+"/triggers/db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var triggers []edgefunc.DBTrigger
	decodeJSON(t, w, &triggers)
	testutil.Equal(t, 2, len(triggers))
}

func TestListDBTriggers_Empty(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	r := triggerAdminRouter(db, nil, nil, nil)

	req := httptest.NewRequest("GET", "/api/admin/functions/"+testFunctionID+"/triggers/db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Equal(t, "[]\n", w.Body.String())
}

func TestGetDBTrigger(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	db.triggers["t1"] = &edgefunc.DBTrigger{ID: "t1", FunctionID: testFunctionID, TableName: "users"}
	r := triggerAdminRouter(db, nil, nil, nil)

	req := httptest.NewRequest("GET", "/api/admin/functions/"+testFunctionID+"/triggers/db/t1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var trigger edgefunc.DBTrigger
	decodeJSON(t, w, &trigger)
	testutil.Equal(t, "t1", trigger.ID)
}

func TestGetDBTrigger_NotFound(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	r := triggerAdminRouter(db, nil, nil, nil)

	req := httptest.NewRequest("GET", "/api/admin/functions/"+testFunctionID+"/triggers/db/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestDeleteDBTrigger(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	db.triggers["t1"] = &edgefunc.DBTrigger{ID: "t1", FunctionID: testFunctionID}
	r := triggerAdminRouter(db, nil, nil, nil)

	req := httptest.NewRequest("DELETE", "/api/admin/functions/"+testFunctionID+"/triggers/db/t1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, 0, len(db.triggers))
}

func TestDeleteDBTrigger_NotFound(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	r := triggerAdminRouter(db, nil, nil, nil)

	req := httptest.NewRequest("DELETE", "/api/admin/functions/"+testFunctionID+"/triggers/db/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestEnableDBTrigger(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	db.triggers["t1"] = &edgefunc.DBTrigger{ID: "t1", FunctionID: testFunctionID, Enabled: false}
	r := triggerAdminRouter(db, nil, nil, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/db/t1/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var trigger edgefunc.DBTrigger
	decodeJSON(t, w, &trigger)
	testutil.True(t, trigger.Enabled)
}

func TestDisableDBTrigger(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	db.triggers["t1"] = &edgefunc.DBTrigger{ID: "t1", FunctionID: testFunctionID, Enabled: true}
	r := triggerAdminRouter(db, nil, nil, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/db/t1/disable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var trigger edgefunc.DBTrigger
	decodeJSON(t, w, &trigger)
	testutil.False(t, trigger.Enabled)
}

func TestEnableDBTrigger_NotFound(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	r := triggerAdminRouter(db, nil, nil, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/db/nope/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

// ========== Cron Trigger Tests ==========

func TestCreateCronTrigger(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	body := `{"cron_expr":"*/5 * * * *","timezone":"America/New_York","payload":{"key":"value"}}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusCreated, w.Code)
	var trigger edgefunc.CronTrigger
	decodeJSON(t, w, &trigger)
	testutil.Equal(t, "*/5 * * * *", trigger.CronExpr)
	testutil.Equal(t, "America/New_York", trigger.Timezone)
	testutil.Equal(t, testFunctionID, trigger.FunctionID)
}

func TestCreateCronTrigger_MissingCronExpr(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	body := `{"timezone":"UTC"}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCreateCronTrigger_InvalidCronExpr(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.createFn = func(_ context.Context, _ edgefunc.CreateCronTriggerInput) (*edgefunc.CronTrigger, error) {
		return nil, edgefunc.ErrInvalidCronExpr
	}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	body := `{"cron_expr":"not-a-cron"}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCreateCronTrigger_InvalidTimezone(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.createFn = func(_ context.Context, _ edgefunc.CreateCronTriggerInput) (*edgefunc.CronTrigger, error) {
		return nil, edgefunc.ErrInvalidTimezone
	}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	body := `{"cron_expr":"* * * * *","timezone":"Mars/Olympus"}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestListCronTriggers(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{ID: "c1", FunctionID: testFunctionID, CronExpr: "0 * * * *"}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("GET", "/api/admin/functions/"+testFunctionID+"/triggers/cron", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var triggers []edgefunc.CronTrigger
	decodeJSON(t, w, &triggers)
	testutil.Equal(t, 1, len(triggers))
}

func TestListCronTriggers_Empty(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("GET", "/api/admin/functions/"+testFunctionID+"/triggers/cron", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Equal(t, "[]\n", w.Body.String())
}

func TestGetCronTrigger(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{ID: "c1", FunctionID: testFunctionID, CronExpr: "0 * * * *"}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("GET", "/api/admin/functions/"+testFunctionID+"/triggers/cron/c1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var trigger edgefunc.CronTrigger
	decodeJSON(t, w, &trigger)
	testutil.Equal(t, "c1", trigger.ID)
}

func TestGetCronTrigger_NotFound(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("GET", "/api/admin/functions/"+testFunctionID+"/triggers/cron/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestDeleteCronTrigger(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{ID: "c1", FunctionID: testFunctionID}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("DELETE", "/api/admin/functions/"+testFunctionID+"/triggers/cron/c1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, 0, len(cron.triggers))
}

func TestDeleteCronTrigger_NotFound(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("DELETE", "/api/admin/functions/"+testFunctionID+"/triggers/cron/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestEnableCronTrigger(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{ID: "c1", FunctionID: testFunctionID, Enabled: false}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron/c1/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var trigger edgefunc.CronTrigger
	decodeJSON(t, w, &trigger)
	testutil.True(t, trigger.Enabled)
}

func TestDisableCronTrigger(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{ID: "c1", FunctionID: testFunctionID, Enabled: true}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron/c1/disable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var trigger edgefunc.CronTrigger
	decodeJSON(t, w, &trigger)
	testutil.False(t, trigger.Enabled)
}

func TestManualRunCronTrigger(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{
		ID:         "c1",
		FunctionID: testFunctionID,
		Payload:    json.RawMessage(`{"test":true}`),
		Enabled:    true,
	}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron/c1/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Equal(t, testFunctionID, cron.invoker.lastFunctionID)
}

func TestManualRunCronTrigger_ForwardsPayloadAndResponse(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{
		ID:         "c1",
		FunctionID: testFunctionID,
		Payload:    json.RawMessage(`{"job":"run","attempt":2}`),
		Enabled:    true,
	}
	cron.invoker.resp = edgefunc.Response{
		StatusCode: http.StatusAccepted,
		Body:       []byte(`{"accepted":true}`),
	}
	cron.invoker.hasResp = true
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron/c1/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Equal(t, `{"job":"run","attempt":2}`, string(cron.invoker.lastRequest.Body))
	var body struct {
		StatusCode int    `json:"statusCode"`
		Body       string `json:"body"`
	}
	decodeJSON(t, w, &body)
	testutil.Equal(t, http.StatusAccepted, body.StatusCode)
	testutil.Equal(t, `{"accepted":true}`, body.Body)
}

func TestManualRunCronTrigger_SetsTriggerMetadata(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{
		ID:         "c1",
		FunctionID: testFunctionID,
		Payload:    json.RawMessage(`{"job":"run"}`),
		Enabled:    true,
	}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron/c1/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Equal(t, "POST", cron.invoker.lastRequest.Method)
	testutil.Equal(t, "/cron", cron.invoker.lastRequest.Path)
	testutil.True(t, cron.invoker.hasTriggerMeta, "manual run should include trigger metadata")
	testutil.Equal(t, edgefunc.TriggerCron, cron.invoker.lastTrigger.Type)
	testutil.Equal(t, "c1", cron.invoker.lastTrigger.ID)
}

func TestManualRunCronTrigger_NotFound(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron/nope/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestManualRunCronTrigger_Disabled(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{
		ID:         "c1",
		FunctionID: testFunctionID,
		Payload:    json.RawMessage(`{"job":"run"}`),
		Enabled:    false,
	}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron/c1/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusConflict, w.Code)
	testutil.Contains(t, w.Body.String(), "cron trigger is disabled")
	testutil.Equal(t, "", cron.invoker.lastFunctionID)
	testutil.False(t, cron.invoker.hasTriggerMeta)
}

func TestManualRunCronTrigger_InvokeError(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{
		ID:         "c1",
		FunctionID: testFunctionID,
		Payload:    json.RawMessage(`{}`),
		Enabled:    true,
	}
	cron.invoker.err = errors.New("function crashed")
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron/c1/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusInternalServerError, w.Code)
}

// ========== Storage Trigger Tests ==========

func TestCreateStorageTrigger(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	r := triggerAdminRouter(nil, nil, nil, st)

	body := `{"bucket":"avatars","event_types":["upload","delete"],"prefix_filter":"images/","suffix_filter":".png"}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/storage", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusCreated, w.Code)
	var trigger edgefunc.StorageTrigger
	decodeJSON(t, w, &trigger)
	testutil.Equal(t, "avatars", trigger.Bucket)
	testutil.Equal(t, testFunctionID, trigger.FunctionID)
	testutil.SliceLen(t, trigger.EventTypes, 2)
	testutil.Equal(t, "images/", trigger.PrefixFilter)
	testutil.Equal(t, ".png", trigger.SuffixFilter)
}

func TestCreateStorageTrigger_MissingBucket(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	r := triggerAdminRouter(nil, nil, nil, st)

	body := `{"event_types":["upload"]}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/storage", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCreateStorageTrigger_MissingEventTypes(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	r := triggerAdminRouter(nil, nil, nil, st)

	body := `{"bucket":"avatars"}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/storage", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCreateStorageTrigger_InvalidEventType(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	st.createFn = func(_ context.Context, _ edgefunc.CreateStorageTriggerInput) (*edgefunc.StorageTrigger, error) {
		return nil, edgefunc.ErrInvalidEventType
	}
	r := triggerAdminRouter(nil, nil, nil, st)

	body := `{"bucket":"b","event_types":["copy"]}`
	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/storage", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestListStorageTriggers(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	st.triggers["s1"] = &edgefunc.StorageTrigger{ID: "s1", FunctionID: testFunctionID, Bucket: "avatars"}
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("GET", "/api/admin/functions/"+testFunctionID+"/triggers/storage", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var triggers []edgefunc.StorageTrigger
	decodeJSON(t, w, &triggers)
	testutil.Equal(t, 1, len(triggers))
}

func TestListStorageTriggers_Empty(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("GET", "/api/admin/functions/"+testFunctionID+"/triggers/storage", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Equal(t, "[]\n", w.Body.String())
}

func TestGetStorageTrigger(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	st.triggers["s1"] = &edgefunc.StorageTrigger{ID: "s1", FunctionID: testFunctionID, Bucket: "avatars"}
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("GET", "/api/admin/functions/"+testFunctionID+"/triggers/storage/s1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var trigger edgefunc.StorageTrigger
	decodeJSON(t, w, &trigger)
	testutil.Equal(t, "s1", trigger.ID)
}

func TestGetStorageTrigger_NotFound(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("GET", "/api/admin/functions/"+testFunctionID+"/triggers/storage/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestDeleteStorageTrigger(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	st.triggers["s1"] = &edgefunc.StorageTrigger{ID: "s1", FunctionID: testFunctionID}
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("DELETE", "/api/admin/functions/"+testFunctionID+"/triggers/storage/s1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, 0, len(st.triggers))
}

func TestDeleteStorageTrigger_NotFound(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("DELETE", "/api/admin/functions/"+testFunctionID+"/triggers/storage/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestEnableStorageTrigger(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	st.triggers["s1"] = &edgefunc.StorageTrigger{ID: "s1", FunctionID: testFunctionID, Enabled: false}
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/storage/s1/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var trigger edgefunc.StorageTrigger
	decodeJSON(t, w, &trigger)
	testutil.True(t, trigger.Enabled)
}

func TestDisableStorageTrigger(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	st.triggers["s1"] = &edgefunc.StorageTrigger{ID: "s1", FunctionID: testFunctionID, Enabled: true}
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/storage/s1/disable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var trigger edgefunc.StorageTrigger
	decodeJSON(t, w, &trigger)
	testutil.False(t, trigger.Enabled)
}

func TestEnableStorageTrigger_NotFound(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/storage/nope/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

// ========== Invalid JSON tests ==========

func TestCreateDBTrigger_InvalidJSON(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	r := triggerAdminRouter(db, nil, nil, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/db", strings.NewReader("{broken"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCreateCronTrigger_InvalidJSON(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron", strings.NewReader("{broken"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCreateStorageTrigger_InvalidJSON(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/storage", strings.NewReader("{broken"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

// ========== Cross-function isolation tests ==========
// Triggers must only be accessible under their owning function's URL path.

const otherFunctionID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func TestGetDBTrigger_WrongFunction(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	db.triggers["t1"] = &edgefunc.DBTrigger{ID: "t1", FunctionID: testFunctionID, TableName: "users"}
	r := triggerAdminRouter(db, nil, nil, nil)

	// Access trigger t1 via a different function's URL — should 404.
	req := httptest.NewRequest("GET", "/api/admin/functions/"+otherFunctionID+"/triggers/db/t1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestDeleteDBTrigger_WrongFunction(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	db.triggers["t1"] = &edgefunc.DBTrigger{ID: "t1", FunctionID: testFunctionID, TableName: "users"}
	r := triggerAdminRouter(db, nil, nil, nil)

	req := httptest.NewRequest("DELETE", "/api/admin/functions/"+otherFunctionID+"/triggers/db/t1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	// Trigger must not have been deleted.
	testutil.Equal(t, 1, len(db.triggers))
}

func TestEnableDBTrigger_WrongFunction(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	db.triggers["t1"] = &edgefunc.DBTrigger{ID: "t1", FunctionID: testFunctionID, Enabled: false}
	r := triggerAdminRouter(db, nil, nil, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+otherFunctionID+"/triggers/db/t1/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.False(t, db.triggers["t1"].Enabled)
}

func TestGetCronTrigger_WrongFunction(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{ID: "c1", FunctionID: testFunctionID, CronExpr: "0 * * * *"}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("GET", "/api/admin/functions/"+otherFunctionID+"/triggers/cron/c1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestDeleteCronTrigger_WrongFunction(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{ID: "c1", FunctionID: testFunctionID}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("DELETE", "/api/admin/functions/"+otherFunctionID+"/triggers/cron/c1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.Equal(t, 1, len(cron.triggers))
}

func TestManualRunCronTrigger_WrongFunction(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{
		ID:         "c1",
		FunctionID: testFunctionID,
		Payload:    json.RawMessage(`{}`),
	}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+otherFunctionID+"/triggers/cron/c1/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.Equal(t, "", cron.invoker.lastFunctionID)
}

func TestGetStorageTrigger_WrongFunction(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	st.triggers["s1"] = &edgefunc.StorageTrigger{ID: "s1", FunctionID: testFunctionID, Bucket: "avatars"}
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("GET", "/api/admin/functions/"+otherFunctionID+"/triggers/storage/s1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestDeleteStorageTrigger_WrongFunction(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	st.triggers["s1"] = &edgefunc.StorageTrigger{ID: "s1", FunctionID: testFunctionID}
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("DELETE", "/api/admin/functions/"+otherFunctionID+"/triggers/storage/s1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.Equal(t, 1, len(st.triggers))
}

func TestEnableStorageTrigger_WrongFunction(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	st.triggers["s1"] = &edgefunc.StorageTrigger{ID: "s1", FunctionID: testFunctionID, Enabled: false}
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+otherFunctionID+"/triggers/storage/s1/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.False(t, st.triggers["s1"].Enabled)
}

// ========== Disable handler not-found + wrong-function tests ==========

func TestDisableDBTrigger_NotFound(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	r := triggerAdminRouter(db, nil, nil, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/db/nope/disable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestDisableDBTrigger_WrongFunction(t *testing.T) {
	t.Parallel()
	db := newFakeDBTriggerAdmin()
	db.triggers["t1"] = &edgefunc.DBTrigger{ID: "t1", FunctionID: testFunctionID, Enabled: true}
	r := triggerAdminRouter(db, nil, nil, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+otherFunctionID+"/triggers/db/t1/disable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.True(t, db.triggers["t1"].Enabled)
}

func TestEnableCronTrigger_NotFound(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron/nope/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestEnableCronTrigger_WrongFunction(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{ID: "c1", FunctionID: testFunctionID, Enabled: false}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+otherFunctionID+"/triggers/cron/c1/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.False(t, cron.triggers["c1"].Enabled)
}

func TestDisableCronTrigger_NotFound(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/cron/nope/disable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestDisableCronTrigger_WrongFunction(t *testing.T) {
	t.Parallel()
	cron := newFakeCronTriggerAdmin()
	cron.triggers["c1"] = &edgefunc.CronTrigger{ID: "c1", FunctionID: testFunctionID, Enabled: true}
	r := triggerAdminRouter(nil, cron, cron.invoker, nil)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+otherFunctionID+"/triggers/cron/c1/disable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.True(t, cron.triggers["c1"].Enabled)
}

func TestDisableStorageTrigger_NotFound(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+testFunctionID+"/triggers/storage/nope/disable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestDisableStorageTrigger_WrongFunction(t *testing.T) {
	t.Parallel()
	st := newFakeStorageTriggerAdmin()
	st.triggers["s1"] = &edgefunc.StorageTrigger{ID: "s1", FunctionID: testFunctionID, Enabled: true}
	r := triggerAdminRouter(nil, nil, nil, st)

	req := httptest.NewRequest("POST", "/api/admin/functions/"+otherFunctionID+"/triggers/storage/s1/disable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.True(t, st.triggers["s1"].Enabled)
}
