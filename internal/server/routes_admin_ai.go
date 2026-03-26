// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/routes_admin_ai.go.
package server

import (
	"net/http"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// TODO: Document Server.registerAdminAIRoutes.
func (s *Server) registerAdminAIRoutes(r chi.Router) {
	r.Route("/admin/ai", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/logs", s.handleAdminAILogs)
		r.Get("/usage", s.handleAdminAIUsage)
		r.Get("/usage/daily", s.handleAdminAIUsageDaily)
		assistantLimiter := s.assistantRateLimitMiddleware()
		r.With(middleware.AllowContentType("application/json"), assistantLimiter).Post("/assistant", s.handleAdminAIAssistant)
		r.With(middleware.AllowContentType("application/json"), assistantLimiter).Post("/assistant/stream", s.handleAdminAIAssistantStream)
		r.Get("/assistant/history", s.handleAdminAIAssistantHistory)
		r.Route("/prompts", func(r chi.Router) {
			r.Get("/", s.handleAdminPromptList)
			r.With(middleware.AllowContentType("application/json")).Post("/", s.handleAdminPromptCreate)
			r.Get("/{id}", s.handleAdminPromptGet)
			r.With(middleware.AllowContentType("application/json")).Put("/{id}", s.handleAdminPromptUpdate)
			r.Delete("/{id}", s.handleAdminPromptDelete)
			r.Get("/{id}/versions", s.handleAdminPromptVersions)
			r.With(middleware.AllowContentType("application/json")).Post("/{id}/render", s.handleAdminPromptRender)
		})
	})
}

// TODO: Document Server.assistantRateLimitMiddleware.
func (s *Server) assistantRateLimitMiddleware() func(http.Handler) http.Handler {
	if s.assistantRL == nil {
		return func(next http.Handler) http.Handler { return next }
	}

	limit := s.assistantRateLimit
	if limit <= 0 {
		limit = 20
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := assistantRateLimitKey(r)
			allowed, remaining, resetTime := s.assistantRL.Allow(key)
			if !handleRateLimitDecision(w, limit, allowed, remaining, resetTime) {
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func assistantRateLimitKey(r *http.Request) string {
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil && claims.Subject != "" {
		return "user:" + claims.Subject
	}
	// Admin dashboard auth does not carry a user ID, so IP is the best
	// available stable bucket for admin-token traffic.
	return "admin:" + httputil.ClientIP(r)
}
