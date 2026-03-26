package edgefunc_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
)

// --- Mock CronTriggerStore ---

type mockCronTriggerStore struct {
	triggers  map[string]*edgefunc.CronTrigger
	byFunc    map[string][]string // functionID -> trigger IDs
	deleteErr error
}

func newMockCronTriggerStore() *mockCronTriggerStore {
	return &mockCronTriggerStore{
		triggers: make(map[string]*edgefunc.CronTrigger),
		byFunc:   make(map[string][]string),
	}
}

func (m *mockCronTriggerStore) CreateCronTrigger(_ context.Context, t *edgefunc.CronTrigger) (*edgefunc.CronTrigger, error) {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	t.CreatedAt = time.Now()
	t.UpdatedAt = t.CreatedAt
	m.triggers[t.ID] = t
	m.byFunc[t.FunctionID] = append(m.byFunc[t.FunctionID], t.ID)
	return t, nil
}

func (m *mockCronTriggerStore) GetCronTrigger(_ context.Context, id string) (*edgefunc.CronTrigger, error) {
	t, ok := m.triggers[id]
	if !ok {
		return nil, edgefunc.ErrCronTriggerNotFound
	}
	return t, nil
}

func (m *mockCronTriggerStore) ListCronTriggers(_ context.Context, functionID string) ([]*edgefunc.CronTrigger, error) {
	var result []*edgefunc.CronTrigger
	ids := m.byFunc[functionID]
	for _, id := range ids {
		if t, ok := m.triggers[id]; ok {
			result = append(result, t)
		}
	}
	if result == nil {
		result = []*edgefunc.CronTrigger{}
	}
	return result, nil
}

func (m *mockCronTriggerStore) UpdateCronTrigger(_ context.Context, t *edgefunc.CronTrigger) (*edgefunc.CronTrigger, error) {
	if _, ok := m.triggers[t.ID]; !ok {
		return nil, edgefunc.ErrCronTriggerNotFound
	}
	t.UpdatedAt = time.Now()
	m.triggers[t.ID] = t
	return t, nil
}

func (m *mockCronTriggerStore) DeleteCronTrigger(_ context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	t, ok := m.triggers[id]
	if !ok {
		return edgefunc.ErrCronTriggerNotFound
	}
	delete(m.triggers, id)
	// Remove from byFunc
	ids := m.byFunc[t.FunctionID]
	for i, tid := range ids {
		if tid == id {
			m.byFunc[t.FunctionID] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	return nil
}

// --- Mock JobsService ---

type mockJobsService struct {
	schedules      map[string]*mockSchedule
	handlers       map[string]func(context.Context, json.RawMessage) error
	enabledToggles []enabledToggle
	deleteErr      error
}

type mockSchedule struct {
	ID          string
	Name        string
	JobType     string
	Payload     json.RawMessage
	CronExpr    string
	Timezone    string
	Enabled     bool
	MaxAttempts int
	NextRunAt   *time.Time
}

type enabledToggle struct {
	ID      string
	Enabled bool
}

func newMockJobsService() *mockJobsService {
	return &mockJobsService{
		schedules: make(map[string]*mockSchedule),
		handlers:  make(map[string]func(context.Context, json.RawMessage) error),
	}
}

func (m *mockJobsService) CreateSchedule(ctx context.Context, name, jobType string, payload json.RawMessage, cronExpr, timezone string, maxAttempts int, nextRunAt *time.Time) (string, error) {
	id := uuid.New().String()
	m.schedules[id] = &mockSchedule{
		ID:          id,
		Name:        name,
		JobType:     jobType,
		Payload:     payload,
		CronExpr:    cronExpr,
		Timezone:    timezone,
		Enabled:     true,
		MaxAttempts: maxAttempts,
		NextRunAt:   nextRunAt,
	}
	return id, nil
}

func (m *mockJobsService) DeleteSchedule(ctx context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, ok := m.schedules[id]; !ok {
		return errors.New("schedule not found")
	}
	delete(m.schedules, id)
	return nil
}

func (m *mockJobsService) SetScheduleEnabled(ctx context.Context, id string, enabled bool) error {
	s, ok := m.schedules[id]
	if !ok {
		return errors.New("schedule not found")
	}
	s.Enabled = enabled
	m.enabledToggles = append(m.enabledToggles, enabledToggle{ID: id, Enabled: enabled})
	return nil
}

// --- Tests ---

func TestCronTriggerService_Create(t *testing.T) {
	store := newMockCronTriggerStore()
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(store, js)

	trigger, err := svc.Create(context.Background(), edgefunc.CreateCronTriggerInput{
		FunctionID: uuid.New().String(),
		CronExpr:   "*/5 * * * *",
		Timezone:   "UTC",
		Payload:    json.RawMessage(`{"key":"value"}`),
	})
	testutil.NoError(t, err)
	testutil.True(t, trigger.ID != "", "should have an ID")
	testutil.Equal(t, "*/5 * * * *", trigger.CronExpr)
	testutil.Equal(t, "UTC", trigger.Timezone)
	testutil.True(t, trigger.ScheduleID != "", "should have a schedule ID")
	testutil.True(t, trigger.Enabled, "should be enabled by default")

	// Verify a schedule was created in the jobs service
	testutil.True(t, len(js.schedules) == 1, "should create one schedule")

	// Verify the schedule payload contains the cron trigger ID so the job handler
	// can attribute invocations back to this trigger.
	for _, sched := range js.schedules {
		var jobPayload edgefunc.CronJobPayload
		testutil.NoError(t, json.Unmarshal(sched.Payload, &jobPayload))
		testutil.Equal(t, trigger.ID, jobPayload.CronTriggerID)
	}
}

func TestCronTriggerService_Create_InvalidCron(t *testing.T) {
	store := newMockCronTriggerStore()
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(store, js)

	_, err := svc.Create(context.Background(), edgefunc.CreateCronTriggerInput{
		FunctionID: uuid.New().String(),
		CronExpr:   "not a cron",
		Timezone:   "UTC",
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrInvalidCronExpr), "should reject invalid cron, got: %v", err)
}

func TestCronTriggerService_Create_InvalidTimezone(t *testing.T) {
	store := newMockCronTriggerStore()
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(store, js)

	_, err := svc.Create(context.Background(), edgefunc.CreateCronTriggerInput{
		FunctionID: uuid.New().String(),
		CronExpr:   "*/5 * * * *",
		Timezone:   "Not/ATimezone",
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrInvalidTimezone), "should reject invalid timezone, got: %v", err)
}

func TestCronTriggerService_Create_MissingFunctionID(t *testing.T) {
	store := newMockCronTriggerStore()
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(store, js)

	_, err := svc.Create(context.Background(), edgefunc.CreateCronTriggerInput{
		CronExpr: "*/5 * * * *",
		Timezone: "UTC",
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrFunctionIDRequired), "should require function ID, got: %v", err)
}

func TestCronTriggerService_Create_MissingCronExpr(t *testing.T) {
	store := newMockCronTriggerStore()
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(store, js)

	_, err := svc.Create(context.Background(), edgefunc.CreateCronTriggerInput{
		FunctionID: uuid.New().String(),
		Timezone:   "UTC",
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrCronExprRequired), "should require cron expr, got: %v", err)
}

func TestCronTriggerService_Create_DefaultTimezone(t *testing.T) {
	store := newMockCronTriggerStore()
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(store, js)

	trigger, err := svc.Create(context.Background(), edgefunc.CreateCronTriggerInput{
		FunctionID: uuid.New().String(),
		CronExpr:   "0 * * * *",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, "UTC", trigger.Timezone)
}

func TestCronTriggerService_Get(t *testing.T) {
	store := newMockCronTriggerStore()
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(store, js)

	created, err := svc.Create(context.Background(), edgefunc.CreateCronTriggerInput{
		FunctionID: uuid.New().String(),
		CronExpr:   "*/5 * * * *",
		Timezone:   "UTC",
	})
	testutil.NoError(t, err)

	got, err := svc.Get(context.Background(), created.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, created.ID, got.ID)
	testutil.Equal(t, created.CronExpr, got.CronExpr)
}

func TestCronTriggerService_Get_NotFound(t *testing.T) {
	store := newMockCronTriggerStore()
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(store, js)

	_, err := svc.Get(context.Background(), "nonexistent-id")
	testutil.True(t, errors.Is(err, edgefunc.ErrCronTriggerNotFound), "should return not found, got: %v", err)
}

func TestCronTriggerService_List(t *testing.T) {
	store := newMockCronTriggerStore()
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(store, js)

	funcID := uuid.New().String()
	svc.Create(context.Background(), edgefunc.CreateCronTriggerInput{FunctionID: funcID, CronExpr: "*/5 * * * *"})
	svc.Create(context.Background(), edgefunc.CreateCronTriggerInput{FunctionID: funcID, CronExpr: "0 * * * *"})

	triggers, err := svc.List(context.Background(), funcID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, triggers, 2)
}

func TestCronTriggerService_List_NilReceiverReturnsServiceNotConfigured(t *testing.T) {
	var svc *edgefunc.CronTriggerService

	_, err := svc.List(context.Background(), uuid.New().String())
	testutil.True(t, errors.Is(err, edgefunc.ErrCronTriggerServiceNotConfigured), "expected service-not-configured error, got: %v", err)
}

func TestCronTriggerService_List_NilStoreReturnsServiceNotConfigured(t *testing.T) {
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(nil, js)

	_, err := svc.List(context.Background(), uuid.New().String())
	testutil.True(t, errors.Is(err, edgefunc.ErrCronTriggerServiceNotConfigured), "expected service-not-configured error, got: %v", err)
}

func TestCronTriggerService_Create_NilSchedulerReturnsServiceNotConfigured(t *testing.T) {
	store := newMockCronTriggerStore()
	svc := edgefunc.NewCronTriggerService(store, nil)

	_, err := svc.Create(context.Background(), edgefunc.CreateCronTriggerInput{
		FunctionID: uuid.New().String(),
		CronExpr:   "0 * * * *",
		Timezone:   "UTC",
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrCronTriggerServiceNotConfigured), "expected service-not-configured error, got: %v", err)
}

func TestCronTriggerService_Delete(t *testing.T) {
	store := newMockCronTriggerStore()
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(store, js)

	created, err := svc.Create(context.Background(), edgefunc.CreateCronTriggerInput{
		FunctionID: uuid.New().String(),
		CronExpr:   "*/5 * * * *",
		Timezone:   "UTC",
	})
	testutil.NoError(t, err)

	err = svc.Delete(context.Background(), created.ID)
	testutil.NoError(t, err)

	// Trigger should be gone
	_, err = svc.Get(context.Background(), created.ID)
	testutil.True(t, errors.Is(err, edgefunc.ErrCronTriggerNotFound), "should be deleted")

	// Schedule should also be deleted
	testutil.True(t, len(js.schedules) == 0, "should delete the underlying schedule")
}

func TestCronTriggerService_Delete_NotFound(t *testing.T) {
	store := newMockCronTriggerStore()
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(store, js)

	err := svc.Delete(context.Background(), "nonexistent-id")
	testutil.True(t, errors.Is(err, edgefunc.ErrCronTriggerNotFound), "should return not found, got: %v", err)
}

func TestCronTriggerService_Delete_SchedulerFailureDoesNotDeleteTrigger(t *testing.T) {
	store := newMockCronTriggerStore()
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(store, js)

	created, err := svc.Create(context.Background(), edgefunc.CreateCronTriggerInput{
		FunctionID: uuid.New().String(),
		CronExpr:   "*/5 * * * *",
		Timezone:   "UTC",
	})
	testutil.NoError(t, err)

	js.deleteErr = errors.New("scheduler unavailable")
	err = svc.Delete(context.Background(), created.ID)
	testutil.True(t, err != nil, "delete should fail when scheduler delete fails")

	// Trigger should still exist so we don't orphan an active schedule.
	_, err = svc.Get(context.Background(), created.ID)
	testutil.NoError(t, err)
	testutil.True(t, len(js.schedules) == 1, "schedule should still exist when deletion fails")
}

func TestCronTriggerService_Delete_IgnoresTriggerNotFoundAfterScheduleDelete(t *testing.T) {
	store := newMockCronTriggerStore()
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(store, js)

	created, err := svc.Create(context.Background(), edgefunc.CreateCronTriggerInput{
		FunctionID: uuid.New().String(),
		CronExpr:   "*/5 * * * *",
		Timezone:   "UTC",
	})
	testutil.NoError(t, err)

	// Simulate schedule-first deletion path where DB cascade already removed trigger row.
	store.deleteErr = edgefunc.ErrCronTriggerNotFound
	err = svc.Delete(context.Background(), created.ID)
	testutil.NoError(t, err)
	testutil.True(t, len(js.schedules) == 0, "schedule should be deleted")
}

func TestCronTriggerService_SetEnabled(t *testing.T) {
	store := newMockCronTriggerStore()
	js := newMockJobsService()
	svc := edgefunc.NewCronTriggerService(store, js)

	created, err := svc.Create(context.Background(), edgefunc.CreateCronTriggerInput{
		FunctionID: uuid.New().String(),
		CronExpr:   "*/5 * * * *",
		Timezone:   "UTC",
	})
	testutil.NoError(t, err)
	testutil.True(t, created.Enabled, "should start enabled")

	// Disable
	updated, err := svc.SetEnabled(context.Background(), created.ID, false)
	testutil.NoError(t, err)
	testutil.False(t, updated.Enabled, "should be disabled")

	// Jobs service should have been told
	testutil.True(t, len(js.enabledToggles) == 1, "should toggle schedule")
	testutil.Equal(t, created.ScheduleID, js.enabledToggles[0].ID)
	testutil.False(t, js.enabledToggles[0].Enabled, "schedule should be disabled")

	// Re-enable
	updated, err = svc.SetEnabled(context.Background(), created.ID, true)
	testutil.NoError(t, err)
	testutil.True(t, updated.Enabled, "should be re-enabled")
}

func TestCronJobHandler(t *testing.T) {
	// Mock the function invoker
	var invocations []invokeCall
	invoker := &mockFunctionInvoker{
		fn: func(ctx context.Context, name string, req edgefunc.Request) (edgefunc.Response, error) {
			invocations = append(invocations, invokeCall{name: name, req: req})
			return edgefunc.Response{StatusCode: 200, Body: []byte("ok")}, nil
		},
	}

	handler := edgefunc.NewCronJobHandler(invoker)

	payload := edgefunc.CronJobPayload{
		FunctionID:    uuid.New().String(),
		CronTriggerID: uuid.New().String(),
		Payload:       json.RawMessage(`{"key":"value"}`),
	}
	payloadBytes, _ := json.Marshal(payload)

	err := handler(context.Background(), payloadBytes)
	testutil.NoError(t, err)
	testutil.SliceLen(t, invocations, 1)
	testutil.Equal(t, "POST", invocations[0].req.Method)
	testutil.Equal(t, "/cron", invocations[0].req.Path)
	testutil.Equal(t, `{"key":"value"}`, string(invocations[0].req.Body))
}

func TestCronJobHandler_FunctionNotFound(t *testing.T) {
	invoker := &mockFunctionInvoker{
		fn: func(ctx context.Context, name string, req edgefunc.Request) (edgefunc.Response, error) {
			return edgefunc.Response{}, edgefunc.ErrFunctionNotFound
		},
	}

	handler := edgefunc.NewCronJobHandler(invoker)

	payload := edgefunc.CronJobPayload{
		FunctionID:    uuid.New().String(),
		CronTriggerID: uuid.New().String(),
	}
	payloadBytes, _ := json.Marshal(payload)

	err := handler(context.Background(), payloadBytes)
	testutil.True(t, errors.Is(err, edgefunc.ErrFunctionNotFound), "should return function-not-found error, got: %v", err)
}

func TestCronJobHandler_InvalidPayload(t *testing.T) {
	invoker := &mockFunctionInvoker{}
	handler := edgefunc.NewCronJobHandler(invoker)

	err := handler(context.Background(), json.RawMessage(`{invalid`))
	testutil.True(t, err != nil, "should return error on invalid payload")
}

// --- Helpers ---

type invokeCall struct {
	name string
	req  edgefunc.Request
}

type mockFunctionInvoker struct {
	fn func(ctx context.Context, name string, req edgefunc.Request) (edgefunc.Response, error)
}

func (m *mockFunctionInvoker) InvokeByID(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
	if m.fn != nil {
		return m.fn(ctx, functionID, req)
	}
	return edgefunc.Response{StatusCode: 200}, nil
}
