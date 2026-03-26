package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/allyourbase/ayb/internal/sqlutil"
)

// AuthenticatedRole is the Postgres role used for authenticated API requests.
// SET LOCAL ROLE switches to this role within each request transaction so
// RLS policies are enforced even when the pool connects as a superuser.
const AuthenticatedRole = "ayb_authenticated"

// escapeLiteral escapes a string for safe use as a SQL string literal
// by doubling single quotes.
func escapeLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// rlsStatements returns the SET LOCAL SQL statements that SetRLSContext
// executes. Always includes role, user_id, and email. Conditionally
// includes tenant_id when claims.TenantID is non-empty. Extracted so
// tests can verify SQL generation without a live database connection.
func rlsStatements(claims *Claims) []string {
	stmts := []string{
		"SET LOCAL ROLE " + sqlutil.QuoteIdent(AuthenticatedRole),
		"SET LOCAL ayb.user_id = '" + escapeLiteral(claims.Subject) + "'",
		"SET LOCAL ayb.user_email = '" + escapeLiteral(claims.Email) + "'",
	}
	tenantID := strings.TrimSpace(claims.TenantID)
	if tenantID != "" {
		stmts = append(stmts, "SET LOCAL ayb.tenant_id = '"+escapeLiteral(tenantID)+"'")
	}
	return stmts
}

// SetRLSContext switches to the authenticated role and sets Postgres session
// variables for RLS policies within the given transaction. Uses SET LOCAL
// and set_config(..., true), both scoped to the current transaction.
//
// Users write standard RLS policies referencing these variables:
//
//	CREATE POLICY user_owns_row ON posts
//	    USING (author_id::text = current_setting('ayb.user_id', true));
func SetRLSContext(ctx context.Context, tx pgx.Tx, claims *Claims) error {
	if claims == nil {
		return nil
	}

	// Use SET LOCAL instead of SELECT set_config() to avoid leaving unread
	// result rows on the pgx connection, which causes "conn busy" on commit.
	for _, stmt := range rlsStatements(claims) {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("rls context: %w", err)
		}
	}

	return nil
}
