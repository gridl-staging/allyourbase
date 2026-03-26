// Package cli provides the tenants subcommand for assigning legacy apps without tenant_id to tenant ownership based on app owner email addresses. It supports dry-run preview, execution with --apply, and post-migration consistency validation.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/spf13/cobra"
)

var tenantsCmd = &cobra.Command{
	Use:   "tenants",
	Short: "Manage multi-tenancy and legacy migration",
}

var tenantsMigrateLegacyCmd = &cobra.Command{
	Use:   "migrate-legacy",
	Short: "Migrate legacy apps to tenant ownership",
	Long: `Assign every legacy app (one without a tenant_id) to exactly one tenant,
derived from the app owner's email address.

By default this command runs in DRY-RUN mode and prints a preview of proposed
actions without writing anything. Pass --apply to execute the migration.

Examples:
  ayb tenants migrate-legacy                  # dry-run (safe, default)
  ayb tenants migrate-legacy --apply          # apply migration
  ayb tenants migrate-legacy --check-consistency  # post-migration health check`,
	RunE: runTenantsMigrateLegacy,
}

func init() {
	tenantsMigrateLegacyCmd.Flags().String("config", "", "Path to ayb.toml config file")
	tenantsMigrateLegacyCmd.Flags().String("database-url", "", "PostgreSQL connection URL (overrides config)")
	tenantsMigrateLegacyCmd.Flags().Bool("apply", false, "Execute the migration (default: dry-run)")
	tenantsMigrateLegacyCmd.Flags().Bool("check-consistency", false, "Run post-migration consistency checker only")
	tenantsMigrateLegacyCmd.Flags().Int("batch-size", 50, "Groups processed per transaction")
	tenantsMigrateLegacyCmd.Flags().Int("max-items", 0, "Maximum owner groups to process (0 = unlimited)")

	tenantsCmd.AddCommand(tenantsMigrateLegacyCmd)
}

// Orchestrates the tenant migration workflow by parsing flags, connecting to the database, and delegating to either consistency checking, apply, or dry-run mode based on command arguments.
func runTenantsMigrateLegacy(cmd *cobra.Command, _ []string) error {
	apply, _ := cmd.Flags().GetBool("apply")
	checkOnly, _ := cmd.Flags().GetBool("check-consistency")
	batchSize, _ := cmd.Flags().GetInt("batch-size")
	maxItems, _ := cmd.Flags().GetInt("max-items")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx := context.Background()

	pool, cleanup, err := connectMigrationDB(cmd, ctx, logger)
	if err != nil {
		return err
	}
	defer cleanup()

	svc := tenant.NewMigrationService(pool.DB(), logger)
	opts := tenant.MigrationOpts{BatchSize: batchSize, MaxItems: maxItems}

	if checkOnly {
		return runConsistencyCheck(ctx, svc)
	}
	if apply {
		return runApply(ctx, svc, opts)
	}
	return runDryRun(ctx, svc, opts)
}

// Establishes a PostgreSQL connection with the specified or configured database URL, runs bootstrap and any pending schema migrations, and returns the connection pool with a cleanup function.
func connectMigrationDB(cmd *cobra.Command, ctx context.Context, logger *slog.Logger) (*postgres.Pool, func(), error) {
	cfg, err := loadMigrateConfig(cmd)
	if err != nil {
		return nil, nil, err
	}

	dbURL := cfg.Database.URL
	if v, _ := cmd.Flags().GetString("database-url"); v != "" {
		dbURL = v
	}
	if dbURL == "" {
		return nil, nil, fmt.Errorf("no database URL configured (set database.url in ayb.toml, AYB_DATABASE_URL env, or --database-url flag)")
	}

	pool, err := postgres.New(ctx, postgres.Config{URL: dbURL, MaxConns: 5, MinConns: 1}, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to database: %w", err)
	}

	runner := migrations.NewRunner(pool.DB(), logger)
	if err := runner.Bootstrap(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("bootstrapping migrations: %w", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("running migrations: %w", err)
	}

	return pool, pool.Close, nil
}

// Previews the legacy app to tenant migration without making changes, displaying a table of proposed owner groups and their assignments, followed by a summary of what would be created.
func runDryRun(ctx context.Context, svc *tenant.MigrationService, opts tenant.MigrationOpts) error {
	fmt.Fprintln(os.Stderr, "Mode: dry-run (pass --apply to execute)")
	start := time.Now()

	report, err := svc.MigrationDryRun(ctx, opts)
	if err != nil {
		return fmt.Errorf("dry-run: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "OWNER\tEMAIL\tAPPS\tSLUG\tACTION\tCONFLICT")
	fmt.Fprintln(w, strings.Repeat("---\t", 6))
	for _, g := range report.Groups {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\n",
			g.OwnerUserID, g.OwnerEmail, len(g.AppIDs), g.ProposedSlug, g.Action, g.Conflict,
		)
	}
	w.Flush()

	fmt.Printf("\nSummary (dry-run, %s):\n", time.Since(start).Round(time.Millisecond))
	printMigrationSummary(report.Summary)
	return nil
}

// Executes the legacy app to tenant migration, printing a summary of created and reused tenants, assigned apps, and memberships, then returns an error if any groups failed to migrate.
func runApply(ctx context.Context, svc *tenant.MigrationService, opts tenant.MigrationOpts) error {
	fmt.Fprintln(os.Stderr, "Mode: apply")
	start := time.Now()

	result, err := svc.MigrateLegacyApps(ctx, opts)
	if err != nil {
		return fmt.Errorf("migration: %w", err)
	}

	fmt.Printf("\nSummary (apply, %s):\n", time.Since(start).Round(time.Millisecond))
	printMigrationSummary(*result)

	if len(result.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d error(s):\n", len(result.Errors))
		for _, e := range result.Errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
		return fmt.Errorf("migration completed with %d error(s)", len(result.Errors))
	}
	return nil
}

// Validates the legacy app to tenant migration is complete and consistent, outputting a JSON report and returning an error if any inconsistencies are detected.
func runConsistencyCheck(ctx context.Context, svc *tenant.MigrationService) error {
	fmt.Fprintln(os.Stderr, "Mode: consistency check")

	report, err := svc.CheckMigrationConsistency(ctx)
	if err != nil {
		return fmt.Errorf("consistency check: %w", err)
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(data))

	if !report.Clean {
		return fmt.Errorf("consistency check failed: %s", strings.Join(report.Issues, "; "))
	}
	fmt.Fprintln(os.Stderr, "Consistency check: PASS")
	return nil
}

func printMigrationSummary(r tenant.MigrationResult) {
	fmt.Printf("  Examined groups:     %d\n", r.ExaminedGroups)
	fmt.Printf("  Created tenants:     %d\n", r.CreatedTenants)
	fmt.Printf("  Reused tenants:      %d\n", r.ReusedTenants)
	fmt.Printf("  Assigned apps:       %d\n", r.AssignedApps)
	fmt.Printf("  Created memberships: %d\n", r.CreatedMemberships)
	fmt.Printf("  Skipped groups:      %d\n", r.SkippedGroups)
	fmt.Printf("  Errored groups:      %d\n", r.ErroredGroups)
}
