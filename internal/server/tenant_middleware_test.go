package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func signedTenantTestToken(t *testing.T, secret string, claims *auth.Claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	testutil.NoError(t, err)
	return signed
}

func TestResolveTenantContextFromClaims(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.resolveTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "t1", tenant.TenantFromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{TenantID: "t1"}))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func reqWithAuthenticatedHeaderTenant(path, tenantID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if tenantID != "" {
		req.Header.Set("X-Tenant-ID", tenantID)
	}
	return req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{}))
}

func TestResolveTenantContextFromHeader(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.resolveTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "t2", tenant.TenantFromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	req := reqWithAuthenticatedHeaderTenant("/posts", "t2")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestResolveTenantContextFromURLParam(t *testing.T) {
	t.Parallel()

	s := &Server{}
	r := chi.NewRouter()
	h := s.resolveTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "t3", tenant.TenantFromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))
	r.Get("/tenant/{tenantId}/apps", h.ServeHTTP)

	req := httptest.NewRequest(http.MethodGet, "/tenant/t3/apps", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestResolveTenantContextMissing(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.resolveTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "", tenant.TenantFromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestResolveTenantContextClaimWinsOverHeader(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.resolveTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "from-jwt", tenant.TenantFromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	req.Header.Set("X-Tenant-ID", "from-header")
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{TenantID: "from-jwt"}))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestResolveTenantContextNoClaimsIgnoresHeader(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.resolveTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "", tenant.TenantFromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	req.Header.Set("X-Tenant-ID", "fallback")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestResolveTenantContextNoClaimsAPIHeaderFallback(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
	}{
		{name: "admin status route", path: "/api/admin/status"},
		{name: "realtime sse route", path: "/api/realtime"},
		{name: "realtime websocket route", path: "/api/realtime/ws"},
		{name: "storage upload route", path: "/api/storage/uploads"},
		{name: "storage resumable route", path: "/api/storage/upload/resumable"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &Server{}
			h := s.resolveTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.Equal(t, "api-fallback", tenant.TenantFromContext(r.Context()))
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("X-Tenant-ID", "api-fallback")
			w := httptest.NewRecorder()

			h.ServeHTTP(w, req)
			testutil.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestResolveTenantContextNoClaimsHeaderIgnoredForNonAllowlistedAPIPath(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.resolveTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "", tenant.TenantFromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/collections/posts", nil)
	req.Header.Set("X-Tenant-ID", "api-fallback")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestResolveTenantContextAuthenticatedRequestFallsBackToHeader(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.resolveTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "fallback", tenant.TenantFromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	req := reqWithAuthenticatedHeaderTenant("/posts", "fallback")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestWithTenantScopedAdminOrUserAuthWithoutTenantWiringAllowsUserJWT(t *testing.T) {
	t.Parallel()

	const secret = "tenant-scoped-guard-skip-secret"
	authSvc := auth.NewService(nil, secret, time.Hour, 24*time.Hour, 8, testutil.DiscardLogger())
	s := &Server{authSvc: authSvc}

	router := chi.NewRouter()
	s.withTenantScopedAdminOrUserAuth(router, func(r chi.Router) {
		r.Get("/api/collections/posts", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	token := signedTenantTestToken(t, secret, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-tenantless",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
		Email: "tenantless@example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/collections/posts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestWithTenantScopedAdminOrUserAuthPartialTenantWiringEnforcesTenantContext(t *testing.T) {
	t.Parallel()

	const secret = "tenant-scoped-partial-wiring-secret"
	authSvc := auth.NewService(nil, secret, time.Hour, 24*time.Hour, 8, testutil.DiscardLogger())

	testCases := []struct {
		name   string
		server *Server
	}{
		{
			name: "tenant service only",
			server: &Server{
				authSvc:   authSvc,
				tenantSvc: &mockTenantService{tenant: &tenant.Tenant{ID: "tenant-1", State: tenant.TenantStateActive}},
			},
		},
		{
			name: "permission resolver only",
			server: &Server{
				authSvc: authSvc,
				permResolver: tenant.NewPermissionResolver(
					&permissionResolverTenantSource{tenant: &tenant.Tenant{ID: "tenant-1"}},
					&permissionResolverOrgStore{},
					&permissionResolverTeamMembershipStore{},
					&permissionResolverTeamStore{},
				),
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router := chi.NewRouter()
			tc.server.withTenantScopedAdminOrUserAuth(router, func(r chi.Router) {
				r.Get("/api/collections/posts", func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				})
			})

			token := signedTenantTestToken(t, secret, &auth.Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject:   "user-no-tenant",
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
				},
				Email: "tenantless@example.com",
			})

			req := httptest.NewRequest(http.MethodGet, "/api/collections/posts", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)
			testutil.Equal(t, http.StatusForbidden, w.Code)
		})
	}
}

func TestResolveTenantContextFromBearerToken(t *testing.T) {
	t.Parallel()

	const secret = "tenant-context-test-secret"
	authSvc := auth.NewService(nil, secret, time.Hour, 24*time.Hour, 8, testutil.DiscardLogger())
	s := &Server{authSvc: authSvc}
	h := s.resolveTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "tenant-from-token", tenant.TenantFromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	token := signedTenantTestToken(t, secret, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
		Email:    "tenant@example.com",
		TenantID: "tenant-from-token",
	})

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestResolveTenantContextAdminTokenHeaderFallback(t *testing.T) {
	t.Parallel()

	adminAuth := newAdminAuth("test-password")
	s := &Server{adminAuth: adminAuth}
	h := s.resolveTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "admin-tenant", tenant.TenantFromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	req.Header.Set("Authorization", "Bearer "+adminAuth.token())
	req.Header.Set("X-Tenant-ID", "admin-tenant")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestTenantIDFromContextOrRequestAPIHeaderFallbackWithoutAuth(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	req.Header.Set("X-Tenant-ID", "tenant-log")

	testutil.Equal(t, "tenant-log", tenantIDFromContextOrRequest(req))
}

func TestTenantIDFromContextOrRequestNonAllowlistedAPIHeaderIgnoredWithoutAuth(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/collections/posts", nil)
	req.Header.Set("X-Tenant-ID", "tenant-log")

	testutil.Equal(t, "", tenantIDFromContextOrRequest(req))
}

func TestTenantIDFromContextOrRequestNonAPIHeaderIgnoredWithoutAuth(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	req.Header.Set("X-Tenant-ID", "tenant-log")

	testutil.Equal(t, "", tenantIDFromContextOrRequest(req))
}

func TestRequireTenantContextMissingContext(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.requireTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)

	body := decodeErrorBody(t, w)
	testutil.Equal(t, http.StatusBadRequest, body.Code)
	testutil.Equal(t, "tenant context required", body.Message)
}

func TestRequireTenantContextValidTenant(t *testing.T) {
	t.Parallel()

	t.Run("claims", func(t *testing.T) {
		t.Parallel()
		s := &Server{
			tenantSvc: &mockTenantService{tenant: &tenant.Tenant{State: tenant.TenantStateActive}},
		}
		h := s.requireTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			testutil.Equal(t, "t1", tenant.TenantFromContext(r.Context()))
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/posts", nil)
		req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{TenantID: "t1"}))
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("header", func(t *testing.T) {
		t.Parallel()
		s := &Server{
			tenantSvc: &mockTenantService{tenant: &tenant.Tenant{State: tenant.TenantStateActive}},
		}
		h := s.requireTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			testutil.Equal(t, "t1", tenant.TenantFromContext(r.Context()))
			w.WriteHeader(http.StatusOK)
		}))

		req := reqWithAuthenticatedHeaderTenant("/posts", "t1")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("url-param", func(t *testing.T) {
		t.Parallel()
		s := &Server{
			tenantSvc: &mockTenantService{tenant: &tenant.Tenant{State: tenant.TenantStateActive}},
		}
		r := chi.NewRouter()
		h := s.requireTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			testutil.Equal(t, "t1", tenant.TenantFromContext(r.Context()))
			w.WriteHeader(http.StatusOK)
		}))
		r.Get("/tenants/{tenantId}/apps", h.ServeHTTP)

		req := httptest.NewRequest(http.MethodGet, "/tenants/t1/apps", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
	})
}

func TestRequireTenantContextNilTenantService(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.requireTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := reqWithAuthenticatedHeaderTenant("/posts", "t1")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)

	body := decodeErrorBody(t, w)
	testutil.Equal(t, http.StatusInternalServerError, body.Code)
	testutil.Equal(t, "tenant service not configured", body.Message)
}

func TestRequireTenantContextAnonymousHeaderRejected(t *testing.T) {
	t.Parallel()

	s := &Server{
		tenantSvc: &mockTenantService{tenant: &tenant.Tenant{State: tenant.TenantStateActive}},
	}
	h := s.requireTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	req.Header.Set("X-Tenant-ID", "t1")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)

	body := decodeErrorBody(t, w)
	testutil.Equal(t, http.StatusBadRequest, body.Code)
	testutil.Equal(t, "tenant context required", body.Message)
}

func TestRequireTenantContextDeletedTenant(t *testing.T) {
	t.Parallel()

	s := &Server{
		tenantSvc: &mockTenantService{tenant: &tenant.Tenant{State: tenant.TenantStateDeleted}},
	}
	h := s.requireTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := reqWithAuthenticatedHeaderTenant("/posts", "t1")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)

	body := decodeErrorBody(t, w)
	testutil.Equal(t, http.StatusNotFound, body.Code)
	testutil.Equal(t, "tenant not found", body.Message)
}

func TestRequireTenantContextTenantNotFound(t *testing.T) {
	t.Parallel()

	s := &Server{
		tenantSvc: &mockTenantService{err: tenant.ErrTenantNotFound},
	}
	h := s.requireTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := reqWithAuthenticatedHeaderTenant("/posts", "t1")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)

	body := decodeErrorBody(t, w)
	testutil.Equal(t, http.StatusNotFound, body.Code)
	testutil.Equal(t, "tenant not found", body.Message)
}

func TestRequireTenantContextValidateTenantError(t *testing.T) {
	t.Parallel()

	s := &Server{
		tenantSvc: &mockTenantService{err: errors.New("db unavailable")},
	}
	h := s.requireTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := reqWithAuthenticatedHeaderTenant("/posts", "t1")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusInternalServerError, w.Code)

	body := decodeErrorBody(t, w)
	testutil.Equal(t, http.StatusInternalServerError, body.Code)
	testutil.Equal(t, "failed to validate tenant", body.Message)
}

func TestRegisterStorageRoutesResumableOptionsBypassesAuth(t *testing.T) {
	t.Parallel()

	authSvc := auth.NewService(nil, "test-secret", time.Hour, 24*time.Hour, 8, testutil.DiscardLogger())
	s := &Server{
		authSvc:        authSvc,
		storageHandler: &storage.Handler{},
	}

	r := chi.NewRouter()
	s.registerStorageRoutes(r)

	req := httptest.NewRequest(http.MethodOptions, "/storage/upload/resumable", nil)
	req.Header.Set("Tus-Resumable", "1.0.0")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, "1.0.0", w.Header().Get("Tus-Resumable"))
}

type mockTenantService struct {
	tenant      *tenant.Tenant
	memberships []tenant.TenantMembership
	err         error

	// Per-method overrides (non-nil overrides default behavior).
	transitionResult *tenant.Tenant
	transitionErr    error
	updateResult     *tenant.Tenant
	updateErr        error
	listResult       *tenant.TenantListResult
	listErr          error
	removeMemberErr  error
	addMemberErr     error
	updateRoleErr    error

	// Maintenance mode overrides.
	underMaintenance bool
	maintenanceState *tenant.TenantMaintenanceState
	maintenanceErr   error

	deleteTenantSchemaCalls    int
	lastDeletedSchemaSlug      string
	deleteTenantSchemaErr      error
	assignTenantToOrgCalls     int
	lastAssignedTenantID       string
	lastAssignedOrgID          string
	assignTenantToOrgErr       error
	unassignTenantFromOrgCalls int
	lastUnassignedTenantID     string
	lastUnassignedOrgID        string
	unassignTenantFromOrgErr   error
	listOrgTenantsErr          error
	orgTenants                 []tenant.Tenant
}

func (m *mockTenantService) GetTenant(_ context.Context, _ string) (*tenant.Tenant, error) {
	if m == nil {
		return nil, nil
	}
	return m.tenant, m.err
}

func (m *mockTenantService) CreateTenant(_ context.Context, _ string, _ string, _ string, _ string, _ string, _ json.RawMessage, _ string) (*tenant.Tenant, error) {
	if m == nil {
		return nil, nil
	}
	return m.tenant, m.err
}

func (m *mockTenantService) ListTenants(_ context.Context, _ int, _ int) (*tenant.TenantListResult, error) {
	if m == nil {
		return nil, nil
	}
	if m.listErr != nil {
		return nil, m.listErr
	}
	if m.listResult != nil {
		return m.listResult, nil
	}
	return &tenant.TenantListResult{Items: []tenant.Tenant{}}, m.err
}

func (m *mockTenantService) TransitionState(_ context.Context, _ string, _ tenant.TenantState, _ tenant.TenantState) (*tenant.Tenant, error) {
	if m == nil {
		return nil, nil
	}
	if m.transitionErr != nil {
		return nil, m.transitionErr
	}
	if m.transitionResult != nil {
		return m.transitionResult, nil
	}
	return m.tenant, m.err
}

func (m *mockTenantService) UpdateTenant(_ context.Context, _ string, _ string, _ json.RawMessage) (*tenant.Tenant, error) {
	if m == nil {
		return nil, nil
	}
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	if m.updateResult != nil {
		return m.updateResult, nil
	}
	return m.tenant, m.err
}

func (m *mockTenantService) AddMembership(_ context.Context, _ string, _ string, _ string) (*tenant.TenantMembership, error) {
	if m == nil {
		return nil, nil
	}
	if m.addMemberErr != nil {
		return nil, m.addMemberErr
	}
	if len(m.memberships) > 0 {
		return &m.memberships[0], m.err
	}
	return nil, m.err
}

func (m *mockTenantService) RemoveMembership(_ context.Context, _ string, _ string) error {
	if m == nil {
		return nil
	}
	if m.removeMemberErr != nil {
		return m.removeMemberErr
	}
	return m.err
}

func (m *mockTenantService) ListMemberships(_ context.Context, _ string) ([]tenant.TenantMembership, error) {
	if m == nil {
		return nil, nil
	}
	return m.memberships, m.err
}

func (m *mockTenantService) GetMembership(_ context.Context, _ string, _ string) (*tenant.TenantMembership, error) {
	if m == nil {
		return nil, nil
	}
	if len(m.memberships) > 0 {
		return &m.memberships[0], m.err
	}
	return nil, m.err
}

func (m *mockTenantService) UpdateMembershipRole(_ context.Context, _ string, _ string, _ string) (*tenant.TenantMembership, error) {
	if m == nil {
		return nil, nil
	}
	if m.updateRoleErr != nil {
		return nil, m.updateRoleErr
	}
	if len(m.memberships) > 0 {
		return &m.memberships[0], m.err
	}
	return nil, m.err
}

func (m *mockTenantService) InsertAuditEvent(_ context.Context, _ string, _ *string, _ string, _ string, _ json.RawMessage, _ *string) error {
	if m == nil {
		return nil
	}
	return nil
}

func (m *mockTenantService) IsUnderMaintenance(_ context.Context, _ string) (bool, error) {
	if m == nil {
		return false, nil
	}
	if m.maintenanceErr != nil {
		return false, m.maintenanceErr
	}
	return m.underMaintenance, nil
}

func (m *mockTenantService) EnableMaintenance(_ context.Context, _ string, reason string, _ string) (*tenant.TenantMaintenanceState, error) {
	if m == nil {
		return nil, nil
	}
	if m.maintenanceErr != nil {
		return nil, m.maintenanceErr
	}
	return &tenant.TenantMaintenanceState{Enabled: true, Reason: &reason}, m.err
}

func (m *mockTenantService) DisableMaintenance(_ context.Context, _ string, _ string) (*tenant.TenantMaintenanceState, error) {
	if m == nil {
		return nil, nil
	}
	if m.maintenanceErr != nil {
		return nil, m.maintenanceErr
	}
	return &tenant.TenantMaintenanceState{Enabled: false}, m.err
}

func (m *mockTenantService) GetMaintenanceState(_ context.Context, _ string) (*tenant.TenantMaintenanceState, error) {
	if m == nil {
		return nil, nil
	}
	if m.maintenanceErr != nil {
		return nil, m.maintenanceErr
	}
	return m.maintenanceState, nil
}

func (m *mockTenantService) DeleteTenantSchema(_ context.Context, slug string) error {
	if m == nil {
		return nil
	}
	m.deleteTenantSchemaCalls++
	m.lastDeletedSchemaSlug = slug
	if m.deleteTenantSchemaErr != nil {
		return m.deleteTenantSchemaErr
	}
	return nil
}

func (m *mockTenantService) AssignTenantToOrg(_ context.Context, tenantID, orgID string) error {
	if m == nil {
		return nil
	}
	m.assignTenantToOrgCalls++
	m.lastAssignedTenantID = tenantID
	m.lastAssignedOrgID = orgID
	return m.assignTenantToOrgErr
}

func (m *mockTenantService) UnassignTenantFromOrg(_ context.Context, tenantID, orgID string) error {
	if m == nil {
		return nil
	}
	m.unassignTenantFromOrgCalls++
	m.lastUnassignedTenantID = tenantID
	m.lastUnassignedOrgID = orgID
	return m.unassignTenantFromOrgErr
}

func (m *mockTenantService) ListOrgTenants(_ context.Context, _ string) ([]tenant.Tenant, error) {
	if m == nil {
		return nil, nil
	}
	if m.listOrgTenantsErr != nil {
		return nil, m.listOrgTenantsErr
	}
	if m.orgTenants == nil {
		return []tenant.Tenant{}, nil
	}
	return m.orgTenants, nil
}

type fakeTenantConn struct {
	execCalls    int
	releaseCalls int
	destroyCalls int
	execSQLs     []string
	execErr      error
	execErrs     map[string]error
	destroyErr   error
}

type fakeTenantRow struct{}

func (fakeTenantRow) Scan(...any) error {
	return errors.New("unexpected scan")
}

func (c *fakeTenantConn) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	c.execCalls++
	c.execSQLs = append(c.execSQLs, sql)
	if c.execErrs != nil {
		if err, ok := c.execErrs[sql]; ok {
			return pgconn.NewCommandTag("SET"), err
		}
	}
	return pgconn.NewCommandTag("SET"), c.execErr
}

func (c *fakeTenantConn) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected query")
}

func (c *fakeTenantConn) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return fakeTenantRow{}
}

func (c *fakeTenantConn) Begin(_ context.Context) (pgx.Tx, error) {
	return nil, errors.New("unexpected begin")
}

func (c *fakeTenantConn) Release() {
	c.releaseCalls++
}

func (c *fakeTenantConn) Destroy(_ context.Context) error {
	c.destroyCalls++
	return c.destroyErr
}

// --- enforceTenantContext tests ---

func TestEnforceTenantContext_Missing(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.enforceTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)

	body := decodeErrorBody(t, w)
	testutil.Equal(t, http.StatusForbidden, body.Code)
	testutil.Equal(t, "tenant context required", body.Message)
}

func TestEnforceTenantContext_Present(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.enforceTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "t1")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceTenantContext_AdminTokenAllowedWithoutTenant(t *testing.T) {
	t.Parallel()

	adminAuth := newAdminAuth("test-password")
	s := &Server{adminAuth: adminAuth}
	h := s.enforceTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	req.Header.Set("Authorization", "Bearer "+adminAuth.token())
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceTenantContext_TenantlessUserClaimsDenied(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.enforceTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user-1"},
		Email:            "legacy@example.com",
	}))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)

	body := decodeErrorBody(t, w)
	testutil.Equal(t, "tenant context required", body.Message)
}

func TestEnforceTenantContext_OrgScopedClaimsStillRequireTenant(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.enforceTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user-1"},
		Email:            "org@example.com",
		OrgID:            "org-1",
	}))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)

	body := decodeErrorBody(t, w)
	testutil.Equal(t, "tenant context required", body.Message)
}

func TestEnforceTenantContext_TenantlessAPIKeyClaimsDenied(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.enforceTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		APIKeyID: "api-key-1",
		AppID:    "app-1",
	}))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)

	body := decodeErrorBody(t, w)
	testutil.Equal(t, "tenant context required", body.Message)
}

func TestSetTenantSearchPathSchemaModeSetsSearchPath(t *testing.T) {
	t.Parallel()

	conn := &fakeTenantConn{}
	s := &Server{
		tenantSvc: &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", Slug: "tenant-a", IsolationMode: "schema"},
		},
		tenantConnAcquire: func(_ context.Context) (tenantSearchPathConn, error) {
			return conn, nil
		},
	}

	h := s.setTenantSearchPath(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.True(t, tenant.RequestConnFromContext(r.Context()) != nil, "expected request connection in context")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req = req.WithContext(tenant.ContextWithTenantID(req.Context(), "t1"))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, 2, conn.execCalls)
	testutil.SliceLen(t, conn.execSQLs, 2)
	testutil.Contains(t, conn.execSQLs[0], `SET search_path TO "tenant-a", public`)
	testutil.Equal(t, `SET search_path TO public`, conn.execSQLs[1])
	testutil.Equal(t, 1, conn.releaseCalls)
}

func TestSetTenantSearchPathSharedModePassThrough(t *testing.T) {
	t.Parallel()

	conn := &fakeTenantConn{}
	s := &Server{
		tenantSvc: &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", Slug: "tenant-a", IsolationMode: "shared"},
		},
		tenantConnAcquire: func(_ context.Context) (tenantSearchPathConn, error) {
			return conn, nil
		},
	}

	h := s.setTenantSearchPath(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req = req.WithContext(tenant.ContextWithTenantID(req.Context(), "t1"))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, 0, conn.execCalls)
	testutil.Equal(t, 0, conn.releaseCalls)
}

func TestSetTenantSearchPathMissingTenantIDPassThrough(t *testing.T) {
	t.Parallel()

	conn := &fakeTenantConn{}
	s := &Server{
		tenantSvc: &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", Slug: "tenant-a", IsolationMode: "schema"},
		},
		tenantConnAcquire: func(_ context.Context) (tenantSearchPathConn, error) {
			return conn, nil
		},
	}

	h := s.setTenantSearchPath(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusAccepted, w.Code)
	testutil.Equal(t, 0, conn.execCalls)
	testutil.Equal(t, 0, conn.releaseCalls)
}

func TestSetTenantSearchPathTenantLookupErrorReturns503(t *testing.T) {
	t.Parallel()

	conn := &fakeTenantConn{}
	s := &Server{
		tenantSvc: &mockTenantService{
			err: errors.New("lookup failed"),
		},
		tenantConnAcquire: func(_ context.Context) (tenantSearchPathConn, error) {
			return conn, nil
		},
	}

	nextCalled := false
	h := s.setTenantSearchPath(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req = req.WithContext(tenant.ContextWithTenantID(req.Context(), "t1"))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	testutil.False(t, nextCalled, "next handler should not be called when tenant lookup fails for schema isolation")
	testutil.Equal(t, 0, conn.execCalls)
	testutil.Equal(t, 0, conn.releaseCalls)
}

func TestSetTenantSearchPathNilTenantLookupReturns503(t *testing.T) {
	t.Parallel()

	conn := &fakeTenantConn{}
	s := &Server{
		tenantSvc: &mockTenantService{},
		tenantConnAcquire: func(_ context.Context) (tenantSearchPathConn, error) {
			return conn, nil
		},
	}

	nextCalled := false
	h := s.setTenantSearchPath(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req = req.WithContext(tenant.ContextWithTenantID(req.Context(), "t1"))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	testutil.False(t, nextCalled, "next handler should not be called when tenant lookup returns nil for schema isolation")
	testutil.Equal(t, 0, conn.execCalls)
	testutil.Equal(t, 0, conn.releaseCalls)
}

func TestSetTenantSearchPathResetFailureDestroysConn(t *testing.T) {
	t.Parallel()

	conn := &fakeTenantConn{
		execErrs: map[string]error{
			`SET search_path TO public`: errors.New("reset failed"),
		},
	}
	s := &Server{
		tenantSvc: &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", Slug: "tenant-a", IsolationMode: "schema"},
		},
		tenantConnAcquire: func(_ context.Context) (tenantSearchPathConn, error) {
			return conn, nil
		},
	}

	h := s.setTenantSearchPath(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.True(t, tenant.RequestConnFromContext(r.Context()) != nil, "expected request connection in context")
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req = req.WithContext(tenant.ContextWithTenantID(req.Context(), "t1"))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, 2, conn.execCalls)
	testutil.Equal(t, 0, conn.releaseCalls)
	testutil.Equal(t, 1, conn.destroyCalls)
}

func TestSetTenantSearchPathConnAcquireFailureReturns503(t *testing.T) {
	t.Parallel()

	s := &Server{
		tenantSvc: &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", Slug: "tenant-a", IsolationMode: "schema"},
		},
		tenantConnAcquire: func(_ context.Context) (tenantSearchPathConn, error) {
			return nil, errors.New("pool exhausted")
		},
	}

	nextCalled := false
	h := s.setTenantSearchPath(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req = req.WithContext(tenant.ContextWithTenantID(req.Context(), "t1"))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	testutil.False(t, nextCalled, "next handler should not be called when conn acquisition fails for schema-isolated tenant")
	body := decodeErrorBody(t, w)
	testutil.Equal(t, http.StatusServiceUnavailable, body.Code)
	testutil.Equal(t, "tenant schema isolation unavailable", body.Message)
}

func TestSetTenantSearchPathSetFailureReturns503(t *testing.T) {
	t.Parallel()

	conn := &fakeTenantConn{
		execErrs: map[string]error{
			`SET search_path TO "tenant-a", public`: errors.New("set search_path failed"),
		},
	}
	s := &Server{
		tenantSvc: &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", Slug: "tenant-a", IsolationMode: "schema"},
		},
		tenantConnAcquire: func(_ context.Context) (tenantSearchPathConn, error) {
			return conn, nil
		},
	}

	nextCalled := false
	h := s.setTenantSearchPath(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req = req.WithContext(tenant.ContextWithTenantID(req.Context(), "t1"))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	testutil.False(t, nextCalled, "next handler should not be called when SET search_path fails for schema-isolated tenant")
	testutil.Equal(t, 0, conn.releaseCalls)
	testutil.Equal(t, 1, conn.destroyCalls)
	body := decodeErrorBody(t, w)
	testutil.Equal(t, http.StatusServiceUnavailable, body.Code)
	testutil.Equal(t, "tenant schema isolation unavailable", body.Message)
}

// --- enforceTenantMatch tests ---

func TestEnforceTenantMatch_Matching(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.enforceTenantMatch(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "t1")
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{TenantID: "t1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceTenantMatch_Mismatch(t *testing.T) {
	t.Parallel()

	s := &Server{}
	h := s.enforceTenantMatch(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "t1")
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{TenantID: "t2"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)

	body := decodeErrorBody(t, w)
	testutil.Equal(t, http.StatusForbidden, body.Code)
	testutil.Equal(t, "tenant mismatch", body.Message)
}

func TestEnforceTenantMatch_NoClaims(t *testing.T) {
	t.Parallel()

	// No JWT claims (e.g. admin token path) — should pass.
	s := &Server{}
	h := s.enforceTenantMatch(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "t1")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceTenantMatch_EmptyClaimTenantID(t *testing.T) {
	t.Parallel()

	// JWT present but TenantID is empty (legacy pre-migration token) — should pass.
	s := &Server{}
	h := s.enforceTenantMatch(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "t1")
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{TenantID: ""})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceTenantMatch_WhitespaceClaimTenantID(t *testing.T) {
	t.Parallel()

	// JWT present but TenantID is only whitespace — should be treated as empty
	// (legacy pre-migration token) and pass through.
	s := &Server{}
	h := s.enforceTenantMatch(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/posts", nil)
	ctx := tenant.ContextWithTenantID(req.Context(), "t1")
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{TenantID: "   "})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

type noopAuditInserter struct{}

func (n *noopAuditInserter) InsertAuditEvent(_ context.Context, _ string, _ *string, _ string, _ string, _ json.RawMessage, _ *string) error {
	return nil
}

type trackingAuditInserter struct {
	mu      sync.Mutex
	actions []string
}

func (t *trackingAuditInserter) InsertAuditEvent(_ context.Context, _ string, _ *string, action string, _ string, _ json.RawMessage, _ *string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.actions = append(t.actions, action)
	return nil
}

func (t *trackingAuditInserter) getActions() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.actions))
	copy(out, t.actions)
	return out
}

func TestSetTenantService_AutoWiresAuditEmitter(t *testing.T) {
	t.Parallel()

	s := &Server{}
	s.SetTenantService(&mockTenantService{})

	testutil.True(t, s.AuditEmitter() != nil, "expected default audit emitter when tenant service is set")

	s.SetTenantService(nil)
	testutil.True(t, s.AuditEmitter() == nil, "expected default audit emitter to clear when tenant service is unset")
}

func TestSetTenantService_DoesNotOverrideManualAuditEmitter(t *testing.T) {
	t.Parallel()

	s := &Server{}
	custom := tenant.NewAuditEmitterWithInserter(&noopAuditInserter{}, nil)
	s.SetAuditEmitter(custom)
	s.SetTenantService(&mockTenantService{})

	testutil.True(t, s.AuditEmitter() == custom, "expected manual emitter to be preserved across SetTenantService")
}

// --- enforceTenantAvailability tests ---

func reqWithTenantContext(tenantID string, path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if tenantID != "" {
		ctx := tenant.ContextWithTenantID(req.Context(), tenantID)
		req = req.WithContext(ctx)
	}
	return req
}

func TestEnforceTenantAvailability_HealthyTenant(t *testing.T) {
	t.Parallel()
	s := &Server{tenantSvc: &mockTenantService{}}
	h := s.enforceTenantAvailability(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/posts"))
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceTenantAvailability_NoTenantContext(t *testing.T) {
	t.Parallel()
	s := &Server{tenantSvc: &mockTenantService{underMaintenance: true}}
	h := s.enforceTenantAvailability(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("", "/api/posts"))
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceTenantAvailability_MaintenanceReturns503(t *testing.T) {
	t.Parallel()
	s := &Server{tenantSvc: &mockTenantService{underMaintenance: true}}
	h := s.enforceTenantAvailability(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/posts"))
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	testutil.Equal(t, "60", w.Header().Get("Retry-After"))
	body := decodeErrorBody(t, w)
	testutil.Equal(t, "tenant under maintenance", body.Message)
}

func TestEnforceTenantAvailability_MaintenanceLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()
	s := &Server{tenantSvc: &mockTenantService{maintenanceErr: errors.New("db unavailable")}}
	h := s.enforceTenantAvailability(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/posts"))
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	testutil.Equal(t, "30", w.Header().Get("Retry-After"))
	body := decodeErrorBody(t, w)
	testutil.Equal(t, "tenant availability unknown", body.Message)
}

func TestEnforceTenantAvailability_BreakerOpenReturns503(t *testing.T) {
	t.Parallel()
	tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
		FailureThreshold: 1,
		OpenDuration:     30 * time.Second,
	}, nil)
	tracker.RecordFailure("t1")
	s := &Server{tenantSvc: &mockTenantService{}, tenantBreakerTracker: tracker}
	h := s.enforceTenantAvailability(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/posts"))
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	testutil.True(t, w.Header().Get("Retry-After") != "", "expected Retry-After header")
	body := decodeErrorBody(t, w)
	testutil.Equal(t, "tenant temporarily unavailable", body.Message)
}

func TestEnforceTenantAvailability_BypassesRecoveryEndpoints(t *testing.T) {
	t.Parallel()

	recoveryPaths := []string{
		"/api/admin/tenants/t1/maintenance/enable",
		"/api/admin/tenants/t1/maintenance/disable",
		"/api/admin/tenants/t1/maintenance",
		"/api/admin/tenants/t1/breaker/reset",
		"/api/admin/tenants/t1/breaker",
	}

	for _, path := range recoveryPaths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			s := &Server{tenantSvc: &mockTenantService{underMaintenance: true}}
			h := s.enforceTenantAvailability(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, reqWithTenantContext("t1", path))
			testutil.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestEnforceTenantAvailability_DoesNotBypassNonAdminSuffixMatches(t *testing.T) {
	t.Parallel()

	nonAdminPaths := []string{
		"/api/projects/t1/maintenance",
		"/api/projects/t1/maintenance/enable",
		"/api/projects/t1/breaker",
		"/api/projects/t1/breaker/reset",
	}

	for _, path := range nonAdminPaths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			s := &Server{tenantSvc: &mockTenantService{underMaintenance: true}}
			h := s.enforceTenantAvailability(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, reqWithTenantContext("t1", path))
			testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
		})
	}
}

// --- recordBreakerOutcome tests ---

func TestRecordBreakerOutcome_5xxRecordsFailure(t *testing.T) {
	t.Parallel()
	tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
		FailureThreshold: 2,
		OpenDuration:     30 * time.Second,
	}, nil)
	s := &Server{tenantBreakerTracker: tracker}
	h := s.recordBreakerOutcome(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	// First 5xx — one failure recorded.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/posts"))
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	snap := tracker.StateSnapshot("t1")
	testutil.Equal(t, tenant.BreakerStateClosed, snap.State)
	testutil.Equal(t, 1, snap.ConsecutiveFailures)

	// Second 5xx — threshold reached, breaker opens.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/posts"))
	snap = tracker.StateSnapshot("t1")
	testutil.Equal(t, tenant.BreakerStateOpen, snap.State)
	testutil.Equal(t, 2, snap.ConsecutiveFailures)
}

func TestRecordBreakerOutcome_HealthyRecordsSuccess(t *testing.T) {
	t.Parallel()
	tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
		FailureThreshold: 3,
		OpenDuration:     30 * time.Second,
	}, nil)
	s := &Server{tenantBreakerTracker: tracker}

	// Record one failure, then success resets counter.
	tracker.RecordFailure("t1")
	h := s.recordBreakerOutcome(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/posts"))
	testutil.Equal(t, http.StatusOK, w.Code)
	snap := tracker.StateSnapshot("t1")
	testutil.Equal(t, tenant.BreakerStateClosed, snap.State)
	testutil.Equal(t, 0, snap.ConsecutiveFailures)
}

func TestRecordBreakerOutcome_NoTenantContextPassesThrough(t *testing.T) {
	t.Parallel()
	tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
		FailureThreshold: 1,
		OpenDuration:     30 * time.Second,
	}, nil)
	s := &Server{tenantBreakerTracker: tracker}
	h := s.recordBreakerOutcome(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("", "/api/posts"))
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	// No tenant, so no breaker state should exist.
	snap := tracker.StateSnapshot("no-tenant")
	testutil.Equal(t, tenant.BreakerStateClosed, snap.State)
}

func TestRecordBreakerOutcome_SkipsRecoveryEndpoints(t *testing.T) {
	t.Parallel()
	tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
		FailureThreshold: 1,
		OpenDuration:     30 * time.Second,
	}, nil)
	s := &Server{tenantBreakerTracker: tracker}
	h := s.recordBreakerOutcome(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/admin/tenants/t1/breaker/reset"))
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	// Should NOT have recorded a failure (recovery endpoint exempt).
	snap := tracker.StateSnapshot("t1")
	testutil.Equal(t, tenant.BreakerStateClosed, snap.State)
	testutil.Equal(t, 0, snap.ConsecutiveFailures)
}

func TestRecordBreakerOutcome_DoesNotSkipNonAdminSuffixMatches(t *testing.T) {
	t.Parallel()
	tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
		FailureThreshold: 1,
		OpenDuration:     30 * time.Second,
	}, nil)
	s := &Server{tenantBreakerTracker: tracker}
	h := s.recordBreakerOutcome(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/projects/t1/breaker/reset"))
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	// Non-admin routes must not be exempt from breaker-outcome recording.
	snap := tracker.StateSnapshot("t1")
	testutil.Equal(t, tenant.BreakerStateOpen, snap.State)
	testutil.Equal(t, 1, snap.ConsecutiveFailures)
}

func TestRecordBreakerOutcome_4xxDoesNotMutateBreaker(t *testing.T) {
	t.Parallel()
	tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
		FailureThreshold: 1,
		OpenDuration:     30 * time.Second,
	}, nil)
	s := &Server{tenantBreakerTracker: tracker}
	h := s.recordBreakerOutcome(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/posts"))
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	snap := tracker.StateSnapshot("t1")
	testutil.Equal(t, tenant.BreakerStateClosed, snap.State)
	testutil.Equal(t, 0, snap.ConsecutiveFailures)
}

// --- statusRecorder interface forwarding tests ---

func TestStatusRecorder_ForwardsFlusher(t *testing.T) {
	t.Parallel()
	tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
		FailureThreshold: 5,
		OpenDuration:     30 * time.Second,
	}, nil)
	s := &Server{tenantBreakerTracker: tracker}

	flushed := false
	h := s.recordBreakerOutcome(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher to be available through statusRecorder")
		}
		f.Flush()
		flushed = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/realtime"))
	testutil.True(t, flushed, "handler should have been able to flush through statusRecorder")
}

func TestStatusRecorder_ForwardsHijacker(t *testing.T) {
	t.Parallel()
	tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
		FailureThreshold: 5,
		OpenDuration:     30 * time.Second,
	}, nil)
	s := &Server{tenantBreakerTracker: tracker}

	h := s.recordBreakerOutcome(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// httptest.ResponseRecorder does not implement Hijacker,
		// so Hijack() should return an error — but the type assertion must succeed.
		_, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("expected http.Hijacker to be available through statusRecorder")
		}
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/realtime/ws"))
}

// --- Breaker transition audit emission tests ---

func TestRecordBreakerOutcome_EmitsBreakerOpenedOnTransition(t *testing.T) {
	t.Parallel()
	tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
		FailureThreshold: 2,
		OpenDuration:     30 * time.Second,
	}, nil)
	inserter := &trackingAuditInserter{}
	emitter := tenant.NewAuditEmitterWithInserter(inserter, nil)
	s := &Server{tenantBreakerTracker: tracker, auditEmitter: emitter}

	h := s.recordBreakerOutcome(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	// First 5xx — no audit yet (still closed).
	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/posts"))
	testutil.Equal(t, 0, len(inserter.getActions()))

	// Second 5xx — threshold reached, breaker opens, audit emitted.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/posts"))
	actions := inserter.getActions()
	testutil.Equal(t, 1, len(actions))
	testutil.Equal(t, tenant.AuditActionBreakerOpened, actions[0])
}

func TestRecordBreakerOutcome_EmitsBreakerClosedOnRecovery(t *testing.T) {
	t.Parallel()
	now := time.Now()
	tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
		FailureThreshold:    1,
		OpenDuration:        50 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	}, func() time.Time { return now })
	inserter := &trackingAuditInserter{}
	emitter := tenant.NewAuditEmitterWithInserter(inserter, nil)
	s := &Server{tenantBreakerTracker: tracker, auditEmitter: emitter}

	// Drive breaker open.
	tracker.RecordFailure("t1")
	testutil.Equal(t, tenant.BreakerStateOpen, tracker.State("t1"))

	// Advance past cooldown to allow half_open probe.
	now = now.Add(100 * time.Millisecond)

	// Allow a probe so half_open transitions.
	err := tracker.Allow("t1")
	testutil.True(t, err == nil, "expected allow in half_open")

	// Clear any audit events from the failure above.
	inserter.mu.Lock()
	inserter.actions = nil
	inserter.mu.Unlock()

	// Successful response should close breaker and emit audit.
	h := s.recordBreakerOutcome(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, reqWithTenantContext("t1", "/api/posts"))

	actions := inserter.getActions()
	testutil.Equal(t, 1, len(actions))
	testutil.Equal(t, tenant.AuditActionBreakerClosed, actions[0])
}
