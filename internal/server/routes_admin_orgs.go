// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/routes_admin_orgs.go.
package server

import "github.com/go-chi/chi/v5"

// TODO: Document Server.registerAdminOrgRoutes.
func (s *Server) registerAdminOrgRoutes(r chi.Router) {
	r.Route("/admin/orgs", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Post("/", s.orgStoreHandler(handleAdminCreateOrg))
		r.Get("/", s.orgStoreHandler(handleAdminListOrgs))

		r.Route("/{orgId}", func(r chi.Router) {
			r.Get("/", s.orgTeamAndTenantAdminHandler(handleAdminGetOrg))
			r.Put("/", s.orgStoreHandler(handleAdminUpdateOrg))
			r.Delete("/", s.orgStoreTenantAdminHandler(handleAdminDeleteOrg))

			r.Get("/usage", s.orgStoreUsageHandler(handleAdminOrgUsage))
			r.Get("/audit", s.orgStoreAuditHandler(handleAdminOrgAudit))

			r.Route("/teams", func(r chi.Router) {
				r.Post("/", s.orgTeamStoreHandler(handleAdminCreateTeam))
				r.Get("/", s.orgTeamStoreHandler(handleAdminListTeams))

				r.Route("/{teamId}", func(r chi.Router) {
					r.Get("/", s.orgTeamStoreHandler(handleAdminGetTeam))
					r.Put("/", s.orgTeamStoreHandler(handleAdminUpdateTeam))
					r.Delete("/", s.orgTeamStoreHandler(handleAdminDeleteTeam))

					r.Route("/members", func(r chi.Router) {
						r.Post("/", s.teamMembershipStoresHandler(handleAdminAddTeamMember))
						r.Get("/", s.teamMembershipStoreAndTeamStoreHandler(handleAdminListTeamMembers))
						r.Route("/{userId}", func(r chi.Router) {
							r.Delete("/", s.teamMembershipStoreAndTeamStoreHandler(handleAdminRemoveTeamMember))
							r.Put("/role", s.teamMembershipStoreAndTeamStoreHandler(handleAdminUpdateTeamMemberRole))
						})
					})
				})
			})

			r.Route("/members", func(r chi.Router) {
				r.Post("/", s.orgStoreMembershipHandler(handleAdminAddOrgMember))
				r.Get("/", s.orgStoreMembershipHandler(handleAdminListOrgMembers))
				r.Route("/{userId}", func(r chi.Router) {
					r.Delete("/", s.orgStoreMembershipHandler(handleAdminRemoveOrgMember))
					r.Put("/role", s.orgStoreMembershipHandler(handleAdminUpdateOrgMemberRole))
				})
			})

			r.Route("/tenants", func(r chi.Router) {
				r.Post("/", s.orgStoreTenantAdminHandler(handleAdminAssignTenantToOrg))
				r.Get("/", s.orgStoreTenantAdminHandler(handleAdminListOrgTenants))
				r.Delete("/{tenantId}", s.orgStoreTenantAdminHandler(handleAdminUnassignTenantFromOrg))
			})
		})
	})
}
