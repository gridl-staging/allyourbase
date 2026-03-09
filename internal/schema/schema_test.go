package schema

import (
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestRelkindToString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		relkind string
		want    string
	}{
		{"r", "table"},
		{"v", "view"},
		{"m", "materialized_view"},
		{"p", "partitioned_table"},
		{"x", "table"}, // unknown defaults to table
		{"", "table"},
	}
	for _, tt := range tests {
		t.Run(tt.relkind+"->"+tt.want, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.want, relkindToString(tt.relkind))
		})
	}
}

func TestFkActionToString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		action string
		want   string
	}{
		{"a", "NO ACTION"},
		{"r", "RESTRICT"},
		{"c", "CASCADE"},
		{"n", "SET NULL"},
		{"d", "SET DEFAULT"},
		{"x", "NO ACTION"}, // unknown defaults to NO ACTION
		{"", "NO ACTION"},
	}
	for _, tt := range tests {
		t.Run(tt.action+"->"+tt.want, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.want, fkActionToString(tt.action))
		})
	}
}

func TestTableByName(t *testing.T) {
	t.Parallel()
	sc := &SchemaCache{
		Tables: map[string]*Table{
			"public.users": {Schema: "public", Name: "users"},
			"public.posts": {Schema: "public", Name: "posts"},
			"other.items":  {Schema: "other", Name: "items"},
		},
	}

	t.Run("finds public table by name", func(t *testing.T) {
		t.Parallel()
		tbl := sc.TableByName("users")
		testutil.NotNil(t, tbl)
		testutil.Equal(t, "users", tbl.Name)
		testutil.Equal(t, "public", tbl.Schema)
	})

	t.Run("finds non-public table by fallback scan", func(t *testing.T) {
		t.Parallel()
		tbl := sc.TableByName("items")
		testutil.NotNil(t, tbl)
		testutil.Equal(t, "items", tbl.Name)
		testutil.Equal(t, "other", tbl.Schema)
	})

	t.Run("returns nil for missing table", func(t *testing.T) {
		t.Parallel()
		tbl := sc.TableByName("nonexistent")
		testutil.Nil(t, tbl)
	})

	t.Run("prefers public schema", func(t *testing.T) {
		t.Parallel()
		sc2 := &SchemaCache{
			Tables: map[string]*Table{
				"public.data": {Schema: "public", Name: "data"},
				"other.data":  {Schema: "other", Name: "data"},
			},
		}
		tbl := sc2.TableByName("data")
		testutil.NotNil(t, tbl)
		testutil.Equal(t, "public", tbl.Schema)
	})
}

func TestColumnByName(t *testing.T) {
	t.Parallel()
	tbl := &Table{
		Columns: []*Column{
			{Name: "id", Position: 1},
			{Name: "name", Position: 2},
			{Name: "email", Position: 3},
		},
	}

	t.Run("finds existing column", func(t *testing.T) {
		t.Parallel()
		col := tbl.ColumnByName("name")
		testutil.NotNil(t, col)
		testutil.Equal(t, "name", col.Name)
		testutil.Equal(t, 2, col.Position)
	})

	t.Run("returns nil for missing column", func(t *testing.T) {
		t.Parallel()
		col := tbl.ColumnByName("nonexistent")
		testutil.Nil(t, col)
	})
}

func TestTableList(t *testing.T) {
	t.Parallel()
	sc := &SchemaCache{
		Tables: map[string]*Table{
			"public.a": {Schema: "public", Name: "a"},
			"public.b": {Schema: "public", Name: "b"},
			"other.c":  {Schema: "other", Name: "c"},
		},
	}

	tables := sc.TableList()
	testutil.SliceLen(t, tables, 3)

	// Verify all tables are present (order is non-deterministic from map).
	names := map[string]bool{}
	for _, tbl := range tables {
		names[tbl.Schema+"."+tbl.Name] = true
	}
	testutil.True(t, names["public.a"], "expected public.a in list")
	testutil.True(t, names["public.b"], "expected public.b in list")
	testutil.True(t, names["other.c"], "expected other.c in list")
}

func TestDeriveFieldName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		columns  []string
		refTable string
		want     string
	}{
		{
			name:     "single column with _id suffix",
			columns:  []string{"author_id"},
			refTable: "users",
			want:     "author",
		},
		{
			name:     "single column without _id suffix",
			columns:  []string{"creator"},
			refTable: "users",
			want:     "users",
		},
		{
			name:     "composite FK uses table name",
			columns:  []string{"org_id", "team_id"},
			refTable: "teams",
			want:     "teams",
		},
		{
			name:     "user_id maps to user",
			columns:  []string{"user_id"},
			refTable: "users",
			want:     "user",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := deriveFieldName(tt.columns, tt.refTable)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestBuildRelationships(t *testing.T) {
	t.Parallel()
	tables := map[string]*Table{
		"public.users": {Schema: "public", Name: "users"},
		"public.posts": {
			Schema: "public",
			Name:   "posts",
			ForeignKeys: []*ForeignKey{
				{
					ConstraintName:    "fk_posts_author",
					Columns:           []string{"author_id"},
					ReferencedSchema:  "public",
					ReferencedTable:   "users",
					ReferencedColumns: []string{"id"},
				},
			},
		},
	}

	buildRelationships(tables)

	// posts should have a many-to-one relationship to users.
	posts := tables["public.posts"]
	testutil.SliceLen(t, posts.Relationships, 1)
	testutil.Equal(t, "many-to-one", posts.Relationships[0].Type)
	testutil.Equal(t, "users", posts.Relationships[0].ToTable)
	testutil.Equal(t, "author", posts.Relationships[0].FieldName)

	// users should have a one-to-many relationship from posts.
	users := tables["public.users"]
	testutil.SliceLen(t, users.Relationships, 1)
	testutil.Equal(t, "one-to-many", users.Relationships[0].Type)
	testutil.Equal(t, "posts", users.Relationships[0].ToTable)
	testutil.Equal(t, "posts", users.Relationships[0].FieldName)
}

func TestSchemaFilter(t *testing.T) {
	t.Parallel()
	clause, args := schemaFilter("n", 1)

	// Should exclude information_schema, pg_catalog, pg_toast, and pg_% pattern.
	testutil.Contains(t, clause, "n.nspname != $1")
	testutil.Contains(t, clause, "n.nspname NOT LIKE")
	testutil.Equal(t, 4, len(args))

	// Args should contain the excluded schema names.
	found := map[string]bool{}
	for _, a := range args {
		if s, ok := a.(string); ok {
			found[s] = true
		}
	}
	testutil.True(t, found["information_schema"], "missing information_schema")
	testutil.True(t, found["pg_catalog"], "missing pg_catalog")
	testutil.True(t, found["pg_toast"], "missing pg_toast")
	testutil.True(t, found["pg_%"], "missing pg_% pattern")
}

func TestSchemaFilterParamOffset(t *testing.T) {
	t.Parallel()
	clause, args := schemaFilter("s", 5)

	testutil.Contains(t, clause, "s.nspname != $5")
	testutil.Contains(t, clause, "s.nspname != $6")
	testutil.Contains(t, clause, "s.nspname != $7")
	testutil.Contains(t, clause, "s.nspname NOT LIKE $8")
	testutil.Equal(t, 4, len(args))
}

// TestSetForTestingSignalsReady verifies that SetForTesting closes the ready
// channel on first call with a non-nil cache, making <-Ready() unblock.
func TestSetForTestingSignalsReady(t *testing.T) {
	t.Parallel()
	h := &CacheHolder{ready: make(chan struct{})}
	sc := &SchemaCache{}

	// Ready should not be closed yet.
	select {
	case <-h.Ready():
		t.Fatal("ready should not be signalled before SetForTesting")
	default:
	}

	h.SetForTesting(sc)

	// Ready should now be closed (with timeout to avoid hanging on bug).
	select {
	case <-h.Ready():
		// expected
	case <-time.After(time.Second):
		t.Fatal("ready channel not closed after SetForTesting")
	}

	testutil.Equal(t, sc, h.Get())
}

// TestSetForTestingNilDoesNotSignal verifies that SetForTesting(nil) does NOT
// close the ready channel (nil cache is not a valid ready state).
func TestSetForTestingNilDoesNotSignal(t *testing.T) {
	t.Parallel()
	h := &CacheHolder{ready: make(chan struct{})}

	h.SetForTesting(nil)

	select {
	case <-h.Ready():
		t.Fatal("ready should not be signalled when SetForTesting called with nil")
	default:
	}

	testutil.Nil(t, h.Get())
}

// TestSetForTestingIdempotent verifies that calling SetForTesting multiple times
// with non-nil caches does not panic (no double-close of ready channel).
func TestSetForTestingIdempotent(t *testing.T) {
	t.Parallel()
	h := &CacheHolder{ready: make(chan struct{})}
	sc1 := &SchemaCache{BuiltAt: time.Now()}
	sc2 := &SchemaCache{BuiltAt: time.Now().Add(time.Second)}

	// First call should signal ready.
	h.SetForTesting(sc1)
	select {
	case <-h.Ready():
	case <-time.After(time.Second):
		t.Fatal("ready not signalled after first SetForTesting")
	}

	// Second call must NOT panic (double-close would panic).
	h.SetForTesting(sc2)

	// Cache should now reflect the second value.
	testutil.Equal(t, sc2, h.Get())
}
