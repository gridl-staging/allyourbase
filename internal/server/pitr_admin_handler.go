// Package server pitr_admin_handler.go defines HTTP handlers for point-in-time recovery operations, providing endpoints to validate recovery windows, initiate database restores, and manage restore jobs.
package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/backup"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// pitrAdmin is the interface the PITR admin handlers need.
type pitrAdmin interface {
	ValidateWindow(ctx context.Context, projectID, databaseID string, targetTime time.Time) (*backup.RestorePlan, error)
	Restore(ctx context.Context, projectID, databaseID string, targetTime time.Time, requestedBy string) (*backup.RestoreJob, error)
	GetJob(ctx context.Context, id string) (*backup.RestoreJob, error)
	ListJobs(ctx context.Context, projectID, databaseID string) ([]backup.RestoreJob, error)
	AbandonJob(ctx context.Context, id string) error
}

type pitrValidateRequest struct {
	TargetTime time.Time `json:"target_time"`
	DatabaseID string    `json:"database_id"`
}

type pitrRestoreRequest struct {
	TargetTime time.Time `json:"target_time"`
	DatabaseID string    `json:"database_id"`
	DryRun     bool      `json:"dry_run"`
}

type pitrValidateResponse struct {
	BaseBackup          *backup.BackupRecord `json:"base_backup"`
	EarliestRecoverable time.Time            `json:"earliest_recoverable"`
	LatestRecoverable   time.Time            `json:"latest_recoverable"`
	EstimatedWALBytes   int64                `json:"estimated_wal_bytes"`
	WALSegmentsCount    int                  `json:"wal_segments_count"`
}

func newPITRValidateResponse(plan *backup.RestorePlan) pitrValidateResponse {
	return pitrValidateResponse{
		BaseBackup:          plan.BaseBackup,
		EarliestRecoverable: plan.EarliestRecoverable,
		LatestRecoverable:   plan.LatestRecoverable,
		EstimatedWALBytes:   plan.EstimatedWALBytes,
		WALSegmentsCount:    len(plan.WALSegments),
	}
}

// handlePITRValidate handles HTTP requests to validate a point-in-time recovery window for a database. It extracts the project ID from the URL and expects a JSON body with target_time and database_id. It returns details about the recoverable window including the base backup, earliest and latest recoverable times, estimated WAL bytes, and WAL segment count. Returns StatusServiceUnavailable if PITR is not configured, StatusBadRequest if required parameters are missing or invalid, and StatusOK with recovery plan details on success.
func (s *Server) handlePITRValidate(w http.ResponseWriter, r *http.Request) {
	if s.pitrService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "PITR not configured")
		return
	}

	projectID := chi.URLParam(r, "projectId")
	if projectID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "projectId is required")
		return
	}

	var req pitrValidateRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if req.TargetTime.IsZero() {
		httputil.WriteError(w, http.StatusBadRequest, "target_time is required")
		return
	}

	if req.DatabaseID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "database_id is required")
		return
	}

	plan, err := s.pitrService.ValidateWindow(r.Context(), projectID, req.DatabaseID, req.TargetTime)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, newPITRValidateResponse(plan))
}

// handlePITRRestore handles HTTP requests to restore a database to a point in time or validate a restore operation. It takes the project ID from the URL and a JSON body with target_time, database_id, and dry_run flag. If dry_run is true, it validates the recovery window and returns recovery plan details. Otherwise, it initiates a restore job and returns the job ID and status. Returns StatusServiceUnavailable if PITR is not configured, StatusBadRequest if required parameters are missing or invalid, StatusConflict if the database is in shadow mode, and StatusAccepted with the restore job details on success.
func (s *Server) handlePITRRestore(w http.ResponseWriter, r *http.Request) {
	if s.pitrService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "PITR not configured")
		return
	}

	projectID := chi.URLParam(r, "projectId")
	if projectID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "projectId is required")
		return
	}

	var req pitrRestoreRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if req.TargetTime.IsZero() {
		httputil.WriteError(w, http.StatusBadRequest, "target_time is required")
		return
	}

	if req.DatabaseID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "database_id is required")
		return
	}

	if req.DryRun {
		plan, err := s.pitrService.ValidateWindow(r.Context(), projectID, req.DatabaseID, req.TargetTime)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.WriteJSON(w, http.StatusOK, newPITRValidateResponse(plan))
		return
	}

	requestedBy := requestedByFromRequest(r)

	job, err := s.pitrService.Restore(r.Context(), projectID, req.DatabaseID, req.TargetTime, requestedBy)
	if err != nil {
		if strings.Contains(err.Error(), "shadow mode") {
			httputil.WriteError(w, http.StatusConflict, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{
		"job_id": job.ID,
		"status": job.Status,
		"phase":  job.Phase,
	})
}

// handlePITRJobGet handles HTTP requests to retrieve a specific restore job by job ID extracted from the URL. Returns the job details on success. Returns StatusServiceUnavailable if PITR is not configured, StatusBadRequest if jobId is missing, StatusNotFound if the job does not exist, and StatusInternalServerError on database errors.
func (s *Server) handlePITRJobGet(w http.ResponseWriter, r *http.Request) {
	if s.pitrService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "PITR not configured")
		return
	}

	jobID := chi.URLParam(r, "jobId")
	if jobID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "jobId is required")
		return
	}

	job, err := s.pitrService.GetJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "job not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, job)
}

// handlePITRJobList handles HTTP requests to list all restore jobs for a project and database. It extracts the project ID from the URL and requires a database_id query parameter. Returns a JSON object containing a jobs array and count. Returns StatusServiceUnavailable if PITR is not configured, StatusBadRequest if required parameters are missing, StatusInternalServerError on database errors, and StatusOK with the jobs list on success.
func (s *Server) handlePITRJobList(w http.ResponseWriter, r *http.Request) {
	if s.pitrService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "PITR not configured")
		return
	}

	projectID := chi.URLParam(r, "projectId")
	if projectID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "projectId is required")
		return
	}

	databaseID := r.URL.Query().Get("database_id")
	if databaseID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "database_id query parameter is required")
		return
	}

	jobs, err := s.pitrService.ListJobs(r.Context(), projectID, databaseID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if jobs == nil {
		jobs = []backup.RestoreJob{}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"jobs":  jobs,
		"count": len(jobs),
	})
}

// handlePITRJobAbandon handles HTTP requests to abandon an active restore job by job ID extracted from the URL. Returns StatusServiceUnavailable if PITR is not configured, StatusBadRequest if jobId is missing, StatusNotFound if the job does not exist or is not active, StatusInternalServerError on database errors, and StatusOK with status abandoned on success.
func (s *Server) handlePITRJobAbandon(w http.ResponseWriter, r *http.Request) {
	if s.pitrService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "PITR not configured")
		return
	}

	jobID := chi.URLParam(r, "jobId")
	if jobID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "jobId is required")
		return
	}

	err := s.pitrService.AbandonJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "not active") {
			httputil.WriteError(w, http.StatusNotFound, "restore job not found or not active")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "abandoned"})
}

// TODO: Document requestedByFromRequest.
func requestedByFromRequest(r *http.Request) string {
	if r == nil {
		return "admin"
	}
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		if subject := strings.TrimSpace(claims.Subject); subject != "" {
			return subject
		}
		if apiKeyID := strings.TrimSpace(claims.APIKeyID); apiKeyID != "" {
			return "api_key:" + apiKeyID
		}
	}
	if principal := strings.TrimSpace(audit.PrincipalFromContext(r.Context())); principal != "" {
		return "admin_token:" + principal
	}
	if ip := strings.TrimSpace(httputil.ClientIP(r)); ip != "" {
		return "admin_ip:" + ip
	}
	return "admin"
}
