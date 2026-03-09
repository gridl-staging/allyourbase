package sbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// Compiled patterns for Supabase → AYB RLS expression rewriting.
// Order matters: more specific patterns must come before general ones.
var rlsReplacements = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	// (auth.uid())::text and (uid())::text → current_setting('ayb.user_id', true)
	{regexp.MustCompile(`\((?:auth\.)?uid\(\)\)::text`), "current_setting('ayb.user_id', true)"},
	// auth.uid() and uid() → current_setting('ayb.user_id', true)::uuid
	{regexp.MustCompile(`(?:auth\.)?uid\(\)`), "current_setting('ayb.user_id', true)::uuid"},
	// auth.role() and role() → current_setting('ayb.user_role', true)
	{regexp.MustCompile(`(?:auth\.)?role\(\)`), "current_setting('ayb.user_role', true)"},
	// auth.jwt() ->> 'email' and jwt() ->> 'email' → current_setting('ayb.user_email', true)
	{regexp.MustCompile(`(?:auth\.)?jwt\(\)\s*->>\s*'email'`), "current_setting('ayb.user_email', true)"},
}

// RewriteRLSExpression replaces Supabase auth function references with
// AYB session variable equivalents.
func RewriteRLSExpression(expr string) string {
	if expr == "" {
		return expr
	}
	for _, r := range rlsReplacements {
		expr = r.pattern.ReplaceAllString(expr, r.replacement)
	}
	return expr
}

// ReadRLSPolicies reads existing RLS policies from the public schema.
func ReadRLSPolicies(ctx context.Context, db *sql.DB) ([]RLSPolicy, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT pol.polname,
		       c.relname,
		       n.nspname,
		       CASE pol.polcmd
		           WHEN 'r' THEN 'SELECT'
		           WHEN 'a' THEN 'INSERT'
		           WHEN 'w' THEN 'UPDATE'
		           WHEN 'd' THEN 'DELETE'
		           WHEN '*' THEN 'ALL'
		       END,
		       pol.polpermissive,
		       pg_get_expr(pol.polqual, pol.polrelid),
		       pg_get_expr(pol.polwithcheck, pol.polrelid)
		FROM pg_policy pol
		JOIN pg_class c ON c.oid = pol.polrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		ORDER BY c.relname, pol.polname
	`)
	if err != nil {
		return nil, fmt.Errorf("querying RLS policies: %w", err)
	}
	defer rows.Close()

	var policies []RLSPolicy
	for rows.Next() {
		var p RLSPolicy
		var usingExpr, checkExpr sql.NullString
		if err := rows.Scan(
			&p.PolicyName, &p.TableName, &p.SchemaName,
			&p.Command, &p.Permissive, &usingExpr, &checkExpr,
		); err != nil {
			return nil, fmt.Errorf("scanning policy: %w", err)
		}
		if usingExpr.Valid {
			p.UsingExpr = usingExpr.String
		}
		if checkExpr.Valid {
			p.CheckExpr = checkExpr.String
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

// GenerateRewrittenPolicy produces a CREATE POLICY SQL statement with
// Supabase auth references rewritten to AYB session variables.
func GenerateRewrittenPolicy(p RLSPolicy) string {
	var sb strings.Builder

	permissive := "PERMISSIVE"
	if !p.Permissive {
		permissive = "RESTRICTIVE"
	}

	fmt.Fprintf(&sb, "CREATE POLICY %q ON %q.%q AS %s FOR %s",
		p.PolicyName, p.SchemaName, p.TableName, permissive, p.Command)

	if p.UsingExpr != "" {
		fmt.Fprintf(&sb, " USING (%s)", RewriteRLSExpression(p.UsingExpr))
	}
	if p.CheckExpr != "" {
		fmt.Fprintf(&sb, " WITH CHECK (%s)", RewriteRLSExpression(p.CheckExpr))
	}

	sb.WriteString(";")
	return sb.String()
}
