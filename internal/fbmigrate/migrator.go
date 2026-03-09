package fbmigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Migrator orchestrates Firebase → AYB migration.
type Migrator struct {
	db       *sql.DB
	opts     MigrationOptions
	stats    MigrationStats
	output   io.Writer
	verbose  bool
	progress migrate.ProgressReporter
}

// NewMigrator creates a new Firebase migrator, validating options and connecting to the target DB.
func NewMigrator(opts MigrationOptions) (*Migrator, error) {
	if opts.AuthExportPath == "" && opts.FirestoreExportPath == "" && opts.RTDBExportPath == "" && opts.StorageExportPath == "" {
		return nil, fmt.Errorf("at least one export path is required (auth, Firestore, RTDB, or storage)")
	}
	if opts.DatabaseURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	// Validate auth export file exists if specified.
	if opts.AuthExportPath != "" {
		if _, err := os.Stat(opts.AuthExportPath); err != nil {
			return nil, fmt.Errorf("auth export file: %w", err)
		}
	}

	// Validate Firestore export directory exists if specified.
	if opts.FirestoreExportPath != "" {
		info, err := os.Stat(opts.FirestoreExportPath)
		if err != nil {
			return nil, fmt.Errorf("firestore export path: %w", err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("firestore export path must be a directory")
		}
	}

	// Validate RTDB export file exists if specified.
	if opts.RTDBExportPath != "" {
		if _, err := os.Stat(opts.RTDBExportPath); err != nil {
			return nil, fmt.Errorf("RTDB export file: %w", err)
		}
	}

	// Validate storage export directory exists if specified.
	if opts.StorageExportPath != "" {
		info, err := os.Stat(opts.StorageExportPath)
		if err != nil {
			return nil, fmt.Errorf("storage export path: %w", err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("storage export path must be a directory")
		}
	}

	db, err := sql.Open("pgx", opts.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
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
		db:       db,
		opts:     opts,
		output:   output,
		verbose:  opts.Verbose,
		progress: progress,
	}, nil
}

// Close releases the database connection.
func (m *Migrator) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// phaseCount returns the number of migration phases.
func (m *Migrator) phaseCount() int {
	n := 0
	if m.opts.AuthExportPath != "" {
		n += 2 // auth users + OAuth links
	}
	if m.opts.FirestoreExportPath != "" {
		n++ // Firestore data
	}
	if m.opts.RTDBExportPath != "" {
		n++ // Realtime Database
	}
	if m.opts.StorageExportPath != "" {
		n++ // Storage files
	}
	return n
}

// Migrate runs the full Firebase → AYB migration.
func (m *Migrator) Migrate(ctx context.Context) (*MigrationStats, error) {
	fmt.Fprintln(m.output, "Starting Firebase migration...")

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	totalPhases := m.phaseCount()
	phaseIdx := 0

	// Phase: Auth users.
	if m.opts.AuthExportPath != "" {
		users, hashConfig, err := ParseAuthExport(m.opts.AuthExportPath)
		if err != nil {
			return nil, err
		}

		// Use provided hash config or the one from the export.
		if m.opts.HashConfig != nil {
			hashConfig = m.opts.HashConfig
		}

		phaseIdx++
		if err := m.migrateAuthUsers(ctx, tx, users, hashConfig, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("auth migration: %w", err)
		}

		phaseIdx++
		if err := m.migrateOAuthLinks(ctx, tx, users, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("OAuth migration: %w", err)
		}
	}

	// Phase: Firestore data.
	if m.opts.FirestoreExportPath != "" {
		phaseIdx++
		if err := m.migrateFirestoreData(ctx, tx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("firestore migration: %w", err)
		}
	}

	// Phase: Realtime Database.
	if m.opts.RTDBExportPath != "" {
		phaseIdx++
		if err := m.migrateRTDB(ctx, tx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("RTDB migration: %w", err)
		}
	}

	// Commit DB transaction before filesystem operations.
	if m.opts.DryRun {
		fmt.Fprintln(m.output, "\n[DRY RUN] Rolling back (no changes made)")
	} else {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("committing transaction: %w", err)
		}
	}

	// Phase: Storage files (outside transaction — filesystem operations).
	if m.opts.StorageExportPath != "" && !m.opts.DryRun {
		phaseIdx++
		if err := m.migrateStorage(phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("storage migration: %w", err)
		}
	}

	fmt.Fprintln(m.output, "\nMigration complete!")
	m.printStats()

	return &m.stats, nil
}

// Analyze performs pre-flight analysis of the Firebase export.
func (m *Migrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	report := &migrate.AnalysisReport{
		SourceType: "Firebase",
	}

	if m.opts.AuthExportPath != "" {
		users, _, err := ParseAuthExport(m.opts.AuthExportPath)
		if err != nil {
			return nil, fmt.Errorf("parsing auth export: %w", err)
		}
		report.SourceInfo = m.opts.AuthExportPath

		for _, u := range users {
			if u.Disabled {
				continue
			}
			if IsAnonymousUser(u) || IsPhoneOnlyUser(u) {
				continue
			}
			if !IsEmailUser(u) {
				continue
			}
			report.AuthUsers++
			// Count OAuth links only for email users, matching migrateOAuthLinks() behavior.
			for range OAuthProviders(u) {
				report.OAuthLinks++
			}
		}
	}

	if m.opts.FirestoreExportPath != "" {
		collections, err := ParseFirestoreExport(m.opts.FirestoreExportPath)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not read Firestore export: %v", err))
		} else {
			report.Tables = len(collections)
			for _, c := range collections {
				report.Records += len(c.Documents)
			}
		}
	}

	// Analyze RTDB export.
	if m.opts.RTDBExportPath != "" {
		nodes, err := ParseRTDBExport(m.opts.RTDBExportPath)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not read RTDB export: %v", err))
		} else {
			report.Tables += len(nodes)
			for _, n := range nodes {
				report.Records += len(n.Children)
			}
		}
	}

	// Analyze storage export.
	if m.opts.StorageExportPath != "" {
		buckets, err := scanStorageExport(m.opts.StorageExportPath)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not scan storage export: %v", err))
		} else {
			for _, files := range buckets {
				report.Files += len(files)
				for _, f := range files {
					report.FileSizeBytes += f.Size
				}
			}
		}
	}

	return report, nil
}

// BuildValidationSummary compares source analysis with migration stats.
func BuildValidationSummary(report *migrate.AnalysisReport, stats *MigrationStats) *migrate.ValidationSummary {
	summary := &migrate.ValidationSummary{
		SourceLabel: "Firebase (source)",
		TargetLabel: "AYB (target)",
	}

	if report.AuthUsers > 0 || stats.Users > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Auth users", SourceCount: report.AuthUsers, TargetCount: stats.Users,
		})
	}
	if report.OAuthLinks > 0 || stats.OAuthLinks > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "OAuth links", SourceCount: report.OAuthLinks, TargetCount: stats.OAuthLinks,
		})
	}
	if report.Tables > 0 || stats.Collections > 0 || stats.RTDBNodes > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Collections", SourceCount: report.Tables, TargetCount: stats.Collections + stats.RTDBNodes,
		})
	}
	if report.Records > 0 || stats.Documents > 0 || stats.RTDBRecords > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Documents", SourceCount: report.Records, TargetCount: stats.Documents + stats.RTDBRecords,
		})
	}
	if stats.RTDBNodes > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "RTDB nodes", SourceCount: stats.RTDBNodes, TargetCount: stats.RTDBNodes,
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

func (m *Migrator) migrateAuthUsers(ctx context.Context, tx *sql.Tx, users []FirebaseUser, hashConfig *FirebaseHashConfig, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Auth users", Index: phaseIdx, Total: totalPhases}
	m.progress.StartPhase(phase, len(users))
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating auth users...")

	// Ensure _ayb_users exists.
	var tableExists bool
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = '_ayb_users'
		)
	`).Scan(&tableExists)
	if err != nil || !tableExists {
		return fmt.Errorf("_ayb_users table not found — run 'ayb start' or 'ayb migrate up' first")
	}

	for i, u := range users {
		// Skip disabled, anonymous, and phone-only users.
		if u.Disabled {
			m.stats.Skipped++
			if m.verbose {
				fmt.Fprintf(m.output, "  skipped user %s (disabled)\n", u.LocalID)
			}
			m.progress.Progress(phase, i+1, len(users))
			continue
		}
		if IsAnonymousUser(u) || IsPhoneOnlyUser(u) {
			m.stats.Skipped++
			if m.verbose {
				fmt.Fprintf(m.output, "  skipped user %s (anonymous/phone-only)\n", u.LocalID)
			}
			m.progress.Progress(phase, i+1, len(users))
			continue
		}

		if !IsEmailUser(u) {
			m.stats.Skipped++
			m.progress.Progress(phase, i+1, len(users))
			continue
		}

		// Encode password hash.
		passwordHash := "$none$"
		if IsPasswordUser(u) {
			passwordHash = EncodeFirebaseScryptHash(u.PasswordHash, u.Salt, hashConfig)
		}

		if m.verbose {
			fmt.Fprintf(m.output, "  %s (%s) verified=%v\n", u.Email, u.LocalID, u.EmailVerified)
		}

		aybUserID := FirebaseIDToUUID(u.LocalID)

		result, err := tx.ExecContext(ctx,
			`INSERT INTO _ayb_users (id, email, password_hash, email_verified, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (id) DO NOTHING`,
			aybUserID, strings.ToLower(u.Email), passwordHash,
			u.EmailVerified, parseEpochMs(u.CreatedAt), parseEpochMs(u.CreatedAt),
		)
		if err != nil {
			m.stats.Errors = append(m.stats.Errors, fmt.Sprintf("inserting user %s: %v", u.Email, err))
			m.progress.Progress(phase, i+1, len(users))
			continue
		}
		if n, _ := result.RowsAffected(); n > 0 {
			m.stats.Users++
		}
		m.progress.Progress(phase, i+1, len(users))
	}

	m.progress.CompletePhase(phase, m.stats.Users, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d users migrated (%d skipped)\n", m.stats.Users, m.stats.Skipped)
	return nil
}

func (m *Migrator) migrateOAuthLinks(ctx context.Context, tx *sql.Tx, users []FirebaseUser, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "OAuth", Index: phaseIdx, Total: totalPhases}
	m.progress.StartPhase(phase, 0)
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating OAuth identities...")

	for _, u := range users {
		if u.Disabled || !IsEmailUser(u) {
			continue
		}
		providers := OAuthProviders(u)
		for _, p := range providers {
			providerName := NormalizeProvider(p.ProviderID)
			email := p.Email
			if email == "" {
				email = u.Email
			}

			if m.verbose {
				fmt.Fprintf(m.output, "  %s → %s (%s)\n", providerName, email, p.RawID)
			}

			aybUserID := FirebaseIDToUUID(u.LocalID)

			result, err := tx.ExecContext(ctx,
				`INSERT INTO _ayb_oauth_accounts (user_id, provider, provider_user_id, email, name, created_at)
				 VALUES ($1, $2, $3, $4, $5, $6)
				 ON CONFLICT (provider, provider_user_id) DO NOTHING`,
				aybUserID, providerName, p.RawID, email, p.DisplayName, parseEpochMs(u.CreatedAt),
			)
			if err != nil {
				m.stats.Errors = append(m.stats.Errors,
					fmt.Sprintf("inserting OAuth for user %s: %v", u.LocalID, err))
				continue
			}
			if n, _ := result.RowsAffected(); n > 0 {
				m.stats.OAuthLinks++
			}
		}
	}

	m.progress.CompletePhase(phase, m.stats.OAuthLinks, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d OAuth identities migrated\n", m.stats.OAuthLinks)
	return nil
}

func (m *Migrator) migrateFirestoreData(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Firestore", Index: phaseIdx, Total: totalPhases}

	collections, err := ParseFirestoreExport(m.opts.FirestoreExportPath)
	if err != nil {
		return err
	}

	var totalDocs int
	for _, c := range collections {
		totalDocs += len(c.Documents)
	}

	m.progress.StartPhase(phase, totalDocs)
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating Firestore data...")

	processed := 0
	for _, coll := range collections {
		tableName := NormalizeCollectionName(coll.Name)

		// Create table.
		ddl := CreateCollectionTableSQL(tableName)
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("creating table %s: %w", tableName, err)
		}

		// Create GIN index.
		indexSQL := CreateCollectionIndexSQL(tableName)
		if _, err := tx.ExecContext(ctx, indexSQL); err != nil {
			m.progress.Warn(fmt.Sprintf("creating index on %s: %v", tableName, err))
		}

		m.stats.Collections++

		// Insert documents.
		for _, doc := range coll.Documents {
			flatFields := FlattenFirestoreFields(doc.Fields)
			jsonData, err := json.Marshal(flatFields)
			if err != nil {
				m.stats.Errors = append(m.stats.Errors,
					fmt.Sprintf("marshaling document %s in %s: %v", doc.ID, tableName, err))
				processed++
				m.progress.Progress(phase, processed, totalDocs)
				continue
			}

			result, err := tx.ExecContext(ctx,
				fmt.Sprintf(`INSERT INTO %q ("id", "data") VALUES ($1, $2) ON CONFLICT ("id") DO NOTHING`, tableName),
				doc.ID, jsonData,
			)
			if err != nil {
				m.stats.Errors = append(m.stats.Errors,
					fmt.Sprintf("inserting document %s into %s: %v", doc.ID, tableName, err))
				processed++
				m.progress.Progress(phase, processed, totalDocs)
				continue
			}
			if n, _ := result.RowsAffected(); n > 0 {
				m.stats.Documents++
			}
			processed++
			m.progress.Progress(phase, processed, totalDocs)
		}

		if m.verbose {
			fmt.Fprintf(m.output, "  %s: %d documents\n", tableName, len(coll.Documents))
		}
	}

	m.progress.CompletePhase(phase, totalDocs, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d documents across %d collections\n", m.stats.Documents, m.stats.Collections)
	return nil
}

func (m *Migrator) printStats() {
	fmt.Fprintf(m.output, "\nSummary:\n")
	if m.stats.Users > 0 {
		fmt.Fprintf(m.output, "  Users:       %d\n", m.stats.Users)
	}
	if m.stats.OAuthLinks > 0 {
		fmt.Fprintf(m.output, "  OAuth:       %d\n", m.stats.OAuthLinks)
	}
	if m.stats.Collections > 0 {
		fmt.Fprintf(m.output, "  Collections: %d\n", m.stats.Collections)
	}
	if m.stats.Documents > 0 {
		fmt.Fprintf(m.output, "  Documents:   %d\n", m.stats.Documents)
	}
	if m.stats.RTDBNodes > 0 {
		fmt.Fprintf(m.output, "  RTDB nodes:  %d\n", m.stats.RTDBNodes)
	}
	if m.stats.RTDBRecords > 0 {
		fmt.Fprintf(m.output, "  RTDB records: %d\n", m.stats.RTDBRecords)
	}
	if m.stats.StorageFiles > 0 {
		fmt.Fprintf(m.output, "  Files:       %d (%s)\n", m.stats.StorageFiles, migrate.FormatBytes(m.stats.StorageBytes))
	}
	if m.stats.Skipped > 0 {
		fmt.Fprintf(m.output, "  Skipped:     %d\n", m.stats.Skipped)
	}
	if len(m.stats.Errors) > 0 {
		fmt.Fprintf(m.output, "  Errors:      %d\n", len(m.stats.Errors))
		for _, e := range m.stats.Errors {
			fmt.Fprintf(m.output, "    - %s\n", e)
		}
	}
}

// parseEpochMs converts a millisecond epoch string to time.Time.
func parseEpochMs(s string) time.Time {
	if s == "" {
		return time.Now()
	}
	var ms int64
	if _, err := fmt.Sscanf(s, "%d", &ms); err != nil {
		return time.Now()
	}
	return time.Unix(ms/1000, (ms%1000)*int64(time.Millisecond))
}
