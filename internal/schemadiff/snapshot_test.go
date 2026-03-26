package schemadiff

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestFromSchemaCache_tables(t *testing.T) {
	t.Parallel()

	cache := &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users": {
				Schema:     "public",
				Name:       "users",
				Kind:       "table",
				RLSEnabled: true,
				Columns: []*schema.Column{
					{Name: "id", TypeName: "uuid", IsPrimaryKey: true, Position: 1},
					{Name: "email", TypeName: "text", IsNullable: false, Position: 2},
					{Name: "bio", TypeName: "text", IsNullable: true, Position: 3},
				},
				PrimaryKey: []string{"id"},
				Indexes: []*schema.Index{
					{Name: "users_pkey", IsPrimary: true, IsUnique: true, Method: "btree", Definition: "CREATE UNIQUE INDEX users_pkey ON public.users USING btree (id)"},
					{Name: "users_email_idx", IsUnique: true, Method: "btree", Definition: "CREATE UNIQUE INDEX users_email_idx ON public.users USING btree (email)"},
				},
				ForeignKeys: []*schema.ForeignKey{
					{
						ConstraintName:    "fk_users_org",
						Columns:           []string{"org_id"},
						ReferencedSchema:  "public",
						ReferencedTable:   "orgs",
						ReferencedColumns: []string{"id"},
						OnDelete:          "CASCADE",
					},
				},
				CheckConstraints: []*schema.CheckConstraint{
					{Name: "users_email_check", Definition: "email LIKE '%@%'"},
				},
				RLSPolicies: []*schema.RLSPolicy{
					{
						Name:       "users_own",
						Command:    "SELECT",
						Permissive: true,
						Roles:      []string{"authenticated"},
						UsingExpr:  "(auth.uid() = id)",
					},
				},
			},
		},
		Enums: map[uint32]*schema.EnumType{
			1: {Schema: "public", Name: "status", Values: []string{"active", "inactive"}},
		},
	}

	snap := FromSchemaCache(cache)

	testutil.Equal(t, 1, len(snap.Tables))
	st := snap.Tables[0]
	testutil.Equal(t, "public", st.Schema)
	testutil.Equal(t, "users", st.Name)
	testutil.Equal(t, "table", st.Kind)
	testutil.True(t, st.RLSEnabled, "expected RLSEnabled")

	// Columns
	testutil.Equal(t, 3, len(st.Columns))
	testutil.Equal(t, "id", st.Columns[0].Name)
	testutil.True(t, st.Columns[0].IsPrimaryKey, "id should be PK")

	// Indexes sorted by name
	testutil.Equal(t, 2, len(st.Indexes))
	testutil.Equal(t, "users_email_idx", st.Indexes[0].Name)
	testutil.Equal(t, "users_pkey", st.Indexes[1].Name)

	// FK
	testutil.Equal(t, 1, len(st.ForeignKeys))
	testutil.Equal(t, "fk_users_org", st.ForeignKeys[0].ConstraintName)
	testutil.Equal(t, "CASCADE", st.ForeignKeys[0].OnDelete)

	// Check constraints
	testutil.Equal(t, 1, len(st.CheckConstraints))
	testutil.Equal(t, "users_email_check", st.CheckConstraints[0].Name)
	testutil.Equal(t, "email LIKE '%@%'", st.CheckConstraints[0].Definition)

	// RLS policies sorted by name
	testutil.Equal(t, 1, len(st.RLSPolicies))
	testutil.Equal(t, "users_own", st.RLSPolicies[0].Name)
	testutil.Equal(t, "SELECT", st.RLSPolicies[0].Command)
	testutil.True(t, st.RLSPolicies[0].Permissive, "expected permissive")
	testutil.Equal(t, "(auth.uid() = id)", st.RLSPolicies[0].UsingExpr)

	// Enums
	testutil.Equal(t, 1, len(snap.Enums))
	testutil.Equal(t, "status", snap.Enums[0].Name)
	testutil.Equal(t, 2, len(snap.Enums[0].Values))
}

func TestFromSchemaCache_tablesSortedBySchemaName(t *testing.T) {
	t.Parallel()

	cache := &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.z_table": {Schema: "public", Name: "z_table", Columns: []*schema.Column{}},
			"public.a_table": {Schema: "public", Name: "a_table", Columns: []*schema.Column{}},
			"other.b_table":  {Schema: "other", Name: "b_table", Columns: []*schema.Column{}},
		},
	}

	snap := FromSchemaCache(cache)
	testutil.Equal(t, 3, len(snap.Tables))
	testutil.Equal(t, "other.b_table", snap.Tables[0].FullName())
	testutil.Equal(t, "public.a_table", snap.Tables[1].FullName())
	testutil.Equal(t, "public.z_table", snap.Tables[2].FullName())
}

func TestFromSchemaCache_enumColumnValues(t *testing.T) {
	t.Parallel()

	cache := &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.posts": {
				Schema: "public",
				Name:   "posts",
				Kind:   "table",
				Columns: []*schema.Column{
					{
						Name:       "status",
						TypeName:   "status",
						IsEnum:     true,
						EnumValues: []string{"draft", "published", "archived"},
						Position:   1,
					},
				},
			},
		},
	}

	snap := FromSchemaCache(cache)
	testutil.Equal(t, 3, len(snap.Tables[0].Columns[0].EnumValues))
	testutil.Equal(t, "draft", snap.Tables[0].Columns[0].EnumValues[0])
}

func TestSaveAndLoadSnapshot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.json")

	original := &Snapshot{
		Tables: []SnapTable{
			{
				Schema: "public",
				Name:   "users",
				Kind:   "table",
				Columns: []SnapColumn{
					{Name: "id", TypeName: "uuid", IsPrimaryKey: true},
					{Name: "email", TypeName: "text"},
				},
				Indexes: []SnapIndex{
					{Name: "users_pkey", IsPrimary: true, IsUnique: true, Method: "btree"},
				},
				CheckConstraints: []SnapCheckConstraint{
					{Name: "ck_positive", Definition: "age > 0"},
				},
				RLSPolicies: []SnapRLSPolicy{
					{Name: "pol1", Command: "ALL", Permissive: true, Roles: []string{"public"}},
				},
			},
		},
		Enums: []SnapEnum{
			{Schema: "public", Name: "role_type", Values: []string{"admin", "user"}},
		},
		Extensions: []SnapExtension{
			{Name: "pg_trgm", Version: "1.6"},
			{Name: "pgvector", Version: "0.7.0"},
		},
	}

	err := SaveSnapshot(path, original)
	testutil.NoError(t, err)

	// File must exist.
	_, err = os.Stat(path)
	testutil.NoError(t, err)

	loaded, err := LoadSnapshot(path)
	testutil.NoError(t, err)

	testutil.Equal(t, 1, len(loaded.Tables))
	testutil.Equal(t, "users", loaded.Tables[0].Name)
	testutil.Equal(t, 2, len(loaded.Tables[0].Columns))
	testutil.Equal(t, 1, len(loaded.Tables[0].Indexes))
	testutil.Equal(t, 1, len(loaded.Tables[0].CheckConstraints))
	testutil.Equal(t, "ck_positive", loaded.Tables[0].CheckConstraints[0].Name)
	testutil.Equal(t, "age > 0", loaded.Tables[0].CheckConstraints[0].Definition)
	testutil.Equal(t, 1, len(loaded.Tables[0].RLSPolicies))
	testutil.Equal(t, 1, len(loaded.Enums))
	testutil.Equal(t, "role_type", loaded.Enums[0].Name)
	testutil.Equal(t, 2, len(loaded.Extensions))
	testutil.Equal(t, "pg_trgm", loaded.Extensions[0].Name)
	testutil.Equal(t, "pgvector", loaded.Extensions[1].Name)
}

func TestLoadSnapshot_missingFile(t *testing.T) {
	t.Parallel()

	_, err := LoadSnapshot("/nonexistent/path/snapshot.json")
	testutil.ErrorContains(t, err, "reading snapshot")
}

func TestSaveSnapshot_invalidDir(t *testing.T) {
	t.Parallel()

	snap := &Snapshot{}
	err := SaveSnapshot("/nonexistent/dir/snapshot.json", snap)
	testutil.ErrorContains(t, err, "writing snapshot")
}

func TestSnapshotJSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := &Snapshot{
		Tables: []SnapTable{
			{
				Schema: "public",
				Name:   "products",
				Kind:   "table",
				Columns: []SnapColumn{
					{Name: "id", TypeName: "bigint", IsPrimaryKey: true},
					{Name: "price", TypeName: "numeric", IsNullable: false},
					{Name: "tags", TypeName: "text[]", IsNullable: true},
				},
				ForeignKeys: []SnapForeignKey{
					{
						ConstraintName:    "fk_category",
						Columns:           []string{"category_id"},
						ReferencedSchema:  "public",
						ReferencedTable:   "categories",
						ReferencedColumns: []string{"id"},
						OnDelete:          "SET NULL",
					},
				},
				RLSEnabled: false,
			},
		},
		Enums:      []SnapEnum{},
		Extensions: []SnapExtension{{Name: "postgis", Version: "3.4.0"}},
	}

	data, err := json.Marshal(original)
	testutil.NoError(t, err)

	var loaded Snapshot
	err = json.Unmarshal(data, &loaded)
	testutil.NoError(t, err)

	testutil.Equal(t, 1, len(loaded.Tables))
	testutil.Equal(t, "products", loaded.Tables[0].Name)
	testutil.Equal(t, 3, len(loaded.Tables[0].Columns))
	testutil.Equal(t, 1, len(loaded.Tables[0].ForeignKeys))
	testutil.Equal(t, "fk_category", loaded.Tables[0].ForeignKeys[0].ConstraintName)
	testutil.Equal(t, "SET NULL", loaded.Tables[0].ForeignKeys[0].OnDelete)
	testutil.Equal(t, 1, len(loaded.Extensions))
	testutil.Equal(t, "postgis", loaded.Extensions[0].Name)
}

func TestTableFullName(t *testing.T) {
	t.Parallel()

	tbl := SnapTable{Schema: "myschema", Name: "mytable"}
	testutil.Equal(t, "myschema.mytable", tbl.FullName())
}

func TestSnapshotTableByName(t *testing.T) {
	t.Parallel()

	snap := &Snapshot{
		Tables: []SnapTable{
			{Schema: "public", Name: "users"},
			{Schema: "public", Name: "posts"},
		},
	}

	tbl := snap.tableByName("public.users")
	testutil.NotNil(t, tbl)
	testutil.Equal(t, "users", tbl.Name)

	missing := snap.tableByName("public.missing")
	testutil.Nil(t, missing)
}

func TestSnapshotEnumByName(t *testing.T) {
	t.Parallel()

	snap := &Snapshot{
		Enums: []SnapEnum{
			{Schema: "public", Name: "status", Values: []string{"a", "b"}},
		},
	}

	e := snap.enumByName("public", "status")
	testutil.NotNil(t, e)
	testutil.Equal(t, "status", e.Name)

	missing := snap.enumByName("public", "nope")
	testutil.Nil(t, missing)
}

func TestSnapshotExtensionByName(t *testing.T) {
	t.Parallel()

	snap := &Snapshot{
		Extensions: []SnapExtension{
			{Name: "pgvector", Version: "0.7.0"},
		},
	}

	ext := snap.extensionByName("pgvector")
	testutil.NotNil(t, ext)
	testutil.Equal(t, "0.7.0", ext.Version)

	missing := snap.extensionByName("postgis")
	testutil.Nil(t, missing)
}
