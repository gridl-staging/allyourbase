// Package server custom_domains.go manages custom domain bindings with DNS verification and certificate tracking.
package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode"
)

// DomainStatus represents the lifecycle state of a custom domain binding.
type DomainStatus string

const (
	StatusPendingVerification DomainStatus = "pending_verification"
	StatusVerified            DomainStatus = "verified"
	StatusActive              DomainStatus = "active"
	StatusVerificationFailed  DomainStatus = "verification_failed"
	StatusTombstoned          DomainStatus = "tombstoned"
	StatusVerificationLapsed  DomainStatus = "verification_lapsed"
)

// ErrDomainNotFound is returned when a domain binding doesn't exist.
var ErrDomainNotFound = errors.New("domain not found")

// ErrDomainHostnameConflict is returned when a hostname is already bound (non-tombstoned).
var ErrDomainHostnameConflict = errors.New("hostname already bound")

// DomainBinding represents a row in _ayb_custom_domains.
// VerificationRecord is a computed field (not stored in DB) populated at response time.
// DomainBinding represents a custom domain binding record stored in _ayb_custom_domains, tracking hostname, verification state, certificate metadata, and health status. The VerificationRecord field is computed at response time to provide DNS challenge instructions and is not persisted to the database.
type DomainBinding struct {
	ID                 string       `json:"id"`
	Hostname           string       `json:"hostname"`
	Environment        string       `json:"environment"`
	Status             DomainStatus `json:"status"`
	VerificationToken  string       `json:"verificationToken"`
	VerificationRecord string       `json:"verificationRecord,omitempty"`
	CertRef            *string      `json:"certRef,omitempty"`
	CertExpiry         *time.Time   `json:"certExpiry,omitempty"`
	RedirectMode       *string      `json:"redirectMode,omitempty"`
	LastError          *string      `json:"lastError,omitempty"`
	TombstonedAt       *time.Time   `json:"tombstonedAt,omitempty"`
	CreatedAt          time.Time    `json:"createdAt"`
	UpdatedAt          time.Time    `json:"updatedAt"`
	HealthStatus       string       `json:"healthStatus"`
	LastHealthCheck    *time.Time   `json:"lastHealthCheck,omitempty"`
	ReverifyFailures   int          `json:"reverifyFailures"`
}

// populateVerificationRecord fills in the computed VerificationRecord field.
// The required DNS entry is a TXT record at _ayb-challenge.<hostname> with the token value.
func (b *DomainBinding) populateVerificationRecord() {
	if b != nil {
		b.VerificationRecord = fmt.Sprintf("_ayb-challenge.%s TXT %s", b.Hostname, b.VerificationToken)
	}
}

// DomainBindingListResult is a paginated list of domain bindings.
type DomainBindingListResult struct {
	Items      []DomainBinding `json:"items"`
	Page       int             `json:"page"`
	PerPage    int             `json:"perPage"`
	TotalItems int             `json:"totalItems"`
	TotalPages int             `json:"totalPages"`
}

// normalizeAndValidateHostname normalizes a hostname to lowercase and validates it
// against RFC 952/1123 rules. Returns the normalized hostname and nil on success,
// or empty string and an error on failure.
func normalizeAndValidateHostname(hostname string) (string, error) {
	if hostname == "" {
		return "", errors.New("hostname is required")
	}

	// Reject wildcards.
	if strings.Contains(hostname, "*") {
		return "", errors.New("wildcard hostnames are not supported")
	}

	normalized := strings.ToLower(strings.TrimSpace(hostname))

	// Reject IP literals (v4 and v6) before checking for ':' so that IPv6
	// addresses like "::1" produce the correct error message.
	if ip := net.ParseIP(normalized); ip != nil {
		return "", errors.New("IP addresses are not valid hostnames")
	}

	// Reject port suffixes (must come after IP check so IPv6 literals aren't
	// misclassified as "has port").
	if strings.Contains(normalized, ":") {
		return "", errors.New("hostname must not include a port")
	}

	// Total length check (max 253 chars for a valid DNS name).
	if len(normalized) > 253 {
		return "", fmt.Errorf("hostname exceeds maximum length of 253 characters")
	}

	// Validate each label.
	labels := strings.Split(normalized, ".")
	if len(labels) < 2 {
		return "", errors.New("hostname must have at least two labels (e.g. example.com)")
	}
	for _, label := range labels {
		if err := validateHostnameLabel(label); err != nil {
			return "", err
		}
	}

	return normalized, nil
}

// validateHostnameLabel validates a single DNS label per RFC 952/1123.
func validateHostnameLabel(label string) error {
	if len(label) == 0 {
		return errors.New("hostname label must not be empty")
	}
	if len(label) > 63 {
		return fmt.Errorf("hostname label %q exceeds maximum length of 63 characters", label)
	}
	if label[0] == '-' || label[len(label)-1] == '-' {
		return fmt.Errorf("hostname label %q must not start or end with a hyphen", label)
	}
	for _, r := range label {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' {
			return fmt.Errorf("hostname label %q contains invalid character %q (only a-z, 0-9, and hyphens are allowed)", label, r)
		}
		// Reject non-ASCII (punycode is out of scope for now).
		if r > unicode.MaxASCII {
			return fmt.Errorf("hostname label %q contains non-ASCII character; use punycode encoding for internationalized domain names", label)
		}
	}
	return nil
}

// generateVerificationToken returns a 64-character hex string derived from 32
// cryptographically random bytes.
func generateVerificationToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is unrecoverable in practice; panic to surface it immediately.
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b)
}
