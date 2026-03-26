package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/allyourbase/ayb/internal/jobs"
)

// DomainHealthChecker is the interface for health check operations on custom domains.
// *DomainStore satisfies this interface.
type DomainHealthChecker interface {
	ListDomainsForHealthCheck(ctx context.Context) ([]DomainBinding, error)
	UpdateDomainHealth(ctx context.Context, id string, healthStatus string) error
}

// DomainReverifier is the interface for DNS re-verification operations.
// *DomainStore satisfies this interface.
type DomainReverifier interface {
	ListDomainsForReverify(ctx context.Context) ([]DomainBinding, error)
	IncrementReverifyFailures(ctx context.Context, id string) error
	ResetReverifyFailures(ctx context.Context, id string) error
	ListLapsedDomainsForCleanup(ctx context.Context) ([]DomainBinding, error)
}

const (
	JobTypeDomainHealthCheck = "domain_health_check"
	JobTypeDomainReverify    = "domain_reverify"
)

// reverifyFailureThreshold is the number of consecutive DNS re-verification failures
// before a domain transitions to verification_lapsed.
const reverifyFailureThreshold = 3

// DomainHealthCheckHandler returns a job handler that checks certificate validity
// for all active domains and updates their health status. Domains without a cert
// (CertRef == nil) are skipped. Individual failures are logged but do not fail the
// entire job.
func DomainHealthCheckHandler(checker DomainHealthChecker, certMgr CertManager, logger *slog.Logger) jobs.JobHandler {
	return func(ctx context.Context, _ json.RawMessage) error {
		domains, err := checker.ListDomainsForHealthCheck(ctx)
		if err != nil {
			return fmt.Errorf("domain_health_check: list domains: %w", err)
		}

		for _, domain := range domains {
			if domain.CertRef == nil {
				continue
			}

			certInfo, err := certMgr.GetCertificate(ctx, domain.Hostname)
			if err != nil {
				if logger != nil {
					logger.Warn("domain_health_check: cert unhealthy",
						"domain_id", domain.ID, "hostname", domain.Hostname, "error", err)
				}
				if updateErr := checker.UpdateDomainHealth(ctx, domain.ID, "unhealthy"); updateErr != nil {
					if logger != nil {
						logger.Warn("domain_health_check: failed to update health",
							"domain_id", domain.ID, "error", updateErr)
					}
				}
				continue
			}

			// Check if cert is expired.
			if time.Now().After(certInfo.NotAfter) {
				if logger != nil {
					logger.Warn("domain_health_check: cert expired",
						"domain_id", domain.ID, "hostname", domain.Hostname,
						"expired_at", certInfo.NotAfter)
				}
				if updateErr := checker.UpdateDomainHealth(ctx, domain.ID, "unhealthy"); updateErr != nil {
					if logger != nil {
						logger.Warn("domain_health_check: failed to update health",
							"domain_id", domain.ID, "error", updateErr)
					}
				}
				continue
			}

			// SLO alert: cert expiring within 7 days.
			daysUntilExpiry := time.Until(certInfo.NotAfter).Hours() / 24
			if daysUntilExpiry < 7 {
				if logger != nil {
					logger.Log(ctx, slog.LevelWarn, "domain certificate expiry warning",
						"domain_id", domain.ID,
						"hostname", domain.Hostname,
						"days_until_expiry", daysUntilExpiry,
					)
				}
			}

			if updateErr := checker.UpdateDomainHealth(ctx, domain.ID, "healthy"); updateErr != nil {
				if logger != nil {
					logger.Warn("domain_health_check: failed to update health",
						"domain_id", domain.ID, "error", updateErr)
				}
			}
		}

		return nil
	}
}

// RegisterDomainHealthCheckSchedule registers a 15-minute schedule for domain health checks.
func RegisterDomainHealthCheckSchedule(ctx context.Context, svc *jobs.Service) error {
	return registerDomainSchedule(ctx, svc, "domain_health_check_15m", JobTypeDomainHealthCheck, "*/15 * * * *")
}

// DomainReverifyHandler returns a job handler that periodically re-checks DNS ownership
// for all active domains. Two phases:
//  1. Re-verify: for every active domain, re-check DNS TXT record. On success, reset
//     failure counter. On failure, increment counter. If counter reaches threshold (3),
//     transition to verification_lapsed.
//  2. Cleanup: tombstone lapsed domains past the 7-day grace period.
func DomainReverifyHandler(reverifier DomainReverifier, mgr domainManager, resolver DNSResolver, logger *slog.Logger) jobs.JobHandler {
	return func(ctx context.Context, _ json.RawMessage) error {
		// Phase 1: re-verify active domains.
		domains, err := reverifier.ListDomainsForReverify(ctx)
		if err != nil {
			return fmt.Errorf("domain_reverify: list domains: %w", err)
		}

		for _, domain := range domains {
			challengeHostname := "_ayb-challenge." + domain.Hostname
			txtRecords, lookupErr := resolver.LookupTXT(ctx, challengeHostname)

			verified := false
			if lookupErr == nil {
				for _, txt := range txtRecords {
					if txt == domain.VerificationToken {
						verified = true
						break
					}
				}
			}

			if verified {
				if resetErr := reverifier.ResetReverifyFailures(ctx, domain.ID); resetErr != nil {
					if logger != nil {
						logger.Warn("domain_reverify: failed to reset failures",
							"domain_id", domain.ID, "error", resetErr)
					}
				}
				continue
			}

			// DNS verification failed.
			if incErr := reverifier.IncrementReverifyFailures(ctx, domain.ID); incErr != nil {
				if logger != nil {
					logger.Warn("domain_reverify: failed to increment failures",
						"domain_id", domain.ID, "error", incErr)
				}
			}

			if domain.ReverifyFailures+1 >= reverifyFailureThreshold {
				errMsg := fmt.Sprintf("DNS re-verification failed %d times", domain.ReverifyFailures+1)
				if _, updateErr := mgr.UpdateDomainStatus(ctx, domain.ID, StatusVerificationLapsed, &errMsg); updateErr != nil {
					if logger != nil {
						logger.Warn("domain_reverify: failed to lapse domain",
							"domain_id", domain.ID, "error", updateErr)
					}
				} else if logger != nil {
					logger.Warn("domain_reverify: domain lapsed",
						"domain_id", domain.ID, "hostname", domain.Hostname,
						"failures", domain.ReverifyFailures+1)
				}
			}
		}

		// Phase 2: cleanup lapsed domains past grace period.
		lapsed, err := reverifier.ListLapsedDomainsForCleanup(ctx)
		if err != nil {
			if logger != nil {
				logger.Warn("domain_reverify: failed to list lapsed domains", "error", err)
			}
			return nil // Don't fail the whole job for cleanup errors.
		}

		for _, domain := range lapsed {
			if deleteErr := mgr.DeleteDomain(ctx, domain.ID); deleteErr != nil {
				if logger != nil {
					logger.Warn("domain_reverify: failed to tombstone lapsed domain",
						"domain_id", domain.ID, "hostname", domain.Hostname, "error", deleteErr)
				}
			} else if logger != nil {
				logger.Info("domain_reverify: tombstoned lapsed domain",
					"domain_id", domain.ID, "hostname", domain.Hostname)
			}
		}

		return nil
	}
}

// RegisterDomainReverifySchedule registers a daily schedule for DNS re-verification.
func RegisterDomainReverifySchedule(ctx context.Context, svc *jobs.Service) error {
	return registerDomainSchedule(ctx, svc, "domain_reverify_daily", JobTypeDomainReverify, "0 4 * * *")
}
