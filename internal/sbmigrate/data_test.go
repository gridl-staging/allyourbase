package sbmigrate

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestIsInternalTable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		table    string
		internal bool
	}{
		{"supabase prefix", "_supabase_migrations", true},
		{"realtime prefix", "_realtime_broadcasts", true},
		{"analytics prefix", "_analytics_events", true},
		{"pgsodium prefix", "_pgsodium_key", true},
		{"prisma prefix", "_prisma_migrations", true},
		{"schema_migrations", "schema_migrations", true},
		{"supabase_migrations", "supabase_migrations", true},
		{"buckets_vectors", "buckets_vectors", true},
		{"vector_indexes", "vector_indexes", true},
		{"storage.buckets_vectors", "storage.buckets_vectors", true},
		{"user table", "users", false},
		{"posts table", "posts", false},
		{"underscore prefix not matched", "_custom_table", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isInternalTable(tt.table)
			testutil.Equal(t, tt.internal, got)
		})
	}
}

func TestIsAYBTable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		table string
		ayb   bool
	}{
		{"ayb users", "_ayb_users", true},
		{"ayb oauth", "_ayb_oauth_accounts", true},
		{"ayb refresh", "_ayb_refresh_tokens", true},
		{"user table", "users", false},
		{"partial match", "ayb_something", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isAYBTable(tt.table)
			testutil.Equal(t, tt.ayb, got)
		})
	}
}

func TestPgTypeName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{"character varying", "character varying", "varchar"},
		{"character", "character", "char"},
		{"timestamp no tz", "timestamp without time zone", "timestamp"},
		{"timestamp with tz", "timestamp with time zone", "timestamptz"},
		{"time no tz", "time without time zone", "time"},
		{"time with tz", "time with time zone", "timetz"},
		{"double precision", "double precision", "float8"},
		{"boolean", "boolean", "bool"},
		{"ARRAY fallback", "ARRAY", "jsonb"},
		{"USER-DEFINED fallback", "USER-DEFINED", "text"},
		{"integer passthrough", "integer", "integer"},
		{"text passthrough", "text", "text"},
		{"uuid passthrough", "uuid", "uuid"},
		{"jsonb passthrough", "jsonb", "jsonb"},
		{"bigint passthrough", "bigint", "bigint"},
		{"numeric passthrough", "numeric", "numeric"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pgTypeName(tt.input)
			testutil.Equal(t, tt.output, got)
		})
	}
}

func TestCreateTableSQL(t *testing.T) {
	t.Parallel()
	t.Run("simple table with PK", func(t *testing.T) {
		t.Parallel()
		table := TableInfo{
			Name: "posts",
			Columns: []ColumnInfo{
				{Name: "id", DataType: "integer", IsNullable: false, DefaultValue: "nextval('posts_id_seq'::regclass)", OrdinalPos: 1},
				{Name: "title", DataType: "text", IsNullable: false, OrdinalPos: 2},
				{Name: "body", DataType: "text", IsNullable: true, OrdinalPos: 3},
			},
			PrimaryKey: "id",
		}
		got := createTableSQL(table)
		testutil.Contains(t, got, `CREATE TABLE IF NOT EXISTS "posts"`)
		testutil.Contains(t, got, `"id" integer NOT NULL DEFAULT nextval('posts_id_seq'::regclass)`)
		testutil.Contains(t, got, `"title" text NOT NULL`)
		testutil.Contains(t, got, `"body" text`)
		testutil.Contains(t, got, `PRIMARY KEY ("id")`)
	})

	t.Run("table with foreign key", func(t *testing.T) {
		t.Parallel()
		table := TableInfo{
			Name: "comments",
			Columns: []ColumnInfo{
				{Name: "id", DataType: "integer", IsNullable: false, OrdinalPos: 1},
				{Name: "post_id", DataType: "integer", IsNullable: false, OrdinalPos: 2},
				{Name: "content", DataType: "text", IsNullable: false, OrdinalPos: 3},
			},
			PrimaryKey: "id",
			ForeignKeys: []ForeignKeyInfo{
				{ConstraintName: "fk_post", ColumnName: "post_id", RefTable: "posts", RefColumn: "id"},
			},
		}
		got := createTableSQL(table)
		testutil.Contains(t, got, `PRIMARY KEY ("id")`)
		testutil.Contains(t, got, `CONSTRAINT "fk_post" FOREIGN KEY ("post_id") REFERENCES "posts"("id")`)
	})

	t.Run("table with nullable columns", func(t *testing.T) {
		t.Parallel()
		table := TableInfo{
			Name: "profiles",
			Columns: []ColumnInfo{
				{Name: "id", DataType: "uuid", IsNullable: false, OrdinalPos: 1},
				{Name: "bio", DataType: "text", IsNullable: true, OrdinalPos: 2},
				{Name: "avatar_url", DataType: "character varying", IsNullable: true, OrdinalPos: 3},
			},
			PrimaryKey: "id",
		}
		got := createTableSQL(table)
		testutil.Contains(t, got, `"id" uuid NOT NULL`)
		testutil.Contains(t, got, `"bio" text`)
		testutil.Contains(t, got, `"avatar_url" varchar`)
		// bio should NOT have NOT NULL
		testutil.False(t, strings.Contains(got, `"bio" text NOT NULL`), "bio should be nullable")
	})

	t.Run("table without PK", func(t *testing.T) {
		t.Parallel()
		table := TableInfo{
			Name: "events",
			Columns: []ColumnInfo{
				{Name: "event_type", DataType: "text", IsNullable: false, OrdinalPos: 1},
				{Name: "payload", DataType: "jsonb", IsNullable: true, OrdinalPos: 2},
			},
		}
		got := createTableSQL(table)
		testutil.Contains(t, got, `CREATE TABLE IF NOT EXISTS "events"`)
		testutil.False(t, strings.Contains(got, "PRIMARY KEY"), "should not have PRIMARY KEY")
	})

	t.Run("table with multiple FKs", func(t *testing.T) {
		t.Parallel()
		table := TableInfo{
			Name: "order_items",
			Columns: []ColumnInfo{
				{Name: "id", DataType: "integer", IsNullable: false, OrdinalPos: 1},
				{Name: "order_id", DataType: "integer", IsNullable: false, OrdinalPos: 2},
				{Name: "product_id", DataType: "integer", IsNullable: false, OrdinalPos: 3},
			},
			PrimaryKey: "id",
			ForeignKeys: []ForeignKeyInfo{
				{ConstraintName: "fk_order", ColumnName: "order_id", RefTable: "orders", RefColumn: "id"},
				{ConstraintName: "fk_product", ColumnName: "product_id", RefTable: "products", RefColumn: "id"},
			},
		}
		got := createTableSQL(table)
		testutil.Contains(t, got, `CONSTRAINT "fk_order"`)
		testutil.Contains(t, got, `CONSTRAINT "fk_product"`)
	})

	t.Run("empty table", func(t *testing.T) {
		t.Parallel()
		table := TableInfo{
			Name:    "empty",
			Columns: nil,
		}
		got := createTableSQL(table)
		testutil.Contains(t, got, `CREATE TABLE IF NOT EXISTS "empty"`)
		testutil.Contains(t, got, ");")
	})

	t.Run("type mappings in DDL", func(t *testing.T) {
		t.Parallel()
		table := TableInfo{
			Name: "typed",
			Columns: []ColumnInfo{
				{Name: "ts", DataType: "timestamp with time zone", IsNullable: true, OrdinalPos: 1},
				{Name: "flag", DataType: "boolean", IsNullable: false, OrdinalPos: 2},
				{Name: "score", DataType: "double precision", IsNullable: true, OrdinalPos: 3},
				{Name: "name", DataType: "character varying", IsNullable: false, OrdinalPos: 4},
			},
		}
		got := createTableSQL(table)
		testutil.Contains(t, got, `"ts" timestamptz`)
		testutil.Contains(t, got, `"flag" bool NOT NULL`)
		testutil.Contains(t, got, `"score" float8`)
		testutil.Contains(t, got, `"name" varchar NOT NULL`)
	})
}

func TestCreateViewSQL(t *testing.T) {
	t.Parallel()
	view := ViewInfo{
		Name:       "active_users",
		Definition: "SELECT id, email FROM users WHERE active = true",
	}
	got := createViewSQL(view)
	testutil.Equal(t, `CREATE OR REPLACE VIEW "active_users" AS SELECT id, email FROM users WHERE active = true`, got)
}

func TestPhaseCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts MigrationOptions
		want int
	}{
		{
			name: "all phases enabled",
			opts: MigrationOptions{},
			want: 5, // schema + data + auth + oauth + rls
		},
		{
			name: "skip data",
			opts: MigrationOptions{SkipData: true},
			want: 3, // auth + oauth + rls
		},
		{
			name: "skip oauth",
			opts: MigrationOptions{SkipOAuth: true},
			want: 4, // schema + data + auth + rls
		},
		{
			name: "skip rls",
			opts: MigrationOptions{SkipRLS: true},
			want: 4, // schema + data + auth + oauth
		},
		{
			name: "skip all optional",
			opts: MigrationOptions{SkipData: true, SkipOAuth: true, SkipRLS: true},
			want: 1, // auth only
		},
		{
			name: "skip data and oauth",
			opts: MigrationOptions{SkipData: true, SkipOAuth: true},
			want: 2, // auth + rls
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := &Migrator{opts: tt.opts}
			got := m.phaseCount()
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestBuildValidationSummary(t *testing.T) {
	t.Parallel()
	t.Run("full migration", func(t *testing.T) {
		t.Parallel()
		report := &migrate.AnalysisReport{
			Tables:      5,
			Views:       2,
			Records:     1000,
			AuthUsers:   50,
			OAuthLinks:  10,
			RLSPolicies: 3,
		}
		stats := &MigrationStats{
			Tables:     5,
			Views:      2,
			Records:    1000,
			Users:      50,
			OAuthLinks: 10,
			Policies:   3,
		}
		summary := BuildValidationSummary(report, stats)
		testutil.Equal(t, "Supabase (source)", summary.SourceLabel)
		testutil.Equal(t, "AYB (target)", summary.TargetLabel)
		testutil.Equal(t, 6, len(summary.Rows))
		// All counts match, no warnings.
		testutil.Equal(t, 0, len(summary.Warnings))

		// Verify row labels and values.
		testutil.Equal(t, "Tables", summary.Rows[0].Label)
		testutil.Equal(t, 5, summary.Rows[0].SourceCount)
		testutil.Equal(t, 5, summary.Rows[0].TargetCount)

		testutil.Equal(t, "Views", summary.Rows[1].Label)
		testutil.Equal(t, "Records", summary.Rows[2].Label)
		testutil.Equal(t, "Auth users", summary.Rows[3].Label)
		testutil.Equal(t, "OAuth links", summary.Rows[4].Label)
		testutil.Equal(t, "RLS policies", summary.Rows[5].Label)
	})

	t.Run("auth only (no data)", func(t *testing.T) {
		t.Parallel()
		report := &migrate.AnalysisReport{
			AuthUsers: 10,
		}
		stats := &MigrationStats{
			Users: 10,
		}
		summary := BuildValidationSummary(report, stats)
		// Should only have auth users row (tables/views/records/oauth/rls all zero).
		testutil.Equal(t, 1, len(summary.Rows))
		testutil.Equal(t, "Auth users", summary.Rows[0].Label)
	})

	t.Run("with skipped and errors", func(t *testing.T) {
		t.Parallel()
		report := &migrate.AnalysisReport{AuthUsers: 10}
		stats := &MigrationStats{
			Users:   10,
			Skipped: 2,
			Errors:  []string{"error1"},
		}
		summary := BuildValidationSummary(report, stats)
		testutil.Equal(t, 2, len(summary.Warnings))
		testutil.Contains(t, summary.Warnings[0], "2 items skipped")
		testutil.Contains(t, summary.Warnings[1], "1 errors occurred")
	})

	t.Run("adds mismatch warning", func(t *testing.T) {
		t.Parallel()
		report := &migrate.AnalysisReport{AuthUsers: 10}
		stats := &MigrationStats{Users: 8}
		summary := BuildValidationSummary(report, stats)

		testutil.Equal(t, 1, len(summary.Warnings))
		testutil.Contains(t, summary.Warnings[0], "Auth users count mismatch")
		testutil.Contains(t, summary.Warnings[0], "source=10")
		testutil.Contains(t, summary.Warnings[0], "target=8")
	})

	t.Run("mismatch counts", func(t *testing.T) {
		t.Parallel()
		report := &migrate.AnalysisReport{
			Tables:    3,
			AuthUsers: 10,
		}
		stats := &MigrationStats{
			Tables: 2,
			Users:  10,
		}
		summary := BuildValidationSummary(report, stats)
		// Tables row should show mismatch.
		testutil.Equal(t, "Tables", summary.Rows[0].Label)
		testutil.Equal(t, 3, summary.Rows[0].SourceCount)
		testutil.Equal(t, 2, summary.Rows[0].TargetCount)
	})
}

func TestQuoteLiteral(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "users", "'users'"},
		{"with single quote", "it's", "'it''s'"},
		{"empty", "", "''"},
		{"multiple quotes", "a'b'c", "'a''b''c'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := quoteLiteral(tt.input)
			testutil.Equal(t, tt.want, got)
		})
	}
}
