package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/adhocore/gronx"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/jobs"
)

var scheduleNow = func() time.Time { return time.Now() }

const defaultScheduleTimezone = "UTC"

func isScheduleNotFound(err error) bool {
	return strings.Contains(err.Error(), "not found")
}

func writeScheduleServiceError(w http.ResponseWriter, err error, internalMessage string) bool {
	if err == nil {
		return false
	}
	if isScheduleNotFound(err) {
		httputil.WriteError(w, http.StatusNotFound, "schedule not found")
		return true
	}
	httputil.WriteError(w, http.StatusInternalServerError, internalMessage)
	return true
}

func validateScheduleCronExpr(w http.ResponseWriter, cronExpr string) bool {
	if gronx.New().IsValid(cronExpr) {
		return true
	}
	httputil.WriteError(w, http.StatusBadRequest, "invalid cron expression")
	return false
}

func validateScheduleTimezone(w http.ResponseWriter, timezone string) bool {
	if _, err := time.LoadLocation(timezone); err == nil {
		return true
	}
	httputil.WriteError(w, http.StatusBadRequest, "invalid timezone")
	return false
}

func scheduleNextRunAt(cronExpr, timezone string) (*time.Time, error) {
	nextRunAt, err := jobs.CronNextTime(cronExpr, timezone, scheduleNow())
	if err != nil {
		return nil, err
	}
	return &nextRunAt, nil
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
		if !validateScheduleCronExpr(w, req.CronExpr) {
			return
		}
		if req.Timezone == "" {
			req.Timezone = defaultScheduleTimezone
		}
		if !validateScheduleTimezone(w, req.Timezone) {
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
		nextRunAt, err := scheduleNextRunAt(req.CronExpr, req.Timezone)
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
			NextRunAt:   nextRunAt,
		})
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to create schedule")
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, sched)
	}
}

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
		if writeScheduleServiceError(w, err, "failed to get schedule") {
			return
		}

		// Merge: use request values when provided, existing values otherwise.
		cronExpr := existing.CronExpr
		if req.CronExpr != "" {
			if !validateScheduleCronExpr(w, req.CronExpr) {
				return
			}
			cronExpr = req.CronExpr
		}

		tz := existing.Timezone
		if req.Timezone != "" {
			if !validateScheduleTimezone(w, req.Timezone) {
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
			t, err := scheduleNextRunAt(cronExpr, tz)
			if err != nil {
				httputil.WriteError(w, http.StatusBadRequest, "failed to compute next run time")
				return
			}
			nextRunAt = t
		} else {
			nextRunAt = existing.NextRunAt
		}

		sched, err := svc.UpdateSchedule(r.Context(), id, cronExpr, tz, payload, enabled, nextRunAt)
		if writeScheduleServiceError(w, err, "failed to update schedule") {
			return
		}

		httputil.WriteJSON(w, http.StatusOK, sched)
	}
}

func handleAdminDeleteSchedule(svc jobAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scheduleID, ok := parseUUIDParamWithLabel(w, r, "id", "schedule id")
		if !ok {
			return
		}
		id := scheduleID.String()

		err := svc.DeleteSchedule(r.Context(), id)
		if writeScheduleServiceError(w, err, "failed to delete schedule") {
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func handleAdminEnableSchedule(svc jobAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scheduleID, ok := parseUUIDParamWithLabel(w, r, "id", "schedule id")
		if !ok {
			return
		}
		id := scheduleID.String()

		sched, err := svc.SetScheduleEnabled(r.Context(), id, true)
		if writeScheduleServiceError(w, err, "failed to enable schedule") {
			return
		}

		httputil.WriteJSON(w, http.StatusOK, sched)
	}
}

func handleAdminDisableSchedule(svc jobAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scheduleID, ok := parseUUIDParamWithLabel(w, r, "id", "schedule id")
		if !ok {
			return
		}
		id := scheduleID.String()

		sched, err := svc.SetScheduleEnabled(r.Context(), id, false)
		if writeScheduleServiceError(w, err, "failed to disable schedule") {
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
