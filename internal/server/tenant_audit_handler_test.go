package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

func tenantAdminToken(t *testing.T, s *Server) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"testpass"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var body map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	token := body["token"]
	testutil.True(t, token != "", "expected non-empty admin token")
	return token
}

func TestGetIPAddressPrefersForwardedClientIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.10, 203.0.113.20")

	ip := getIPAddress(req)
	if ip == nil {
		t.Fatal("expected parsed IP")
	}
	testutil.Equal(t, "198.51.100.10", *ip)
}

func TestGetIPAddressStripsRemotePort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.44:12345"

	ip := getIPAddress(req)
	if ip == nil {
		t.Fatal("expected parsed IP")
	}
	testutil.Equal(t, "192.0.2.44", *ip)
}

func TestGetIPAddressRejectsInvalidRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "not-an-ip"

	ip := getIPAddress(req)
	testutil.True(t, ip == nil, "expected invalid remote addr to be ignored")
}

func TestGetActorIDUsesClaimsSubject(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	claims := &auth.Claims{}
	claims.Subject = "7e70aca2-f59c-4faa-a67a-a2f7f1908bb2"
	req = req.WithContext(auth.ContextWithClaims(req.Context(), claims))

	actorID := getActorID(req)
	if actorID == nil {
		t.Fatal("expected actor id from claims")
	}
	testutil.Equal(t, "7e70aca2-f59c-4faa-a67a-a2f7f1908bb2", *actorID)
}

func TestGetActorIDUsesAuditPrincipalFallback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(audit.ContextWithPrincipal(req.Context(), "8bc2f7ea-a37f-4f42-8865-cb47e07c4f58"))

	actorID := getActorID(req)
	if actorID == nil {
		t.Fatal("expected actor id from audit principal")
	}
	testutil.Equal(t, "8bc2f7ea-a37f-4f42-8865-cb47e07c4f58", *actorID)
}

func TestGetActorIDIgnoresHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Actor-ID", "11111111-1111-1111-1111-111111111111")

	actorID := getActorID(req)
	testutil.True(t, actorID == nil, "expected untrusted actor header to be ignored")
}

func TestTenantAuditQueryEndpoint_MethodNotAllowed(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/tenant-1/audit", nil)
	w := httptest.NewRecorder()

	svc := &mockTenantAuditQueryService{}
	tenantAuditQueryHandler(svc)(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestTenantAuditQueryEndpoint_InvalidTenantID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/invalid/audit", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", "invalid")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	svc := &mockTenantAuditQueryService{}
	tenantAuditQueryHandler(svc)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTenantAuditQueryEndpoint_InvalidFromFilter(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/00000000-0000-0000-0000-000000000001/audit?from=invalid", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", "00000000-0000-0000-0000-000000000001")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	svc := &mockTenantAuditQueryService{}
	tenantAuditQueryHandler(svc)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTenantAuditQueryEndpoint_InvalidToFilter(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/00000000-0000-0000-0000-000000000001/audit?to=invalid", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", "00000000-0000-0000-0000-000000000001")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	svc := &mockTenantAuditQueryService{}
	tenantAuditQueryHandler(svc)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTenantAuditQueryEndpoint_InvalidLimit(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/00000000-0000-0000-0000-000000000001/audit?limit=invalid", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", "00000000-0000-0000-0000-000000000001")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	svc := &mockTenantAuditQueryService{}
	tenantAuditQueryHandler(svc)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTenantAuditQueryEndpoint_NegativeLimit(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/00000000-0000-0000-0000-000000000001/audit?limit=-5", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", "00000000-0000-0000-0000-000000000001")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	svc := &mockTenantAuditQueryService{}
	tenantAuditQueryHandler(svc)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTenantAuditQueryEndpoint_InvalidOffset(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/00000000-0000-0000-0000-000000000001/audit?offset=invalid", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", "00000000-0000-0000-0000-000000000001")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	svc := &mockTenantAuditQueryService{}
	tenantAuditQueryHandler(svc)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTenantAuditQueryEndpoint_NegativeOffset(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/00000000-0000-0000-0000-000000000001/audit?offset=-5", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", "00000000-0000-0000-0000-000000000001")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	svc := &mockTenantAuditQueryService{}
	tenantAuditQueryHandler(svc)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTenantAuditQueryEndpoint_InvalidActorID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/00000000-0000-0000-0000-000000000001/audit?actor_id=invalid", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", "00000000-0000-0000-0000-000000000001")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	svc := &mockTenantAuditQueryService{}
	tenantAuditQueryHandler(svc)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTenantAuditQueryEndpoint_Success(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000001"
	now := time.Now().UTC().Truncate(time.Microsecond)
	events := []tenant.TenantAuditEvent{
		{
			ID:        "00000000-0000-0000-0000-000000000002",
			TenantID:  tenantID,
			Action:    tenant.AuditActionTenantCreated,
			Result:    tenant.AuditResultSuccess,
			Metadata:  json.RawMessage(`{"name":"Test"}`),
			CreatedAt: now,
		},
		{
			ID:        "00000000-0000-0000-0000-000000000003",
			TenantID:  tenantID,
			Action:    tenant.AuditActionMembershipAdded,
			Result:    tenant.AuditResultSuccess,
			Metadata:  json.RawMessage(`{"userId":"user-1","role":"admin"}`),
			CreatedAt: now.Add(-1 * time.Hour),
		},
	}

	svc := &mockTenantAuditQueryService{
		auditEvents: events,
	}

	r := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/"+tenantID+"/audit", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", tenantID)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	tenantAuditQueryHandler(svc)(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var result tenantAuditListResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result.Count != 2 {
		t.Errorf("count = %d, want 2", result.Count)
	}
	if result.Limit != 50 {
		t.Errorf("limit = %d, want 50 (default)", result.Limit)
	}
	if result.Offset != 0 {
		t.Errorf("offset = %d, want 0", result.Offset)
	}
	if len(result.Items) != 2 {
		t.Errorf("items count = %d, want 2", len(result.Items))
	}
}

func TestTenantAuditQueryEndpoint_WithFilters(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000001"
	actorID := "00000000-0000-0000-0000-000000000010"
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour).Format(time.RFC3339)
	to := now.Format(time.RFC3339)

	events := []tenant.TenantAuditEvent{
		{
			ID:        "00000000-0000-0000-0000-000000000002",
			TenantID:  tenantID,
			ActorID:   &actorID,
			Action:    tenant.AuditActionTenantCreated,
			Result:    tenant.AuditResultSuccess,
			CreatedAt: now.Add(-12 * time.Hour),
		},
	}

	svc := &mockTenantAuditQueryService{
		auditEvents: events,
	}

	url := "/api/admin/tenants/" + tenantID + "/audit?action=tenant.created&result=success&actor_id=" + actorID + "&from=" + from + "&to=" + to + "&limit=10&offset=5"
	r := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", tenantID)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	tenantAuditQueryHandler(svc)(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var result tenantAuditListResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result.Limit != 10 {
		t.Errorf("limit = %d, want 10", result.Limit)
	}
	if result.Offset != 5 {
		t.Errorf("offset = %d, want 5", result.Offset)
	}

	if svc.lastAuditQuery == nil {
		t.Fatal("lastAuditQuery is nil")
	}
	if svc.lastAuditQuery.Action != "tenant.created" {
		t.Errorf("action filter = %q, want %q", svc.lastAuditQuery.Action, "tenant.created")
	}
	if svc.lastAuditQuery.Result != "success" {
		t.Errorf("result filter = %q, want %q", svc.lastAuditQuery.Result, "success")
	}
	if svc.lastAuditQuery.ActorID != actorID {
		t.Errorf("actor_id filter = %q, want %q", svc.lastAuditQuery.ActorID, actorID)
	}
}

func TestTenantAuditQueryEndpoint_EmptyResult(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000001"

	svc := &mockTenantAuditQueryService{
		auditEvents: []tenant.TenantAuditEvent{},
	}

	r := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/"+tenantID+"/audit", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", tenantID)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	tenantAuditQueryHandler(svc)(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result tenantAuditListResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result.Count != 0 {
		t.Errorf("count = %d, want 0", result.Count)
	}
	if len(result.Items) != 0 {
		t.Errorf("items count = %d, want 0", len(result.Items))
	}
}

func TestTenantAuditQueryEndpoint_DateRangeValidation(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000001"
	now := time.Now().UTC()
	from := now.Format(time.RFC3339)
	to := now.Add(-24 * time.Hour).Format(time.RFC3339) // to is before from

	r := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/"+tenantID+"/audit?from="+from+"&to="+to, nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", tenantID)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	svc := &mockTenantAuditQueryService{}
	tenantAuditQueryHandler(svc)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestParseAuditFilters(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		wantErr        bool
		expectedLimit  int
		expectedOffset int
	}{
		{"no filters", "/audit?tenantId=00000000-0000-0000-0000-000000000001", false, 50, 0},
		{"with limit", "/audit?tenantId=00000000-0000-0000-0000-000000000001&limit=100", false, 100, 0},
		{"with offset", "/audit?tenantId=00000000-0000-0000-0000-000000000001&offset=50", false, 50, 50},
		{"limit too large", "/audit?tenantId=00000000-0000-0000-0000-000000000001&limit=10000", false, 1000, 0}, // capped
		{"zero limit", "/audit?tenantId=00000000-0000-0000-0000-000000000001&limit=0", false, 50, 0},            // use default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("tenantId", "00000000-0000-0000-0000-000000000001")
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			filters, err := parseAuditFilters(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseAuditFilters() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				query := filters.toQuery()
				if query.Limit != tt.expectedLimit {
					t.Errorf("limit = %d, want %d", query.Limit, tt.expectedLimit)
				}
				if query.Offset != tt.expectedOffset {
					t.Errorf("offset = %d, want %d", query.Offset, tt.expectedOffset)
				}
			}
		})
	}
}

func TestTenantAuditQueryEndpoint_Ordering(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000001"
	now := time.Now().UTC()

	events := []tenant.TenantAuditEvent{
		{
			ID:        "00000000-0000-0000-0000-000000000002",
			TenantID:  tenantID,
			Action:    tenant.AuditActionTenantCreated,
			Result:    tenant.AuditResultSuccess,
			CreatedAt: now.Add(-2 * time.Hour),
		},
		{
			ID:        "00000000-0000-0000-0000-000000000003",
			TenantID:  tenantID,
			Action:    tenant.AuditActionMembershipAdded,
			Result:    tenant.AuditResultSuccess,
			CreatedAt: now.Add(-1 * time.Hour),
		},
		{
			ID:        "00000000-0000-0000-0000-000000000004",
			TenantID:  tenantID,
			Action:    tenant.AuditActionTenantUpdated,
			Result:    tenant.AuditResultSuccess,
			CreatedAt: now,
		},
	}

	svc := &mockTenantAuditQueryService{
		auditEvents: events,
	}

	r := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/"+tenantID+"/audit", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", tenantID)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	tenantAuditQueryHandler(svc)(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result tenantAuditListResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Verify service was called
	if svc.lastAuditQuery == nil {
		t.Fatal("lastAuditQuery is nil")
	}
	// The service layer should handle ordering; we just verify it was called correctly
	if svc.lastAuditQuery.TenantID != tenantID {
		t.Errorf("tenantID = %q, want %q", svc.lastAuditQuery.TenantID, tenantID)
	}
}

func TestHandleAdminTenantAudit_ServiceNotConfigured(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/00000000-0000-0000-0000-000000000001/audit", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", "00000000-0000-0000-0000-000000000001")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	s.handleAdminTenantAudit(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleAdminTenantAudit_ServiceMissingQueryMethod(t *testing.T) {
	s := &Server{tenantSvc: &mockTenantService{}}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/00000000-0000-0000-0000-000000000001/audit", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantId", "00000000-0000-0000-0000-000000000001")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	s.handleAdminTenantAudit(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestServerTenantAuditRouteRegistered(t *testing.T) {
	cfg := config.Default()
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(nil, logger)
	s := New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/00000000-0000-0000-0000-000000000001/audit", nil)
	w := httptest.NewRecorder()

	s.Router().ServeHTTP(w, req)

	// If the route isn't registered we'd get 404. Handler-level validation/service
	// behavior determines the non-404 status.
	if w.Code == http.StatusNotFound {
		t.Fatalf("tenant audit route not registered: got 404")
	}
}

func TestServerTenantAdminRoutesUseCurrentTenantService(t *testing.T) {
	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(nil, logger)
	s := New(cfg, logger, ch, nil, nil, nil)
	s.SetTenantService(&mockTenantService{
		listResult: &tenant.TenantListResult{
			Items: []tenant.Tenant{{ID: "tenant-1", Name: "Acme"}},
		},
	})

	token := tenantAdminToken(t, s)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	s.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var result tenant.TenantListResult
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	testutil.Equal(t, 1, len(result.Items))
	testutil.Equal(t, "tenant-1", result.Items[0].ID)
}

type mockTenantAuditQueryService struct {
	auditEvents    []tenant.TenantAuditEvent
	lastAuditQuery *tenant.AuditQuery
	err            error
}

func (m *mockTenantAuditQueryService) QueryAuditEvents(ctx context.Context, query tenant.AuditQuery) ([]tenant.TenantAuditEvent, error) {
	m.lastAuditQuery = &query
	return m.auditEvents, m.err
}
