package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
)

// domainManager is the interface for admin custom domain operations.
// DomainStore satisfies this interface.
type domainManager interface {
	CreateDomain(ctx context.Context, hostname, environment, redirectMode string) (*DomainBinding, error)
	GetDomain(ctx context.Context, id string) (*DomainBinding, error)
	ListDomains(ctx context.Context, page, perPage int) (*DomainBindingListResult, error)
	DeleteDomain(ctx context.Context, id string) error
	TriggerVerification(ctx context.Context, id string) (*DomainBinding, error)
	UpdateDomainStatus(ctx context.Context, id string, status DomainStatus, lastError *string) (*DomainBinding, error)
	SetDomainCert(ctx context.Context, id string, certRef string, certExpiry time.Time) (*DomainBinding, error)
	ListDomainsForCertRenewal(ctx context.Context, renewBefore time.Time) ([]DomainBinding, error)
}

type createDomainRequest struct {
	Hostname     string `json:"hostname"`
	Environment  string `json:"environment"`
	RedirectMode string `json:"redirectMode"`
}

// extractDomainID extracts and validates the "id" URL parameter as a UUID.
// Returns the id and true on success, or writes an error response and returns false.
func extractDomainID(w http.ResponseWriter, r *http.Request) (string, bool) {
	domainID, ok := parseUUIDParamWithLabel(w, r, "id", "domain id")
	if !ok {
		return "", false
	}
	return domainID.String(), true
}

// handleAdminListDomains returns a paginated list of all domain bindings.
func handleAdminListDomains(svc domainManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))

		result, err := svc.ListDomains(r.Context(), page, perPage)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list domains")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, result)
	}
}

// handleAdminCreateDomain creates a new custom domain binding.
func handleAdminCreateDomain(svc domainManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createDomainRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		normalized, err := normalizeAndValidateHostname(req.Hostname)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		if req.RedirectMode != "" && req.RedirectMode != "permanent" && req.RedirectMode != "temporary" {
			httputil.WriteError(w, http.StatusBadRequest, "redirectMode must be 'permanent' or 'temporary'")
			return
		}

		binding, err := svc.CreateDomain(r.Context(), normalized, req.Environment, req.RedirectMode)
		if err != nil {
			if errors.Is(err, ErrDomainHostnameConflict) {
				httputil.WriteError(w, http.StatusConflict, "hostname already bound")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to create domain")
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, binding)
	}
}

// handleAdminGetDomain returns a single domain binding by ID.
func handleAdminGetDomain(svc domainManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := extractDomainID(w, r)
		if !ok {
			return
		}

		binding, err := svc.GetDomain(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrDomainNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "domain not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get domain")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, binding)
	}
}

// handleAdminDeleteDomain soft-deletes a domain binding (tombstone).
func handleAdminDeleteDomain(svc domainManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := extractDomainID(w, r)
		if !ok {
			return
		}

		err := svc.DeleteDomain(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrDomainNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "domain not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to delete domain")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// handleAdminTriggerDomainVerify triggers DNS verification for a domain binding.
// When verifyRL is non-nil, applies per-domain rate limiting keyed by domain ID.
func handleAdminTriggerDomainVerify(svc domainManager, verifyRL *auth.RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := extractDomainID(w, r)
		if !ok {
			return
		}

		if verifyRL != nil {
			allowed, remaining, resetTime := verifyRL.Allow(id)
			w.Header().Set("X-RateLimit-Limit", "10")
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))
			if !allowed {
				retryAfter := int(time.Until(resetTime).Seconds()) + 1
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				httputil.WriteError(w, http.StatusTooManyRequests, "too many verification attempts for this domain")
				return
			}
		}

		binding, err := svc.TriggerVerification(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrDomainNotFound) {
				httputil.WriteError(w, http.StatusNotFound, "domain not found")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "failed to trigger verification")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, binding)
	}
}
