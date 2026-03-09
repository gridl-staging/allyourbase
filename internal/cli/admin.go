package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/spf13/cobra"
)

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Admin user management",
}

var adminCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new user account",
	Long: `Create a new user account in the database. Requires a running database
(or database URL in config/env/flag).

Example:
  ayb admin create --email admin@example.com --password mysecretpassword`,
	RunE: runAdminCreate,
}

var adminResetPasswordCmd = &cobra.Command{
	Use:   "reset-password",
	Short: "Reset the admin dashboard password",
	Long: `Generate a new random admin password for the running AYB server.
Sends SIGUSR1 to the server process, which regenerates the password and
prints it to the server's stderr. The new password is also returned here.

This only works for auto-generated passwords. If admin.password is set in
ayb.toml or AYB_ADMIN_PASSWORD, change it there and restart.

Example:
  ayb admin reset-password`,
	RunE: runAdminResetPassword,
}

func init() {
	adminCmd.AddCommand(adminCreateCmd)
	adminCmd.AddCommand(adminResetPasswordCmd)

	adminCreateCmd.Flags().String("config", "", "Path to ayb.toml config file")
	adminCreateCmd.Flags().String("database-url", "", "PostgreSQL connection URL (overrides config)")
	adminCreateCmd.Flags().String("email", "", "User email address")
	adminCreateCmd.Flags().String("password", "", "User password (min length from config, default 8)")
	adminCreateCmd.MarkFlagRequired("email")
	adminCreateCmd.MarkFlagRequired("password")
}

func runAdminCreate(cmd *cobra.Command, args []string) error {
	email, _ := cmd.Flags().GetString("email")
	password, _ := cmd.Flags().GetString("password")
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cfg, err := loadMigrateConfig(cmd)
	if err != nil {
		return err
	}

	dbURL := cfg.Database.URL
	if v, _ := cmd.Flags().GetString("database-url"); v != "" {
		dbURL = v
	}
	if dbURL == "" {
		return fmt.Errorf("no database URL configured (set database.url in ayb.toml, AYB_DATABASE_URL env, or --database-url flag)")
	}

	ctx := context.Background()
	pool, err := postgres.New(ctx, postgres.Config{
		URL:      dbURL,
		MaxConns: 5,
		MinConns: 1,
	}, logger)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	// Ensure system migrations are applied (creates _ayb_users table).
	migRunner := migrations.NewRunner(pool.DB(), logger)
	if err := migRunner.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrapping migrations: %w", err)
	}
	if _, err := migRunner.Run(ctx); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	user, err := auth.CreateUser(ctx, pool.DB(), email, password, cfg.Auth.MinPasswordLength)
	if err != nil {
		return fmt.Errorf("creating user: %w", err)
	}

	fmt.Printf("Created user: %s (%s)\n", user.Email, user.ID)
	return nil
}

func runAdminResetPassword(cmd *cobra.Command, args []string) error {
	pid, _, err := readAYBPID()
	if err != nil {
		return fmt.Errorf("AYB server not running (no PID file): %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("cannot find process %d: %w", pid, err)
	}

	// Clean up any stale result file.
	resultPath, err := aybResetResultPath()
	if err != nil {
		return fmt.Errorf("resolving result path: %w", err)
	}
	os.Remove(resultPath)

	// Send SIGUSR1 to trigger password regeneration.
	if err := sendUSR1(proc); err != nil {
		return fmt.Errorf("sending signal to AYB (pid %d): %w", pid, err)
	}

	// Poll for result file (server writes it after regenerating).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(resultPath)
		if err == nil && len(data) > 0 {
			os.Remove(resultPath)
			fmt.Fprintf(os.Stderr, "\n  New admin password:  %s\n\n", string(data))
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for password reset result (check server stderr)")
}
