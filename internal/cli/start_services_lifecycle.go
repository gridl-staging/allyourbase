// Package cli Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar22_pm_02_ci_e2e_wiring/allyourbase_dev/internal/cli/start_services_lifecycle.go.
package cli

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/backup"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/pgmanager"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/caddyserver/certmagic"
	"github.com/jackc/pgx/v5/pgxpool"
)

// coreServices holds the auth, storage, mail, and SMS services created during startup.
type coreServices struct {
	authSvc     *auth.Service
	storageSvc  *storage.Service
	smsProvider sms.Provider
	mailSvc     mailer.Mailer
}

// shutdownState holds resources created during service wiring that require
// lifecycle management during graceful shutdown.
type shutdownState struct {
	srv                  *server.Server
	certmagicConfig      *certmagic.Config
	tlsRedirectSrv       *http.Server
	jobSvc               *jobs.Service
	edgePool             *edgefunc.Pool
	backupSched          *backup.Scheduler
	physicalBackupSched  *backup.PhysicalScheduler
	integrityBackupSched *backup.IntegrityScheduler
	fireDrillSched       *backup.FireDrillScheduler
	pitrRetentionSched   *backup.PITRRetentionScheduler
	restoreOrchestrator  *backup.RestoreOrchestrator
	tenantRateLimiter    *tenant.TenantRateLimiter
	tenantBreaker        *tenant.TenantBreakerTracker
	extDB                *sql.DB
}

type readyState struct {
	isTTY             bool
	generatedPassword string
	logPath           string
	logLevel          *slog.LevelVar
}

// shutdown stops schedulers, abandons active restore jobs, and closes the TLS redirect server.
func (s *shutdownState) shutdown(ctx context.Context, logger *slog.Logger) {
	if s.backupSched != nil {
		s.backupSched.Stop()
	}
	if s.physicalBackupSched != nil {
		s.physicalBackupSched.Stop()
	}
	if s.pitrRetentionSched != nil {
		s.pitrRetentionSched.Stop()
	}
	if s.integrityBackupSched != nil {
		s.integrityBackupSched.Stop()
	}
	if s.fireDrillSched != nil {
		s.fireDrillSched.Stop()
	}
	if s.restoreOrchestrator != nil {
		for _, jobID := range s.restoreOrchestrator.ActiveJobIDs() {
			if err := s.restoreOrchestrator.Abandon(ctx, jobID); err != nil {
				logger.Error("failed to abandon active restore job", "job_id", jobID, "error", err)
			}
		}
	}
	if s.tlsRedirectSrv != nil {
		_ = s.tlsRedirectSrv.Close()
	}
}

// cleanup releases resources whose deferred cleanup was previously in runStartForeground.
func (s *shutdownState) cleanup(pool *pgxpool.Pool, logger *slog.Logger) {
	if s.tenantBreaker != nil {
		if err := s.tenantBreaker.Snapshot(context.Background(), pool); err != nil {
			logger.Warn("failed to snapshot tenant breaker state on shutdown", "error", err)
		}
	}
	if s.tenantRateLimiter != nil {
		s.tenantRateLimiter.Stop()
	}
	if s.edgePool != nil {
		s.edgePool.Close()
	}
	if s.extDB != nil {
		s.extDB.Close()
	}
}

// TODO: Document runForegroundPreflight.
func runForegroundPreflight(cfg *config.Config, configPath, fromValue string, logger *slog.Logger) error {
	ln, err := net.Listen("tcp", cfg.Address())
	if err != nil {
		return portError(cfg.Server.Port, err)
	}
	ln.Close()

	if configPath == "" {
		if _, err := os.Stat("ayb.toml"); os.IsNotExist(err) {
			if err := config.GenerateDefault("ayb.toml"); err != nil {
				logger.Warn("could not generate default ayb.toml", "error", err)
			} else {
				logger.Info("generated default ayb.toml")
			}
		}
	}

	if fromValue == "" || migrate.DetectSource(fromValue) != migrate.SourcePocketBase {
		return nil
	}
	if _, err := os.Stat(fromValue); os.IsNotExist(err) {
		return fmt.Errorf("migration failed: source path %q does not exist", fromValue)
	}

	return nil
}

// TODO: Document waitForServerReady.
func waitForServerReady(
	ready <-chan struct{},
	errCh <-chan error,
	usrCh <-chan os.Signal,
	cfg *config.Config,
	pgMgr *pgmanager.Manager,
	srv *server.Server,
	logger *slog.Logger,
	sp *startupProgress,
	state readyState,
) (func(), error) {
	cleanup := func() {}

	select {
	case <-ready:
		sp.done()
		if state.isTTY {
			state.logLevel.Set(parseSlogLevel(cfg.Logging.Level))
		}

		baseURL := loopbackServerURL(cfg.Server.Port)
		cleanup = prepareServerReadyArtifacts(cfg, baseURL, logger)
		if state.isTTY {
			printBannerBodyTo(os.Stderr, cfg, pgMgr != nil, true, state.generatedPassword, state.logPath)
		} else {
			printBanner(cfg, pgMgr != nil, state.generatedPassword, state.logPath)
		}
		go handlePasswordResets(usrCh, srv, baseURL, logger)
		return cleanup, nil
	case err := <-errCh:
		sp.fail()
		stopManagedPostgres(pgMgr, logger)
		return cleanup, portError(cfg.Server.Port, err)
	}
}

// TODO: Document prepareServerReadyArtifacts.
func prepareServerReadyArtifacts(cfg *config.Config, baseURL string, logger *slog.Logger) func() {
	var cleanupFuncs []func()

	if pidPath, err := aybPIDPath(); err == nil {
		_ = os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n%d", os.Getpid(), cfg.Server.Port)), 0o644)
		cleanupFuncs = append(cleanupFuncs, func() {
			_ = os.Remove(pidPath)
		})
	}
	if cfg.Admin.Password != "" {
		if tokenPath, err := writeAdminTokenFile(baseURL, cfg.Admin.Password); err == nil {
			cleanupFuncs = append(cleanupFuncs, func() {
				_ = os.Remove(tokenPath)
			})
		} else {
			logger.Warn("failed to write admin token file", "error", err)
		}
	}

	return func() {
		for _, fn := range cleanupFuncs {
			fn()
		}
	}
}

func loopbackServerURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// TODO: Document writeAdminTokenFile.
func writeAdminTokenFile(baseURL, password string) (string, error) {
	trimmedPassword := strings.TrimSpace(password)
	if trimmedPassword == "" {
		return "", fmt.Errorf("admin password is empty")
	}
	token, err := adminLogin(baseURL, trimmedPassword)
	if err != nil {
		return "", fmt.Errorf("exchanging admin password for bearer token: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("admin auth response returned empty token")
	}
	tokenPath, err := aybAdminTokenPath()
	if err != nil {
		return "", fmt.Errorf("resolving admin token path: %w", err)
	}
	if err := os.WriteFile(tokenPath, []byte(token), 0o600); err != nil {
		return "", fmt.Errorf("writing admin token file: %w", err)
	}
	return tokenPath, nil
}

func writePasswordResetResultFile(password string) error {
	resultPath, err := aybResetResultPath()
	if err != nil {
		return fmt.Errorf("resolving password reset result path: %w", err)
	}
	if err := os.WriteFile(resultPath, []byte(password), 0o600); err != nil {
		return fmt.Errorf("writing password reset result file: %w", err)
	}
	return nil
}

// runGracefulShutdown waits for errors or termination signals and performs graceful shutdown by stopping all managed services and closing the server.
func runGracefulShutdown(
	ctx context.Context,
	errCh <-chan error,
	sigCh chan os.Signal,
	watcherCancel context.CancelFunc,
	state *shutdownState,
	pgMgr *pgmanager.Manager,
	logger *slog.Logger,
) error {
	select {
	case err := <-errCh:
		stopManagedPostgres(pgMgr, logger)
		return err
	case sig := <-sigCh:
		logger.Info("received signal, shutting down", "signal", sig)
		fmt.Fprintf(os.Stderr, "\n  Shutting down... (press Ctrl-C again to force)\n")
		signal.Stop(sigCh)
		watcherCancel()
		state.shutdown(ctx, logger)
		if err := state.srv.Shutdown(ctx); err != nil {
			logger.Error("shutdown error", "error", err)
		}
		stopManagedPostgres(pgMgr, logger)
		return nil
	}
}

func stopManagedPostgres(pgMgr *pgmanager.Manager, logger *slog.Logger) {
	if pgMgr == nil {
		return
	}
	if err := pgMgr.Stop(); err != nil {
		logger.Error("error stopping managed postgres", "error", err)
	}
}

// initDatabase starts managed PG if needed, connects to the database, runs
// system and user migrations, handles --from and --branch, and initializes

// TODO: Document handlePasswordResets.
func handlePasswordResets(usrCh <-chan os.Signal, srv *server.Server, baseURL string, logger *slog.Logger) {
	for range usrCh {
		newPw, err := srv.ResetAdminPassword()
		if err != nil {
			logger.Error("password reset failed", "error", err)
			continue
		}
		// Write result file so the CLI command can read it.
		if err := writePasswordResetResultFile(newPw); err != nil {
			logger.Warn("failed to write password reset result file", "error", err)
		}
		// Refresh admin-token file so CLI commands use a valid bearer token.
		if _, err := writeAdminTokenFile(baseURL, newPw); err != nil {
			logger.Warn("failed to refresh admin token file after password reset", "error", err)
		}
		fmt.Fprintf(os.Stderr, "\n  Admin password reset: %s\n\n", newPw)
	}
}

// --- Factory functions (moved from start.go) ---
