// Package server Contains HTTP handler helper functions for health checks, OpenAPI specs, admin UI, metrics endpoints, and request parameter parsing utilities.
package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/observability"
	"github.com/allyourbase/ayb/openapi"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	serviceUnavailableJobQueue       = "job queue is not enabled"
	serviceUnavailableMatviews       = "materialized view management requires a database connection"
	serviceUnavailableEmailTemplates = "email template management requires a database connection"
	serviceUnavailablePush           = "push notifications are not enabled"
	serviceUnavailableVaultSecrets   = "vault secrets management is not enabled"
	serviceUnavailableEdgeFunctions  = "edge functions are not enabled"
)

func serviceUnavailable(w http.ResponseWriter, message string) {
	httputil.WriteError(w, http.StatusServiceUnavailable, message)
}

// handleHealth handles HTTP requests to the health check endpoint, returning a JSON response with server status and database connectivity information.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	type healthResponse struct {
		Status   string `json:"status"`
		Database string `json:"database"`
	}

	if s.pool == nil {
		httputil.WriteJSON(w, http.StatusOK, healthResponse{
			Status:   "ok",
			Database: "not configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.pool.Ping(ctx); err != nil {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, healthResponse{
			Status:   "degraded",
			Database: "unreachable",
		})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, healthResponse{
		Status:   "ok",
		Database: "ok",
	})
}

func handleFavicon(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	w.Write(openapi.Spec)
}

func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	sc := s.schema.Get()
	if sc == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "schema cache not ready")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sc)
}

// registerMetricsEndpoint mounts the Prometheus /metrics endpoint with optional
// bearer-token authentication. No-op when metrics are disabled.
func registerMetricsEndpoint(r *chi.Mux, cfg *config.Config, httpMetrics *observability.HTTPMetrics) {
	if !cfg.Metrics.Enabled || httpMetrics == nil {
		return
	}
	metricsPath := cfg.Metrics.Path
	if metricsPath == "" {
		metricsPath = "/metrics"
	}
	r.Get(metricsPath, func(w http.ResponseWriter, req *http.Request) {
		if cfg.Metrics.AuthToken != "" {
			authz := strings.TrimSpace(req.Header.Get("Authorization"))
			parts := strings.Fields(authz)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] != cfg.Metrics.AuthToken {
				w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
				httputil.WriteError(w, http.StatusUnauthorized, "metrics endpoint unauthorized")
				return
			}
		}
		httpMetrics.Handler().ServeHTTP(w, req)
	})
}

// registerAdminSPA mounts the single-page admin dashboard and the OAuth
// authorize page when the admin UI is enabled. No-op otherwise.
func registerAdminSPA(r *chi.Mux, cfg *config.Config) {
	if !cfg.Admin.Enabled {
		return
	}
	adminPath := normalizedAdminPath(cfg.Admin.Path)
	spa := staticSPAHandler(adminPath)
	r.Route(adminPath, func(sub chi.Router) {
		sub.Get("/", spa)
		sub.Get("/*", spa)
	})
	r.Route("/oauth", func(sub chi.Router) {
		sub.Get("/authorize", spa)
	})
}

func normalizedAdminPath(adminPath string) string {
	adminPath = strings.TrimSpace(adminPath)
	if adminPath == "" {
		return "/admin"
	}
	adminPath = strings.TrimRight(adminPath, "/")
	if adminPath == "" {
		return "/"
	}
	return adminPath
}

func adminPathWithTrailingSlash(adminPath string) string {
	adminPath = normalizedAdminPath(adminPath)
	if adminPath == "/" {
		return "/"
	}
	return adminPath + "/"
}

// parseUUIDParam extracts and validates a UUID from a chi URL parameter.
// Returns false and writes a 400 JSON error if the param is missing or malformed.
func parseUUIDParam(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	return parseUUIDParamWithLabel(w, r, param, param)
}

// parseUUIDParamWithLabel extracts and validates a UUID from a chi URL parameter
// and uses label in any validation error returned to the client.
func parseUUIDParamWithLabel(w http.ResponseWriter, r *http.Request, param, label string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid "+label+" format")
		return uuid.UUID{}, false
	}
	return id, true
}
