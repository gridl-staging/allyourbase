// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/storage_cdn_admin_handler.go.
package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const (
	storageCDNPurgeAllRateLimit       = 1
	storageCDNPurgeAllRateLimitWindow = time.Minute
)

type adminStorageCDNPurgeRequest struct {
	URLs     []string `json:"urls"`
	PurgeAll bool     `json:"purge_all"`
}

type adminStorageCDNPurgeResponse struct {
	Operation string `json:"operation"`
	Submitted int    `json:"submitted"`
	Provider  string `json:"provider"`
}

func (s *Server) registerAdminStorageCDNRoutes(r chi.Router) {
	if s.storageHandler == nil {
		return
	}

	r.Route("/admin/storage/cdn", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.With(middleware.AllowContentType("application/json")).Post("/purge", s.handleAdminStorageCDNPurge)
	})
}

// TODO: Document Server.handleAdminStorageCDNPurge.
func (s *Server) handleAdminStorageCDNPurge(w http.ResponseWriter, r *http.Request) {
	if s.storageHandler == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "storage service is not enabled")
		return
	}

	var req adminStorageCDNPurgeRequest
	if !decodeStrictAdminStorageCDNPurgeRequest(w, r, &req) {
		return
	}

	validatedURLs, operation, err := validateAdminStorageCDNPurgeRequest(req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if operation == "purge_all" {
		if !s.allowAdminStorageCDNPurgeAll(w, r) {
			return
		}
		s.storageHandler.EnqueuePurgeAll(r.Context())
		httputil.WriteJSON(w, http.StatusAccepted, adminStorageCDNPurgeResponse{
			Operation: operation,
			Submitted: 1,
			Provider:  s.storageHandler.CDNProviderName(),
		})
		return
	}

	s.storageHandler.EnqueuePurgeURLs(r.Context(), validatedURLs)
	httputil.WriteJSON(w, http.StatusAccepted, adminStorageCDNPurgeResponse{
		Operation: operation,
		Submitted: len(validatedURLs),
		Provider:  s.storageHandler.CDNProviderName(),
	})
}

func decodeStrictAdminStorageCDNPurgeRequest(w http.ResponseWriter, r *http.Request, out *adminStorageCDNPurgeRequest) bool {
	r.Body = http.MaxBytesReader(w, r.Body, httputil.MaxBodySize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// TODO: Document validateAdminStorageCDNPurgeRequest.
func validateAdminStorageCDNPurgeRequest(req adminStorageCDNPurgeRequest) ([]string, string, error) {
	cleanURLs := storage.NormalizePublicURLs(req.URLs)
	if req.PurgeAll && len(cleanURLs) > 0 {
		return nil, "", errors.New("choose exactly one mode: either urls or purge_all")
	}
	if req.PurgeAll {
		return nil, "purge_all", nil
	}
	if len(cleanURLs) == 0 {
		return nil, "", errors.New("choose exactly one mode: either urls or purge_all")
	}
	for _, rawURL := range cleanURLs {
		parsed, err := url.Parse(rawURL)
		if err != nil || strings.TrimSpace(parsed.Host) == "" {
			return nil, "", errors.New("urls must contain absolute public URLs")
		}
		scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
		if scheme != "http" && scheme != "https" {
			return nil, "", errors.New("urls must contain absolute public URLs")
		}
	}
	return cleanURLs, "purge_urls", nil
}

// TODO: Document Server.allowAdminStorageCDNPurgeAll.
func (s *Server) allowAdminStorageCDNPurgeAll(w http.ResponseWriter, r *http.Request) bool {
	if s.storageCDNPurgeAllRL == nil {
		return true
	}
	allowed, remaining, resetTime := s.storageCDNPurgeAllRL.Allow(httputil.ClientIP(r))
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(storageCDNPurgeAllRateLimit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))
	if allowed {
		return true
	}
	retryAfter := int(time.Until(resetTime).Seconds()) + 1
	if retryAfter < 1 {
		retryAfter = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	httputil.WriteError(w, http.StatusTooManyRequests, "cdn purge_all rate limit exceeded")
	return false
}
