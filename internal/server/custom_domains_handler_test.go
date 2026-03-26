package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

// fakeDomainManager is an in-memory fake for testing domain admin handlers.
type fakeDomainManager struct {
	domains   []DomainBinding
	listErr   error
	getErr    error
	createErr error
	deleteErr error
	verifyErr error
}

func (f *fakeDomainManager) CreateDomain(_ context.Context, hostname, environment, redirectMode string) (*DomainBinding, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	if environment == "" {
		environment = "production"
	}
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	token := "aabbccdd00112233aabbccdd00112233aabbccdd00112233aabbccdd00112233"
	var redirectModePtr *string
	if redirectMode != "" {
		redirectModePtr = &redirectMode
	}
	b := DomainBinding{
		ID:                "00000000-0000-0000-0000-000000000099",
		Hostname:          hostname,
		Environment:       environment,
		Status:            StatusPendingVerification,
		VerificationToken: token,
		RedirectMode:      redirectModePtr,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	b.populateVerificationRecord()
	f.domains = append(f.domains, b)
	return &b, nil
}

func (f *fakeDomainManager) GetDomain(_ context.Context, id string) (*DomainBinding, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	for _, d := range f.domains {
		if d.ID == id {
			b := d
			b.populateVerificationRecord()
			return &b, nil
		}
	}
	return nil, ErrDomainNotFound
}

func (f *fakeDomainManager) ListDomains(_ context.Context, page, perPage int) (*DomainBindingListResult, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	total := len(f.domains)
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}

	items := f.domains[start:end]
	if items == nil {
		items = []DomainBinding{}
	}

	totalPages := total / perPage
	if total%perPage != 0 {
		totalPages++
	}

	return &DomainBindingListResult{
		Items:      items,
		Page:       page,
		PerPage:    perPage,
		TotalItems: total,
		TotalPages: totalPages,
	}, nil
}

func (f *fakeDomainManager) DeleteDomain(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	for i, d := range f.domains {
		if d.ID == id {
			f.domains = append(f.domains[:i], f.domains[i+1:]...)
			return nil
		}
	}
	return ErrDomainNotFound
}

func (f *fakeDomainManager) TriggerVerification(_ context.Context, id string) (*DomainBinding, error) {
	if f.verifyErr != nil {
		return nil, f.verifyErr
	}
	for _, d := range f.domains {
		if d.ID == id {
			b := d
			b.populateVerificationRecord()
			return &b, nil
		}
	}
	return nil, ErrDomainNotFound
}

func (f *fakeDomainManager) UpdateDomainStatus(_ context.Context, id string, status DomainStatus, lastError *string) (*DomainBinding, error) {
	for i, d := range f.domains {
		if d.ID == id {
			if d.Status == StatusTombstoned {
				return nil, ErrDomainNotFound
			}
			f.domains[i].Status = status
			f.domains[i].LastError = lastError
			b := f.domains[i]
			b.populateVerificationRecord()
			return &b, nil
		}
	}
	return nil, ErrDomainNotFound
}

func (f *fakeDomainManager) SetDomainCert(_ context.Context, id string, certRef string, certExpiry time.Time) (*DomainBinding, error) {
	for i, d := range f.domains {
		if d.ID == id {
			f.domains[i].Status = StatusActive
			f.domains[i].CertRef = &certRef
			f.domains[i].CertExpiry = &certExpiry
			f.domains[i].LastError = nil
			b := f.domains[i]
			b.populateVerificationRecord()
			return &b, nil
		}
	}
	return nil, ErrDomainNotFound
}

func (f *fakeDomainManager) ListDomainsForCertRenewal(_ context.Context, renewBefore time.Time) ([]DomainBinding, error) {
	var result []DomainBinding
	for _, d := range f.domains {
		if d.Status == StatusActive && d.CertExpiry != nil && d.CertExpiry.Before(renewBefore) {
			b := d
			b.populateVerificationRecord()
			result = append(result, b)
		}
	}
	return result, nil
}

func sampleDomains() []DomainBinding {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	token := "aabbccdd00112233aabbccdd00112233aabbccdd00112233aabbccdd00112233"
	return []DomainBinding{
		{ID: "00000000-0000-0000-0000-000000000001", Hostname: "app.example.com", Environment: "production", Status: StatusActive, VerificationToken: token, CreatedAt: now, UpdatedAt: now},
		{ID: "00000000-0000-0000-0000-000000000002", Hostname: "staging.example.com", Environment: "staging", Status: StatusVerified, VerificationToken: token, CreatedAt: now, UpdatedAt: now},
		{ID: "00000000-0000-0000-0000-000000000003", Hostname: "other.example.com", Environment: "production", Status: StatusPendingVerification, VerificationToken: token, CreatedAt: now, UpdatedAt: now},
	}
}

func strptr(s string) *string {
	return &s
}

// --- Create domain ---

func TestAdminCreateDomainSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{}
	handler := handleAdminCreateDomain(mgr)

	body := `{"hostname":"new.example.com","environment":"production"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)

	var b DomainBinding
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&b))
	testutil.Equal(t, "new.example.com", b.Hostname)
	testutil.Equal(t, string(StatusPendingVerification), string(b.Status))
	// verificationRecord should be populated
	testutil.True(t, strings.Contains(b.VerificationRecord, "_ayb-challenge.new.example.com"), "expected verificationRecord to contain challenge hostname, got: %s", b.VerificationRecord)
	testutil.True(t, b.VerificationToken != "", "expected verificationToken to be non-empty")
}

func TestAdminCreateDomainMissingHostname(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{}
	handler := handleAdminCreateDomain(mgr)

	body := `{"environment":"production"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "hostname is required")
}

func TestAdminCreateDomainIPv4Rejected(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{}
	handler := handleAdminCreateDomain(mgr)

	body := `{"hostname":"192.168.1.1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "IP addresses are not valid hostnames")
}

func TestAdminCreateDomainIPv6Rejected(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{}
	handler := handleAdminCreateDomain(mgr)

	body := `{"hostname":"::1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "IP addresses are not valid hostnames")
}

func TestAdminCreateDomainWildcardRejected(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{}
	handler := handleAdminCreateDomain(mgr)

	body := `{"hostname":"*.example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "wildcard")
}

func TestAdminCreateDomainTooLongRejected(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{}
	handler := handleAdminCreateDomain(mgr)

	// 254-char hostname
	longLabel := strings.Repeat("a", 63)
	longHostname := fmt.Sprintf("%s.%s.%s.%s.z", longLabel, longLabel, longLabel, longLabel)
	body := fmt.Sprintf(`{"hostname":%q}`, longHostname)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminCreateDomainConflict(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{createErr: ErrDomainHostnameConflict}
	handler := handleAdminCreateDomain(mgr)

	body := `{"hostname":"taken.example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusConflict, w.Code)
	testutil.Contains(t, w.Body.String(), "already bound")
}

func TestAdminCreateDomainServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{createErr: fmt.Errorf("db down")}
	handler := handleAdminCreateDomain(mgr)

	body := `{"hostname":"new.example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to create domain")
}

func TestAdminCreateDomainInvalidRedirectMode(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{}
	handler := handleAdminCreateDomain(mgr)

	body := `{"hostname":"new.example.com","redirectMode":"foobar"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "redirectMode must be")
}

func TestAdminCreateDomainValidRedirectMode(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{}
	handler := handleAdminCreateDomain(mgr)

	body := `{"hostname":"new.example.com","redirectMode":"permanent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	var b DomainBinding
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&b))
	testutil.Equal(t, "permanent", *b.RedirectMode)
}

func TestAdminCreateDomainNormalizesHostname(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{}
	handler := handleAdminCreateDomain(mgr)

	body := `{"hostname":"NEW.EXAMPLE.COM"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	var b DomainBinding
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&b))
	testutil.Equal(t, "new.example.com", b.Hostname)
}

// --- Get domain ---

func TestAdminGetDomainSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains()}
	handler := handleAdminGetDomain(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/domains/{id}", handler)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/domains/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var b DomainBinding
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&b))
	testutil.Equal(t, "app.example.com", b.Hostname)
	testutil.Equal(t, string(StatusActive), string(b.Status))
}

func TestAdminGetDomainNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains()}
	handler := handleAdminGetDomain(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/domains/{id}", handler)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/domains/00000000-0000-0000-0000-000000000099", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "domain not found")
}

func TestAdminGetDomainInvalidUUID(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains()}
	handler := handleAdminGetDomain(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/domains/{id}", handler)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/domains/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid domain id format")
}

func TestAdminGetDomainServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains(), getErr: fmt.Errorf("db connection lost")}
	handler := handleAdminGetDomain(mgr)

	r := chi.NewRouter()
	r.Get("/api/admin/domains/{id}", handler)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/domains/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to get domain")
}

// --- List domains ---

func TestAdminListDomainsSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains()}
	handler := handleAdminListDomains(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/domains", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var result DomainBindingListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	testutil.Equal(t, 3, result.TotalItems)
	testutil.Equal(t, 3, len(result.Items))
}

func TestAdminListDomainsEmpty(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: nil}
	handler := handleAdminListDomains(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/domains", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var result DomainBindingListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	testutil.Equal(t, 0, result.TotalItems)
}

func TestAdminListDomainsWithPagination(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains()}
	handler := handleAdminListDomains(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/domains?page=1&perPage=2", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var result DomainBindingListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	testutil.Equal(t, 3, result.TotalItems)
	testutil.Equal(t, 2, len(result.Items))
	testutil.Equal(t, 2, result.TotalPages)
}

func TestAdminListDomainsPaginationDefaults(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains()}
	handler := handleAdminListDomains(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/domains?page=0&perPage=0", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var result DomainBindingListResult
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	testutil.Equal(t, 1, result.Page)
	testutil.Equal(t, 20, result.PerPage)
}

func TestAdminListDomainsServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{listErr: fmt.Errorf("db down")}
	handler := handleAdminListDomains(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/domains", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to list domains")
}

// --- Delete domain ---

func TestAdminDeleteDomainSuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains()}
	handler := handleAdminDeleteDomain(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/domains/{id}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/domains/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, 2, len(mgr.domains))
}

func TestAdminDeleteDomainNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains()}
	handler := handleAdminDeleteDomain(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/domains/{id}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/domains/00000000-0000-0000-0000-000000000099", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "domain not found")
}

func TestAdminDeleteDomainInvalidUUID(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains()}
	handler := handleAdminDeleteDomain(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/domains/{id}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/domains/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid domain id format")
}

func TestAdminDeleteDomainServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains(), deleteErr: fmt.Errorf("db constraint")}
	handler := handleAdminDeleteDomain(mgr)

	r := chi.NewRouter()
	r.Delete("/api/admin/domains/{id}", handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/domains/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to delete domain")
}

// --- Trigger verification ---

func TestAdminTriggerVerifySuccess(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains()}
	handler := handleAdminTriggerDomainVerify(mgr, nil)

	r := chi.NewRouter()
	r.Post("/api/admin/domains/{id}/verify", handler)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains/00000000-0000-0000-0000-000000000001/verify", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var b DomainBinding
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&b))
	testutil.Equal(t, "app.example.com", b.Hostname)
}

func TestAdminTriggerVerifyNotFound(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains()}
	handler := handleAdminTriggerDomainVerify(mgr, nil)

	r := chi.NewRouter()
	r.Post("/api/admin/domains/{id}/verify", handler)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains/00000000-0000-0000-0000-000000000099/verify", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "domain not found")
}

func TestAdminTriggerVerifyServiceError(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains(), verifyErr: fmt.Errorf("dns lookup timeout")}
	handler := handleAdminTriggerDomainVerify(mgr, nil)

	r := chi.NewRouter()
	r.Post("/api/admin/domains/{id}/verify", handler)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains/00000000-0000-0000-0000-000000000001/verify", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to trigger verification")
}

func TestAdminTriggerVerifyInvalidUUID(t *testing.T) {
	t.Parallel()
	mgr := &fakeDomainManager{domains: sampleDomains()}
	handler := handleAdminTriggerDomainVerify(mgr, nil)

	r := chi.NewRouter()
	r.Post("/api/admin/domains/{id}/verify", handler)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains/not-a-uuid/verify", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid domain id format")
}

// --- normalizeAndValidateHostname ---

func TestNormalizeAndValidateHostnameValid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com"},
		{"EXAMPLE.COM", "example.com"},
		{"sub.example.com", "sub.example.com"},
		{"a-b.example.com", "a-b.example.com"},
		{"deep.sub.example.com", "deep.sub.example.com"},
		{"xn--bcher-kva.example.com", "xn--bcher-kva.example.com"}, // punycode allowed
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeAndValidateHostname(tc.input)
			testutil.NoError(t, err)
			testutil.Equal(t, tc.expected, got)
		})
	}
}

func TestNormalizeAndValidateHostnameInvalid(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",                               // empty
		"192.168.1.1",                    // IPv4
		"::1",                            // IPv6
		"2001:db8::1",                    // IPv6
		"*.example.com",                  // wildcard
		"example.com:8080",               // port suffix
		"-leadinghyphen.com",             // leading hyphen in label
		"trailinghyphen-.com",            // trailing hyphen in label
		strings.Repeat("a", 64) + ".com", // label > 63 chars
		strings.Repeat("a", 64) + "." + strings.Repeat("b", 64) + "." + strings.Repeat("c", 64) + "." + strings.Repeat("d", 64) + ".z", // total > 253
		"example",       // single label (no dot)
		"has space.com", // space in label
	}
	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("%q", tc), func(t *testing.T) {
			t.Parallel()
			got, err := normalizeAndValidateHostname(tc)
			testutil.True(t, err != nil, "expected error for input %q, got nil with result %q", tc, got)
		})
	}
}
