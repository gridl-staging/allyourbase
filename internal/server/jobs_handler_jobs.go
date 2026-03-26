package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
)

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
		jobID, ok := parseUUIDParamWithLabel(w, r, "id", "job id")
		if !ok {
			return
		}
		id := jobID.String()

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
		jobID, ok := parseUUIDParamWithLabel(w, r, "id", "job id")
		if !ok {
			return
		}
		id := jobID.String()

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
		jobID, ok := parseUUIDParamWithLabel(w, r, "id", "job id")
		if !ok {
			return
		}
		id := jobID.String()

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

func (s *Server) handleJobsList(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		serviceUnavailable(w, serviceUnavailableJobQueue)
		return
	}
	handleAdminListJobs(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleJobsGet(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		serviceUnavailable(w, serviceUnavailableJobQueue)
		return
	}
	handleAdminGetJob(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleJobsRetry(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		serviceUnavailable(w, serviceUnavailableJobQueue)
		return
	}
	handleAdminRetryJob(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleJobsCancel(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		serviceUnavailable(w, serviceUnavailableJobQueue)
		return
	}
	handleAdminCancelJob(s.jobService).ServeHTTP(w, r)
}

func (s *Server) handleJobsStats(w http.ResponseWriter, r *http.Request) {
	if s.jobService == nil {
		serviceUnavailable(w, serviceUnavailableJobQueue)
		return
	}
	handleAdminJobStats(s.jobService).ServeHTTP(w, r)
}
