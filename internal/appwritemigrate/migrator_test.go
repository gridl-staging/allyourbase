package appwritemigrate

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNewMigratorRejectsMissingExportPath(t *testing.T) {
	t.Parallel()

	_, err := NewMigrator(MigrationOptions{DatabaseURL: "postgres://localhost/test"})
	testutil.ErrorContains(t, err, "export path is required")
}

func TestBuildPlanFromFixture(t *testing.T) {
	t.Parallel()

	m := &Migrator{opts: MigrationOptions{
		ExportPath: filepath.Join("testdata", "export.json"),
		SkipRLS:    false,
		SkipData:   false,
	}, progress: migrate.NopReporter{}}

	plan, report, err := m.buildPlan(context.Background())
	testutil.NoError(t, err)

	testutil.Equal(t, 2, report.Tables)
	testutil.Equal(t, 2, plan.stats.Collections)
	testutil.Equal(t, 12, plan.stats.Attributes)
	testutil.Equal(t, 2, plan.stats.Indexes)
	testutil.Equal(t, 2, plan.stats.Documents)
	testutil.Equal(t, 2, plan.stats.Policies)

	allSQL := plan.JoinedSQL()
	testutil.Contains(t, allSQL, "CREATE TABLE \"public\".\"authors\"")
	testutil.Contains(t, allSQL, "CREATE TABLE \"public\".\"posts\"")
	testutil.Contains(t, allSQL, "\"author_id\" text")
	testutil.Contains(t, allSQL, "CREATE INDEX")
	testutil.Contains(t, allSQL, "INSERT INTO")
	testutil.Contains(t, allSQL, "CREATE POLICY")
	testutil.Contains(t, allSQL, "CHECK")
}

func TestAppwriteTypeMappingCoversRequiredTypes(t *testing.T) {
	t.Parallel()

	types := map[string]string{
		"string":       "text",
		"integer":      "bigint",
		"float":        "double precision",
		"boolean":      "boolean",
		"email":        "text",
		"enum":         "text",
		"datetime":     "timestamptz",
		"relationship": "text",
		"ip":           "inet",
		"url":          "text",
	}

	for input, expected := range types {
		if got := mapAppwriteType(input); got != expected {
			t.Fatalf("type %s => %s, want %s", input, got, expected)
		}
	}
}

func TestBuildValidationSummary(t *testing.T) {
	t.Parallel()

	report := &migrate.AnalysisReport{Tables: 2, Records: 2, RLSPolicies: 1}
	stats := &MigrationStats{Collections: 2, Documents: 2, Policies: 1}

	summary := BuildValidationSummary(report, stats)
	testutil.Equal(t, "Appwrite (source)", summary.SourceLabel)
	testutil.Equal(t, "AYB (target)", summary.TargetLabel)
	testutil.Equal(t, 3, len(summary.Rows))
}

func TestBuildCreateTableSQLAddsImplicitIDColumn(t *testing.T) {
	t.Parallel()

	coll := appwriteCollection{
		Name: "notes",
		Attributes: []appwriteAttribute{
			{Key: "title", Type: "string", Required: true},
		},
	}

	sql := buildCreateTableSQL(coll)
	testutil.Contains(t, sql, `"id" text PRIMARY KEY`)
	testutil.Contains(t, sql, `"title" text NOT NULL`)
}
