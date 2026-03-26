package nhostmigrate

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/allyourbase/ayb/internal/migrate"
)

type migrationPlan struct {
	migrate.StatementPlan
	stats      MigrationStats
	fkKeys     map[string]struct{}
	rlsEnabled map[string]struct{}
}

// BuildValidationSummary compares source analysis with migration stats.
func BuildValidationSummary(report *migrate.AnalysisReport, stats *MigrationStats) *migrate.ValidationSummary {
	summary := &migrate.ValidationSummary{
		SourceLabel: nhostSourceLabel,
		TargetLabel: aybTargetLabel,
	}

	if report.Tables > 0 || stats.Tables > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{Label: "Tables", SourceCount: report.Tables, TargetCount: stats.Tables})
	}
	if report.Views > 0 || stats.Views > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{Label: "Views", SourceCount: report.Views, TargetCount: stats.Views})
	}
	if report.Records > 0 || stats.Records > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{Label: "Records", SourceCount: report.Records, TargetCount: stats.Records})
	}
	if report.RLSPolicies > 0 || stats.Policies > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{Label: "RLS policies", SourceCount: report.RLSPolicies, TargetCount: stats.Policies})
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

// buildPlan reads the pg_dump file, parses SQL statements, applies skipping rules, and overlays Hasura v3 metadata to construct a migration plan with execution statements and statistics for analysis.
func (m *Migrator) buildPlan(ctx context.Context) (*migrationPlan, *migrate.AnalysisReport, error) {
	_ = ctx
	plan := &migrationPlan{
		fkKeys:     make(map[string]struct{}),
		rlsEnabled: make(map[string]struct{}),
	}
	foreignKeysBySource := make(map[string]metadataForeignKey)
	report := &migrate.AnalysisReport{
		SourceType: "NHost",
		SourceInfo: fmt.Sprintf("metadata=%s dump=%s", m.opts.HasuraMetadataPath, m.opts.PgDumpPath),
	}

	dumpBytes, err := os.ReadFile(m.opts.PgDumpPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading pg_dump: %w", err)
	}
	for _, stmt := range splitSQLStatements(string(dumpBytes)) {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		kind := classifySQLStatement(stmt)
		if kind == "" {
			continue
		}
		if shouldSkipStatement(stmt) {
			plan.stats.Skipped++
			continue
		}
		plan.Add(kind, stmt)
		switch kind {
		case "create_table":
			plan.stats.Tables++
			report.Tables++
		case "create_view":
			plan.stats.Views++
			report.Views++
		case "create_index":
			plan.stats.Indexes++
		case "insert":
			rows := countInsertRows(stmt)
			plan.stats.Records += rows
			report.Records += rows
		case "foreign_key":
			plan.stats.ForeignKeys++
			if fk, ok := parseForeignKeyStatement(stmt); ok {
				plan.fkKeys[fk.Key()] = struct{}{}
				foreignKeysBySource[foreignKeySourceKey(fk.FromSchema, fk.FromTable, fk.FromColumn)] = fk
			}
		}
	}

	tableFiles, err := loadHasuraV3TableFiles(m.opts.HasuraMetadataPath)
	if err != nil {
		return nil, nil, err
	}

	for _, tf := range tableFiles {
		if shouldSkipQualifiedTable(tf.Table.Schema, tf.Table.Name) {
			plan.stats.Skipped++
			continue
		}
		for _, fk := range tf.ForeignKeys(func(fromSchema, fromTable, fromColumn string) (metadataForeignKey, bool) {
			fk, ok := foreignKeysBySource[foreignKeySourceKey(fromSchema, fromTable, fromColumn)]
			return fk, ok
		}) {
			if shouldSkipQualifiedTable(fk.FromSchema, fk.FromTable) || shouldSkipQualifiedTable(fk.ToSchema, fk.ToTable) {
				plan.stats.Skipped++
				continue
			}
			key := fk.Key()
			if _, exists := plan.fkKeys[key]; exists {
				continue
			}
			plan.fkKeys[key] = struct{}{}
			plan.Add("foreign_key", fk.SQL())
			plan.stats.ForeignKeys++
		}

		if m.opts.SkipRLS {
			continue
		}
		actions := tf.PermissionActions()
		if len(actions) == 0 {
			continue
		}
		tableKey := qualifiedTableKey(tf.Table.Schema, tf.Table.Name)
		if _, exists := plan.rlsEnabled[tableKey]; !exists {
			plan.rlsEnabled[tableKey] = struct{}{}
			plan.Add("enable_rls", fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY", migrate.QuoteQualifiedTable(tf.Table.Schema, tf.Table.Name)))
		}

		for _, p := range actions {
			policySQL := buildPolicySQL(tf.Table.Schema, tf.Table.Name, p.Role, p.Action)
			plan.Add("policy", policySQL)
			plan.stats.Policies++
			report.RLSPolicies++
		}
	}

	return plan, report, nil
}
