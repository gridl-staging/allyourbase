// Package directusmigrate Migrator handles schema migration from Directus snapshot exports into AYB PostgreSQL databases, creating tables, foreign key relations, and row-level security policies.
package directusmigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	directusSourceLabel = "Directus (source)"
	aybTargetLabel      = "AYB (target)"
)

// Migrator imports Directus snapshot schema into AYB.
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

// NewMigrator creates a new Directus migrator.
func NewMigrator(opts MigrationOptions) (*Migrator, error) {
	if strings.TrimSpace(opts.SnapshotPath) == "" {
		return nil, fmt.Errorf("snapshot path is required")
	}
	if strings.TrimSpace(opts.DatabaseURL) == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	fi, err := os.Stat(opts.SnapshotPath)
	if err != nil {
		return nil, fmt.Errorf("snapshot path: %w", err)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("snapshot path must be a file")
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

	m := &Migrator{
		db:       db,
		opts:     opts,
		output:   output,
		verbose:  opts.Verbose,
		progress: progress,
	}
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

// Analyze inspects the snapshot and returns an analysis report.
func (m *Migrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	_, report, err := m.buildPlan(ctx)
	if err != nil {
		return nil, err
	}
	return report, nil
}

// Migrate applies schema + relations + policies in a transaction.
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
		SourceLabel: directusSourceLabel,
		TargetLabel: aybTargetLabel,
		Rows: []migrate.ValidationRow{
			{Label: "Collections", SourceCount: report.Tables, TargetCount: stats.Collections},
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

type directusSnapshot struct {
	Collections []directusCollection `json:"collections"`
	Fields      []directusField      `json:"fields"`
	Relations   []directusRelation   `json:"relations"`
	Permissions []directusPermission `json:"permissions"`
}

type directusCollection struct {
	Collection string `json:"collection"`
}

type directusField struct {
	Collection string              `json:"collection"`
	Field      string              `json:"field"`
	Type       string              `json:"type"`
	Schema     directusFieldSchema `json:"schema"`
}

type directusFieldSchema struct {
	IsNullable   bool `json:"is_nullable"`
	IsPrimaryKey bool `json:"is_primary_key"`
}

type directusRelation struct {
	Collection        string `json:"collection"`
	Field             string `json:"field"`
	RelatedCollection string `json:"related_collection"`
	Meta              struct {
		OneField string `json:"one_field"`
	} `json:"meta"`
	Schema struct {
		ForeignKeyColumn string `json:"foreign_key_column"`
	} `json:"schema"`
}

type directusPermission struct {
	Role       string `json:"role"`
	Collection string `json:"collection"`
	Action     string `json:"action"`
}

// buildPlan parses the Directus snapshot file and generates SQL statements to create tables, foreign key relations, and row-level security policies. It returns a migration plan containing all statements, an analysis report of schema objects found, and tracks statistics including collections, fields, relations, policies, and skipped items.
func (m *Migrator) buildPlan(ctx context.Context) (*migrationPlan, *migrate.AnalysisReport, error) {
	_ = ctx
	payload, err := os.ReadFile(m.opts.SnapshotPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading snapshot: %w", err)
	}

	var snapshot directusSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return nil, nil, fmt.Errorf("parsing snapshot: %w", err)
	}

	plan := &migrationPlan{rlsEnabled: make(map[string]struct{})}
	report := &migrate.AnalysisReport{SourceType: "Directus", SourceInfo: m.opts.SnapshotPath}

	fieldsByCollection := make(map[string][]directusField)
	for _, f := range snapshot.Fields {
		if shouldSkipCollection(f.Collection) {
			plan.stats.Skipped++
			continue
		}
		fieldsByCollection[f.Collection] = append(fieldsByCollection[f.Collection], f)
	}

	collections := make([]string, 0)
	for _, coll := range snapshot.Collections {
		if shouldSkipCollection(coll.Collection) {
			plan.stats.Skipped++
			continue
		}
		collections = append(collections, coll.Collection)
	}
	sort.Strings(collections)

	for _, coll := range collections {
		tableSQL := buildCreateTableSQL(coll, fieldsByCollection[coll])
		plan.Add("create_table", tableSQL)
		plan.stats.Collections++
		report.Tables++
		plan.stats.Fields += len(fieldsByCollection[coll])
	}

	for _, rel := range snapshot.Relations {
		if shouldSkipCollection(rel.Collection) || shouldSkipCollection(rel.RelatedCollection) {
			plan.stats.Skipped++
			continue
		}
		if strings.TrimSpace(rel.Field) == "" || strings.TrimSpace(rel.Collection) == "" || strings.TrimSpace(rel.RelatedCollection) == "" {
			plan.stats.Skipped++
			continue
		}
		refColumn := strings.TrimSpace(rel.Schema.ForeignKeyColumn)
		if refColumn == "" {
			refColumn = relatedPrimaryKey(fieldsByCollection[rel.RelatedCollection])
		}
		if refColumn == "" {
			refColumn = "id"
		}
		constraint := migrate.SanitizeIdentifier(fmt.Sprintf("fk_%s_%s_%s", rel.Collection, rel.Field, rel.RelatedCollection))
		stmt := fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(%s)",
			migrate.QuoteQualifiedTable("public", rel.Collection),
			migrate.QuoteIdent(constraint),
			migrate.QuoteIdent(rel.Field),
			migrate.QuoteQualifiedTable("public", rel.RelatedCollection),
			migrate.QuoteIdent(refColumn),
		)
		plan.Add("relation", stmt)
		plan.stats.Relations++
	}

	if !m.opts.SkipRLS {
		for _, perm := range snapshot.Permissions {
			if shouldSkipCollection(perm.Collection) {
				plan.stats.Skipped++
				continue
			}
			action := mapPermissionAction(perm.Action)
			if action == "" {
				plan.stats.Skipped++
				continue
			}
			tableKey := perm.Collection
			if _, exists := plan.rlsEnabled[tableKey]; !exists {
				plan.rlsEnabled[tableKey] = struct{}{}
				plan.Add("enable_rls", fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY", migrate.QuoteQualifiedTable("public", perm.Collection)))
			}
			plan.Add("policy", buildPolicySQL("public", perm.Collection, perm.Role, action))
			plan.stats.Policies++
			report.RLSPolicies++
		}
	}

	return plan, report, nil
}

func shouldSkipCollection(collection string) bool {
	return strings.HasPrefix(strings.ToLower(collection), "directus_")
}

func relatedPrimaryKey(fields []directusField) string {
	for _, field := range fields {
		if field.Schema.IsPrimaryKey && strings.TrimSpace(field.Field) != "" {
			return field.Field
		}
	}
	return ""
}

// mapDirectusType converts Directus field type names to their PostgreSQL equivalents, returning text as the default for unrecognized types.
func mapDirectusType(typ string) string {
	switch strings.ToLower(typ) {
	case "string", "csv", "hash":
		return "text"
	case "integer":
		return "bigint"
	case "float":
		return "double precision"
	case "boolean":
		return "boolean"
	case "datetime":
		return "timestamptz"
	case "json":
		return "jsonb"
	case "uuid":
		return "uuid"
	case "geometry":
		return "geometry"
	default:
		return "text"
	}
}

// buildCreateTableSQL generates a CREATE TABLE statement for the given collection, mapping Directus fields to PostgreSQL columns with appropriate types and constraints such as NOT NULL and PRIMARY KEY.
func buildCreateTableSQL(collection string, fields []directusField) string {
	if len(fields) == 0 {
		return fmt.Sprintf("CREATE TABLE %s (%s)", migrate.QuoteQualifiedTable("public", collection), migrate.QuoteIdent("id")+" bigserial PRIMARY KEY")
	}
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].Field < fields[j].Field })

	cols := make([]string, 0, len(fields))
	for _, f := range fields {
		col := fmt.Sprintf("%s %s", migrate.QuoteIdent(f.Field), mapDirectusType(f.Type))
		if !f.Schema.IsNullable {
			col += " NOT NULL"
		}
		if f.Schema.IsPrimaryKey {
			col += " PRIMARY KEY"
		}
		cols = append(cols, col)
	}

	return fmt.Sprintf("CREATE TABLE %s (%s)", migrate.QuoteQualifiedTable("public", collection), strings.Join(cols, ", "))
}

func mapPermissionAction(action string) string {
	switch strings.ToLower(action) {
	case "read":
		return "SELECT"
	case "create":
		return "INSERT"
	case "update":
		return "UPDATE"
	case "delete":
		return "DELETE"
	default:
		return ""
	}
}

func buildPolicySQL(schema, table, role, action string) string {
	return migrate.BuildAllowPolicySQL(schema, table, role, action)
}
