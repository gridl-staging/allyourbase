package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/allyourbase/ayb/internal/fbmigrate"
	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/pbmigrate"
	"github.com/allyourbase/ayb/internal/sbmigrate"
	"github.com/allyourbase/ayb/internal/urlutil"
)

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
		logger.Info("detected generic PostgreSQL source", "url", urlutil.RedactURL(from))
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
	logger.Info("detected Supabase source", "url", urlutil.RedactURL(sourceURL))

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
