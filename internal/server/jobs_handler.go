package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/adhocore/gronx"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/go-chi/chi/v5"
)

// jobAdmin is the interface for job queue admin operations.
// jobs.Service satisfies this interface.
type jobAdmin interface {
	List(ctx context.Context, state, jobType string, limit, offset int) ([]jobs.Job, error)
	Get(ctx context.Context, jobID string) (*jobs.Job, error)
	RetryNow(ctx context.Context, jobID string) (*jobs.Job, error)
	Cancel(ctx context.Context, jobID string) (*jobs.Job, error)
	Stats(ctx context.Context) (*jobs.QueueStats, error)

	ListSchedules(ctx context.Context) ([]jobs.Schedule, error)
	GetSchedule(ctx context.Context, id string) (*jobs.Schedule, error)
	CreateSchedule(ctx context.Context, sched *jobs.Schedule) (*jobs.Schedule, error)
	UpdateSchedule(ctx context.Context, id, cronExpr, timezone string, payload json.RawMessage, enabled bool, nextRunAt *time.Time) (*jobs.Schedule, error)
	DeleteSchedule(ctx context.Context, id string) error
	SetScheduleEnabled(ctx context.Context, id string, enabled bool) (*jobs.Schedule, error)
}

type jobListResponse struct {
	Items []jobs.Job `json:"items"`
	Count int        `json:"count"` // number of items returned (page size, not total)
}

type scheduleListResponse struct {
	Items []jobs.Schedule `json:"items"`
	Count int             `json:"count"` // number of items returned
}

type createScheduleRequest struct {
	Name        string          `json:"name"`
	JobType     string          `json:"jobType"`
	CronExpr    string          `json:"cronExpr"`
	Timezone    string          `json:"timezone"`
	Payload     json.RawMessage `json:"payload"`
	Enabled     *bool           `json:"enabled"`
	MaxAttempts int             `json:"maxAttempts"`
}

type updateScheduleRequest struct {
	CronExpr string          `json:"cronExpr"`
	Timezone string          `json:"timezone"`
	Payload  json.RawMessage `json:"payload"`
	Enabled  *bool           `json:"enabled"`
}

// handleAdminListJobs returns a list of jobs with optional filters.
func handleAdminListJobs(svc jobAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		if state != "" {
			switch state {
			case "queued", "running", "completed", "failed", "canceled":
			default:
				httputil.WriteError(w, http.StatusBadRequest, "invalid state filter; must be one of: queued, running, completed, failed, canceled")
				return
			}
		}
		jobType := r.URL.Query().Get("type")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if limit <= 0 {
			limit = 50
		}
		if limit > 500 {
			limit = 500
		}
		if offset < 0 {
			offset = 0
		}

		items, err := svc.List(r.Context(), state, jobType, limit, offset)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list jobs")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, jobListResponse{
			Items: items,
			Count: len(items),
		})
	}
}

// handleAdminGetJob returns a single job by ID.
func handleAdminGetJob(svc jobAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid job id format")
			return
		}

		job, err := svc.Get(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				httputil.WriteError(w, http.StatusNotFound, "job not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get job")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, job)
	}
}

// handleAdminRetryJob resets a failed job to queued.
func handleAdminRetryJob(svc jobAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid job id format")
			return
		}

		job, err := svc.RetryNow(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not in failed state") {
				httputil.WriteError(w, http.StatusConflict, err.Error())
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to retry job")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, job)
	}
}

// handleAdminCancelJob cancels a queued job.
func handleAdminCancelJob(svc jobAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid job id format")
			return
		}

		job, err := svc.Cancel(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not in queued state") {
				httputil.WriteError(w, http.StatusConflict, err.Error())
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to cancel job")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, job)
	}
}

// handleAdminJobStats returns aggregate queue statistics.
func handleAdminJobStats(svc jobAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := svc.Stats(r.Context())
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get queue stats")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, stats)
	}
}

// handleAdminListSchedules returns all schedules.
func handleAdminListSchedules(svc jobAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := svc.ListSchedules(r.Context())
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list schedules")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, scheduleListResponse{
			Items: items,
			Count: len(items),
		})
	}
}

// handleAdminCreateSchedule creates a new schedule.
func handleAdminCreateSchedule(svc jobAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createScheduleRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			httputil.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if len(req.Name) > 100 {
			httputil.WriteError(w, http.StatusBadRequest, "name must be at most 100 characters")
			return
		}
		if req.JobType == "" {
			httputil.WriteError(w, http.StatusBadRequest, "jobType is required")
			return
		}
		if len(req.JobType) > 100 {
			httputil.WriteError(w, http.StatusBadRequest, "jobType must be at most 100 characters")
			return
		}
		if req.CronExpr == "" {
			httputil.WriteError(w, http.StatusBadRequest, "cronExpr is required")
			return
		}
		gron := gronx.New()
		if !gron.IsValid(req.CronExpr) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid cron expression")
			return
		}
		if req.Timezone == "" {
			req.Timezone = "UTC"
		}
		if _, err := time.LoadLocation(req.Timezone); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid timezone")
			return
		}

		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		maxAttempts := 3
		if req.MaxAttempts > 0 {
			maxAttempts = req.MaxAttempts
		}

		// Compute initial next_run_at.
		nextRunAt, err := jobs.CronNextTime(req.CronExpr, req.Timezone, time.Now())
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "failed to compute next run time: "+err.Error())
			return
		}

		sched, err := svc.CreateSchedule(r.Context(), &jobs.Schedule{
			Name:        req.Name,
			JobType:     req.JobType,
			Payload:     req.Payload,
			CronExpr:    req.CronExpr,
			Timezone:    req.Timezone,
			Enabled:     enabled,
			MaxAttempts: maxAttempts,
			NextRunAt:   &nextRunAt,
		})
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to create schedule")
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, sched)
	}
}

// handleAdminUpdateSchedule updates a schedule's mutable fields.
// Uses read-modify-write: fetches the existing schedule first, then merges
// only the fields the client provided, avoiding zero-value overwrites.
func handleAdminUpdateSchedule(svc jobAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid schedule id format")
			return
		}

		var req updateScheduleRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		// Fetch existing schedule to use as base for merge.
		existing, err := svc.GetSchedule(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				httputil.WriteError(w, http.StatusNotFound, "schedule not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get schedule")
			return
		}

		// Merge: use request values when provided, existing values otherwise.
		cronExpr := existing.CronExpr
		if req.CronExpr != "" {
			gron := gronx.New()
			if !gron.IsValid(req.CronExpr) {
				httputil.WriteError(w, http.StatusBadRequest, "invalid cron expression")
				return
			}
			cronExpr = req.CronExpr
		}

		tz := existing.Timezone
		if req.Timezone != "" {
			if _, err := time.LoadLocation(req.Timezone); err != nil {
				httputil.WriteError(w, http.StatusBadRequest, "invalid timezone")
				return
			}
			tz = req.Timezone
		}

		enabled := existing.Enabled
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		payload := existing.Payload
		if req.Payload != nil {
			payload = req.Payload
		}

		// Recompute next_run_at if cron or timezone changed.
		var nextRunAt *time.Time
		cronChanged := req.CronExpr != "" && req.CronExpr != existing.CronExpr
		tzChanged := req.Timezone != "" && req.Timezone != existing.Timezone
		enableTransition := !existing.Enabled && enabled
		if cronChanged || tzChanged || enableTransition {
			t, err := jobs.CronNextTime(cronExpr, tz, time.Now())
			if err != nil {
				httputil.WriteError(w, http.StatusBadRequest, "failed to compute next run time")
				return
			}
			nextRunAt = &t
		} else {
			nextRunAt = existing.NextRunAt
		}

		sched, err := svc.UpdateSchedule(r.Context(), id, cronExpr, tz, payload, enabled, nextRunAt)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				httputil.WriteError(w, http.StatusNotFound, "schedule not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update schedule")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, sched)
	}
}

// handleAdminDeleteSchedule hard-deletes a schedule.
func handleAdminDeleteSchedule(svc jobAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid schedule id format")
			return
		}

		err := svc.DeleteSchedule(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				httputil.WriteError(w, http.StatusNotFound, "schedule not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to delete schedule")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// handleAdminEnableSchedule enables a schedule.
func handleAdminEnableSchedule(svc jobAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid schedule id format")
			return
		}

		sched, err := svc.SetScheduleEnabled(r.Context(), id, true)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				httputil.WriteError(w, http.StatusNotFound, "schedule not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to enable schedule")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, sched)
	}
}

// handleAdminDisableSchedule disables a schedule.
func handleAdminDisableSchedule(svc jobAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid schedule id format")
			return
		}

		sched, err := svc.SetScheduleEnabled(r.Context(), id, false)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				httputil.WriteError(w, http.StatusNotFound, "schedule not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to disable schedule")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, sched)
	}
}
