// Package cli Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/cli/start_services_backup.go.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/backup"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/allyourbase/ayb/internal/server"
)

// wireBackupServices initializes and starts the backup service if enabled, configuring S3 storage, scheduling, and optionally Point-in-Time Recovery infrastructure.
func wireBackupServices(ctx context.Context, srv *server.Server, cfg *config.Config, pool *postgres.Pool, state *shutdownState, logger *slog.Logger) {
	if !cfg.Backup.Enabled || pool == nil {
		return
	}

	bkCfg := backup.Config{
		Enabled:        true,
		Bucket:         cfg.Backup.Bucket,
		Region:         cfg.Backup.Region,
		Prefix:         cfg.Backup.Prefix,
		Schedule:       cfg.Backup.Schedule,
		RetentionCount: cfg.Backup.RetentionCount,
		RetentionDays:  cfg.Backup.RetentionDays,
		Encryption:     cfg.Backup.Encryption,
		Endpoint:       cfg.Backup.Endpoint,
		AccessKey:      cfg.Backup.AccessKey,
		SecretKey:      cfg.Backup.SecretKey,
		PITR: backup.PITRConfig{
			Enabled:                  cfg.Backup.PITR.Enabled,
			ArchiveBucket:            cfg.Backup.PITR.ArchiveBucket,
			ArchivePrefix:            cfg.Backup.PITR.ArchivePrefix,
			WALRetentionDays:         cfg.Backup.PITR.WALRetentionDays,
			BaseBackupRetentionDays:  cfg.Backup.PITR.BaseBackupRetentionDays,
			ComplianceSnapshotMonths: cfg.Backup.PITR.ComplianceSnapshotMonths,
			RetentionSchedule:        cfg.Backup.PITR.RetentionSchedule,
			EnvironmentClass:         cfg.Backup.PITR.EnvironmentClass,
			KMSKeyID:                 cfg.Backup.PITR.KMSKeyID,
			RPOMinutes:               cfg.Backup.PITR.RPOMinutes,
			StorageBudgetBytes:       cfg.Backup.PITR.StorageBudgetBytes,
			ShadowMode:               cfg.Backup.PITR.ShadowMode,
			BaseBackupSchedule:       cfg.Backup.PITR.BaseBackupSchedule,
			VerifySchedule:           cfg.Backup.PITR.VerifySchedule,
		},
	}

	endpoint := cfg.Backup.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("s3.%s.amazonaws.com", cfg.Backup.Region)
	}
	bkStore, storeErr := backup.NewS3Store(ctx, backup.S3Config{
		Endpoint:  endpoint,
		Bucket:    cfg.Backup.Bucket,
		Region:    cfg.Backup.Region,
		AccessKey: cfg.Backup.AccessKey,
		SecretKey: cfg.Backup.SecretKey,
		UseSSL:    cfg.Backup.UseSSL,
	})
	if storeErr != nil {
		logger.Error("failed to initialise backup S3 store", "error", storeErr)
		return
	}

	dbName := extractDBName(cfg.Database.URL)
	bkRepo := backup.NewRepository(pool.DB())
	bkDumper := &backup.DumpRunner{}
	bkNotify := backup.NewLogNotifier(logger)

	bkEngine := backup.NewEngine(bkCfg, bkStore, bkRepo, bkDumper, bkNotify, logger, dbName, cfg.Database.URL)
	bkRetention := backup.NewRetentionJob(bkCfg, bkStore, bkRepo, bkNotify, logger)

	adminSvc := backup.NewAdminService(bkEngine, bkRepo)
	srv.SetBackupService(adminSvc)

	sched, schedErr := backup.NewScheduler(bkEngine, bkRetention, cfg.Backup.Schedule, logger)
	if schedErr != nil {
		logger.Error("failed to create backup scheduler", "error", schedErr)
	} else {
		sched.Start(ctx)
		state.backupSched = sched
	}

	if cfg.Backup.PITR.Enabled {
		wirePITRServices(ctx, pitrWireDependencies{
			server:         srv,
			config:         cfg,
			pool:           pool,
			state:          state,
			backupConfig:   bkCfg,
			backupRepo:     bkRepo,
			backupNotifier: bkNotify,
			endpoint:       endpoint,
			databaseName:   dbName,
			logger:         logger,
		})
	}

	logger.Info("backup service enabled",
		"bucket", cfg.Backup.Bucket,
		"schedule", cfg.Backup.Schedule,
		"retention_count", cfg.Backup.RetentionCount,
		"retention_days", cfg.Backup.RetentionDays,
	)
}

type pitrWireDependencies struct {
	server         *server.Server
	config         *config.Config
	pool           *postgres.Pool
	state          *shutdownState
	backupConfig   backup.Config
	backupRepo     backup.Repo
	backupNotifier backup.Notifier
	endpoint       string
	databaseName   string
	logger         *slog.Logger
}

type pitrSchedulerContext struct {
	ctx          context.Context
	state        *shutdownState
	logger       *slog.Logger
	projectID    string
	databaseName string
	config       backup.PITRConfig
}

// wirePITRServices sets up Point-in-Time Recovery infrastructure and associated schedulers.
func wirePITRServices(ctx context.Context, deps pitrWireDependencies) {
	pitrStore, storeErr := newPITRArchiveStore(ctx, deps.config, deps.endpoint)
	if storeErr != nil {
		deps.logger.Error("failed to initialise PITR S3 store", "error", storeErr)
		return
	}

	manifestRepo := backup.NewPgManifestRepo(deps.pool.DB())
	walSegmentRepo := backup.NewPgWALSegmentRepo(deps.pool.DB())
	manifestWriter := backup.NewManifestWriter(pitrStore, manifestRepo, walSegmentRepo, deps.backupConfig.PITR)
	walLagChecker := backup.NewWALLagChecker(walSegmentRepo, deps.backupNotifier, deps.backupConfig.PITR.RPOMinutes, deps.logger)
	scheduleChecker := backup.NewScheduleChecker(deps.backupRepo, deps.backupNotifier, deps.backupConfig.PITR.BaseBackupSchedule, deps.logger)
	budgetChecker := backup.NewStorageBudgetChecker(deps.backupRepo, walSegmentRepo, deps.backupNotifier, deps.logger)

	schedulerCtx := pitrSchedulerContext{
		ctx:          ctx,
		state:        deps.state,
		logger:       deps.logger,
		projectID:    pitrProjectIDFromEnv(),
		databaseName: deps.databaseName,
		config:       deps.backupConfig.PITR,
	}
	startPhysicalBackupScheduler(
		schedulerCtx,
		backup.NewPhysicalEngine(
			deps.backupConfig.PITR,
			pitrStore,
			deps.backupRepo,
			backup.NewBaseBackupRunner(deps.config.Database.URL),
			deps.backupNotifier,
			schedulerCtx.projectID,
			schedulerCtx.databaseName,
			manifestWriter,
		),
	)

	integrityVerifier := backup.NewIntegrityVerifier(
		deps.backupRepo,
		manifestRepo,
		walSegmentRepo,
		pitrStore,
		backup.NewPgIntegrityReportRepo(deps.pool.DB()),
		deps.backupNotifier,
		deps.backupConfig.PITR.ArchivePrefix,
	)
	startIntegrityBackupScheduler(schedulerCtx, integrityVerifier, walLagChecker)

	restorePlanner := backup.NewRestorePlanner(deps.backupRepo, walSegmentRepo, manifestRepo)
	restoreJobRepo := backup.NewPgRestoreJobRepo(deps.pool.DB())
	deps.state.restoreOrchestrator = backup.NewRestoreOrchestrator(
		restorePlanner,
		restoreJobRepo,
		pitrStore,
		deps.backupNotifier,
		deps.backupConfig.PITR,
		deps.config.Database.URL,
		deps.backupConfig.PITR.ArchivePrefix,
		deps.logger,
	)
	deps.server.SetPITRService(
		backup.NewPITRAdminService(
			restorePlanner,
			deps.state.restoreOrchestrator,
			restoreJobRepo,
			audit.NewAuditLogger(deps.config.Audit, deps.pool.DB()),
		),
	)

	retentionJob := backup.NewPITRRetentionJob(
		deps.backupConfig.PITR,
		pitrStore,
		deps.backupRepo,
		walSegmentRepo,
		manifestRepo,
		deps.backupNotifier,
		deps.logger,
		deps.backupConfig.PITR.ArchivePrefix,
	)
	startPITRRetentionBackupScheduler(schedulerCtx, retentionJob, walLagChecker, budgetChecker, scheduleChecker)
	startFireDrillBackupScheduler(schedulerCtx, backup.NewFireDrillRunner(restorePlanner, deps.backupNotifier, deps.logger))
}

func newPITRArchiveStore(ctx context.Context, cfg *config.Config, endpoint string) (backup.Store, error) {
	return backup.NewS3Store(ctx, backup.S3Config{
		Endpoint:   endpoint,
		Bucket:     cfg.Backup.PITR.ArchiveBucket,
		Region:     cfg.Backup.Region,
		AccessKey:  cfg.Backup.AccessKey,
		SecretKey:  cfg.Backup.SecretKey,
		Encryption: cfg.Backup.Encryption,
		KMSKeyID:   cfg.Backup.PITR.KMSKeyID,
		UseSSL:     cfg.Backup.UseSSL,
	})
}

func pitrProjectIDFromEnv() string {
	projectID := os.Getenv("AYB_PROJECT_ID")
	if projectID == "" {
		return "default"
	}
	return projectID
}

func startPhysicalBackupScheduler(schedulerCtx pitrSchedulerContext, physicalEngine *backup.PhysicalEngine) {
	physicalSched, err := backup.NewPhysicalScheduler(physicalEngine, schedulerCtx.config, schedulerCtx.logger)
	if err != nil {
		schedulerCtx.logger.Error("failed to create physical backup scheduler", "error", err)
		return
	}
	physicalSched.Start(schedulerCtx.ctx)
	schedulerCtx.state.physicalBackupSched = physicalSched
}

// TODO: Document startIntegrityBackupScheduler.
func startIntegrityBackupScheduler(
	schedulerCtx pitrSchedulerContext,
	integrityVerifier *backup.IntegrityVerifier,
	walLagChecker *backup.WALLagChecker,
) {
	integritySched, err := backup.NewIntegrityScheduler(
		integrityVerifier,
		schedulerCtx.config,
		schedulerCtx.logger,
		schedulerCtx.projectID,
		schedulerCtx.databaseName,
		walLagChecker,
	)
	if err != nil {
		schedulerCtx.logger.Error("failed to create integrity scheduler", "error", err)
		return
	}
	integritySched.Start(schedulerCtx.ctx)
	schedulerCtx.state.integrityBackupSched = integritySched
}

// TODO: Document startPITRRetentionBackupScheduler.
func startPITRRetentionBackupScheduler(
	schedulerCtx pitrSchedulerContext,
	retentionJob *backup.PITRRetentionJob,
	walLagChecker *backup.WALLagChecker,
	budgetChecker *backup.StorageBudgetChecker,
	scheduleChecker *backup.ScheduleChecker,
) {
	retentionSched, err := backup.NewPITRRetentionScheduler(
		retentionJob,
		schedulerCtx.config,
		schedulerCtx.logger,
		schedulerCtx.projectID,
		schedulerCtx.databaseName,
		walLagChecker,
		budgetChecker,
		scheduleChecker,
	)
	if err != nil {
		schedulerCtx.logger.Error("failed to create PITR retention scheduler", "error", err)
		return
	}
	retentionSched.Start(schedulerCtx.ctx)
	schedulerCtx.state.pitrRetentionSched = retentionSched
}

// TODO: Document startFireDrillBackupScheduler.
func startFireDrillBackupScheduler(
	schedulerCtx pitrSchedulerContext,
	fireDrillRunner *backup.FireDrillRunner,
) {
	fireDrillSched, err := backup.NewFireDrillScheduler(
		fireDrillRunner,
		schedulerCtx.config,
		schedulerCtx.logger,
		schedulerCtx.projectID,
		schedulerCtx.databaseName,
	)
	if err != nil {
		schedulerCtx.logger.Error("failed to create fire drill scheduler", "error", err)
		return
	}
	fireDrillSched.Start(schedulerCtx.ctx)
	schedulerCtx.state.fireDrillSched = fireDrillSched
}

// handlePasswordResets handles SIGUSR1-triggered admin password resets.
