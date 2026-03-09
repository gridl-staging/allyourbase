package pbmigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
)

// Migrator orchestrates the migration process
type Migrator struct {
	reader   *Reader
	db       *sql.DB
	opts     MigrationOptions
	stats    MigrationStats
	output   io.Writer
	verbose  bool
	progress migrate.ProgressReporter
}

// NewMigrator creates a new migrator
func NewMigrator(opts MigrationOptions) (*Migrator, error) {
	// Validate options
	if opts.SourcePath == "" {
		return nil, fmt.Errorf("source path is required")
	}

	if opts.DatabaseURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	// Create reader
	reader, err := NewReader(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create reader: %w", err)
	}

	// Connect to PostgreSQL
	db, err := sql.Open("pgx", opts.DatabaseURL)
	if err != nil {
		reader.Close()
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		reader.Close()
		db.Close()
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	var output io.Writer = os.Stdout
	if opts.DryRun && !opts.Verbose {
		output = io.Discard
	}

	progress := opts.Progress
	if progress == nil {
		progress = migrate.NopReporter{}
	}

	return &Migrator{
		reader:   reader,
		db:       db,
		opts:     opts,
		output:   output,
		verbose:  opts.Verbose,
		progress: progress,
	}, nil
}

// Close closes database connections
func (m *Migrator) Close() error {
	if m.reader != nil {
		m.reader.Close()
	}
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// Migrate performs the full migration with progress reporting.
func (m *Migrator) Migrate(ctx context.Context) (*MigrationStats, error) {
	start := time.Now()
	fmt.Fprintln(m.output, "Starting PocketBase migration...")

	// Read collections
	collections, err := m.reader.ReadCollections()
	if err != nil {
		return nil, fmt.Errorf("failed to read collections: %w", err)
	}

	fmt.Fprintf(m.output, "Found %d collections\n\n", len(collections))
	m.stats.Collections = len(collections)

	// Count phases for progress reporting
	totalPhases := 4 // schema, data, auth, RLS
	if !m.opts.SkipFiles && !m.opts.DryRun {
		totalPhases = 5
	}
	phaseIdx := 0

	// Start transaction
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Phase 1: Schema
	phaseIdx++
	phase := migrate.Phase{Name: "Schema", Index: phaseIdx, Total: totalPhases}
	phaseStart := time.Now()
	schemaCount := countSchemaTables(collections)
	m.progress.StartPhase(phase, schemaCount)
	if err := m.migrateSchema(ctx, tx, collections, phase); err != nil {
		return nil, fmt.Errorf("schema migration failed: %w", err)
	}
	m.progress.CompletePhase(phase, m.stats.Tables+m.stats.Views, time.Since(phaseStart))

	// Phase 2: Data
	phaseIdx++
	phase = migrate.Phase{Name: "Data", Index: phaseIdx, Total: totalPhases}
	phaseStart = time.Now()
	totalRecords := countTotalRecords(m.reader, collections)
	m.progress.StartPhase(phase, totalRecords)
	if err := m.migrateData(ctx, tx, collections, phase); err != nil {
		return nil, fmt.Errorf("data migration failed: %w", err)
	}
	m.progress.CompletePhase(phase, m.stats.Records, time.Since(phaseStart))

	// Phase 3: Auth users
	phaseIdx++
	phase = migrate.Phase{Name: "Auth users", Index: phaseIdx, Total: totalPhases}
	phaseStart = time.Now()
	authCount := countAuthUsers(m.reader, collections)
	m.progress.StartPhase(phase, authCount)
	if err := m.migrateAuthUsers(ctx, tx, collections); err != nil {
		return nil, fmt.Errorf("auth migration failed: %w", err)
	}
	m.progress.CompletePhase(phase, authCount, time.Since(phaseStart))

	// Phase 4: RLS policies
	phaseIdx++
	phase = migrate.Phase{Name: "RLS policies", Index: phaseIdx, Total: totalPhases}
	phaseStart = time.Now()
	m.progress.StartPhase(phase, 0)
	if err := m.migrateRLS(ctx, tx, collections); err != nil {
		return nil, fmt.Errorf("RLS migration failed: %w", err)
	}
	m.progress.CompletePhase(phase, m.stats.Policies, time.Since(phaseStart))

	// Commit transaction
	if m.opts.DryRun {
		fmt.Fprintln(m.output, "\n[DRY RUN] Rolling back transaction (no changes made)")
	} else {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	// Phase 5: Files (outside transaction)
	if !m.opts.SkipFiles && !m.opts.DryRun {
		phaseIdx++
		phase = migrate.Phase{Name: "Storage files", Index: phaseIdx, Total: totalPhases}
		phaseStart = time.Now()
		m.progress.StartPhase(phase, 0)
		if err := m.migrateFiles(ctx, collections); err != nil {
			return nil, fmt.Errorf("file migration failed: %w", err)
		}
		m.progress.CompletePhase(phase, m.stats.Files, time.Since(phaseStart))
	}

	elapsed := time.Since(start)
	fmt.Fprintf(m.output, "\nMigration complete in %s\n", formatElapsed(elapsed))

	return &m.stats, nil
}

// migrateSchema creates tables and views
func (m *Migrator) migrateSchema(ctx context.Context, tx *sql.Tx, collections []PBCollection, phase migrate.Phase) error {
	completed := 0
	for _, coll := range collections {
		// Skip system collections (they're internal to PocketBase)
		if coll.System {
			if m.verbose {
				fmt.Fprintf(m.output, "  - Skipping system collection: %s\n", coll.Name)
			}
			continue
		}

		// Skip auth collections (handled separately)
		if coll.Type == "auth" {
			if m.verbose {
				fmt.Fprintf(m.output, "  - Skipping auth collection (will migrate to ayb_auth_users): %s\n", coll.Name)
			}
			continue
		}

		var sqlStmt string
		var typeName string

		if coll.Type == "view" {
			sqlStmt = BuildCreateViewSQL(coll)
			typeName = "view"
			m.stats.Views++
		} else {
			sqlStmt = BuildCreateTableSQL(coll)
			typeName = "table"
			m.stats.Tables++
		}

		if m.verbose {
			fmt.Fprintf(m.output, "  + %s (%s)\n", coll.Name, typeName)
		}

		if !m.opts.DryRun {
			if _, err := tx.ExecContext(ctx, sqlStmt); err != nil {
				return fmt.Errorf("failed to create %s %s: %w", typeName, coll.Name, err)
			}
		}

		completed++
		m.progress.Progress(phase, completed, completed)
	}

	return nil
}

// migrateData imports records
func (m *Migrator) migrateData(ctx context.Context, tx *sql.Tx, collections []PBCollection, phase migrate.Phase) error {
	totalCompleted := 0

	for _, coll := range collections {
		// Skip system, auth, and view collections
		if coll.System || coll.Type == "auth" || coll.Type == "view" {
			continue
		}

		count, err := m.reader.CountRecords(coll.Name)
		if err != nil {
			return fmt.Errorf("failed to count records in %s: %w", coll.Name, err)
		}

		if count == 0 {
			if m.verbose {
				fmt.Fprintf(m.output, "  %s: 0 records (skipping)\n", coll.Name)
			}
			continue
		}

		// Read records
		records, err := m.reader.ReadRecords(coll.Name, coll.Schema)
		if err != nil {
			return fmt.Errorf("failed to read records from %s: %w", coll.Name, err)
		}

		// Insert records in batches
		batchSize := 1000
		for i := 0; i < len(records); i += batchSize {
			end := i + batchSize
			if end > len(records) {
				end = len(records)
			}

			batch := records[i:end]

			if !m.opts.DryRun {
				if err := m.insertBatch(ctx, tx, coll.Name, coll.Schema, batch); err != nil {
					return fmt.Errorf("failed to insert batch into %s: %w", coll.Name, err)
				}
			}

			m.stats.Records += len(batch)
			totalCompleted += len(batch)
			m.progress.Progress(phase, totalCompleted, totalCompleted)
		}
	}

	return nil
}

// insertBatch inserts a batch of records
func (m *Migrator) insertBatch(ctx context.Context, tx *sql.Tx, tableName string, schema []PBField, records []PBRecord) error {
	if len(records) == 0 {
		return nil
	}

	// Build column list
	columns := []string{"id"}
	if _, ok := records[0].Data["created"]; ok {
		columns = append(columns, "created")
	}
	if _, ok := records[0].Data["updated"]; ok {
		columns = append(columns, "updated")
	}
	for _, field := range schema {
		if !field.System {
			alreadyIncluded := false
			for _, col := range columns {
				if col == field.Name {
					alreadyIncluded = true
					break
				}
			}
			if !alreadyIncluded {
				columns = append(columns, field.Name)
			}
		}
	}

	// Build field type lookup for SQLite -> PostgreSQL type coercion
	fieldTypes := make(map[string]string)
	fieldsByName := make(map[string]PBField)
	for _, field := range schema {
		fieldTypes[field.Name] = field.Type
		fieldsByName[field.Name] = field
	}

	// Build INSERT statement for batch
	for _, record := range records {
		// Build placeholders and values
		placeholders := make([]string, len(columns))
		values := make([]interface{}, len(columns))

		for i, col := range columns {
			placeholders[i] = fmt.Sprintf("$%d", i+1)

			switch col {
			case "id":
				values[i] = record.ID
			case "created", "updated":
				values[i] = record.Data[col]
			default:
				val := record.Data[col]
				// Convert SQLite INTEGER booleans (1/0) to Go bool for PostgreSQL BOOLEAN columns
				if fieldTypes[col] == "bool" {
					val = coerceToBool(val)
				}
				if field, ok := fieldsByName[col]; ok && fieldExpectsTextArray(field) {
					val = coerceToTextArray(val)
				}
				values[i] = val
			}
		}

		// Execute INSERT
		query := fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES (%s)",
			SanitizeIdentifier(tableName),
			joinQuoted(columns),
			join(placeholders, ", "),
		)

		if _, err := tx.ExecContext(ctx, query, values...); err != nil {
			return fmt.Errorf("failed to insert record %s: %w", record.ID, err)
		}
	}

	return nil
}

// coerceToBool converts SQLite INTEGER values (1/0) to Go bool for PostgreSQL BOOLEAN columns.
// SQLite stores booleans as INTEGER; pgx doesn't auto-convert int64 -> bool.
func coerceToBool(val interface{}) interface{} {
	switch v := val.(type) {
	case bool:
		return v
	case int64:
		return v != 0
	case int:
		return v != 0
	case float64:
		return v != 0
	default:
		return val
	}
}

func fieldExpectsTextArray(field PBField) bool {
	switch field.Type {
	case "select", "file", "relation":
		return fieldMaxSelect(field) > 1
	default:
		return false
	}
}

func coerceToTextArray(val interface{}) interface{} {
	switch v := val.(type) {
	case nil:
		return []string{}
	case string:
		if v == "" {
			return []string{}
		}
		var arr []string
		if err := json.Unmarshal([]byte(v), &arr); err == nil {
			return arr
		}
		return []string{v}
	case []byte:
		s := string(v)
		if s == "" {
			return []string{}
		}
		var arr []string
		if err := json.Unmarshal(v, &arr); err == nil {
			return arr
		}
		return []string{s}
	case []string:
		return v
	case []interface{}:
		arr := make([]string, 0, len(v))
		for _, item := range v {
			arr = append(arr, fmt.Sprint(item))
		}
		return arr
	default:
		return val
	}
}

// migrateRLS creates RLS policies
func (m *Migrator) migrateRLS(ctx context.Context, tx *sql.Tx, collections []PBCollection) error {
	for _, coll := range collections {
		// Skip system, auth, and view collections
		if coll.System || coll.Type == "auth" || coll.Type == "view" {
			continue
		}

		// Generate policies
		policies, err := GenerateRLSPolicies(coll)
		if err != nil {
			return fmt.Errorf("failed to generate policies for %s: %w", coll.Name, err)
		}

		if len(policies) == 0 {
			continue
		}

		// Enable RLS
		enableSQL := EnableRLS(coll.Name)
		if !m.opts.DryRun {
			if _, err := tx.ExecContext(ctx, enableSQL); err != nil {
				return fmt.Errorf("failed to enable RLS on %s: %w", coll.Name, err)
			}
		}

		// Create policies
		for _, policy := range policies {
			if !m.opts.DryRun {
				if _, err := tx.ExecContext(ctx, policy); err != nil {
					return fmt.Errorf("failed to create policy on %s: %w", coll.Name, err)
				}
			}
			m.stats.Policies++
		}

		if m.verbose {
			fmt.Fprintf(m.output, "  + %s: %d policies\n", coll.Name, len(policies))
		}
	}

	return nil
}

// printStats prints migration statistics
func (m *Migrator) printStats() {
	fmt.Fprintf(m.output, "  Collections: %d\n", m.stats.Collections)
	fmt.Fprintf(m.output, "  Tables: %d\n", m.stats.Tables)
	fmt.Fprintf(m.output, "  Views: %d\n", m.stats.Views)
	fmt.Fprintf(m.output, "  Records: %d\n", m.stats.Records)
	fmt.Fprintf(m.output, "  Policies: %d\n", m.stats.Policies)
	if !m.opts.SkipFiles {
		fmt.Fprintf(m.output, "  Files: %d\n", m.stats.Files)
	}
}

// BuildValidationSummary constructs a post-migration validation summary
// by comparing the analyzed source counts against the actual migration stats.
func BuildValidationSummary(report *migrate.AnalysisReport, stats *MigrationStats) *migrate.ValidationSummary {
	return &migrate.ValidationSummary{
		SourceLabel: fmt.Sprintf("Source (%s)", report.SourceType),
		TargetLabel: "Target (AYB)",
		Rows: []migrate.ValidationRow{
			{Label: "Tables", SourceCount: report.Tables, TargetCount: stats.Tables},
			{Label: "Views", SourceCount: report.Views, TargetCount: stats.Views},
			{Label: "Records", SourceCount: report.Records, TargetCount: stats.Records},
			{Label: "Auth users", SourceCount: report.AuthUsers, TargetCount: countUserStats(stats)},
			{Label: "RLS policies", SourceCount: report.RLSPolicies, TargetCount: stats.Policies},
			{Label: "Files", SourceCount: report.Files, TargetCount: stats.Files},
		},
	}
}

func countUserStats(stats *MigrationStats) int {
	// Auth users are counted in Records for the existing migrator.
	// We need a separate field. For now, return 0 — we'll refine this
	// when we add AuthUsers to MigrationStats.
	return stats.AuthUsers
}

// Helper functions

func countSchemaTables(collections []PBCollection) int {
	count := 0
	for _, coll := range collections {
		if !coll.System && coll.Type != "auth" {
			count++
		}
	}
	return count
}

func countTotalRecords(reader *Reader, collections []PBCollection) int {
	total := 0
	for _, coll := range collections {
		if coll.System || coll.Type == "auth" || coll.Type == "view" {
			continue
		}
		count, err := reader.CountRecords(coll.Name)
		if err == nil {
			total += count
		}
	}
	return total
}

func countAuthUsers(reader *Reader, collections []PBCollection) int {
	total := 0
	for _, coll := range collections {
		if coll.Type == "auth" && !coll.System {
			count, err := reader.CountRecords(coll.Name)
			if err == nil {
				total += count
			}
		}
	}
	return total
}

func formatElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func joinQuoted(strs []string) string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = SanitizeIdentifier(s)
	}
	return join(quoted, ", ")
}

func join(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
