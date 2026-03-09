package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

// fakeJobService is an in-memory fake for testing jobs admin handlers.
type fakeJobService struct {
	jobs      []jobs.Job
	schedules []jobs.Schedule
	listErr   error
	getErr    error
	retryErr  error
	cancelErr error
	statsErr  error

	schedCreateErr error
	schedUpdateErr error
	schedDeleteErr error
	schedEnableErr error

	lastUpdateScheduleID string
	lastUpdateEnabled    bool
	lastUpdateNextRunAt  *time.Time
}

func (f *fakeJobService) List(_ context.Context, state, jobType string, limit, offset int) ([]jobs.Job, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var result []jobs.Job
	for _, j := range f.jobs {
		if state != "" && string(j.State) != state {
			continue
		}
		if jobType != "" && j.Type != jobType {
			continue
		}
		result = append(result, j)
	}
	if result == nil {
		result = []jobs.Job{}
	}
	if limit <= 0 {
		limit = 50
	}
	if offset > len(result) {
		return []jobs.Job{}, nil
	}
	end := offset + limit
	if end > len(result) {
		end = len(result)
	}
	return result[offset:end], nil
}

func (f *fakeJobService) Get(_ context.Context, id string) (*jobs.Job, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	for _, j := range f.jobs {
		if j.ID == id {
			return &j, nil
		}
	}
	return nil, fmt.Errorf("job %s not found", id)
}

func (f *fakeJobService) RetryNow(_ context.Context, id string) (*jobs.Job, error) {
	if f.retryErr != nil {
		return nil, f.retryErr
	}
	for i := range f.jobs {
		if f.jobs[i].ID == id && f.jobs[i].State == jobs.StateFailed {
			f.jobs[i].State = jobs.StateQueued
			f.jobs[i].Attempts = 0
			return &f.jobs[i], nil
		}
	}
	return nil, fmt.Errorf("job %s not found or not in failed state", id)
}

func (f *fakeJobService) Cancel(_ context.Context, id string) (*jobs.Job, error) {
	if f.cancelErr != nil {
		return nil, f.cancelErr
	}
	for i := range f.jobs {
		if f.jobs[i].ID == id && f.jobs[i].State == jobs.StateQueued {
			f.jobs[i].State = jobs.StateCanceled
			now := time.Now()
			f.jobs[i].CanceledAt = &now
			return &f.jobs[i], nil
		}
	}
	return nil, fmt.Errorf("job %s not found or not in queued state", id)
}

func (f *fakeJobService) Stats(_ context.Context) (*jobs.QueueStats, error) {
	if f.statsErr != nil {
		return nil, f.statsErr
	}
	stats := &jobs.QueueStats{}
	for _, j := range f.jobs {
		switch j.State {
		case jobs.StateQueued:
			stats.Queued++
		case jobs.StateRunning:
			stats.Running++
		case jobs.StateCompleted:
			stats.Completed++
		case jobs.StateFailed:
			stats.Failed++
		case jobs.StateCanceled:
			stats.Canceled++
		}
	}
	return stats, nil
}

func (f *fakeJobService) ListSchedules(_ context.Context) ([]jobs.Schedule, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.schedules == nil {
		return []jobs.Schedule{}, nil
	}
	return f.schedules, nil
}

func (f *fakeJobService) GetSchedule(_ context.Context, id string) (*jobs.Schedule, error) {
	for _, s := range f.schedules {
		if s.ID == id {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("schedule %s not found", id)
}

func (f *fakeJobService) CreateSchedule(_ context.Context, sched *jobs.Schedule) (*jobs.Schedule, error) {
	if f.schedCreateErr != nil {
		return nil, f.schedCreateErr
	}
	sched.ID = "00000000-0000-0000-0000-000000000099"
	sched.CreatedAt = time.Now()
	sched.UpdatedAt = time.Now()
	f.schedules = append(f.schedules, *sched)
	return sched, nil
}

func (f *fakeJobService) UpdateSchedule(_ context.Context, id, cronExpr, timezone string, payload json.RawMessage, enabled bool, nextRunAt *time.Time) (*jobs.Schedule, error) {
	if f.schedUpdateErr != nil {
		return nil, f.schedUpdateErr
	}
	f.lastUpdateScheduleID = id
	f.lastUpdateEnabled = enabled
	f.lastUpdateNextRunAt = nextRunAt
	for i := range f.schedules {
		if f.schedules[i].ID == id {
			f.schedules[i].CronExpr = cronExpr
			f.schedules[i].Timezone = timezone
			f.schedules[i].Enabled = enabled
			return &f.schedules[i], nil
		}
	}
	return nil, fmt.Errorf("schedule %s not found", id)
}

func (f *fakeJobService) DeleteSchedule(_ context.Context, id string) error {
	if f.schedDeleteErr != nil {
		return f.schedDeleteErr
	}
	for i := range f.schedules {
		if f.schedules[i].ID == id {
			f.schedules = append(f.schedules[:i], f.schedules[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("schedule %s not found", id)
}

func (f *fakeJobService) SetScheduleEnabled(_ context.Context, id string, enabled bool) (*jobs.Schedule, error) {
	if f.schedEnableErr != nil {
		return nil, f.schedEnableErr
	}
	for i := range f.schedules {
		if f.schedules[i].ID == id {
			f.schedules[i].Enabled = enabled
			return &f.schedules[i], nil
		}
	}
	return nil, fmt.Errorf("schedule %s not found", id)
}

func newFakeJobService() *fakeJobService {
	now := time.Now()
	return &fakeJobService{
		jobs: []jobs.Job{
			{
				ID:          "11111111-1111-1111-1111-111111111111",
				Type:        "stale_session_cleanup",
				Payload:     json.RawMessage("{}"),
				State:       jobs.StateCompleted,
				Attempts:    1,
				MaxAttempts: 3,
				CreatedAt:   now,
				UpdatedAt:   now,
				CompletedAt: &now,
			},
			{
				ID:          "22222222-2222-2222-2222-222222222222",
				Type:        "webhook_delivery_prune",
				Payload:     json.RawMessage(`{"retention_hours":168}`),
				State:       jobs.StateQueued,
				Attempts:    0,
				MaxAttempts: 3,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			{
				ID:          "33333333-3333-3333-3333-333333333333",
				Type:        "stale_session_cleanup",
				Payload:     json.RawMessage("{}"),
				State:       jobs.StateFailed,
				Attempts:    3,
				MaxAttempts: 3,
				LastError:   jobStrPtr("connection refused"),
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
		schedules: []jobs.Schedule{
			{
				ID:          "aaaa1111-1111-1111-1111-111111111111",
				Name:        "session_cleanup_hourly",
				JobType:     "stale_session_cleanup",
				CronExpr:    "0 * * * *",
				Timezone:    "UTC",
				Enabled:     true,
				MaxAttempts: 3,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
	}
}

func jobStrPtr(s string) *string { return &s }

// --- Jobs List ---

func TestHandleAdminListJobs(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminListJobs(svc)

	req := httptest.NewRequest("GET", "/api/admin/jobs", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []jobs.Job `json:"items"`
		Count int        `json:"count"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, 3, resp.Count)
	testutil.Equal(t, 3, len(resp.Items))
}

func TestHandleAdminListJobsFilterState(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminListJobs(svc)

	req := httptest.NewRequest("GET", "/api/admin/jobs?state=queued", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []jobs.Job `json:"items"`
		Count int        `json:"count"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, 1, resp.Count)
}

func TestHandleAdminListJobsFilterType(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminListJobs(svc)

	req := httptest.NewRequest("GET", "/api/admin/jobs?type=stale_session_cleanup", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []jobs.Job `json:"items"`
		Count int        `json:"count"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, 2, resp.Count)
}

// --- Jobs Get ---

func TestHandleAdminGetJob(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminGetJob(svc)

	r := chi.NewRouter()
	r.Get("/api/admin/jobs/{id}", handler)

	req := httptest.NewRequest("GET", "/api/admin/jobs/11111111-1111-1111-1111-111111111111", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var job jobs.Job
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &job))
	testutil.Equal(t, "11111111-1111-1111-1111-111111111111", job.ID)
}

func TestHandleAdminGetJobNotFound(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminGetJob(svc)

	r := chi.NewRouter()
	r.Get("/api/admin/jobs/{id}", handler)

	req := httptest.NewRequest("GET", "/api/admin/jobs/99999999-9999-9999-9999-999999999999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleAdminGetJobInvalidUUID(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminGetJob(svc)

	r := chi.NewRouter()
	r.Get("/api/admin/jobs/{id}", handler)

	req := httptest.NewRequest("GET", "/api/admin/jobs/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Jobs Retry ---

func TestHandleAdminRetryJob(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminRetryJob(svc)

	r := chi.NewRouter()
	r.Post("/api/admin/jobs/{id}/retry", handler)

	req := httptest.NewRequest("POST", "/api/admin/jobs/33333333-3333-3333-3333-333333333333/retry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var job jobs.Job
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &job))
	testutil.Equal(t, jobs.StateQueued, job.State)
}

func TestHandleAdminRetryJobNotFailed(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminRetryJob(svc)

	r := chi.NewRouter()
	r.Post("/api/admin/jobs/{id}/retry", handler)

	// Job 22222222 is queued, not failed.
	req := httptest.NewRequest("POST", "/api/admin/jobs/22222222-2222-2222-2222-222222222222/retry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusConflict, w.Code)
}

// --- Jobs Cancel ---

func TestHandleAdminCancelJob(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminCancelJob(svc)

	r := chi.NewRouter()
	r.Post("/api/admin/jobs/{id}/cancel", handler)

	req := httptest.NewRequest("POST", "/api/admin/jobs/22222222-2222-2222-2222-222222222222/cancel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var job jobs.Job
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &job))
	testutil.Equal(t, jobs.StateCanceled, job.State)
}

func TestHandleAdminCancelJobNotQueued(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminCancelJob(svc)

	r := chi.NewRouter()
	r.Post("/api/admin/jobs/{id}/cancel", handler)

	// Job 11111111 is completed, not queued.
	req := httptest.NewRequest("POST", "/api/admin/jobs/11111111-1111-1111-1111-111111111111/cancel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusConflict, w.Code)
}

// --- Jobs Stats ---

func TestHandleAdminJobStats(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminJobStats(svc)

	req := httptest.NewRequest("GET", "/api/admin/jobs/stats", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var stats jobs.QueueStats
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))
	testutil.Equal(t, 1, stats.Queued)
	testutil.Equal(t, 1, stats.Completed)
	testutil.Equal(t, 1, stats.Failed)
}

// --- Schedules List ---

func TestHandleAdminListSchedules(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminListSchedules(svc)

	req := httptest.NewRequest("GET", "/api/admin/schedules", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []jobs.Schedule `json:"items"`
		Count int             `json:"count"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, 1, resp.Count)
}

// --- Schedules Create ---

func TestHandleAdminCreateSchedule(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminCreateSchedule(svc)

	body := `{"name":"test_sched","jobType":"stale_session_cleanup","cronExpr":"0 * * * *","timezone":"UTC","enabled":true}`
	req := httptest.NewRequest("POST", "/api/admin/schedules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var sched jobs.Schedule
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &sched))
	testutil.Equal(t, "test_sched", sched.Name)
}

func TestHandleAdminCreateScheduleMissingName(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminCreateSchedule(svc)

	body := `{"jobType":"test","cronExpr":"0 * * * *"}`
	req := httptest.NewRequest("POST", "/api/admin/schedules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleAdminCreateScheduleInvalidCron(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminCreateSchedule(svc)

	body := `{"name":"bad","jobType":"test","cronExpr":"invalid","timezone":"UTC"}`
	req := httptest.NewRequest("POST", "/api/admin/schedules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Schedules Update ---

func TestHandleAdminUpdateSchedule(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminUpdateSchedule(svc)

	r := chi.NewRouter()
	r.Put("/api/admin/schedules/{id}", handler)

	body := `{"cronExpr":"*/5 * * * *","timezone":"UTC","enabled":true}`
	req := httptest.NewRequest("PUT", "/api/admin/schedules/aaaa1111-1111-1111-1111-111111111111", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var sched jobs.Schedule
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &sched))
	testutil.Equal(t, "*/5 * * * *", sched.CronExpr)
}

func TestHandleAdminUpdateScheduleNotFound(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminUpdateSchedule(svc)

	r := chi.NewRouter()
	r.Put("/api/admin/schedules/{id}", handler)

	body := `{"cronExpr":"*/5 * * * *","timezone":"UTC","enabled":true}`
	req := httptest.NewRequest("PUT", "/api/admin/schedules/99999999-9999-9999-9999-999999999999", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleAdminUpdateScheduleEnableRecomputesNextRunAt(t *testing.T) {
	svc := newFakeJobService()
	// Simulate a disabled schedule with next_run_at cleared.
	svc.schedules[0].Enabled = false
	svc.schedules[0].NextRunAt = nil
	handler := handleAdminUpdateSchedule(svc)

	r := chi.NewRouter()
	r.Put("/api/admin/schedules/{id}", handler)

	body := `{"enabled":true}`
	req := httptest.NewRequest("PUT", "/api/admin/schedules/aaaa1111-1111-1111-1111-111111111111", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "aaaa1111-1111-1111-1111-111111111111", svc.lastUpdateScheduleID)
	testutil.True(t, svc.lastUpdateEnabled, "enabled should be true")
	testutil.NotNil(t, svc.lastUpdateNextRunAt)
}

// --- Schedules Delete ---

func TestHandleAdminDeleteSchedule(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminDeleteSchedule(svc)

	r := chi.NewRouter()
	r.Delete("/api/admin/schedules/{id}", handler)

	req := httptest.NewRequest("DELETE", "/api/admin/schedules/aaaa1111-1111-1111-1111-111111111111", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
}

func TestHandleAdminDeleteScheduleNotFound(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminDeleteSchedule(svc)

	r := chi.NewRouter()
	r.Delete("/api/admin/schedules/{id}", handler)

	req := httptest.NewRequest("DELETE", "/api/admin/schedules/99999999-9999-9999-9999-999999999999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

// --- Schedules Enable/Disable ---

func TestHandleAdminEnableSchedule(t *testing.T) {
	svc := newFakeJobService()
	// Disable it first.
	svc.schedules[0].Enabled = false
	handler := handleAdminEnableSchedule(svc)

	r := chi.NewRouter()
	r.Post("/api/admin/schedules/{id}/enable", handler)

	req := httptest.NewRequest("POST", "/api/admin/schedules/aaaa1111-1111-1111-1111-111111111111/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var sched jobs.Schedule
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &sched))
	testutil.True(t, sched.Enabled, "schedule should be enabled")
}

func TestHandleAdminDisableSchedule(t *testing.T) {
	svc := newFakeJobService()
	handler := handleAdminDisableSchedule(svc)

	r := chi.NewRouter()
	r.Post("/api/admin/schedules/{id}/disable", handler)

	req := httptest.NewRequest("POST", "/api/admin/schedules/aaaa1111-1111-1111-1111-111111111111/disable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var sched jobs.Schedule
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &sched))
	testutil.False(t, sched.Enabled, "schedule should be disabled")
}
