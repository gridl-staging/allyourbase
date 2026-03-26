// Package cli Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar19_03_go_code_quality_refactoring/allyourbase_dev/internal/cli/start_services_wire.go.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/api"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/branching"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/emailtemplates"
	"github.com/allyourbase/ayb/internal/extensions"
	"github.com/allyourbase/ayb/internal/fdw"
	aybgraphql "github.com/allyourbase/ayb/internal/graphql"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/matview"
	"github.com/allyourbase/ayb/internal/notifications"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/allyourbase/ayb/internal/push"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/vault"
	"github.com/caddyserver/certmagic"
	"github.com/jackc/pgx/v5/stdlib"
)

type wireServicesArgs struct {
	ctx         context.Context
	cfg         *config.Config
	pool        *postgres.Pool
	core        *coreServices
	schemaCache *schema.CacheHolder
	logger      *slog.Logger
}

type wireServicesPhaseState struct {
	state      *shutdownState
	srv        *server.Server
	vaultStore *vault.Store
	edgeSvc    *edgefunc.Service
	billingSvc billing.BillingService
	aiLogStore *ai.PgLogStore
	jobStore   *jobs.Store
}

// wireServices initializes all application services including vault, billing, edge functions, AI, email templates, job queue, and push notifications, returning a shutdown state that coordinates graceful shutdown.
func wireServices(
	ctx context.Context,
	cfg *config.Config,
	pool *postgres.Pool,
	core *coreServices,
	schemaCache *schema.CacheHolder,
	logger *slog.Logger,
) (*shutdownState, error) {
	args := wireServicesArgs{
		ctx:         ctx,
		cfg:         cfg,
		pool:        pool,
		core:        core,
		schemaCache: schemaCache,
		logger:      logger,
	}
	phase := &wireServicesPhaseState{state: &shutdownState{}}

	if err := wireServerAndVaultBootstrap(args, phase); err != nil {
		return nil, err
	}
	if err := wireEdgeAIAndEmailServices(args, phase); err != nil {
		return nil, err
	}
	if err := wireJobsPushTenantBackupAndBranch(args, phase); err != nil {
		return nil, err
	}
	wireGraphQLAndStartJobs(args, phase)

	return phase.state, nil
}

// TODO: Document wireServerAndVaultBootstrap.
func wireServerAndVaultBootstrap(args wireServicesArgs, phase *wireServicesPhaseState) error {
	var vaultEngine *vault.Vault
	if args.pool != nil {
		masterKey, err := vault.ResolveMasterKey(args.cfg.Vault.MasterKey)
		if err != nil {
			return fmt.Errorf("resolving vault master key: %w", err)
		}
		vaultEngine, err = vault.New(masterKey)
		if err != nil {
			return fmt.Errorf("initializing vault: %w", err)
		}
		phase.vaultStore = vault.NewStore(args.pool.DB(), vaultEngine)
	}

	fieldEncryptor := api.NewFieldEncryptorFromConfig(vaultEngine, args.cfg.EncryptedColumns)
	srv := server.NewWithFieldEncryptor(args.cfg, args.logger, args.schemaCache, args.pool.DB(), args.core.authSvc, args.core.storageSvc, fieldEncryptor)
	phase.srv = srv
	phase.state.srv = srv

	phase.billingSvc = buildBillingService(args.cfg, args.pool.DB(), args.logger)
	srv.SetBillingService(phase.billingSvc)
	srv.SetSupportService(buildSupportService(args.cfg, args.pool.DB()))

	if phase.vaultStore != nil {
		srv.SetVaultStore(phase.vaultStore)
		args.logger.Info("vault secret store enabled")
		fdwSvc := fdw.NewService(args.pool.DB(), phase.vaultStore)
		srv.SetFDWService(fdwSvc)
		args.logger.Info("fdw management service enabled")
		if args.core.authSvc != nil {
			args.core.authSvc.SetProviderTokenStore(auth.NewProviderTokenStore(args.pool.DB(), vaultEngine, args.logger))
		}
	}

	return nil
}

func wireEdgeAIAndEmailServices(args wireServicesArgs, phase *wireServicesPhaseState) error {
	edgePool := wireEdgeRuntimeAndCoreAdminServices(args, phase)
	wireAIServices(args, phase, edgePool)
	wireEmailAndTLSServices(args, phase, edgePool)
	return nil
}

// TODO: Document wireEdgeRuntimeAndCoreAdminServices.
func wireEdgeRuntimeAndCoreAdminServices(args wireServicesArgs, phase *wireServicesPhaseState) *edgefunc.Pool {
	edgeRuntimeCfg := buildEdgeFuncRuntimeConfig(args.cfg)
	edgePoolOpts := []edgefunc.PoolOption{
		edgefunc.WithPoolHTTPClient(edgefunc.NewSSRFSafeClient(edgeRuntimeCfg.FetchDomainAllowlist)),
		edgefunc.WithPoolMemoryLimitMB(edgeRuntimeCfg.MemoryLimitMB),
		edgefunc.WithPoolMaxConcurrentInvocations(edgeRuntimeCfg.MaxConcurrentInvocations),
		edgefunc.WithPoolCodeCacheSize(edgeRuntimeCfg.CodeCacheSize),
		edgefunc.WithPoolSchemaCache(args.schemaCache.Get),
	}
	if args.core.authSvc != nil {
		edgePoolOpts = append(edgePoolOpts, edgefunc.WithPoolProviderTokenGetter(args.core.authSvc.GetProviderToken))
	}
	edgePool := edgefunc.NewPool(edgeRuntimeCfg.PoolSize, edgePoolOpts...)
	phase.state.edgePool = edgePool

	edgeStore := edgefunc.NewPostgresStore(args.pool.DB())
	edgeLogStore := edgefunc.NewPostgresLogStore(args.pool.DB())
	edgeQueryExecutor := edgefunc.NewPostgresQueryExecutor(args.pool.DB(), nil)
	edgeOpts := []edgefunc.ServiceOption{
		edgefunc.WithServiceQueryExecutor(edgeQueryExecutor),
		edgefunc.WithDefaultTimeout(edgeRuntimeCfg.DefaultTimeout),
	}
	if phase.vaultStore != nil {
		edgeOpts = append(edgeOpts, edgefunc.WithVaultProvider(phase.vaultStore))
	}
	phase.edgeSvc = edgefunc.NewService(edgeStore, edgePool, edgeLogStore, edgeOpts...)
	phase.srv.SetEdgeFuncService(phase.edgeSvc)
	args.logger.Info(
		"edge function service enabled",
		"pool_size", edgeRuntimeCfg.PoolSize,
		"max_concurrent_invocations", edgeRuntimeCfg.MaxConcurrentInvocations,
		"default_timeout_ms", edgeRuntimeCfg.DefaultTimeout.Milliseconds(),
		"max_request_body_bytes", edgeRuntimeCfg.MaxRequestBodyBytes,
		"memory_limit_mb", edgeRuntimeCfg.MemoryLimitMB,
		"code_cache_size", edgeRuntimeCfg.CodeCacheSize,
		"fetch_domain_allowlist_count", len(edgeRuntimeCfg.FetchDomainAllowlist),
	)

	if args.core.smsProvider != nil {
		phase.srv.SetSMSProvider(args.cfg.Auth.SMSProvider, args.core.smsProvider, args.cfg.Auth.SMSAllowedCountries)
	}

	if args.pool != nil {
		mvStore := matview.NewStore(args.pool.DB())
		mvSvc := matview.NewService(mvStore)
		phase.srv.SetMatviewAdmin(matview.NewAdmin(mvStore, mvSvc))

		phase.state.extDB = stdlib.OpenDBFromPool(args.pool.DB())
		extSvc := extensions.NewService(phase.state.extDB)
		phase.srv.SetExtService(extSvc)
		args.logger.Info("extension management service enabled")
	}

	return edgePool
}

// TODO: Document wireAIServices.
func wireAIServices(args wireServicesArgs, phase *wireServicesPhaseState, edgePool *edgefunc.Pool) {
	var aiReg *ai.Registry
	var assistantHistoryStore ai.AssistantHistoryStore
	if args.pool != nil {
		assistantHistoryStore = ai.NewPgAssistantHistoryStore(args.pool.DB())
	}

	if len(args.cfg.AI.Providers) > 0 && args.pool != nil {
		var vaultSecrets map[string]string
		if phase.vaultStore != nil {
			var vaultSecretsErr error
			vaultSecrets, vaultSecretsErr = phase.vaultStore.GetAllSecretsDecrypted(args.ctx)
			if vaultSecretsErr != nil {
				args.logger.Warn("could not read vault secrets for AI providers", "error", vaultSecretsErr)
			}
		}

		builtAIReg, aiErr := ai.BuildRegistry(args.cfg.AI, vaultSecrets)
		if aiErr != nil {
			args.logger.Warn("AI registry initialization failed, AI features disabled", "error", aiErr)
		} else {
			aiReg = builtAIReg
			phase.aiLogStore = ai.NewPgLogStore(args.pool.DB())
			aiPromptStore := ai.NewPgPromptStore(args.pool.DB())
			aiPromptCache := ai.NewPromptCache()

			phase.srv.SetAILogStore(phase.aiLogStore)
			phase.srv.SetPromptStore(aiPromptStore)

			aiTimeout := time.Duration(args.cfg.AI.TimeoutSecs) * time.Second
			if aiTimeout <= 0 {
				aiTimeout = 30 * time.Second
			}
			breakerTracker := ai.NewProviderHealthTracker(ai.BreakerConfig{
				FailureThreshold:    args.cfg.AI.Breaker.FailureThreshold,
				OpenDuration:        time.Duration(args.cfg.AI.Breaker.OpenSeconds) * time.Second,
				HalfOpenMaxRequests: args.cfg.AI.Breaker.HalfOpenProbeLimit,
			}, nil)
			for name := range args.cfg.AI.Providers {
				if provider, getErr := aiReg.Get(name); getErr == nil {
					retried := ai.NewRetryProvider(provider, args.cfg.AI.MaxRetries, aiTimeout)
					breakered := ai.NewBreakerProvider(retried, name, breakerTracker)
					aiReg.Register(name, ai.NewLoggingProvider(breakered, name, phase.aiLogStore))
				}
			}

			wireAIEdgeCallbacks(edgePool, aiReg, args.cfg.AI, aiPromptCache, aiPromptStore)
			wireAIEmbedding(phase.srv, aiReg, args.cfg.AI, args.logger)
			args.logger.Info("AI subsystem enabled", "providers", len(args.cfg.AI.Providers))
		}
	}

	wireDashboardAIAssistant(phase.srv, args.cfg, aiReg, args.schemaCache, assistantHistoryStore, args.logger)
}

// TODO: Document wireEmailAndTLSServices.
func wireEmailAndTLSServices(args wireServicesArgs, phase *wireServicesPhaseState, edgePool *edgefunc.Pool) {
	var etSvc *emailtemplates.Service
	if args.pool != nil {
		etStore := emailtemplates.NewStore(args.pool.DB())
		etSvc = emailtemplates.NewService(etStore, emailtemplates.DefaultBuiltins())
		etSvc.SetLogger(args.logger)
		etSvc.SetMailer(args.core.mailSvc)
		phase.srv.SetEmailTemplateService(etSvc)
		if args.core.authSvc != nil {
			args.core.authSvc.SetEmailTemplateService(etSvc)
		}
		args.logger.Info("email template service enabled")
	}

	phase.srv.SetMailer(args.core.mailSvc)
	emailRateLimit := args.cfg.Email.Policy.EffectiveSendRateLimit()
	emailRateWindow := time.Duration(args.cfg.Email.Policy.EffectiveSendRateWindow()) * time.Second
	emailRL := auth.NewRateLimiter(emailRateLimit, emailRateWindow)
	phase.srv.SetEmailRateLimiter(emailRL)
	wireEdgeEmailBridge(edgePool, args.core.mailSvc, args.cfg.Email, etSvc)

	if args.cfg.Server.TLSEnabled {
		var certmagicCache *certmagic.Cache
		phase.state.certmagicConfig, certmagicCache = buildCertmagicConfig(args.cfg, args.logger)
		phase.srv.SetCertManager(server.NewCertmagicCertManager(phase.state.certmagicConfig, certmagicCache))
	}
}

// TODO: Document wireJobsPushTenantBackupAndBranch.
func wireJobsPushTenantBackupAndBranch(args wireServicesArgs, phase *wireServicesPhaseState) error {
	if args.cfg.Jobs.Enabled && args.pool != nil {
		phase.jobStore = jobs.NewStore(args.pool.DB())
		jobCfg := jobs.ServiceConfig{
			WorkerConcurrency: args.cfg.Jobs.WorkerConcurrency,
			PollInterval:      time.Duration(args.cfg.Jobs.PollIntervalMs) * time.Millisecond,
			LeaseDuration:     time.Duration(args.cfg.Jobs.LeaseDurationS) * time.Second,
			SchedulerEnabled:  args.cfg.Jobs.SchedulerEnabled,
			SchedulerTick:     time.Duration(args.cfg.Jobs.SchedulerTickS) * time.Second,
			ShutdownTimeout:   time.Duration(args.cfg.Server.ShutdownTimeout) * time.Second,
			WorkerID:          fmt.Sprintf("ayb-%d", os.Getpid()),
		}
		phase.state.jobSvc = jobs.NewService(phase.jobStore, args.logger, jobCfg)
		jobs.RegisterBuiltinHandlers(phase.state.jobSvc, args.pool.DB(), args.logger)
		phase.srv.SetJobService(phase.state.jobSvc)

		wireJobDomainHandlers(args.ctx, phase.srv, phase.state.jobSvc, args.logger)

		if err := phase.state.jobSvc.RegisterDefaultSchedulesWithAuditRetention(args.ctx, args.cfg.Audit.RetentionDays, args.cfg.Logging.RequestLogRetentionDays); err != nil {
			args.logger.Error("failed to register default job schedules", "error", err)
		}
		if args.core.authSvc != nil {
			jobs.RegisterProviderTokenRefreshHandler(phase.state.jobSvc, args.core.authSvc)
			if err := jobs.RegisterProviderTokenRefreshSchedule(args.ctx, phase.state.jobSvc); err != nil {
				args.logger.Warn("failed to register provider token refresh schedule", "error", err)
			}
		}
		if phase.aiLogStore != nil {
			jobs.RegisterAIUsageAggregationHandler(phase.state.jobSvc, phase.aiLogStore)
			if err := jobs.RegisterAIUsageAggregationSchedule(args.ctx, phase.state.jobSvc); err != nil {
				args.logger.Warn("failed to register ai usage aggregation schedule", "error", err)
			}
		}
		wireBillingUsageSyncJobs(args.ctx, args.cfg, phase.state.jobSvc, phase.billingSvc, args.pool.DB(), args.logger)
	}

	if args.pool != nil {
		notifStore := notifications.NewStore(args.pool.DB())
		phase.srv.SetNotificationService(notifStore)
	}

	wireTenantServices(args.ctx, phase.srv, args.cfg, args.pool, phase.state, args.logger)

	if args.cfg.Push.Enabled && args.pool != nil {
		providers := buildPushProviders(args.cfg, args.logger)
		pushStore := push.NewStore(args.pool.DB())
		pushSvc := push.NewService(pushStore, providers, phase.state.jobSvc)
		pushSvc.SetLogger(args.logger)
		phase.srv.SetPushService(pushSvc)

		if phase.state.jobSvc != nil {
			phase.state.jobSvc.RegisterHandler(push.JobTypePushDelivery, push.PushDeliveryJobHandler(pushSvc))
			phase.state.jobSvc.RegisterHandler(push.JobTypePushTokenClean, push.PushTokenCleanupJobHandler(pushStore, 270))
			registerPushTokenCleanupSchedule(args.ctx, phase.jobStore, args.logger)
		}

		args.logger.Info("push notification service enabled", "providers", pushProviderNames(providers))
	}

	var edgeStorageSvc storageEventRegistrar
	if args.core.storageSvc != nil {
		edgeStorageSvc = args.core.storageSvc
	}
	wireEdgeTriggerRuntime(
		args.ctx,
		args.pool.DB(),
		args.cfg.Database.URL,
		edgeStorageSvc,
		newCronJobsSchedulerAdapter(phase.state.jobSvc),
		phase.edgeSvc,
		args.logger,
		func(db *edgefunc.DBTriggerService, cron *edgefunc.CronTriggerService, st *edgefunc.StorageTriggerService, inv edgefunc.FunctionInvoker) {
			phase.srv.SetTriggerServices(db, cron, st, inv)
		},
	)

	wireBackupServices(args.ctx, phase.srv, args.cfg, args.pool, phase.state, args.logger)

	if args.pool != nil {
		branchRepo := branching.NewPgRepo(args.pool.DB())
		branchMgr := branching.NewManager(args.pool.DB(), branchRepo, args.logger, branching.ManagerConfig{
			DefaultSourceURL: args.cfg.Database.URL,
		})
		phase.srv.SetBranchService(branchMgr)
		args.logger.Info("branch admin service enabled")
	}

	return nil
}

// TODO: Document wireGraphQLAndStartJobs.
func wireGraphQLAndStartJobs(args wireServicesArgs, phase *wireServicesPhaseState) {
	if args.cfg.GraphQL.Enabled && args.pool != nil {
		gqlHandler := aybgraphql.NewHandler(args.pool.DB(), args.schemaCache, args.logger)
		if args.core.authSvc != nil {
			gqlHandler.SetAuthValidator(args.core.authSvc)
		}
		if hub := phase.srv.RealtimeHub(); hub != nil {
			gqlHandler.SetHub(hub)
		}
		gqlHandler.SetLimits(args.cfg.GraphQL.MaxDepth, args.cfg.GraphQL.MaxComplexity)
		if checker := graphQLAdminCheckerForIntrospectionMode(args.cfg.GraphQL.Introspection, phase.srv.IsAdminToken); checker != nil {
			gqlHandler.SetAdminChecker(checker)
		}
		phase.srv.SetGraphQLHandler(gqlHandler)
		args.logger.Info("graphql endpoint enabled")
	}

	if phase.state.jobSvc != nil {
		phase.state.jobSvc.Start(args.ctx)
		args.logger.Info("job queue enabled",
			"workers", args.cfg.Jobs.WorkerConcurrency,
			"poll_interval_ms", args.cfg.Jobs.PollIntervalMs,
			"scheduler_tick_s", args.cfg.Jobs.SchedulerTickS,
		)
	}
}

func graphQLAdminCheckerForIntrospectionMode(mode string, defaultChecker func(*http.Request) bool) func(*http.Request) bool {
	switch mode {
	case "open":
		return nil
	case "disabled":
		return func(*http.Request) bool { return false }
	default:
		return defaultChecker
	}
}
