package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/logging"
	"github.com/allyourbase/ayb/internal/observability"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/replica"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sites"
	"github.com/allyourbase/ayb/internal/status"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/webhooks"
	"github.com/allyourbase/ayb/internal/ws"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Package-level test-seam variables for replica pool construction.
var newReplicaPool = pgxpool.New

var pingReplicaPool = func(ctx context.Context, pool *pgxpool.Pool) error {
	return pool.Ping(ctx)
}

var newReplicaStore = func(pool *pgxpool.Pool) replica.ReplicaStore {
	return replica.NewPostgresReplicaStore(pool)
}

var newWebhookDispatcher = func(store webhooks.WebhookLister, logger *slog.Logger) webhookDispatcher {
	return webhooks.NewDispatcher(store, logger)
}

// initTracing sets up OpenTelemetry tracing, adds tracing middleware to the
// router, and creates an instrumented HTTP transport for outbound calls.
func initTracing(cfg *config.Config, r *chi.Mux, logger *slog.Logger) (*sdktrace.TracerProvider, http.RoundTripper) {
	var tracerProvider *sdktrace.TracerProvider
	if tp, err := observability.NewTracerProvider(observability.TelemetryConfig{
		Enabled:      cfg.Telemetry.Enabled,
		OTLPEndpoint: cfg.Telemetry.OTLPEndpoint,
		ServiceName:  cfg.Telemetry.ServiceName,
		SampleRate:   cfg.Telemetry.SampleRate,
	}); err != nil {
		logger.Error("initializing tracer provider", "error", err)
	} else if tp != nil {
		observability.SetGlobalTracerAndPropagator(tp)
		tracerProvider = tp
		r.Use(observability.OtelChiMiddleware(cfg.Telemetry.ServiceName, r))
	}

	var outboundTransport http.RoundTripper
	if tracerProvider != nil {
		outboundTransport = observability.NewOtelHTTPTransport(tracerProvider)
		auth.SetOAuthHTTPTransport(outboundTransport)
	}
	return tracerProvider, outboundTransport
}

// initDrainManager creates a drain manager from config, adding enabled log
// drains and wrapping the logger with drain fanout. Returns nil manager if no
// drains are configured.
func initDrainManager(cfg *config.Config, logger *slog.Logger, transport http.RoundTripper) (*logging.DrainManager, *slog.Logger) {
	if len(cfg.Logging.Drains) == 0 {
		return nil, logger
	}
	drainManager := logging.NewDrainManager()
	for i := range cfg.Logging.Drains {
		cfg.Logging.Drains[i] = normalizeLogDrainConfig(cfg.Logging.Drains[i], i)
		if cfg.Logging.Drains[i].Enabled != nil && !*cfg.Logging.Drains[i].Enabled {
			continue
		}
		drain, err := newLogDrainFromConfig(cfg.Logging.Drains[i], transport)
		if err != nil {
			logger.Warn("invalid configured log drain", "index", i, "id", cfg.Logging.Drains[i].ID, "error", err)
			continue
		}
		drainManager.AddDrain(cfg.Logging.Drains[i].ID, drain, logging.DrainWorkerConfig{
			BatchSize:     cfg.Logging.Drains[i].BatchSize,
			FlushInterval: time.Duration(cfg.Logging.Drains[i].FlushIntervalSecs) * time.Second,
			QueueSize:     10000,
		})
	}
	logger = wrapLoggerForDrainFanout(logger, drainManager)
	return drainManager, logger
}

// initObservability creates HTTP metrics, infrastructure metrics, and tenant
// metrics collectors. Returns nil values when metrics are disabled.
func initObservability(cfg *config.Config, pool *pgxpool.Pool, poolRouter *replica.PoolRouter, healthChecker *replica.HealthChecker, logger *slog.Logger) (*observability.HTTPMetrics, *observability.InfraMetrics, tenantMetricsRecorder) {
	if !cfg.Metrics.Enabled {
		return nil, nil, nil
	}

	var poolStatFn observability.PoolStatFunc
	if pool != nil {
		poolStatFn = func() (total, idle, inUse, max int32) {
			s := pool.Stat()
			return s.TotalConns(), s.IdleConns(), s.AcquiredConns(), s.MaxConns()
		}
	}
	infraMetrics := observability.NewInfraMetrics(poolStatFn)
	collectors := []observability.InfraCollector{infraMetrics.Collector()}
	if poolRouter != nil {
		replicaMetrics := observability.NewReplicaMetrics(
			func() []observability.ReplicaStatEntry {
				if healthChecker == nil {
					return nil
				}
				statuses := healthChecker.Statuses()
				entries := make([]observability.ReplicaStatEntry, 0, len(statuses))
				for _, status := range statuses {
					entries = append(entries, observability.ReplicaStatEntry{
						URL:      replica.SanitizeReplicaURL(status.Config.URL),
						State:    status.State.String(),
						LagBytes: status.LagBytes,
					})
				}
				return entries
			},
			func() (primaryReads, replicaReads uint64) {
				if poolRouter == nil {
					return 0, 0
				}
				return poolRouter.RoutingStats()
			},
		)
		collectors = append(collectors, replicaMetrics.Collector())
	}

	httpMetrics, err := observability.NewHTTPMetrics(collectors...)
	if err != nil {
		logger.Error("initializing metrics", "error", err)
		return nil, infraMetrics, nil
	}

	var tenantMetrics tenantMetricsRecorder
	if httpMetrics != nil && httpMetrics.MeterProvider() != nil {
		tm, tmErr := observability.NewTenantMetrics(httpMetrics.MeterProvider().Meter("ayb"))
		if tmErr != nil {
			logger.Error("initializing tenant metrics", "error", tmErr)
		} else {
			tenantMetrics = tm
		}
	}
	return httpMetrics, infraMetrics, tenantMetrics
}

// initRealtimeHub creates the realtime event hub, WebSocket handler with
// broadcast/presence, connection manager with lifecycle governance, and
// realtime inspector. Registers realtime metrics if httpMetrics is available.
func initRealtimeHub(cfg *config.Config, pool *pgxpool.Pool, schemaCache *schema.CacheHolder, authSvc *auth.Service, logger *slog.Logger, httpMetrics *observability.HTTPMetrics) (*realtime.Hub, *ws.Handler, *realtime.ConnectionManager, *realtime.Inspector) {
	hub := realtime.NewHub(logger)

	// WebSocket realtime transport.
	var wsAuthValidator ws.TokenValidator
	if authSvc != nil {
		wsAuthValidator = authSvc
	}
	wsHandler := ws.NewHandler(wsAuthValidator, logger)
	wsHandler.PingInterval = time.Duration(cfg.Realtime.HeartbeatIntervalSeconds) * time.Second
	wsHandler.Broadcast = ws.NewBroadcastHub(
		logger,
		ws.BroadcastHubOptions{
			RateLimit:       cfg.Realtime.BroadcastRateLimitPerSecond,
			RateWindow:      time.Second,
			MaxPayloadBytes: cfg.Realtime.BroadcastMaxMessageBytes,
		},
	)
	wsHandler.Presence = ws.NewPresenceHub(
		logger,
		ws.PresenceHubOptions{
			LeaveTimeout: time.Duration(cfg.Realtime.PresenceLeaveTimeoutSeconds) * time.Second,
		},
	)
	wsBridge := realtime.NewWSBridge(hub, pool, schemaCache, logger)
	wsBridge.SetupHandler(wsHandler)
	realtimeInspector := realtime.NewInspector(hub, wsHandler)

	// Realtime metrics aggregator for OTel/Prometheus.
	if cfg.Metrics.Enabled && httpMetrics != nil {
		realtimeMetrics := observability.NewRealtimeMetricsAggregator(
			func() int { return realtimeInspector.Snapshot().Connections.SSE },
			func() int { return realtimeInspector.Snapshot().Connections.WS },
			func() int { return len(realtimeInspector.Snapshot().Subscriptions.Channels.Broadcast) },
			func() int { return len(realtimeInspector.Snapshot().Subscriptions.Channels.Presence) },
			func() uint64 { return wsHandler.Broadcast.MessagesSent() },
			func() uint64 { return wsHandler.Presence.SyncedCount() },
		)
		meter := httpMetrics.MeterProvider().Meter("ayb")
		if err := realtimeMetrics.Collector()(meter); err != nil {
			logger.Error("initializing realtime metrics", "error", err)
		}
	}

	// Connection manager: cross-transport lifecycle governance driven by RealtimeConfig.
	connManager := realtime.NewConnectionManager(realtime.ConnectionManagerOptions{
		MaxConnectionsPerUser: cfg.Realtime.MaxConnectionsPerUser,
		IdleTimeout:           60 * time.Second,
	})
	bridgeOnConnect := wsHandler.OnConnect
	bridgeOnDisconnect := wsHandler.OnDisconnect
	wsHandler.OnConnect = func(c *ws.Conn) {
		meta := realtime.ConnectionMeta{
			ClientID:  c.ID(),
			UserID:    realtime.UserKey(c.Claims()),
			Transport: "ws",
			// CloseFunc uses 1001 Going Away for CM-initiated closes (idle, drain,
			// force-disconnect). The limit-violation case below uses 4008 explicitly.
			CloseFunc: func() { c.Close(1001, "going away") },
			HasSubscriptions: func() bool {
				subs := c.Subscriptions()
				channels := c.Channels()
				return len(subs) > 0 || len(channels) > 0
			},
		}
		if err := connManager.Register(meta); err != nil {
			if errors.Is(err, realtime.ErrDraining) {
				c.Close(1001, "server shutting down")
			} else {
				c.Close(4008, "connection limit exceeded")
			}
			return
		}
		if bridgeOnConnect != nil {
			bridgeOnConnect(c)
		}
	}
	wsHandler.OnDisconnect = func(c *ws.Conn) {
		if bridgeOnDisconnect != nil {
			bridgeOnDisconnect(c)
		}
		connManager.Deregister(c.ID())
	}

	return hub, wsHandler, connManager, realtimeInspector
}

// initWebhookDispatcher creates the webhook dispatcher when a pool is available,
// configuring outbound transport and starting the pruner if jobs are disabled.
func initWebhookDispatcher(cfg *config.Config, pool *pgxpool.Pool, logger *slog.Logger, transport http.RoundTripper) webhookDispatcher {
	if pool == nil {
		return nil
	}
	whStore := webhooks.NewStore(pool)
	dispatcher := newWebhookDispatcher(whStore, logger)
	if transport != nil {
		if configured, ok := dispatcher.(interface{ SetHTTPTransport(http.RoundTripper) }); ok {
			configured.SetHTTPTransport(transport)
		}
	}
	dispatcher.SetDeliveryStore(whStore)
	// When the job queue is enabled, the scheduled webhook_delivery_prune
	// job handles pruning. Only start the legacy timer-based pruner when
	// jobs are disabled (default) for backward compatibility.
	if !cfg.Jobs.Enabled {
		dispatcher.StartPruner(1*time.Hour, 7*24*time.Hour)
	}
	return dispatcher
}

// initStatusSystem creates the status history, incident store, and status
// checker when status monitoring is enabled.
func initStatusSystem(cfg *config.Config, pool *pgxpool.Pool) (*status.StatusHistory, status.IncidentStore, *status.Checker) {
	if !cfg.Status.Enabled {
		return nil, nil, nil
	}
	statusHistory := status.NewStatusHistory(cfg.Status.HistorySize)
	probes := []status.Probe{
		status.NewStorageProbe(),
		status.NewAuthProbe(),
		status.NewRealtimeProbe(),
		status.NewFunctionsProbe(),
	}
	var statusIncidentStore status.IncidentStore
	if pool != nil {
		statusIncidentStore = status.NewPgIncidentStore(pool)
		probes = append([]status.Probe{status.NewDatabaseProbe(pool)}, probes...)
	}
	statusChecker := status.NewChecker(
		probes,
		statusHistory,
		time.Duration(cfg.Status.CheckIntervalSeconds)*time.Second,
	)
	return statusHistory, statusIncidentStore, statusChecker
}

// replicaRoutingResult holds the output of buildReplicaRouting so callers
// can wire both the router/checker and pass the pool map to LifecycleService.
type replicaRoutingResult struct {
	router       *replica.PoolRouter
	checker      *replica.HealthChecker
	initialPools map[string]*pgxpool.Pool
}

// buildReplicaRouting creates a PoolRouter and HealthChecker from persisted
// topology records, connecting to active replicas and skipping those that fail
// to connect or ping. It always returns a usable router/checker when
// store+primary are present, even with zero connected replicas (pass-through
// mode), so that AddReplica works on a fresh or degraded system.
func buildReplicaRouting(ctx context.Context, store replica.ReplicaStore, primary *pgxpool.Pool, logger *slog.Logger) replicaRoutingResult {
	empty := replicaRoutingResult{}
	if store == nil || primary == nil {
		return empty
	}
	if logger == nil {
		logger = slog.Default()
	}
	if ctx == nil {
		ctx = context.Background()
	}

	records, err := safeReplicaStoreList(ctx, store)
	if err != nil {
		logger.Warn("replica topology load failed; replica routing disabled", "error", err)
		return empty
	}

	replicaPools := make([]replica.ReplicaPool, 0)
	initialPools := make(map[string]*pgxpool.Pool)
	for _, record := range records {
		if record.Role != replica.TopologyRoleReplica || record.State != replica.TopologyStateActive {
			continue
		}
		connectionURL := record.ConnectionURL()
		sanitizedConnectionURL := replica.SanitizeReplicaURL(connectionURL)
		dialURL := replica.DialURLWithPrimaryCredentials(connectionURL, primary)
		pool, dialErr := newReplicaPool(ctx, dialURL)
		if dialErr != nil {
			logger.Warn("replica connection failed; skipping replica", "name", record.Name, "url", sanitizedConnectionURL, "error", dialErr)
			continue
		}

		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		pingErr := pingReplicaPool(pingCtx, pool)
		cancel()
		if pingErr != nil {
			pool.Close()
			logger.Warn("replica ping failed; skipping replica", "name", record.Name, "url", sanitizedConnectionURL, "error", pingErr)
			continue
		}

		replicaConfig := replica.NormalizeReplicaConfig(config.ReplicaConfig{
			URL:         connectionURL,
			Weight:      record.Weight,
			MaxLagBytes: record.MaxLagBytes,
		})
		replicaPools = append(replicaPools, replica.ReplicaPool{
			Name:   record.Name,
			Pool:   pool,
			Config: replicaConfig,
		})
		initialPools[record.Name] = pool
	}

	poolRouter := replica.NewPoolRouter(primary, replicaPools, logger)
	healthChecker := replica.NewHealthChecker(poolRouter, 0, logger)
	if len(replicaPools) > 0 {
		logger.Info("replica routing enabled", "replicas", len(replicaPools))
	} else {
		logger.Info("replica routing enabled in pass-through mode; no active replicas connected")
	}
	return replicaRoutingResult{
		router:       poolRouter,
		checker:      healthChecker,
		initialPools: initialPools,
	}
}

// buildLifecycleService constructs the replica LifecycleService when
// all required dependencies are available. Returns nil otherwise.
func buildLifecycleService(cfg *config.Config, store replica.ReplicaStore, pool *pgxpool.Pool, routing replicaRoutingResult, logger *slog.Logger) replicaLifecycle {
	if store == nil || pool == nil || routing.router == nil || routing.checker == nil {
		return nil
	}
	auditLogger := audit.NewAuditLogger(cfg.Audit, pool)
	return replica.NewLifecycleService(store, routing.router, routing.checker, auditLogger, logger, routing.initialPools)
}

// normalizeLogDrainConfig fills in default values for a LogDrainConfig.
func normalizeLogDrainConfig(cfg config.LogDrainConfig, index int) config.LogDrainConfig {
	if cfg.ID == "" {
		cfg.ID = fmt.Sprintf("drain-%d", index)
	}
	if cfg.Enabled == nil {
		enabled := true
		cfg.Enabled = &enabled
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}
	if cfg.FlushIntervalSecs == 0 {
		cfg.FlushIntervalSecs = 5
	}
	if cfg.Headers == nil {
		cfg.Headers = map[string]string{}
	}
	return cfg
}

// newLogDrainFromConfig creates a LogDrain from config, validating that type
// and URL are set and that batch and flush parameters are non-negative.
func newLogDrainFromConfig(cfg config.LogDrainConfig, transport http.RoundTripper) (logging.LogDrain, error) {
	if cfg.Type == "" {
		return nil, fmt.Errorf("type is required")
	}
	switch cfg.Type {
	case "http", "datadog", "loki":
	default:
		return nil, fmt.Errorf("unsupported log drain type: %q", cfg.Type)
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("url is required")
	}
	if cfg.BatchSize < 0 {
		return nil, fmt.Errorf("batch_size must be non-negative, got %d", cfg.BatchSize)
	}
	if cfg.FlushIntervalSecs < 0 {
		return nil, fmt.Errorf("flush_interval_seconds must be non-negative, got %d", cfg.FlushIntervalSecs)
	}
	if cfg.Headers == nil {
		cfg.Headers = map[string]string{}
	}

	drainCfg := logging.DrainConfig{
		ID:                cfg.ID,
		Type:              cfg.Type,
		URL:               cfg.URL,
		Headers:           cfg.Headers,
		BatchSize:         cfg.BatchSize,
		FlushIntervalSecs: cfg.FlushIntervalSecs,
		Enabled:           cfg.Enabled != nil && *cfg.Enabled,
	}

	switch cfg.Type {
	case "http":
		drain := logging.NewHTTPDrain(drainCfg)
		if transport != nil {
			drain.SetHTTPTransport(transport)
		}
		return drain, nil
	case "datadog":
		drain := logging.NewDatadogDrain(drainCfg)
		if transport != nil {
			drain.SetHTTPTransport(transport)
		}
		return drain, nil
	case "loki":
		drain := logging.NewLokiDrain(drainCfg)
		if transport != nil {
			drain.SetHTTPTransport(transport)
		}
		return drain, nil
	}

	return nil, fmt.Errorf("unsupported log drain type: %q", cfg.Type)
}

// initDefaults wires post-construction defaults: pool-dependent stores, admin
// auth, storage handler, usage data source, and all rate limiters.
func (s *Server) initDefaults(logger *slog.Logger) {
	if s.authSvc != nil {
		s.appRL = auth.NewAppRateLimiter()
	}
	if s.pool != nil {
		s.msgStore = &pgMessageStore{pool: s.pool}
		s.domainStore = NewDomainStore(s.pool, audit.NewAuditLogger(s.cfg.Audit, s.pool))
		s.siteStore = sites.NewService(s.pool, logger)
		s.tenantConnAcquire = newTenantConnAcquire(s.pool)
	}
	if s.cfg.Admin.Password != "" {
		s.adminAuth = newAdminAuth(s.cfg.Admin.Password)
	} else if s.pool != nil {
		logger.Warn("admin password not set — admin endpoints are disabled until admin.password is configured.")
	}

	if s.pool != nil {
		tenantService := tenant.NewService(s.pool, logger)
		billingStore := billing.NewStore(s.pool)
		s.usageSrc = newUsageDataSource(tenantService, billingStore)
		s.usageAggregate = billing.NewUsageAggregator(s.pool, billingStore)
		s.orgUsageQuerier = &dbOrgUsageQuerier{pool: s.pool}
		s.orgAuditQuerier = &dbOrgAuditQuerier{pool: s.pool}
	}

	// Construct storage handler before route registration.
	if s.storageSvc != nil {
		s.storageHandler = storage.NewHandler(s.storageSvc, s.logger, s.cfg.Storage.MaxFileSizeBytes(), s.cfg.Storage.CDNURL, s.isAdminToken)
		s.storageHandler.SetCDNProvider(newStorageCDNProvider(s.cfg.Storage.CDN, s.logger))
		s.applyTenantQuotaDependenciesToStorageHandler()
		s.storageCDNPurgeAllRL = auth.NewRateLimiter(storageCDNPurgeAllRateLimit, storageCDNPurgeAllRateLimitWindow)
	}

	// Admin login rate limiter (always created, independent of auth service).
	adminRateLimit := s.cfg.Admin.LoginRateLimit
	if adminRateLimit <= 0 {
		adminRateLimit = 20
	}
	s.adminRL = auth.NewRateLimiter(adminRateLimit, time.Minute)

	apiLimit := 100
	apiWindow := time.Minute
	if parsedLimit, parsedWindow, err := config.ParseRateLimitSpec(s.cfg.RateLimit.API); err != nil {
		logger.Warn("invalid rate_limit.api; using default", "value", s.cfg.RateLimit.API, "error", err)
	} else {
		apiLimit, apiWindow = parsedLimit, parsedWindow
	}
	s.apiRL = auth.NewRateLimiter(apiLimit, apiWindow)
	s.apiRateLimit = apiLimit

	apiAnonLimit := 30
	apiAnonWindow := time.Minute
	if parsedLimit, parsedWindow, err := config.ParseRateLimitSpec(s.cfg.RateLimit.APIAnonymous); err != nil {
		logger.Warn("invalid rate_limit.api_anonymous; using default", "value", s.cfg.RateLimit.APIAnonymous, "error", err)
	} else {
		apiAnonLimit, apiAnonWindow = parsedLimit, parsedWindow
	}
	s.apiAnonRL = auth.NewRateLimiter(apiAnonLimit, apiAnonWindow)
	s.apiAnonRateLimit = apiAnonLimit

	assistantLimit := 20
	assistantWindow := time.Minute
	if parsedLimit, parsedWindow, err := config.ParseRateLimitSpec(s.cfg.DashboardAI.RateLimit); err != nil {
		logger.Warn("invalid dashboard_ai.rate_limit; using default", "value", s.cfg.DashboardAI.RateLimit, "error", err)
	} else {
		assistantLimit, assistantWindow = parsedLimit, parsedWindow
	}
	s.assistantRL = auth.NewRateLimiter(assistantLimit, assistantWindow)
	s.assistantRateLimit = assistantLimit
}
