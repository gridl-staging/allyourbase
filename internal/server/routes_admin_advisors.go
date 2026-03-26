package server

import "github.com/go-chi/chi/v5"

func (s *Server) registerAdminAdvisorRoutes(r chi.Router) {
	r.Route("/admin/advisors", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/security", s.handleAdminSecurityAdvisor)
		r.Get("/performance", s.handleAdminPerformanceAdvisor)
	})
}
