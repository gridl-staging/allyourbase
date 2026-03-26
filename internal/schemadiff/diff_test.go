package schemadiff

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// helpers

func snapWithTable(t SnapTable) *Snapshot {
	return &Snapshot{Tables: []SnapTable{t}}
}

func baseTable(schema, name string, cols ...SnapColumn) SnapTable {
	return SnapTable{Schema: schema, Name: name, Kind: "table", Columns: cols}
}

func col(name, typ string) SnapColumn {
	return SnapColumn{Name: name, TypeName: typ}
}

func nullableCol(name, typ string) SnapColumn {
	return SnapColumn{Name: name, TypeName: typ, IsNullable: true}
}

func defaultCol(name, typ, def string) SnapColumn {
	return SnapColumn{Name: name, TypeName: typ, DefaultExpr: def}
}

func findChange(cs ChangeSet, ct ChangeType) *Change {
	for i := range cs {
		if cs[i].Type == ct {
			return &cs[i]
		}
	}
	return nil
}

func findChanges(cs ChangeSet, ct ChangeType) []Change {
	var out []Change
	for _, c := range cs {
		if c.Type == ct {
			out = append(out, c)
		}
	}
	return out
}

// Tests

func TestDiff_empty(t *testing.T) {
	t.Parallel()

	cs := Diff(&Snapshot{}, &Snapshot{})
	testutil.Equal(t, 0, len(cs))
}

func TestDiff_nilSnapshots(t *testing.T) {
	t.Parallel()

	cs := Diff(nil, nil)
	testutil.Equal(t, 0, len(cs))
}

func TestDiff_noChange(t *testing.T) {
	t.Parallel()

	snap := &Snapshot{
		Tables: []SnapTable{
			baseTable("public", "users", col("id", "uuid"), col("name", "text")),
		},
		Enums:      []SnapEnum{{Schema: "public", Name: "status", Values: []string{"a", "b"}}},
		Extensions: []SnapExtension{{Name: "pgvector", Version: "0.7.0"}},
	}

	cs := Diff(snap, snap)
	testutil.Equal(t, 0, len(cs))
}

func TestDiff_createTable(t *testing.T) {
	t.Parallel()

	newSnap := snapWithTable(baseTable("public", "posts",
		col("id", "uuid"),
		col("title", "text"),
	))

	cs := Diff(&Snapshot{}, newSnap)

	createChanges := findChanges(cs, ChangeCreateTable)
	testutil.Equal(t, 1, len(createChanges))
	c := createChanges[0]
	testutil.Equal(t, "public", c.SchemaName)
	testutil.Equal(t, "posts", c.TableName)
	testutil.Equal(t, 2, len(c.AllColumns))
}

func TestDiff_dropTable(t *testing.T) {
	t.Parallel()

	oldSnap := snapWithTable(baseTable("public", "legacy"))

	cs := Diff(oldSnap, &Snapshot{})

	dropChanges := findChanges(cs, ChangeDropTable)
	testutil.Equal(t, 1, len(dropChanges))
	testutil.Equal(t, "legacy", dropChanges[0].TableName)
}

func TestDiff_addColumn(t *testing.T) {
	t.Parallel()

	old := snapWithTable(baseTable("public", "users", col("id", "uuid")))
	new := snapWithTable(baseTable("public", "users",
		col("id", "uuid"),
		col("email", "text"),
	))

	cs := Diff(old, new)
	adds := findChanges(cs, ChangeAddColumn)
	testutil.Equal(t, 1, len(adds))
	testutil.Equal(t, "email", adds[0].ColumnName)
	testutil.Equal(t, "text", adds[0].NewTypeName)
}

func TestDiff_dropColumn(t *testing.T) {
	t.Parallel()

	old := snapWithTable(baseTable("public", "users",
		col("id", "uuid"),
		col("legacy_col", "text"),
	))
	new := snapWithTable(baseTable("public", "users", col("id", "uuid")))

	cs := Diff(old, new)
	drops := findChanges(cs, ChangeDropColumn)
	testutil.Equal(t, 1, len(drops))
	testutil.Equal(t, "legacy_col", drops[0].ColumnName)
}

func TestDiff_alterColumnType(t *testing.T) {
	t.Parallel()

	old := snapWithTable(baseTable("public", "products", col("price", "integer")))
	new := snapWithTable(baseTable("public", "products", col("price", "numeric")))

	cs := Diff(old, new)
	alters := findChanges(cs, ChangeAlterColumnType)
	testutil.Equal(t, 1, len(alters))
	testutil.Equal(t, "price", alters[0].ColumnName)
	testutil.Equal(t, "integer", alters[0].OldTypeName)
	testutil.Equal(t, "numeric", alters[0].NewTypeName)
}

func TestDiff_alterColumnDefault(t *testing.T) {
	t.Parallel()

	old := snapWithTable(baseTable("public", "users", defaultCol("status", "text", "'active'")))
	new := snapWithTable(baseTable("public", "users", defaultCol("status", "text", "'inactive'")))

	cs := Diff(old, new)
	alters := findChanges(cs, ChangeAlterColumnDefault)
	testutil.Equal(t, 1, len(alters))
	testutil.Equal(t, "status", alters[0].ColumnName)
	testutil.Equal(t, "'active'", alters[0].OldDefault)
	testutil.Equal(t, "'inactive'", alters[0].NewDefault)
}

func TestDiff_alterColumnNullable(t *testing.T) {
	t.Parallel()

	old := snapWithTable(baseTable("public", "users", col("bio", "text")))
	new := snapWithTable(baseTable("public", "users", nullableCol("bio", "text")))

	cs := Diff(old, new)
	alters := findChanges(cs, ChangeAlterColumnNullable)
	testutil.Equal(t, 1, len(alters))
	testutil.Equal(t, "bio", alters[0].ColumnName)
	testutil.False(t, alters[0].OldNullable, "expected old nullable=false")
	testutil.True(t, alters[0].NewNullable, "expected new nullable=true")
}

func TestDiff_createIndex(t *testing.T) {
	t.Parallel()

	idx := SnapIndex{Name: "users_email_idx", IsUnique: true, Method: "btree",
		Definition: "CREATE UNIQUE INDEX users_email_idx ON public.users USING btree (email)"}

	old := snapWithTable(baseTable("public", "users", col("id", "uuid"), col("email", "text")))
	new := &Snapshot{Tables: []SnapTable{
		{Schema: "public", Name: "users", Kind: "table",
			Columns: []SnapColumn{col("id", "uuid"), col("email", "text")},
			Indexes: []SnapIndex{idx},
		},
	}}

	cs := Diff(old, new)
	creates := findChanges(cs, ChangeCreateIndex)
	testutil.Equal(t, 1, len(creates))
	testutil.Equal(t, "users_email_idx", creates[0].Index.Name)
	testutil.True(t, creates[0].Index.IsUnique, "expected unique index")
}

func TestDiff_dropIndex(t *testing.T) {
	t.Parallel()

	idx := SnapIndex{Name: "old_idx", Method: "btree",
		Definition: "CREATE INDEX old_idx ON public.t USING btree (col)"}

	old := &Snapshot{Tables: []SnapTable{
		{Schema: "public", Name: "t", Kind: "table",
			Columns: []SnapColumn{col("col", "text")},
			Indexes: []SnapIndex{idx},
		},
	}}
	new := snapWithTable(baseTable("public", "t", col("col", "text")))

	cs := Diff(old, new)
	drops := findChanges(cs, ChangeDropIndex)
	testutil.Equal(t, 1, len(drops))
	testutil.Equal(t, "old_idx", drops[0].Index.Name)
}

func TestDiff_addForeignKey(t *testing.T) {
	t.Parallel()

	fk := SnapForeignKey{
		ConstraintName:    "fk_posts_author",
		Columns:           []string{"author_id"},
		ReferencedSchema:  "public",
		ReferencedTable:   "users",
		ReferencedColumns: []string{"id"},
		OnDelete:          "CASCADE",
	}

	old := snapWithTable(SnapTable{Schema: "public", Name: "posts", Kind: "table",
		Columns: []SnapColumn{col("id", "uuid"), col("author_id", "uuid")}})
	new := &Snapshot{Tables: []SnapTable{{Schema: "public", Name: "posts", Kind: "table",
		Columns:     []SnapColumn{col("id", "uuid"), col("author_id", "uuid")},
		ForeignKeys: []SnapForeignKey{fk},
	}}}

	cs := Diff(old, new)
	adds := findChanges(cs, ChangeAddForeignKey)
	testutil.Equal(t, 1, len(adds))
	testutil.Equal(t, "fk_posts_author", adds[0].ForeignKey.ConstraintName)
	testutil.Equal(t, "CASCADE", adds[0].ForeignKey.OnDelete)
}

func TestDiff_dropForeignKey(t *testing.T) {
	t.Parallel()

	fk := SnapForeignKey{ConstraintName: "fk_old", Columns: []string{"user_id"},
		ReferencedSchema: "public", ReferencedTable: "users", ReferencedColumns: []string{"id"}}

	old := &Snapshot{Tables: []SnapTable{{Schema: "public", Name: "t", Kind: "table",
		Columns: []SnapColumn{col("user_id", "uuid")}, ForeignKeys: []SnapForeignKey{fk}}}}
	new := snapWithTable(baseTable("public", "t", col("user_id", "uuid")))

	cs := Diff(old, new)
	drops := findChanges(cs, ChangeDropForeignKey)
	testutil.Equal(t, 1, len(drops))
	testutil.Equal(t, "fk_old", drops[0].ForeignKey.ConstraintName)
}

func TestDiff_addCheckConstraint(t *testing.T) {
	t.Parallel()

	cc := SnapCheckConstraint{Name: "ck_age", Definition: "age >= 0"}

	old := snapWithTable(baseTable("public", "users", col("age", "integer")))
	new := &Snapshot{Tables: []SnapTable{{Schema: "public", Name: "users", Kind: "table",
		Columns:          []SnapColumn{col("age", "integer")},
		CheckConstraints: []SnapCheckConstraint{cc},
	}}}

	cs := Diff(old, new)
	adds := findChanges(cs, ChangeAddCheckConstraint)
	testutil.Equal(t, 1, len(adds))
	testutil.Equal(t, "ck_age", adds[0].CheckConstraint.Name)
	testutil.Equal(t, "age >= 0", adds[0].CheckConstraint.Definition)
}

func TestDiff_dropCheckConstraint(t *testing.T) {
	t.Parallel()

	cc := SnapCheckConstraint{Name: "ck_old", Definition: "val > 0"}

	old := &Snapshot{Tables: []SnapTable{{Schema: "public", Name: "t", Kind: "table",
		Columns:          []SnapColumn{col("val", "integer")},
		CheckConstraints: []SnapCheckConstraint{cc},
	}}}
	new := snapWithTable(baseTable("public", "t", col("val", "integer")))

	cs := Diff(old, new)
	drops := findChanges(cs, ChangeDropCheckConstraint)
	testutil.Equal(t, 1, len(drops))
	testutil.Equal(t, "ck_old", drops[0].CheckConstraint.Name)
}

func TestDiff_addRLSPolicy(t *testing.T) {
	t.Parallel()

	pol := SnapRLSPolicy{Name: "users_own", Command: "SELECT", Permissive: true,
		Roles: []string{"authenticated"}, UsingExpr: "(uid = auth.uid())"}

	old := snapWithTable(baseTable("public", "users", col("id", "uuid")))
	new := &Snapshot{Tables: []SnapTable{{Schema: "public", Name: "users", Kind: "table",
		Columns:     []SnapColumn{col("id", "uuid")},
		RLSPolicies: []SnapRLSPolicy{pol},
	}}}

	cs := Diff(old, new)
	adds := findChanges(cs, ChangeAddRLSPolicy)
	testutil.Equal(t, 1, len(adds))
	testutil.Equal(t, "users_own", adds[0].RLSPolicy.Name)
	testutil.Equal(t, "SELECT", adds[0].RLSPolicy.Command)
}

func TestDiff_dropRLSPolicy(t *testing.T) {
	t.Parallel()

	pol := SnapRLSPolicy{Name: "old_pol", Command: "ALL", Permissive: true, Roles: []string{"public"}}

	old := &Snapshot{Tables: []SnapTable{{Schema: "public", Name: "t", Kind: "table",
		Columns:     []SnapColumn{col("id", "uuid")},
		RLSPolicies: []SnapRLSPolicy{pol},
	}}}
	new := snapWithTable(baseTable("public", "t", col("id", "uuid")))

	cs := Diff(old, new)
	drops := findChanges(cs, ChangeDropRLSPolicy)
	testutil.Equal(t, 1, len(drops))
	testutil.Equal(t, "old_pol", drops[0].RLSPolicy.Name)
}

func TestDiff_createEnum(t *testing.T) {
	t.Parallel()

	new := &Snapshot{
		Enums: []SnapEnum{{Schema: "public", Name: "status", Values: []string{"active", "inactive"}}},
	}

	cs := Diff(&Snapshot{}, new)
	creates := findChanges(cs, ChangeCreateEnum)
	testutil.Equal(t, 1, len(creates))
	testutil.Equal(t, "status", creates[0].EnumName)
	testutil.Equal(t, 2, len(creates[0].EnumValues))
}

func TestDiff_enumAddValue(t *testing.T) {
	t.Parallel()

	old := &Snapshot{
		Enums: []SnapEnum{{Schema: "public", Name: "status", Values: []string{"active", "inactive"}}},
	}
	new := &Snapshot{
		Enums: []SnapEnum{{Schema: "public", Name: "status", Values: []string{"active", "inactive", "pending"}}},
	}

	cs := Diff(old, new)
	adds := findChanges(cs, ChangeAlterEnumAddValue)
	testutil.Equal(t, 1, len(adds))
	testutil.Equal(t, "pending", adds[0].NewValue)
}

func TestDiff_enableExtension(t *testing.T) {
	t.Parallel()

	new := &Snapshot{
		Extensions: []SnapExtension{{Name: "pgvector", Version: "0.7.0"}},
	}

	cs := Diff(&Snapshot{}, new)
	enables := findChanges(cs, ChangeEnableExtension)
	testutil.Equal(t, 1, len(enables))
	testutil.Equal(t, "pgvector", enables[0].ExtensionName)
}

func TestDiff_disableExtension(t *testing.T) {
	t.Parallel()

	old := &Snapshot{
		Extensions: []SnapExtension{{Name: "pgvector", Version: "0.7.0"}},
	}

	cs := Diff(old, &Snapshot{})
	disables := findChanges(cs, ChangeDisableExtension)
	testutil.Equal(t, 1, len(disables))
	testutil.Equal(t, "pgvector", disables[0].ExtensionName)
}

func TestDiff_multiTableSimultaneous(t *testing.T) {
	t.Parallel()

	old := &Snapshot{
		Tables: []SnapTable{
			baseTable("public", "a", col("id", "uuid")),
			baseTable("public", "b", col("id", "uuid"), col("name", "text")),
		},
	}
	new := &Snapshot{
		Tables: []SnapTable{
			baseTable("public", "a", col("id", "uuid"), col("extra", "integer")), // added col
			baseTable("public", "c", col("id", "uuid")),                          // new table
			// b dropped
		},
	}

	cs := Diff(old, new)

	testutil.NotNil(t, findChange(cs, ChangeAddColumn))
	testutil.Equal(t, "extra", findChange(cs, ChangeAddColumn).ColumnName)
	testutil.NotNil(t, findChange(cs, ChangeCreateTable))
	testutil.Equal(t, "c", findChange(cs, ChangeCreateTable).TableName)
	testutil.NotNil(t, findChange(cs, ChangeDropTable))
	testutil.Equal(t, "b", findChange(cs, ChangeDropTable).TableName)
}

func TestDiff_createTableIncludesNonPrimaryIndexes(t *testing.T) {
	t.Parallel()

	newSnap := &Snapshot{Tables: []SnapTable{{
		Schema: "public",
		Name:   "orders",
		Kind:   "table",
		Columns: []SnapColumn{
			{Name: "id", TypeName: "uuid", IsPrimaryKey: true},
			{Name: "user_id", TypeName: "uuid"},
		},
		Indexes: []SnapIndex{
			{Name: "orders_pkey", IsPrimary: true, IsUnique: true, Method: "btree"},
			{Name: "orders_user_idx", IsUnique: false, Method: "btree",
				Definition: "CREATE INDEX orders_user_idx ON public.orders USING btree (user_id)"},
		},
	}}}

	cs := Diff(&Snapshot{}, newSnap)
	creates := findChanges(cs, ChangeCreateTable)
	testutil.Equal(t, 1, len(creates))
	idxCreates := findChanges(cs, ChangeCreateIndex)
	testutil.Equal(t, 1, len(idxCreates)) // only non-primary
	testutil.Equal(t, "orders_user_idx", idxCreates[0].Index.Name)
}
