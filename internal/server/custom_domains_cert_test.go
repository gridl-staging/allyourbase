package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

// fakeCertManager is an in-memory fake for testing cert job handlers.
type fakeCertManager struct {
	manageDomainErr   error
	unmanageDomainErr error
	getCertResult     *CertInfo
	getCertErr        error
	managedDomains    []string
	unmanagedDomains  []string
}

func (f *fakeCertManager) ManageDomain(_ context.Context, hostname string) error {
	f.managedDomains = append(f.managedDomains, hostname)
	return f.manageDomainErr
}

func (f *fakeCertManager) UnmanageDomain(hostname string) error {
	f.unmanagedDomains = append(f.unmanagedDomains, hostname)
	return f.unmanageDomainErr
}

func (f *fakeCertManager) GetCertificate(_ context.Context, hostname string) (*CertInfo, error) {
	if f.getCertErr != nil {
		return nil, f.getCertErr
	}
	if f.getCertResult != nil {
		result := *f.getCertResult
		result.Hostname = hostname
		return &result, nil
	}
	return nil, ErrCertNotFound
}

// sampleVerifiedDomain returns a DomainBinding in verified status for testing.
func sampleVerifiedDomain() DomainBinding {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	token := "aabbccdd00112233aabbccdd00112233aabbccdd00112233aabbccdd00112233"
	return DomainBinding{
		ID:                "00000000-0000-0000-0000-000000000010",
		Hostname:          "app.example.com",
		Environment:       "production",
		Status:            StatusVerified,
		VerificationToken: token,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	testutil.NoError(t, err)
	return b
}

// --- DomainCertProvisionHandler ---

func TestDomainCertProvisionHandlerSuccess(t *testing.T) {
	t.Parallel()

	expiry := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	certInfo := &CertInfo{
		Issuer:    "Let's Encrypt",
		NotAfter:  expiry,
		SerialHex: "deadbeef",
	}

	domain := sampleVerifiedDomain()
	mgr := &fakeDomainManager{domains: []DomainBinding{domain}}
	certMgr := &fakeCertManager{getCertResult: certInfo}

	handler := DomainCertProvisionHandler(mgr, certMgr, nil)
	payload := mustMarshal(t, domainCertProvisionPayload{DomainID: domain.ID})

	err := handler(context.Background(), payload)
	testutil.NoError(t, err)

	// certMgr.ManageDomain called with correct hostname
	testutil.Equal(t, 1, len(certMgr.managedDomains))
	testutil.Equal(t, "app.example.com", certMgr.managedDomains[0])

	// domain transitioned to active with cert metadata
	updated, getErr := mgr.GetDomain(context.Background(), domain.ID)
	testutil.NoError(t, getErr)
	testutil.Equal(t, string(StatusActive), string(updated.Status))
	testutil.True(t, updated.CertRef != nil, "expected CertRef to be set")
	testutil.Equal(t, "deadbeef", *updated.CertRef)
	testutil.True(t, updated.CertExpiry != nil, "expected CertExpiry to be set")
	testutil.Equal(t, expiry, *updated.CertExpiry)
}

func TestDomainCertProvisionHandlerStaleJobDomainNotFound(t *testing.T) {
	t.Parallel()

	mgr := &fakeDomainManager{domains: []DomainBinding{}}
	certMgr := &fakeCertManager{}

	handler := DomainCertProvisionHandler(mgr, certMgr, nil)
	payload := mustMarshal(t, domainCertProvisionPayload{DomainID: "00000000-0000-0000-0000-000000000099"})

	err := handler(context.Background(), payload)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(certMgr.managedDomains))
}

func TestDomainCertProvisionHandlerStaleJobWrongStatus(t *testing.T) {
	t.Parallel()

	for _, status := range []DomainStatus{StatusActive, StatusTombstoned, StatusPendingVerification, StatusVerificationFailed} {
		status := status
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			domain := sampleVerifiedDomain()
			domain.Status = status
			mgr := &fakeDomainManager{domains: []DomainBinding{domain}}
			certMgr := &fakeCertManager{}

			handler := DomainCertProvisionHandler(mgr, certMgr, nil)
			payload := mustMarshal(t, domainCertProvisionPayload{DomainID: domain.ID})

			err := handler(context.Background(), payload)
			testutil.NoError(t, err)
			testutil.Equal(t, 0, len(certMgr.managedDomains))
		})
	}
}

func TestDomainCertProvisionHandlerIssuanceFailure(t *testing.T) {
	t.Parallel()

	issuanceErr := errors.New("ACME rate limit exceeded")
	domain := sampleVerifiedDomain()
	mgr := &fakeDomainManager{domains: []DomainBinding{domain}}
	certMgr := &fakeCertManager{manageDomainErr: issuanceErr}

	handler := DomainCertProvisionHandler(mgr, certMgr, nil)
	payload := mustMarshal(t, domainCertProvisionPayload{DomainID: domain.ID})

	err := handler(context.Background(), payload)
	testutil.True(t, err != nil, "expected error from issuance failure")
	testutil.Contains(t, err.Error(), "manage domain")

	// domain stays verified with last_error set
	updated, getErr := mgr.GetDomain(context.Background(), domain.ID)
	testutil.NoError(t, getErr)
	testutil.Equal(t, string(StatusVerified), string(updated.Status))
	testutil.True(t, updated.LastError != nil, "expected LastError to be set")
	testutil.Contains(t, *updated.LastError, "ACME rate limit exceeded")
}

// --- DomainCertRevokeHandler ---

func TestDomainCertRevokeHandlerSuccess(t *testing.T) {
	t.Parallel()

	certMgr := &fakeCertManager{}
	handler := DomainCertRevokeHandler(certMgr, nil)
	payload := mustMarshal(t, domainCertRevokePayload{
		DomainID: "00000000-0000-0000-0000-000000000001",
		Hostname: "app.example.com",
	})

	err := handler(context.Background(), payload)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(certMgr.unmanagedDomains))
	testutil.Equal(t, "app.example.com", certMgr.unmanagedDomains[0])
}

func TestDomainCertRevokeHandlerFailure(t *testing.T) {
	t.Parallel()

	revokeErr := errors.New("certmagic error")
	certMgr := &fakeCertManager{unmanageDomainErr: revokeErr}
	handler := DomainCertRevokeHandler(certMgr, nil)
	payload := mustMarshal(t, domainCertRevokePayload{
		DomainID: "00000000-0000-0000-0000-000000000001",
		Hostname: "app.example.com",
	})

	err := handler(context.Background(), payload)
	testutil.True(t, err != nil, "expected error from revoke failure")
	testutil.Contains(t, err.Error(), "certmagic error")
}

// --- DomainCertRenewHandler ---

func TestDomainCertRenewHandlerSyncsMetadata(t *testing.T) {
	t.Parallel()

	now := time.Now()
	oldExpiry := now.Add(10 * 24 * time.Hour) // within 30 days — qualifies for renewal
	newExpiry := now.Add(90 * 24 * time.Hour) // certmagic renewed

	serialHex := "abc123"
	certRef := "oldhex"
	domain := DomainBinding{
		ID:                "00000000-0000-0000-0000-000000000020",
		Hostname:          "renewed.example.com",
		Environment:       "production",
		Status:            StatusActive,
		VerificationToken: "tok",
		CertRef:           &certRef,
		CertExpiry:        &oldExpiry,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	mgr := &fakeDomainManager{domains: []DomainBinding{domain}}
	certMgr := &fakeCertManager{
		getCertResult: &CertInfo{
			Issuer:    "Let's Encrypt",
			NotAfter:  newExpiry,
			SerialHex: serialHex,
		},
	}

	handler := DomainCertRenewHandler(mgr, certMgr, nil)
	err := handler(context.Background(), nil)
	testutil.NoError(t, err)

	// cert metadata updated in DB
	updated, getErr := mgr.GetDomain(context.Background(), domain.ID)
	testutil.NoError(t, getErr)
	testutil.True(t, updated.CertExpiry != nil, "expected CertExpiry to be updated")
	testutil.Equal(t, newExpiry, *updated.CertExpiry)
	testutil.True(t, updated.CertRef != nil, "expected CertRef to be updated")
	testutil.Equal(t, serialHex, *updated.CertRef)
}

func TestDomainCertRenewHandlerNoDomains(t *testing.T) {
	t.Parallel()

	mgr := &fakeDomainManager{domains: []DomainBinding{}}
	certMgr := &fakeCertManager{}

	handler := DomainCertRenewHandler(mgr, certMgr, nil)
	err := handler(context.Background(), nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(certMgr.managedDomains))
	testutil.Equal(t, 0, len(certMgr.unmanagedDomains))
}

func TestDomainCertRenewHandlerNoUpdateWhenExpiryUnchanged(t *testing.T) {
	t.Parallel()

	now := time.Now()
	expiry := now.Add(20 * 24 * time.Hour) // within 30 days
	certRef := "deadbeef"
	domain := DomainBinding{
		ID:                "00000000-0000-0000-0000-000000000021",
		Hostname:          "same.example.com",
		Environment:       "production",
		Status:            StatusActive,
		VerificationToken: "tok",
		CertRef:           &certRef,
		CertExpiry:        &expiry,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	mgr := &fakeDomainManager{domains: []DomainBinding{domain}}
	certMgr := &fakeCertManager{
		getCertResult: &CertInfo{
			Issuer:    "Let's Encrypt",
			NotAfter:  expiry, // same as DB — no update needed
			SerialHex: certRef,
		},
	}

	handler := DomainCertRenewHandler(mgr, certMgr, nil)
	err := handler(context.Background(), nil)
	testutil.NoError(t, err)
	// Status should remain active — no unnecessary update
	updated, _ := mgr.GetDomain(context.Background(), domain.ID)
	testutil.Equal(t, string(StatusActive), string(updated.Status))
}
