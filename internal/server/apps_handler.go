package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// appManager is the interface for admin app operations.
// auth.Service satisfies this interface.
type appManager interface {
	CreateApp(ctx context.Context, name, description, ownerUserID string) (*auth.App, error)
	GetApp(ctx context.Context, id string) (*auth.App, error)
	ListApps(ctx context.Context, page, perPage int) (*auth.AppListResult, error)
	UpdateApp(ctx context.Context, id, name, description string, rateLimitRPS, rateLimitWindowSeconds int) (*auth.App, error)
	DeleteApp(ctx context.Context, id string) error
}

type createAppRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerUserID string `json:"ownerUserId"`
}

type updateAppRequest struct {
	Name                   string `json:"name"`
	Description            string `json:"description"`
	RateLimitRPS           int    `json:"rateLimitRps"`
	RateLimitWindowSeconds int    `json:"rateLimitWindowSeconds"`
}

// handleAdminListApps returns a paginated list of all apps.
func handleAdminListApps(svc appManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))

		result, err := svc.ListApps(r.Context(), page, perPage)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list apps")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, result)
	}
}

// handleAdminGetApp returns a single app by ID.
func handleAdminGetApp(svc appManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			httputil.WriteError(w, http.StatusBadRequest, "app id is required")
			return
		}
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid app id format")
			return
		}

		app, err := svc.GetApp(r.Context(), id)
		if err != nil {
			if errors.Is(err, auth.ErrAppNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "app not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get app")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, app)
	}
}

// handleAdminCreateApp creates a new app.
func handleAdminCreateApp(svc appManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createAppRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			httputil.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.OwnerUserID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "ownerUserId is required")
			return
		}
		if !httputil.IsValidUUID(req.OwnerUserID) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid ownerUserId format")
			return
		}

		app, err := svc.CreateApp(r.Context(), req.Name, req.Description, req.OwnerUserID)
		if err != nil {
			if errors.Is(err, auth.ErrAppOwnerNotFound) {
				httputil.WriteError(w, http.StatusBadRequest, "owner user not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to create app")
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, app)
	}
}

// handleAdminUpdateApp updates an existing app.
func handleAdminUpdateApp(svc appManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			httputil.WriteError(w, http.StatusBadRequest, "app id is required")
			return
		}
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid app id format")
			return
		}

		var req updateAppRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			httputil.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.RateLimitRPS < 0 || req.RateLimitWindowSeconds < 0 {
			httputil.WriteError(w, http.StatusBadRequest, "rate limit values must be non-negative")
			return
		}

		app, err := svc.UpdateApp(r.Context(), id, req.Name, req.Description, req.RateLimitRPS, req.RateLimitWindowSeconds)
		if err != nil {
			if errors.Is(err, auth.ErrAppNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "app not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update app")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, app)
	}
}

// handleAdminDeleteApp deletes an app by ID.
func handleAdminDeleteApp(svc appManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			httputil.WriteError(w, http.StatusBadRequest, "app id is required")
			return
		}
		if !httputil.IsValidUUID(id) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid app id format")
			return
		}

		err := svc.DeleteApp(r.Context(), id)
		if err != nil {
			if errors.Is(err, auth.ErrAppNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "app not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to delete app")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
