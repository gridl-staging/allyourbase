package typegen

import (
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func newCache(tables map[string]*schema.Table) *schema.SchemaCache {
	return &schema.SchemaCache{
		Tables:  tables,
		Schemas: []string{"public"},
		BuiltAt: time.Now(),
	}
}

func TestTypeScriptBasicTable(t *testing.T) {
	t.Parallel()
	sc := newCache(map[string]*schema.Table{
		"public.posts": {
			Schema: "public", Name: "posts", Kind: "table",
			Columns: []*schema.Column{
				{Name: "id", Position: 1, JSONType: "integer", IsPrimaryKey: true, DefaultExpr: "nextval('posts_id_seq')"},
				{Name: "title", Position: 2, JSONType: "string"},
				{Name: "content", Position: 3, JSONType: "string", IsNullable: true},
				{Name: "published", Position: 4, JSONType: "boolean"},
				{Name: "created_at", Position: 5, JSONType: "string", DefaultExpr: "now()"},
			},
			PrimaryKey: []string{"id"},
		},
	})

	out := TypeScript(sc)

	testutil.Contains(t, out, "export interface Posts {")
	testutil.Contains(t, out, "  id: number;")
	testutil.Contains(t, out, "  title: string;")
	testutil.Contains(t, out, "  content: string | null;")
	testutil.Contains(t, out, "  published: boolean;")
	testutil.Contains(t, out, "  created_at: string;")
	testutil.Contains(t, out, `export type PostsCreate = Omit<Posts, "id" | "created_at">;`)
	testutil.Contains(t, out, "export type PostsUpdate = Partial<PostsCreate>;")
}

func TestTypeScriptAllJSONTypes(t *testing.T) {
	t.Parallel()
	sc := newCache(map[string]*schema.Table{
		"public.all_types": {
			Schema: "public", Name: "all_types", Kind: "table",
			Columns: []*schema.Column{
				{Name: "str_col", Position: 1, JSONType: "string"},
				{Name: "int_col", Position: 2, JSONType: "integer"},
				{Name: "num_col", Position: 3, JSONType: "number"},
				{Name: "bool_col", Position: 4, JSONType: "boolean"},
				{Name: "obj_col", Position: 5, JSONType: "object"},
				{Name: "arr_col", Position: 6, JSONType: "array"},
			},
		},
	})

	out := TypeScript(sc)

	testutil.Contains(t, out, "  str_col: string;")
	testutil.Contains(t, out, "  int_col: number;")
	testutil.Contains(t, out, "  num_col: number;")
	testutil.Contains(t, out, "  bool_col: boolean;")
	testutil.Contains(t, out, "  obj_col: Record<string, unknown>;")
	testutil.Contains(t, out, "  arr_col: unknown[];")
}

func TestTypeScriptNullableColumns(t *testing.T) {
	t.Parallel()
	sc := newCache(map[string]*schema.Table{
		"public.nullable": {
			Schema: "public", Name: "nullable", Kind: "table",
			Columns: []*schema.Column{
				{Name: "a", Position: 1, JSONType: "string", IsNullable: true},
				{Name: "b", Position: 2, JSONType: "integer", IsNullable: true},
				{Name: "c", Position: 3, JSONType: "boolean", IsNullable: true},
			},
		},
	})

	out := TypeScript(sc)

	testutil.Contains(t, out, "  a: string | null;")
	testutil.Contains(t, out, "  b: number | null;")
	testutil.Contains(t, out, "  c: boolean | null;")
}

func TestTypeScriptNoNullableColumns(t *testing.T) {
	t.Parallel()
	sc := newCache(map[string]*schema.Table{
		"public.strict": {
			Schema: "public", Name: "strict", Kind: "table",
			Columns: []*schema.Column{
				{Name: "x", Position: 1, JSONType: "string"},
				{Name: "y", Position: 2, JSONType: "integer"},
			},
		},
	})

	out := TypeScript(sc)

	testutil.False(t, strings.Contains(out, "| null"), "should have no null types")
}

func TestTypeScriptEnumType(t *testing.T) {
	t.Parallel()
	sc := newCache(map[string]*schema.Table{
		"public.tasks": {
			Schema: "public", Name: "tasks", Kind: "table",
			Columns: []*schema.Column{
				{Name: "id", Position: 1, JSONType: "integer", IsPrimaryKey: true},
				{Name: "status", Position: 2, JSONType: "string", TypeName: "task_status", IsEnum: true, EnumValues: []string{"pending", "active", "done"}},
			},
			PrimaryKey: []string{"id"},
		},
	})

	out := TypeScript(sc)

	testutil.Contains(t, out, `export type TaskStatus = "pending" | "active" | "done";`)
	testutil.Contains(t, out, "  status: TaskStatus;")
}

func TestTypeScriptSystemTablesExcluded(t *testing.T) {
	t.Parallel()
	sc := newCache(map[string]*schema.Table{
		"public._ayb_users": {
			Schema: "public", Name: "_ayb_users", Kind: "table",
			Columns: []*schema.Column{
				{Name: "id", Position: 1, JSONType: "string"},
			},
		},
		"public._ayb_sessions": {
			Schema: "public", Name: "_ayb_sessions", Kind: "table",
			Columns: []*schema.Column{
				{Name: "id", Position: 1, JSONType: "string"},
			},
		},
		"public.posts": {
			Schema: "public", Name: "posts", Kind: "table",
			Columns: []*schema.Column{
				{Name: "id", Position: 1, JSONType: "integer"},
			},
		},
	})

	out := TypeScript(sc)

	testutil.False(t, strings.Contains(out, "AybUsers"), "system table _ayb_users should be excluded")
	testutil.False(t, strings.Contains(out, "AybSessions"), "system table _ayb_sessions should be excluded")
	testutil.Contains(t, out, "export interface Posts {")
}

func TestTypeScriptEmptySchema(t *testing.T) {
	t.Parallel()
	sc := newCache(map[string]*schema.Table{})

	out := TypeScript(sc)

	testutil.Contains(t, out, "DO NOT EDIT")
	// Should be valid output with just the header.
	testutil.False(t, strings.Contains(out, "export interface"), "empty schema should have no interfaces")
}

func TestTypeScriptCreateOmitsPKAndDefaults(t *testing.T) {
	t.Parallel()
	sc := newCache(map[string]*schema.Table{
		"public.items": {
			Schema: "public", Name: "items", Kind: "table",
			Columns: []*schema.Column{
				{Name: "id", Position: 1, JSONType: "integer", IsPrimaryKey: true, DefaultExpr: "nextval('items_id_seq')"},
				{Name: "name", Position: 2, JSONType: "string"},
				{Name: "created_at", Position: 3, JSONType: "string", DefaultExpr: "now()"},
				{Name: "updated_at", Position: 4, JSONType: "string", DefaultExpr: "now()"},
			},
			PrimaryKey: []string{"id"},
		},
	})

	out := TypeScript(sc)

	testutil.Contains(t, out, `export type ItemsCreate = Omit<Items, "id" | "created_at" | "updated_at">;`)
}

func TestTypeScriptCreateNoOmitWhenNoDefaults(t *testing.T) {
	t.Parallel()
	sc := newCache(map[string]*schema.Table{
		"public.tags": {
			Schema: "public", Name: "tags", Kind: "table",
			Columns: []*schema.Column{
				{Name: "name", Position: 1, JSONType: "string"},
				{Name: "color", Position: 2, JSONType: "string"},
			},
		},
	})

	out := TypeScript(sc)

	testutil.Contains(t, out, "export type TagsCreate = Tags;")
}

func TestTypeScriptComments(t *testing.T) {
	t.Parallel()
	sc := newCache(map[string]*schema.Table{
		"public.docs": {
			Schema: "public", Name: "docs", Kind: "table",
			Comment: "Documentation entries",
			Columns: []*schema.Column{
				{Name: "id", Position: 1, JSONType: "integer", IsPrimaryKey: true},
				{Name: "body", Position: 2, JSONType: "string", Comment: "Markdown content"},
			},
			PrimaryKey: []string{"id"},
		},
	})

	out := TypeScript(sc)

	testutil.Contains(t, out, "/** Documentation entries */")
	testutil.Contains(t, out, "/** Markdown content */")
}

func TestTypeScriptMultipleTablesSorted(t *testing.T) {
	t.Parallel()
	sc := newCache(map[string]*schema.Table{
		"public.zebras": {Schema: "public", Name: "zebras", Kind: "table", Columns: []*schema.Column{{Name: "id", Position: 1, JSONType: "integer"}}},
		"public.apples": {Schema: "public", Name: "apples", Kind: "table", Columns: []*schema.Column{{Name: "id", Position: 1, JSONType: "integer"}}},
		"public.middle": {Schema: "public", Name: "middle", Kind: "table", Columns: []*schema.Column{{Name: "id", Position: 1, JSONType: "integer"}}},
	})

	out := TypeScript(sc)

	// Apples should come before Middle, Middle before Zebras.
	applesIdx := strings.Index(out, "export interface Apples")
	middleIdx := strings.Index(out, "export interface Middle")
	zebrasIdx := strings.Index(out, "export interface Zebras")
	testutil.True(t, applesIdx < middleIdx, "Apples should come before Middle")
	testutil.True(t, middleIdx < zebrasIdx, "Middle should come before Zebras")
}

func TestPascalCase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{"posts", "Posts"},
		{"user_profiles", "UserProfiles"},
		{"_ayb_users", "AybUsers"},
		{"task_status", "TaskStatus"},
		{"already", "Already"},
		{"a_b_c", "ABC"},
		{"with-dashes", "WithDashes"},
		{"with spaces", "WithSpaces"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.want, pascalCase(tt.in))
		})
	}
}

func TestTypeScriptUnknownJSONType(t *testing.T) {
	t.Parallel()
	sc := newCache(map[string]*schema.Table{
		"public.misc": {
			Schema: "public", Name: "misc", Kind: "table",
			Columns: []*schema.Column{
				{Name: "data", Position: 1, JSONType: "somethingelse"},
			},
		},
	})

	out := TypeScript(sc)

	// Unknown JSON types fall through to string.
	testutil.Contains(t, out, "  data: string;")
}

func TestTypeScriptNullableEnum(t *testing.T) {
	t.Parallel()
	sc := newCache(map[string]*schema.Table{
		"public.jobs": {
			Schema: "public", Name: "jobs", Kind: "table",
			Columns: []*schema.Column{
				{Name: "id", Position: 1, JSONType: "integer", IsPrimaryKey: true},
				{Name: "priority", Position: 2, JSONType: "string", TypeName: "priority_level", IsEnum: true, EnumValues: []string{"low", "medium", "high"}, IsNullable: true},
			},
			PrimaryKey: []string{"id"},
		},
	})

	out := TypeScript(sc)

	testutil.Contains(t, out, `export type PriorityLevel = "low" | "medium" | "high";`)
	testutil.Contains(t, out, "  priority: PriorityLevel | null;")
}

func TestTypeScriptCompositePrimaryKey(t *testing.T) {
	t.Parallel()
	sc := newCache(map[string]*schema.Table{
		"public.order_items": {
			Schema: "public", Name: "order_items", Kind: "table",
			Columns: []*schema.Column{
				{Name: "order_id", Position: 1, JSONType: "integer", IsPrimaryKey: true},
				{Name: "product_id", Position: 2, JSONType: "integer", IsPrimaryKey: true},
				{Name: "quantity", Position: 3, JSONType: "integer"},
			},
			PrimaryKey: []string{"order_id", "product_id"},
		},
	})

	out := TypeScript(sc)

	// Both PK columns should be omitted from Create.
	testutil.Contains(t, out, `export type OrderItemsCreate = Omit<OrderItems, "order_id" | "product_id">;`)
}

func TestIsSystemTable(t *testing.T) {
	t.Parallel()
	testutil.True(t, isSystemTable("_ayb_users"), "_ayb_users is system")
	testutil.True(t, isSystemTable("_ayb_sessions"), "_ayb_sessions is system")
	testutil.False(t, isSystemTable("posts"), "posts is not system")
	testutil.False(t, isSystemTable("ayb_data"), "ayb_data is not system (no underscore prefix)")
}
