package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
)

// isTenantRecoveryEndpoint returns true only for tenant admin recovery
// endpoints that must bypass availability gating (maintenance mode and breaker)
// so operators can recover a blocked tenant.
func isTenantRecoveryEndpoint(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 5 {
		return false
	}
	if parts[0] != "api" || parts[1] != "admin" || parts[2] != "tenants" || strings.TrimSpace(parts[3]) == "" {
		return false
	}
	switch parts[4] {
	case "maintenance":
		return len(parts) == 5 || (len(parts) == 6 && (parts[5] == "enable" || parts[5] == "disable"))
	case "breaker":
		return len(parts) == 5 || (len(parts) == 6 && parts[5] == "reset")
	default:
		return false
	}
}

// enforceTenantAvailability is middleware that returns 503 Service Unavailable for tenants under maintenance or with an open circuit breaker, including a Retry-After header, while allowing recovery endpoints to proceed.
func (s *Server) enforceTenantAvailability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant.TenantFromContext(r.Context())
		if tenantID == "" {
			next.ServeHTTP(w, r)
			return
		}

		if isTenantRecoveryEndpoint(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		if s.tenantSvc != nil {
			underMaintenance, err := s.tenantSvc.IsUnderMaintenance(r.Context(), tenantID)
			if err != nil {
				if logger := s.currentLogger(); logger != nil {
					logger.Warn("maintenance mode check failed, blocking request (fail-closed)", "error", err, "tenant_id", tenantID)
				}
				w.Header().Set("Retry-After", "30")
				httputil.WriteError(w, http.StatusServiceUnavailable, "tenant availability unknown")
				return
			} else if underMaintenance {
				w.Header().Set("Retry-After", "60")
				httputil.WriteError(w, http.StatusServiceUnavailable, "tenant under maintenance")
				return
			}
		}

		if s.tenantBreakerTracker != nil {
			err := s.tenantBreakerTracker.Allow(tenantID)
			if err != nil {
				var breakerErr *tenant.TenantBreakerOpenError
				if errors.As(err, &breakerErr) {
					retryAfter := int(breakerErr.RetryAfter.Seconds())
					if retryAfter <= 0 {
						retryAfter = 30
					}
					w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
					httputil.WriteError(w, http.StatusServiceUnavailable, "tenant temporarily unavailable")
					return
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}
