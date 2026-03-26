package server

import (
	"net/http"

	"github.com/allyourbase/ayb/internal/api"
	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/webhooks"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// registerAPIRoutes mounts user-facing JSON API routes: schema, messaging,
// push, notifications, support, email, realtime, webhooks, GraphQL, and
// auto-generated CRUD.
func (s *Server) registerAPIRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		s.applyAPISharedMiddleware(r)
		r.Use(middleware.AllowContentType("application/json"))
		s.registerAPISchemaAndUserRoutes(r)
		s.registerAPIRealtimeRoutes(r)
		s.registerAPIWebhookRoutes(r)
		s.registerAPIGraphQLRoutes(r)
	})

	r.Group(func(r chi.Router) {
		s.applyAPISharedMiddleware(r)
		s.registerAPICRUDRoutes(r)
	})
}

func (s *Server) applyAPISharedMiddleware(r chi.Router) {
	if s.authSvc == nil && s.apiAnonRL != nil {
		r.Use(APIRouteRateLimitMiddleware(nil, s.apiAnonRL, 0, s.apiAnonRateLimit))
	}
	s.applyAPIAuditContext(r)
}

func (s *Server) applyAPIAuditContext(r chi.Router) {
	// Store the client IP in the request context for audit logging.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := audit.ContextWithIP(r.Context(), httputil.ClientIP(r))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
}

func (s *Server) registerAPISchemaAndUserRoutes(r chi.Router) {
	if s.authSvc != nil {
		r.With(s.requireAdminOrUserAuth(s.authSvc)).Get("/schema", s.handleSchema)
		if s.usageSrc != nil {
			r.With(s.requireAdminOrUserAuth(s.authSvc)).Get("/usage", handleTenantUsage(s.usageSrc))
			if s.usageAggregate != nil {
				r.With(s.requireAdminOrUserAuth(s.authSvc)).Get("/usage/limits", handleTenantUsageLimits(s.usageSrc, s.usageAggregate))
			}
		}

		// Messaging SMS endpoints (user auth required).
		r.Route("/messaging/sms", func(r chi.Router) {
			r.Use(s.requireUserAuthWithRateLimit(s.authSvc))
			r.Post("/send", s.handleMessagingSMSSend)
			r.Get("/messages", s.handleMessagingSMSList)
			r.Get("/messages/{id}", s.handleMessagingSMSGet)
		})

		// User push device token endpoints (user auth required).
		r.Route("/push", func(r chi.Router) {
			r.Use(s.requireUserAuthWithRateLimit(s.authSvc))
			r.Post("/devices", s.handlePushUserRegister)
			r.Get("/devices", s.handlePushUserListDevices)
			r.Delete("/devices/{id}", s.handlePushUserRevokeDevice)
		})

		// User in-app notifications endpoints (user auth required).
		r.Route("/notifications", func(r chi.Router) {
			r.Use(s.requireUserAuthWithRateLimit(s.authSvc))
			r.Get("/", s.handleNotificationsList)
			r.Post("/{id}/read", s.handleNotificationMarkRead)
			r.Post("/read-all", s.handleNotificationMarkAllRead)
		})

		// User support ticket endpoints (user auth required).
		if s.cfg.Support.Enabled {
			r.Route("/support/tickets", func(r chi.Router) {
				r.Use(s.requireUserAuthWithRateLimit(s.authSvc))
				r.With(middleware.AllowContentType("application/json")).Post("/", s.handleCreateSupportTicket)
				r.Get("/", s.handleListSupportTickets)
				r.Get("/{id}", s.handleGetSupportTicket)
				r.With(middleware.AllowContentType("application/json")).Post("/{id}/messages", s.handleAddSupportMessage)
			})
		}

		// Email send endpoint (user auth required, write scope enforced in handler).
		r.With(s.requireUserAuthWithRateLimit(s.authSvc)).Post("/email/send", s.handlePublicEmailSend)
		return
	}

	r.Get("/schema", s.handleSchema)
}

func (s *Server) registerAPIRealtimeRoutes(r chi.Router) {
	// Realtime SSE (handles its own auth for EventSource compatibility).
	rtHandler := realtime.NewHandler(s.hub, s.pool, s.authSvc, s.schema, s.logger)
	rtHandler.CM = s.connManager
	r.Get("/realtime", rtHandler.ServeHTTP)

	// Realtime WebSocket (handles its own auth via token message or header).
	r.Method(http.MethodGet, "/realtime/ws", s.tenantWSAdmissionDynamic(s.wsHandler))
}

func (s *Server) registerAPIWebhookRoutes(r chi.Router) {
	// Webhook management (admin-only).
	if s.pool == nil {
		return
	}
	whStore := webhooks.NewStore(s.pool)
	whHandler := webhooks.NewHandler(whStore, whStore, s.logger)
	r.Route("/webhooks", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Mount("/", whHandler.Routes())
	})
}

func (s *Server) registerAPIGraphQLRoutes(r chi.Router) {
	// Mount GraphQL API (when wired).
	if s.graphqlHandler == nil {
		return
	}
	if s.authSvc != nil {
		r.With(s.requireAdminOrUserAuth(s.authSvc)).Post("/graphql", s.graphqlHandler.ServeHTTP)
	} else {
		r.Post("/graphql", s.graphqlHandler.ServeHTTP)
	}
	// WebSocket upgrade for graphql-ws. Auth is handled in protocol init.
	r.Get("/graphql", s.graphqlHandler.ServeHTTP)
}

func (s *Server) registerAPICRUDRoutes(r chi.Router) {
	// Mount auto-generated CRUD API.
	if s.pool == nil {
		return
	}
	auditLogger := audit.NewAuditLogger(s.cfg.Audit, s.pool)
	apiHandler := api.NewHandler(s.pool, s.schema, s.logger, s.hub, s.webhookDispatcher, auditLogger, s.fieldEncryptor)
	apiHandler.ApplyOptions(api.WithAPILimits(s.cfg.API))
	apiHandler.ApplyOptions(api.WithPoolRouter(s.poolRouter))
	if s.embedFn != nil {
		apiHandler.ApplyOptions(api.WithEmbedder(s.embedFn))
		if s.embedConfigDim > 0 {
			apiHandler.ApplyOptions(api.WithConfiguredEmbeddingDimension(s.embedConfigDim))
		}
	}
	if s.authSvc != nil {
		s.withTenantScopedAdminOrUserAuth(r, func(r chi.Router) {
			r.Mount("/", apiHandler.Routes())
		})
		return
	}
	r.Mount("/", apiHandler.Routes())
}
