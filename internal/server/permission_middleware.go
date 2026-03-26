// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/permission_middleware.go.
package server

import (
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
)

// TODO: Document Server.requireTenantPermission.
func (s *Server) requireTenantPermission(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.isAdminToken(r) {
			next.ServeHTTP(w, r)
			return
		}

		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil || strings.TrimSpace(claims.Subject) == "" {
			next.ServeHTTP(w, r)
			return
		}

		tenantID := strings.TrimSpace(tenant.TenantFromContext(r.Context()))
		if tenantID == "" {
			next.ServeHTTP(w, r)
			return
		}
		if s == nil || s.permResolver == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "tenant permissions not configured")
			return
		}

		permission, err := s.permResolver.ResolvePermissions(r.Context(), claims.Subject, tenantID)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to resolve tenant permissions")
			return
		}
		if permission == nil {
			httputil.WriteError(w, http.StatusForbidden, "forbidden")
			return
		}

		next.ServeHTTP(w, r.WithContext(tenant.ContextWithPermission(r.Context(), permission)))
	})
}
