package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/adhocore/gronx"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/jobs"
)

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
		scheduleID, ok := parseUUIDParamWithLabel(w, r, "id", "schedule id")
		if !ok {
			return
		}
		id := scheduleID.String()

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
		scheduleID, ok := parseUUIDParamWithLabel(w, r, "id", "schedule id")
		if !ok {
			return
		}
		id := scheduleID.String()

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
		scheduleID, ok := parseUUIDParamWithLabel(w, r, "id", "schedule id")
		if !ok {
			return
		}
		id := scheduleID.String()

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
		scheduleID, ok := parseUUIDParamWithLabel(w, r, "id", "schedule id")
		if !ok {
			return
		}
		id := scheduleID.String()

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

func (s *Server) handleSchedulesList(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		serviceUnavailable(w, serviceUnavailableJobQueue)
		return
	}
	handleAdminListSchedules(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleSchedulesCreate(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		serviceUnavailable(w, serviceUnavailableJobQueue)
		return
	}
	handleAdminCreateSchedule(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleSchedulesUpdate(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		serviceUnavailable(w, serviceUnavailableJobQueue)
		return
	}
	handleAdminUpdateSchedule(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleSchedulesDelete(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		serviceUnavailable(w, serviceUnavailableJobQueue)
		return
	}
	handleAdminDeleteSchedule(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleSchedulesEnable(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		serviceUnavailable(w, serviceUnavailableJobQueue)
		return
	}
	handleAdminEnableSchedule(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleSchedulesDisable(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		serviceUnavailable(w, serviceUnavailableJobQueue)
		return
	}
	handleAdminDisableSchedule(s.jobService).ServeHTTP(w, r)
}
