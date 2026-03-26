package server

import "github.com/go-chi/chi/v5"

// registerAdminSitesRoutes registers admin site and deploy management routes.
func (s *Server) registerAdminSitesRoutes(r chi.Router) {
	if s.siteStore == nil {
		return
	}

	r.Route("/admin/sites", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", handleAdminListSites(s.siteStore))
		r.Post("/", handleAdminCreateSite(s.siteStore))
		r.Get("/{siteId}", handleAdminGetSite(s.siteStore))
		r.Put("/{siteId}", handleAdminUpdateSite(s.siteStore))
		r.Delete("/{siteId}", handleAdminDeleteSite(s.siteStore))

		// Deploy sub-routes scoped to a site.
		r.Route("/{siteId}/deploys", func(r chi.Router) {
			r.Get("/", handleAdminListDeploys(s.siteStore))
			r.Post("/", handleAdminCreateDeploy(s.siteStore))
			r.Get("/{deployId}", handleAdminGetDeploy(s.siteStore))
			if s.storageSvc != nil {
				r.Post("/{deployId}/files", handleAdminUploadDeployFile(s.siteStore, s.storageSvc))
			}
			r.Post("/{deployId}/promote", handleAdminPromoteDeploy(s.siteStore))
			r.Post("/{deployId}/fail", handleAdminFailDeploy(s.siteStore))
			r.Post("/rollback", handleAdminRollbackDeploy(s.siteStore))
		})
	})
}
