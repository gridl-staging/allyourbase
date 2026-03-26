package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/go-chi/chi/v5"
)

// quotaManager is the subset of storage.Service used for admin quota operations.
type quotaManager interface {
	GetUsage(ctx context.Context, userID string) (*storage.QuotaInfo, error)
	SetUserQuota(ctx context.Context, userID string, quotaMB *int) error
}

// handleAdminGetUserQuota handles GET /api/admin/users/{id}/storage-quota.
// Returns the user's current storage usage and quota.
func handleAdminGetUserQuota(svc quotaManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "id")
		info, err := svc.GetUsage(r.Context(), userID)
		if err != nil {
			if errors.Is(err, storage.ErrQuotaUserNotFound) {
				httputil.WriteError(w, http.StatusNotFound, err.Error())
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, info)
	}
}

type setQuotaRequest struct {
	QuotaMB *int `json:"quota_mb"` // nil removes the per-user override
}

// handleAdminSetUserQuota handles PUT /api/admin/users/{id}/storage-quota.
// Sets (or clears) a per-user storage quota override.
func handleAdminSetUserQuota(svc quotaManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "id")

		var req setQuotaRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.QuotaMB != nil && *req.QuotaMB < 0 {
			httputil.WriteError(w, http.StatusBadRequest, "quota_mb must be >= 0")
			return
		}

		if err := svc.SetUserQuota(r.Context(), userID, req.QuotaMB); err != nil {
			if errors.Is(err, storage.ErrQuotaUserNotFound) {
				httputil.WriteError(w, http.StatusNotFound, err.Error())
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}

		info, err := svc.GetUsage(r.Context(), userID)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, info)
	}
}
