package pbmigrate

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestFieldTypeToPgType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		field    PBField
		expected string
	}{
		{
			name:     "text field",
			field:    PBField{Type: "text"},
			expected: "TEXT",
		},
		{
			name:     "email field",
			field:    PBField{Type: "email"},
			expected: "TEXT",
		},
		{
			name:     "url field",
			field:    PBField{Type: "url"},
			expected: "TEXT",
		},
		{
			name:     "editor field",
			field:    PBField{Type: "editor"},
			expected: "TEXT",
		},
		{
			name:     "number field",
			field:    PBField{Type: "number"},
			expected: "DOUBLE PRECISION",
		},
		{
			name:     "bool field",
			field:    PBField{Type: "bool"},
			expected: "BOOLEAN",
		},
		{
			name:     "date field",
			field:    PBField{Type: "date"},
			expected: "TIMESTAMP WITH TIME ZONE",
		},
		{
			name:     "json field",
			field:    PBField{Type: "json"},
			expected: "JSONB",
		},
		{
			name:     "single select",
			field:    PBField{Type: "select", Options: map[string]interface{}{"maxSelect": float64(1)}},
			expected: "TEXT",
		},
		{
			name:     "multiple select",
			field:    PBField{Type: "select", Options: map[string]interface{}{"maxSelect": float64(5)}},
			expected: "TEXT[]",
		},
		{
			name:     "single file",
			field:    PBField{Type: "file", Options: map[string]interface{}{"maxSelect": float64(1)}},
			expected: "TEXT",
		},
		{
			name:     "multiple files",
			field:    PBField{Type: "file", Options: map[string]interface{}{"maxSelect": float64(10)}},
			expected: "TEXT[]",
		},
		{
			name:     "single relation",
			field:    PBField{Type: "relation", Options: map[string]interface{}{"maxSelect": float64(1)}},
			expected: "TEXT",
		},
		{
			name:     "multiple relations",
			field:    PBField{Type: "relation", Options: map[string]interface{}{"maxSelect": float64(3)}},
			expected: "TEXT[]",
		},
		{
			name:     "unknown type fallback",
			field:    PBField{Type: "unknown"},
			expected: "TEXT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := FieldTypeToPgType(tt.field)
			testutil.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildCreateTableSQL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		coll     PBCollection
		contains []string
	}{
		{
			name: "simple table",
			coll: PBCollection{
				Name: "posts",
				Type: "base",
				Schema: []PBField{
					{Name: "title", Type: "text", Required: true},
					{Name: "content", Type: "editor"},
					{Name: "published", Type: "bool"},
				},
			},
			contains: []string{
				"CREATE TABLE",
				`"posts"`,
				"id TEXT PRIMARY KEY",
				"created TIMESTAMP WITH TIME ZONE",
				"updated TIMESTAMP WITH TIME ZONE",
				`"title" TEXT NOT NULL`,
				`"content" TEXT`,
				`"published" BOOLEAN`,
			},
		},
		{
			name: "table with unique constraint",
			coll: PBCollection{
				Name: "users",
				Type: "base",
				Schema: []PBField{
					{Name: "username", Type: "text", Required: true, Unique: true},
					{Name: "email", Type: "email", Required: true, Unique: true},
				},
			},
			contains: []string{
				`"username" TEXT NOT NULL UNIQUE`,
				`"email" TEXT NOT NULL UNIQUE`,
			},
		},
		{
			name: "table with array fields",
			coll: PBCollection{
				Name: "articles",
				Type: "base",
				Schema: []PBField{
					{Name: "tags", Type: "select", Options: map[string]interface{}{"maxSelect": float64(10)}},
					{Name: "attachments", Type: "file", Options: map[string]interface{}{"maxSelect": float64(5)}},
				},
			},
			contains: []string{
				`"tags" TEXT[]`,
				`"attachments" TEXT[]`,
			},
		},
		{
			name: "skip system fields",
			coll: PBCollection{
				Name: "test",
				Type: "base",
				Schema: []PBField{
					{Name: "id", Type: "text", System: true},
					{Name: "created", Type: "date", System: true},
					{Name: "name", Type: "text"},
				},
			},
			contains: []string{
				`"name" TEXT`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sql := BuildCreateTableSQL(tt.coll)

			for _, substr := range tt.contains {
				if !strings.Contains(sql, substr) {
					t.Errorf("expected SQL to contain %q, got:\n%s", substr, sql)
				}
			}
		})
	}
}

func TestBuildCreateViewSQL(t *testing.T) {
	t.Parallel()
	coll := PBCollection{
		Name:      "active_users",
		Type:      "view",
		ViewQuery: "SELECT * FROM users WHERE active = true",
	}

	sql := BuildCreateViewSQL(coll)

	testutil.Contains(t, sql, "CREATE VIEW")
	testutil.Contains(t, sql, `"active_users"`)
	testutil.Contains(t, sql, "SELECT * FROM users WHERE active = true")
}

func TestSanitizeIdentifier(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", `"simple"`},
		{"with_underscore", `"with_underscore"`},
		{"with-dash", `"with-dash"`},
		{"with space", `"with space"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			result := SanitizeIdentifier(tt.input)
			testutil.Equal(t, tt.expected, result)
		})
	}
}

func TestIsReservedWord(t *testing.T) {
	t.Parallel()
	tests := []struct {
		word     string
		reserved bool
	}{
		{"user", true},
		{"select", true},
		{"table", true},
		{"posts", false},
		{"custom_name", false},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			t.Parallel()
			result := IsReservedWord(tt.word)
			testutil.Equal(t, tt.reserved, result)
		})
	}
}
