package pbmigrate

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestConvertRuleToRLS(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		tableName string
		action    string
		rule      *string
		contains  []string
		empty     bool
	}{
		{
			name:      "null rule (locked)",
			tableName: "posts",
			action:    "list",
			rule:      nil,
			empty:     true,
		},
		{
			name:      "empty rule (open to all)",
			tableName: "posts",
			action:    "list",
			rule:      strPtr(""),
			contains:  []string{"CREATE POLICY", "FOR SELECT", "USING (true)"},
		},
		{
			name:      "authenticated user check",
			tableName: "posts",
			action:    "list",
			rule:      strPtr("@request.auth.id != ''"),
			contains:  []string{"current_setting('app.user_id', true)", "<>", "''"},
		},
		{
			name:      "owner check",
			tableName: "posts",
			action:    "update",
			rule:      strPtr("@request.auth.id = author_id"),
			contains:  []string{"current_setting('app.user_id', true)", "=", "author_id"},
		},
		{
			name:      "complex expression with AND",
			tableName: "posts",
			action:    "list",
			rule:      strPtr("@request.auth.id = author && status = 'published'"),
			contains:  []string{"current_setting('app.user_id', true)", "AND", "status", "published"},
		},
		{
			name:      "complex expression with OR",
			tableName: "posts",
			action:    "list",
			rule:      strPtr("public = true || @request.auth.id = author"),
			contains:  []string{"public", "true", "OR", "current_setting('app.user_id', true)"},
		},
		{
			name:      "create rule uses WITH CHECK",
			tableName: "posts",
			action:    "create",
			rule:      strPtr("@request.auth.id != ''"),
			contains:  []string{"FOR INSERT", "WITH CHECK"},
		},
		{
			name:      "delete rule uses USING",
			tableName: "posts",
			action:    "delete",
			rule:      strPtr("@request.auth.id = author"),
			contains:  []string{"FOR DELETE", "USING"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sql, err := ConvertRuleToRLS(tt.tableName, tt.action, tt.rule)
			testutil.NoError(t, err)

			if tt.empty {
				testutil.Equal(t, "", sql)
				return
			}

			for _, substr := range tt.contains {
				if !strings.Contains(sql, substr) {
					t.Errorf("expected SQL to contain %q, got:\n%s", substr, sql)
				}
			}
		})
	}
}

func TestConvertRuleExpression(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		rule     string
		contains []string
	}{
		{
			name:     "auth.id replacement",
			rule:     "@request.auth.id",
			contains: []string{"current_setting('app.user_id', true)"},
		},
		{
			name:     "auth field access",
			rule:     "@request.auth.role",
			contains: []string{"SELECT role FROM ayb_auth_users", "current_setting('app.user_id', true)"},
		},
		{
			name:     "collection reference",
			rule:     "@collection.users.role = 'admin'",
			contains: []string{"SELECT role FROM users", "current_setting('app.user_id', true)"},
		},
		{
			name:     "AND operator",
			rule:     "a = 1 && b = 2",
			contains: []string{"a = 1 AND b = 2"},
		},
		{
			name:     "OR operator",
			rule:     "a = 1 || b = 2",
			contains: []string{"a = 1 OR b = 2"},
		},
		{
			name:     "not equals",
			rule:     "status != 'draft'",
			contains: []string{"status <> 'draft'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := convertRuleExpression(tt.rule)
			testutil.NoError(t, err)

			for _, substr := range tt.contains {
				if !strings.Contains(result, substr) {
					t.Errorf("expected result to contain %q, got: %s", substr, result)
				}
			}
		})
	}
}

func TestGenerateRLSPolicies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		coll         PBCollection
		expectCount  int
		expectEnable bool
	}{
		{
			name: "all rules defined",
			coll: PBCollection{
				Name:       "posts",
				ListRule:   strPtr(""),
				ViewRule:   strPtr(""),
				CreateRule: strPtr("@request.auth.id != ''"),
				UpdateRule: strPtr("@request.auth.id = author"),
				DeleteRule: strPtr("@request.auth.id = author"),
			},
			expectCount: 4, // list (SELECT) + create + update + delete
		},
		{
			name: "some rules locked",
			coll: PBCollection{
				Name:       "posts",
				ListRule:   strPtr(""),
				ViewRule:   strPtr(""),
				CreateRule: nil, // locked
				UpdateRule: nil, // locked
				DeleteRule: nil, // locked
			},
			expectCount: 1, // only list (SELECT)
		},
		{
			name: "all rules locked",
			coll: PBCollection{
				Name:       "admin_data",
				ListRule:   nil,
				ViewRule:   nil,
				CreateRule: nil,
				UpdateRule: nil,
				DeleteRule: nil,
			},
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			policies, err := GenerateRLSPolicies(tt.coll)
			testutil.NoError(t, err)
			testutil.Equal(t, tt.expectCount, len(policies))

			// Verify each policy is valid SQL
			for _, policy := range policies {
				testutil.Contains(t, policy, "CREATE POLICY")
				testutil.Contains(t, policy, tt.coll.Name)
			}
		})
	}
}

func TestEnableRLS(t *testing.T) {
	t.Parallel()
	sql := EnableRLS("posts")
	testutil.Contains(t, sql, "ALTER TABLE")
	testutil.Contains(t, sql, `"posts"`)
	testutil.Contains(t, sql, "ENABLE ROW LEVEL SECURITY")
}

func TestBuildRLSPolicy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		tableName  string
		action     string
		expression string
		contains   []string
	}{
		{
			name:       "SELECT policy",
			tableName:  "posts",
			action:     "list",
			expression: "true",
			contains:   []string{"CREATE POLICY", `"posts_list_policy"`, "FOR SELECT", "USING (true)"},
		},
		{
			name:       "INSERT policy",
			tableName:  "posts",
			action:     "create",
			expression: "auth_check()",
			contains:   []string{"FOR INSERT", "WITH CHECK (auth_check())"},
		},
		{
			name:       "UPDATE policy",
			tableName:  "posts",
			action:     "update",
			expression: "owner_check()",
			contains:   []string{"FOR UPDATE", "USING (owner_check())"},
		},
		{
			name:       "DELETE policy",
			tableName:  "posts",
			action:     "delete",
			expression: "owner_check()",
			contains:   []string{"FOR DELETE", "USING (owner_check())"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sql := buildRLSPolicy(tt.tableName, tt.action, tt.expression)

			for _, substr := range tt.contains {
				if !strings.Contains(sql, substr) {
					t.Errorf("expected SQL to contain %q, got:\n%s", substr, sql)
				}
			}
		})
	}
}

// Helper to create string pointer
func strPtr(s string) *string {
	return &s
}
