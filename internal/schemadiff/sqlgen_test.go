package schemadiff

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// assertContains checks that the SQL string contains the expected substring.
func assertSQL(t testing.TB, sql, want string) {
	t.Helper()
	if !strings.Contains(sql, want) {
		t.Errorf("SQL does not contain %q\nGot:\n%s", want, sql)
	}
}

func TestGenerateUp_createTable(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{
		{
			Type:       ChangeCreateTable,
			SchemaName: "public",
			TableName:  "users",
			AllColumns: []SnapColumn{
				{Name: "id", TypeName: "uuid", IsPrimaryKey: true},
				{Name: "email", TypeName: "text", IsNullable: false},
				{Name: "bio", TypeName: "text", IsNullable: true},
			},
		},
	}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `CREATE TABLE IF NOT EXISTS "public"."users"`)
	assertSQL(t, sql, `"id" uuid NOT NULL`)
	assertSQL(t, sql, `"email" text NOT NULL`)
	assertSQL(t, sql, `"bio" text`)
	assertSQL(t, sql, `PRIMARY KEY ("id")`)
}

func TestGenerateDown_createTable(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{
		{
			Type:       ChangeCreateTable,
			SchemaName: "public",
			TableName:  "users",
			AllColumns: []SnapColumn{{Name: "id", TypeName: "uuid"}},
		},
	}

	sql := GenerateDown(cs)
	assertSQL(t, sql, `DROP TABLE IF EXISTS "public"."users"`)
}

func TestGenerateUp_dropTable(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{Type: ChangeDropTable, SchemaName: "public", TableName: "legacy"}}
	sql := GenerateUp(cs)
	assertSQL(t, sql, `DROP TABLE IF EXISTS "public"."legacy"`)
}

func TestGenerateUp_addColumn(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:        ChangeAddColumn,
		SchemaName:  "public",
		TableName:   "users",
		ColumnName:  "email",
		NewTypeName: "text",
		NewNullable: false,
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `ALTER TABLE "public"."users" ADD COLUMN IF NOT EXISTS "email" text NOT NULL`)
}

func TestGenerateUp_addColumnWithDefault(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:        ChangeAddColumn,
		SchemaName:  "public",
		TableName:   "users",
		ColumnName:  "score",
		NewTypeName: "integer",
		NewNullable: false,
		NewDefault:  "0",
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `DEFAULT 0`)
}

func TestGenerateDown_addColumn(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeAddColumn,
		SchemaName: "public",
		TableName:  "users",
		ColumnName: "email",
	}}

	sql := GenerateDown(cs)
	assertSQL(t, sql, `ALTER TABLE "public"."users" DROP COLUMN IF EXISTS "email"`)
}

func TestGenerateUp_dropColumn(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:        ChangeDropColumn,
		SchemaName:  "public",
		TableName:   "users",
		ColumnName:  "old_col",
		OldTypeName: "text",
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `ALTER TABLE "public"."users" DROP COLUMN IF EXISTS "old_col"`)
}

func TestGenerateUp_alterColumnType(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:        ChangeAlterColumnType,
		SchemaName:  "public",
		TableName:   "products",
		ColumnName:  "price",
		OldTypeName: "integer",
		NewTypeName: "numeric",
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `ALTER TABLE "public"."products" ALTER COLUMN "price" TYPE numeric USING "price"::numeric`)
}

func TestGenerateDown_alterColumnType(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:        ChangeAlterColumnType,
		SchemaName:  "public",
		TableName:   "products",
		ColumnName:  "price",
		OldTypeName: "integer",
		NewTypeName: "numeric",
	}}

	sql := GenerateDown(cs)
	assertSQL(t, sql, `ALTER TABLE "public"."products" ALTER COLUMN "price" TYPE integer USING "price"::integer`)
}

func TestGenerateUp_alterColumnDefault_set(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeAlterColumnDefault,
		SchemaName: "public",
		TableName:  "users",
		ColumnName: "status",
		OldDefault: "",
		NewDefault: "'active'",
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `SET DEFAULT 'active'`)
}

func TestGenerateUp_alterColumnDefault_drop(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeAlterColumnDefault,
		SchemaName: "public",
		TableName:  "users",
		ColumnName: "status",
		OldDefault: "'active'",
		NewDefault: "",
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `DROP DEFAULT`)
}

func TestGenerateUp_alterColumnNullable_dropNotNull(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:        ChangeAlterColumnNullable,
		SchemaName:  "public",
		TableName:   "users",
		ColumnName:  "bio",
		OldNullable: false,
		NewNullable: true,
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `DROP NOT NULL`)
}

func TestGenerateUp_alterColumnNullable_setNotNull(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:        ChangeAlterColumnNullable,
		SchemaName:  "public",
		TableName:   "users",
		ColumnName:  "bio",
		OldNullable: true,
		NewNullable: false,
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `SET NOT NULL`)
}

func TestGenerateUp_createIndex(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeCreateIndex,
		SchemaName: "public",
		TableName:  "users",
		Index: SnapIndex{
			Name:       "users_email_idx",
			IsUnique:   true,
			Method:     "btree",
			Definition: "CREATE UNIQUE INDEX users_email_idx ON public.users USING btree (email)",
		},
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, "CREATE UNIQUE INDEX users_email_idx ON public.users USING btree (email)")
}

func TestGenerateUp_dropIndex(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeDropIndex,
		SchemaName: "public",
		TableName:  "users",
		Index:      SnapIndex{Name: "old_idx"},
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `DROP INDEX IF EXISTS "public"."old_idx"`)
}

func TestGenerateUp_addForeignKey(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeAddForeignKey,
		SchemaName: "public",
		TableName:  "posts",
		ForeignKey: SnapForeignKey{
			ConstraintName:    "fk_posts_author",
			Columns:           []string{"author_id"},
			ReferencedSchema:  "public",
			ReferencedTable:   "users",
			ReferencedColumns: []string{"id"},
			OnDelete:          "CASCADE",
		},
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `ADD CONSTRAINT "fk_posts_author" FOREIGN KEY ("author_id") REFERENCES "public"."users" ("id") ON DELETE CASCADE`)
}

func TestGenerateDown_addForeignKey(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeAddForeignKey,
		SchemaName: "public",
		TableName:  "posts",
		ForeignKey: SnapForeignKey{ConstraintName: "fk_posts_author"},
	}}

	sql := GenerateDown(cs)
	assertSQL(t, sql, `DROP CONSTRAINT IF EXISTS "fk_posts_author"`)
}

func TestGenerateUp_addCheckConstraint(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeAddCheckConstraint,
		SchemaName: "public",
		TableName:  "users",
		CheckConstraint: SnapCheckConstraint{
			Name:       "ck_age",
			Definition: "age >= 0",
		},
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `ADD CONSTRAINT "ck_age" CHECK (age >= 0)`)
}

func TestGenerateDown_addCheckConstraint(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:            ChangeAddCheckConstraint,
		SchemaName:      "public",
		TableName:       "users",
		CheckConstraint: SnapCheckConstraint{Name: "ck_age"},
	}}

	sql := GenerateDown(cs)
	assertSQL(t, sql, `DROP CONSTRAINT IF EXISTS "ck_age"`)
}

func TestGenerateUp_dropCheckConstraint(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:            ChangeDropCheckConstraint,
		SchemaName:      "public",
		TableName:       "users",
		CheckConstraint: SnapCheckConstraint{Name: "ck_old", Definition: "val > 0"},
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `DROP CONSTRAINT IF EXISTS "ck_old"`)
}

func TestGenerateDown_dropCheckConstraint_restores(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:            ChangeDropCheckConstraint,
		SchemaName:      "public",
		TableName:       "users",
		CheckConstraint: SnapCheckConstraint{Name: "ck_old", Definition: "val > 0"},
	}}

	sql := GenerateDown(cs)
	assertSQL(t, sql, `ADD CONSTRAINT "ck_old" CHECK (val > 0)`)
}

func TestGenerateUp_createEnum(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeCreateEnum,
		EnumSchema: "public",
		EnumName:   "status",
		EnumValues: []string{"active", "inactive"},
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `CREATE TYPE "public"."status" AS ENUM ('active', 'inactive')`)
}

func TestGenerateDown_createEnum(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeCreateEnum,
		EnumSchema: "public",
		EnumName:   "status",
		EnumValues: []string{"active"},
	}}

	sql := GenerateDown(cs)
	assertSQL(t, sql, `DROP TYPE IF EXISTS "public"."status"`)
}

func TestGenerateUp_alterEnumAddValue(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeAlterEnumAddValue,
		EnumSchema: "public",
		EnumName:   "status",
		NewValue:   "pending",
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `ALTER TYPE "public"."status" ADD VALUE IF NOT EXISTS 'pending'`)
}

func TestGenerateDown_alterEnumAddValue_isComment(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeAlterEnumAddValue,
		EnumSchema: "public",
		EnumName:   "status",
		NewValue:   "pending",
	}}

	sql := GenerateDown(cs)
	assertSQL(t, sql, "-- Cannot remove enum value")
}

func TestGenerateUp_addRLSPolicy(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeAddRLSPolicy,
		SchemaName: "public",
		TableName:  "users",
		RLSPolicy: SnapRLSPolicy{
			Name:       "users_own",
			Command:    "SELECT",
			Permissive: true,
			Roles:      []string{"authenticated"},
			UsingExpr:  "auth.uid() = id",
		},
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `CREATE POLICY "users_own" ON "public"."users"`)
	assertSQL(t, sql, "AS PERMISSIVE")
	assertSQL(t, sql, "FOR SELECT")
	assertSQL(t, sql, "TO authenticated")
	assertSQL(t, sql, "USING (auth.uid() = id)")
}

func TestGenerateUp_dropRLSPolicy(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeDropRLSPolicy,
		SchemaName: "public",
		TableName:  "users",
		RLSPolicy:  SnapRLSPolicy{Name: "old_pol"},
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, `DROP POLICY IF EXISTS "old_pol" ON "public"."users"`)
}

func TestGenerateUp_enableExtension(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{Type: ChangeEnableExtension, ExtensionName: "pgvector"}}
	sql := GenerateUp(cs)
	assertSQL(t, sql, `CREATE EXTENSION IF NOT EXISTS "pgvector"`)
}

func TestGenerateUp_disableExtension(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{Type: ChangeDisableExtension, ExtensionName: "pgvector"}}
	sql := GenerateUp(cs)
	assertSQL(t, sql, `DROP EXTENSION IF EXISTS "pgvector"`)
}

func TestGenerateDown_enableExtension(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{Type: ChangeEnableExtension, ExtensionName: "pgvector"}}
	sql := GenerateDown(cs)
	assertSQL(t, sql, `DROP EXTENSION IF EXISTS "pgvector"`)
}

func TestGenerateDown_disableExtension(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{Type: ChangeDisableExtension, ExtensionName: "pgvector"}}
	sql := GenerateDown(cs)
	assertSQL(t, sql, `CREATE EXTENSION IF NOT EXISTS "pgvector"`)
}

func TestGenerateUp_restrictivePolicyFlag(t *testing.T) {
	t.Parallel()

	cs := ChangeSet{{
		Type:       ChangeAddRLSPolicy,
		SchemaName: "public",
		TableName:  "t",
		RLSPolicy: SnapRLSPolicy{
			Name:       "restrict_pol",
			Command:    "ALL",
			Permissive: false,
			Roles:      []string{"public"},
		},
	}}

	sql := GenerateUp(cs)
	assertSQL(t, sql, "AS RESTRICTIVE")
}

func TestGenerateUp_emptyChangeset(t *testing.T) {
	t.Parallel()

	testutil.Equal(t, "", GenerateUp(nil))
	testutil.Equal(t, "", GenerateDown(nil))
}

func TestGenerateDown_reverseOrder(t *testing.T) {
	t.Parallel()

	// Create table then add column — down should drop column first, then table.
	cs := ChangeSet{
		{
			Type:       ChangeCreateTable,
			SchemaName: "public",
			TableName:  "t",
			AllColumns: []SnapColumn{{Name: "id", TypeName: "uuid"}},
		},
		{
			Type:        ChangeAddColumn,
			SchemaName:  "public",
			TableName:   "t",
			ColumnName:  "extra",
			NewTypeName: "text",
		},
	}

	sql := GenerateDown(cs)
	// "DROP COLUMN" should appear before "DROP TABLE" in the output.
	dropCol := strings.Index(sql, "DROP COLUMN")
	dropTable := strings.Index(sql, "DROP TABLE")
	if dropCol == -1 || dropTable == -1 {
		t.Fatal("expected both DROP COLUMN and DROP TABLE in down SQL")
	}
	if dropCol > dropTable {
		t.Errorf("expected DROP COLUMN before DROP TABLE in down SQL\nGot:\n%s", sql)
	}
}
