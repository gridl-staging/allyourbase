package server

import (
	"net/http"
	"reflect"

	"github.com/allyourbase/ayb/internal/api"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/logging"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/allyourbase/ayb/internal/support"
	"github.com/allyourbase/ayb/internal/tenant"
)

// SetLogBuffer attaches a log buffer for the /api/admin/logs endpoint.
func (s *Server) SetLogBuffer(lb *LogBuffer) {
	s.logBuffer = lb
}

// SetSMSProvider configures the SMS provider for the messaging API.
func (s *Server) SetSMSProvider(name string, p sms.Provider, allowedCountries []string) {
	s.smsProvider = p
	s.smsProviderName = name
	s.smsAllowedCountries = allowedCountries
	if s.pool != nil {
		s.msgStore = &pgMessageStore{pool: s.pool}
	}
}

// SetJobService wires the job queue service for admin API endpoints.
func (s *Server) SetJobService(svc *jobs.Service) {
	s.jobService = svc
	if ds, ok := s.domainStore.(*DomainStore); ok {
		ds.SetJobService(svc)
	}
}

// DomainManager returns the domain binding manager, if configured.
func (s *Server) DomainManager() domainManager {
	return s.domainStore
}

// SetSiteService wires the site management service for test injection.
func (s *Server) SetSiteService(svc siteManager) {
	s.siteStore = svc
}

// SetCertManager wires the certificate manager for custom domain TLS provisioning.
func (s *Server) SetCertManager(cm CertManager) {
	s.certManager = cm
}

// CertManager returns the certificate manager, or nil if TLS is not enabled.
func (s *Server) CertManager() CertManager {
	return s.certManager
}

// SetTenantService wires the tenant service for tenant-context and tenant-scoped routes.
func (s *Server) SetTenantService(svc tenantAdmin) {
	s.tenantSvc = normalizeNilInterface(svc)
	if s.tenantSvc == nil {
		s.tenantQuotaReader = nil
		if !s.auditEmitterManual {
			s.auditEmitter = nil
		}
		s.applyTenantQuotaDependenciesToStorageHandler()
		return
	}
	reader, ok := s.tenantSvc.(tenantQuotaReader)
	if !ok {
		s.tenantQuotaReader = nil
	} else {
		s.tenantQuotaReader = reader
	}
	if !s.auditEmitterManual {
		s.auditEmitter = tenant.NewAuditEmitterWithInserter(s.tenantSvc, s.currentLogger())
	}
	s.applyTenantQuotaDependenciesToStorageHandler()
}

// SetOrgStore wires the organization store for admin org endpoints.
func (s *Server) SetOrgStore(store tenant.OrgStore) {
	s.orgStore = normalizeNilInterface(store)
}

// SetTeamStore wires the team store for admin team endpoints.
func (s *Server) SetTeamStore(store tenant.TeamStore) {
	s.teamStore = normalizeNilInterface(store)
}

// SetOrgMembershipStore wires the org membership store for admin org membership endpoints.
func (s *Server) SetOrgMembershipStore(store tenant.OrgMembershipStore) {
	s.orgMembershipStore = normalizeNilInterface(store)
}

// SetTeamMembershipStore wires the team membership store for admin team membership endpoints.
func (s *Server) SetTeamMembershipStore(store tenant.TeamMembershipStore) {
	s.teamMembershipStore = normalizeNilInterface(store)
}

// SetPermissionResolver wires tenant permission resolution middleware dependencies.
func (s *Server) SetPermissionResolver(resolver *tenant.PermissionResolver) {
	s.permResolver = resolver
}

// SetUsageAccumulator wires the usage accumulator used by tenant quota enforcement.
func (s *Server) SetUsageAccumulator(ua *tenant.UsageAccumulator) {
	s.usageAccumulator = normalizeNilInterface(ua)
	s.applyTenantQuotaDependenciesToStorageHandler()
}

// SetQuotaChecker wires the tenant quota decision logic for enforcement.
func (s *Server) SetQuotaChecker(checker tenant.QuotaChecker) {
	s.quotaChecker = normalizeNilInterface(checker)
	s.applyTenantQuotaDependenciesToStorageHandler()
}

// SetTenantRateLimiter wires the sliding-window tenant request rate limiter.
func (s *Server) SetTenantRateLimiter(limiter *tenant.TenantRateLimiter) {
	s.tenantRateLimiter = limiter
}

// SetTenantConnCounter wires the tenant websocket connection counter.
func (s *Server) SetTenantConnCounter(counter *tenant.TenantConnCounter) {
	s.tenantConnCounter = counter
}

// SetTenantBreakerTracker wires the per-tenant circuit breaker tracker.
func (s *Server) SetTenantBreakerTracker(tracker *tenant.TenantBreakerTracker) {
	s.tenantBreakerTracker = tracker
}

// SetBillingService wires the billing lifecycle service.
func (s *Server) SetBillingService(svc billing.BillingService) {
	s.billingService = normalizeNilInterface(svc)
}

// SetSupportService wires the support ticket service.
func (s *Server) SetSupportService(svc support.SupportService) {
	s.supportSvc = normalizeNilInterface(svc)
}

// SetAuditEmitter wires the centralized tenant audit emitter for lifecycle/membership/quota/guard events.
func (s *Server) SetAuditEmitter(emitter *tenant.AuditEmitter) {
	s.auditEmitter = normalizeNilInterface(emitter)
	s.auditEmitterManual = s.auditEmitter != nil
	if s.auditEmitter == nil && s.tenantSvc != nil {
		s.auditEmitter = tenant.NewAuditEmitterWithInserter(s.tenantSvc, s.currentLogger())
	}
}

// AuditEmitter returns the centralized tenant audit emitter, or nil if not configured.
func (s *Server) AuditEmitter() *tenant.AuditEmitter {
	return s.auditEmitter
}

func (s *Server) applyTenantQuotaDependenciesToStorageHandler() {
	if s.storageHandler == nil {
		return
	}
	s.storageHandler.SetTenantQuota(s.tenantQuotaReader, s.quotaChecker, s.usageAccumulator)
	s.storageHandler.SetTenantQuotaTelemetry(s.tenantMetrics, s.auditEmitter)
}

// SetMatviewAdmin wires the matview admin facade for admin API endpoints.
func (s *Server) SetMatviewAdmin(svc matviewAdmin) {
	s.matviewSvc = svc
}

// SetEmailTemplateService wires the email template service for admin API endpoints.
func (s *Server) SetEmailTemplateService(svc emailTemplateAdmin) {
	s.emailTplSvc = svc
}

// SetPushService wires the push service for user/admin push API endpoints.
func (s *Server) SetPushService(svc pushAdmin) {
	s.pushSvc = svc
}

// SetNotificationService wires the notification service for user/admin endpoints.
func (s *Server) SetNotificationService(svc notificationAdmin) {
	s.notifSvc = normalizeNilInterface(svc)
}

// SetVaultStore wires vault secret storage for admin secret API endpoints.
func (s *Server) SetVaultStore(store VaultSecretStore) {
	s.vaultStore = store
}

// SetEdgeFuncService wires the edge function service for public trigger and admin endpoints.
func (s *Server) SetEdgeFuncService(svc edgeFuncAdmin) {
	s.edgeFuncSvc = svc
	if s.authSvc != nil && svc != nil {
		dispatcher := auth.NewHookDispatcher(s.cfg.Auth.Hooks, &edgeFuncHookAdapter{svc: svc}, s.logger)
		s.authSvc.SetHookDispatcher(dispatcher)
	}
	if svc != nil {
		if setter, ok := svc.(interface {
			SetInvocationLogWriter(edgefunc.InvocationLogWriter)
		}); ok {
			setter.SetInvocationLogWriter(&edgeFuncDrainWriter{
				managerProvider: func() *logging.DrainManager {
					return s.currentDrainManager()
				},
			})
		}
	}
}

// SetTriggerServices wires the trigger admin services for the admin API.
func (s *Server) SetTriggerServices(db dbTriggerAdmin, cron cronTriggerAdmin, st storageTriggerAdmin, invoker edgefunc.FunctionInvoker) {
	s.dbTriggerSvc = normalizeNilInterface(db)
	s.cronTriggerSvc = normalizeNilInterface(cron)
	s.storageTriggerSvc = normalizeNilInterface(st)
	s.funcInvoker = normalizeNilInterface(invoker)
}

// normalizeNilInterface converts typed-nil interface values to a true nil interface.
func normalizeNilInterface[T any](value T) T {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return value
	}
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if rv.IsNil() {
			var zero T
			return zero
		}
	}
	return value
}

// SetAILogStore wires the AI call log store for admin AI endpoints.
func (s *Server) SetAILogStore(store aiLogStore) {
	s.aiLogStore = store
}

// SetPromptStore wires the AI prompt store for admin prompt endpoints.
func (s *Server) SetPromptStore(store promptStore) {
	s.promptStore = store
}

// SetAIAssistantService wires the dashboard AI assistant service.
func (s *Server) SetAIAssistantService(svc assistantService) {
	s.assistantSvc = normalizeNilInterface(svc)
}

// SetExtService wires the extension management service for admin endpoints.
func (s *Server) SetExtService(svc extensionAdmin) {
	s.extService = svc
}

// SetFDWService wires the FDW management service for admin endpoints.
func (s *Server) SetFDWService(svc fdwAdmin) {
	s.fdwService = svc
}

// SetBackupService wires the backup service for admin backup endpoints.
func (s *Server) SetBackupService(svc backupAdmin) {
	s.backupService = svc
}

// SetBranchService wires the branch service for admin branch endpoints.
func (s *Server) SetBranchService(svc branchAdmin) {
	s.branchService = svc
}

// SetPITRService wires the PITR service for admin PITR endpoints.
func (s *Server) SetPITRService(svc pitrAdmin) {
	s.pitrService = svc
}

// SetGraphQLHandler wires the GraphQL HTTP handler.
func (s *Server) SetGraphQLHandler(h http.Handler) {
	s.graphqlHandler = h
}

// RealtimeHub returns the server's realtime event hub.
func (s *Server) RealtimeHub() *realtime.Hub {
	return s.hub
}

// IsAdminToken reports whether the request carries a valid admin token.
func (s *Server) IsAdminToken(r *http.Request) bool {
	return s.isAdminToken(r)
}

// SetEmbedder wires the embedding function for semantic search.
func (s *Server) SetEmbedder(fn api.EmbedFunc) {
	s.embedFn = fn
}

// SetEmbeddingConfiguredDimension wires configured embedding model dimension
// for semantic-search preflight validation.
func (s *Server) SetEmbeddingConfiguredDimension(dim int) {
	s.embedConfigDim = dim
}
