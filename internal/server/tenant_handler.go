// Package server Handlers for tenant administration operations including creation, deletion, state transitions, and membership management.
package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

func isValidSlug(slug string) bool {
	return tenant.IsValidSlug(slug)
}

func isValidIsolationMode(mode string) bool {
	switch mode {
	case "", "shared", "schema", "database":
		return true
	default:
		return false
	}
}

func isValidPlanTier(tier string) bool {
	if tier == "" {
		return true
	}

	switch billing.Plan(tier) {
	case billing.PlanFree, billing.PlanStarter, billing.PlanPro, billing.PlanEnterprise:
		return true
	default:
		return false
	}
}

func tenantIDFromURL(r *http.Request, w http.ResponseWriter) (string, bool) {
	tenantID := chi.URLParam(r, "tenantId")
	if tenantID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "tenant id is required")
		return "", false
	}
	return tenantID, true
}

func lookupTenantByID(r *http.Request, w http.ResponseWriter, svc tenantAdmin, tenantID string) (*tenant.Tenant, bool) {
	currentTenant, err := svc.GetTenant(r.Context(), tenantID)
	if err != nil {
		if errors.Is(err, tenant.ErrTenantNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "tenant not found")
			return nil, false
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get tenant")
		return nil, false
	}
	return currentTenant, true
}

// lookupTenant extracts the tenantId URL param, validates it is non-empty,
// and loads the tenant via GetTenant. Returns the tenant and true on success.
// On failure, writes an appropriate HTTP error and returns nil, false.
func lookupTenant(r *http.Request, w http.ResponseWriter, svc tenantAdmin) (*tenant.Tenant, bool) {
	tenantID, ok := tenantIDFromURL(r, w)
	if !ok {
		return nil, false
	}
	return lookupTenantByID(r, w, svc, tenantID)
}

func (s *Server) tenantAdminHandler(handler func(tenantAdmin) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.tenantSvc == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "tenant service not configured")
			return
		}
		handler(s.tenantSvc).ServeHTTP(w, r)
	}
}

func (s *Server) tenantAdminAuditHandler(handler func(tenantAdmin, *tenant.AuditEmitter) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.tenantSvc == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "tenant service not configured")
			return
		}
		handler(s.tenantSvc, s.auditEmitter).ServeHTTP(w, r)
	}
}

func (s *Server) tenantBreakerHandler(handler func(*tenant.TenantBreakerTracker) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.tenantBreakerTracker == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "breaker not configured")
			return
		}
		handler(s.tenantBreakerTracker).ServeHTTP(w, r)
	}
}

func (s *Server) tenantBreakerAuditHandler(handler func(*tenant.AuditEmitter, *tenant.TenantBreakerTracker) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.tenantBreakerTracker == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "breaker not configured")
			return
		}
		handler(s.auditEmitter, s.tenantBreakerTracker).ServeHTTP(w, r)
	}
}

// keep string utils here because actor extraction is shared by tenant admin flows.
func nonEmptyTrimmed(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
