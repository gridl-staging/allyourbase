package nhostmigrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSplitSQLStatementsKeepsDollarQuotedSemicolons(t *testing.T) {
	t.Parallel()

	sqlText := `
CREATE FUNCTION public.bump(v integer) RETURNS integer
LANGUAGE plpgsql
AS $func$
BEGIN
  IF v > 0 THEN
    RETURN v + 1;
  END IF;
  RETURN v;
END;
$func$;
CREATE TABLE public.sample (id bigint PRIMARY KEY);
`

	statements := splitSQLStatements(sqlText)
	testutil.Equal(t, 2, len(statements))
	testutil.Contains(t, statements[0], "RETURN v + 1;")
	testutil.Contains(t, statements[0], "END;")
	testutil.Contains(t, statements[1], "CREATE TABLE public.sample")
}

func TestCountInsertRowsMultiRow(t *testing.T) {
	t.Parallel()

	stmt := "INSERT INTO public.posts (id, title) VALUES (1, 'A'), (2, 'B'), (3, 'C')"
	testutil.Equal(t, 3, countInsertRows(stmt))
}

func TestBuildPlanSuppressesDuplicateForeignKeys(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dumpPath := filepath.Join(root, "dump.sql")
	metadataRoot := filepath.Join(root, "metadata")
	tablesDir := filepath.Join(metadataRoot, "databases", "default", "tables")

	testutil.NoError(t, os.MkdirAll(tablesDir, 0o755))

	dump := `
CREATE TABLE public.authors (id bigint PRIMARY KEY);
CREATE TABLE public.posts (id bigint PRIMARY KEY, author_id bigint NOT NULL);
ALTER TABLE ONLY public.posts
  ADD CONSTRAINT posts_author_id_fkey
  FOREIGN KEY (author_id) REFERENCES public.authors(id);
`
	testutil.NoError(t, os.WriteFile(dumpPath, []byte(dump), 0o644))

	authorsMetadata := `{
  "table": {"schema": "public", "name": "authors"},
  "array_relationships": [
    {
      "name": "posts",
      "using": {
        "foreign_key_constraint_on": {
          "table": {"schema": "public", "name": "posts"},
          "column": "author_id"
        }
      }
    }
  ]
}`
	testutil.NoError(t, os.WriteFile(filepath.Join(tablesDir, "public_authors.json"), []byte(authorsMetadata), 0o644))

	m := &Migrator{
		opts: MigrationOptions{
			HasuraMetadataPath: metadataRoot,
			PgDumpPath:         dumpPath,
		},
		progress: migrate.NopReporter{},
	}

	plan, _, err := m.buildPlan(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 1, plan.stats.ForeignKeys)
}

func TestParseObjectRelationshipSupportsForeignKeyConstraintOn(t *testing.T) {
	t.Parallel()

	current := qualifiedTable{Schema: "public", Name: "posts"}
	rel := hasuraRelationship{
		Name: "author",
		Using: []byte(`{
  "foreign_key_constraint_on": {
    "table": {"schema": "public", "name": "authors"},
    "column": "author_id"
  }
}`),
	}

	fk, ok := parseObjectRelationship(current, rel)
	testutil.True(t, ok)
	testutil.Equal(t, "public", fk.FromSchema)
	testutil.Equal(t, "posts", fk.FromTable)
	testutil.Equal(t, "author_id", fk.FromColumn)
	testutil.Equal(t, "public", fk.ToSchema)
	testutil.Equal(t, "authors", fk.ToTable)
	testutil.Equal(t, "id", fk.ToColumn)
}

func TestParseObjectRelationshipSupportsForeignKeyConstraintOnString(t *testing.T) {
	t.Parallel()

	current := qualifiedTable{Schema: "public", Name: "posts"}
	rel := hasuraRelationship{
		Name:  "author",
		Using: []byte(`{"foreign_key_constraint_on":"author_id"}`),
	}

	fk, ok := parseObjectRelationshipWithResolver(current, rel, func(fromSchema, fromTable, fromColumn string) (metadataForeignKey, bool) {
		testutil.Equal(t, "public", fromSchema)
		testutil.Equal(t, "posts", fromTable)
		testutil.Equal(t, "author_id", fromColumn)
		return metadataForeignKey{
			FromSchema: fromSchema,
			FromTable:  fromTable,
			FromColumn: fromColumn,
			ToSchema:   "public",
			ToTable:    "authors",
			ToColumn:   "id",
		}, true
	})

	testutil.True(t, ok)
	testutil.Equal(t, "public", fk.FromSchema)
	testutil.Equal(t, "posts", fk.FromTable)
	testutil.Equal(t, "author_id", fk.FromColumn)
	testutil.Equal(t, "public", fk.ToSchema)
	testutil.Equal(t, "authors", fk.ToTable)
	testutil.Equal(t, "id", fk.ToColumn)
}
