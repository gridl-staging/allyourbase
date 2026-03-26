// Package cli start.go implements the server startup command, routing to
// foreground or detached mode.
package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/jobs"
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

var registerBillingUsageSyncHandler = jobs.RegisterBillingUsageSyncHandler
var registerBillingUsageSyncSchedule = jobs.RegisterBillingUsageSyncSchedule

func init() {
	startCmd.Flags().String("database-url", "", "PostgreSQL connection URL")
	startCmd.Flags().Int("port", 0, "Server port (default 8090)")
	startCmd.Flags().String("host", "", "Server host (default 127.0.0.1)")
	startCmd.Flags().String("config", "", "Path to ayb.toml config file")
	startCmd.Flags().String("from", "", "Migrate from another platform and start (path to pb_data, or postgres:// URL)")
	startCmd.Flags().String("domain", "", "Domain for automatic HTTPS via Let's Encrypt (e.g. api.myapp.com)")
	startCmd.Flags().String("branch", "", "Start using a database branch (created via ayb branch create)")
	startCmd.Flags().Bool("foreground", false, "Run in foreground (blocks terminal)")
	startCmd.Flags().MarkHidden("foreground") //nolint:errcheck
}

// runStart is the entry point for the start command that determines whether
// to run the server in foreground or detached mode.
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

func ensureConfiguredAdminPassword(cfg *config.Config) (string, error) {
	if !cfg.Admin.Enabled || cfg.Admin.Password != "" {
		return "", nil
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating admin password: %w", err)
	}
	generatedPassword := hex.EncodeToString(b)
	cfg.Admin.Password = generatedPassword
	return generatedPassword, nil
}

// TODO: Document runStartForeground.
func runStartForeground(cmd *cobra.Command, args []string) error {
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
	fromValue, _ := cmd.Flags().GetString("from")
	branchName, _ := cmd.Flags().GetString("branch")

	cfg, err := config.Load(configPath, flags)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	generatedPassword, err := ensureConfiguredAdminPassword(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	isTTY := colorEnabled()
	sp := newStartupProgress(os.Stderr, isTTY, isTTY)
	logger, logLevel, logPath, closeLog := newLogger(cfg.Logging.Level, cfg.Logging.Format)
	defer closeLog()
	if isTTY {
		logLevel.Set(slog.LevelWarn)
	}
	sp.header(bannerVersion(buildVersion))

	if err := runForegroundPreflight(cfg, configPath, fromValue, logger); err != nil {
		return err
	}

	pool, pgMgr, schemaCache, watcherCancel, err := initDatabase(ctx, cfg, fromValue, branchName, sigCh, logger, sp)
	if err != nil {
		return err
	}
	if pool == nil {
		return nil // early signal exit
	}
	defer pool.Close()
	defer watcherCancel()

	// --- Core services ---
	core, err := initCoreServices(ctx, cfg, pool, logger)
	if err != nil {
		stopManagedPostgres(pgMgr, logger)
		return err
	}

	sp.step("Starting server...")
	state, err := wireServices(ctx, cfg, pool, core, schemaCache, logger)
	if err != nil {
		stopManagedPostgres(pgMgr, logger)
		return err
	}
	defer state.cleanup(pool.DB(), logger)

	usrCh := notifyUSR1()
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		if cfg.Server.TLSEnabled {
			ln, redir, tlsErr := buildTLSListener(ctx, cfg, state.certmagicConfig, logger)
			if tlsErr != nil {
				errCh <- tlsErr
				return
			}
			state.tlsRedirectSrv = redir
			errCh <- state.srv.StartTLSWithReady(ln, ready)
		} else {
			errCh <- state.srv.StartWithReady(ready)
		}
	}()

	readyCleanup, err := waitForServerReady(ready, errCh, usrCh, cfg, pgMgr, state.srv, logger, sp, readyState{
		isTTY:             isTTY,
		generatedPassword: generatedPassword,
		logPath:           logPath,
		logLevel:          logLevel,
	})
	if err != nil {
		return err
	}
	defer readyCleanup()

	return runGracefulShutdown(ctx, errCh, sigCh, watcherCancel, state, pgMgr, logger)
}
