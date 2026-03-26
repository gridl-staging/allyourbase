// Package server This file implements TLS certificate management for custom domains using certmagic, with job handlers for certificate provisioning, revocation, and renewal.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/caddyserver/certmagic"
)

// ErrCertNotFound is returned when certmagic has no cached certificate for a hostname.
var ErrCertNotFound = errors.New("certificate not found")

// CertInfo holds certificate metadata extracted from certmagic's cache.
type CertInfo struct {
	Hostname  string
	Issuer    string
	NotAfter  time.Time
	SerialHex string
}

// CertManager manages TLS certificates for custom domain bindings.
type CertManager interface {
	// ManageDomain begins ACME certificate management for hostname.
	// Blocks until issuance completes (appropriate because this runs inside an async job).
	ManageDomain(ctx context.Context, hostname string) error
	// UnmanageDomain stops certmagic from renewing the certificate for hostname.
	// Does NOT revoke with the CA. Safe to call if hostname is not managed (no-op).
	UnmanageDomain(hostname string) error
	// GetCertificate returns cached certificate metadata for hostname.
	// Returns ErrCertNotFound if no certificate is in the cache.
	GetCertificate(ctx context.Context, hostname string) (*CertInfo, error)
}

// certmagicCertManager implements CertManager using certmagic.
type certmagicCertManager struct {
	magic *certmagic.Config
	cache *certmagic.Cache
}

// NewCertmagicCertManager wraps a certmagic Config and Cache as a CertManager.
// The cache is needed for UnmanageDomain (Cache.RemoveManaged); the config is used for
// ManageDomain (Config.ManageSync) and GetCertificate (Config.CacheManagedCertificate).
func NewCertmagicCertManager(magic *certmagic.Config, cache *certmagic.Cache) CertManager {
	return &certmagicCertManager{magic: magic, cache: cache}
}

func (m *certmagicCertManager) ManageDomain(ctx context.Context, hostname string) error {
	return m.magic.ManageSync(ctx, []string{hostname})
}

func (m *certmagicCertManager) UnmanageDomain(hostname string) error {
	m.cache.RemoveManaged([]certmagic.SubjectIssuer{{Subject: hostname}})
	return nil
}

// GetCertificate returns cached certificate metadata for hostname. Returns ErrCertNotFound if no certificate is cached or if the certificate has no leaf.
func (m *certmagicCertManager) GetCertificate(ctx context.Context, hostname string) (*CertInfo, error) {
	cert, err := m.magic.CacheManagedCertificate(ctx, hostname)
	if err != nil {
		return nil, ErrCertNotFound
	}
	if cert.Leaf == nil {
		slog.Default().Warn("certmagic certificate has nil Leaf", "hostname", hostname)
		return nil, ErrCertNotFound
	}
	return &CertInfo{
		Hostname:  hostname,
		Issuer:    cert.Leaf.Issuer.CommonName,
		NotAfter:  cert.Leaf.NotAfter,
		SerialHex: cert.Leaf.SerialNumber.Text(16),
	}, nil
}

// Job type constants for certificate lifecycle jobs.
const (
	JobTypeDomainCertProvision = "domain_cert_provision"
	JobTypeDomainCertRevoke    = "domain_cert_revoke"
	JobTypeDomainCertRenew     = "domain_cert_renew"
)

type domainCertProvisionPayload struct {
	DomainID string `json:"domain_id"`
}

type domainCertRevokePayload struct {
	DomainID string `json:"domain_id"`
	Hostname string `json:"hostname"`
}

// DomainCertProvisionHandler returns a job handler that provisions a TLS certificate
// for a newly verified domain. The job is enqueued automatically when a domain
// transitions to StatusVerified.
func DomainCertProvisionHandler(mgr domainManager, certMgr CertManager, logger *slog.Logger) jobs.JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p domainCertProvisionPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("domain_cert_provision: invalid payload: %w", err)
		}

		domain, err := mgr.GetDomain(ctx, p.DomainID)
		if err != nil {
			if errors.Is(err, ErrDomainNotFound) {
				return nil // stale job — domain deleted
			}
			return fmt.Errorf("domain_cert_provision: get domain: %w", err)
		}
		if domain.Status != StatusVerified {
			// stale: already active, tombstoned, or re-failed
			return nil
		}

		if err := certMgr.ManageDomain(ctx, domain.Hostname); err != nil {
			errMsg := fmt.Sprintf("cert issuance failed: %v", err)
			if _, updateErr := mgr.UpdateDomainStatus(ctx, domain.ID, StatusVerified, &errMsg); updateErr != nil {
				if logger != nil {
					logger.Warn("domain_cert_provision: failed to record issuance error",
						"domain_id", domain.ID, "error", updateErr)
				}
			}
			return fmt.Errorf("domain_cert_provision: manage domain: %w", err)
		}

		certInfo, err := certMgr.GetCertificate(ctx, domain.Hostname)
		if err != nil {
			return fmt.Errorf("domain_cert_provision: get certificate metadata: %w", err)
		}

		if _, err := mgr.SetDomainCert(ctx, domain.ID, certInfo.SerialHex, certInfo.NotAfter); err != nil {
			return fmt.Errorf("domain_cert_provision: set domain cert: %w", err)
		}

		if logger != nil {
			logger.Info("domain certificate provisioned",
				"domain_id", domain.ID,
				"hostname", domain.Hostname,
				"cert_expiry", certInfo.NotAfter,
			)
		}
		return nil
	}
}

// DomainCertRevokeHandler returns a job handler that stops certmagic from
// renewing the certificate for a tombstoned domain. The payload carries the
// hostname directly because the domain may be tombstoned before the job runs.
func DomainCertRevokeHandler(certMgr CertManager, logger *slog.Logger) jobs.JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p domainCertRevokePayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("domain_cert_revoke: invalid payload: %w", err)
		}

		if err := certMgr.UnmanageDomain(p.Hostname); err != nil {
			if logger != nil {
				logger.Warn("domain_cert_revoke: failed to unmanage domain",
					"domain_id", p.DomainID, "hostname", p.Hostname, "error", err)
			}
			return fmt.Errorf("domain_cert_revoke: %w", err)
		}

		if logger != nil {
			logger.Info("domain certificate revoked",
				"domain_id", p.DomainID, "hostname", p.Hostname)
		}
		return nil
	}
}

// DomainCertRenewHandler returns a job handler that syncs certificate expiry
// metadata from certmagic's cache into the database for all active domains
// with certs expiring within 30 days. Certmagic handles actual renewal internally;
// this job only syncs metadata and logs alerts for certs that certmagic failed to renew.
func DomainCertRenewHandler(mgr domainManager, certMgr CertManager, logger *slog.Logger) jobs.JobHandler {
	return func(ctx context.Context, _ json.RawMessage) error {
		renewBefore := time.Now().Add(30 * 24 * time.Hour)
		domains, err := mgr.ListDomainsForCertRenewal(ctx, renewBefore)
		if err != nil {
			return fmt.Errorf("domain_cert_renew: list domains: %w", err)
		}

		for _, domain := range domains {
			certInfo, err := certMgr.GetCertificate(ctx, domain.Hostname)
			if err != nil {
				if logger != nil {
					logger.Warn("domain_cert_renew: failed to get certificate",
						"domain_id", domain.ID, "hostname", domain.Hostname, "error", err)
				}
				continue
			}

			// Sync updated expiry if certmagic renewed internally.
			if domain.CertExpiry == nil || !certInfo.NotAfter.Equal(*domain.CertExpiry) {
				if _, err := mgr.SetDomainCert(ctx, domain.ID, certInfo.SerialHex, certInfo.NotAfter); err != nil {
					if logger != nil {
						logger.Warn("domain_cert_renew: failed to sync cert metadata",
							"domain_id", domain.ID, "hostname", domain.Hostname, "error", err)
					}
					continue
				}
			}

			// Alert if cert is still expiring within 7 days despite certmagic's best efforts.
			daysUntilExpiry := time.Until(certInfo.NotAfter).Hours() / 24
			if daysUntilExpiry < 7 {
				if logger != nil {
					logger.Log(ctx, slog.LevelWarn, "domain certificate expiry critical",
						"domain_id", domain.ID,
						"hostname", domain.Hostname,
						"days_until_expiry", daysUntilExpiry,
					)
				}
			}
		}

		return nil
	}
}

// RegisterDomainCertRenewSchedule registers a twice-daily schedule for certificate renewal checks.
func RegisterDomainCertRenewSchedule(ctx context.Context, svc *jobs.Service) error {
	return registerDomainSchedule(ctx, svc, "domain_cert_renew_12h", JobTypeDomainCertRenew, "0 */12 * * *")
}
