package directusmigrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNewMigratorRejectsMissingSnapshotPath(t *testing.T) {
	t.Parallel()

	_, err := NewMigrator(MigrationOptions{DatabaseURL: "postgres://localhost/test"})
	testutil.ErrorContains(t, err, "snapshot path is required")
}

func TestBuildPlanFromFixture(t *testing.T) {
	t.Parallel()

	m := &Migrator{opts: MigrationOptions{
		SnapshotPath: filepath.Join("testdata", "snapshot.json"),
		SkipRLS:      false,
	}, progress: migrate.NopReporter{}}

	plan, report, err := m.buildPlan(context.Background())
	testutil.NoError(t, err)

	testutil.Equal(t, 2, report.Tables)
	testutil.Equal(t, 2, plan.stats.Collections)
	testutil.Equal(t, 6, plan.stats.Fields)
	testutil.Equal(t, 1, plan.stats.Relations)
	testutil.Equal(t, 2, plan.stats.Policies)

	allSQL := plan.JoinedSQL()
	testutil.Contains(t, allSQL, "CREATE TABLE \"public\".\"authors\"")
	testutil.Contains(t, allSQL, "CREATE TABLE \"public\".\"articles\"")
	testutil.False(t, contains(allSQL, "directus_users"), "directus_* collections must be skipped")
	testutil.Contains(t, allSQL, "\"published_at\" timestamptz")
	testutil.Contains(t, allSQL, "\"score\" double precision")
	testutil.Contains(t, allSQL, "FOREIGN KEY (\"author_id\")")
	testutil.Contains(t, allSQL, "CREATE POLICY")
}

func TestBuildValidationSummary(t *testing.T) {
	t.Parallel()

	report := &migrate.AnalysisReport{Tables: 2, RLSPolicies: 1}
	stats := &MigrationStats{Collections: 2, Policies: 1}

	summary := BuildValidationSummary(report, stats)
	testutil.Equal(t, "Directus (source)", summary.SourceLabel)
	testutil.Equal(t, "AYB (target)", summary.TargetLabel)
	testutil.Equal(t, 2, len(summary.Rows))
}

func TestSkipsDirectusSystemCollections(t *testing.T) {
	t.Parallel()
	if !shouldSkipCollection("directus_users") {
		t.Fatal("expected directus_users to be skipped")
	}
	if shouldSkipCollection("articles") {
		t.Fatal("did not expect articles to be skipped")
	}
}

func TestBuildPlanRelationUsesRelatedPrimaryKey(t *testing.T) {
	t.Parallel()

	snapshot := `{
  "collections": [
    {"collection": "authors"},
    {"collection": "articles"}
  ],
  "fields": [
    {"collection": "authors", "field": "author_key", "type": "uuid", "schema": {"is_primary_key": true, "is_nullable": false}},
    {"collection": "articles", "field": "id", "type": "uuid", "schema": {"is_primary_key": true, "is_nullable": false}},
    {"collection": "articles", "field": "author_key", "type": "uuid", "schema": {"is_nullable": false}}
  ],
  "relations": [
    {"collection": "articles", "field": "author_key", "related_collection": "authors", "meta": {"one_field": "articles"}}
  ],
  "permissions": []
}`

	tmp := filepath.Join(t.TempDir(), "snapshot.json")
	testutil.NoError(t, os.WriteFile(tmp, []byte(snapshot), 0o644))

	m := &Migrator{opts: MigrationOptions{SnapshotPath: tmp}, progress: migrate.NopReporter{}}
	plan, _, err := m.buildPlan(context.Background())
	testutil.NoError(t, err)

	allSQL := plan.JoinedSQL()
	testutil.Contains(t, allSQL, `REFERENCES "public"."authors"("author_key")`)
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
