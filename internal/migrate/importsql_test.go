package migrate

import (
	"strings"
	"testing"
)

func TestSanitizeIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"CamelCase", "camelcase"},
		{"with spaces", "with_spaces"},
		{"with-hyphens", "with_hyphens"},
		{"with.dots", "with_dots"},
		{"with@special#chars!", "with_special_chars_"},
		{"123startsWithDigit", "123startswithdigit"},
		{"_underscore", "_underscore"},
		{"", "id"},
		{"!!!", "___"},
		{"MiXeD_CaSe_123", "mixed_case_123"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SanitizeIdentifier(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildAllowPolicySQL(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		table  string
		role   string
		action string
		want   string // substring checks
	}{
		{
			name:   "SELECT policy",
			schema: "public",
			table:  "posts",
			role:   "authenticated",
			action: "SELECT",
			want:   "FOR SELECT TO",
		},
		{
			name:   "INSERT policy uses WITH CHECK",
			schema: "public",
			table:  "posts",
			role:   "authenticated",
			action: "INSERT",
			want:   "WITH CHECK (true)",
		},
		{
			name:   "UPDATE policy uses USING",
			schema: "public",
			table:  "posts",
			role:   "authenticated",
			action: "UPDATE",
			want:   "FOR UPDATE TO",
		},
		{
			name:   "DELETE policy uses USING",
			schema: "public",
			table:  "posts",
			role:   "authenticated",
			action: "DELETE",
			want:   "FOR DELETE TO",
		},
		{
			name:   "empty schema defaults to public",
			schema: "",
			table:  "users",
			role:   "anon",
			action: "SELECT",
			want:   `"public"."users"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildAllowPolicySQL(tt.schema, tt.table, tt.role, tt.action)
			if !strings.Contains(got, "CREATE POLICY") {
				t.Errorf("expected CREATE POLICY prefix, got %q", got)
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("expected %q to contain %q", got, tt.want)
			}
		})
	}
}

func TestBuildAllowPolicySQLPolicyName(t *testing.T) {
	// Verify the policy name is sanitized from table+role+action.
	got := BuildAllowPolicySQL("public", "My Table", "auth-user", "SELECT")
	// "My Table" + "auth-user" + "SELECT" → sanitized to lowercase with underscores.
	if !strings.Contains(got, "my_table_auth_user_select") {
		t.Errorf("expected sanitized policy name, got %q", got)
	}
}

func TestSQLString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"it's", "'it''s'"},
		{"", "''"},
		{"O'Brien's", "'O''Brien''s'"},
		{"no special", "'no special'"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SQLString(tt.input)
			if got != tt.want {
				t.Errorf("SQLString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestQuoteQualifiedTableDefaultsToPublic(t *testing.T) {
	got := QuoteQualifiedTable("", "users")
	if !strings.Contains(got, "public") {
		t.Errorf("expected public schema default, got %q", got)
	}
	if !strings.Contains(got, "users") {
		t.Errorf("expected table name users, got %q", got)
	}
}

func TestQuoteQualifiedTableExplicitSchema(t *testing.T) {
	got := QuoteQualifiedTable("myschema", "mytable")
	if !strings.Contains(got, "myschema") {
		t.Errorf("expected schema myschema, got %q", got)
	}
	if !strings.Contains(got, "mytable") {
		t.Errorf("expected table mytable, got %q", got)
	}
}
