// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar19_03_go_code_quality_refactoring/allyourbase_dev/internal/server/routes_admin.go.
package server

import (
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// registerAdminRoutes mounts all admin-gated route groups onto the given router.
func (s *Server) registerAdminRoutes(r chi.Router) {
	s.registerAdminCoreRoutes(r)
	s.registerAdminAdvisorRoutes(r)
	s.registerAdminDataRoutes(r)
	s.registerAdminServicesRoutes(r)
	s.registerAdminPlatformRoutes(r)
	s.registerAdminOrgRoutes(r)
	s.registerAdminSitesRoutes(r)
}

// registerAdminCoreRoutes registers admin status, login, replicas, realtime,
// user management, API keys, apps, OAuth clients, auth settings/providers/hooks,
// SAML, custom domains, and logs.
func (s *Server) registerAdminCoreRoutes(r chi.Router) {
	r.Get("/admin/status", s.handleAdminStatus)
	r.With(s.adminRL.Middleware).Post("/admin/auth", s.handleAdminLogin)

	s.registerAdminReplicaRoutes(r)
	r.Route("/admin/realtime", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/stats", s.handleAdminRealtimeStats)
		r.Get("/connections", s.handleAdminRealtimeConnections)
		r.Get("/subscriptions", s.handleAdminRealtimeSubscriptions)
		r.Post("/connections/{id}/disconnect", s.handleAdminRealtimeForceDisconnect)
	})

	// Admin user management (admin-auth gated, requires auth to be enabled).
	if s.authSvc != nil {
		r.Route("/admin/users", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", handleAdminListUsers(s.authSvc))
			r.Delete("/{id}", handleAdminDeleteUser(s.authSvc))
			r.Get("/{id}/sessions", handleAdminListUserSessions(s.authSvc))
			r.Delete("/{id}/sessions/{session_id}", handleAdminDeleteUserSession(s.authSvc))
			r.Delete("/{id}/sessions", handleAdminDeleteAllUserSessions(s.authSvc))
			r.Get("/{id}/provider-tokens", handleAdminListUserProviderTokens(s.authSvc))
			r.Delete("/{id}/provider-tokens/{provider}", handleAdminDeleteUserProviderToken(s.authSvc))
			if s.storageSvc != nil {
				r.Get("/{id}/storage-quota", handleAdminGetUserQuota(s.storageSvc))
				r.Put("/{id}/storage-quota", handleAdminSetUserQuota(s.storageSvc))
			}
		})

		// Admin API key management.
		r.Route("/admin/api-keys", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", handleAdminListAPIKeys(s.authSvc))
			r.Post("/", handleAdminCreateAPIKey(s.authSvc))
			r.Delete("/{id}", handleAdminRevokeAPIKey(s.authSvc))
		})

		// Admin app management.
		r.Route("/admin/apps", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", handleAdminListApps(s.authSvc))
			r.Post("/", handleAdminCreateApp(s.authSvc))
			r.Get("/{id}", handleAdminGetApp(s.authSvc))
			r.Put("/{id}", handleAdminUpdateApp(s.authSvc))
			r.Delete("/{id}", handleAdminDeleteApp(s.authSvc))
		})

		// Admin OAuth client management.
		r.Route("/admin/oauth/clients", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", handleAdminListOAuthClients(s.authSvc))
			r.Post("/", handleAdminCreateOAuthClient(s.authSvc))
			r.Get("/{clientId}", handleAdminGetOAuthClient(s.authSvc))
			r.Put("/{clientId}", handleAdminUpdateOAuthClient(s.authSvc))
			r.Delete("/{clientId}", handleAdminRevokeOAuthClient(s.authSvc))
			r.Post("/{clientId}/rotate-secret", handleAdminRotateOAuthClientSecret(s.authSvc))
		})
	}

	// Admin custom domain management (admin-auth gated, requires pool).
	if s.pool != nil {
		verifyRL := auth.NewRateLimiter(10, time.Hour)
		r.Route("/admin/domains", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", handleAdminListDomains(s.domainStore))
			r.Post("/", handleAdminCreateDomain(s.domainStore))
			r.Get("/{id}", handleAdminGetDomain(s.domainStore))
			r.Delete("/{id}", handleAdminDeleteDomain(s.domainStore))
			r.Post("/{id}/verify", handleAdminTriggerDomainVerify(s.domainStore, verifyRL))
		})
	}

	// Admin logs (admin-auth gated).
	r.Route("/admin/logs", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleAdminLogs)
	})

	s.registerAdminAuthConfigRoutes(r)
}

// TODO: Document Server.registerAdminAuthConfigRoutes.
func (s *Server) registerAdminAuthConfigRoutes(r chi.Router) {
	// Admin auth settings (admin-auth gated).
	r.Route("/admin/auth-settings", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleAdminAuthSettingsGet)
		r.With(middleware.AllowContentType("application/json")).Put("/", s.handleAdminAuthSettingsUpdate)
	})

	// Admin auth provider management (admin-auth gated).
	r.Route("/admin/auth/providers", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleAdminAuthProvidersList)
		r.With(middleware.AllowContentType("application/json")).Put("/{provider}", s.handleAdminAuthProvidersUpdate)
		r.Delete("/{provider}", s.handleAdminAuthProvidersDelete)
		r.Post("/{provider}/test", s.handleAdminAuthProvidersTest)
	})

	// Admin auth hooks config and SAML (admin-auth gated, requires auth service).
	if s.authSvc != nil {
		r.Route("/admin/auth/hooks", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", handleAdminAuthHooks(s.cfg))
		})
		r.Route("/admin/auth/saml", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", s.handleAdminSAMLList)
			r.With(middleware.AllowContentType("application/json")).Post("/", s.handleAdminSAMLCreate)
			r.With(middleware.AllowContentType("application/json")).Put("/{name}", s.handleAdminSAMLUpdate)
			r.Delete("/{name}", s.handleAdminSAMLDelete)
		})
	}
}

// registerAdminDataRoutes registers SQL, RLS, matviews, extensions, FDW,
// vector indexes, branches, and backups/PITR routes.
func (s *Server) registerAdminDataRoutes(r chi.Router) {
	// Admin SQL editor and RLS policy management (admin-auth gated, requires pool).
	if s.pool != nil {
		s.logger.Info("registering admin SQL and RLS routes")
		r.Route("/admin/sql", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Post("/", handleAdminSQL(s.pool, s.schema))
		})

		// Admin RLS policy management.
		r.Route("/admin/rls", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", handleListRlsPolicies(s.pool))
			r.Post("/", handleCreateRlsPolicy(s.pool))
			r.Post("/templates/storage-objects/{template}", handleApplyStorageObjectsTemplate(s.pool))
			r.Get("/{table}", handleListRlsPolicies(s.pool))
			r.Get("/{table}/status", handleGetRlsStatus(s.pool))
			r.Post("/{table}/enable", handleEnableRls(s.pool))
			r.Post("/{table}/disable", handleDisableRls(s.pool))
			r.Delete("/{table}/{policy}", handleDeleteRlsPolicy(s.pool))
		})
	} else {
		s.logger.Warn("pool is nil, skipping admin SQL and RLS routes")
	}

	// Admin materialized view management (admin-auth gated).
	r.Route("/admin/matviews", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleMatviewsList)
		r.Post("/", s.handleMatviewsRegister)
		r.Get("/{id}", s.handleMatviewsGet)
		r.Put("/{id}", s.handleMatviewsUpdate)
		r.Delete("/{id}", s.handleMatviewsDelete)
		r.Post("/{id}/refresh", s.handleMatviewsRefresh)
	})

	// Admin vector index management (admin-auth gated).
	r.Route("/admin/vector/indexes", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.With(middleware.AllowContentType("application/json")).Post("/", s.handleAdminVectorIndexCreate)
		r.Get("/", s.handleAdminVectorIndexList)
	})

	// Extension management (admin-auth gated).
	r.Route("/admin/extensions", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleAdminExtensionList)
		r.With(middleware.AllowContentType("application/json")).Post("/", s.handleAdminExtensionEnable)
		r.Delete("/{name}", s.handleAdminExtensionDisable)
	})
	// FDW management (admin-auth gated).
	r.Route("/admin/fdw", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Route("/servers", func(r chi.Router) {
			r.Get("/", s.handleAdminFDWListServers)
			r.With(middleware.AllowContentType("application/json")).Post("/", s.handleAdminFDWCreateServer)
			r.With(middleware.AllowContentType("application/json")).Post("/{name}/import", s.handleAdminFDWImportTables)
			r.Delete("/{name}", s.handleAdminFDWDropServer)
		})
		r.Route("/tables", func(r chi.Router) {
			r.Get("/", s.handleAdminFDWListTables)
			r.Delete("/{schema}/{table}", s.handleAdminFDWDropTable)
		})
	})

	// Branch management (admin-auth gated).
	r.Route("/admin/branches", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleAdminBranchList)
		r.Post("/", s.handleAdminBranchCreate)
		r.Delete("/{name}", s.handleAdminBranchDelete)
	})
	// Backup management (admin-auth gated).
	r.Route("/admin/backups", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleAdminBackupList)
		r.Post("/", s.handleAdminBackupTrigger)

		// PITR restore management (inherits requireAdminToken from parent group).
		r.Route("/projects/{projectId}/pitr", func(r chi.Router) {
			r.Post("/validate", s.handlePITRValidate)
			r.Post("/restore", s.handlePITRRestore)
			r.Get("/jobs", s.handlePITRJobList)
		})
	})

	// Restore job management (admin-auth gated, cross-project resource).
	r.Route("/admin/backups/restore-jobs", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/{jobId}", s.handlePITRJobGet)
		r.Delete("/{jobId}", s.handlePITRJobAbandon)
	})

	s.registerAdminStorageCDNRoutes(r)
}

// registerAdminServicesRoutes registers incidents, support, log drains,
// audit, analytics, stats, secrets, SMS, jobs, schedules, email templates,
// push, and notifications routes.
func (s *Server) registerAdminServicesRoutes(r chi.Router) {
	s.registerAdminIncidentRoutes(r)
	s.registerAdminSupportTicketRoutes(r)
	s.registerAdminLoggingDrainRoutes(r)
	s.registerAdminAuditRoutes(r)
	s.registerAdminAnalyticsRoutes(r)
	// PARITY: 6.4 — Dashboard lacks a dedicated rich request/query logs viewer workflow; backend analytics APIs are present.

	s.registerAdminStatsRoutes(r)

	// Admin secrets management (admin-auth gated).
	r.Route("/admin/secrets", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleListSecrets)
		r.Get("/{name}", s.handleGetSecret)
		r.With(middleware.AllowContentType("application/json")).Post("/", s.handleCreateSecret)
		r.With(middleware.AllowContentType("application/json")).Put("/{name}", s.handleUpdateSecret)
		r.Delete("/{name}", s.handleDeleteSecret)
		if s.authSvc != nil {
			r.Post("/rotate", s.handleAdminSecretsRotate)
		}
	})

	// Admin SMS (admin-auth gated).
	r.Route("/admin/sms", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/health", s.handleAdminSMSHealth)
		r.Get("/messages", s.handleAdminSMSMessages)
		r.With(middleware.AllowContentType("application/json")).Post("/send", s.handleAdminSMSSend)
	})

	// Admin job queue management (admin-auth gated, requires jobs service).
	// Routes are registered unconditionally; the SetJobService method
	// wires the actual service at startup when jobs.enabled = true.
	r.Route("/admin/jobs", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleJobsList)
		r.Get("/stats", s.handleJobsStats)
		r.Get("/{id}", s.handleJobsGet)
		r.Post("/{id}/retry", s.handleJobsRetry)
		r.Post("/{id}/cancel", s.handleJobsCancel)
	})

	r.Route("/admin/schedules", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleSchedulesList)
		r.Post("/", s.handleSchedulesCreate)
		r.Put("/{id}", s.handleSchedulesUpdate)
		r.Delete("/{id}", s.handleSchedulesDelete)
		r.Post("/{id}/enable", s.handleSchedulesEnable)
		r.Post("/{id}/disable", s.handleSchedulesDisable)
	})

	// Admin email template management (admin-auth gated).
	// Routes registered unconditionally; SetEmailTemplateService wires the service at startup.
	r.Route("/admin/email/templates", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleEmailTemplatesList)
		r.Get("/{key}", s.handleEmailTemplatesGet)
		r.Put("/{key}", s.handleEmailTemplatesUpsert)
		r.Delete("/{key}", s.handleEmailTemplatesDelete)
		r.Patch("/{key}", s.handleEmailTemplatesPatch)
		r.Post("/{key}/preview", s.handleEmailTemplatesPreview)
	})
	r.With(s.requireAdminToken).Post("/admin/email/send", s.handleEmailSend)

	// Admin push notification management (admin-auth gated).
	// Routes are registered unconditionally; SetPushService wires the service at startup.
	r.Route("/admin/push", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/devices", s.handlePushAdminListDevices)
		r.Delete("/devices/{id}", s.handlePushAdminRevokeDevice)
		r.With(middleware.AllowContentType("application/json")).Post("/devices", s.handlePushAdminRegisterDevice)
		r.With(middleware.AllowContentType("application/json")).Post("/send", s.handlePushAdminSend)
		r.With(middleware.AllowContentType("application/json")).Post("/send-to-token", s.handlePushAdminSendToToken)
		r.Get("/deliveries", s.handlePushAdminListDeliveries)
		r.Get("/deliveries/{id}", s.handlePushAdminGetDelivery)
	})

	// Admin in-app notification creation (admin-auth gated).
	r.With(s.requireAdminToken).Post("/admin/notifications", s.handleNotificationsCreate)
}

func (s *Server) registerAdminIncidentRoutes(r chi.Router) {
	// Status incidents (admin-auth gated).
	if s.cfg.Status.Enabled && s.pool != nil && s.statusIncidentStore != nil {
		r.Route("/admin/incidents", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.With(middleware.AllowContentType("application/json")).Post("/", handleCreateIncident(s.statusIncidentStore))
			r.Get("/", handleListIncidents(s.statusIncidentStore))
			r.With(middleware.AllowContentType("application/json")).Put("/{id}", handleUpdateIncident(s.statusIncidentStore))
			r.With(middleware.AllowContentType("application/json")).Post("/{id}/updates", handleAddIncidentUpdate(s.statusIncidentStore))
		})
	}
}

func (s *Server) registerAdminSupportTicketRoutes(r chi.Router) {
	// Support ticket admin routes (admin-auth gated).
	if s.cfg.Support.Enabled {
		r.Route("/admin/support/tickets", func(r chi.Router) {
			r.Use(s.requireAdminToken)
			r.Get("/", s.handleAdminListSupportTickets)
			r.Get("/{id}", s.handleAdminGetSupportTicket)
			r.With(middleware.AllowContentType("application/json")).Put("/{id}", s.handleAdminUpdateSupportTicket)
			r.With(middleware.AllowContentType("application/json")).Post("/{id}/messages", s.handleAdminAddSupportMessage)
		})
	}
}

func (s *Server) registerAdminLoggingDrainRoutes(r chi.Router) {
	// Admin log drain management (admin-auth gated).
	r.Route("/admin/logging/drains", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleListDrains)
		r.Post("/", s.handleCreateDrain)
		r.Delete("/{id}", s.handleDeleteDrain)
	})
}

func (s *Server) registerAdminAuditRoutes(r chi.Router) {
	// Admin audit log query (admin-auth gated).
	r.Route("/admin/audit", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleAdminAudit)
	})
}

func (s *Server) registerAdminAnalyticsRoutes(r chi.Router) {
	r.Route("/admin/analytics", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/requests", s.handleAdminRequestLogs)
		r.Get("/queries", s.handleAdminQueryAnalytics)
	})
}

func (s *Server) registerAdminStatsRoutes(r chi.Router) {
	// Admin stats (admin-auth gated).
	r.Route("/admin/stats", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleAdminStats)
	})
}

// registerAdminPlatformRoutes registers edge functions/triggers, AI/prompts,
// usage, and tenant management routes.
func (s *Server) registerAdminPlatformRoutes(r chi.Router) {
	// Admin edge function management (admin-auth gated).
	// Routes registered unconditionally; SetEdgeFuncService wires the service at startup.
	r.Route("/admin/functions", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.handleEdgeFuncAdminList)
		r.Post("/", s.handleEdgeFuncAdminDeploy)
		r.Get("/by-name/{name}", s.handleEdgeFuncAdminGetByName)
		r.Get("/{id}", s.handleEdgeFuncAdminGet)
		r.Put("/{id}", s.handleEdgeFuncAdminUpdate)
		r.Delete("/{id}", s.handleEdgeFuncAdminDelete)
		r.Get("/{id}/logs", s.handleEdgeFuncAdminLogs)
		r.Post("/{id}/invoke", s.handleEdgeFuncAdminInvoke)

		// Trigger management sub-routes under each function.
		r.Route("/{id}/triggers", func(r chi.Router) {
			r.Route("/db", func(r chi.Router) {
				r.Get("/", s.handleDBTriggerList)
				r.Post("/", s.handleDBTriggerCreate)
				r.Get("/{triggerId}", s.handleDBTriggerGet)
				r.Delete("/{triggerId}", s.handleDBTriggerDelete)
				r.Post("/{triggerId}/enable", s.handleDBTriggerEnable)
				r.Post("/{triggerId}/disable", s.handleDBTriggerDisable)
			})
			r.Route("/cron", func(r chi.Router) {
				r.Get("/", s.handleCronTriggerList)
				r.Post("/", s.handleCronTriggerCreate)
				r.Get("/{triggerId}", s.handleCronTriggerGet)
				r.Delete("/{triggerId}", s.handleCronTriggerDelete)
				r.Post("/{triggerId}/enable", s.handleCronTriggerEnable)
				r.Post("/{triggerId}/disable", s.handleCronTriggerDisable)
				r.Post("/{triggerId}/run", s.handleCronTriggerManualRun)
			})
			r.Route("/storage", func(r chi.Router) {
				r.Get("/", s.handleStorageTriggerList)
				r.Post("/", s.handleStorageTriggerCreate)
				r.Get("/{triggerId}", s.handleStorageTriggerGet)
				r.Delete("/{triggerId}", s.handleStorageTriggerDelete)
				r.Post("/{triggerId}/enable", s.handleStorageTriggerEnable)
				r.Post("/{triggerId}/disable", s.handleStorageTriggerDisable)
			})
		})
	})

	s.registerAdminAIRoutes(r)

	s.registerAdminUsageRoutes(r)

	// Tenant management (admin-auth gated).
	r.Route("/admin/tenants", func(r chi.Router) {
		r.Use(s.requireAdminToken)
		r.Get("/", s.tenantAdminHandler(handleAdminListTenants))
		r.Post("/", s.tenantAdminAuditHandler(handleAdminCreateTenant))
		r.Get("/{tenantId}", s.tenantAdminHandler(handleAdminGetTenant))
		r.Get("/{tenantId}/audit", s.handleAdminTenantAudit)
		r.Put("/{tenantId}", s.tenantAdminAuditHandler(handleAdminUpdateTenant))
		r.Post("/{tenantId}/suspend", s.tenantAdminAuditHandler(handleAdminSuspendTenant))
		r.Post("/{tenantId}/resume", s.tenantAdminAuditHandler(handleAdminResumeTenant))
		r.Delete("/{tenantId}", s.tenantAdminAuditHandler(handleAdminDeleteTenant))
		r.Get("/{tenantId}/members", s.tenantAdminHandler(handleAdminListTenantMembers))
		r.Post("/{tenantId}/members", s.tenantAdminAuditHandler(handleAdminAddTenantMember))
		r.Put("/{tenantId}/members/{userId}", s.tenantAdminAuditHandler(handleAdminUpdateTenantMemberRole))
		r.Delete("/{tenantId}/members/{userId}", s.tenantAdminAuditHandler(handleAdminRemoveTenantMember))

		// Tenant maintenance mode endpoints.
		r.Post("/{tenantId}/maintenance/enable", s.tenantAdminAuditHandler(handleAdminEnableMaintenance))
		r.Post("/{tenantId}/maintenance/disable", s.tenantAdminAuditHandler(handleAdminDisableMaintenance))
		r.Get("/{tenantId}/maintenance", s.tenantAdminHandler(handleAdminGetMaintenance))

		// Tenant circuit breaker endpoints (read tracker dynamically).
		r.Get("/{tenantId}/breaker", s.tenantBreakerHandler(handleAdminGetBreaker))
		r.Post("/{tenantId}/breaker/reset", s.tenantBreakerAuditHandler(handleAdminResetBreaker))
	})
}
