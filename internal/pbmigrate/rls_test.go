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
		wantErr  string
	}{
		{
			name:     "auth.id replacement",
			rule:     "@request.auth.id",
			contains: []string{"current_setting('app.user_id', true)"},
		},
		{
			name:     "auth field access",
			rule:     "@request.auth.role",
			contains: []string{`SELECT "role" FROM "ayb_auth_users"`, "current_setting('app.user_id', true)"},
		},
		{
			name:    "collection reference rejected",
			rule:    "@collection.users.role = 'admin'",
			wantErr: "unsupported PocketBase token",
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
		{
			name:     "regex operator passthrough",
			rule:     "slug ~ '^[a-z0-9-]+$'",
			contains: []string{"slug ~ '^[a-z0-9-]+$'"},
		},
		{
			name:     "email literal with at sign preserved",
			rule:     "email = 'user@example.com'",
			contains: []string{"email = 'user@example.com'"},
		},
		{
			name:     "regex literal with at sign preserved",
			rule:     "email ~ '^[^@]+@example.com$'",
			contains: []string{"email ~ '^[^@]+@example.com$'"},
		},
		{
			name:     "quoted auth token literal preserved",
			rule:     "note = '@request.auth.id'",
			contains: []string{"note = '@request.auth.id'"},
		},
		{
			name:     "quoted nested auth token literal preserved",
			rule:     "note = '@request.auth.profile.name'",
			contains: []string{"note = '@request.auth.profile.name'"},
		},
		{
			name:     "quoted auth literal preserved while live token converts",
			rule:     "note = '@request.auth.id' && @request.auth.id != ''",
			contains: []string{"note = '@request.auth.id' AND current_setting('app.user_id', true) <> ''"},
		},
		{
			name:    "nested auth field access rejected",
			rule:    "@request.auth.profile.name = 'admin'",
			wantErr: "unsupported nested @request.auth field access",
		},
		{
			name:    "request data reference rejected",
			rule:    "@request.data.email != ''",
			wantErr: "unsupported PocketBase token",
		},
		// Explicitly supported PostgreSQL-equivalent syntax
		{
			name:     "negated regex operator passthrough",
			rule:     "slug !~ '^admin'",
			contains: []string{"slug !~ '^admin'"},
		},
		{
			name:     "literal boolean true passthrough",
			rule:     "active = true",
			contains: []string{"active = true"},
		},
		{
			name:     "literal boolean false passthrough",
			rule:     "deleted = false",
			contains: []string{"deleted = false"},
		},
		{
			name:     "function call passthrough as valid PostgreSQL",
			rule:     "LENGTH(name) > 0",
			contains: []string{"LENGTH(name) > 0"},
		},
		// PocketBase array filter operators must be rejected
		{
			name:    "array contains operator rejected",
			rule:    "tags ?~ 'sport'",
			wantErr: "unsupported PocketBase array operator",
		},
		{
			name:    "array equals operator rejected",
			rule:    "roles ?= 'admin'",
			wantErr: "unsupported PocketBase array operator",
		},
		{
			name:    "array not-equals operator rejected",
			rule:    "tags ?!= 'spam'",
			wantErr: "unsupported PocketBase array operator",
		},
		{
			name:    "array negated-contains operator rejected",
			rule:    "tags ?!~ 'draft'",
			wantErr: "unsupported PocketBase array operator",
		},
		// Quoted array operator literal must NOT be rejected
		{
			name:     "quoted array operator preserved",
			rule:     "note = 'tags ?~ sport'",
			contains: []string{"note = 'tags ?~ sport'"},
		},
		{
			name:    "stacked statement rejected",
			rule:    "true); DROP TABLE users; --",
			wantErr: "multiple SQL statements are not allowed",
		},
		{
			name:    "sql comment rejected",
			rule:    "true -- bypass",
			wantErr: "SQL comments are not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cl := convertRuleExpression(tt.rule)
			if tt.wantErr != "" {
				testutil.Equal(t, RuleStatusUnsupported, cl.Status)
				testutil.Contains(t, cl.Diagnostic, tt.wantErr)
				return
			}
			testutil.Equal(t, RuleStatusConvertible, cl.Status)

			for _, substr := range tt.contains {
				if !strings.Contains(cl.PgExpr, substr) {
					t.Errorf("expected result to contain %q, got: %s", substr, cl.PgExpr)
				}
			}
		})
	}
}

func TestClassifyRule(t *testing.T) {
	t.Parallel()

	t.Run("nil rule is locked", func(t *testing.T) {
		t.Parallel()
		cl := classifyRule(nil)
		testutil.Equal(t, RuleStatusLocked, cl.Status)
		testutil.Equal(t, "", cl.PgExpr)
		testutil.Contains(t, cl.Diagnostic, "locked")
	})

	t.Run("empty rule is open (convertible to true)", func(t *testing.T) {
		t.Parallel()
		cl := classifyRule(strPtr(""))
		testutil.Equal(t, RuleStatusConvertible, cl.Status)
		testutil.Equal(t, "true", cl.PgExpr)
	})

	t.Run("convertible expression", func(t *testing.T) {
		t.Parallel()
		cl := classifyRule(strPtr("@request.auth.id != ''"))
		testutil.Equal(t, RuleStatusConvertible, cl.Status)
		testutil.Contains(t, cl.PgExpr, "current_setting")
		testutil.Equal(t, "@request.auth.id != ''", cl.Rule)
	})

	t.Run("unsupported expression", func(t *testing.T) {
		t.Parallel()
		cl := classifyRule(strPtr("@collection.users.id = '123'"))
		testutil.Equal(t, RuleStatusUnsupported, cl.Status)
		testutil.Equal(t, "", cl.PgExpr)
		testutil.Contains(t, cl.Diagnostic, "unsupported")
	})
}

func TestGenerateRLSPolicies_UnsupportedRuleReturnsError(t *testing.T) {
	t.Parallel()

	coll := PBCollection{
		Name:       "posts",
		Type:       "base",
		ListRule:   strPtr(""),
		ViewRule:   strPtr(""),
		CreateRule: strPtr("@request.data.email != ''"),
		UpdateRule: nil,
		DeleteRule: nil,
	}

	policies, _, err := GenerateRLSPolicies(coll)
	testutil.ErrorContains(t, err, "unsupported PocketBase token")
	testutil.Nil(t, policies)
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
			policies, _, err := GenerateRLSPolicies(tt.coll)
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

func TestGenerateRLSPolicies_ReturnsDiagnostics(t *testing.T) {
	t.Parallel()

	t.Run("diagnostics for non-convertible rules", func(t *testing.T) {
		t.Parallel()
		coll := PBCollection{
			Name:       "posts",
			Type:       "base",
			ListRule:   strPtr(""),
			CreateRule: strPtr("@request.data.email != ''"), // non-convertible
			UpdateRule: strPtr("@request.auth.id = author"), // convertible
		}

		policies, diags, err := GenerateRLSPolicies(coll)
		testutil.ErrorContains(t, err, "unsupported PocketBase token")
		testutil.Nil(t, policies)
		testutil.SliceLen(t, diags, 1)
		testutil.Equal(t, "posts", diags[0].Collection)
		testutil.Equal(t, "create", diags[0].Action)
		testutil.Contains(t, diags[0].Message, "unsupported")
	})

	t.Run("no diagnostics when all convertible", func(t *testing.T) {
		t.Parallel()
		coll := PBCollection{
			Name:     "posts",
			Type:     "base",
			ListRule: strPtr(""),
		}

		policies, diags, err := GenerateRLSPolicies(coll)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(policies))
		testutil.Nil(t, diags)
	})

	t.Run("multiple non-convertible rules all diagnosed", func(t *testing.T) {
		t.Parallel()
		coll := PBCollection{
			Name:       "posts",
			Type:       "base",
			ListRule:   strPtr("tags ?~ 'important'"),         // non-convertible
			CreateRule: strPtr("@request.data.email != ''"),   // non-convertible
			UpdateRule: strPtr("@request.auth.id = author"),   // convertible
			DeleteRule: strPtr("@collection.admins.id != ''"), // non-convertible
		}

		policies, diags, err := GenerateRLSPolicies(coll)
		testutil.ErrorContains(t, err, "unsupported")
		testutil.Nil(t, policies)
		testutil.Equal(t, 3, len(diags))
	})
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

// TestConvertRuleExpression_ClassificationContract verifies the three-bucket
// classification contract: every rule expression must be classified as either
// convertible (RuleStatusConvertible with PgExpr set), or unsupported
// (RuleStatusUnsupported with Diagnostic set). Raw PocketBase tokens or
// partially converted expressions must never appear in a convertible result.
func TestConvertRuleExpression_ClassificationContract(t *testing.T) {
	t.Parallel()

	// Bucket 1: Convertible — returns RuleStatusConvertible with no raw PocketBase tokens
	convertible := []struct {
		name string
		rule string
	}{
		{"auth id", "@request.auth.id != ''"},
		{"auth field", "@request.auth.role = 'admin'"},
		{"logical operators", "a = 1 && b = 2 || c = 3"},
		{"not-equals", "status != 'draft'"},
		{"regex match", "slug ~ '^[a-z]+$'"},
		{"negated regex", "slug !~ '^admin'"},
		{"boolean true literal", "active = true"},
		{"boolean false literal", "deleted = false"},
		{"quoted at-sign literal", "email = 'user@example.com'"},
		{"plain column comparison", "status = 'published'"},
	}

	for _, tc := range convertible {
		t.Run("convertible/"+tc.name, func(t *testing.T) {
			t.Parallel()
			cl := convertRuleExpression(tc.rule)
			testutil.Equal(t, RuleStatusConvertible, cl.Status)
			// Converted expression must not contain raw PocketBase tokens
			if token := findUnsupportedPocketBaseToken(cl.PgExpr); token != "" {
				t.Errorf("converted expression contains raw PocketBase token %q: %s", token, cl.PgExpr)
			}
		})
	}

	// Buckets 2+3: Non-convertible — returns RuleStatusUnsupported with diagnostic
	nonConvertible := []struct {
		name         string
		rule         string
		diagContains string
	}{
		{"@collection reference", "@collection.users.role = 'admin'", "unsupported PocketBase token"},
		{"@request.data reference", "@request.data.email != ''", "unsupported PocketBase token"},
		{"nested @request.auth", "@request.auth.profile.name = 'admin'", "unsupported nested"},
		{"array ?~ operator", "tags ?~ 'sport'", "unsupported PocketBase array operator"},
		{"array ?= operator", "roles ?= 'admin'", "unsupported PocketBase array operator"},
		{"array ?!= operator", "tags ?!= 'spam'", "unsupported PocketBase array operator"},
		{"array ?!~ operator", "tags ?!~ 'draft'", "unsupported PocketBase array operator"},
	}

	for _, tc := range nonConvertible {
		t.Run("non-convertible/"+tc.name, func(t *testing.T) {
			t.Parallel()
			cl := convertRuleExpression(tc.rule)
			testutil.Equal(t, RuleStatusUnsupported, cl.Status)
			testutil.Contains(t, cl.Diagnostic, tc.diagContains)
		})
	}
}

// TestGenerateRLSPolicies_OnlyEmitsConvertible verifies that GenerateRLSPolicies
// classifies each rule and never emits policy SQL for non-convertible rules.
// It returns nil policies (not partial results) when any rule is unsupported.
func TestGenerateRLSPolicies_OnlyEmitsConvertible(t *testing.T) {
	t.Parallel()

	t.Run("no partial policies when one rule non-convertible", func(t *testing.T) {
		t.Parallel()
		coll := PBCollection{
			Name:       "posts",
			Type:       "base",
			ListRule:   strPtr(""),                           // convertible (open)
			CreateRule: strPtr("@collection.users.id != ''"), // non-convertible
			UpdateRule: strPtr("@request.auth.id = author"),  // convertible
			DeleteRule: strPtr("@request.auth.id = author"),  // convertible
		}

		policies, diags, err := GenerateRLSPolicies(coll)
		testutil.ErrorContains(t, err, "unsupported PocketBase token")
		// Must return nil, not partial policies from the convertible rules
		testutil.Nil(t, policies)
		// Diagnostic must be present for the non-convertible rule
		testutil.SliceLen(t, diags, 1)
		testutil.Equal(t, "create", diags[0].Action)
	})

	t.Run("emits all policies when all rules convertible", func(t *testing.T) {
		t.Parallel()
		coll := PBCollection{
			Name:       "posts",
			Type:       "base",
			ListRule:   strPtr(""),
			CreateRule: strPtr("@request.auth.id != ''"),
			UpdateRule: strPtr("@request.auth.id = author"),
			DeleteRule: nil, // locked, skipped
		}

		policies, diags, err := GenerateRLSPolicies(coll)
		testutil.NoError(t, err)
		testutil.Equal(t, 3, len(policies))
		testutil.Nil(t, diags)
		for _, p := range policies {
			testutil.Contains(t, p, "CREATE POLICY")
		}
	})

	t.Run("array operator in rule prevents emission", func(t *testing.T) {
		t.Parallel()
		coll := PBCollection{
			Name:     "posts",
			Type:     "base",
			ListRule: strPtr("tags ?~ 'important'"),
		}

		policies, _, err := GenerateRLSPolicies(coll)
		testutil.ErrorContains(t, err, "unsupported PocketBase array operator")
		testutil.Nil(t, policies)
	})
}

// TestConvertRuleExpression_MixedQuotedLiteralsWithLiveTokens verifies that
// quoted PocketBase tokens (string literals) survive alongside live tokens that
// are either successfully converted or correctly rejected.
func TestConvertRuleExpression_MixedQuotedLiteralsWithLiveTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		rule         string
		wantStatus   RuleStatus
		contains     []string // for convertible: substrings expected in PgExpr
		diagContains string   // for unsupported: substring expected in Diagnostic
	}{
		{
			name:       "quoted @collection literal with convertible live token",
			rule:       "note = '@collection.users.id' && @request.auth.id != ''",
			wantStatus: RuleStatusConvertible,
			contains:   []string{"note = '@collection.users.id'", "current_setting('app.user_id', true) <> ''"},
		},
		{
			name:       "quoted @request.data literal with convertible live token",
			rule:       "desc = '@request.data.title' && @request.auth.id != ''",
			wantStatus: RuleStatusConvertible,
			contains:   []string{"desc = '@request.data.title'", "current_setting('app.user_id', true) <> ''"},
		},
		{
			name:         "live unsupported token next to quoted convertible literal",
			rule:         "note = '@request.auth.id' && @request.data.email != ''",
			wantStatus:   RuleStatusUnsupported,
			diagContains: "unsupported PocketBase token",
		},
		{
			name:         "quoted array op literal with live unsupported @collection token",
			rule:         "label = 'tags ?~ sport' && @collection.admins.id != ''",
			wantStatus:   RuleStatusUnsupported,
			diagContains: "unsupported PocketBase token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cl := convertRuleExpression(tt.rule)
			testutil.Equal(t, tt.wantStatus, cl.Status)
			if tt.wantStatus == RuleStatusConvertible {
				for _, substr := range tt.contains {
					if !strings.Contains(cl.PgExpr, substr) {
						t.Errorf("expected PgExpr to contain %q, got: %s", substr, cl.PgExpr)
					}
				}
			} else {
				testutil.Contains(t, cl.Diagnostic, tt.diagContains)
			}
		})
	}
}

// TestMigrateRLS_DiagnosticTextAndAttribution complements the migrator_unit_test
// by verifying exact diagnostic message content and collection attribution from
// the rls.go layer (via GenerateRLSPolicies).
func TestGenerateRLSPolicies_ExactDiagnosticAttribution(t *testing.T) {
	t.Parallel()

	coll := PBCollection{
		Name:       "articles",
		Type:       "base",
		ListRule:   strPtr(""),
		CreateRule: strPtr("@collection.editors.role = 'editor'"),
	}

	policies, diags, err := GenerateRLSPolicies(coll)
	testutil.ErrorContains(t, err, "unsupported PocketBase token")
	testutil.Nil(t, policies)
	testutil.SliceLen(t, diags, 1)

	d := diags[0]
	testutil.Equal(t, "articles", d.Collection)
	testutil.Equal(t, "create", d.Action)
	testutil.Equal(t, "@collection.editors.role = 'editor'", d.Rule)
	testutil.Contains(t, d.Message, "@collection.editors.role")
	testutil.Contains(t, d.Message, "manual review required")
}

// Helper to create string pointer
func strPtr(s string) *string {
	return &s
}
