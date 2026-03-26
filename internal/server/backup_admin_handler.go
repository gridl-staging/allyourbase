// Package server This file implements HTTP handlers for backup administration, allowing clients to list existing backups and trigger new backup operations.
package server

import (
	"context"
	"net/http"
	"strconv"

	"github.com/allyourbase/ayb/internal/backup"
	"github.com/allyourbase/ayb/internal/httputil"
)

// backupAdmin is the interface the backup admin handlers need.
type backupAdmin interface {
	List(ctx context.Context, f backup.ListFilter) ([]backup.BackupRecord, int, error)
	TriggerBackup(ctx context.Context) (backup.RunResult, error)
}

// handleAdminBackupList handles requests to list backup records with optional filtering by status and pagination via limit and offset query parameters, defaulting to a limit of 50 and offset of 0.
func (s *Server) handleAdminBackupList(w http.ResponseWriter, r *http.Request) {
	if s.backupService == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"backups": []backup.BackupRecord{}, "total": 0})
		return
	}

	status := r.URL.Query().Get("status")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 50
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}
	offset := 0
	if offsetStr != "" {
		if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
			offset = v
		}
	}

	records, total, err := s.backupService.List(r.Context(), backup.ListFilter{
		Status: status,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if records == nil {
		records = []backup.BackupRecord{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"backups": records, "total": total})
}

// handleAdminBackupTrigger handles requests to initiate a backup operation, returning the backup ID and status with a 202 Accepted response, or an error if the backup service is not configured.
func (s *Server) handleAdminBackupTrigger(w http.ResponseWriter, r *http.Request) {
	if s.backupService == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "backup service not configured")
		return
	}

	result, err := s.backupService.TriggerBackup(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"backup_id": result.BackupID,
		"status":    result.Status,
	})
}
