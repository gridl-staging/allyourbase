// Package server implements DNS-based custom domain verification with TXT record polling and job-based retries that use fixed polling bands.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"time"

	"github.com/allyourbase/ayb/internal/jobs"
)

const JobTypeDomainDNSVerify = "domain_dns_verify"

// DNSResolver resolves TXT records for domain verification.
type DNSResolver interface {
	LookupTXT(ctx context.Context, hostname string) ([]string, error)
}

type netDNSResolver struct{}

func (r netDNSResolver) LookupTXT(ctx context.Context, hostname string) ([]string, error) {
	records, err := net.DefaultResolver.LookupTXT(ctx, hostname)
	if err != nil {
		return nil, err
	}

	stripped := make([]string, 0, len(records))
	for _, record := range records {
		stripped = append(stripped, trimTrailingDot(record))
	}
	return stripped, nil
}

func NewNetDNSResolver() DNSResolver {
	return netDNSResolver{}
}

// JobEnqueuer enqueues jobs as part of the DNS verification retry flow.
type JobEnqueuer interface {
	Enqueue(ctx context.Context, jobType string, payload json.RawMessage, opts jobs.EnqueueOpts) (*jobs.Job, error)
}

type domainVerifyPayload struct {
	DomainID  string    `json:"domain_id"`
	StartedAt time.Time `json:"started_at"`
	Attempt   int       `json:"attempt"`
}

func verifyRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	// Intentionally not exponential: use two fixed polling bands for DNS propagation.
	if attempt <= 20 {
		return 30*time.Second + time.Duration(rand.Intn(3))*time.Second
	}
	return 5*time.Minute + time.Duration(rand.Intn(11))*time.Second
}

func verifyTimedOut(startedAt time.Time) bool {
	return time.Since(startedAt) > 24*time.Hour
}

// DomainDNSVerifyHandler creates a job handler that polls TXT records until a domain token
// matches, then advances status to verified.
func DomainDNSVerifyHandler(mgr domainManager, resolver DNSResolver, enqueuer JobEnqueuer, logger *slog.Logger) jobs.JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p domainVerifyPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("domain_dns_verify: invalid payload: %w", err)
		}

		domain, err := mgr.GetDomain(ctx, p.DomainID)
		if err != nil {
			if errors.Is(err, ErrDomainNotFound) {
				return nil
			}
			return fmt.Errorf("domain_dns_verify: %w", err)
		}
		if domain.Status != StatusPendingVerification {
			return nil
		}

		challengeHostname := "_ayb-challenge." + domain.Hostname
		txtRecords, err := resolver.LookupTXT(ctx, challengeHostname)
		if err != nil {
			errMsg := fmt.Sprintf("DNS lookup failed for %s: %v", challengeHostname, err)
			return handleVerifyOutcome(ctx, p, domain, errMsg, mgr, enqueuer, logger)
		}

		for _, txt := range txtRecords {
			if txt == domain.VerificationToken {
				if _, err := mgr.UpdateDomainStatus(ctx, domain.ID, StatusVerified, nil); err != nil {
					return fmt.Errorf("domain_dns_verify: mark verified: %w", err)
				}
				if logger != nil {
					logger.Info("domain verification succeeded", "domain_id", domain.ID, "hostname", domain.Hostname)
				}
				return nil
			}
		}

		if len(txtRecords) == 0 {
			errMsg := fmt.Sprintf("no TXT record found at %s", challengeHostname)
			return handleVerifyOutcome(ctx, p, domain, errMsg, mgr, enqueuer, logger)
		}

		errMsg := fmt.Sprintf("TXT record found at %s but value does not match expected token", challengeHostname)
		return handleVerifyOutcome(ctx, p, domain, errMsg, mgr, enqueuer, logger)
	}
}

// handleVerifyOutcome updates domain verification status after a failed DNS lookup attempt, either marking the domain permanently failed if the 24-hour timeout has elapsed or enqueuing a retry using the fixed-band polling schedule.
func handleVerifyOutcome(
	ctx context.Context,
	p domainVerifyPayload,
	domain *DomainBinding,
	errMsg string,
	mgr domainManager,
	enqueuer JobEnqueuer,
	logger *slog.Logger,
) error {
	status := StatusPendingVerification
	if verifyTimedOut(p.StartedAt) {
		status = StatusVerificationFailed
	}

	updated, err := mgr.UpdateDomainStatus(ctx, domain.ID, status, &errMsg)
	if err != nil {
		return fmt.Errorf("domain_dns_verify: update status: %w", err)
	}
	if updated.Status == StatusVerificationFailed {
		if logger != nil {
			logger.Info("domain verification failed", "domain_id", domain.ID, "hostname", domain.Hostname, "last_error", errMsg)
		}
		return nil
	}

	if enqueuer == nil {
		return fmt.Errorf("domain_dns_verify: enqueuer not configured")
	}

	nextPayload := domainVerifyPayload{
		DomainID:  domain.ID,
		StartedAt: p.StartedAt,
		Attempt:   p.Attempt + 1,
	}
	nextPayloadRaw, err := json.Marshal(nextPayload)
	if err != nil {
		return fmt.Errorf("domain_dns_verify: marshal payload: %w", err)
	}

	runAt := time.Now().Add(verifyRetryDelay(p.Attempt))
	if _, err := enqueuer.Enqueue(ctx, JobTypeDomainDNSVerify, nextPayloadRaw, jobs.EnqueueOpts{RunAt: &runAt, MaxAttempts: 1}); err != nil {
		return fmt.Errorf("domain_dns_verify: enqueue retry: %w", err)
	}
	if logger != nil {
		logger.Info("domain verification retry scheduled", "domain_id", domain.ID, "attempt", nextPayload.Attempt, "run_at", runAt)
	}

	return nil
}

func trimTrailingDot(hostname string) string {
	for len(hostname) > 0 && hostname[len(hostname)-1] == '.' {
		hostname = hostname[:len(hostname)-1]
	}
	return hostname
}
