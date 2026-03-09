package server

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RlsPolicy represents a row-level security policy on a table.
type RlsPolicy struct {
	TableSchema   string   `json:"tableSchema"`
	TableName     string   `json:"tableName"`
	PolicyName    string   `json:"policyName"`
	Command       string   `json:"command"`
	Permissive    string   `json:"permissive"`
	Roles         []string `json:"roles"`
	UsingExpr     *string  `json:"usingExpr"`
	WithCheckExpr *string  `json:"withCheckExpr"`
}

// RlsTableStatus indicates whether RLS is enabled on a table.
type RlsTableStatus struct {
	RlsEnabled bool `json:"rlsEnabled"`
	ForceRls   bool `json:"forceRls"`
}

// rlsQuerier abstracts database access for testing.
type rlsQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgxRow
	Query(ctx context.Context, sql string, args ...any) (pgxRows, error)
	Exec(ctx context.Context, sql string, args ...any) error
}

// pgxRow matches pgx's Row interface.
type pgxRow interface {
	Scan(dest ...any) error
}

// pgxRows matches pgx's Rows interface.
type pgxRows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}

// poolAdapter wraps pgxpool.Pool to satisfy rlsQuerier.
type poolAdapter struct {
	pool *pgxpool.Pool
}

func (a *poolAdapter) QueryRow(ctx context.Context, sql string, args ...any) pgxRow {
	return a.pool.QueryRow(ctx, sql, args...)
}

func (a *poolAdapter) Query(ctx context.Context, sql string, args ...any) (pgxRows, error) {
	rows, err := a.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (a *poolAdapter) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := a.pool.Exec(ctx, sql, args...)
	return err
}

// identifierRE validates SQL identifiers (table/policy names).
var identifierRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func isValidIdentifier(s string) bool {
	return identifierRE.MatchString(s)
}

// handleListRlsPolicies returns all RLS policies, optionally filtered by table.
func handleListRlsPolicies(pool *pgxpool.Pool) http.HandlerFunc {
	q := &poolAdapter{pool: pool}
	return handleListRlsPoliciesWithQuerier(q)
}

func handleListRlsPoliciesWithQuerier(q rlsQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		table := chi.URLParam(r, "table")
		policies, err := listPolicies(r.Context(), q, table)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list policies")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, policies)
	}
}

func listPolicies(ctx context.Context, q rlsQuerier, table string) ([]RlsPolicy, error) {
	query := `
		SELECT
			n.nspname AS table_schema,
			c.relname AS table_name,
			p.polname AS policy_name,
			CASE p.polcmd
				WHEN 'r' THEN 'SELECT'
				WHEN 'a' THEN 'INSERT'
				WHEN 'w' THEN 'UPDATE'
				WHEN 'd' THEN 'DELETE'
				WHEN '*' THEN 'ALL'
			END AS command,
			CASE WHEN p.polpermissive THEN 'PERMISSIVE' ELSE 'RESTRICTIVE' END AS permissive,
			COALESCE(ARRAY(
				SELECT rolname FROM pg_roles WHERE oid = ANY(p.polroles)
			), ARRAY[]::text[]) AS roles,
			pg_get_expr(p.polqual, p.polrelid) AS using_expr,
			pg_get_expr(p.polwithcheck, p.polrelid) AS with_check_expr
		FROM pg_policy p
		JOIN pg_class c ON c.oid = p.polrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
	`
	args := []any{}
	if table != "" {
		query += " AND c.relname = $1"
		args = append(args, table)
	}
	query += " ORDER BY n.nspname, c.relname, p.polname"

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query policies: %w", err)
	}
	defer rows.Close()

	var policies []RlsPolicy
	for rows.Next() {
		var pol RlsPolicy
		if err := rows.Scan(
			&pol.TableSchema, &pol.TableName, &pol.PolicyName,
			&pol.Command, &pol.Permissive, &pol.Roles,
			&pol.UsingExpr, &pol.WithCheckExpr,
		); err != nil {
			return nil, fmt.Errorf("scan policy: %w", err)
		}
		policies = append(policies, pol)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	if policies == nil {
		policies = []RlsPolicy{}
	}
	return policies, nil
}

// handleGetRlsStatus returns whether RLS is enabled on a table.
func handleGetRlsStatus(pool *pgxpool.Pool) http.HandlerFunc {
	q := &poolAdapter{pool: pool}
	return handleGetRlsStatusWithQuerier(q)
}

func handleGetRlsStatusWithQuerier(q rlsQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		table := chi.URLParam(r, "table")
		if table == "" {
			httputil.WriteError(w, http.StatusBadRequest, "table name is required")
			return
		}
		if !isValidIdentifier(table) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid table name")
			return
		}

		var status RlsTableStatus
		err := q.QueryRow(r.Context(),
			`SELECT relrowsecurity, relforcerowsecurity
			 FROM pg_class c
			 JOIN pg_namespace n ON n.oid = c.relnamespace
			 WHERE c.relname = $1 AND n.nspname NOT IN ('pg_catalog', 'information_schema')
			 LIMIT 1`, table,
		).Scan(&status.RlsEnabled, &status.ForceRls)
		if err != nil {
			httputil.WriteError(w, http.StatusNotFound, "table not found")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, status)
	}
}

type createPolicyRequest struct {
	Table      string   `json:"table"`
	Schema     string   `json:"schema"`
	Name       string   `json:"name"`
	Command    string   `json:"command"`
	Permissive *bool    `json:"permissive"`
	Roles      []string `json:"roles"`
	Using      string   `json:"using"`
	WithCheck  string   `json:"withCheck"`
}

// handleCreateRlsPolicy creates a new RLS policy.
func handleCreateRlsPolicy(pool *pgxpool.Pool) http.HandlerFunc {
	q := &poolAdapter{pool: pool}
	return handleCreateRlsPolicyWithQuerier(q)
}

func handleCreateRlsPolicyWithQuerier(q rlsQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createPolicyRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.Table == "" {
			httputil.WriteError(w, http.StatusBadRequest, "table is required")
			return
		}
		if req.Name == "" {
			httputil.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if !isValidIdentifier(req.Table) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid table name")
			return
		}
		if !isValidIdentifier(req.Name) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid policy name")
			return
		}

		schema := req.Schema
		if schema == "" {
			schema = "public"
		}
		if !isValidIdentifier(schema) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid schema name")
			return
		}

		cmd := strings.ToUpper(req.Command)
		if cmd == "" {
			cmd = "ALL"
		}
		validCommands := map[string]bool{"ALL": true, "SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true}
		if !validCommands[cmd] {
			httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "command must be one of: ALL, SELECT, INSERT, UPDATE, DELETE",
				"https://allyourbase.io/guide/authentication#row-level-security-rls")
			return
		}

		// Build CREATE POLICY statement using quoted identifiers for safety.
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf(`CREATE POLICY %q ON %q.%q`, req.Name, schema, req.Table))

		// Permissive/Restrictive (default PERMISSIVE)
		if req.Permissive != nil && !*req.Permissive {
			sb.WriteString(" AS RESTRICTIVE")
		}

		sb.WriteString(fmt.Sprintf(" FOR %s", cmd))

		if len(req.Roles) > 0 {
			quoted := make([]string, len(req.Roles))
			for i, role := range req.Roles {
				if role == "PUBLIC" || role == "public" {
					quoted[i] = "PUBLIC"
				} else {
					if !isValidIdentifier(role) {
						httputil.WriteError(w, http.StatusBadRequest, "invalid role name: "+role)
						return
					}
					quoted[i] = fmt.Sprintf("%q", role)
				}
			}
			sb.WriteString(fmt.Sprintf(" TO %s", strings.Join(quoted, ", ")))
		}

		if req.Using != "" {
			sb.WriteString(fmt.Sprintf(" USING (%s)", req.Using))
		}
		if req.WithCheck != "" {
			sb.WriteString(fmt.Sprintf(" WITH CHECK (%s)", req.WithCheck))
		}

		if err := q.Exec(r.Context(), sb.String()); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "failed to create policy: "+err.Error())
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, map[string]string{"message": "policy created"})
	}
}

// handleDeleteRlsPolicy drops an RLS policy.
func handleDeleteRlsPolicy(pool *pgxpool.Pool) http.HandlerFunc {
	q := &poolAdapter{pool: pool}
	return handleDeleteRlsPolicyWithQuerier(q)
}

func handleDeleteRlsPolicyWithQuerier(q rlsQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		table := chi.URLParam(r, "table")
		policy := chi.URLParam(r, "policy")

		if table == "" || policy == "" {
			httputil.WriteError(w, http.StatusBadRequest, "table and policy name are required")
			return
		}
		if !isValidIdentifier(table) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid table name")
			return
		}
		if !isValidIdentifier(policy) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid policy name")
			return
		}

		stmt := fmt.Sprintf(`DROP POLICY %q ON %q`, policy, table)
		if err := q.Exec(r.Context(), stmt); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "failed to drop policy: "+err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleEnableRls enables RLS on a table.
func handleEnableRls(pool *pgxpool.Pool) http.HandlerFunc {
	q := &poolAdapter{pool: pool}
	return handleEnableRlsWithQuerier(q)
}

func handleEnableRlsWithQuerier(q rlsQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		table := chi.URLParam(r, "table")
		if table == "" {
			httputil.WriteError(w, http.StatusBadRequest, "table name is required")
			return
		}
		if !isValidIdentifier(table) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid table name")
			return
		}

		stmt := fmt.Sprintf(`ALTER TABLE %q ENABLE ROW LEVEL SECURITY`, table)
		if err := q.Exec(r.Context(), stmt); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "failed to enable RLS: "+err.Error())
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "RLS enabled on " + table})
	}
}

// handleDisableRls disables RLS on a table.
func handleDisableRls(pool *pgxpool.Pool) http.HandlerFunc {
	q := &poolAdapter{pool: pool}
	return handleDisableRlsWithQuerier(q)
}

func handleDisableRlsWithQuerier(q rlsQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		table := chi.URLParam(r, "table")
		if table == "" {
			httputil.WriteError(w, http.StatusBadRequest, "table name is required")
			return
		}
		if !isValidIdentifier(table) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid table name")
			return
		}

		stmt := fmt.Sprintf(`ALTER TABLE %q DISABLE ROW LEVEL SECURITY`, table)
		if err := q.Exec(r.Context(), stmt); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "failed to disable RLS: "+err.Error())
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "RLS disabled on " + table})
	}
}
