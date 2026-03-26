package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// registerAdminReplicaRoutes mounts all replica management routes under
// /admin/replicas, including read-only status endpoints and lifecycle
// operations (add, remove, promote, failover).
func (s *Server) registerAdminReplicaRoutes(r chi.Router) {
	r.Route("/admin/replicas", func(r chi.Router) {
		r.Use(s.requireAdminToken)

		// Read-only status endpoints.
		r.Get("/", s.handleListReplicas)
		r.Post("/check", s.handleCheckReplicas)

		// Lifecycle mutation endpoints.
		r.With(middleware.AllowContentType("application/json")).Post("/", s.handleAddReplica)
		r.Post("/failover", s.handleFailover)

		// Per-replica endpoints (must come after fixed subpaths).
		r.Route("/{name}", func(r chi.Router) {
			r.Delete("/", s.handleRemoveReplica)
			r.Post("/promote", s.handlePromoteReplica)
		})
	})
}
