package sbmigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Migrator handles migration from Supabase to AYB.
type Migrator struct {
	source   *sql.DB
	target   *sql.DB
	opts     MigrationOptions
	stats    MigrationStats
	output   io.Writer
	verbose  bool
	progress migrate.ProgressReporter
	// sourceColumnCache memoizes source schema column existence checks.
	sourceColumnCache map[string]bool
	// skippedTables tracks source tables intentionally skipped due schema incompatibilities.
	skippedTables map[string]string
}

// NewMigrator creates a migrator that connects to both the source (Supabase)
// and target (AYB) PostgreSQL databases.
func NewMigrator(opts MigrationOptions) (*Migrator, error) {
	if opts.SourceURL == "" {
		return nil, fmt.Errorf("source database URL is required")
	}
	if opts.TargetURL == "" {
		return nil, fmt.Errorf("target database URL is required")
	}

	source, err := sql.Open("pgx", opts.SourceURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to source database: %w", err)
	}
	if err := source.PingContext(context.Background()); err != nil {
		source.Close()
		return nil, fmt.Errorf("pinging source database: %w", err)
	}

	target, err := sql.Open("pgx", opts.TargetURL)
	if err != nil {
		source.Close()
		return nil, fmt.Errorf("connecting to target database: %w", err)
	}
	if err := target.PingContext(context.Background()); err != nil {
		source.Close()
		target.Close()
		return nil, fmt.Errorf("pinging target database: %w", err)
	}

	// Verify source is a Supabase database.
	var exists bool
	err = source.QueryRowContext(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'auth' AND table_name = 'users'
		)
	`).Scan(&exists)
	if err != nil || !exists {
		source.Close()
		target.Close()
		return nil, fmt.Errorf("source database does not appear to be a Supabase database (auth.users table not found)")
	}

	output := io.Writer(os.Stdout)
	if opts.DryRun && !opts.Verbose {
		output = io.Discard
	}

	progress := opts.Progress
	if progress == nil {
		progress = migrate.NopReporter{}
	}

	return &Migrator{
		source:   source,
		target:   target,
		opts:     opts,
		output:   output,
		verbose:  opts.Verbose,
		progress: progress,
	}, nil
}

// Close releases both database connections.
func (m *Migrator) Close() error {
	var errs []string
	if m.source != nil {
		if err := m.source.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if m.target != nil {
		if err := m.target.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("closing connections: %s", strings.Join(errs, "; "))
	}
	return nil
}

// phaseCount returns the total number of migration phases based on options.
func (m *Migrator) phaseCount() int {
	n := 1 // auth is always run
	if !m.opts.SkipData {
		n += 2 // schema + data
	}
	if !m.opts.SkipOAuth {
		n++
	}
	if !m.opts.SkipRLS {
		n++
	}
	if !m.opts.SkipStorage && m.opts.StorageExportPath != "" {
		n++
	}
	return n
}

// Migrate runs the full Supabase → AYB migration in a single transaction.
func (m *Migrator) Migrate(ctx context.Context) (*MigrationStats, error) {
	fmt.Fprintln(m.output, "Starting Supabase migration...")

	// Begin target transaction.
	tx, err := m.target.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Verify _ayb_users table exists.
	var tableExists bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = '_ayb_users'
		)
	`).Scan(&tableExists)
	if err != nil || !tableExists {
		return nil, fmt.Errorf("_ayb_users table not found — run 'ayb start' or 'ayb migrate up' first")
	}

	// Check for existing users unless --force.
	if !m.opts.Force {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM _ayb_users`).Scan(&count); err != nil {
			return nil, fmt.Errorf("checking existing users: %w", err)
		}
		if count > 0 {
			return nil, fmt.Errorf("_ayb_users table is not empty (%d users) — use --force to proceed", count)
		}
	}

	totalPhases := m.phaseCount()
	phaseIdx := 0

	// Phase: Schema (create tables + views in target).
	if !m.opts.SkipData {
		phaseIdx++
		if err := m.migrateSchema(ctx, tx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("schema migration: %w", err)
		}
	}

	// Phase: Data (stream rows from source to target).
	if !m.opts.SkipData {
		phaseIdx++
		if err := m.migrateData(ctx, tx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("data migration: %w", err)
		}
	}

	// Phase: Auth users.
	phaseIdx++
	if err := m.migrateAuthUsers(ctx, tx, phaseIdx, totalPhases); err != nil {
		return nil, fmt.Errorf("auth user migration: %w", err)
	}

	// Phase: OAuth identities.
	if !m.opts.SkipOAuth {
		phaseIdx++
		if err := m.migrateOAuthIdentities(ctx, tx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("OAuth identity migration: %w", err)
		}
	}

	// Phase: RLS policies.
	if !m.opts.SkipRLS {
		phaseIdx++
		if err := m.migrateRLSPolicies(ctx, tx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("RLS policy migration: %w", err)
		}
	}

	// Commit DB transaction before file operations.
	if m.opts.DryRun {
		fmt.Fprintln(m.output, "\n[DRY RUN] Rolling back (no changes made)")
	} else {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("committing transaction: %w", err)
		}
	}

	// Phase: Storage files (outside transaction — filesystem operations).
	if !m.opts.SkipStorage && m.opts.StorageExportPath != "" && !m.opts.DryRun {
		phaseIdx++
		if err := m.migrateStorage(ctx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("storage migration: %w", err)
		}
	}

	fmt.Fprintln(m.output, "\nMigration complete!")
	m.printStats()

	return &m.stats, nil
}

// Analyze performs a pre-flight analysis of the source database.
func (m *Migrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	report := &migrate.AnalysisReport{
		SourceType: "Supabase",
		SourceInfo: redactURL(m.opts.SourceURL),
	}

	// Count auth users (excluding anonymous, matching Migrate behavior).
	hasIsAnonymous, err := m.sourceColumnExists(ctx, "auth", "users", "is_anonymous")
	if err != nil {
		return nil, err
	}
	hasDeletedAt, err := m.sourceColumnExists(ctx, "auth", "users", "deleted_at")
	if err != nil {
		return nil, err
	}
	authQuery := buildAuthUsersCountQuery(m.opts.IncludeAnonymous, hasIsAnonymous, hasDeletedAt)
	err = m.source.QueryRowContext(ctx, authQuery).Scan(&report.AuthUsers)
	if err != nil {
		return nil, fmt.Errorf("counting auth users: %w", err)
	}

	// Count OAuth identities (excluding 'email' provider).
	err = m.source.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM auth.identities
		WHERE provider != 'email'
	`).Scan(&report.OAuthLinks)
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("could not count OAuth identities: %v", err))
	}

	// Count RLS policies.
	err = m.source.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pg_policy pol
		JOIN pg_class c ON c.oid = pol.polrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
	`).Scan(&report.RLSPolicies)
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("could not count RLS policies: %v", err))
	}

	// Count public tables and total rows.
	if !m.opts.SkipData {
		tables, err := introspectTables(ctx, m.source)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not introspect tables: %v", err))
		} else {
			report.Tables = len(tables)
			for _, t := range tables {
				report.Records += int(t.RowCount)
			}
		}

		views, err := introspectViews(ctx, m.source)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not introspect views: %v", err))
		} else {
			report.Views = len(views)
		}
	}

	// Count storage objects.
	if !m.opts.SkipStorage {
		buckets, err := m.listStorageBuckets(ctx)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not list storage buckets: %v", err))
		} else {
			for _, b := range buckets {
				objects, err := m.listStorageObjects(ctx, b.ID)
				if err != nil {
					report.Warnings = append(report.Warnings,
						fmt.Sprintf("could not list objects in bucket %s: %v", b.Name, err))
					continue
				}
				report.Files += len(objects)
				for _, o := range objects {
					report.FileSizeBytes += o.Size
				}
			}
		}
	}

	return report, nil
}

// BuildValidationSummary compares source analysis with migration stats.
func BuildValidationSummary(report *migrate.AnalysisReport, stats *MigrationStats) *migrate.ValidationSummary {
	summary := &migrate.ValidationSummary{
		SourceLabel: "Supabase (source)",
		TargetLabel: "AYB (target)",
	}

	if report.Tables > 0 || stats.Tables > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Tables", SourceCount: report.Tables, TargetCount: stats.Tables,
		})
	}
	if report.Views > 0 || stats.Views > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Views", SourceCount: report.Views, TargetCount: stats.Views,
		})
	}
	if report.Records > 0 || stats.Records > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Records", SourceCount: report.Records, TargetCount: stats.Records,
		})
	}
	summary.Rows = append(summary.Rows, migrate.ValidationRow{
		Label: "Auth users", SourceCount: report.AuthUsers, TargetCount: stats.Users,
	})
	if report.OAuthLinks > 0 || stats.OAuthLinks > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "OAuth links", SourceCount: report.OAuthLinks, TargetCount: stats.OAuthLinks,
		})
	}
	if report.RLSPolicies > 0 || stats.Policies > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "RLS policies", SourceCount: report.RLSPolicies, TargetCount: stats.Policies,
		})
	}
	if report.Files > 0 || stats.StorageFiles > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Storage files", SourceCount: report.Files, TargetCount: stats.StorageFiles,
		})
	}

	for _, row := range summary.Rows {
		if row.SourceCount != row.TargetCount {
			summary.Warnings = append(summary.Warnings,
				fmt.Sprintf("%s count mismatch: source=%d target=%d", row.Label, row.SourceCount, row.TargetCount))
		}
	}

	if stats.Skipped > 0 {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("%d items skipped during migration", stats.Skipped))
	}
	if len(stats.Errors) > 0 {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("%d errors occurred during migration", len(stats.Errors)))
	}

	return summary
}

func (m *Migrator) migrateSchema(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Schema", Index: phaseIdx, Total: totalPhases}

	tables, err := introspectTables(ctx, m.source)
	if err != nil {
		return fmt.Errorf("introspecting tables: %w", err)
	}

	views, err := introspectViews(ctx, m.source)
	if err != nil {
		return fmt.Errorf("introspecting views: %w", err)
	}

	totalItems := len(tables) + len(views)
	m.progress.StartPhase(phase, totalItems)
	start := time.Now()

	fmt.Fprintln(m.output, "Creating schema...")

	type deferredTable struct {
		table   TableInfo
		lastErr error
	}
	deferred := make([]deferredTable, 0)

	for i, t := range tables {
		savepoint := fmt.Sprintf("ayb_schema_table_%d", i)
		if err := createTableWithSavepoint(ctx, tx, t, savepoint); err != nil {
			if isSkippableSchemaTableError(err) {
				deferred = append(deferred, deferredTable{table: t, lastErr: err})
				continue
			}
			return fmt.Errorf("creating table %s: %w", t.Name, err)
		}
		m.stats.Tables++
		m.progress.Progress(phase, i+1, totalItems)
		if m.verbose {
			fmt.Fprintf(m.output, "  CREATE TABLE %s (%d columns)\n", t.Name, len(t.Columns))
		}
	}

	// Retry deferred tables to handle valid FK dependencies created later in this phase.
	if len(deferred) > 0 {
		for pass := 1; pass <= len(deferred); pass++ {
			if len(deferred) == 0 {
				break
			}

			next := make([]deferredTable, 0, len(deferred))
			progressed := false

			for i, item := range deferred {
				savepoint := fmt.Sprintf("ayb_schema_table_retry_%d_%d", pass, i)
				if err := createTableWithSavepoint(ctx, tx, item.table, savepoint); err != nil {
					if isSkippableSchemaTableError(err) {
						item.lastErr = err
						next = append(next, item)
						continue
					}
					return fmt.Errorf("creating table %s: %w", item.table.Name, err)
				}

				progressed = true
				m.stats.Tables++
				if m.verbose {
					fmt.Fprintf(m.output, "  CREATE TABLE %s (%d columns)\n", item.table.Name, len(item.table.Columns))
				}
			}

			if !progressed {
				for _, item := range next {
					m.markSkippedTable(item.table.Name, item.lastErr)
					m.stats.Skipped++
					m.progress.Warn(fmt.Sprintf("skipping table %s due source/target schema incompatibility: %v", item.table.Name, item.lastErr))
				}
				break
			}

			deferred = next
		}
	}

	for i, v := range views {
		ddl := createViewSQL(v)
		savepoint := fmt.Sprintf("ayb_schema_view_%d", i)
		if err := execSavepointCommand(ctx, tx, "SAVEPOINT "+savepoint); err != nil {
			return fmt.Errorf("creating savepoint for view %s: %w", v.Name, err)
		}
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			if rbErr := execSavepointCommand(ctx, tx, "ROLLBACK TO SAVEPOINT "+savepoint); rbErr != nil {
				return fmt.Errorf("rolling back savepoint for view %s after error %v: %w", v.Name, err, rbErr)
			}
			if relErr := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); relErr != nil {
				return fmt.Errorf("releasing savepoint for view %s after rollback: %w", v.Name, relErr)
			}
			// Views may depend on tables that don't exist in the target yet.
			// Log a warning instead of failing.
			m.progress.Warn(fmt.Sprintf("skipping view %s: %v", v.Name, err))
			continue
		}
		if err := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); err != nil {
			return fmt.Errorf("releasing savepoint for view %s: %w", v.Name, err)
		}
		m.stats.Views++
		if m.verbose {
			fmt.Fprintf(m.output, "  CREATE VIEW %s\n", v.Name)
		}
	}

	m.progress.CompletePhase(phase, totalItems, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d tables, %d views created\n", m.stats.Tables, m.stats.Views)
	return nil
}

func (m *Migrator) migrateData(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Data", Index: phaseIdx, Total: totalPhases}

	// Re-introspect to get tables that now exist in the target.
	tables, err := introspectTables(ctx, m.source)
	if err != nil {
		return fmt.Errorf("introspecting tables for data copy: %w", err)
	}
	tables = m.filterSkippedTables(tables)

	var totalRows int64
	for _, t := range tables {
		totalRows += t.RowCount
	}

	m.progress.StartPhase(phase, int(totalRows))
	start := time.Now()

	fmt.Fprintln(m.output, "Copying data...")

	type deferredDataTable struct {
		table   TableInfo
		lastErr error
	}
	deferred := make([]deferredDataTable, 0)

	copied := 0
	for i, t := range tables {
		savepoint := fmt.Sprintf("ayb_data_table_%d", i)
		count, err := copyTableDataWithSavepoint(ctx, m.source, tx, t, savepoint, func(n int) {
			m.progress.Progress(phase, copied+n, int(totalRows))
		})
		if err != nil {
			if isRetriableDataTableError(err) {
				deferred = append(deferred, deferredDataTable{table: t, lastErr: err})
				continue
			}
			return fmt.Errorf("copying data for %s: %w", t.Name, err)
		}
		copied += count
		m.stats.Records += count
		if m.verbose {
			fmt.Fprintf(m.output, "  %s: %d rows\n", t.Name, count)
		}
	}

	// Retry deferred tables to resolve FK dependencies where parent rows are copied later.
	if len(deferred) > 0 {
		for pass := 1; pass <= len(deferred); pass++ {
			if len(deferred) == 0 {
				break
			}

			next := make([]deferredDataTable, 0, len(deferred))
			progressed := false

			for i, item := range deferred {
				savepoint := fmt.Sprintf("ayb_data_table_retry_%d_%d", pass, i)
				count, err := copyTableDataWithSavepoint(ctx, m.source, tx, item.table, savepoint, func(n int) {
					m.progress.Progress(phase, copied+n, int(totalRows))
				})
				if err != nil {
					if isRetriableDataTableError(err) {
						item.lastErr = err
						next = append(next, item)
						continue
					}
					return fmt.Errorf("copying data for %s: %w", item.table.Name, err)
				}

				progressed = true
				copied += count
				m.stats.Records += count
				if m.verbose {
					fmt.Fprintf(m.output, "  %s: %d rows\n", item.table.Name, count)
				}
			}

			if !progressed {
				for _, item := range next {
					m.markSkippedTable(item.table.Name, item.lastErr)
					m.stats.Skipped++
					m.progress.Warn(fmt.Sprintf("skipping data copy for %s due unresolved dependency: %v", item.table.Name, item.lastErr))
				}
				break
			}

			deferred = next
		}
	}

	// Reset sequences.
	seqCount, err := resetSequences(ctx, tx, tables)
	if err != nil {
		m.progress.Warn(fmt.Sprintf("sequence reset: %v", err))
	}
	m.stats.Sequences = seqCount

	m.progress.CompletePhase(phase, int(totalRows), time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d records copied across %d tables\n", m.stats.Records, len(tables))
	return nil
}

func (m *Migrator) migrateAuthUsers(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Auth users", Index: phaseIdx, Total: totalPhases}
	m.progress.StartPhase(phase, 0) // unknown count until we query
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating auth users...")

	hasIsAnonymous, err := m.sourceColumnExists(ctx, "auth", "users", "is_anonymous")
	if err != nil {
		return err
	}
	hasDeletedAt, err := m.sourceColumnExists(ctx, "auth", "users", "deleted_at")
	if err != nil {
		return err
	}
	hasEmailConfirmedAt, err := m.sourceColumnExists(ctx, "auth", "users", "email_confirmed_at")
	if err != nil {
		return err
	}
	hasConfirmedAt, err := m.sourceColumnExists(ctx, "auth", "users", "confirmed_at")
	if err != nil {
		return err
	}
	confirmedAtExpr := "NULL::timestamptz"
	if hasEmailConfirmedAt {
		confirmedAtExpr = "email_confirmed_at"
	} else if hasConfirmedAt {
		confirmedAtExpr = "confirmed_at"
	}
	query := buildAuthUsersSelectQuery(m.opts.IncludeAnonymous, hasIsAnonymous, hasDeletedAt, confirmedAtExpr)

	rows, err := m.source.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("querying auth.users: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var u SupabaseUser
		var emailConfAt sql.NullTime

		if err := rows.Scan(
			&u.ID, &u.Email, &u.EncryptedPassword,
			&emailConfAt, &u.CreatedAt, &u.UpdatedAt,
			&u.IsAnonymous,
		); err != nil {
			return fmt.Errorf("scanning user: %w", err)
		}

		// Skip users without email (phone-only, anonymous).
		if u.Email == "" {
			m.stats.Skipped++
			if m.verbose {
				fmt.Fprintf(m.output, "  skipped user %s (no email)\n", u.ID)
			}
			continue
		}

		// Skip users without a password hash (OAuth-only users get empty string).
		// They'll still be importable via OAuth identity linking.
		if u.EncryptedPassword == "" {
			// Insert with a placeholder — they can only auth via OAuth or password reset.
			u.EncryptedPassword = "$none$"
		}

		emailVerified := emailConfAt.Valid

		if m.verbose {
			fmt.Fprintf(m.output, "  %s (%s) verified=%v\n", u.Email, u.ID, emailVerified)
		}

		result, err := tx.ExecContext(ctx,
			`INSERT INTO _ayb_users (id, email, password_hash, email_verified, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (id) DO NOTHING`,
			u.ID, strings.ToLower(u.Email), u.EncryptedPassword,
			emailVerified, u.CreatedAt, u.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("inserting user %s: %w", u.Email, err)
		}
		if n, _ := result.RowsAffected(); n > 0 {
			m.stats.Users++
		}
		m.progress.Progress(phase, m.stats.Users, 0)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	m.progress.CompletePhase(phase, m.stats.Users, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d users migrated (%d skipped)\n", m.stats.Users, m.stats.Skipped)
	return nil
}

func (m *Migrator) migrateOAuthIdentities(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "OAuth", Index: phaseIdx, Total: totalPhases}
	m.progress.StartPhase(phase, 0)
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating OAuth identities...")

	hasIdentityData, err := m.sourceColumnExists(ctx, "auth", "identities", "identity_data")
	if err != nil {
		return err
	}
	hasProviderID, err := m.sourceColumnExists(ctx, "auth", "identities", "provider_id")
	if err != nil {
		return err
	}
	hasCreatedAt, err := m.sourceColumnExists(ctx, "auth", "identities", "created_at")
	if err != nil {
		return err
	}
	hasDeletedAt, err := m.sourceColumnExists(ctx, "auth", "users", "deleted_at")
	if err != nil {
		return err
	}
	query := buildOAuthIdentitiesQuery(hasIdentityData, hasProviderID, hasCreatedAt, hasDeletedAt)
	rows, err := m.source.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("querying auth.identities: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID, provider, identityDataJSON string
		var createdAt sql.NullTime

		if err := rows.Scan(&userID, &provider, &identityDataJSON, &createdAt); err != nil {
			return fmt.Errorf("scanning identity: %w", err)
		}

		// Skip the "email" provider — that's password auth, not OAuth.
		if provider == "email" {
			continue
		}

		var identityData map[string]any
		if err := json.Unmarshal([]byte(identityDataJSON), &identityData); err != nil {
			m.stats.Errors = append(m.stats.Errors,
				fmt.Sprintf("parsing identity_data for user %s: %v", userID, err))
			continue
		}

		providerUserID := extractString(identityData, "sub", "provider_id")
		email := extractString(identityData, "email")
		name := extractString(identityData, "name", "full_name")

		if providerUserID == "" {
			m.stats.Skipped++
			if m.verbose {
				fmt.Fprintf(m.output, "  skipped identity for user %s (no provider_user_id)\n", userID)
			}
			continue
		}

		if m.verbose {
			fmt.Fprintf(m.output, "  %s → %s (%s)\n", provider, email, providerUserID)
		}

		created := time.Now()
		if createdAt.Valid {
			created = createdAt.Time
		}

		result, err := tx.ExecContext(ctx,
			`INSERT INTO _ayb_oauth_accounts (user_id, provider, provider_user_id, email, name, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (provider, provider_user_id) DO NOTHING`,
			userID, provider, providerUserID, email, name, created,
		)
		if err != nil {
			return fmt.Errorf("inserting OAuth account for user %s: %w", userID, err)
		}
		if n, _ := result.RowsAffected(); n > 0 {
			m.stats.OAuthLinks++
		}
		m.progress.Progress(phase, m.stats.OAuthLinks, 0)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	m.progress.CompletePhase(phase, m.stats.OAuthLinks, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d OAuth identities migrated\n", m.stats.OAuthLinks)
	return nil
}

func (m *Migrator) migrateRLSPolicies(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "RLS policies", Index: phaseIdx, Total: totalPhases}
	m.progress.StartPhase(phase, 0)
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating RLS policies...")

	policies, err := ReadRLSPolicies(ctx, m.source)
	if err != nil {
		return err
	}

	if len(policies) == 0 {
		fmt.Fprintln(m.output, "  no RLS policies found in public schema")
		m.progress.CompletePhase(phase, 0, time.Since(start))
		return nil
	}

	for _, p := range policies {
		if m.isSkippedTable(p.TableName) {
			m.progress.Warn(fmt.Sprintf("skipping policy %s on %s: table was skipped during schema migration", p.PolicyName, p.TableName))
			continue
		}

		if m.verbose {
			fmt.Fprintf(m.output, "  %s.%s: %s\n", p.TableName, p.PolicyName, p.Command)
		}

		// Drop existing policy on target (idempotent).
		dropSQL := fmt.Sprintf("DROP POLICY IF EXISTS %q ON %q.%q",
			p.PolicyName, p.SchemaName, p.TableName)
		if _, err := tx.ExecContext(ctx, dropSQL); err != nil {
			return fmt.Errorf("dropping policy %s on %s: %w", p.PolicyName, p.TableName, err)
		}

		// Enable RLS on the table.
		enableSQL := fmt.Sprintf("ALTER TABLE %q.%q ENABLE ROW LEVEL SECURITY",
			p.SchemaName, p.TableName)
		if _, err := tx.ExecContext(ctx, enableSQL); err != nil {
			return fmt.Errorf("enabling RLS on %s: %w", p.TableName, err)
		}

		// Create rewritten policy.
		rewrittenSQL := GenerateRewrittenPolicy(p)
		if _, err := tx.ExecContext(ctx, rewrittenSQL); err != nil {
			return fmt.Errorf("creating policy %s on %s: %w", p.PolicyName, p.TableName, err)
		}
		m.stats.Policies++
		m.progress.Progress(phase, m.stats.Policies, len(policies))
	}

	m.progress.CompletePhase(phase, m.stats.Policies, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d RLS policies rewritten\n", m.stats.Policies)
	return nil
}

func (m *Migrator) printStats() {
	fmt.Fprintf(m.output, "\nSummary:\n")
	if m.stats.Tables > 0 {
		fmt.Fprintf(m.output, "  Tables:     %d\n", m.stats.Tables)
	}
	if m.stats.Views > 0 {
		fmt.Fprintf(m.output, "  Views:      %d\n", m.stats.Views)
	}
	if m.stats.Records > 0 {
		fmt.Fprintf(m.output, "  Records:    %d\n", m.stats.Records)
	}
	if m.stats.Sequences > 0 {
		fmt.Fprintf(m.output, "  Sequences:  %d\n", m.stats.Sequences)
	}
	fmt.Fprintf(m.output, "  Users:      %d\n", m.stats.Users)
	fmt.Fprintf(m.output, "  OAuth:      %d\n", m.stats.OAuthLinks)
	fmt.Fprintf(m.output, "  RLS:        %d\n", m.stats.Policies)
	if m.stats.StorageFiles > 0 {
		fmt.Fprintf(m.output, "  Files:      %d (%s)\n", m.stats.StorageFiles, migrate.FormatBytes(m.stats.StorageBytes))
	}
	fmt.Fprintf(m.output, "  Skipped:    %d\n", m.stats.Skipped)
	if len(m.stats.Errors) > 0 {
		fmt.Fprintf(m.output, "  Errors:     %d\n", len(m.stats.Errors))
		for _, e := range m.stats.Errors {
			fmt.Fprintf(m.output, "    - %s\n", e)
		}
	}
}

// redactURL strips credentials from a PostgreSQL connection URL for safe display.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.User = nil
	return u.String()
}

// extractString tries multiple keys in a map and returns the first non-empty string value.
func extractString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if val, ok := data[key]; ok {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func (m *Migrator) sourceColumnExists(ctx context.Context, schema, table, column string) (bool, error) {
	if m.sourceColumnCache == nil {
		m.sourceColumnCache = make(map[string]bool)
	}

	key := schema + "." + table + "." + column
	if exists, ok := m.sourceColumnCache[key]; ok {
		return exists, nil
	}

	var exists bool
	err := m.source.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = $1
			  AND table_name = $2
			  AND column_name = $3
		)
	`, schema, table, column).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking source schema for %s: %w", key, err)
	}

	m.sourceColumnCache[key] = exists
	return exists, nil
}

func buildAuthUsersCountQuery(includeAnonymous, hasIsAnonymous, hasDeletedAt bool) string {
	query := `SELECT COUNT(*) FROM auth.users WHERE 1=1`
	if hasDeletedAt {
		query += " AND deleted_at IS NULL"
	}
	if hasIsAnonymous && !includeAnonymous {
		query += " AND (is_anonymous = false OR is_anonymous IS NULL)"
	}
	return query
}

func buildAuthUsersSelectQuery(includeAnonymous, hasIsAnonymous, hasDeletedAt bool, confirmedAtExpr string) string {
	anonymousExpr := "false"
	if hasIsAnonymous {
		anonymousExpr = "COALESCE(is_anonymous, false)"
	}
	if strings.TrimSpace(confirmedAtExpr) == "" {
		confirmedAtExpr = "NULL::timestamptz"
	}

	query := fmt.Sprintf(`
		SELECT id, COALESCE(email, ''), COALESCE(encrypted_password, ''),
		       %s AS email_confirmed_at, created_at, updated_at,
		       %s AS is_anonymous
		FROM auth.users
		WHERE 1=1`, confirmedAtExpr, anonymousExpr)
	if hasDeletedAt {
		query += " AND deleted_at IS NULL"
	}
	if hasIsAnonymous && !includeAnonymous {
		query += " AND (is_anonymous = false OR is_anonymous IS NULL)"
	}
	query += " ORDER BY created_at"
	return query
}

func (m *Migrator) markSkippedTable(name string, err error) {
	if m.skippedTables == nil {
		m.skippedTables = make(map[string]string)
	}
	m.skippedTables[name] = err.Error()
}

func (m *Migrator) isSkippedTable(name string) bool {
	if m.skippedTables == nil {
		return false
	}
	_, ok := m.skippedTables[name]
	return ok
}

func (m *Migrator) filterSkippedTables(tables []TableInfo) []TableInfo {
	if len(tables) == 0 || len(m.skippedTables) == 0 {
		return tables
	}

	filtered := make([]TableInfo, 0, len(tables))
	for _, t := range tables {
		if m.isSkippedTable(t.Name) {
			if m.verbose {
				fmt.Fprintf(m.output, "  skipped data copy for %s (schema incompatibility)\n", t.Name)
			}
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

func isSkippableSchemaTableError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	switch pgErr.Code {
	case "42883": // undefined_function
		return true
	case "42704": // undefined_object (often undefined type)
		return true
	case "42P01": // undefined_table (e.g. FK references skipped table)
		return true
	case "0A000": // feature_not_supported
		return true
	default:
		return false
	}
}

func isRetriableDataTableError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	switch pgErr.Code {
	case "23503": // foreign_key_violation
		return true
	case "42P01": // undefined_table
		return true
	default:
		return false
	}
}

func execSavepointCommand(ctx context.Context, tx *sql.Tx, stmt string) error {
	_, err := tx.ExecContext(ctx, stmt)
	return err
}

func copyTableDataWithSavepoint(
	ctx context.Context,
	source *sql.DB,
	tx *sql.Tx,
	table TableInfo,
	savepoint string,
	progressFn func(int),
) (int, error) {
	if err := execSavepointCommand(ctx, tx, "SAVEPOINT "+savepoint); err != nil {
		return 0, fmt.Errorf("creating savepoint for data copy %s: %w", table.Name, err)
	}

	count, err := copyTableData(ctx, source, tx, table, progressFn)
	if err != nil {
		if rbErr := execSavepointCommand(ctx, tx, "ROLLBACK TO SAVEPOINT "+savepoint); rbErr != nil {
			return 0, fmt.Errorf("rolling back savepoint for data copy %s after error %v: %w", table.Name, err, rbErr)
		}
		if relErr := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); relErr != nil {
			return 0, fmt.Errorf("releasing savepoint for data copy %s after rollback: %w", table.Name, relErr)
		}
		return 0, err
	}

	if err := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); err != nil {
		return 0, fmt.Errorf("releasing savepoint for data copy %s: %w", table.Name, err)
	}

	return count, nil
}

func createTableWithSavepoint(ctx context.Context, tx *sql.Tx, table TableInfo, savepoint string) error {
	ddl := createTableSQL(table)
	if err := execSavepointCommand(ctx, tx, "SAVEPOINT "+savepoint); err != nil {
		return fmt.Errorf("creating savepoint for table %s: %w", table.Name, err)
	}
	if _, err := tx.ExecContext(ctx, ddl); err != nil {
		if rbErr := execSavepointCommand(ctx, tx, "ROLLBACK TO SAVEPOINT "+savepoint); rbErr != nil {
			return fmt.Errorf("rolling back savepoint for table %s after error %v: %w", table.Name, err, rbErr)
		}
		if relErr := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); relErr != nil {
			return fmt.Errorf("releasing savepoint for table %s after rollback: %w", table.Name, relErr)
		}
		return err
	}
	if err := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); err != nil {
		return fmt.Errorf("releasing savepoint for table %s: %w", table.Name, err)
	}
	return nil
}

func buildOAuthIdentitiesQuery(hasIdentityData, hasProviderID, hasCreatedAt, usersHasDeletedAt bool) string {
	identityDataExpr := `'{}'::text`
	if hasIdentityData {
		identityDataExpr = `COALESCE(i.identity_data::text, '{}')`
	} else if hasProviderID {
		identityDataExpr = `jsonb_build_object(
			'provider_id', COALESCE(i.provider_id::text, ''),
			'sub', COALESCE(i.provider_id::text, '')
		)::text`
	}

	createdAtExpr := "NULL::timestamptz"
	orderByExpr := "i.user_id"
	if hasCreatedAt {
		createdAtExpr = "i.created_at"
		orderByExpr = "i.created_at"
	}
	usersWhere := ""
	if usersHasDeletedAt {
		usersWhere = "WHERE u.deleted_at IS NULL"
	}

	return fmt.Sprintf(`
		SELECT i.user_id, i.provider, %s, %s
		FROM auth.identities i
		JOIN auth.users u ON u.id = i.user_id
		%s
		ORDER BY %s
	`, identityDataExpr, createdAtExpr, usersWhere, orderByExpr)
}
