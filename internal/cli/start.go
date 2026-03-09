package cli

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/cli/ui"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/emailtemplates"
	"github.com/allyourbase/ayb/internal/fbmigrate"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/matview"
	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/pbmigrate"
	"github.com/allyourbase/ayb/internal/pgmanager"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/allyourbase/ayb/internal/sbmigrate"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/caddyserver/certmagic"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the AYB server",
	Long: `Start the Allyourbase server. If no database URL is configured,
AYB starts a managed PostgreSQL instance automatically.

With external database:
  ayb start --database-url postgresql://user:pass@localhost:5432/mydb

Migrate and start from PocketBase (single command):
  ayb start --from ./pb_data

Migrate and start from Supabase:
  ayb start --from postgres://db.xxx.supabase.co:5432/postgres`,
	RunE: runStart,
}

func init() {
	startCmd.Flags().String("database-url", "", "PostgreSQL connection URL")
	startCmd.Flags().Int("port", 0, "Server port (default 8090)")
	startCmd.Flags().String("host", "", "Server host (default 0.0.0.0)")
	startCmd.Flags().String("config", "", "Path to ayb.toml config file")
	startCmd.Flags().String("from", "", "Migrate from another platform and start (path to pb_data, or postgres:// URL)")
	startCmd.Flags().String("domain", "", "Domain for automatic HTTPS via Let's Encrypt (e.g. api.myapp.com)")
	startCmd.Flags().Bool("foreground", false, "Run in foreground (blocks terminal)")
	startCmd.Flags().MarkHidden("foreground") //nolint:errcheck
}

func runStart(cmd *cobra.Command, args []string) error {
	fg, _ := cmd.Flags().GetBool("foreground")
	fromValue, _ := cmd.Flags().GetString("from")

	// --from requires interactive output, force foreground.
	if fromValue != "" {
		fg = true
	}

	// Windows doesn't support background mode.
	if !fg && !detachSupported() {
		fmt.Fprintln(os.Stderr, "Background mode not supported on this platform, running in foreground.")
		fg = true
	}

	if fg {
		return runStartForeground(cmd, args)
	}
	return runStartDetached(cmd, args)
}

type oauthProviderModeConfigSetter interface {
	SetOAuthProviderModeConfig(auth.OAuthProviderModeConfig)
}

func applyOAuthProviderModeConfig(target oauthProviderModeConfigSetter, cfg *config.Config) {
	if target == nil || cfg == nil || !cfg.Auth.OAuthProviderMode.Enabled {
		return
	}
	target.SetOAuthProviderModeConfig(auth.OAuthProviderModeConfig{
		AccessTokenDuration:  time.Duration(cfg.Auth.OAuthProviderMode.AccessTokenDuration) * time.Second,
		RefreshTokenDuration: time.Duration(cfg.Auth.OAuthProviderMode.RefreshTokenDuration) * time.Second,
		AuthCodeDuration:     time.Duration(cfg.Auth.OAuthProviderMode.AuthCodeDuration) * time.Second,
	})
}

func runStartForeground(cmd *cobra.Command, args []string) error {
	// Collect CLI flag overrides.
	flags := make(map[string]string)
	if v, _ := cmd.Flags().GetString("database-url"); v != "" {
		flags["database-url"] = v
	}
	if v, _ := cmd.Flags().GetInt("port"); v != 0 {
		flags["port"] = fmt.Sprintf("%d", v)
	}
	if v, _ := cmd.Flags().GetString("host"); v != "" {
		flags["host"] = v
	}
	if v, _ := cmd.Flags().GetString("domain"); v != "" {
		flags["tls-domain"] = v
	}

	configPath, _ := cmd.Flags().GetString("config")

	// Load config (defaults → file → env → flags).
	cfg, err := config.Load(configPath, flags)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Auto-generate admin password if not set.
	generatedPassword := ""
	if cfg.Admin.Enabled && cfg.Admin.Password == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return fmt.Errorf("generating admin password: %w", err)
		}
		generatedPassword = hex.EncodeToString(b)
		cfg.Admin.Password = generatedPassword
	}

	// Register signal handlers EARLY — before any blocking work (G1).
	// If user runs `ayb stop` during PG download, we catch it and clean up.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	// Detect interactive terminal for pretty startup output.
	isTTY := colorEnabled()
	sp := newStartupProgress(os.Stderr, isTTY, isTTY)

	// Set up logger. In TTY mode, suppress INFO during startup
	// (pretty progress lines replace them). Level is restored after server starts.
	logger, logLevel, logPath, closeLog := newLogger(cfg.Logging.Level, cfg.Logging.Format)
	defer closeLog()
	if isTTY {
		logLevel.Set(slog.LevelWarn)
	}

	// Show startup header.
	sp.header(bannerVersion(buildVersion))

	// Early port check: fail fast before expensive startup work.
	if ln, err := net.Listen("tcp", cfg.Address()); err != nil {
		return portError(cfg.Server.Port, err)
	} else {
		ln.Close()
	}

	// Auto-generate config file if it doesn't exist.
	if configPath == "" {
		if _, err := os.Stat("ayb.toml"); os.IsNotExist(err) {
			if err := config.GenerateDefault("ayb.toml"); err != nil {
				logger.Warn("could not generate default ayb.toml", "error", err)
			} else {
				logger.Info("generated default ayb.toml")
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start managed PostgreSQL if no database URL is configured.
	var pgMgr *pgmanager.Manager
	if cfg.Database.URL == "" {
		// Check for early signal before expensive PG startup.
		select {
		case <-sigCh:
			return nil
		default:
		}

		sp.step("Starting managed PostgreSQL...")
		logger.Info("no database URL configured, starting managed PostgreSQL")
		pgMgr = pgmanager.New(pgmanager.Config{
			Port:    uint32(cfg.Database.EmbeddedPort),
			DataDir: cfg.Database.EmbeddedDataDir,
			Logger:  logger,
		})
		connURL, err := pgMgr.Start(ctx)
		if err != nil {
			sp.fail()
			return fmt.Errorf("starting managed postgres: %w", err)
		}
		cfg.Database.URL = connURL
		sp.done()
	}

	// Check for early signal before DB connect.
	select {
	case <-sigCh:
		if pgMgr != nil {
			_ = pgMgr.Stop()
		}
		return nil
	default:
	}

	// Connect to PostgreSQL.
	sp.step("Connecting to database...")
	pool, err := postgres.New(ctx, postgres.Config{
		URL:             cfg.Database.URL,
		MaxConns:        int32(cfg.Database.MaxConns),
		MinConns:        int32(cfg.Database.MinConns),
		HealthCheckSecs: cfg.Database.HealthCheckSecs,
	}, logger)
	if err != nil {
		sp.fail()
		if pgMgr != nil {
			_ = pgMgr.Stop()
		}
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()
	sp.done()

	// Run system migrations.
	migRunner := migrations.NewRunner(pool.DB(), logger)
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

	// Run --from migration if specified (after system migrations, before user migrations).
	fromValue, _ := cmd.Flags().GetString("from")
	if fromValue != "" {
		if err := runFromMigration(ctx, fromValue, cfg.Database.URL, logger); err != nil {
			if pgMgr != nil {
				if stopErr := pgMgr.Stop(); stopErr != nil {
					logger.Error("error stopping managed postgres", "error", stopErr)
				}
			}
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	// Apply user migrations if the directory exists.
	if cfg.Database.MigrationsDir != "" {
		if _, err := os.Stat(cfg.Database.MigrationsDir); err == nil {
			userRunner := migrations.NewUserRunner(pool.DB(), cfg.Database.MigrationsDir, logger)
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

	// Check for early signal before schema loading.
	select {
	case <-sigCh:
		if pgMgr != nil {
			_ = pgMgr.Stop()
		}
		return nil
	default:
	}

	// Initialize schema cache and start watcher.
	sp.step("Loading schema...")
	schemaCache := schema.NewCacheHolder(pool.DB(), logger)
	watcher := schema.NewWatcher(schemaCache, pool.DB(), cfg.Database.URL, logger)

	watcherCtx, watcherCancel := context.WithCancel(ctx)
	defer watcherCancel()

	watcherErrCh := make(chan error, 1)
	go func() {
		watcherErrCh <- watcher.Start(watcherCtx)
	}()

	// Wait for initial schema load before starting HTTP server.
	select {
	case err := <-watcherErrCh:
		sp.fail()
		return fmt.Errorf("schema watcher: %w", err)
	case <-schemaCache.Ready():
		sp.done()
		logger.Info("schema cache ready")
	}

	// Build mailer (shared between auth service and email template service).
	mailSvc := buildMailer(cfg, logger)

	// Conditionally create auth service.
	var authSvc *auth.Service
	var smsProvider sms.Provider // nil when SMS disabled; set on both authSvc and server
	if cfg.Auth.Enabled {
		authSvc = auth.NewService(
			pool.DB(),
			cfg.Auth.JWTSecret,
			time.Duration(cfg.Auth.TokenDuration)*time.Second,
			time.Duration(cfg.Auth.RefreshTokenDuration)*time.Second,
			cfg.Auth.MinPasswordLength,
			logger,
		)

		// Inject mailer into auth service.
		baseURL := cfg.PublicBaseURL() + "/api"
		authSvc.SetMailer(mailSvc, cfg.Email.FromName, baseURL)
		if cfg.Auth.MagicLinkEnabled {
			dur := time.Duration(cfg.Auth.MagicLinkDuration) * time.Second
			if dur <= 0 {
				dur = 10 * time.Minute
			}
			authSvc.SetMagicLinkDuration(dur)
			logger.Info("magic link auth enabled", "duration", dur)
		}
		if cfg.Auth.SMSEnabled {
			smsProvider = buildSMSProvider(cfg, logger)
			authSvc.SetSMSProvider(smsProvider)
			authSvc.SetSMSConfig(sms.Config{
				CodeLength:       cfg.Auth.SMSCodeLength,
				Expiry:           time.Duration(cfg.Auth.SMSCodeExpiry) * time.Second,
				MaxAttempts:      cfg.Auth.SMSMaxAttempts,
				DailyLimit:       cfg.Auth.SMSDailyLimit,
				AllowedCountries: cfg.Auth.SMSAllowedCountries,
				TestPhoneNumbers: cfg.Auth.SMSTestPhoneNumbers,
			})
			logger.Info("SMS OTP auth enabled", "provider", cfg.Auth.SMSProvider)
		}
		applyOAuthProviderModeConfig(authSvc, cfg)
		logger.Info("auth enabled", "email_backend", cfg.Email.Backend)
	}

	// Conditionally create storage service.
	var storageSvc *storage.Service
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
				return fmt.Errorf("initializing S3 storage backend: %w", err)
			}
			storageBackend = s3b
			logger.Info("storage enabled", "backend", "s3", "endpoint", cfg.Storage.S3Endpoint, "bucket", cfg.Storage.S3Bucket)
		default:
			lb, err := storage.NewLocalBackend(cfg.Storage.LocalPath)
			if err != nil {
				return fmt.Errorf("initializing local storage backend: %w", err)
			}
			storageBackend = lb
			logger.Info("storage enabled", "backend", "local", "path", cfg.Storage.LocalPath)
		}
		signKey := cfg.Auth.JWTSecret
		if signKey == "" {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				return fmt.Errorf("generating storage sign key: %w", err)
			}
			signKey = hex.EncodeToString(b)
			logger.Info("generated random storage sign key (signed URLs will not survive restarts)")
		}
		storageSvc = storage.NewService(pool.DB(), storageBackend, signKey, logger)
	}

	// Create and start HTTP server.
	sp.step("Starting server...")
	srv := server.New(cfg, logger, schemaCache, pool.DB(), authSvc, storageSvc)

	// Wire SMS provider into server for the transactional messaging API.
	if smsProvider != nil {
		srv.SetSMSProvider(cfg.Auth.SMSProvider, smsProvider, cfg.Auth.SMSAllowedCountries)
	}

	// Wire matview admin service (requires pool for registry table access).
	if pool != nil {
		mvStore := matview.NewStore(pool.DB())
		mvSvc := matview.NewService(mvStore)
		srv.SetMatviewAdmin(matview.NewAdmin(mvStore, mvSvc))
	}

	// Wire email template service (requires pool for custom override storage).
	if pool != nil {
		etStore := emailtemplates.NewStore(pool.DB())
		etSvc := emailtemplates.NewService(etStore, emailtemplates.DefaultBuiltins())
		etSvc.SetLogger(logger)
		etSvc.SetMailer(mailSvc)
		srv.SetEmailTemplateService(etSvc)
		if authSvc != nil {
			authSvc.SetEmailTemplateService(etSvc)
		}
		logger.Info("email template service enabled")
	}

	// Wire job queue service if enabled.
	if cfg.Jobs.Enabled && pool != nil {
		jobStore := jobs.NewStore(pool.DB())
		jobCfg := jobs.ServiceConfig{
			WorkerConcurrency: cfg.Jobs.WorkerConcurrency,
			PollInterval:      time.Duration(cfg.Jobs.PollIntervalMs) * time.Millisecond,
			LeaseDuration:     time.Duration(cfg.Jobs.LeaseDurationS) * time.Second,
			SchedulerEnabled:  cfg.Jobs.SchedulerEnabled,
			SchedulerTick:     time.Duration(cfg.Jobs.SchedulerTickS) * time.Second,
			ShutdownTimeout:   time.Duration(cfg.Server.ShutdownTimeout) * time.Second,
			WorkerID:          fmt.Sprintf("ayb-%d", os.Getpid()),
		}
		jobSvc := jobs.NewService(jobStore, logger, jobCfg)
		jobs.RegisterBuiltinHandlers(jobSvc, pool.DB(), logger)
		srv.SetJobService(jobSvc)

		if err := jobSvc.RegisterDefaultSchedules(ctx); err != nil {
			logger.Error("failed to register default job schedules", "error", err)
		}

		jobSvc.Start(ctx)
		logger.Info("job queue enabled",
			"workers", cfg.Jobs.WorkerConcurrency,
			"poll_interval_ms", cfg.Jobs.PollIntervalMs,
			"scheduler_tick_s", cfg.Jobs.SchedulerTickS,
		)
	}

	// Password reset on SIGUSR1.
	usrCh := notifyUSR1()

	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		if cfg.Server.TLSEnabled {
			ln, err := buildTLSListener(ctx, cfg, logger)
			if err != nil {
				errCh <- err
				return
			}
			errCh <- srv.StartTLSWithReady(ln, ready)
		} else {
			errCh <- srv.StartWithReady(ready)
		}
	}()

	// Wait for port to be bound before printing banner.
	select {
	case <-ready:
		sp.done()

		// Restore configured log level for runtime (request logging, etc.).
		if isTTY {
			logLevel.Set(parseSlogLevel(cfg.Logging.Level))
		}

		// Write PID file so `ayb stop` and `ayb status` can find us.
		if pidPath, err := aybPIDPath(); err == nil {
			_ = os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n%d", os.Getpid(), cfg.Server.Port)), 0o644)
			defer os.Remove(pidPath)
		}

		// Save admin token so `ayb sql` can authenticate automatically.
		if cfg.Admin.Password != "" {
			if tokenPath, err := aybAdminTokenPath(); err == nil {
				_ = os.WriteFile(tokenPath, []byte(cfg.Admin.Password), 0o600)
				defer os.Remove(tokenPath)
			}
		}

		// In TTY mode the header was already printed; show just the body.
		// In non-TTY mode show the full banner (header + body).
		if isTTY {
			printBannerBodyTo(os.Stderr, cfg, pgMgr != nil, true, generatedPassword, logPath)
		} else {
			printBanner(cfg, pgMgr != nil, generatedPassword, logPath)
		}

		// Handle SIGUSR1 for password reset in background.
		go func() {
			for range usrCh {
				newPw, err := srv.ResetAdminPassword()
				if err != nil {
					logger.Error("password reset failed", "error", err)
					continue
				}
				// Write result file so the CLI command can read it.
				if resultPath, err := aybResetResultPath(); err == nil {
					_ = os.WriteFile(resultPath, []byte(newPw), 0o600)
				}
				fmt.Fprintf(os.Stderr, "\n  Admin password reset: %s\n\n", newPw)
			}
		}()
	case err := <-errCh:
		sp.fail()
		if pgMgr != nil {
			if stopErr := pgMgr.Stop(); stopErr != nil {
				logger.Error("error stopping managed postgres", "error", stopErr)
			}
		}
		return portError(cfg.Server.Port, err)
	}

	select {
	case err := <-errCh:
		if pgMgr != nil {
			if stopErr := pgMgr.Stop(); stopErr != nil {
				logger.Error("error stopping managed postgres", "error", stopErr)
			}
		}
		return err
	case sig := <-sigCh:
		logger.Info("received signal, shutting down", "signal", sig)
		fmt.Fprintf(os.Stderr, "\n  Shutting down... (press Ctrl-C again to force)\n")
		signal.Stop(sigCh) // Second Ctrl-C triggers Go default (immediate exit).

		watcherCancel()
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("shutdown error", "error", err)
		}
		if pgMgr != nil {
			if stopErr := pgMgr.Stop(); stopErr != nil {
				logger.Error("error stopping managed postgres", "error", stopErr)
			}
		}
		return nil
	}
}

// buildChildArgs constructs args for the re-exec'd child process.
// Uses os.Args directly to avoid cobra Flags().Visit() bugs (#1019, #1315).

// runStartDetached re-execs `ayb start --foreground` in a detached session,
// polls for readiness, prints the banner, and exits. Like pg_ctl start.
func runStartDetached(cmd *cobra.Command, _ []string) error {
	// --- 1. Already running? ---
	if pid, port, err := readAYBPID(); err == nil {
		proc, findErr := os.FindProcess(pid)
		if findErr == nil && proc.Signal(syscall.Signal(0)) == nil {
			// Process alive. Check health.
			client := &http.Client{Timeout: 2 * time.Second}
			healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
			if resp, hErr := client.Get(healthURL); hErr == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					fmt.Fprintf(os.Stderr, "AYB server is already running (PID %d, port %d).\n", pid, port)
					fmt.Fprintf(os.Stderr, "Stop with: ayb stop\n")
					return nil
				}
			}
			// Process alive but health fails — still starting up (G7).
			return waitForExistingServer(port)
		}
		// Stale PID file.
		cleanupServerFiles()
	}

	// --- 2. Load config (for port, banner info) ---
	configPath, _ := cmd.Flags().GetString("config")
	flags := make(map[string]string)
	if v, _ := cmd.Flags().GetString("database-url"); v != "" {
		flags["database-url"] = v
	}
	if v, _ := cmd.Flags().GetInt("port"); v != 0 {
		flags["port"] = fmt.Sprintf("%d", v)
	}
	if v, _ := cmd.Flags().GetString("host"); v != "" {
		flags["host"] = v
	}

	cfg, err := config.Load(configPath, flags)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// --- 3. Early port check ---
	if ln, err := net.Listen("tcp", cfg.Address()); err != nil {
		return portError(cfg.Server.Port, err)
	} else {
		ln.Close()
	}

	// --- 4. Detect first run (G6) ---
	firstRun := isFirstRun()
	timeout := 60 * time.Second
	if firstRun {
		timeout = 300 * time.Second
	}

	// --- 5. Build child command (G2, G3) ---
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("resolving executable symlinks: %w", err)
	}

	childArgs := buildChildArgs()
	child := exec.Command(exePath, childArgs...)
	child.Dir, _ = os.Getwd()
	child.Env = os.Environ()

	// --- 6. Redirect child output to log file (G9: must be *os.File) ---
	logPath := logFilePath()
	if logPath != "" {
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("opening log file: %w", err)
		}
		defer logFile.Close()
		child.Stdout = logFile
		child.Stderr = logFile
	}

	// --- 7. Detach (G8: setDetachAttrs is no-op on Windows) ---
	setDetachAttrs(child)

	// --- 8. Start ---
	isTTY := colorEnabled()
	sp := newStartupProgress(os.Stderr, isTTY, isTTY)
	sp.header(bannerVersion(buildVersion))

	if firstRun {
		sp.step("Downloading PostgreSQL and starting server (first run)...")
	} else {
		sp.step("Starting server...")
	}

	if err := child.Start(); err != nil {
		sp.fail()
		return fmt.Errorf("starting server process: %w", err)
	}

	// Detect early child death (G10).
	childDone := make(chan struct{})
	go func() {
		child.Wait()
		close(childDone)
	}()

	// --- 9. Poll for readiness (G4: check health AND admin-token file) ---
	port := cfg.Server.Port
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	httpClient := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	needAdminToken := cfg.Admin.Enabled && cfg.Admin.Password == ""
	tokenPath, _ := aybAdminTokenPath()

	for {
		select {
		case <-childDone:
			sp.fail()
			return fmt.Errorf("server exited during startup (check %s)", logPath)
		case <-ticker.C:
			if time.Now().After(deadline) {
				sp.fail()
				_ = child.Process.Signal(syscall.SIGTERM)
				return fmt.Errorf("server did not become ready within %s (check %s)", timeout, logPath)
			}
			resp, err := httpClient.Get(healthURL)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				continue
			}
			// Health OK. Also wait for admin-token file if needed (G4).
			if needAdminToken {
				if _, err := os.Stat(tokenPath); err != nil {
					continue // token not written yet
				}
			}
			// Ready!
			sp.done()
			goto ready
		}
	}

ready:
	// --- 10. Read admin password ---
	generatedPassword := ""
	if needAdminToken {
		if data, err := os.ReadFile(tokenPath); err == nil {
			generatedPassword = strings.TrimSpace(string(data))
		}
	}

	// --- 11. Print banner ---
	embeddedPG := cfg.Database.URL == ""
	if isTTY {
		printBannerBodyTo(os.Stderr, cfg, embeddedPG, true, generatedPassword, logPath)
	} else {
		printBanner(cfg, embeddedPG, generatedPassword, logPath)
	}

	fmt.Fprintf(os.Stderr, "  %s\n\n", dim("Stop with: ayb stop", isTTY))

	return nil
}

// waitForExistingServer polls an already-running server until it becomes healthy (G7).
func waitForExistingServer(port int) error {
	isTTY := colorEnabled()
	sp := newStartupProgress(os.Stderr, isTTY, isTTY)
	sp.step("Waiting for server to become ready...")

	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(60 * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		resp, err := client.Get(healthURL)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			sp.done()
			fmt.Fprintf(os.Stderr, "AYB server is running (port %d).\n", port)
			return nil
		}
	}
	sp.fail()
	return fmt.Errorf("existing server (port %d) did not become ready within 60s", port)
}

// aybPIDPath returns the path to the AYB server PID file (~/.ayb/ayb.pid).
func aybPIDPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ayb", "ayb.pid"), nil
}

// aybAdminTokenPath returns the path to the saved admin token (~/.ayb/admin-token).
func aybAdminTokenPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ayb", "admin-token"), nil
}

// aybResetResultPath returns the path for the password reset result file (~/.ayb/.pw_reset_result).
func aybResetResultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ayb", ".pw_reset_result"), nil
}

// readAYBPID reads the PID and port from the AYB PID file.
// Returns pid, port, error. Port may be 0 if the file uses the old format.
func readAYBPID() (int, int, error) {
	pidPath, err := aybPIDPath()
	if err != nil {
		return 0, 0, err
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, 0, err
	}
	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(lines) == 0 || lines[0] == "" {
		return 0, 0, fmt.Errorf("empty pid file")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("parsing pid: %w", err)
	}
	var port int
	if len(lines) > 1 && strings.TrimSpace(lines[1]) != "" {
		port, err = strconv.Atoi(strings.TrimSpace(lines[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("parsing port: %w", err)
		}
	}
	return pid, port, nil
}

func buildMailer(cfg *config.Config, logger *slog.Logger) mailer.Mailer {
	switch cfg.Email.Backend {
	case "smtp":
		port := cfg.Email.SMTP.Port
		if port == 0 {
			port = 587
		}
		return mailer.NewSMTPMailer(mailer.SMTPConfig{
			Host:       cfg.Email.SMTP.Host,
			Port:       port,
			Username:   cfg.Email.SMTP.Username,
			Password:   cfg.Email.SMTP.Password,
			From:       cfg.Email.From,
			FromName:   cfg.Email.FromName,
			TLS:        cfg.Email.SMTP.TLS,
			AuthMethod: cfg.Email.SMTP.AuthMethod,
		})
	case "webhook":
		timeout := time.Duration(cfg.Email.Webhook.Timeout) * time.Second
		if timeout == 0 {
			timeout = 10 * time.Second
		}
		return mailer.NewWebhookMailer(mailer.WebhookConfig{
			URL:     cfg.Email.Webhook.URL,
			Secret:  cfg.Email.Webhook.Secret,
			Timeout: timeout,
		})
	default:
		return mailer.NewLogMailer(logger)
	}
}

func buildSMSProvider(cfg *config.Config, logger *slog.Logger) sms.Provider {
	switch cfg.Auth.SMSProvider {
	case "twilio":
		return sms.NewTwilioProvider(cfg.Auth.TwilioSID, cfg.Auth.TwilioToken, cfg.Auth.TwilioFrom, "")
	case "plivo":
		return sms.NewPlivoProvider(cfg.Auth.PlivoAuthID, cfg.Auth.PlivoAuthToken, cfg.Auth.PlivoFrom, "")
	case "telnyx":
		return sms.NewTelnyxProvider(cfg.Auth.TelnyxAPIKey, cfg.Auth.TelnyxFrom, "")
	case "msg91":
		return sms.NewMSG91Provider(cfg.Auth.MSG91AuthKey, cfg.Auth.MSG91TemplateID, "")
	case "sns":
		publisher, err := newSNSPublisher(cfg.Auth.AWSRegion)
		if err != nil {
			logger.Error("failed to create AWS SNS client, falling back to log provider", "error", err)
			return sms.NewLogProvider(logger)
		}
		return sms.NewSNSProvider(publisher)
	case "vonage":
		return sms.NewVonageProvider(cfg.Auth.VonageAPIKey, cfg.Auth.VonageAPISecret, cfg.Auth.VonageFrom, "")
	case "webhook":
		return sms.NewWebhookProvider(cfg.Auth.SMSWebhookURL, cfg.Auth.SMSWebhookSecret)
	default:
		return sms.NewLogProvider(logger)
	}
}

// logFilePath returns the path to today's log file (~/.ayb/logs/ayb-YYYYMMDD.log).
// It creates the logs directory if needed. Returns "" on any error.
func logFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".ayb", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	return filepath.Join(dir, fmt.Sprintf("ayb-%s.log", time.Now().Format("20060102")))
}

// cleanOldLogs removes log files older than 7 days.
func cleanOldLogs() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".ayb", "logs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -7)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// multiHandler fans out log records to multiple handlers.
type multiHandler struct {
	handlers []slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			if err := handler.Handle(ctx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}

// newLogger creates a logger that writes to stderr and optionally to a log file.
// The log file receives all levels (DEBUG+) while stderr uses the configured level.
// Returns the logger, the stderr level var (for runtime adjustment), the log file
// path (empty if file logging failed), and an optional file closer.
func newLogger(level, format string) (*slog.Logger, *slog.LevelVar, string, func()) {
	var lvlVar slog.LevelVar
	lvlVar.Set(parseSlogLevel(level))

	opts := &slog.HandlerOptions{Level: &lvlVar}

	var stderrHandler slog.Handler
	if format == "text" {
		stderrHandler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		stderrHandler = slog.NewJSONHandler(os.Stderr, opts)
	}

	// Try to open a log file for detailed output.
	logPath := logFilePath()
	if logPath == "" {
		return slog.New(stderrHandler), &lvlVar, "", func() {}
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return slog.New(stderrHandler), &lvlVar, "", func() {}
	}

	fileOpts := &slog.HandlerOptions{Level: slog.LevelDebug}
	fileHandler := slog.NewJSONHandler(f, fileOpts)

	handler := &multiHandler{handlers: []slog.Handler{stderrHandler, fileHandler}}

	// Clean old logs in the background.
	go cleanOldLogs()

	return slog.New(handler), &lvlVar, logPath, func() { f.Close() }
}

func parseSlogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// startupProgress provides human-readable startup steps for interactive terminals.
// In TTY mode it shows animated spinners; in non-TTY mode all methods are no-ops.
type startupProgress struct {
	w        io.Writer
	spinner  *ui.StepSpinner
	active   bool
	useColor bool
}

func newStartupProgress(w io.Writer, active bool, useColor bool) *startupProgress {
	return &startupProgress{
		w:        w,
		spinner:  ui.NewStepSpinner(w, !active),
		active:   active,
		useColor: useColor,
	}
}

// portInUse returns true if the given port is already bound on the local machine.
func portInUse(port int) bool {
	if port <= 0 {
		return false
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return true
	}
	ln.Close()
	return false
}

// healthCheckURL returns the URL to poll for health during background startup.
// TLS always listens on port 443; plain HTTP uses the configured port.
func healthCheckURL(cfg *config.Config) string {
	if cfg.Server.TLSEnabled {
		return "https://127.0.0.1:443/health"
	}
	port := cfg.Server.Port
	if port == 0 {
		port = 8090
	}
	return fmt.Sprintf("http://127.0.0.1:%d/health", port)
}

// buildChildArgs returns the arguments to pass when re-exec'ing as a background
// child. It takes os.Args[1:], strips any existing --foreground flags, and
// appends --foreground so the child runs in the foreground.
func buildChildArgs() []string {
	args := make([]string, 0, len(os.Args))
	for _, a := range os.Args[1:] {
		if a == "--foreground" || strings.HasPrefix(a, "--foreground=") {
			continue
		}
		args = append(args, a)
	}
	return append(args, "--foreground")
}

// cleanupServerFiles removes the PID and admin token files left by a previous run.
func cleanupServerFiles() {
	if pidPath, err := aybPIDPath(); err == nil {
		os.Remove(pidPath) //nolint:errcheck
	}
	if tokenPath, err := aybAdminTokenPath(); err == nil {
		os.Remove(tokenPath) //nolint:errcheck
	}
}

// isFirstRun returns true when AYB has never downloaded its embedded PostgreSQL.
func isFirstRun() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return true
	}
	_, err = os.Stat(filepath.Join(home, ".ayb", "pg", "postgres.txz"))
	return os.IsNotExist(err)
}

func (sp *startupProgress) header(version string) {
	if !sp.active {
		return
	}
	fmt.Fprintf(sp.w, "\n  %s %s\n\n",
		ui.BrandEmoji,
		boldCyan(fmt.Sprintf("Allyourbase v%s", version), sp.useColor))
}

func (sp *startupProgress) step(msg string) {
	if !sp.active {
		return
	}
	sp.spinner.Start(msg)
}

func (sp *startupProgress) done() {
	if !sp.active {
		return
	}
	sp.spinner.Done()
}

func (sp *startupProgress) fail() {
	if !sp.active {
		return
	}
	sp.spinner.Fail()
}

// buildTLSListener uses certmagic to obtain a Let's Encrypt certificate and
// returns a TLS net.Listener on port 443. It also starts an HTTP-01 challenge
// responder + HTTP→HTTPS redirect on port 80 in a background goroutine.
func buildTLSListener(ctx context.Context, cfg *config.Config, logger *slog.Logger) (net.Listener, error) {
	certDir := cfg.Server.TLSCertDir
	if certDir == "" {
		home, _ := os.UserHomeDir()
		certDir = filepath.Join(home, ".ayb", "certs")
	}

	if cfg.Server.TLSEmail != "" {
		certmagic.DefaultACME.Email = cfg.Server.TLSEmail
	}

	magic := certmagic.NewDefault()
	magic.Storage = &certmagic.FileStorage{Path: certDir}

	logger.Info("obtaining TLS certificate", "domain", cfg.Server.TLSDomain)
	if err := magic.ManageSync(ctx, []string{cfg.Server.TLSDomain}); err != nil {
		return nil, fmt.Errorf("obtaining TLS certificate for %s: %w", cfg.Server.TLSDomain, err)
	}

	tlsCfg := magic.TLSConfig()

	// Port 80: handle HTTP-01 ACME challenges and redirect everything else to https.
	go func() {
		domain := cfg.Server.TLSDomain
		handler := certmagic.DefaultACME.HTTPChallengeHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := "https://" + domain + r.RequestURI
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		}))
		srv := &http.Server{
			Addr:              ":80",
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		if err := srv.ListenAndServe(); err != nil {
			logger.Warn("HTTP redirect listener error", "error", err)
		}
	}()

	ln, err := tls.Listen("tcp", fmt.Sprintf("%s:443", cfg.Server.Host), tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("TLS listen on :443: %w", err)
	}
	return ln, nil
}

// portError wraps common listen errors with actionable suggestions.
func portError(port int, err error) error {
	if strings.Contains(err.Error(), "address already in use") {
		return fmt.Errorf("%s", ui.FormatError(
			fmt.Sprintf("port %d is already in use", port),
			fmt.Sprintf("ayb start --port %d   # use a different port", port+1),
			"ayb stop                # stop the running server",
		))
	}
	return err
}

// printBanner writes a human-readable startup summary to stderr.
// This is separate from structured logging and designed for first-time users.
func printBanner(cfg *config.Config, embeddedPG bool, generatedPassword, logPath string) {
	printBannerTo(os.Stderr, cfg, embeddedPG, colorEnabled(), generatedPassword, logPath)
}

// printBannerTo writes the full banner (header + body) to w. Extracted for testing.
func printBannerTo(w io.Writer, cfg *config.Config, embeddedPG bool, useColor bool, generatedPassword, logPath string) {
	ver := bannerVersion(buildVersion)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s\n", ui.BrandEmoji,
		boldCyan(fmt.Sprintf("Allyourbase v%s", ver), useColor))
	printBannerBodyTo(w, cfg, embeddedPG, useColor, generatedPassword, logPath)
}

// printBannerBodyTo writes everything after the header (URLs, hints, etc.).
// Used by TTY mode where the header is shown early during startup progress.
func printBannerBodyTo(w io.Writer, cfg *config.Config, embeddedPG bool, useColor bool, generatedPassword, logPath string) {
	apiURL := cfg.PublicBaseURL() + "/api"

	dbMode := "external"
	if embeddedPG {
		dbMode = "managed"
	}

	// Pad labels before colorizing so ANSI codes don't break alignment.
	padLabel := func(label string, width int) string {
		return bold(fmt.Sprintf("%-*s", width, label), useColor)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s\n", padLabel("API:", 10), cyan(apiURL, useColor))
	if cfg.Admin.Enabled {
		adminURL := cfg.PublicBaseURL() + cfg.Admin.Path
		fmt.Fprintf(w, "  %s %s\n", padLabel("Admin:", 10), cyan(adminURL, useColor))
	}
	fmt.Fprintf(w, "  %s %s\n", padLabel("Database:", 10), dbMode)
	if cfg.Auth.MinPasswordLength > 0 && cfg.Auth.MinPasswordLength < 8 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %s\n", yellow(fmt.Sprintf(
			"WARNING: min_password_length is %d (recommended: 8+). Not suitable for production.",
			cfg.Auth.MinPasswordLength), useColor))
	}
	if cfg.Admin.Enabled && generatedPassword != "" {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %s  %s\n", bold("Admin password:", useColor), boldGreen(generatedPassword, useColor))
		fmt.Fprintf(w, "  %s\n", dim("To reset: ayb admin reset-password", useColor))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s\n", padLabel("Docs:", 10), dim("https://allyourbase.io/guide/quickstart", useColor))
	if logPath != "" {
		fmt.Fprintf(w, "  %s %s\n", padLabel("Logs:", 10), dim(logPath, useColor))
	}

	// Print next-step hints for new users (no leading whitespace for easy copy-paste).
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", dim("Try:", useColor))
	fmt.Fprintf(w, "%s\n", green(`ayb sql "CREATE TABLE posts (id serial PRIMARY KEY, title text)"`, useColor))
	fmt.Fprintf(w, "%s\n", green("ayb schema", useColor))
	fmt.Fprintln(w)

	// Demo hints — show new users how to run the bundled demo apps.
	fmt.Fprintf(w, "  %s\n", dim("Demos:", useColor))
	fmt.Fprintf(w, "%s  %s\n", green("ayb demo kanban    ", useColor), dim("# Trello-lite kanban board  (port 5173)", useColor))
	fmt.Fprintf(w, "%s  %s\n", green("ayb demo live-polls", useColor), dim("# real-time polling app     (port 5175)", useColor))
	fmt.Fprintln(w)
}

// redactURL removes userinfo (username:password) from a URL for safe logging.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "***"
	}
	if u.User != nil {
		u.User = nil
		// Re-insert redacted marker at string level to avoid URL-encoding of *.
		s := u.String()
		return strings.Replace(s, "://", "://***@", 1)
	}
	return u.String()
}

// bannerVersion extracts a clean semver string for the startup banner.
// Release builds (e.g. "v0.1.0") → "0.1.0".
// Dev builds (e.g. "v0.1.0-43-ge534c04-dirty") → "0.1.0-dev".
// Full version is always available via `ayb version`.
func bannerVersion(raw string) string {
	v := strings.TrimPrefix(raw, "v")
	// A bare semver tag (e.g. "0.1.0") has no hyphen after the patch number,
	// or has a pre-release label like "0.1.0-beta.1". Git-describe appends
	// "-<N>-g<hash>" when commits exist past the tag. Detect that pattern.
	parts := strings.SplitN(v, "-", 2)
	if len(parts) == 1 {
		return v // clean tag, e.g. "0.1.0"
	}
	// If the first segment after the hyphen is a number, it's a git-describe
	// commit count (e.g. "0.1.0-43-ge534c04"), not a semver pre-release.
	if len(parts[1]) > 0 && parts[1][0] >= '0' && parts[1][0] <= '9' {
		return parts[0] + "-dev"
	}
	return v // pre-release tag like "0.1.0-beta.1"
}

// runFromMigration handles the --from flag on ayb start.
// It auto-detects the source type, runs pre-flight analysis, migrates data,
// and prints a validation summary.
func runFromMigration(ctx context.Context, from string, databaseURL string, logger *slog.Logger) error {
	sourceType := migrate.DetectSource(from)

	switch sourceType {
	case migrate.SourcePocketBase:
		return runFromPocketBase(ctx, from, databaseURL, logger)
	case migrate.SourceSupabase:
		return runFromSupabase(ctx, from, databaseURL, logger)
	case migrate.SourceFirebase:
		return runFromFirebase(ctx, from, databaseURL, logger)
	case migrate.SourcePostgres:
		logger.Info("detected generic PostgreSQL source", "url", redactURL(from))
		return fmt.Errorf("generic PostgreSQL --from migration is not yet implemented")
	default:
		return fmt.Errorf("could not detect migration source type from %q (expected: path to pb_data, postgres:// URL, or firebase:// URL)", from)
	}
}

// runFromPocketBase runs PocketBase migration as part of ayb start --from.
func runFromPocketBase(ctx context.Context, sourcePath string, databaseURL string, logger *slog.Logger) error {
	logger.Info("detected PocketBase source", "path", sourcePath)

	// Pre-flight analysis
	report, err := pbmigrate.Analyze(sourcePath)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	report.PrintReport(os.Stderr)

	// Run migration with CLI progress reporter
	progress := migrate.NewCLIReporter(os.Stderr)

	fmt.Fprintf(os.Stderr, "  Migrating %s -> AYB...\n\n", report.SourceType)

	migrator, err := pbmigrate.NewMigrator(pbmigrate.MigrationOptions{
		SourcePath:  sourcePath,
		DatabaseURL: databaseURL,
		Progress:    progress,
	})
	if err != nil {
		return err
	}
	defer migrator.Close()

	stats, err := migrator.Migrate(ctx)
	if err != nil {
		return err
	}

	// Validation summary
	summary := pbmigrate.BuildValidationSummary(report, stats)
	summary.PrintSummary(os.Stderr)

	return nil
}

// runFromSupabase runs Supabase migration as part of ayb start --from.
func runFromSupabase(ctx context.Context, sourceURL string, databaseURL string, logger *slog.Logger) error {
	logger.Info("detected Supabase source", "url", redactURL(sourceURL))

	progress := migrate.NewCLIReporter(os.Stderr)

	migrator, err := sbmigrate.NewMigrator(sbmigrate.MigrationOptions{
		SourceURL: sourceURL,
		TargetURL: databaseURL,
		Progress:  progress,
	})
	if err != nil {
		return err
	}
	defer migrator.Close()

	// Pre-flight analysis.
	report, err := migrator.Analyze(ctx)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	report.PrintReport(os.Stderr)

	fmt.Fprintf(os.Stderr, "  Migrating %s -> AYB...\n\n", report.SourceType)

	stats, err := migrator.Migrate(ctx)
	if err != nil {
		return err
	}

	// Validation summary.
	summaryReport := normalizeSupabaseSummaryReport(report, false, false, false, false, "")
	summary := sbmigrate.BuildValidationSummary(summaryReport, stats)
	summary.PrintSummary(os.Stderr)

	return nil
}

// runFromFirebase runs Firebase migration as part of ayb start --from.
func runFromFirebase(ctx context.Context, from string, databaseURL string, logger *slog.Logger) error {
	logger.Info("detected Firebase source", "from", from)

	progress := migrate.NewCLIReporter(os.Stderr)

	// Determine if --from is a .json auth export or a firebase:// URL.
	opts := fbmigrate.MigrationOptions{
		DatabaseURL: databaseURL,
		Progress:    progress,
	}
	if strings.HasSuffix(from, ".json") {
		opts.AuthExportPath = from
	} else {
		// firebase:// URL — not yet supported for auto-detection of export paths.
		return fmt.Errorf("firebase --from requires a path to a .json auth export file")
	}

	migrator, err := fbmigrate.NewMigrator(opts)
	if err != nil {
		return err
	}
	defer migrator.Close()

	// Pre-flight analysis.
	report, err := migrator.Analyze(ctx)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	report.PrintReport(os.Stderr)

	fmt.Fprintf(os.Stderr, "  Migrating %s -> AYB...\n\n", report.SourceType)

	stats, err := migrator.Migrate(ctx)
	if err != nil {
		return err
	}

	// Validation summary.
	summary := fbmigrate.BuildValidationSummary(report, stats)
	summary.PrintSummary(os.Stderr)

	return nil
}
