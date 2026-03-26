package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

type tenantLookupTrackingService struct {
	*mockTenantService
	getTenantCalls int
}

func (m *tenantLookupTrackingService) GetTenant(ctx context.Context, id string) (*tenant.Tenant, error) {
	m.getTenantCalls++
	return m.mockTenantService.GetTenant(ctx, id)
}

func withURLParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	return req.WithContext(ctx)
}

type tenantCreateTrackingService struct {
	*mockTenantService
	createTenantCalls  int
	addMembershipCalls int
	lastTenantID       string
	lastUserID         string
	lastRole           string
	lastIsolationMode  string
	lastPlanTier       string
	lastIdempotencyKey string
	createTenantErr    error
	addMembershipErr   error
}

func (m *tenantCreateTrackingService) CreateTenant(ctx context.Context, name, slug, isolationMode, planTier, region string, orgMetadata json.RawMessage, idempotencyKey string) (*tenant.Tenant, error) {
	m.createTenantCalls++
	m.lastIsolationMode = isolationMode
	m.lastPlanTier = planTier
	m.lastIdempotencyKey = idempotencyKey
	if m.createTenantErr != nil {
		return nil, m.createTenantErr
	}
	return m.mockTenantService.CreateTenant(ctx, name, slug, isolationMode, planTier, region, orgMetadata, idempotencyKey)
}

func (m *tenantCreateTrackingService) AddMembership(ctx context.Context, tenantID, userID, role string) (*tenant.TenantMembership, error) {
	m.addMembershipCalls++
	m.lastTenantID = tenantID
	m.lastUserID = userID
	m.lastRole = role
	if m.addMembershipErr != nil {
		return nil, m.addMembershipErr
	}
	return m.mockTenantService.AddMembership(ctx, tenantID, userID, role)
}

// --- handleAdminListTenants tests ---

func TestHandleAdminListTenants(t *testing.T) {
	t.Parallel()

	t.Run("happy path returns list", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			listResult: &tenant.TenantListResult{
				Items:      []tenant.Tenant{{ID: "t1", Name: "Acme", Slug: "acme"}},
				Page:       1,
				PerPage:    20,
				TotalItems: 1,
				TotalPages: 1,
			},
		}
		h := handleAdminListTenants(svc)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants?page=1&perPage=20", nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var result tenant.TenantListResult
		testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
		testutil.Equal(t, 1, len(result.Items))
		testutil.Equal(t, "t1", result.Items[0].ID)
	})

	t.Run("empty list", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{}
		h := handleAdminListTenants(svc)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants", nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var result tenant.TenantListResult
		testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
		testutil.Equal(t, 0, len(result.Items))
	})

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{listErr: errors.New("db down")}
		h := handleAdminListTenants(svc)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants", nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodeErrorBody(t, w)
		testutil.Equal(t, "failed to list tenants", body.Message)
	})
}

// --- handleAdminCreateTenant tests ---

func TestHandleAdminCreateTenant(t *testing.T) {
	t.Parallel()
	validUUID := "00000000-0000-0000-0000-000000000011"

	t.Run("success creates tenant and owner membership", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{
				tenant: &tenant.Tenant{ID: "tenant-1", State: tenant.TenantStateActive},
				memberships: []tenant.TenantMembership{{
					TenantID: "tenant-1",
					UserID:   validUUID,
					Role:     tenant.MemberRoleOwner,
				}},
			},
		}

		h := handleAdminCreateTenant(svc, nil)
		body := `{"name":"Acme","slug":"acme","ownerUserId":"` + validUUID + `","planTier":"pro"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusCreated, w.Code)
		testutil.Equal(t, 1, svc.createTenantCalls)
		testutil.Equal(t, 1, svc.addMembershipCalls)
		testutil.Equal(t, "tenant-1", svc.lastTenantID)
		testutil.Equal(t, validUUID, svc.lastUserID)
		testutil.Equal(t, tenant.MemberRoleOwner, svc.lastRole)
	})

	t.Run("missing name returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{tenant: &tenant.Tenant{ID: "t1"}},
		}
		h := handleAdminCreateTenant(svc, nil)
		body := `{"slug":"acme","ownerUserId":"` + validUUID + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		testutil.Equal(t, 0, svc.createTenantCalls)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "name is required", errBody.Message)
	})

	t.Run("missing slug returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{tenant: &tenant.Tenant{ID: "t1"}},
		}
		h := handleAdminCreateTenant(svc, nil)
		body := `{"name":"Acme","ownerUserId":"` + validUUID + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		testutil.Equal(t, 0, svc.createTenantCalls)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "slug is required", errBody.Message)
	})

	t.Run("invalid slug format returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{tenant: &tenant.Tenant{ID: "t1"}},
		}
		h := handleAdminCreateTenant(svc, nil)
		body := `{"name":"Acme","slug":"AB!!","ownerUserId":"` + validUUID + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		testutil.Equal(t, 0, svc.createTenantCalls)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "invalid slug format", errBody.Message)
	})

	t.Run("invalid isolationMode returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{tenant: &tenant.Tenant{ID: "t1"}},
		}
		h := handleAdminCreateTenant(svc, nil)
		body := `{"name":"Acme","slug":"acme","ownerUserId":"` + validUUID + `","isolationMode":"invalid"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		testutil.Equal(t, 0, svc.createTenantCalls)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "invalid isolationMode", errBody.Message)
	})

	t.Run("database isolationMode maps to shared", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{tenant: &tenant.Tenant{ID: "t1"}},
		}
		h := handleAdminCreateTenant(svc, nil)
		body := `{"name":"Acme","slug":"acme","ownerUserId":"` + validUUID + `","isolationMode":"database"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusCreated, w.Code)
		testutil.Equal(t, "shared", svc.lastIsolationMode)
	})

	t.Run("invalid planTier returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{tenant: &tenant.Tenant{ID: "t1"}},
		}
		h := handleAdminCreateTenant(svc, nil)
		body := `{"name":"Acme","slug":"acme","ownerUserId":"` + validUUID + `","planTier":"platinum"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		testutil.Equal(t, 0, svc.createTenantCalls)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "invalid planTier", errBody.Message)
	})

	t.Run("starter planTier creates tenant successfully", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{
				tenant: &tenant.Tenant{ID: "tenant-starter", State: tenant.TenantStateActive},
				memberships: []tenant.TenantMembership{{
					TenantID: "tenant-starter",
					UserID:   validUUID,
					Role:     tenant.MemberRoleOwner,
				}},
			},
		}
		h := handleAdminCreateTenant(svc, nil)
		body := `{"name":"Starter Co","slug":"starter-co","ownerUserId":"` + validUUID + `","planTier":"starter"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusCreated, w.Code)
		testutil.Equal(t, 1, svc.createTenantCalls)
		testutil.Equal(t, 1, svc.addMembershipCalls)
		testutil.Equal(t, string(billing.PlanStarter), svc.lastPlanTier)
	})

	t.Run("ownerless create succeeds without membership", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{
				tenant: &tenant.Tenant{ID: "tenant-ownerless", Slug: "acme", State: tenant.TenantStateProvisioning},
			},
		}
		h := handleAdminCreateTenant(svc, nil)
		body := `{"name":"Acme","slug":"acme","ownerUserId":"","planTier":"pro"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusCreated, w.Code)
		testutil.Equal(t, 1, svc.createTenantCalls)
		testutil.Equal(t, 0, svc.addMembershipCalls)

		var resp tenant.Tenant
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		testutil.Equal(t, "tenant-ownerless", resp.ID)
		testutil.Equal(t, "acme", resp.Slug)
		testutil.Equal(t, tenant.TenantStateProvisioning, resp.State)
	})

	t.Run("invalid ownerUserId format returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{tenant: &tenant.Tenant{ID: "t1"}},
		}
		h := handleAdminCreateTenant(svc, nil)
		body := `{"name":"Acme","slug":"acme","ownerUserId":"not-a-uuid","planTier":"pro"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		testutil.Equal(t, 0, svc.createTenantCalls)
		testutil.Equal(t, 0, svc.addMembershipCalls)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "invalid ownerUserId format", errBody.Message)
	})

	t.Run("slug taken returns 409", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{tenant: &tenant.Tenant{ID: "t1"}},
			createTenantErr:   tenant.ErrTenantSlugTaken,
		}
		h := handleAdminCreateTenant(svc, nil)
		body := `{"name":"Acme","slug":"acme","ownerUserId":"` + validUUID + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusConflict, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "tenant slug is already taken", errBody.Message)
	})

	t.Run("add membership failure returns 500", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{tenant: &tenant.Tenant{ID: "tenant-1"}},
			addMembershipErr:  errors.New("db unavailable"),
		}
		h := handleAdminCreateTenant(svc, nil)
		body := `{"name":"Acme","slug":"acme","ownerUserId":"` + validUUID + `","planTier":"pro"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusInternalServerError, w.Code)
		testutil.Equal(t, 1, svc.createTenantCalls)
		testutil.Equal(t, 1, svc.addMembershipCalls)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "failed to create owner membership", errBody.Message)
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{tenant: &tenant.Tenant{ID: "t1"}}
		h := handleAdminCreateTenant(svc, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader("{invalid"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "invalid JSON body", errBody.Message)
	})

	t.Run("idempotency key from header is passed through", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{
				tenant: &tenant.Tenant{ID: "tenant-1", State: tenant.TenantStateActive},
				memberships: []tenant.TenantMembership{{
					TenantID: "tenant-1",
					UserID:   validUUID,
					Role:     tenant.MemberRoleOwner,
				}},
			},
		}

		h := handleAdminCreateTenant(svc, nil)
		body := `{"name":"Acme","slug":"acme","ownerUserId":"` + validUUID + `","planTier":"pro"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "idem-header-1")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusCreated, w.Code)
		testutil.Equal(t, "idem-header-1", svc.lastIdempotencyKey)
	})

	t.Run("idempotent create with existing membership succeeds", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{
				tenant: &tenant.Tenant{ID: "tenant-1", State: tenant.TenantStateActive},
			},
			// Simulate the idempotent path: tenant already exists, membership already exists.
			addMembershipErr: tenant.ErrMembershipExists,
		}

		h := handleAdminCreateTenant(svc, nil)
		body := `{"name":"Acme","slug":"acme","ownerUserId":"` + validUUID + `","planTier":"pro","idempotencyKey":"idem-dup"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusCreated, w.Code)
		testutil.Equal(t, 1, svc.createTenantCalls)
		testutil.Equal(t, 1, svc.addMembershipCalls)
	})

	t.Run("body idempotency key overrides header", func(t *testing.T) {
		t.Parallel()
		svc := &tenantCreateTrackingService{
			mockTenantService: &mockTenantService{
				tenant: &tenant.Tenant{ID: "tenant-1", State: tenant.TenantStateActive},
				memberships: []tenant.TenantMembership{{
					TenantID: "tenant-1",
					UserID:   validUUID,
					Role:     tenant.MemberRoleOwner,
				}},
			},
		}

		h := handleAdminCreateTenant(svc, nil)
		body := `{"name":"Acme","slug":"acme","ownerUserId":"` + validUUID + `","planTier":"pro","idempotencyKey":"idem-body-1"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "idem-header-1")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusCreated, w.Code)
		testutil.Equal(t, "idem-body-1", svc.lastIdempotencyKey)
	})
}

func TestIsValidIsolationMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode string
		want bool
	}{
		{name: "empty", mode: "", want: true},
		{name: "shared", mode: "shared", want: true},
		{name: "schema", mode: "schema", want: true},
		{name: "database transition mode", mode: "database", want: true},
		{name: "invalid", mode: "invalid", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.want, isValidIsolationMode(tt.mode))
		})
	}
}

func TestIsValidPlanTier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tier string
		want bool
	}{
		{name: "empty", tier: "", want: true},
		{name: "free", tier: string(billing.PlanFree), want: true},
		{name: "starter", tier: string(billing.PlanStarter), want: true},
		{name: "pro", tier: string(billing.PlanPro), want: true},
		{name: "enterprise", tier: string(billing.PlanEnterprise), want: true},
		{name: "invalid", tier: "platinum", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.want, isValidPlanTier(tt.tier))
		})
	}
}

// --- handleAdminGetTenant tests ---

func TestHandleAdminGetTenant(t *testing.T) {
	t.Parallel()

	t.Run("success returns tenant", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", Name: "Acme", Slug: "acme", State: tenant.TenantStateActive},
		}
		h := handleAdminGetTenant(svc)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/t1", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var result tenant.Tenant
		testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
		testutil.Equal(t, "t1", result.ID)
		testutil.Equal(t, "Acme", result.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{err: tenant.ErrTenantNotFound}
		h := handleAdminGetTenant(svc)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/missing", nil)
		req = withURLParams(req, map[string]string{"tenantId": "missing"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusNotFound, w.Code)
		body := decodeErrorBody(t, w)
		testutil.Equal(t, "tenant not found", body.Message)
	})

	t.Run("missing tenantId returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{tenant: &tenant.Tenant{ID: "t1"}}
		h := handleAdminGetTenant(svc)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/", nil)
		req = withURLParams(req, map[string]string{"tenantId": ""})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		body := decodeErrorBody(t, w)
		testutil.Equal(t, "tenant id is required", body.Message)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{err: errors.New("db down")}
		h := handleAdminGetTenant(svc)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/t1", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodeErrorBody(t, w)
		testutil.Equal(t, "failed to get tenant", body.Message)
	})
}

// --- handleAdminUpdateTenant tests ---

func TestHandleAdminUpdateTenant(t *testing.T) {
	t.Parallel()

	t.Run("success updates name", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:       &tenant.Tenant{ID: "t1", Name: "Old", State: tenant.TenantStateActive},
			updateResult: &tenant.Tenant{ID: "t1", Name: "New", State: tenant.TenantStateActive},
		}
		h := handleAdminUpdateTenant(svc, nil)
		body := `{"name":"New"}`
		req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/t1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var result tenant.Tenant
		testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
		testutil.Equal(t, "New", result.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{err: tenant.ErrTenantNotFound}
		h := handleAdminUpdateTenant(svc, nil)
		body := `{"name":"New"}`
		req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/missing", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "missing"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("deleted tenant returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateDeleted},
		}
		h := handleAdminUpdateTenant(svc, nil)
		body := `{"name":"New"}`
		req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/t1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "cannot update deleted tenant", errBody.Message)
	})

	t.Run("empty update returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
		}
		h := handleAdminUpdateTenant(svc, nil)
		body := `{}`
		req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/t1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "no fields to update", errBody.Message)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:    &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
			updateErr: errors.New("db down"),
		}
		h := handleAdminUpdateTenant(svc, nil)
		body := `{"name":"New"}`
		req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/t1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusInternalServerError, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "failed to update tenant", errBody.Message)
	})

	t.Run("emitter preserves updated field metadata", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:       &tenant.Tenant{ID: "t1", Name: "Old", State: tenant.TenantStateActive},
			updateResult: &tenant.Tenant{ID: "t1", Name: "New", State: tenant.TenantStateActive},
		}
		auditCapture := &quotaAuditCapture{}
		emitter := tenant.NewAuditEmitterWithInserter(auditCapture, nil)

		h := handleAdminUpdateTenant(svc, emitter)
		body := `{"name":"New","orgMetadata":{"tier":"gold"}}`
		req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/t1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		testutil.Equal(t, tenant.AuditActionTenantUpdated, auditCapture.action)

		var meta map[string]any
		testutil.NoError(t, json.Unmarshal(auditCapture.metadata, &meta))
		changesRaw, ok := meta["changes"].(map[string]any)
		testutil.True(t, ok, "expected audit metadata to include changes map")
		testutil.Equal(t, "New", changesRaw["name"])
		orgMeta, ok := changesRaw["orgMetadata"].(map[string]any)
		testutil.True(t, ok, "expected orgMetadata map in changes")
		testutil.Equal(t, "gold", orgMeta["tier"])
	})
}

// --- handleAdminSuspendTenant tests ---

func TestHandleAdminSuspendTenant(t *testing.T) {
	t.Parallel()

	t.Run("success transitions active to suspended", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:           &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
			transitionResult: &tenant.Tenant{ID: "t1", State: tenant.TenantStateSuspended},
		}
		h := handleAdminSuspendTenant(svc, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/suspend", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var result tenant.Tenant
		testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
		testutil.Equal(t, tenant.TenantStateSuspended, result.State)
	})

	t.Run("non-active tenant returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateSuspended},
		}
		h := handleAdminSuspendTenant(svc, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/suspend", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "tenant is not active", errBody.Message)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{err: tenant.ErrTenantNotFound}
		h := handleAdminSuspendTenant(svc, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/missing/suspend", nil)
		req = withURLParams(req, map[string]string{"tenantId": "missing"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("transition error returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:        &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
			transitionErr: tenant.ErrInvalidStateTransition,
		}
		h := handleAdminSuspendTenant(svc, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/suspend", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "cannot suspend tenant", errBody.Message)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:        &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
			transitionErr: errors.New("db down"),
		}
		h := handleAdminSuspendTenant(svc, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/suspend", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusInternalServerError, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "failed to suspend tenant", errBody.Message)
	})
}

// --- handleAdminResumeTenant tests ---

func TestHandleAdminResumeTenant(t *testing.T) {
	t.Parallel()

	t.Run("success transitions suspended to active", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:           &tenant.Tenant{ID: "t1", State: tenant.TenantStateSuspended},
			transitionResult: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
		}
		h := handleAdminResumeTenant(svc, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/resume", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var result tenant.Tenant
		testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
		testutil.Equal(t, tenant.TenantStateActive, result.State)
	})

	t.Run("non-suspended tenant returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
		}
		h := handleAdminResumeTenant(svc, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/resume", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "tenant is not suspended", errBody.Message)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{err: tenant.ErrTenantNotFound}
		h := handleAdminResumeTenant(svc, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/missing/resume", nil)
		req = withURLParams(req, map[string]string{"tenantId": "missing"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("transition error returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:        &tenant.Tenant{ID: "t1", State: tenant.TenantStateSuspended},
			transitionErr: tenant.ErrInvalidStateTransition,
		}
		h := handleAdminResumeTenant(svc, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/resume", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "cannot resume tenant", errBody.Message)
	})
}

// --- handleAdminDeleteTenant tests ---

func TestHandleAdminDeleteTenant(t *testing.T) {
	t.Parallel()

	t.Run("success transitions active to deleting", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:           &tenant.Tenant{ID: "t1", Slug: "tenant-a", IsolationMode: "schema", State: tenant.TenantStateActive},
			transitionResult: &tenant.Tenant{ID: "t1", State: tenant.TenantStateDeleting},
		}
		h := handleAdminDeleteTenant(svc, nil)
		req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/t1", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var result tenant.Tenant
		testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
		testutil.Equal(t, tenant.TenantStateDeleting, result.State)
		testutil.Equal(t, 1, svc.deleteTenantSchemaCalls)
		testutil.Equal(t, "tenant-a", svc.lastDeletedSchemaSlug)
	})

	t.Run("success transitions suspended to deleting", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:           &tenant.Tenant{ID: "t1", IsolationMode: "shared", State: tenant.TenantStateSuspended},
			transitionResult: &tenant.Tenant{ID: "t1", State: tenant.TenantStateDeleting},
		}
		h := handleAdminDeleteTenant(svc, nil)
		req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/t1", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		testutil.Equal(t, 0, svc.deleteTenantSchemaCalls)
	})

	t.Run("schema drop failure does not fail delete transition", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:                &tenant.Tenant{ID: "t1", Slug: "tenant-a", IsolationMode: "schema", State: tenant.TenantStateActive},
			transitionResult:      &tenant.Tenant{ID: "t1", State: tenant.TenantStateDeleting},
			deleteTenantSchemaErr: errors.New("drop failed"),
		}
		h := handleAdminDeleteTenant(svc, nil)
		req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/t1", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		testutil.Equal(t, 1, svc.deleteTenantSchemaCalls)
	})

	t.Run("already deleted returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateDeleted},
		}
		h := handleAdminDeleteTenant(svc, nil)
		req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/t1", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "tenant is already deleted", errBody.Message)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{err: tenant.ErrTenantNotFound}
		h := handleAdminDeleteTenant(svc, nil)
		req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/missing", nil)
		req = withURLParams(req, map[string]string{"tenantId": "missing"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("transition error returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:        &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
			transitionErr: tenant.ErrInvalidStateTransition,
		}
		h := handleAdminDeleteTenant(svc, nil)
		req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/t1", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "cannot delete tenant", errBody.Message)
	})
}

// --- handleAdminListTenantMembers tests ---

func TestHandleAdminListTenantMembers(t *testing.T) {
	t.Parallel()

	t.Run("success returns membership list", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
			memberships: []tenant.TenantMembership{
				{ID: "m1", TenantID: "t1", UserID: "u1", Role: tenant.MemberRoleOwner},
				{ID: "m2", TenantID: "t1", UserID: "u2", Role: tenant.MemberRoleMember},
			},
		}
		h := handleAdminListTenantMembers(svc)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/t1/members", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var result membershipListResult
		testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
		testutil.Equal(t, 2, len(result.Items))
	})

	t.Run("empty membership list", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
		}
		h := handleAdminListTenantMembers(svc)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/t1/members", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("tenant not found returns 404", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{err: tenant.ErrTenantNotFound}
		h := handleAdminListTenantMembers(svc)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/missing/members", nil)
		req = withURLParams(req, map[string]string{"tenantId": "missing"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusNotFound, w.Code)
	})
}

// --- handleAdminAddTenantMember tests ---

func TestHandleAdminAddTenantMember(t *testing.T) {
	t.Parallel()
	validUUID := "00000000-0000-0000-0000-000000000022"

	t.Run("success adds membership", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
			memberships: []tenant.TenantMembership{
				{ID: "m1", TenantID: "t1", UserID: validUUID, Role: tenant.MemberRoleMember},
			},
		}
		h := handleAdminAddTenantMember(svc, nil)
		body := `{"userId":"` + validUUID + `","role":"member"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/members", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("missing userId returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
		}
		h := handleAdminAddTenantMember(svc, nil)
		body := `{"role":"member"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/members", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "userId is required", errBody.Message)
	})

	t.Run("missing role returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
		}
		h := handleAdminAddTenantMember(svc, nil)
		body := `{"userId":"` + validUUID + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/members", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "role is required", errBody.Message)
	})

	t.Run("invalid role returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
		}
		h := handleAdminAddTenantMember(svc, nil)
		body := `{"userId":"` + validUUID + `","role":"superadmin"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/members", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "invalid role", errBody.Message)
	})

	t.Run("duplicate membership returns 409", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:       &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
			addMemberErr: tenant.ErrMembershipExists,
		}
		h := handleAdminAddTenantMember(svc, nil)
		body := `{"userId":"` + validUUID + `","role":"member"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/members", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusConflict, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "membership already exists", errBody.Message)
	})

	t.Run("tenant not found returns 404", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{err: tenant.ErrTenantNotFound}
		h := handleAdminAddTenantMember(svc, nil)
		body := `{"userId":"` + validUUID + `","role":"member"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/missing/members", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "missing"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid userId format returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
		}
		h := handleAdminAddTenantMember(svc, nil)
		body := `{"userId":"not-a-uuid","role":"member"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/members", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "invalid userId format", errBody.Message)
	})
}

// --- handleAdminRemoveTenantMember tests ---

func TestHandleAdminRemoveTenantMember(t *testing.T) {
	t.Parallel()

	t.Run("success removes membership", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
		}
		h := handleAdminRemoveTenantMember(svc, nil)
		req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/t1/members/u1", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1", "userId": "u1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("missing userId skips tenant lookup", func(t *testing.T) {
		t.Parallel()
		svc := &tenantLookupTrackingService{
			mockTenantService: &mockTenantService{
				tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
			},
		}
		h := handleAdminRemoveTenantMember(svc, nil)
		req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/t1/members/", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		testutil.Equal(t, 0, svc.getTenantCalls)
		body := decodeErrorBody(t, w)
		testutil.Equal(t, "user id is required", body.Message)
	})

	t.Run("membership not found returns 404", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:          &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
			removeMemberErr: tenant.ErrMembershipNotFound,
		}
		h := handleAdminRemoveTenantMember(svc, nil)
		req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/t1/members/u1", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1", "userId": "u1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusNotFound, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "membership not found", errBody.Message)
	})

	t.Run("tenant not found returns 404", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{err: tenant.ErrTenantNotFound}
		h := handleAdminRemoveTenantMember(svc, nil)
		req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/missing/members/u1", nil)
		req = withURLParams(req, map[string]string{"tenantId": "missing", "userId": "u1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusNotFound, w.Code)
	})
}

// --- handleAdminUpdateTenantMemberRole tests ---

func TestHandleAdminUpdateTenantMemberRole(t *testing.T) {
	t.Parallel()

	t.Run("success updates role", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
			memberships: []tenant.TenantMembership{
				{ID: "m1", TenantID: "t1", UserID: "u1", Role: tenant.MemberRoleAdmin},
			},
		}
		h := handleAdminUpdateTenantMemberRole(svc, nil)
		body := `{"role":"admin"}`
		req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/t1/members/u1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1", "userId": "u1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var result tenant.TenantMembership
		testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
		testutil.Equal(t, tenant.MemberRoleAdmin, result.Role)
	})

	t.Run("missing userId skips tenant lookup", func(t *testing.T) {
		t.Parallel()
		svc := &tenantLookupTrackingService{
			mockTenantService: &mockTenantService{
				tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
			},
		}
		h := handleAdminUpdateTenantMemberRole(svc, nil)
		req := httptest.NewRequest(http.MethodPatch, "/api/admin/tenants/t1/members/", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		testutil.Equal(t, 0, svc.getTenantCalls)
		body := decodeErrorBody(t, w)
		testutil.Equal(t, "user id is required", body.Message)
	})

	t.Run("missing role returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
		}
		h := handleAdminUpdateTenantMemberRole(svc, nil)
		body := `{}`
		req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/t1/members/u1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1", "userId": "u1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "role is required", errBody.Message)
	})

	t.Run("invalid role returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant: &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
		}
		h := handleAdminUpdateTenantMemberRole(svc, nil)
		body := `{"role":"superadmin"}`
		req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/t1/members/u1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1", "userId": "u1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "invalid role", errBody.Message)
	})

	t.Run("membership not found returns 404", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{
			tenant:        &tenant.Tenant{ID: "t1", State: tenant.TenantStateActive},
			updateRoleErr: tenant.ErrMembershipNotFound,
		}
		h := handleAdminUpdateTenantMemberRole(svc, nil)
		body := `{"role":"admin"}`
		req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/t1/members/u1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1", "userId": "u1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusNotFound, w.Code)
		errBody := decodeErrorBody(t, w)
		testutil.Equal(t, "membership not found", errBody.Message)
	})

	t.Run("tenant not found returns 404", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{err: tenant.ErrTenantNotFound}
		h := handleAdminUpdateTenantMemberRole(svc, nil)
		body := `{"role":"admin"}`
		req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/missing/members/u1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "missing", "userId": "u1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusNotFound, w.Code)
	})
}

// --- Maintenance mode handler tests ---

func TestHandleAdminEnableMaintenance(t *testing.T) {
	t.Parallel()

	t.Run("success enables maintenance", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{}
		h := handleAdminEnableMaintenance(svc, nil)
		body := `{"reason":"planned upgrade"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/maintenance/enable", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var resp maintenanceStateResponse
		json.NewDecoder(w.Body).Decode(&resp)
		testutil.True(t, resp.Enabled, "expected maintenance enabled")
	})

	t.Run("missing tenantId returns 400", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{}
		h := handleAdminEnableMaintenance(svc, nil)
		body := `{"reason":"test"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants//maintenance/enable", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": ""})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{maintenanceErr: errors.New("db error")}
		h := handleAdminEnableMaintenance(svc, nil)
		body := `{"reason":"test"}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/maintenance/enable", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandleAdminDisableMaintenance(t *testing.T) {
	t.Parallel()

	t.Run("success disables maintenance", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{}
		h := handleAdminDisableMaintenance(svc, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/maintenance/disable", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var resp maintenanceStateResponse
		json.NewDecoder(w.Body).Decode(&resp)
		testutil.True(t, !resp.Enabled, "expected maintenance disabled")
	})
}

func TestHandleAdminGetMaintenance(t *testing.T) {
	t.Parallel()

	t.Run("no maintenance state returns default", func(t *testing.T) {
		t.Parallel()
		svc := &mockTenantService{}
		h := handleAdminGetMaintenance(svc)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/t1/maintenance", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var resp maintenanceStateResponse
		json.NewDecoder(w.Body).Decode(&resp)
		testutil.True(t, !resp.Enabled, "expected default maintenance disabled")
		testutil.Equal(t, "t1", resp.TenantID)
	})

	t.Run("existing maintenance state returned", func(t *testing.T) {
		t.Parallel()
		reason := "upgrade"
		svc := &mockTenantService{maintenanceState: &tenant.TenantMaintenanceState{
			TenantID: "t1",
			Enabled:  true,
			Reason:   &reason,
		}}
		h := handleAdminGetMaintenance(svc)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/t1/maintenance", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var resp maintenanceStateResponse
		json.NewDecoder(w.Body).Decode(&resp)
		testutil.True(t, resp.Enabled, "expected maintenance enabled")
	})
}

// --- Breaker handler tests ---

func TestHandleAdminGetBreaker(t *testing.T) {
	t.Parallel()

	t.Run("returns closed state for new tenant", func(t *testing.T) {
		t.Parallel()
		tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
			FailureThreshold: 3,
			OpenDuration:     30 * time.Second,
		}, nil)
		h := handleAdminGetBreaker(tracker)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/t1/breaker", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var resp breakerStateResponse
		json.NewDecoder(w.Body).Decode(&resp)
		testutil.Equal(t, "closed", resp.State)
		testutil.Equal(t, 0, resp.ConsecutiveFailures)
	})

	t.Run("returns open state with counters", func(t *testing.T) {
		t.Parallel()
		tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
			FailureThreshold: 2,
			OpenDuration:     30 * time.Second,
		}, nil)
		tracker.RecordFailure("t1")
		tracker.RecordFailure("t1")
		h := handleAdminGetBreaker(tracker)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/t1/breaker", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var resp breakerStateResponse
		json.NewDecoder(w.Body).Decode(&resp)
		testutil.Equal(t, "open", resp.State)
		testutil.Equal(t, 2, resp.ConsecutiveFailures)
	})

	t.Run("missing tenantId returns 400", func(t *testing.T) {
		t.Parallel()
		tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{}, nil)
		h := handleAdminGetBreaker(tracker)
		req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants//breaker", nil)
		req = withURLParams(req, map[string]string{"tenantId": ""})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleAdminResetBreaker(t *testing.T) {
	t.Parallel()

	t.Run("resets open breaker to closed", func(t *testing.T) {
		t.Parallel()
		tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{
			FailureThreshold: 1,
			OpenDuration:     30 * time.Second,
		}, nil)
		tracker.RecordFailure("t1")
		testutil.Equal(t, tenant.BreakerStateOpen, tracker.State("t1"))

		h := handleAdminResetBreaker(nil, tracker)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/t1/breaker/reset", nil)
		req = withURLParams(req, map[string]string{"tenantId": "t1"})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusOK, w.Code)
		var resp breakerStateResponse
		json.NewDecoder(w.Body).Decode(&resp)
		testutil.Equal(t, "closed", resp.State)
		testutil.Equal(t, 0, resp.ConsecutiveFailures)
		testutil.Equal(t, 0, resp.HalfOpenProbes)
	})

	t.Run("missing tenantId returns 400", func(t *testing.T) {
		t.Parallel()
		tracker := tenant.NewTenantBreakerTracker(tenant.TenantBreakerConfig{}, nil)
		h := handleAdminResetBreaker(nil, tracker)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants//breaker/reset", nil)
		req = withURLParams(req, map[string]string{"tenantId": ""})
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
	})
}
