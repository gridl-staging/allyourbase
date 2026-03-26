package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/go-chi/chi/v5"
)

// fakeHealthChecker implements DomainHealthChecker for testing.
type fakeHealthChecker struct {
	domains       []DomainBinding
	listErr       error
	healthUpdates []healthUpdate // tracks UpdateDomainHealth calls
	updateErr     error
}

type healthUpdate struct {
	ID           string
	HealthStatus string
}

func (f *fakeHealthChecker) ListDomainsForHealthCheck(_ context.Context) ([]DomainBinding, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.domains, nil
}

func (f *fakeHealthChecker) UpdateDomainHealth(_ context.Context, id string, healthStatus string) error {
	f.healthUpdates = append(f.healthUpdates, healthUpdate{ID: id, HealthStatus: healthStatus})
	return f.updateErr
}

// fakeReverifier implements DomainReverifier for testing.
type fakeReverifier struct {
	domainsForReverify []DomainBinding
	reverifyListErr    error
	lapsedDomains      []DomainBinding
	lapsedListErr      error
	incrementCalls     []string // domain IDs
	incrementErr       error
	resetCalls         []string // domain IDs
	resetErr           error
}

func (f *fakeReverifier) ListDomainsForReverify(_ context.Context) ([]DomainBinding, error) {
	if f.reverifyListErr != nil {
		return nil, f.reverifyListErr
	}
	return f.domainsForReverify, nil
}

func (f *fakeReverifier) IncrementReverifyFailures(_ context.Context, id string) error {
	f.incrementCalls = append(f.incrementCalls, id)
	return f.incrementErr
}

func (f *fakeReverifier) ResetReverifyFailures(_ context.Context, id string) error {
	f.resetCalls = append(f.resetCalls, id)
	return f.resetErr
}

func (f *fakeReverifier) ListLapsedDomainsForCleanup(_ context.Context) ([]DomainBinding, error) {
	if f.lapsedListErr != nil {
		return nil, f.lapsedListErr
	}
	return f.lapsedDomains, nil
}

// fakeResolverForReverify implements DNSResolver for re-verify tests.
type fakeResolverForReverify struct {
	records map[string][]string // hostname → TXT records
	err     error
}

func (f *fakeResolverForReverify) LookupTXT(_ context.Context, hostname string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.records[hostname], nil
}

func sampleActiveDomain(id, hostname, token string) DomainBinding {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	certRef := "abc123"
	certExpiry := now.Add(60 * 24 * time.Hour)
	return DomainBinding{
		ID:                id,
		Hostname:          hostname,
		Environment:       "production",
		Status:            StatusActive,
		VerificationToken: token,
		CertRef:           &certRef,
		CertExpiry:        &certExpiry,
		CreatedAt:         now,
		UpdatedAt:         now,
		HealthStatus:      "unknown",
	}
}

// --- Health check handler tests ---

func TestDomainHealthCheckHandler_HealthyCerts(t *testing.T) {
	d1 := sampleActiveDomain("id-1", "a.example.com", "tok1")
	d2 := sampleActiveDomain("id-2", "b.example.com", "tok2")

	checker := &fakeHealthChecker{domains: []DomainBinding{d1, d2}}
	certMgr := &fakeCertManager{
		getCertResult: &CertInfo{
			NotAfter:  time.Now().Add(60 * 24 * time.Hour),
			SerialHex: "abc123",
		},
	}

	handler := DomainHealthCheckHandler(checker, certMgr, nil)
	err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(checker.healthUpdates) != 2 {
		t.Fatalf("expected 2 health updates, got %d", len(checker.healthUpdates))
	}
	for _, u := range checker.healthUpdates {
		if u.HealthStatus != "healthy" {
			t.Errorf("expected healthy for %s, got %s", u.ID, u.HealthStatus)
		}
	}
}

func TestDomainHealthCheckHandler_UnhealthyCert(t *testing.T) {
	d1 := sampleActiveDomain("id-1", "a.example.com", "tok1")
	d2 := sampleActiveDomain("id-2", "b.example.com", "tok2")

	checker := &fakeHealthChecker{domains: []DomainBinding{d1, d2}}

	// Return healthy cert for first host, error for second.
	certMgr := &perHostFakeCertManager{
		results: map[string]*CertInfo{
			"a.example.com": {
				NotAfter:  time.Now().Add(60 * 24 * time.Hour),
				SerialHex: "abc123",
			},
		},
		errors: map[string]error{
			"b.example.com": ErrCertNotFound,
		},
	}

	handler := DomainHealthCheckHandler(checker, certMgr, nil)
	err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(checker.healthUpdates) != 2 {
		t.Fatalf("expected 2 health updates, got %d", len(checker.healthUpdates))
	}
	if checker.healthUpdates[0].HealthStatus != "healthy" {
		t.Errorf("expected healthy for id-1, got %s", checker.healthUpdates[0].HealthStatus)
	}
	if checker.healthUpdates[1].HealthStatus != "unhealthy" {
		t.Errorf("expected unhealthy for id-2, got %s", checker.healthUpdates[1].HealthStatus)
	}
}

func TestDomainHealthCheckHandler_NoDomains(t *testing.T) {
	checker := &fakeHealthChecker{domains: []DomainBinding{}}
	certMgr := &fakeCertManager{}

	handler := DomainHealthCheckHandler(checker, certMgr, nil)
	err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(checker.healthUpdates) != 0 {
		t.Errorf("expected 0 health updates, got %d", len(checker.healthUpdates))
	}
}

func TestDomainHealthCheckHandler_SkipNoCertRef(t *testing.T) {
	d := sampleActiveDomain("id-1", "a.example.com", "tok1")
	d.CertRef = nil // not yet provisioned

	checker := &fakeHealthChecker{domains: []DomainBinding{d}}
	certMgr := &fakeCertManager{}

	handler := DomainHealthCheckHandler(checker, certMgr, nil)
	err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(checker.healthUpdates) != 0 {
		t.Errorf("expected 0 health updates (skipped no cert), got %d", len(checker.healthUpdates))
	}
}

func TestDomainHealthCheckHandler_ExpiredCert(t *testing.T) {
	d := sampleActiveDomain("id-1", "a.example.com", "tok1")

	checker := &fakeHealthChecker{domains: []DomainBinding{d}}
	certMgr := &fakeCertManager{
		getCertResult: &CertInfo{
			NotAfter:  time.Now().Add(-24 * time.Hour), // expired yesterday
			SerialHex: "abc123",
		},
	}

	handler := DomainHealthCheckHandler(checker, certMgr, nil)
	err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(checker.healthUpdates) != 1 {
		t.Fatalf("expected 1 health update, got %d", len(checker.healthUpdates))
	}
	if checker.healthUpdates[0].HealthStatus != "unhealthy" {
		t.Errorf("expected unhealthy for expired cert, got %s", checker.healthUpdates[0].HealthStatus)
	}
}

// --- Re-verify handler tests ---

func TestDomainReverifyHandler_DNSStillValid(t *testing.T) {
	token := "aabbccdd00112233"
	d := sampleActiveDomain("id-1", "app.example.com", token)

	reverifier := &fakeReverifier{
		domainsForReverify: []DomainBinding{d},
	}
	mgr := &fakeDomainManager{domains: []DomainBinding{d}}
	resolver := &fakeResolverForReverify{
		records: map[string][]string{
			"_ayb-challenge.app.example.com": {token},
		},
	}

	handler := DomainReverifyHandler(reverifier, mgr, resolver, nil)
	err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(reverifier.resetCalls) != 1 || reverifier.resetCalls[0] != "id-1" {
		t.Errorf("expected ResetReverifyFailures called for id-1, got %v", reverifier.resetCalls)
	}
	if len(reverifier.incrementCalls) != 0 {
		t.Errorf("expected no IncrementReverifyFailures calls, got %v", reverifier.incrementCalls)
	}
}

func TestDomainReverifyHandler_DNSFailed_ThresholdReached(t *testing.T) {
	token := "aabbccdd00112233"
	d := sampleActiveDomain("id-1", "app.example.com", token)
	d.ReverifyFailures = 2 // Already at 2, will hit threshold of 3 on this failure.

	reverifier := &fakeReverifier{
		domainsForReverify: []DomainBinding{d},
	}
	mgr := &fakeDomainManager{domains: []DomainBinding{d}}
	resolver := &fakeResolverForReverify{
		records: map[string][]string{}, // No records — failure.
	}

	handler := DomainReverifyHandler(reverifier, mgr, resolver, nil)
	err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(reverifier.incrementCalls) != 1 || reverifier.incrementCalls[0] != "id-1" {
		t.Errorf("expected IncrementReverifyFailures called for id-1, got %v", reverifier.incrementCalls)
	}

	// Verify UpdateDomainStatus was called with StatusVerificationLapsed.
	found := false
	for _, d := range mgr.domains {
		if d.ID == "id-1" && d.Status == StatusVerificationLapsed {
			found = true
		}
	}
	if !found {
		t.Error("expected domain id-1 to be in verification_lapsed status")
	}
}

func TestDomainReverifyHandler_LapsedCleanup(t *testing.T) {
	d := sampleActiveDomain("id-lapsed", "old.example.com", "tok")
	d.Status = StatusVerificationLapsed

	reverifier := &fakeReverifier{
		domainsForReverify: []DomainBinding{}, // No active domains to re-verify.
		lapsedDomains:      []DomainBinding{d},
	}
	mgr := &fakeDomainManager{domains: []DomainBinding{d}}

	handler := DomainReverifyHandler(reverifier, mgr, &fakeResolverForReverify{}, nil)
	err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// Verify DeleteDomain was called — domain should be removed from mgr.domains.
	for _, d := range mgr.domains {
		if d.ID == "id-lapsed" {
			t.Error("expected domain id-lapsed to be deleted (tombstoned)")
		}
	}
}

func TestDomainReverifyHandler_DNSLookupError(t *testing.T) {
	token := "aabbccdd00112233"
	d := sampleActiveDomain("id-1", "app.example.com", token)
	d.ReverifyFailures = 0

	reverifier := &fakeReverifier{
		domainsForReverify: []DomainBinding{d},
	}
	mgr := &fakeDomainManager{domains: []DomainBinding{d}}
	resolver := &fakeResolverForReverify{
		err: errors.New("DNS timeout"),
	}

	handler := DomainReverifyHandler(reverifier, mgr, resolver, nil)
	err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(reverifier.incrementCalls) != 1 {
		t.Errorf("expected 1 increment call, got %d", len(reverifier.incrementCalls))
	}
	// Should NOT lapse yet — only 1 failure, threshold is 3.
	for _, d := range mgr.domains {
		if d.ID == "id-1" && d.Status != StatusActive {
			t.Errorf("expected domain to remain active, got %s", d.Status)
		}
	}
}

// --- Verify rate limiting test ---

func TestVerifyRateLimiting(t *testing.T) {
	mgr := &fakeDomainManager{
		domains: []DomainBinding{
			sampleActiveDomain("00000000-0000-0000-0000-000000000001", "app.example.com", "tok"),
		},
	}
	rl := auth.NewRateLimiter(2, time.Minute)
	defer rl.Stop()

	handler := handleAdminTriggerDomainVerify(mgr, rl)

	r := chi.NewRouter()
	r.Post("/api/admin/domains/{id}/verify", handler)

	domainID := "00000000-0000-0000-0000-000000000001"

	// First two requests should succeed.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/domains/"+domainID+"/verify", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d: %s", i+1, w.Code, w.Body.String())
		}
		if w.Header().Get("X-RateLimit-Remaining") == "" {
			t.Errorf("request %d: expected X-RateLimit-Remaining header", i+1)
		}
	}

	// Third request should be rate limited.
	req := httptest.NewRequest(http.MethodPost, "/api/admin/domains/"+domainID+"/verify", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}

// perHostFakeCertManager returns different results per hostname.
type perHostFakeCertManager struct {
	results map[string]*CertInfo
	errors  map[string]error
}

func (f *perHostFakeCertManager) ManageDomain(_ context.Context, hostname string) error {
	return nil
}

func (f *perHostFakeCertManager) UnmanageDomain(hostname string) error {
	return nil
}

func (f *perHostFakeCertManager) GetCertificate(_ context.Context, hostname string) (*CertInfo, error) {
	if err, ok := f.errors[hostname]; ok {
		return nil, err
	}
	if info, ok := f.results[hostname]; ok {
		result := *info
		result.Hostname = hostname
		return &result, nil
	}
	return nil, ErrCertNotFound
}
