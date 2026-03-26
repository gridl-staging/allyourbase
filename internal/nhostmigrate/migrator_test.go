package nhostmigrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNewMigratorRejectsMissingPaths(t *testing.T) {
	t.Parallel()

	_, err := NewMigrator(MigrationOptions{DatabaseURL: "postgres://localhost/test"})
	testutil.ErrorContains(t, err, "hasura metadata path is required")

	_, err = NewMigrator(MigrationOptions{HasuraMetadataPath: "/tmp/metadata", DatabaseURL: "postgres://localhost/test"})
	testutil.ErrorContains(t, err, "pg_dump path is required")
}

func TestBuildPlanFromFixtures(t *testing.T) {
	t.Parallel()

	root := filepath.Join("testdata")
	m := &Migrator{opts: MigrationOptions{
		HasuraMetadataPath: filepath.Join(root, "metadata"),
		PgDumpPath:         filepath.Join(root, "pg_dump.sql"),
		SkipRLS:            false,
	}, progress: migrate.NopReporter{}}

	plan, report, err := m.buildPlan(context.Background())
	testutil.NoError(t, err)

	testutil.Equal(t, 2, report.Tables)
	testutil.Equal(t, 2, report.Records)
	testutil.Equal(t, 2, report.RLSPolicies)
	testutil.Equal(t, 2, plan.stats.Tables)
	testutil.Equal(t, 1, plan.stats.Indexes)
	testutil.Equal(t, 2, plan.stats.Policies)
	testutil.Equal(t, 1, plan.stats.ForeignKeys)

	allSQL := plan.JoinedSQL()
	testutil.Contains(t, allSQL, "CREATE TABLE public.authors")
	testutil.Contains(t, allSQL, "CREATE TABLE public.posts")
	testutil.Contains(t, allSQL, "CREATE INDEX posts_title_idx")
	testutil.Contains(t, allSQL, `ALTER TABLE ONLY "public"."posts"`)
	testutil.Contains(t, allSQL, "CREATE POLICY")
	testutil.False(t, containsSubstring(allSQL, "hdb_catalog"), "hasura catalog SQL must be skipped")
}

func TestBuildValidationSummary(t *testing.T) {
	t.Parallel()

	report := &migrate.AnalysisReport{Tables: 2, Views: 1, Records: 10, RLSPolicies: 2}
	stats := &MigrationStats{Tables: 2, Views: 1, Records: 10, Policies: 2}

	summary := BuildValidationSummary(report, stats)
	testutil.Equal(t, "NHost (source)", summary.SourceLabel)
	testutil.Equal(t, "AYB (target)", summary.TargetLabel)
	testutil.Equal(t, 4, len(summary.Rows))
	testutil.Equal(t, 0, len(summary.Warnings))
}

func TestLoadHasuraV3TableFilesSupportsYAML(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	tablesDir := filepath.Join(root, "databases", "default", "tables")
	testutil.NoError(t, os.MkdirAll(tablesDir, 0o755))

	yamlBody := `table:
  schema: public
  name: comments
select_permissions:
  - role: user
`
	testutil.NoError(t, os.WriteFile(filepath.Join(tablesDir, "public_comments.yaml"), []byte(yamlBody), 0o644))

	files, err := loadHasuraV3TableFiles(root)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(files))
	if len(files) != 1 {
		return
	}
	testutil.Equal(t, "public", files[0].Table.Schema)
	testutil.Equal(t, "comments", files[0].Table.Name)
	testutil.Equal(t, 1, len(files[0].SelectPermissions))
}

func containsSubstring(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
