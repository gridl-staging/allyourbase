package directusmigrate

import (
	"strings"
	"testing"
)

func TestShouldSkipCollection(t *testing.T) {
	t.Parallel()

	// directus_ prefix (and case variations) must be skipped.
	skip := []string{
		"directus_users",
		"directus_files",
		"DIRECTUS_SETTINGS",
		"Directus_Activity",
		"directus_",
	}
	for _, c := range skip {
		if !shouldSkipCollection(c) {
			t.Errorf("shouldSkipCollection(%q) = false, want true", c)
		}
	}

	// Normal collections must be kept.
	keep := []string{
		"users",
		"orders",
		"products",
		"direct_messages", // "direct_" is not "directus_"
		"",                // empty string has no directus_ prefix
	}
	for _, c := range keep {
		if shouldSkipCollection(c) {
			t.Errorf("shouldSkipCollection(%q) = true, want false", c)
		}
	}
}

func TestRelatedPrimaryKey(t *testing.T) {
	t.Parallel()

	t.Run("finds PK field", func(t *testing.T) {
		fields := []directusField{
			{Field: "name", Schema: directusFieldSchema{IsPrimaryKey: false}},
			{Field: "id", Schema: directusFieldSchema{IsPrimaryKey: true}},
		}
		got := relatedPrimaryKey(fields)
		if got != "id" {
			t.Fatalf("relatedPrimaryKey = %q, want %q", got, "id")
		}
	})

	t.Run("returns empty when no PK", func(t *testing.T) {
		fields := []directusField{
			{Field: "name", Schema: directusFieldSchema{IsPrimaryKey: false}},
			{Field: "email", Schema: directusFieldSchema{IsPrimaryKey: false}},
		}
		got := relatedPrimaryKey(fields)
		if got != "" {
			t.Fatalf("relatedPrimaryKey = %q, want empty", got)
		}
	})

	t.Run("returns empty for nil/empty slice", func(t *testing.T) {
		if got := relatedPrimaryKey(nil); got != "" {
			t.Fatalf("relatedPrimaryKey(nil) = %q, want empty", got)
		}
		if got := relatedPrimaryKey([]directusField{}); got != "" {
			t.Fatalf("relatedPrimaryKey([]) = %q, want empty", got)
		}
	})

	t.Run("skips PK field with blank Field name", func(t *testing.T) {
		// A field marked as primary key but with a blank Field name should be ignored.
		fields := []directusField{
			{Field: "  ", Schema: directusFieldSchema{IsPrimaryKey: true}},
			{Field: "id", Schema: directusFieldSchema{IsPrimaryKey: true}},
		}
		got := relatedPrimaryKey(fields)
		if got != "id" {
			t.Fatalf("relatedPrimaryKey = %q, want %q", got, "id")
		}
	})
}

func TestMapDirectusType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		// string group
		{"string", "text"},
		{"csv", "text"},
		{"hash", "text"},
		// integer
		{"integer", "bigint"},
		// float
		{"float", "double precision"},
		// boolean
		{"boolean", "boolean"},
		// datetime
		{"datetime", "timestamptz"},
		// json
		{"json", "jsonb"},
		// uuid
		{"uuid", "uuid"},
		// geometry
		{"geometry", "geometry"},
		// unknown falls back to text
		{"unknown_type", "text"},
		{"", "text"},
		// case insensitive
		{"STRING", "text"},
		{"INTEGER", "bigint"},
		{"BOOLEAN", "boolean"},
	}
	for _, tc := range tests {
		got := mapDirectusType(tc.input)
		if got != tc.want {
			t.Errorf("mapDirectusType(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestMapPermissionAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"read", "SELECT"},
		{"create", "INSERT"},
		{"update", "UPDATE"},
		{"delete", "DELETE"},
		// case insensitive
		{"READ", "SELECT"},
		{"CREATE", "INSERT"},
		{"UPDATE", "UPDATE"},
		{"DELETE", "DELETE"},
		// unknown returns empty
		{"share", ""},
		{"", ""},
		{"unknown", ""},
	}
	for _, tc := range tests {
		got := mapPermissionAction(tc.input)
		if got != tc.want {
			t.Errorf("mapPermissionAction(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestBuildCreateTableSQL(t *testing.T) {
	t.Parallel()

	t.Run("empty fields gets id bigserial PK", func(t *testing.T) {
		t.Parallel()
		got := buildCreateTableSQL("items", nil)
		// Must contain the table name and the fallback id bigserial PRIMARY KEY column.
		if !strings.Contains(got, `"items"`) {
			t.Errorf("expected table name in SQL: %s", got)
		}
		if !strings.Contains(got, "bigserial PRIMARY KEY") {
			t.Errorf("expected bigserial PRIMARY KEY for empty fields: %s", got)
		}
		if !strings.Contains(got, `"id"`) {
			t.Errorf("expected id column for empty fields: %s", got)
		}
	})

	t.Run("empty field slice also gets fallback", func(t *testing.T) {
		t.Parallel()
		got := buildCreateTableSQL("orders", []directusField{})
		if !strings.Contains(got, "bigserial PRIMARY KEY") {
			t.Errorf("expected bigserial PRIMARY KEY for empty field slice: %s", got)
		}
	})

	t.Run("fields are sorted alphabetically", func(t *testing.T) {
		t.Parallel()
		fields := []directusField{
			{Field: "title", Type: "string", Schema: directusFieldSchema{IsNullable: true}},
			{Field: "created_at", Type: "datetime", Schema: directusFieldSchema{IsNullable: true}},
			{Field: "id", Type: "integer", Schema: directusFieldSchema{IsPrimaryKey: true, IsNullable: false}},
		}
		got := buildCreateTableSQL("posts", fields)
		// created_at must come before id, id before title in sorted order.
		posCreated := strings.Index(got, "created_at")
		posID := strings.Index(got, `"id"`)
		posTitle := strings.Index(got, "title")
		if posCreated == -1 || posID == -1 || posTitle == -1 {
			t.Fatalf("missing column in SQL: %s", got)
		}
		if !(posCreated < posID && posID < posTitle) {
			t.Errorf("columns not in sorted order (created_at, id, title): %s", got)
		}
	})

	t.Run("NOT NULL applied when IsNullable false", func(t *testing.T) {
		t.Parallel()
		fields := []directusField{
			{Field: "name", Type: "string", Schema: directusFieldSchema{IsNullable: false}},
		}
		got := buildCreateTableSQL("people", fields)
		if !strings.Contains(got, "NOT NULL") {
			t.Errorf("expected NOT NULL for non-nullable field: %s", got)
		}
	})

	t.Run("no NOT NULL when IsNullable true", func(t *testing.T) {
		t.Parallel()
		fields := []directusField{
			{Field: "bio", Type: "string", Schema: directusFieldSchema{IsNullable: true}},
		}
		got := buildCreateTableSQL("authors", fields)
		if strings.Contains(got, "NOT NULL") {
			t.Errorf("unexpected NOT NULL for nullable field: %s", got)
		}
	})

	t.Run("PRIMARY KEY applied for primary key field", func(t *testing.T) {
		t.Parallel()
		fields := []directusField{
			{Field: "id", Type: "integer", Schema: directusFieldSchema{IsPrimaryKey: true, IsNullable: false}},
			{Field: "slug", Type: "string", Schema: directusFieldSchema{IsNullable: true}},
		}
		got := buildCreateTableSQL("articles", fields)
		if !strings.Contains(got, "PRIMARY KEY") {
			t.Errorf("expected PRIMARY KEY constraint: %s", got)
		}
	})

	t.Run("table name is schema-qualified", func(t *testing.T) {
		t.Parallel()
		got := buildCreateTableSQL("widgets", nil)
		// QuoteQualifiedTable("public", "widgets") should produce public."widgets".
		if !strings.Contains(got, "public") {
			t.Errorf("expected public schema qualification: %s", got)
		}
	})
}
