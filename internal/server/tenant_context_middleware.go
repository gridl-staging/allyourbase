package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/observability"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"
)

// resolveTenantContext is middleware that extracts the tenant ID from JWT claims, URL params, or request headers and stores it in the request context.
func (s *Server) resolveTenantContext(next http.Handler) http.Handler {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tenantID := tenantIDFromRequest(r)
		if tenantID == "" && s != nil && s.isAdminToken(r) {
			tenantID = requestHeaderTenantID(r)
		}

		if tenantID != "" {
			ctx = tenant.ContextWithTenantID(ctx, tenantID)
			// Add tenant attributes to the current OTel span for distributed tracing.
			if span := trace.SpanFromContext(ctx); span != nil {
				observability.SetSpanTenantAttrs(span, tenantID)
			}
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
	if s == nil || s.authSvc == nil {
		return handler
	}
	return auth.OptionalAuth(s.authSvc)(handler)
}

func requestHeaderTenantID(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
}

// tenantIDFromRequest extracts a tenant ID from the request by checking JWT claims first, then URL params, and finally the X-Tenant-ID header for allowed anonymous paths.
func tenantIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	claims := auth.ClaimsFromContext(r.Context())
	if claims != nil {
		if tenantID := strings.TrimSpace(claims.TenantID); tenantID != "" {
			return tenantID
		}
	}
	if tenantID := strings.TrimSpace(chi.URLParam(r, "tenantId")); tenantID != "" {
		return tenantID
	}
	if claims == nil && !allowsAnonymousTenantHeaderFallback(r.URL.Path) {
		return ""
	}
	// Allow anonymous tenant header fallback only on explicitly public
	// tenant-aware API surfaces that rely on tenant context for quotas.
	return requestHeaderTenantID(r)
}

func allowsAnonymousTenantHeaderFallback(path string) bool {
	normalizedPath := strings.TrimSpace(path)
	switch normalizedPath {
	case "/api/admin/status", "/api/realtime", "/api/realtime/ws":
		return true
	default:
		return strings.HasPrefix(normalizedPath, "/api/storage")
	}
}

// tenantIDFromContextOrRequest returns the tenant ID from the request context if already resolved, falling back to tenantIDFromRequest for /api/ routes only.
func tenantIDFromContextOrRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if tenantID := strings.TrimSpace(tenant.TenantFromContext(r.Context())); tenantID != "" {
		return tenantID
	}
	// resolveTenantContext runs on /api routes; avoid inferring tenant_id from
	// raw headers/params on non-API routes where tenant context is not resolved.
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return ""
	}
	return tenantIDFromRequest(r)
}

// requireTenantContext is middleware that validates tenant presence and existence, returning 400 if missing and 404 if the tenant is not found or has been deleted.
func (s *Server) requireTenantContext(next http.Handler) http.Handler {
	return s.resolveTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant.TenantFromContext(r.Context())
		if tenantID == "" {
			httputil.WriteError(w, http.StatusBadRequest, "tenant context required")
			return
		}

		if s.tenantSvc == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "tenant service not configured")
			return
		}

		t, err := s.tenantSvc.GetTenant(r.Context(), tenantID)
		if err != nil {
			if errors.Is(err, tenant.ErrTenantNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "tenant not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to validate tenant")
			return
		}
		if t != nil && t.State == tenant.TenantStateDeleted {
			httputil.WriteError(w, http.StatusNotFound, "tenant not found")
			return
		}
		next.ServeHTTP(w, r)
	}))
}
