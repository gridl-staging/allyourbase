package appwritemigrate

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// mapAppwriteType
// ---------------------------------------------------------------------------

func TestMapAppwriteType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"string", "text"},
		{"email", "text"},
		{"enum", "text"},
		{"url", "text"},
		{"integer", "bigint"},
		{"float", "double precision"},
		{"boolean", "boolean"},
		{"datetime", "timestamptz"},
		{"relationship", "text"},
		{"ip", "inet"},
		// Unknown types fall back to text.
		{"blob", "text"},
		{"", "text"},
		{"UNKNOWN_TYPE", "text"},
		// Case insensitive.
		{"STRING", "text"},
		{"INTEGER", "bigint"},
		{"FLOAT", "double precision"},
		{"BOOLEAN", "boolean"},
		{"DateTime", "timestamptz"},
		{"IP", "inet"},
	}

	for _, tc := range tests {
		got := mapAppwriteType(tc.input)
		if got != tc.want {
			t.Errorf("mapAppwriteType(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// sqlLiteral
// ---------------------------------------------------------------------------

func TestSQLLiteral(t *testing.T) {
	t.Parallel()

	t.Run("nil returns NULL", func(t *testing.T) {
		t.Parallel()
		if got := sqlLiteral(nil); got != "NULL" {
			t.Fatalf("sqlLiteral(nil) = %q, want NULL", got)
		}
	})

	t.Run("true returns true", func(t *testing.T) {
		t.Parallel()
		if got := sqlLiteral(true); got != "true" {
			t.Fatalf("sqlLiteral(true) = %q, want true", got)
		}
	})

	t.Run("false returns false", func(t *testing.T) {
		t.Parallel()
		if got := sqlLiteral(false); got != "false" {
			t.Fatalf("sqlLiteral(false) = %q, want false", got)
		}
	})

	t.Run("int", func(t *testing.T) {
		t.Parallel()
		if got := sqlLiteral(42); got != "42" {
			t.Fatalf("sqlLiteral(42) = %q, want 42", got)
		}
	})

	t.Run("int negative", func(t *testing.T) {
		t.Parallel()
		if got := sqlLiteral(-7); got != "-7" {
			t.Fatalf("sqlLiteral(-7) = %q, want -7", got)
		}
	})

	t.Run("int64", func(t *testing.T) {
		t.Parallel()
		if got := sqlLiteral(int64(1234567890123)); got != "1234567890123" {
			t.Fatalf("sqlLiteral(int64) = %q, want 1234567890123", got)
		}
	})

	t.Run("float64", func(t *testing.T) {
		t.Parallel()
		if got := sqlLiteral(float64(3.14)); got != "3.14" {
			t.Fatalf("sqlLiteral(3.14) = %q, want 3.14", got)
		}
	})

	t.Run("float64 whole number", func(t *testing.T) {
		t.Parallel()
		if got := sqlLiteral(float64(100)); got != "100" {
			t.Fatalf("sqlLiteral(100.0) = %q, want 100", got)
		}
	})

	t.Run("json.Number integer", func(t *testing.T) {
		t.Parallel()
		if got := sqlLiteral(json.Number("99")); got != "99" {
			t.Fatalf("sqlLiteral(json.Number(99)) = %q, want 99", got)
		}
	})

	t.Run("json.Number float", func(t *testing.T) {
		t.Parallel()
		if got := sqlLiteral(json.Number("1.5")); got != "1.5" {
			t.Fatalf("sqlLiteral(json.Number(1.5)) = %q, want 1.5", got)
		}
	})

	t.Run("plain string", func(t *testing.T) {
		t.Parallel()
		if got := sqlLiteral("hello"); got != "'hello'" {
			t.Fatalf("sqlLiteral(hello) = %q, want 'hello'", got)
		}
	})

	t.Run("string with single quote escaped", func(t *testing.T) {
		t.Parallel()
		// Single quotes must be doubled: O'Brien → 'O''Brien'
		if got := sqlLiteral("O'Brien"); got != "'O''Brien'" {
			t.Fatalf("sqlLiteral(O'Brien) = %q, want 'O''Brien'", got)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()
		if got := sqlLiteral(""); got != "''" {
			t.Fatalf("sqlLiteral('') = %q, want ''", got)
		}
	})

	t.Run("complex type marshaled as JSON string", func(t *testing.T) {
		t.Parallel()
		// A slice is not a known type, so it gets JSON-marshaled then quoted.
		got := sqlLiteral([]string{"a", "b"})
		if !strings.HasPrefix(got, "'") || !strings.HasSuffix(got, "'") {
			t.Fatalf("sqlLiteral(slice) = %q, expected single-quoted JSON", got)
		}
		// The JSON representation of ["a","b"] should appear inside the quotes.
		if !strings.Contains(got, `["a","b"]`) {
			t.Fatalf("sqlLiteral(slice) = %q, expected JSON array inside", got)
		}
	})

	t.Run("map marshaled as JSON string", func(t *testing.T) {
		t.Parallel()
		got := sqlLiteral(map[string]int{"x": 1})
		if !strings.HasPrefix(got, "'") || !strings.HasSuffix(got, "'") {
			t.Fatalf("sqlLiteral(map) = %q, expected single-quoted JSON", got)
		}
	})
}

// ---------------------------------------------------------------------------
// containsRole
// ---------------------------------------------------------------------------

func TestContainsRole(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()
		if !containsRole([]string{"user:*", "admin"}, "user:*") {
			t.Fatal("expected true, got false")
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		if containsRole([]string{"admin", "owner"}, "user:*") {
			t.Fatal("expected false, got true")
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()
		if containsRole([]string{}, "user:*") {
			t.Fatal("expected false for empty slice, got true")
		}
	})

	t.Run("nil slice", func(t *testing.T) {
		t.Parallel()
		if containsRole(nil, "user:*") {
			t.Fatal("expected false for nil slice, got true")
		}
	})

	t.Run("exact match only — no substring", func(t *testing.T) {
		t.Parallel()
		// "user:admin" should not match "user:*"
		if containsRole([]string{"user:admin"}, "user:*") {
			t.Fatal("expected false for non-matching role, got true")
		}
	})
}

// ---------------------------------------------------------------------------
// buildInsertSQL
// ---------------------------------------------------------------------------

func TestBuildInsertSQL(t *testing.T) {
	t.Parallel()

	coll := appwriteCollection{
		Name: "items",
		Attributes: []appwriteAttribute{
			{Key: "name", Type: "string"},
			{Key: "count", Type: "integer"},
		},
	}

	t.Run("$id mapped to id column", func(t *testing.T) {
		t.Parallel()
		doc := map[string]any{"$id": "abc123", "name": "widget", "count": 5}
		sql, ok := buildInsertSQL(coll, doc)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if !strings.Contains(sql, `"id"`) {
			t.Errorf("expected \"id\" column, got: %s", sql)
		}
		if !strings.Contains(sql, "'abc123'") {
			t.Errorf("expected 'abc123' value, got: %s", sql)
		}
	})

	t.Run("unknown columns skipped", func(t *testing.T) {
		t.Parallel()
		doc := map[string]any{"name": "widget", "$unknown_field": "x", "not_in_schema": 99}
		sql, ok := buildInsertSQL(coll, doc)
		if !ok {
			t.Fatal("expected ok=true")
		}
		// $unknown_field is not $id and not in schema — should be absent.
		if strings.Contains(sql, "unknown_field") {
			t.Errorf("unexpected unknown_field in SQL: %s", sql)
		}
		if strings.Contains(sql, "not_in_schema") {
			t.Errorf("unexpected not_in_schema in SQL: %s", sql)
		}
	})

	t.Run("empty doc returns false", func(t *testing.T) {
		t.Parallel()
		_, ok := buildInsertSQL(coll, map[string]any{})
		if ok {
			t.Fatal("expected ok=false for empty doc")
		}
	})

	t.Run("doc with only unknown fields returns false", func(t *testing.T) {
		t.Parallel()
		// "$createdAt" is a built-in Appwrite meta-field not in the schema.
		doc := map[string]any{"$createdAt": "2024-01-01", "$updatedAt": "2024-01-02"}
		_, ok := buildInsertSQL(coll, doc)
		if ok {
			t.Fatal("expected ok=false when no valid columns remain")
		}
	})

	t.Run("valid insert structure", func(t *testing.T) {
		t.Parallel()
		doc := map[string]any{"$id": "id1", "name": "foo", "count": 3}
		sql, ok := buildInsertSQL(coll, doc)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if !strings.HasPrefix(sql, `INSERT INTO "public"."items"`) {
			t.Errorf("unexpected table reference: %s", sql)
		}
		if !strings.Contains(sql, "VALUES") {
			t.Errorf("expected VALUES clause: %s", sql)
		}
	})
}

// ---------------------------------------------------------------------------
// buildCreateTableSQL
// ---------------------------------------------------------------------------

func TestBuildCreateTableSQL(t *testing.T) {
	t.Parallel()

	t.Run("empty attrs gets id primary key only", func(t *testing.T) {
		t.Parallel()
		coll := appwriteCollection{Name: "things"}
		sql := buildCreateTableSQL(coll)
		if !strings.Contains(sql, `"id" text PRIMARY KEY`) {
			t.Errorf("expected id PK column, got: %s", sql)
		}
		// Should be a well-formed CREATE TABLE.
		if !strings.HasPrefix(sql, `CREATE TABLE "public"."things"`) {
			t.Errorf("unexpected table name, got: %s", sql)
		}
	})

	t.Run("attrs are sorted alphabetically", func(t *testing.T) {
		t.Parallel()
		coll := appwriteCollection{
			Name: "sorted",
			Attributes: []appwriteAttribute{
				{Key: "zebra", Type: "string"},
				{Key: "apple", Type: "integer"},
				{Key: "mango", Type: "boolean"},
			},
		}
		sql := buildCreateTableSQL(coll)
		// "apple" should appear before "mango" and "mango" before "zebra".
		appleIdx := strings.Index(sql, `"apple"`)
		mangoIdx := strings.Index(sql, `"mango"`)
		zebraIdx := strings.Index(sql, `"zebra"`)
		if appleIdx < 0 || mangoIdx < 0 || zebraIdx < 0 {
			t.Fatalf("missing expected columns in: %s", sql)
		}
		if !(appleIdx < mangoIdx && mangoIdx < zebraIdx) {
			t.Errorf("columns not sorted: apple=%d mango=%d zebra=%d in: %s", appleIdx, mangoIdx, zebraIdx, sql)
		}
	})

	t.Run("required attr gets NOT NULL", func(t *testing.T) {
		t.Parallel()
		coll := appwriteCollection{
			Name: "nn_test",
			Attributes: []appwriteAttribute{
				{Key: "title", Type: "string", Required: true},
				{Key: "body", Type: "string", Required: false},
			},
		}
		sql := buildCreateTableSQL(coll)
		if !strings.Contains(sql, `"title" text NOT NULL`) {
			t.Errorf("expected NOT NULL for required attr, got: %s", sql)
		}
		// Optional attr must NOT have NOT NULL.
		if strings.Contains(sql, `"body" text NOT NULL`) {
			t.Errorf("unexpected NOT NULL for optional attr, got: %s", sql)
		}
	})

	t.Run("email type gets CHECK constraint", func(t *testing.T) {
		t.Parallel()
		coll := appwriteCollection{
			Name: "users",
			Attributes: []appwriteAttribute{
				{Key: "email_addr", Type: "email"},
			},
		}
		sql := buildCreateTableSQL(coll)
		if !strings.Contains(sql, "CHECK") {
			t.Errorf("expected CHECK constraint for email type, got: %s", sql)
		}
		// The check should reference the column.
		if !strings.Contains(sql, `"email_addr"`) {
			t.Errorf("expected email_addr in CHECK, got: %s", sql)
		}
	})

	t.Run("enum type with elements gets CHECK IN constraint", func(t *testing.T) {
		t.Parallel()
		coll := appwriteCollection{
			Name: "statuses",
			Attributes: []appwriteAttribute{
				{Key: "status", Type: "enum", Elements: []string{"open", "closed", "pending"}},
			},
		}
		sql := buildCreateTableSQL(coll)
		if !strings.Contains(sql, "CHECK") {
			t.Errorf("expected CHECK constraint for enum type, got: %s", sql)
		}
		if !strings.Contains(sql, "IN") {
			t.Errorf("expected IN clause for enum constraint, got: %s", sql)
		}
		for _, val := range []string{"'open'", "'closed'", "'pending'"} {
			if !strings.Contains(sql, val) {
				t.Errorf("expected enum value %s in SQL: %s", val, sql)
			}
		}
	})

	t.Run("enum type without elements gets no CHECK", func(t *testing.T) {
		t.Parallel()
		coll := appwriteCollection{
			Name: "bare_enum",
			Attributes: []appwriteAttribute{
				{Key: "kind", Type: "enum", Elements: nil},
			},
		}
		sql := buildCreateTableSQL(coll)
		// No elements means no CHECK IN constraint (email check is absent too).
		if strings.Contains(sql, "IN (") {
			t.Errorf("unexpected IN clause for enum without elements: %s", sql)
		}
	})

	t.Run("explicit id attr becomes primary key", func(t *testing.T) {
		t.Parallel()
		coll := appwriteCollection{
			Name: "explicit_id",
			Attributes: []appwriteAttribute{
				{Key: "id", Type: "string"},
				{Key: "label", Type: "string"},
			},
		}
		sql := buildCreateTableSQL(coll)
		// Should have exactly one PRIMARY KEY reference on the id column.
		if strings.Count(sql, "PRIMARY KEY") != 1 {
			t.Errorf("expected exactly one PRIMARY KEY, got: %s", sql)
		}
		if !strings.Contains(sql, `"id" text PRIMARY KEY`) {
			t.Errorf("expected explicit id as PK, got: %s", sql)
		}
	})
}
