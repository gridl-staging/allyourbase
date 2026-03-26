// Package server provides the HTTP server implementation for AYB, including route configuration, middleware setup, and request handling for APIs, admin endpoints, and realtime communications.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/api"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/logging"
	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/observability"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/replica"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/allyourbase/ayb/internal/status"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/support"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/webhooks"
	"github.com/allyourbase/ayb/internal/ws"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Server is the main HTTP server for AYB.
// Is the main HTTP server for AYB, handling authentication, authorization, GraphQL and REST APIs, realtime events, webhooks, storage, edge functions, and admin management. It orchestrates database connections, integrates multiple services, applies middleware for logging and rate limiting, and manages graceful shutdown.
type Server struct {
	cfg                    *config.Config
	router                 *chi.Mux
	http                   *http.Server
	logger                 *slog.Logger
	schema                 *schema.CacheHolder
	pool                   *pgxpool.Pool
	poolRouter             *replica.PoolRouter
	healthChecker          *replica.HealthChecker
	lifecycleService       replicaLifecycle // nil when replica lifecycle not wired
	tenantConnAcquire      tenantConnAcquireFunc
	authSvc                *auth.Service     // nil when auth disabled
	authHandler            *auth.Handler     // nil when auth disabled
	samlSvc                *auth.SAMLService // nil when SAML is not configured
	authRL                 *auth.RateLimiter // nil when auth disabled
	assistantRL            *auth.RateLimiter // nil when dashboard AI assistant rate limiting is unavailable
	apiRL                  *auth.RateLimiter // nil when API rate limit is unavailable
	apiAnonRL              *auth.RateLimiter // nil when API anonymous rate limit is unavailable
	authSensitiveRL        *auth.RateLimiter // stricter limiter for sensitive auth endpoints
	appRL                  *auth.AppRateLimiter
	assistantRateLimit     int               // parsed dashboard_ai.rate_limit count
	apiRateLimit           int               // parsed config.RateLimit.API limit
	apiAnonRateLimit       int               // parsed config.RateLimit.APIAnonymous limit
	adminRL                *auth.RateLimiter // admin login rate limiter
	storageCDNPurgeAllRL   *auth.RateLimiter // stricter limiter for admin storage CDN purge_all requests
	hub                    *realtime.Hub
	wsHandler              *ws.Handler // nil when not wired (always wired in New)
	connManager            *realtime.ConnectionManager
	realtimeInspector      *realtime.Inspector
	webhookDispatcher      webhookDispatcher // nil when pool is nil
	jobService             *jobs.Service     // nil when jobs disabled or pool is nil
	tenantSvc              tenantAdmin       // nil when tenant service not configured
	orgStore               tenant.OrgStore
	teamStore              tenant.TeamStore
	orgMembershipStore     tenant.OrgMembershipStore
	teamMembershipStore    tenant.TeamMembershipStore
	permResolver           *tenant.PermissionResolver
	tenantQuotaReader      tenantQuotaReader // nil when tenant quotas unavailable
	tenantRateLimiter      *tenant.TenantRateLimiter
	tenantConnCounter      *tenant.TenantConnCounter
	usageAccumulator       *tenant.UsageAccumulator
	quotaChecker           tenant.QuotaChecker
	tenantMetrics          tenantMetricsRecorder
	tenantBreakerTracker   *tenant.TenantBreakerTracker
	auditEmitter           *tenant.AuditEmitter     // nil when tenant service not configured
	auditEmitterManual     bool                     // true when explicitly set via SetAuditEmitter
	matviewSvc             matviewAdmin             // nil when pool is nil
	emailTplSvc            emailTemplateAdmin       // nil when pool is nil
	pushSvc                pushAdmin                // nil when push is disabled or not wired
	notifSvc               notificationAdmin        // nil when notifications are not wired
	vaultStore             VaultSecretStore         // nil when vault secret management not wired
	edgeFuncSvc            edgeFuncAdmin            // nil when edge functions not wired
	dbTriggerSvc           dbTriggerAdmin           // nil when edge functions not wired
	cronTriggerSvc         cronTriggerAdmin         // nil when edge functions not wired
	storageTriggerSvc      storageTriggerAdmin      // nil when edge functions not wired
	funcInvoker            edgefunc.FunctionInvoker // nil when edge functions not wired; for manual cron runs
	billingService         billing.BillingService   // nil when billing lifecycle not configured
	supportSvc             support.SupportService   // nil when support service not configured
	supportUserEmailLookup func(context.Context, string) (string, error)
	fieldEncryptor         *api.FieldEncryptor
	observabilityMu        sync.RWMutex
	adminMu                sync.RWMutex
	adminAuth              *adminAuth // nil when admin.password not set
	startTime              time.Time
	drainManager           *logging.DrainManager
	logBuffer              *LogBuffer   // nil when not using buffered logging
	smsProvider            sms.Provider // nil when SMS disabled
	smsProviderName        string       // "twilio", "plivo", etc. — stored in messages for audit
	smsAllowedCountries    []string     // country allowlist from config
	msgStore               messageStore // nil when pool is nil
	httpMetrics            *observability.HTTPMetrics
	infraMetrics           *observability.InfraMetrics
	storagePollerCancel    context.CancelFunc
	storageSvc             *storage.Service // nil when storage features are disabled
	storageHandler         *storage.Handler
	statusHistory          *status.StatusHistory
	statusIncidentStore    status.IncidentStore
	statusChecker          *status.Checker
	requestLogger          *RequestLogger
	tracerProvider         *sdktrace.TracerProvider
	aiLogStore             aiLogStore        // nil when AI not wired
	promptStore            promptStore       // nil when AI not wired
	assistantSvc           assistantService  // nil when dashboard AI assistant not wired
	extService             extensionAdmin    // nil when extensions not wired
	fdwService             fdwAdmin          // nil when fdw management not wired
	backupService          backupAdmin       // nil when backup not wired
	branchService          branchAdmin       // nil when branching not wired
	pitrService            pitrAdmin         // nil when PITR not wired
	graphqlHandler         http.Handler      // nil when GraphQL not wired
	openapiJSONCache       openapiCache      // cached OpenAPI 3.1 JSON spec
	embedFn                api.EmbedFunc     // nil when semantic search is not configured
	embedConfigDim         int               // configured embedding dimension for semantic search model
	mailer                 mailer.Mailer     // nil when email sending not configured
	emailRL                *auth.RateLimiter // nil when email rate limiting not configured
	domainStore            domainManager     // nil when pool is nil
	siteStore              siteManager       // nil when pool is nil
	certManager            CertManager       // nil when TLS not enabled or not wired
	usageSrc               usageDataSource   // nil when pool is nil
	usageAggregate         usageAggregateService
	orgUsageQuerier        orgUsageQuerier // nil when pool is nil
	orgAuditQuerier        orgAuditQuerier // nil when pool is nil
	routeTableMu           sync.RWMutex
	routeTable             RouteTable
}

type webhookDispatcher interface {
	Enqueue(event *realtime.Event)
	SetDeliveryStore(ds webhooks.DeliveryStore)
	StartPruner(interval, retention time.Duration)
	Close()
}

// New creates a new Server with middleware and routes configured.
// authSvc and storageSvc may be nil when their features are disabled.
func New(cfg *config.Config, logger *slog.Logger, schemaCache *schema.CacheHolder, pool *pgxpool.Pool, authSvc *auth.Service, storageSvc *storage.Service) *Server {
	return newServer(cfg, logger, schemaCache, pool, authSvc, storageSvc, nil)
}

// NewWithFieldEncryptor creates a new Server with an optional API field encryptor.
// authSvc and storageSvc may be nil when their features are disabled.
func NewWithFieldEncryptor(cfg *config.Config, logger *slog.Logger, schemaCache *schema.CacheHolder, pool *pgxpool.Pool, authSvc *auth.Service, storageSvc *storage.Service, fieldEncryptor *api.FieldEncryptor) *Server {
	return newServer(cfg, logger, schemaCache, pool, authSvc, storageSvc, fieldEncryptor)
}

// TODO: Document newServer.
func newServer(cfg *config.Config, logger *slog.Logger, schemaCache *schema.CacheHolder, pool *pgxpool.Pool, authSvc *auth.Service, storageSvc *storage.Service, fieldEncryptor *api.FieldEncryptor) *Server {
	r := chi.NewRouter()
	tracerProvider, outboundTransport := initTracing(cfg, r, logger)
	drainManager, logger := initDrainManager(cfg, logger, outboundTransport)
	var replicaStore replica.ReplicaStore
	if pool != nil {
		replicaStore = newReplicaStore(pool)
		bootstrapReplicaStoreFromConfig(context.Background(), cfg, replicaStore, logger)
	}
	replicaResult := buildReplicaRouting(context.Background(), replicaStore, pool, logger)
	poolRouter, healthChecker := replicaResult.router, replicaResult.checker
	lifecycleService := buildLifecycleService(cfg, replicaStore, pool, replicaResult, logger)
	httpMetrics, infraMetrics, tenantMetrics := initObservability(cfg, pool, poolRouter, healthChecker, logger)
	hub, wsHandler, connManager, realtimeInspector := initRealtimeHub(cfg, pool, schemaCache, authSvc, logger, httpMetrics)
	webhookDispatcher := initWebhookDispatcher(cfg, pool, logger, outboundTransport)
	statusHistory, statusIncidentStore, statusChecker := initStatusSystem(cfg, pool)

	// Global middleware.
	if httpMetrics != nil {
		r.Use(httpMetrics.Middleware)
	}
	tenantMetricsMiddleware := observability.TenantContextMiddleware(httpMetrics)
	var serverRef *Server
	r.Use(middleware.RequestID)
	r.Use(requestLogger(func() *slog.Logger {
		if serverRef != nil {
			if logger := serverRef.currentLogger(); logger != nil {
				return logger
			}
		}
		return logger
	}))
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)
	r.Use(corsMiddleware(cfg.Server.CORSAllowedOrigins))
	reqLogger := newServerRequestLogger(cfg, logger, pool)
	r.Use(requestLogMiddleware(reqLogger, func() *logging.DrainManager {
		if serverRef != nil {
			return serverRef.currentDrainManager()
		}
		return drainManager
	}))
	applyDeferredServerRouteMiddleware(r, &serverRef)

	s := &Server{
		cfg:                 cfg,
		router:              r,
		logger:              logger,
		schema:              schemaCache,
		pool:                pool,
		poolRouter:          poolRouter,
		healthChecker:       healthChecker,
		lifecycleService:    lifecycleService,
		authSvc:             authSvc,
		storageSvc:          storageSvc,
		hub:                 hub,
		wsHandler:           wsHandler,
		connManager:         connManager,
		realtimeInspector:   realtimeInspector,
		webhookDispatcher:   webhookDispatcher,
		fieldEncryptor:      fieldEncryptor,
		startTime:           time.Now(),
		httpMetrics:         httpMetrics,
		infraMetrics:        infraMetrics,
		tenantMetrics:       tenantMetrics,
		requestLogger:       reqLogger,
		statusHistory:       statusHistory,
		statusIncidentStore: statusIncidentStore,
		statusChecker:       statusChecker,
		drainManager:        drainManager,
		tracerProvider:      tracerProvider,
	}
	serverRef = s
	s.initDefaults(logger)
	r.Get("/health", s.handleHealth)
	r.Get("/favicon.ico", handleFavicon)
	r.Get("/api/openapi.yaml", handleOpenAPISpec)
	r.Get("/api/openapi.json", s.handleOpenAPIJSON)
	r.Get("/api/docs", handleDocs)
	r.HandleFunc("/functions/v1/{name}", s.handleEdgeFuncInvokeProxy)
	r.HandleFunc("/functions/v1/{name}/*", s.handleEdgeFuncInvokeProxy)
	registerMetricsEndpoint(r, cfg, httpMetrics)
	s.registerServerAPIRoutes(r, tenantMetricsMiddleware)
	registerAdminSPA(r, cfg)
	return s
}

func newServerRequestLogger(cfg *config.Config, logger *slog.Logger, pool *pgxpool.Pool) *RequestLogger {
	if !cfg.Logging.RequestLogEnabled || pool == nil {
		return nil
	}
	return NewRequestLogger(RequestLogConfig{
		Enabled:           true,
		BatchSize:         cfg.Logging.RequestLogBatchSize,
		FlushIntervalSecs: cfg.Logging.RequestLogFlushIntervalSecs,
		QueueSize:         cfg.Logging.RequestLogQueueSize,
		RetentionDays:     cfg.Logging.RequestLogRetentionDays,
	}, logger, pool)
}

// TODO: Document applyDeferredServerRouteMiddleware.
func applyDeferredServerRouteMiddleware(r chi.Router, serverRef **Server) {
	useDeferredServerMiddleware(r, serverRef, func(s *Server, next http.Handler) http.Handler {
		return s.hostRouteMiddleware(next)
	})
	useDeferredServerMiddleware(r, serverRef, func(s *Server, next http.Handler) http.Handler {
		return s.siteRuntimeMiddleware(next)
	})
}

func useDeferredServerMiddleware(r chi.Router, serverRef **Server, wrap func(*Server, http.Handler) http.Handler) {
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if s := *serverRef; s != nil {
				wrap(s, next).ServeHTTP(w, req)
				return
			}
			next.ServeHTTP(w, req)
		})
	})
}

// TODO: Document Server.registerServerAPIRoutes.
func (s *Server) registerServerAPIRoutes(r chi.Router, tenantMetricsMiddleware func(http.Handler) http.Handler) {
	serverAllowlist := newIPAllowlist("server.allowed_ips", s.cfg.Server.AllowedIPs, s.logger)
	adminAllowlist := newIPAllowlist("admin.allowed_ips", s.cfg.Admin.AllowedIPs, s.logger)
	r.Route("/api", func(r chi.Router) {
		r.Use(apiRouteAllowlistMiddleware(serverAllowlist, adminAllowlist))
		r.Use(replica.ReplicaRoutingMiddleware(s.poolRouter))
		r.Use(s.resolveTenantContext)
		r.Use(s.enforceTenantAvailability)
		r.Use(tenantMetricsMiddleware)
		r.Use(s.tenantRequestRateMiddlewareDynamic)
		r.Use(s.recordBreakerOutcome)
		r.Use(s.setTenantSearchPath)

		// Public /status endpoint (NOT admin-gated).
		if s.cfg.Status.Enabled && s.cfg.Status.PublicEndpointEnabled {
			r.Get("/status", handlePublicStatus(s.statusHistory, s.statusIncidentStore))
		}

		s.registerAdminRoutes(r)
		s.registerWebhookRoutes(r)
		s.registerAuthRoutes(r)
		s.registerStorageRoutes(r)
		s.registerAPIRoutes(r)
	})
}

// Router returns the chi router for registering additional routes.
func (s *Server) Router() *chi.Mux {
	return s.router
}

func (s *Server) currentLogger() *slog.Logger {
	s.observabilityMu.RLock()
	defer s.observabilityMu.RUnlock()
	return s.logger
}

func (s *Server) currentDrainManager() *logging.DrainManager {
	s.observabilityMu.RLock()
	defer s.observabilityMu.RUnlock()
	return s.drainManager
}
