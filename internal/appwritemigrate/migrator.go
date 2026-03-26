// Package appwritemigrate implements migration of Appwrite database exports to PostgreSQL, including schema generation, data import, and row-level security policy creation.
package appwritemigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	appwriteSourceLabel = "Appwrite (source)"
	aybTargetLabel      = "AYB (target)"
)

// Migrator imports Appwrite database export data.
type Migrator struct {
	db       *sql.DB
	opts     MigrationOptions
	stats    MigrationStats
	output   io.Writer
	verbose  bool
	progress migrate.ProgressReporter
	beginTx  func(context.Context) (txLike, error)
}

type txLike interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	Commit() error
	Rollback() error
}

type migrationPlan struct {
	migrate.StatementPlan
	stats      MigrationStats
	rlsEnabled map[string]struct{}
}

// NewMigrator creates a new Appwrite migrator.
func NewMigrator(opts MigrationOptions) (*Migrator, error) {
	if strings.TrimSpace(opts.ExportPath) == "" {
		return nil, fmt.Errorf("export path is required")
	}
	if strings.TrimSpace(opts.DatabaseURL) == "" {
		return nil, fmt.Errorf("database URL is required")
	}
	fi, err := os.Stat(opts.ExportPath)
	if err != nil {
		return nil, fmt.Errorf("export path: %w", err)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("export path must be a file")
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

	m := &Migrator{db: db, opts: opts, output: output, verbose: opts.Verbose, progress: progress}
	m.beginTx = func(ctx context.Context) (txLike, error) {
		return m.db.BeginTx(ctx, nil)
	}
	return m, nil
}

// Close releases db resources.
func (m *Migrator) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// Analyze inspects Appwrite export and returns a summary report.
func (m *Migrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	_, report, err := m.buildPlan(ctx)
	if err != nil {
		return nil, err
	}
	return report, nil
}

// Migrate applies schema/data/policies in a transaction.
func (m *Migrator) Migrate(ctx context.Context) (*MigrationStats, error) {
	plan, _, err := m.buildPlan(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := m.beginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	phase := migrate.Phase{Name: "Replay SQL", Index: 1, Total: 1}
	m.progress.StartPhase(phase, len(plan.Statements))
	started := time.Now()

	for i, stmt := range plan.Statements {
		if _, err := tx.ExecContext(ctx, stmt.SQL); err != nil {
			plan.stats.Errors = append(plan.stats.Errors, err.Error())
			return nil, fmt.Errorf("executing %s statement %d: %w", stmt.Kind, i+1, err)
		}
		m.progress.Progress(phase, i+1, len(plan.Statements))
	}

	if m.opts.DryRun {
		if err := tx.Rollback(); err != nil {
			return nil, fmt.Errorf("rolling back dry-run: %w", err)
		}
	} else {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("committing transaction: %w", err)
		}
	}

	m.progress.CompletePhase(phase, len(plan.Statements), time.Since(started))
	m.stats = plan.stats
	return &m.stats, nil
}

// BuildValidationSummary compares source counts and migrated counts.
func BuildValidationSummary(report *migrate.AnalysisReport, stats *MigrationStats) *migrate.ValidationSummary {
	summary := &migrate.ValidationSummary{
		SourceLabel: appwriteSourceLabel,
		TargetLabel: aybTargetLabel,
		Rows: []migrate.ValidationRow{
			{Label: "Collections", SourceCount: report.Tables, TargetCount: stats.Collections},
			{Label: "Documents", SourceCount: report.Records, TargetCount: stats.Documents},
			{Label: "RLS policies", SourceCount: report.RLSPolicies, TargetCount: stats.Policies},
		},
	}

	for _, row := range summary.Rows {
		if row.SourceCount != row.TargetCount {
			summary.Warnings = append(summary.Warnings,
				fmt.Sprintf("%s count mismatch: source=%d target=%d", row.Label, row.SourceCount, row.TargetCount))
		}
	}
	if stats.Skipped > 0 {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("%d items skipped during migration", stats.Skipped))
	}
	if len(stats.Errors) > 0 {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("%d errors occurred during migration", len(stats.Errors)))
	}
	return summary
}

type appwriteExport struct {
	Collections []appwriteCollection `json:"collections"`
}

type appwriteCollection struct {
	Name        string              `json:"name"`
	Attributes  []appwriteAttribute `json:"attributes"`
	Indexes     []appwriteIndex     `json:"indexes"`
	Permissions appwritePermissions `json:"permissions"`
	Documents   []map[string]any    `json:"documents"`
}

type appwriteAttribute struct {
	Key               string   `json:"key"`
	Type              string   `json:"type"`
	Required          bool     `json:"required"`
	Elements          []string `json:"elements"`
	RelatedCollection string   `json:"related_collection"`
}

type appwriteIndex struct {
	Key        string   `json:"key"`
	Attributes []string `json:"attributes"`
	Type       string   `json:"type"`
}

type appwritePermissions struct {
	Read   []string `json:"read"`
	Create []string `json:"create"`
	Update []string `json:"update"`
	Delete []string `json:"delete"`
}

// Reads and parses the Appwrite export file to build a complete migration plan including CREATE TABLE statements with all columns and constraints, foreign key relationships, indexes, INSERT statements for all documents, and row-level security policies. Returns the migration plan, analysis report with table and record counts, and any parsing error.
func (m *Migrator) buildPlan(ctx context.Context) (*migrationPlan, *migrate.AnalysisReport, error) {
	_ = ctx
	payload, err := os.ReadFile(m.opts.ExportPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading export: %w", err)
	}

	var export appwriteExport
	if err := json.Unmarshal(payload, &export); err != nil {
		return nil, nil, fmt.Errorf("parsing export: %w", err)
	}

	plan := &migrationPlan{rlsEnabled: make(map[string]struct{})}
	report := &migrate.AnalysisReport{SourceType: "Appwrite", SourceInfo: m.opts.ExportPath}

	for _, coll := range export.Collections {
		if coll.Name == "" {
			plan.stats.Skipped++
			continue
		}
		report.Tables++
		plan.stats.Collections++
		plan.stats.Attributes += len(coll.Attributes)

		plan.Add("create_table", buildCreateTableSQL(coll))

		for _, attr := range coll.Attributes {
			if strings.EqualFold(attr.Type, "relationship") && attr.RelatedCollection != "" {
				constraint := migrate.SanitizeIdentifier(fmt.Sprintf("fk_%s_%s_%s", coll.Name, attr.Key, attr.RelatedCollection))
				plan.Add("foreign_key", fmt.Sprintf(
					"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(%s)",
					migrate.QuoteQualifiedTable("public", coll.Name),
					migrate.QuoteIdent(constraint),
					migrate.QuoteIdent(attr.Key),
					migrate.QuoteQualifiedTable("public", attr.RelatedCollection),
					migrate.QuoteIdent("id"),
				))
			}
		}

		for _, idx := range coll.Indexes {
			if idx.Key == "" || len(idx.Attributes) == 0 {
				plan.stats.Skipped++
				continue
			}
			cols := make([]string, 0, len(idx.Attributes))
			for _, c := range idx.Attributes {
				cols = append(cols, migrate.QuoteIdent(c))
			}
			plan.Add("index", fmt.Sprintf("CREATE INDEX %s ON %s (%s)", migrate.QuoteIdent(migrate.SanitizeIdentifier(idx.Key)), migrate.QuoteQualifiedTable("public", coll.Name), strings.Join(cols, ", ")))
			plan.stats.Indexes++
		}

		if !m.opts.SkipData {
			for _, doc := range coll.Documents {
				stmt, ok := buildInsertSQL(coll, doc)
				if !ok {
					plan.stats.Skipped++
					continue
				}
				plan.Add("insert", stmt)
				plan.stats.Documents++
				report.Records++
			}
		}

		if !m.opts.SkipRLS {
			for action, roles := range map[string][]string{
				"SELECT": coll.Permissions.Read,
				"INSERT": coll.Permissions.Create,
				"UPDATE": coll.Permissions.Update,
				"DELETE": coll.Permissions.Delete,
			} {
				if !containsRole(roles, "user:*") {
					continue
				}
				if _, exists := plan.rlsEnabled[coll.Name]; !exists {
					plan.rlsEnabled[coll.Name] = struct{}{}
					plan.Add("enable_rls", fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY", migrate.QuoteQualifiedTable("public", coll.Name)))
				}
				plan.Add("policy", buildPolicySQL("public", coll.Name, "authenticated", action))
				plan.stats.Policies++
				report.RLSPolicies++
			}
		}
	}

	return plan, report, nil
}

func containsRole(roles []string, want string) bool {
	for _, role := range roles {
		if role == want {
			return true
		}
	}
	return false
}

// Generates a CREATE TABLE statement for an Appwrite collection, ensuring an id primary key exists, applying correct column types and NOT NULL constraints, and adding CHECK constraints for email format validation and enum value restrictions.
func buildCreateTableSQL(coll appwriteCollection) string {
	if len(coll.Attributes) == 0 {
		return fmt.Sprintf("CREATE TABLE %s (%s)", migrate.QuoteQualifiedTable("public", coll.Name), migrate.QuoteIdent("id")+" text PRIMARY KEY")
	}

	sorted := append([]appwriteAttribute(nil), coll.Attributes...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })

	cols := make([]string, 0, len(sorted)+1)
	hasID := false
	for _, attr := range sorted {
		if strings.EqualFold(attr.Key, "id") {
			hasID = true
			break
		}
	}
	if !hasID {
		cols = append(cols, migrate.QuoteIdent("id")+" text PRIMARY KEY")
	}

	for _, attr := range sorted {
		col := fmt.Sprintf("%s %s", migrate.QuoteIdent(attr.Key), mapAppwriteType(attr.Type))
		if attr.Required {
			col += " NOT NULL"
		}
		if strings.EqualFold(attr.Key, "id") {
			col += " PRIMARY KEY"
		}
		if strings.EqualFold(attr.Type, "email") {
			col += " CHECK (position('@' in " + migrate.QuoteIdent(attr.Key) + ") > 1)"
		}
		if strings.EqualFold(attr.Type, "enum") && len(attr.Elements) > 0 {
			lits := make([]string, 0, len(attr.Elements))
			for _, elem := range attr.Elements {
				lits = append(lits, migrate.SQLString(elem))
			}
			col += " CHECK (" + migrate.QuoteIdent(attr.Key) + " IN (" + strings.Join(lits, ", ") + "))"
		}
		cols = append(cols, col)
	}

	return fmt.Sprintf("CREATE TABLE %s (%s)", migrate.QuoteQualifiedTable("public", coll.Name), strings.Join(cols, ", "))
}

// Maps an Appwrite attribute type string to the corresponding PostgreSQL type name, defaulting to text for unrecognized types.
func mapAppwriteType(typ string) string {
	switch strings.ToLower(typ) {
	case "string", "email", "enum", "url":
		return "text"
	case "integer":
		return "bigint"
	case "float":
		return "double precision"
	case "boolean":
		return "boolean"
	case "datetime":
		return "timestamptz"
	case "relationship":
		return "text"
	case "ip":
		return "inet"
	default:
		return "text"
	}
}

// Converts an Appwrite document to a PostgreSQL INSERT statement, mapping the special $id field to the id column and including only attributes defined in the collection schema. Returns the SQL statement and a boolean; false if the document contains no valid columns.
func buildInsertSQL(coll appwriteCollection, doc map[string]any) (string, bool) {
	if len(doc) == 0 {
		return "", false
	}
	validCols := make(map[string]struct{}, len(coll.Attributes))
	for _, attr := range coll.Attributes {
		validCols[attr.Key] = struct{}{}
	}

	columns := make([]string, 0)
	values := make([]string, 0)
	keys := make([]string, 0)
	for k := range doc {
		if k == "$id" {
			keys = append(keys, "$id")
			continue
		}
		if _, ok := validCols[k]; !ok {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if key == "$id" {
			columns = append(columns, migrate.QuoteIdent("id"))
			values = append(values, sqlLiteral(doc[key]))
			continue
		}
		columns = append(columns, migrate.QuoteIdent(key))
		values = append(values, sqlLiteral(doc[key]))
	}
	if len(columns) == 0 {
		return "", false
	}

	return fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		migrate.QuoteQualifiedTable("public", coll.Name),
		strings.Join(columns, ", "),
		strings.Join(values, ", "),
	), true
}

// Converts a Go value to a PostgreSQL SQL literal representation, handling nil, booleans, integers, floats, JSON numbers, strings with proper escaping, and complex types via JSON marshaling.
func sqlLiteral(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	case string:
		return migrate.SQLString(x)
	default:
		b, _ := json.Marshal(x)
		return migrate.SQLString(string(b))
	}
}

func buildPolicySQL(schema, table, role, action string) string {
	return migrate.BuildAllowPolicySQL(schema, table, role, action)
}
