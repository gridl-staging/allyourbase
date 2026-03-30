package nhostmigrate

import "testing"

func TestSplitSQLStatements(t *testing.T) {
	t.Parallel()

	t.Run("simple statements", func(t *testing.T) {
		got := splitSQLStatements("SELECT 1; SELECT 2;")
		assertStrings(t, got, []string{"SELECT 1", "SELECT 2"})
	})

	t.Run("trailing statement without semicolon", func(t *testing.T) {
		got := splitSQLStatements("SELECT 1; SELECT 2")
		assertStrings(t, got, []string{"SELECT 1", "SELECT 2"})
	})

	t.Run("empty input", func(t *testing.T) {
		got := splitSQLStatements("")
		if len(got) != 0 {
			t.Fatalf("expected empty, got %d statements", len(got))
		}
	})

	t.Run("whitespace-only input", func(t *testing.T) {
		got := splitSQLStatements("   \n\t  ")
		if len(got) != 0 {
			t.Fatalf("expected empty, got %d statements", len(got))
		}
	})

	t.Run("semicolons inside single-quoted strings ignored", func(t *testing.T) {
		got := splitSQLStatements("INSERT INTO t VALUES ('a;b'); SELECT 1")
		assertStrings(t, got, []string{"INSERT INTO t VALUES ('a;b')", "SELECT 1"})
	})

	t.Run("semicolons inside double-quoted identifiers ignored", func(t *testing.T) {
		got := splitSQLStatements(`SELECT "col;name" FROM t; SELECT 1`)
		assertStrings(t, got, []string{`SELECT "col;name" FROM t`, "SELECT 1"})
	})

	t.Run("dollar-quoted string preserves internal semicolons", func(t *testing.T) {
		sql := `CREATE FUNCTION f() RETURNS void AS $$ BEGIN RETURN; END; $$ LANGUAGE plpgsql; SELECT 1`
		got := splitSQLStatements(sql)
		if len(got) != 2 {
			t.Fatalf("expected 2 statements, got %d: %v", len(got), got)
		}
		// The first statement should contain the dollar-quoted body with semicolons.
		if got[0] != "CREATE FUNCTION f() RETURNS void AS $$ BEGIN RETURN; END; $$ LANGUAGE plpgsql" {
			t.Fatalf("unexpected first statement: %s", got[0])
		}
	})

	t.Run("named dollar-quoted tag", func(t *testing.T) {
		sql := `CREATE FUNCTION f() AS $body$ SELECT 1; $body$; SELECT 2`
		got := splitSQLStatements(sql)
		assertStrings(t, got, []string{
			"CREATE FUNCTION f() AS $body$ SELECT 1; $body$",
			"SELECT 2",
		})
	})
}

func TestParseDollarTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		wantTag string
		wantOK  bool
	}{
		{"$$rest", "$$", true},
		{"$body$rest", "$body$", true},
		{"$fn_v1$rest", "$fn_v1$", true},
		{"$_$rest", "$_$", true},
		{"$", "", false},          // lone dollar, no closing
		{"$-bad$rest", "", false}, // hyphen not allowed in tag
		{"$123$rest", "$123$", true},
		{"not a dollar", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		tag, ok := parseDollarTag(tc.input)
		if ok != tc.wantOK {
			t.Errorf("parseDollarTag(%q): ok = %v, want %v", tc.input, ok, tc.wantOK)
		}
		if tc.wantOK && tag != tc.wantTag {
			t.Errorf("parseDollarTag(%q) = %q, want %q", tc.input, tag, tc.wantTag)
		}
	}
}

func TestClassifySQLStatement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		stmt string
		want string
	}{
		{"CREATE TABLE users (id int)", "create_table"},
		{"create table IF NOT EXISTS t ()", "create_table"},
		{"CREATE VIEW v AS SELECT 1", "create_view"},
		{"CREATE MATERIALIZED VIEW v AS SELECT 1", "create_view"},
		{"CREATE INDEX idx ON t (col)", "create_index"},
		{"CREATE UNIQUE INDEX idx ON t (col)", "create_index"},
		{"INSERT INTO t VALUES (1)", "insert"},
		{`ALTER TABLE t ADD CONSTRAINT fk FOREIGN KEY (a) REFERENCES other(b)`, "foreign_key"},
		{"SELECT 1", ""},
		{"DROP TABLE t", ""},
		{"", ""},
	}
	for _, tc := range tests {
		got := classifySQLStatement(tc.stmt)
		if got != tc.want {
			t.Errorf("classifySQLStatement(%q) = %q, want %q", tc.stmt, got, tc.want)
		}
	}
}

func TestShouldSkipStatement(t *testing.T) {
	t.Parallel()

	// Statements referencing system schemas or hdb_ tables should be skipped.
	skippable := []string{
		`CREATE TABLE information_schema.foo (id int)`,
		`INSERT INTO pg_catalog.pg_class VALUES (1)`,
		`CREATE TABLE hdb_catalog.some_table (id int)`,
		`CREATE TABLE public.hdb_action_log (id int)`, // hdb_ prefix
		`CREATE INDEX idx ON hdb_catalog.hdb_table (name)`,
	}
	for _, stmt := range skippable {
		if !shouldSkipStatement(stmt) {
			t.Errorf("shouldSkipStatement(%q) = false, want true", stmt)
		}
	}

	// Regular user statements should not be skipped.
	keepable := []string{
		`CREATE TABLE public.users (id int)`,
		`INSERT INTO posts VALUES (1)`,
		`ALTER TABLE orders ADD CONSTRAINT fk FOREIGN KEY (user_id) REFERENCES users(id)`,
		`SELECT 1`, // unrecognized — not skipped
	}
	for _, stmt := range keepable {
		if shouldSkipStatement(stmt) {
			t.Errorf("shouldSkipStatement(%q) = true, want false", stmt)
		}
	}
}

func TestCountInsertRows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		stmt string
		want int
	}{
		{"single row", "INSERT INTO t VALUES (1, 'a')", 1},
		{"two rows", "INSERT INTO t VALUES (1, 'a'), (2, 'b')", 2},
		{"three rows", "INSERT INTO t VALUES (1), (2), (3)", 3},
		{"no VALUES clause", "INSERT INTO t SELECT * FROM s", 1},
		{"nested parens in value", "INSERT INTO t VALUES (1, (SELECT 1)), (2, (SELECT 2))", 2},
		{"single quotes with parens inside", "INSERT INTO t VALUES ('(a)'), ('(b)')", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := countInsertRows(tc.stmt)
			if got != tc.want {
				t.Fatalf("countInsertRows = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestSplitQualifiedTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw        string
		wantSchema string
		wantTable  string
	}{
		{"public.users", "public", "users"},
		{`"public"."users"`, "public", "users"},
		{"users", "public", "users"},
		{`"users"`, "public", "users"},
		{"custom.widgets", "custom", "widgets"},
		// Trailing parentheses/semicolons stripped.
		{"public.users(id)", "public", "users"},
		{"public.users;", "public", "users"},
		// Whitespace trimmed.
		{"  public.users  ", "public", "users"},
	}
	for _, tc := range tests {
		schema, table := splitQualifiedTable(tc.raw)
		if schema != tc.wantSchema || table != tc.wantTable {
			t.Errorf("splitQualifiedTable(%q) = (%q, %q), want (%q, %q)",
				tc.raw, schema, table, tc.wantSchema, tc.wantTable)
		}
	}
}

func TestShouldSkipQualifiedTable(t *testing.T) {
	t.Parallel()

	// System schemas and hdb_ prefix tables should be skipped.
	skip := [][2]string{
		{"information_schema", "columns"},
		{"pg_catalog", "pg_class"},
		{"hdb_catalog", "hdb_table"},
		{"public", "hdb_action_log"}, // hdb_ prefix in public
	}
	for _, pair := range skip {
		if !shouldSkipQualifiedTable(pair[0], pair[1]) {
			t.Errorf("shouldSkipQualifiedTable(%q, %q) = false, want true", pair[0], pair[1])
		}
	}

	// User schemas/tables should not be skipped.
	keep := [][2]string{
		{"public", "users"},
		{"custom", "orders"},
		{"public", "hd_table"}, // "hd_" is not "hdb_"
	}
	for _, pair := range keep {
		if shouldSkipQualifiedTable(pair[0], pair[1]) {
			t.Errorf("shouldSkipQualifiedTable(%q, %q) = true, want false", pair[0], pair[1])
		}
	}
}

func TestDefaultSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"", "public"},
		{"  ", "public"},
		{"\t", "public"},
		{"custom", "custom"},
		{"public", "public"},
	}
	for _, tc := range tests {
		got := defaultSchema(tc.input)
		if got != tc.want {
			t.Errorf("defaultSchema(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestQualifiedTableKey(t *testing.T) {
	t.Parallel()

	if got := qualifiedTableKey("Public", "Users"); got != "public.users" {
		t.Errorf("qualifiedTableKey = %q, want %q", got, "public.users")
	}
}

func TestForeignKeySourceKey(t *testing.T) {
	t.Parallel()

	// With explicit schema.
	if got := foreignKeySourceKey("custom", "orders", "user_id"); got != "custom.orders.user_id" {
		t.Errorf("foreignKeySourceKey = %q, want %q", got, "custom.orders.user_id")
	}
	// Empty schema defaults to "public".
	if got := foreignKeySourceKey("", "orders", "user_id"); got != "public.orders.user_id" {
		t.Errorf("foreignKeySourceKey = %q, want %q", got, "public.orders.user_id")
	}
}

func TestExtractFirstParenValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		s      string
		anchor string
		want   string
	}{
		{"basic", "FOREIGN KEY (user_id) REFERENCES users(id)", "FOREIGN KEY", "user_id"},
		{"no anchor", "users(id)", "", "id"},
		{"double-quoted stripped", `FOREIGN KEY ("user_id")`, "FOREIGN KEY", "user_id"},
		{"anchor not found", "some text", "MISSING", ""},
		{"no parentheses", "FOREIGN KEY no parens", "FOREIGN KEY", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractFirstParenValue(tc.s, tc.anchor)
			if got != tc.want {
				t.Fatalf("extractFirstParenValue(%q, %q) = %q, want %q", tc.s, tc.anchor, got, tc.want)
			}
		})
	}
}

func TestExtractAfterKeyword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		s       string
		keyword string
		want    string
	}{
		{"REFERENCES public.users(id)", "REFERENCES", "public.users(id)"},
		{"REFERENCES users;", "REFERENCES", "users"},
		{"no match here", "REFERENCES", ""},
		// Case insensitive.
		{"references users(id)", "REFERENCES", "users(id)"},
	}
	for _, tc := range tests {
		got := extractAfterKeyword(tc.s, tc.keyword)
		if got != tc.want {
			t.Errorf("extractAfterKeyword(%q, %q) = %q, want %q", tc.s, tc.keyword, got, tc.want)
		}
	}
}

func TestParseForeignKeyStatement(t *testing.T) {
	t.Parallel()

	t.Run("standard ALTER TABLE FK", func(t *testing.T) {
		stmt := `ALTER TABLE public.orders ADD CONSTRAINT fk_user FOREIGN KEY (user_id) REFERENCES public.users(id)`
		fk, ok := parseForeignKeyStatement(stmt)
		if !ok {
			t.Fatal("expected successful parse")
		}
		if fk.FromSchema != "public" || fk.FromTable != "orders" || fk.FromColumn != "user_id" {
			t.Errorf("from = %s.%s.%s, want public.orders.user_id", fk.FromSchema, fk.FromTable, fk.FromColumn)
		}
		if fk.ToSchema != "public" || fk.ToTable != "users" || fk.ToColumn != "id" {
			t.Errorf("to = %s.%s.%s, want public.users.id", fk.ToSchema, fk.ToTable, fk.ToColumn)
		}
	})

	t.Run("unqualified table defaults to public schema", func(t *testing.T) {
		stmt := `ALTER TABLE orders ADD CONSTRAINT fk_user FOREIGN KEY (user_id) REFERENCES users(id)`
		fk, ok := parseForeignKeyStatement(stmt)
		if !ok {
			t.Fatal("expected successful parse")
		}
		if fk.FromSchema != "public" {
			t.Errorf("FromSchema = %q, want public", fk.FromSchema)
		}
		if fk.ToSchema != "public" {
			t.Errorf("ToSchema = %q, want public", fk.ToSchema)
		}
	})

	t.Run("not a foreign key statement", func(t *testing.T) {
		_, ok := parseForeignKeyStatement("SELECT 1")
		if ok {
			t.Fatal("expected parse failure for non-FK statement")
		}
	})

	t.Run("missing REFERENCES", func(t *testing.T) {
		stmt := `ALTER TABLE orders ADD CONSTRAINT fk_user FOREIGN KEY (user_id)`
		_, ok := parseForeignKeyStatement(stmt)
		if ok {
			t.Fatal("expected parse failure for missing REFERENCES")
		}
	})
}

// assertStrings is a test helper that compares two string slices.
func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d\n  got:  %v\n  want: %v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
