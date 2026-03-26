package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
)

// checkAppTenantOwnership verifies that the app belongs to the request's tenant
// context. Returns true if the request should continue (ownership is valid or
// no tenant context is present). Returns false and writes a 403 if the app's
// tenant_id does not match the request tenant.
func checkAppTenantOwnership(w http.ResponseWriter, r *http.Request, app *auth.App) bool {
	ctxTenant := tenant.TenantFromContext(r.Context())
	if ctxTenant == "" {
		return true // no tenant scoping — admin without context
	}
	if app.TenantID == nil || *app.TenantID != ctxTenant {
		httputil.WriteError(w, http.StatusForbidden, "app does not belong to tenant")
		return false
	}
	return true
}

// appManager is the interface for admin app operations.
// auth.Service satisfies this interface.
type appManager interface {
	CreateApp(ctx context.Context, name, description, ownerUserID string) (*auth.App, error)
	GetApp(ctx context.Context, id string) (*auth.App, error)
	ListApps(ctx context.Context, page, perPage int) (*auth.AppListResult, error)
	ListAppsByTenant(ctx context.Context, tenantID string, page, perPage int) (*auth.AppListResult, error)
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
// When a tenant context is present, results are filtered to only include
// apps belonging to that tenant.
func handleAdminListApps(svc appManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))
		ctxTenant := tenant.TenantFromContext(r.Context())

		var (
			result *auth.AppListResult
			err    error
		)
		if ctxTenant != "" {
			result, err = svc.ListAppsByTenant(r.Context(), ctxTenant, page, perPage)
		} else {
			result, err = svc.ListApps(r.Context(), page, perPage)
		}
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
		appID, ok := parseUUIDParamWithLabel(w, r, "id", "app id")
		if !ok {
			return
		}
		id := appID.String()

		app, err := svc.GetApp(r.Context(), id)
		if err != nil {
			if errors.Is(err, auth.ErrAppNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "app not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get app")
			return
		}

		if !checkAppTenantOwnership(w, r, app) {
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
		if !validateOwnerUserID(w, req.OwnerUserID) {
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
		appID, ok := parseUUIDParamWithLabel(w, r, "id", "app id")
		if !ok {
			return
		}
		id := appID.String()

		// Verify tenant ownership before mutation.
		if tenant.TenantFromContext(r.Context()) != "" {
			existing, err := svc.GetApp(r.Context(), id)
			if err != nil {
				if errors.Is(err, auth.ErrAppNotFound) {
					httputil.WriteError(w, http.StatusNotFound, "app not found")
					return
				}
				httputil.WriteError(w, http.StatusInternalServerError, "failed to get app")
				return
			}
			if !checkAppTenantOwnership(w, r, existing) {
				return
			}
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
		appID, ok := parseUUIDParamWithLabel(w, r, "id", "app id")
		if !ok {
			return
		}
		id := appID.String()

		// Verify tenant ownership before deletion.
		if tenant.TenantFromContext(r.Context()) != "" {
			existing, err := svc.GetApp(r.Context(), id)
			if err != nil {
				if errors.Is(err, auth.ErrAppNotFound) {
					httputil.WriteError(w, http.StatusNotFound, "app not found")
					return
				}
				httputil.WriteError(w, http.StatusInternalServerError, "failed to get app")
				return
			}
			if !checkAppTenantOwnership(w, r, existing) {
				return
			}
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
