package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
)

// enforceTenantContext is a hard security gate for tenant-scoped flows.
// Unlike requireTenantContext (which returns 400 and validates tenant
// existence), this middleware returns 403 when tenant identity is missing.
// Admin tokens are explicitly allowed to operate without tenant context.
func (s *Server) enforceTenantContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tenant.TenantFromContext(r.Context()) != "" {
			next.ServeHTTP(w, r)
			return
		}
		if s != nil && s.isAdminToken(r) {
			next.ServeHTTP(w, r)
			return
		}
		httputil.WriteError(w, http.StatusForbidden, "tenant context required")
	})
}

// enforceTenantMatch compares the JWT TenantID claim against the tenant
// context in the request. Returns 403 when a JWT is present with a non-empty
// TenantID that differs from the context tenant. This guards against
// token-switching attacks where a valid JWT for tenant A is used on a route
// scoped to tenant B. Passes through when:
//   - No JWT claims are present (admin token path)
//   - JWT claims have empty/whitespace TenantID (legacy pre-migration token)
//   - JWT TenantID matches the context tenant
func (s *Server) enforceTenantMatch(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		claimTenant := ""
		if claims != nil {
			claimTenant = strings.TrimSpace(claims.TenantID)
		}
		if claimTenant != "" {
			ctxTenant := tenant.TenantFromContext(r.Context())
			if ctxTenant != "" && claimTenant != ctxTenant {
				// Emit cross-tenant blocked audit event.
				if s != nil && s.auditEmitter != nil {
					actorIDPtr := getActorID(r)
					ipAddress := getIPAddress(r)
					resourceID := ""
					if claims != nil {
						resourceID = claims.Subject
					}
					s.auditEmitter.EmitCrossTenantBlocked(r.Context(), claimTenant, ctxTenant, resourceID, r.Method+" "+r.URL.Path, actorIDPtr, ipAddress)
				}
				httputil.WriteError(w, http.StatusForbidden, "tenant mismatch")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// enforceOrgScopeAccess checks that org-scoped API keys are only used with
// tenants belonging to the key's org. Non-org-scoped requests pass through.
func (s *Server) enforceOrgScopeAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil || claims.OrgID == "" {
			next.ServeHTTP(w, r)
			return
		}

		tenantID := tenant.TenantFromContext(r.Context())
		if tenantID == "" {
			next.ServeHTTP(w, r)
			return
		}

		if s.tenantSvc == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "tenant service not configured")
			return
		}

		if err := auth.ResolveAPIKeyTenantAccess(r.Context(), claims, tenantID, tenantOrgChecker{svc: s.tenantSvc}); err != nil {
			if errors.Is(err, auth.ErrOrgScopeUnauthorized) || errors.Is(err, tenant.ErrTenantNotFound) {
				httputil.WriteError(w, http.StatusForbidden, "org-scoped key not authorized for this tenant")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to validate org-scoped key access")
			return
		}

		next.ServeHTTP(w, r)
	})
}

type tenantOrgChecker struct {
	svc tenantAdmin
}

func (c tenantOrgChecker) TenantOrgID(ctx context.Context, tenantID string) (*string, error) {
	t, err := c.svc.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return t.OrgID, nil
}
