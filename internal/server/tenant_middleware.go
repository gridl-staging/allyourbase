// Package server This file implements HTTP middleware for tenant context management, availability gating through maintenance mode and circuit breaker patterns, and PostgreSQL schema isolation enforcement.
package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/observability"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"
)

// tenantAdmin is an interface for tenant administration including CRUD operations, membership management, audit event recording, and maintenance state control.
type tenantAdmin interface {
	GetTenant(ctx context.Context, id string) (*tenant.Tenant, error)
	CreateTenant(ctx context.Context, name, slug, isolationMode, planTier, region string, orgMetadata json.RawMessage, idempotencyKey string) (*tenant.Tenant, error)
	DeleteTenantSchema(ctx context.Context, slug string) error
	ListTenants(ctx context.Context, page, perPage int) (*tenant.TenantListResult, error)
	TransitionState(ctx context.Context, id string, fromState, newState tenant.TenantState) (*tenant.Tenant, error)
	UpdateTenant(ctx context.Context, id string, name string, orgMetadata json.RawMessage) (*tenant.Tenant, error)
	AddMembership(ctx context.Context, tenantID, userID, role string) (*tenant.TenantMembership, error)
	RemoveMembership(ctx context.Context, tenantID, userID string) error
	ListMemberships(ctx context.Context, tenantID string) ([]tenant.TenantMembership, error)
	GetMembership(ctx context.Context, tenantID, userID string) (*tenant.TenantMembership, error)
	UpdateMembershipRole(ctx context.Context, tenantID, userID, role string) (*tenant.TenantMembership, error)
	InsertAuditEvent(ctx context.Context, tenantID string, actorID *string, action, result string, metadata json.RawMessage, ipAddress *string) error
	IsUnderMaintenance(ctx context.Context, tenantID string) (bool, error)
	EnableMaintenance(ctx context.Context, tenantID, reason, actorID string) (*tenant.TenantMaintenanceState, error)
	DisableMaintenance(ctx context.Context, tenantID, actorID string) (*tenant.TenantMaintenanceState, error)
	GetMaintenanceState(ctx context.Context, tenantID string) (*tenant.TenantMaintenanceState, error)
	AssignTenantToOrg(ctx context.Context, tenantID, orgID string) error
	UnassignTenantFromOrg(ctx context.Context, tenantID, orgID string) error
	ListOrgTenants(ctx context.Context, orgID string) ([]tenant.Tenant, error)
}

type tenantSearchPathConn interface {
	tenant.RequestConn
	Destroy(ctx context.Context) error
	Release()
}

type tenantConnAcquireFunc func(ctx context.Context) (tenantSearchPathConn, error)

type pgxTenantSearchPathConn struct {
	conn *pgxpool.Conn
}

func (c *pgxTenantSearchPathConn) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return c.conn.Exec(ctx, sql, arguments...)
}

func (c *pgxTenantSearchPathConn) Query(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error) {
	return c.conn.Query(ctx, sql, arguments...)
}

func (c *pgxTenantSearchPathConn) QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row {
	return c.conn.QueryRow(ctx, sql, arguments...)
}

func (c *pgxTenantSearchPathConn) Begin(ctx context.Context) (pgx.Tx, error) {
	return c.conn.Begin(ctx)
}

func (c *pgxTenantSearchPathConn) Release() {
	if c.conn == nil {
		return
	}
	c.conn.Release()
	c.conn = nil
}

func (c *pgxTenantSearchPathConn) Destroy(ctx context.Context) error {
	if c.conn == nil {
		return nil
	}
	conn := c.conn.Hijack()
	c.conn = nil
	return conn.Close(ctx)
}

func newTenantConnAcquire(pool *pgxpool.Pool) tenantConnAcquireFunc {
	if pool == nil {
		return nil
	}
	return func(ctx context.Context) (tenantSearchPathConn, error) {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return nil, err
		}
		return &pgxTenantSearchPathConn{conn: conn}, nil
	}
}

// TODO: Document Server.resolveTenantContext.
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

// useTenantScopedAccessGuards applies the tenant-scoped authorization chain in
// the required order after request authentication has already succeeded.
func (s *Server) useTenantScopedAccessGuards(r chi.Router) {
	r.Use(s.enforceTenantContext)
	r.Use(s.enforceTenantMatch)
	if s != nil && s.tenantSvc != nil {
		r.Use(s.enforceOrgScopeAccess)
	}
	if s != nil && s.permResolver != nil {
		r.Use(s.requireTenantPermission)
	}
}

// withTenantScopedAdminOrUserAuth mounts routes behind the standard
// admin-or-user authentication and tenant-scoped authorization chain.
func (s *Server) withTenantScopedAdminOrUserAuth(r chi.Router, register func(chi.Router)) {
	r.Group(func(r chi.Router) {
		// Accept either a valid admin HMAC token or a user JWT/API-key.
		r.Use(s.requireAdminOrUserAuth(s.authSvc))
		// Apply tenant-scoped auth guards whenever any tenant wiring is enabled.
		// Baseline guards (tenant context + tenant match) should still enforce
		// tenant isolation for partially wired servers, while full guards are
		// enabled conditionally in useTenantScopedAccessGuards.
		if s != nil && (s.tenantSvc != nil || s.permResolver != nil) {
			s.useTenantScopedAccessGuards(r)
		}
		register(r)
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

func requestHeaderTenantID(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
}

// TODO: Document tenantIDFromRequest.
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

// TODO: Document tenantIDFromContextOrRequest.
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

// setTenantSearchPath is middleware that sets the PostgreSQL search_path to a tenant's schema for schema-isolated tenants, resetting it to public after the request completes.
func (s *Server) setTenantSearchPath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant.TenantFromContext(r.Context())
		if tenantID == "" || s == nil || s.tenantSvc == nil || s.tenantConnAcquire == nil {
			next.ServeHTTP(w, r)
			return
		}

		tenantInfo, err := s.tenantSvc.GetTenant(r.Context(), tenantID)
		if err != nil {
			if logger := s.currentLogger(); logger != nil {
				logger.Warn("failed to load tenant for search_path isolation", "tenant_id", tenantID, "error", err)
			}
			httputil.WriteError(w, http.StatusServiceUnavailable, "tenant schema isolation unavailable")
			return
		}
		if tenantInfo == nil {
			if logger := s.currentLogger(); logger != nil {
				logger.Warn("tenant lookup returned nil during search_path isolation", "tenant_id", tenantID)
			}
			httputil.WriteError(w, http.StatusServiceUnavailable, "tenant schema isolation unavailable")
			return
		}
		if tenantInfo.IsolationMode != "schema" {
			next.ServeHTTP(w, r)
			return
		}

		conn, err := s.tenantConnAcquire(r.Context())
		if err != nil {
			if logger := s.currentLogger(); logger != nil {
				logger.Warn("failed to acquire tenant search_path connection", "tenant_id", tenantID, "error", err)
			}
			httputil.WriteError(w, http.StatusServiceUnavailable, "tenant schema isolation unavailable")
			return
		}

		schemaName := pgx.Identifier{tenantInfo.Slug}.Sanitize()
		searchPathSQL := fmt.Sprintf(`SET search_path TO %s, public`, schemaName)
		if _, err := conn.Exec(r.Context(), searchPathSQL); err != nil {
			if logger := s.currentLogger(); logger != nil {
				logger.Warn("failed to set tenant search_path", "tenant_id", tenantID, "slug", tenantInfo.Slug, "error", err)
			}
			destroyCtx, destroyCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer destroyCancel()
			if destroyErr := conn.Destroy(destroyCtx); destroyErr != nil {
				if logger := s.currentLogger(); logger != nil {
					logger.Warn("failed to destroy connection after search_path set failure", "tenant_id", tenantID, "error", destroyErr)
				}
			}
			httputil.WriteError(w, http.StatusServiceUnavailable, "tenant schema isolation unavailable")
			return
		}

		ctx := tenant.ContextWithRequestConn(r.Context(), conn)
		defer func() {
			resetCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := conn.Exec(resetCtx, `SET search_path TO public`); err != nil {
				if logger := s.currentLogger(); logger != nil {
					logger.Warn("failed to reset tenant search_path", "tenant_id", tenantID, "slug", tenantInfo.Slug, "error", err)
				}
				destroyCtx, destroyCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer destroyCancel()
				if err := conn.Destroy(destroyCtx); err != nil {
					if logger := s.currentLogger(); logger != nil {
						logger.Warn("failed to destroy tainted tenant search_path connection", "tenant_id", tenantID, "slug", tenantInfo.Slug, "error", err)
					}
				}
				return
			}
			conn.Release()
		}()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// recordBreakerOutcome is middleware that uses HTTP response status codes to transition a circuit breaker between open and closed states, emitting audit events on state changes.
func (s *Server) recordBreakerOutcome(next http.Handler) http.Handler {
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

		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rec, r)

		if s.tenantBreakerTracker != nil {
			if rec.statusCode >= 500 {
				prevState, newState := s.tenantBreakerTracker.RecordFailure(tenantID)
				if prevState != tenant.BreakerStateOpen && newState == tenant.BreakerStateOpen && s.auditEmitter != nil {
					snap := s.tenantBreakerTracker.StateSnapshot(tenantID)
					s.auditEmitter.EmitBreakerOpened(r.Context(), tenantID, snap.ConsecutiveFailures)
				}
			} else if rec.statusCode < 400 {
				prevState, _ := s.tenantBreakerTracker.RecordSuccess(tenantID)
				if prevState != tenant.BreakerStateClosed && s.auditEmitter != nil {
					s.auditEmitter.EmitBreakerClosed(r.Context(), tenantID)
				}
			}
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.statusCode = code
	rec.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the underlying ResponseWriter for Go 1.20+ middleware chains.
func (rec *statusRecorder) Unwrap() http.ResponseWriter {
	return rec.ResponseWriter
}

// Flush forwards to the underlying writer so SSE streaming works through
// the recordBreakerOutcome middleware. Without this, the http.Flusher type
// assertion in the realtime SSE handler fails.
func (rec *statusRecorder) Flush() {
	if f, ok := rec.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack forwards to the underlying writer so WebSocket upgrades work through
// the recordBreakerOutcome middleware. Without this, gorilla/websocket's
// Upgrader.Upgrade fails on the http.Hijacker type assertion.
func (rec *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rec.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("hijack not supported")
}
