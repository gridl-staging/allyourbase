package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

// fakeRlsQuerier is an in-memory fake for testing RLS handlers.
type fakeRlsQuerier struct {
	policies []RlsPolicy
	status   *RlsTableStatus
	execStmt string
	queryErr error
	execErr  error
	scanErr  error
}

type fakeRow struct {
	status  *RlsTableStatus
	scanErr error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	if r.status == nil {
		return fmt.Errorf("no row")
	}
	*(dest[0].(*bool)) = r.status.RlsEnabled
	*(dest[1].(*bool)) = r.status.ForceRls
	return nil
}

type fakeRows struct {
	policies []RlsPolicy
	idx      int
	scanErr  error
}

func (r *fakeRows) Next() bool {
	return r.idx < len(r.policies)
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	p := r.policies[r.idx]
	r.idx++
	*(dest[0].(*string)) = p.TableSchema
	*(dest[1].(*string)) = p.TableName
	*(dest[2].(*string)) = p.PolicyName
	*(dest[3].(*string)) = p.Command
	*(dest[4].(*string)) = p.Permissive
	*(dest[5].(*[]string)) = p.Roles
	*(dest[6].(**string)) = p.UsingExpr
	*(dest[7].(**string)) = p.WithCheckExpr
	return nil
}

func (r *fakeRows) Close()     {}
func (r *fakeRows) Err() error { return nil }

func (f *fakeRlsQuerier) QueryRow(_ context.Context, sql string, args ...any) pgxRow {
	return &fakeRow{status: f.status, scanErr: f.scanErr}
}

func (f *fakeRlsQuerier) Query(_ context.Context, sql string, args ...any) (pgxRows, error) {
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return &fakeRows{policies: f.policies}, nil
}

func (f *fakeRlsQuerier) Exec(_ context.Context, sql string, args ...any) error {
	f.execStmt = sql
	return f.execErr
}

func strPtr(s string) *string { return &s }

// --- List policies tests ---

func TestListRlsPoliciesEmpty(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{policies: nil}
	handler := handleListRlsPoliciesWithQuerier(q)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/rls", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var policies []RlsPolicy
	err := json.NewDecoder(w.Body).Decode(&policies)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(policies))
}

func TestListRlsPoliciesWithData(t *testing.T) {
	t.Parallel()
	using := "(user_id = current_setting('app.user_id')::uuid)"
	q := &fakeRlsQuerier{policies: []RlsPolicy{
		{
			TableSchema: "public", TableName: "posts", PolicyName: "owner_access",
			Command: "ALL", Permissive: "PERMISSIVE", Roles: []string{"authenticated"},
			UsingExpr: &using, WithCheckExpr: &using,
		},
		{
			TableSchema: "public", TableName: "posts", PolicyName: "public_read",
			Command: "SELECT", Permissive: "PERMISSIVE", Roles: []string{"PUBLIC"},
			UsingExpr: strPtr("true"), WithCheckExpr: nil,
		},
	}}
	handler := handleListRlsPoliciesWithQuerier(q)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/rls", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var policies []RlsPolicy
	err := json.NewDecoder(w.Body).Decode(&policies)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(policies))

	// Verify first policy fields deeply
	testutil.Equal(t, "owner_access", policies[0].PolicyName)
	testutil.Equal(t, "ALL", policies[0].Command)
	testutil.Equal(t, "PERMISSIVE", policies[0].Permissive)
	testutil.Equal(t, "public", policies[0].TableSchema)
	testutil.Equal(t, "posts", policies[0].TableName)
	testutil.Equal(t, 1, len(policies[0].Roles))
	testutil.Equal(t, "authenticated", policies[0].Roles[0])
	testutil.NotNil(t, policies[0].UsingExpr)
	testutil.Equal(t, using, *policies[0].UsingExpr)
	testutil.NotNil(t, policies[0].WithCheckExpr)

	// Verify second policy
	testutil.Equal(t, "public_read", policies[1].PolicyName)
	testutil.Equal(t, "SELECT", policies[1].Command)
	testutil.Equal(t, 1, len(policies[1].Roles))
	testutil.Equal(t, "PUBLIC", policies[1].Roles[0])
	testutil.NotNil(t, policies[1].UsingExpr)
	testutil.Equal(t, "true", *policies[1].UsingExpr)
	testutil.Nil(t, policies[1].WithCheckExpr)
}

func TestListRlsPoliciesByTable(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{policies: []RlsPolicy{
		{TableSchema: "public", TableName: "posts", PolicyName: "p1", Command: "ALL", Permissive: "PERMISSIVE", Roles: []string{}},
	}}

	r := chi.NewRouter()
	r.Get("/api/admin/rls/{table}", handleListRlsPoliciesWithQuerier(q))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/rls/posts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var policies []RlsPolicy
	err := json.NewDecoder(w.Body).Decode(&policies)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(policies))
	testutil.Equal(t, "p1", policies[0].PolicyName)
	testutil.Equal(t, "posts", policies[0].TableName)
}

func TestListRlsPoliciesQueryError(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{queryErr: fmt.Errorf("connection refused")}
	handler := handleListRlsPoliciesWithQuerier(q)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/rls", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to list policies")
}

// --- RLS status tests ---

func TestGetRlsStatusEnabled(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{status: &RlsTableStatus{RlsEnabled: true, ForceRls: false}}

	r := chi.NewRouter()
	r.Get("/api/admin/rls/{table}/status", handleGetRlsStatusWithQuerier(q))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/rls/posts/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var status RlsTableStatus
	err := json.NewDecoder(w.Body).Decode(&status)
	testutil.NoError(t, err)
	testutil.True(t, status.RlsEnabled, "RLS should be enabled")
	testutil.True(t, !status.ForceRls, "ForceRLS should be disabled")
}

func TestGetRlsStatusTableNotFound(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{status: nil, scanErr: fmt.Errorf("no rows")}

	r := chi.NewRouter()
	r.Get("/api/admin/rls/{table}/status", handleGetRlsStatusWithQuerier(q))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/rls/nonexistent/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "table not found")
}

func TestGetRlsStatusInvalidTable(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}

	r := chi.NewRouter()
	r.Get("/api/admin/rls/{table}/status", handleGetRlsStatusWithQuerier(q))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/rls/drop%20table/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid table name")
}

// --- Create policy tests ---

func TestCreateRlsPolicySuccess(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","name":"owner_only","command":"ALL","using":"(user_id = current_setting('app.user_id')::uuid)"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	testutil.Contains(t, w.Body.String(), "policy created")
	// Validate SQL structure (each part appears in the correct order)
	testutil.Contains(t, q.execStmt, `CREATE POLICY "owner_only"`)
	testutil.Contains(t, q.execStmt, `ON "public"."posts"`)
	testutil.Contains(t, q.execStmt, `FOR ALL`)
	testutil.Contains(t, q.execStmt, `USING (`)
	// Verify the expression is present and not truncated
	testutil.Contains(t, q.execStmt, `current_setting('app.user_id')::uuid`)
	// Verify no RESTRICTIVE keyword (default is PERMISSIVE)
	testutil.False(t, strings.Contains(q.execStmt, "RESTRICTIVE"), "default should be PERMISSIVE")
}

func TestCreateRlsPolicyWithRoles(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","name":"auth_read","command":"SELECT","roles":["authenticated"],"using":"true"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	testutil.Contains(t, q.execStmt, `FOR SELECT`)
	testutil.Contains(t, q.execStmt, `TO "authenticated"`)
}

func TestCreateRlsPolicyRestrictive(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","name":"restrict_ip","command":"ALL","permissive":false,"using":"true"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	testutil.Contains(t, q.execStmt, "AS RESTRICTIVE")
}

func TestCreateRlsPolicyMissingTable(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"name":"test_policy","command":"ALL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "table is required")
}

func TestCreateRlsPolicyMissingName(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","command":"ALL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "name is required")
}

func TestCreateRlsPolicyInvalidCommand(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","name":"test","command":"TRUNCATE"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "command must be one of")
}

func TestCreateRlsPolicySqlInjectionInTableName(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts; DROP TABLE users","name":"test","command":"ALL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid table name")
}

func TestCreateRlsPolicyExecError(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{execErr: fmt.Errorf("policy already exists")}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","name":"duplicate","command":"ALL","using":"true"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to create policy")
}

// --- Delete policy tests ---

func TestDeleteRlsPolicySuccess(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}

	r := chi.NewRouter()
	r.Delete("/api/admin/rls/{table}/{policy}", handleDeleteRlsPolicyWithQuerier(q))

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/rls/posts/owner_access", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Contains(t, q.execStmt, `DROP POLICY "owner_access"`)
	testutil.Contains(t, q.execStmt, `ON "posts"`)
}

func TestDeleteRlsPolicyInvalidName(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}

	r := chi.NewRouter()
	r.Delete("/api/admin/rls/{table}/{policy}", handleDeleteRlsPolicyWithQuerier(q))

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/rls/posts/drop%20table", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid policy name")
}

func TestDeleteRlsPolicyExecError(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{execErr: fmt.Errorf("policy does not exist")}

	r := chi.NewRouter()
	r.Delete("/api/admin/rls/{table}/{policy}", handleDeleteRlsPolicyWithQuerier(q))

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/rls/posts/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to drop policy")
}

// --- Enable/Disable RLS tests ---

func TestEnableRlsSuccess(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}

	r := chi.NewRouter()
	r.Post("/api/admin/rls/{table}/enable", handleEnableRlsWithQuerier(q))

	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls/posts/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "RLS enabled on posts")
	testutil.Contains(t, q.execStmt, `ENABLE ROW LEVEL SECURITY`)
}

func TestDisableRlsSuccess(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}

	r := chi.NewRouter()
	r.Post("/api/admin/rls/{table}/disable", handleDisableRlsWithQuerier(q))

	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls/posts/disable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "RLS disabled on posts")
	testutil.Contains(t, q.execStmt, `DISABLE ROW LEVEL SECURITY`)
}

func TestEnableRlsInvalidTable(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}

	r := chi.NewRouter()
	r.Post("/api/admin/rls/{table}/enable", handleEnableRlsWithQuerier(q))

	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls/1invalid/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid table name")
}

func TestEnableRlsExecError(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{execErr: fmt.Errorf("table not found")}

	r := chi.NewRouter()
	r.Post("/api/admin/rls/{table}/enable", handleEnableRlsWithQuerier(q))

	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls/nonexistent/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to enable RLS")
}

// --- Identifier validation tests ---

func TestIdentifierValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"simple name", "users", true},
		{"underscore name", "api_keys", true},
		{"leading underscore", "_private", true},
		{"with numbers", "table123", true},
		{"starts with number", "123table", false},
		{"space injection", "drop table", false},
		{"semicolon injection", "users;drop", false},
		{"empty string", "", false},
		{"sql comment", "users--", false},
		{"double quotes", `"users"`, false},
		{"single quotes", "'users'", false},
		{"parentheses", "users()", false},
		{"dot notation", "public.users", false},
		{"hyphen", "my-table", false},
		{"unicode", "tàble", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.want, isValidIdentifier(tt.input))
		})
	}
}

// --- SQL injection tests for additional vectors ---

func TestCreateRlsPolicySqlInjectionInPolicyName(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","name":"test'; DROP TABLE posts;--","command":"ALL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid policy name")
}

func TestCreateRlsPolicySqlInjectionInSchema(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","name":"test_policy","schema":"public; DROP TABLE users","command":"ALL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid schema name")
}

func TestCreateRlsPolicyInvalidRole(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","name":"test_policy","command":"ALL","roles":["admin; DROP TABLE users"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid role name")
}

func TestCreateRlsPolicyPublicRole(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","name":"public_read","command":"SELECT","roles":["PUBLIC"],"using":"true"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	testutil.Contains(t, q.execStmt, "TO PUBLIC")
}

func TestCreateRlsPolicyDefaultsToAllCommand(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	// Omit command - should default to ALL
	body := `{"table":"posts","name":"default_cmd","using":"true"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	testutil.Contains(t, q.execStmt, "FOR ALL")
}

func TestCreateRlsPolicyDefaultsToPublicSchema(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	// Omit schema - should default to public
	body := `{"table":"posts","name":"schema_test","command":"ALL","using":"true"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	testutil.Contains(t, q.execStmt, `ON "public"."posts"`)
}

func TestCreateRlsPolicyCustomSchema(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","name":"test","schema":"myapp","command":"ALL","using":"true"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	testutil.Contains(t, q.execStmt, `ON "myapp"."posts"`)
}

func TestCreateRlsPolicyWithCheckOnly(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	// INSERT policies typically use WITH CHECK but not USING
	body := `{"table":"posts","name":"insert_check","command":"INSERT","withCheck":"(tenant_id = current_setting('app.tenant_id')::uuid)"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	testutil.Contains(t, q.execStmt, "WITH CHECK (")
	testutil.False(t, strings.Contains(q.execStmt, "USING ("), "INSERT policy should not have USING")
}

func TestCreateRlsPolicyLowercaseCommand(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","name":"lower_cmd","command":"select","using":"true"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	testutil.Contains(t, q.execStmt, "FOR SELECT")
}

func TestCreateRlsPolicyPermissiveDefault(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	// Omit permissive field - should default to PERMISSIVE (no AS RESTRICTIVE)
	body := `{"table":"posts","name":"permissive_default","command":"ALL","using":"true"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	testutil.False(t, strings.Contains(q.execStmt, "RESTRICTIVE"), "default should be PERMISSIVE")
}

func TestDeleteRlsPolicySqlInjectionInTable(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}

	r := chi.NewRouter()
	r.Delete("/api/admin/rls/{table}/{policy}", handleDeleteRlsPolicyWithQuerier(q))

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/rls/posts%3B+DROP+TABLE+users/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid table name")
}

func TestDisableRlsInvalidTable(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}

	r := chi.NewRouter()
	r.Post("/api/admin/rls/{table}/disable", handleDisableRlsWithQuerier(q))

	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls/1invalid/disable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid table name")
}

func TestDisableRlsExecError(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{execErr: fmt.Errorf("table not found")}

	r := chi.NewRouter()
	r.Post("/api/admin/rls/{table}/disable", handleDisableRlsWithQuerier(q))

	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls/nonexistent/disable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "failed to disable RLS")
}

func TestGetRlsStatusDisabled(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{status: &RlsTableStatus{RlsEnabled: false, ForceRls: false}}

	r := chi.NewRouter()
	r.Get("/api/admin/rls/{table}/status", handleGetRlsStatusWithQuerier(q))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/rls/posts/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var status RlsTableStatus
	err := json.NewDecoder(w.Body).Decode(&status)
	testutil.NoError(t, err)
	testutil.True(t, !status.RlsEnabled, "RLS should be disabled")
	testutil.True(t, !status.ForceRls, "ForceRLS should be disabled")
}

func TestGetRlsStatusWithForceRls(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{status: &RlsTableStatus{RlsEnabled: true, ForceRls: true}}

	r := chi.NewRouter()
	r.Get("/api/admin/rls/{table}/status", handleGetRlsStatusWithQuerier(q))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/rls/posts/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var status RlsTableStatus
	err := json.NewDecoder(w.Body).Decode(&status)
	testutil.NoError(t, err)
	testutil.True(t, status.RlsEnabled, "RLS should be enabled")
	testutil.True(t, status.ForceRls, "ForceRLS should be enabled")
}

func TestListRlsPoliciesNilRolesReturnsEmptyArray(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{policies: []RlsPolicy{
		{
			TableSchema: "public", TableName: "posts", PolicyName: "open_policy",
			Command: "SELECT", Permissive: "PERMISSIVE", Roles: []string{},
			UsingExpr: strPtr("true"), WithCheckExpr: nil,
		},
	}}
	handler := handleListRlsPoliciesWithQuerier(q)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/rls", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var policies []RlsPolicy
	err := json.NewDecoder(w.Body).Decode(&policies)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(policies))
	testutil.Equal(t, 0, len(policies[0].Roles))
	testutil.Nil(t, policies[0].WithCheckExpr)
}

func TestCreateRlsPolicyAllValidCommands(t *testing.T) {
	t.Parallel()
	commands := []string{"ALL", "SELECT", "INSERT", "UPDATE", "DELETE"}
	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()
			q := &fakeRlsQuerier{}
			handler := handleCreateRlsPolicyWithQuerier(q)

			body := fmt.Sprintf(`{"table":"posts","name":"test_%s","command":"%s","using":"true"}`, strings.ToLower(cmd), cmd)
			req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			testutil.Equal(t, http.StatusCreated, w.Code)
			testutil.Contains(t, q.execStmt, fmt.Sprintf("FOR %s", cmd))
		})
	}
}

func TestEnableRlsVerifiesSQL(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}

	r := chi.NewRouter()
	r.Post("/api/admin/rls/{table}/enable", handleEnableRlsWithQuerier(q))

	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls/orders/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, q.execStmt, `ALTER TABLE "orders" ENABLE ROW LEVEL SECURITY`)
}

func TestDisableRlsVerifiesSQL(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}

	r := chi.NewRouter()
	r.Post("/api/admin/rls/{table}/disable", handleDisableRlsWithQuerier(q))

	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls/orders/disable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, q.execStmt, `ALTER TABLE "orders" DISABLE ROW LEVEL SECURITY`)
}

func TestCreateRlsPolicySqlInjectionInRole(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","name":"test","command":"ALL","roles":["valid_role","1nvalid"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid role name")
}

func TestCreateRlsPolicyMultipleRoles(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","name":"multi_role","command":"SELECT","roles":["admin","authenticated"],"using":"true"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	testutil.Contains(t, q.execStmt, `TO "admin", "authenticated"`)
}

func TestCreateRlsPolicyBothExpressions(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}
	handler := handleCreateRlsPolicyWithQuerier(q)

	body := `{"table":"posts","name":"both_expr","command":"UPDATE","using":"(user_id = uid())","withCheck":"(status != 'archived')"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusCreated, w.Code)
	testutil.Contains(t, q.execStmt, `FOR UPDATE`)
	testutil.Contains(t, q.execStmt, `USING (`)
	testutil.Contains(t, q.execStmt, `user_id = uid()`)
	testutil.Contains(t, q.execStmt, `WITH CHECK (`)
	testutil.Contains(t, q.execStmt, `status != 'archived'`)
}

func TestDeleteRlsPolicyVerifiesSQL(t *testing.T) {
	t.Parallel()
	q := &fakeRlsQuerier{}

	r := chi.NewRouter()
	r.Delete("/api/admin/rls/{table}/{policy}", handleDeleteRlsPolicyWithQuerier(q))

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/rls/users/owner_policy", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	// Verify exact SQL structure with quoted identifiers
	testutil.Equal(t, q.execStmt, `DROP POLICY "owner_policy" ON "users"`)
}
