// Package cli Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar19_03_go_code_quality_refactoring/allyourbase_dev/internal/cli/start_services_init.go.
package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/branching"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/pgmanager"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

type initDatabaseManagedPostgres interface {
	Start(ctx context.Context) (string, error)
	Stop() error
}

type initDatabasePool interface {
	DB() *pgxpool.Pool
	Close()
}

type initDatabaseMigrationRunner interface {
	Bootstrap(ctx context.Context) error
	Run(ctx context.Context) (int, error)
}

var (
	newInitDatabaseManagedPostgres = func(cfg pgmanager.Config) initDatabaseManagedPostgres {
		return pgmanager.New(cfg)
	}
	newInitDatabasePool = func(ctx context.Context, cfg postgres.Config, logger *slog.Logger) (initDatabasePool, error) {
		return postgres.New(ctx, cfg, logger)
	}
	newInitDatabaseMigrationRunner = func(pool *pgxpool.Pool, logger *slog.Logger) initDatabaseMigrationRunner {
		return migrations.NewRunner(pool, logger)
	}
	runFromMigrationForInitDatabase = runFromMigration
)

func stopInitDatabaseManagedPostgres(pg initDatabaseManagedPostgres, logger *slog.Logger) {
	if pg == nil {
		return
	}
	if err := pg.Stop(); err != nil {
		logger.Error("error stopping managed postgres", "error", err)
	}
}

func initDatabaseSignalReceived(sigCh <-chan os.Signal) bool {
	select {
	case <-sigCh:
		return true
	default:
		return false
	}
}

// TODO: Document startInitDatabaseManagedPostgres.
func startInitDatabaseManagedPostgres(
	ctx context.Context,
	cfg *config.Config,
	logger *slog.Logger,
	sp *startupProgress,
) (initDatabaseManagedPostgres, *pgmanager.Manager, error) {
	sp.step("Starting managed PostgreSQL...")
	logger.Info("no database URL configured, starting managed PostgreSQL")
	applyDeprecatedManagedPGConfig(cfg, logger)

	managedPG := newInitDatabaseManagedPostgres(pgmanager.Config{
		Port:                   uint32(cfg.ManagedPG.Port),
		DataDir:                cfg.ManagedPG.DataDir,
		BinaryURL:              cfg.ManagedPG.BinaryURL,
		PGVersion:              cfg.ManagedPG.PGVersion,
		Extensions:             cfg.ManagedPG.EffectiveExtensions(),
		SharedPreloadLibraries: cfg.ManagedPG.EffectiveSharedPreloadLibraries(),
		Logger:                 logger,
	})
	var pgMgr *pgmanager.Manager
	if realPGMgr, ok := managedPG.(*pgmanager.Manager); ok {
		pgMgr = realPGMgr
	}

	connURL, err := managedPG.Start(ctx)
	if err != nil {
		sp.fail()
		return nil, nil, fmt.Errorf("starting managed postgres: %w", err)
	}

	cfg.Database.URL = connURL
	sp.done()
	return managedPG, pgMgr, nil
}

func applyDeprecatedManagedPGConfig(cfg *config.Config, logger *slog.Logger) {
	defaultCfg := config.Default()
	managedPGPortLooksDefault := cfg.ManagedPG.Port == 0 || cfg.ManagedPG.Port == defaultCfg.ManagedPG.Port
	deprecatedEmbeddedPortWasConfigured := cfg.Database.EmbeddedPort != 0 &&
		cfg.Database.EmbeddedPort != defaultCfg.Database.EmbeddedPort

	if managedPGPortLooksDefault && deprecatedEmbeddedPortWasConfigured {
		logger.Warn("database.embedded_port is deprecated, use managed_pg.port instead")
		cfg.ManagedPG.Port = cfg.Database.EmbeddedPort
	}

	if cfg.ManagedPG.DataDir == "" && cfg.Database.EmbeddedDataDir != "" {
		logger.Warn("database.embedded_data_dir is deprecated, use managed_pg.data_dir instead")
		cfg.ManagedPG.DataDir = cfg.Database.EmbeddedDataDir
	}
}

func applyInitDatabaseBranchURL(cfg *config.Config, branchName string, logger *slog.Logger) error {
	if branchName == "" {
		return nil
	}

	branchDB := branching.BranchDBName(branchName)
	branchURL, err := branching.ReplaceDatabaseInURL(cfg.Database.URL, branchDB)
	if err != nil {
		return fmt.Errorf("resolving branch database URL: %w", err)
	}
	logger.Info("using branch database", "branch", branchName, "database", branchDB)
	cfg.Database.URL = branchURL
	return nil
}

// TODO: Document connectInitDatabasePool.
func connectInitDatabasePool(
	ctx context.Context,
	cfg *config.Config,
	logger *slog.Logger,
	sp *startupProgress,
) (initDatabasePool, *postgres.Pool, error) {
	sp.step("Connecting to database...")
	initPool, err := newInitDatabasePool(ctx, postgres.Config{
		URL:             cfg.Database.URL,
		MaxConns:        int32(cfg.Database.MaxConns),
		MinConns:        int32(cfg.Database.MinConns),
		HealthCheckSecs: cfg.Database.HealthCheckSecs,
	}, logger)
	if err != nil {
		sp.fail()
		return nil, nil, fmt.Errorf("connecting to database: %w", err)
	}

	var pool *postgres.Pool
	if realPool, ok := initPool.(*postgres.Pool); ok {
		pool = realPool
	}
	sp.done()
	return initPool, pool, nil
}

// TODO: Document runInitDatabaseMigrations.
func runInitDatabaseMigrations(
	ctx context.Context,
	initPool initDatabasePool,
	cfg *config.Config,
	fromValue string,
	logger *slog.Logger,
) error {
	migRunner := newInitDatabaseMigrationRunner(initPool.DB(), logger)
	if err := migRunner.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrapping migrations: %w", err)
	}
	applied, err := migRunner.Run(ctx)
	if err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	if applied > 0 {
		logger.Info("applied system migrations", "count", applied)
	}

	if fromValue != "" {
		if err := runFromMigrationForInitDatabase(ctx, fromValue, cfg.Database.URL, logger); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	if cfg.Database.MigrationsDir != "" {
		if _, err := os.Stat(cfg.Database.MigrationsDir); err == nil {
			userRunner := migrations.NewUserRunner(initPool.DB(), cfg.Database.MigrationsDir, logger)
			if err := userRunner.Bootstrap(ctx); err != nil {
				return fmt.Errorf("bootstrapping user migrations: %w", err)
			}
			userApplied, err := userRunner.Up(ctx)
			if err != nil {
				return fmt.Errorf("running user migrations: %w", err)
			}
			if userApplied > 0 {
				logger.Info("applied user migrations", "count", userApplied)
			}
		}
	}

	return nil
}

// TODO: Document startInitDatabaseSchemaWatcher.
func startInitDatabaseSchemaWatcher(
	ctx context.Context,
	initPool initDatabasePool,
	databaseURL string,
	logger *slog.Logger,
	sp *startupProgress,
) (*schema.CacheHolder, context.CancelFunc, error) {
	sp.step("Loading schema...")
	schemaCache := schema.NewCacheHolder(initPool.DB(), logger)
	watcher := schema.NewWatcher(schemaCache, initPool.DB(), databaseURL, logger)

	watcherCtx, watcherCancel := context.WithCancel(ctx)
	watcherErrCh := make(chan error, 1)
	go func() {
		watcherErrCh <- watcher.Start(watcherCtx)
	}()

	select {
	case err := <-watcherErrCh:
		sp.fail()
		watcherCancel()
		return nil, nil, fmt.Errorf("schema watcher: %w", err)
	case <-schemaCache.Ready():
		sp.done()
		logger.Info("schema cache ready")
	}

	return schemaCache, watcherCancel, nil
}

// initDatabase initializes the database connection by starting managed PostgreSQL if unconfigured, connecting, running migrations, and loading the schema cache. It returns the pool, manager, schema cache holder, and schema watcher cancel function.
func initDatabase(
	ctx context.Context,
	cfg *config.Config,
	fromValue string,
	branchName string,
	sigCh <-chan os.Signal,
	logger *slog.Logger,
	sp *startupProgress,
) (*postgres.Pool, *pgmanager.Manager, *schema.CacheHolder, context.CancelFunc, error) {
	releaseManagedPG := false
	releasePool := false

	var pool *postgres.Pool
	var initPool initDatabasePool
	var pgMgr *pgmanager.Manager
	var managedPG initDatabaseManagedPostgres

	defer func() {
		if releasePool && initPool != nil {
			initPool.Close()
			pool = nil
		}
		if !releaseManagedPG || managedPG == nil {
			return
		}
		stopInitDatabaseManagedPostgres(managedPG, logger)
		pgMgr = nil
	}()

	if cfg.Database.URL == "" {
		if initDatabaseSignalReceived(sigCh) {
			return nil, nil, nil, nil, nil
		}

		managedPGResult, pgMgrResult, err := startInitDatabaseManagedPostgres(ctx, cfg, logger, sp)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		managedPG = managedPGResult
		pgMgr = pgMgrResult
		releaseManagedPG = true
	}

	if err := applyInitDatabaseBranchURL(cfg, branchName, logger); err != nil {
		return nil, nil, nil, nil, err
	}

	if initDatabaseSignalReceived(sigCh) {
		return nil, nil, nil, nil, nil
	}

	initPoolResult, poolResult, err := connectInitDatabasePool(ctx, cfg, logger, sp)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	initPool = initPoolResult
	pool = poolResult
	releasePool = true

	if err := runInitDatabaseMigrations(ctx, initPool, cfg, fromValue, logger); err != nil {
		return nil, nil, nil, nil, err
	}

	if initDatabaseSignalReceived(sigCh) {
		return nil, nil, nil, nil, nil
	}

	schemaCache, watcherCancel, err := startInitDatabaseSchemaWatcher(ctx, initPool, cfg.Database.URL, logger, sp)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	releasePool = false
	releaseManagedPG = false
	return pool, pgMgr, schemaCache, watcherCancel, nil
}

// initCoreServices creates the mailer, auth service, and storage service.
func initCoreServices(ctx context.Context, cfg *config.Config, pool *postgres.Pool, logger *slog.Logger) (*coreServices, error) {
	core := &coreServices{}

	// Build mailer (shared between auth service and email template service).
	core.mailSvc = buildMailer(cfg, logger)

	// Conditionally create auth service.
	if cfg.Auth.Enabled {
		core.authSvc = auth.NewService(
			pool.DB(),
			cfg.Auth.JWTSecret,
			time.Duration(cfg.Auth.TokenDuration)*time.Second,
			time.Duration(cfg.Auth.RefreshTokenDuration)*time.Second,
			cfg.Auth.MinPasswordLength,
			logger,
		)

		// Inject mailer into auth service.
		baseURL := cfg.PublicBaseURL() + "/api"
		core.authSvc.SetMailer(core.mailSvc, cfg.Email.FromName, baseURL)
		if cfg.Auth.MagicLinkEnabled {
			dur := time.Duration(cfg.Auth.MagicLinkDuration) * time.Second
			if dur <= 0 {
				dur = 10 * time.Minute
			}
			core.authSvc.SetMagicLinkDuration(dur)
			logger.Info("magic link auth enabled", "duration", dur)
		}
		if cfg.Auth.SMSEnabled {
			core.smsProvider = buildSMSProvider(cfg, logger)
			core.authSvc.SetSMSProvider(core.smsProvider)
			core.authSvc.SetSMSConfig(sms.Config{
				CodeLength:       cfg.Auth.SMSCodeLength,
				Expiry:           time.Duration(cfg.Auth.SMSCodeExpiry) * time.Second,
				MaxAttempts:      cfg.Auth.SMSMaxAttempts,
				DailyLimit:       cfg.Auth.SMSDailyLimit,
				AllowedCountries: cfg.Auth.SMSAllowedCountries,
				TestPhoneNumbers: cfg.Auth.SMSTestPhoneNumbers,
			})
			logger.Info("SMS OTP auth enabled", "provider", cfg.Auth.SMSProvider)
		}
		// Always resolve and set the TOTP encryption key when auth is enabled.
		encryptionKey, err := resolveTOTPEncryptionKey(cfg.Auth)
		if err != nil {
			if cfg.Auth.TOTPEnabled {
				return nil, fmt.Errorf("configuring TOTP encryption key: %w", err)
			}
			logger.Warn("could not derive TOTP encryption key (TOTP disabled)", "error", err)
		} else {
			if err := core.authSvc.SetEncryptionKey(encryptionKey); err != nil {
				return nil, fmt.Errorf("setting TOTP encryption key: %w", err)
			}
		}
		applyOAuthProviderModeConfig(core.authSvc, cfg)
		logger.Info("auth enabled", "email_backend", cfg.Email.Backend)
	}

	// Conditionally create storage service.
	if cfg.Storage.Enabled {
		var storageBackend storage.Backend
		switch cfg.Storage.Backend {
		case "s3":
			s3b, err := storage.NewS3Backend(ctx, storage.S3Config{
				Endpoint:  cfg.Storage.S3Endpoint,
				Bucket:    cfg.Storage.S3Bucket,
				Region:    cfg.Storage.S3Region,
				AccessKey: cfg.Storage.S3AccessKey,
				SecretKey: cfg.Storage.S3SecretKey,
				UseSSL:    cfg.Storage.S3UseSSL,
			})
			if err != nil {
				return nil, fmt.Errorf("initializing S3 storage backend: %w", err)
			}
			storageBackend = s3b
			logger.Info("storage enabled", "backend", "s3", "endpoint", cfg.Storage.S3Endpoint, "bucket", cfg.Storage.S3Bucket)
		default:
			lb, err := storage.NewLocalBackend(cfg.Storage.LocalPath)
			if err != nil {
				return nil, fmt.Errorf("initializing local storage backend: %w", err)
			}
			storageBackend = lb
			logger.Info("storage enabled", "backend", "local", "path", cfg.Storage.LocalPath)
		}
		signKey := cfg.Auth.JWTSecret
		if signKey == "" {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				return nil, fmt.Errorf("generating storage sign key: %w", err)
			}
			signKey = hex.EncodeToString(b)
			logger.Info("generated random storage sign key (signed URLs will not survive restarts)")
		}
		core.storageSvc = storage.NewService(pool.DB(), storageBackend, signKey, logger, cfg.Storage.DefaultQuotaBytes())
	}

	return core, nil
}

// wireServices creates the HTTP server, wires all services into it, and returns
// the server plus resources that need lifecycle management.
