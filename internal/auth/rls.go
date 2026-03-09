package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// quoteIdent quotes a SQL identifier with double quotes for safe use in queries.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// AuthenticatedRole is the Postgres role used for authenticated API requests.
// SET LOCAL ROLE switches to this role within each request transaction so
// RLS policies are enforced even when the pool connects as a superuser.
const AuthenticatedRole = "ayb_authenticated"

// escapeLiteral escapes a string for safe use as a SQL string literal
// by doubling single quotes.
func escapeLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// rlsStatements returns the three SET LOCAL SQL statements that
// SetRLSContext executes. Extracted so tests can verify SQL generation
// without requiring a live database connection.
func rlsStatements(claims *Claims) (roleSQL, userIDSQL, emailSQL string) {
	roleSQL = "SET LOCAL ROLE " + quoteIdent(AuthenticatedRole)
	userIDSQL = "SET LOCAL ayb.user_id = '" + escapeLiteral(claims.Subject) + "'"
	emailSQL = "SET LOCAL ayb.user_email = '" + escapeLiteral(claims.Email) + "'"
	return
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

	roleSQL, userIDSQL, emailSQL := rlsStatements(claims)

	// Switch to the authenticated role so RLS policies are enforced.
	if _, err := tx.Exec(ctx, roleSQL); err != nil {
		return fmt.Errorf("setting role: %w", err)
	}

	// Use SET LOCAL instead of SELECT set_config() to avoid leaving unread
	// result rows on the pgx connection, which causes "conn busy" on commit.
	if _, err := tx.Exec(ctx, userIDSQL); err != nil {
		return fmt.Errorf("setting ayb.user_id: %w", err)
	}

	if _, err := tx.Exec(ctx, emailSQL); err != nil {
		return fmt.Errorf("setting ayb.user_email: %w", err)
	}

	return nil
}
