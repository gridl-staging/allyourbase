package schema

import (
	"sort"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestFunctionByName(t *testing.T) {
	t.Parallel()

	sc := &SchemaCache{
		Functions: map[string]*Function{
			"public.add_numbers":  {Schema: "public", Name: "add_numbers", ReturnType: "integer"},
			"custom.do_something": {Schema: "custom", Name: "do_something", ReturnType: "void"},
		},
	}

	t.Run("finds by unqualified name in public schema", func(t *testing.T) {
		f := sc.FunctionByName("add_numbers")
		testutil.NotNil(t, f)
		testutil.Equal(t, "add_numbers", f.Name)
		testutil.Equal(t, "public", f.Schema)
	})

	t.Run("finds by unqualified name in non-public schema", func(t *testing.T) {
		// Falls back to scanning all schemas when not in public.
		f := sc.FunctionByName("do_something")
		testutil.NotNil(t, f)
		testutil.Equal(t, "custom", f.Schema)
	})

	t.Run("returns nil for unknown function", func(t *testing.T) {
		f := sc.FunctionByName("nonexistent")
		testutil.Nil(t, f)
	})

	t.Run("nil functions map returns nil", func(t *testing.T) {
		sc2 := &SchemaCache{Functions: nil}
		testutil.Nil(t, sc2.FunctionByName("anything"))
	})
}

func TestFunction_ParamByName(t *testing.T) {
	t.Parallel()

	fn := &Function{
		Name: "create_user",
		Parameters: []*FuncParam{
			{Name: "username", Type: "text", Position: 1},
			{Name: "email", Type: "text", Position: 2},
			{Name: "age", Type: "integer", Position: 3},
		},
	}

	t.Run("finds existing param", func(t *testing.T) {
		p := fn.ParamByName("email")
		testutil.NotNil(t, p)
		testutil.Equal(t, "text", p.Type)
		testutil.Equal(t, 2, p.Position)
	})

	t.Run("returns nil for unknown param", func(t *testing.T) {
		testutil.Nil(t, fn.ParamByName("missing"))
	})

	t.Run("nil params returns nil", func(t *testing.T) {
		fn2 := &Function{Name: "no_params", Parameters: nil}
		testutil.Nil(t, fn2.ParamByName("x"))
	})
}

func TestTable_VectorColumns(t *testing.T) {
	t.Parallel()

	tbl := &Table{
		Columns: []*Column{
			{Name: "id", TypeName: "uuid"},
			{Name: "embedding", TypeName: "vector", IsVector: true, VectorDim: 384},
			{Name: "title", TypeName: "text"},
			{Name: "embedding2", TypeName: "vector", IsVector: true, VectorDim: 768},
		},
	}

	cols := tbl.VectorColumns()
	if len(cols) != 2 {
		t.Fatalf("VectorColumns() len = %d, want 2", len(cols))
	}
	testutil.Equal(t, "embedding", cols[0].Name)
	testutil.Equal(t, "embedding2", cols[1].Name)
}

func TestTable_VectorColumns_None(t *testing.T) {
	t.Parallel()

	tbl := &Table{
		Columns: []*Column{
			{Name: "id", TypeName: "uuid"},
			{Name: "name", TypeName: "text"},
		},
	}

	cols := tbl.VectorColumns()
	if len(cols) != 0 {
		t.Fatalf("VectorColumns() len = %d, want 0", len(cols))
	}
}

func TestSchemaListFromSet(t *testing.T) {
	t.Parallel()

	t.Run("multiple schemas", func(t *testing.T) {
		set := map[string]bool{
			"public":  true,
			"custom":  true,
			"private": true,
		}
		got := schemaListFromSet(set)
		// Result order is non-deterministic (map iteration), so sort for comparison.
		sort.Strings(got)
		want := []string{"custom", "private", "public"}
		if len(got) != len(want) {
			t.Fatalf("schemaListFromSet len = %d, want %d", len(got), len(want))
		}
		for i, s := range got {
			testutil.Equal(t, want[i], s)
		}
	})

	t.Run("empty set", func(t *testing.T) {
		got := schemaListFromSet(map[string]bool{})
		if len(got) != 0 {
			t.Fatalf("schemaListFromSet(empty) len = %d, want 0", len(got))
		}
	})

	t.Run("single schema", func(t *testing.T) {
		got := schemaListFromSet(map[string]bool{"public": true})
		testutil.SliceLen(t, got, 1)
		testutil.Equal(t, "public", got[0])
	})
}

func TestStringSliceContains(t *testing.T) {
	t.Parallel()

	testutil.True(t, stringSliceContains([]string{"a", "b", "c"}, "b"))
	testutil.True(t, stringSliceContains([]string{"x"}, "x"))
	testutil.False(t, stringSliceContains([]string{"a", "b"}, "c"))
	testutil.False(t, stringSliceContains(nil, "a"))
	testutil.False(t, stringSliceContains([]string{}, "a"))
}

func TestTableHasSpatialIndexForColumn(t *testing.T) {
	t.Parallel()

	indexes := []*Index{
		{Name: "idx_geo", Method: "gist", Columns: []string{"geom"}},
		{Name: "idx_btree", Method: "btree", Columns: []string{"name"}},
		{Name: "idx_sp", Method: "spgist", Columns: []string{"location"}},
	}

	// Columns covered by gist/spgist indexes should return true.
	testutil.True(t, tableHasSpatialIndexForColumn(indexes, "geom"))
	testutil.True(t, tableHasSpatialIndexForColumn(indexes, "location"))

	// Column with btree index (not spatial) should return false.
	testutil.False(t, tableHasSpatialIndexForColumn(indexes, "name"))

	// Column with no index at all should return false.
	testutil.False(t, tableHasSpatialIndexForColumn(indexes, "unindexed"))

	// Nil/empty indexes should return false.
	testutil.False(t, tableHasSpatialIndexForColumn(nil, "geom"))
	testutil.False(t, tableHasSpatialIndexForColumn([]*Index{}, "geom"))
}

func TestTableByName_NonPublicFallback(t *testing.T) {
	t.Parallel()

	sc := &SchemaCache{
		Tables: map[string]*Table{
			"public.users":   {Schema: "public", Name: "users"},
			"custom.widgets": {Schema: "custom", Name: "widgets"},
		},
	}

	// Direct public match.
	testutil.NotNil(t, sc.TableByName("users"))
	testutil.Equal(t, "public", sc.TableByName("users").Schema)

	// Fallback scan for non-public schema table.
	testutil.NotNil(t, sc.TableByName("widgets"))
	testutil.Equal(t, "custom", sc.TableByName("widgets").Schema)

	// Not found at all.
	testutil.Nil(t, sc.TableByName("nonexistent"))
}

func TestTable_HasGeometry_NoColumns(t *testing.T) {
	t.Parallel()

	// Table with no columns should return false for HasGeometry.
	tbl := &Table{Columns: nil}
	testutil.False(t, tbl.HasGeometry())
}

func TestTable_HasVector_NoColumns(t *testing.T) {
	t.Parallel()

	tbl := &Table{Columns: nil}
	testutil.False(t, tbl.HasVector())
}

func TestColumnByName_EmptyColumns(t *testing.T) {
	t.Parallel()

	tbl := &Table{Columns: nil}
	testutil.Nil(t, tbl.ColumnByName("anything"))
}
