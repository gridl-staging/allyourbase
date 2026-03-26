package migrate

import (
	"strings"

	"github.com/allyourbase/ayb/internal/sqlutil"
)

// QuoteIdent returns a safely quoted SQL identifier.
// Delegates to sqlutil.QuoteIdent — kept for API compatibility with external importers.
func QuoteIdent(s string) string {
	return sqlutil.QuoteIdent(s)
}

// QuoteQualifiedTable returns a quoted schema.table reference.
// Defaults empty schema to "public" (migrate-specific behavior).
func QuoteQualifiedTable(schema, table string) string {
	if schema == "" {
		schema = "public"
	}
	return sqlutil.QuoteQualifiedName(schema, table)
}

// SanitizeIdentifier converts arbitrary input into a safe SQL identifier token.
func SanitizeIdentifier(s string) string {
	s = strings.ToLower(s)
	b := strings.Builder{}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('_')
	}
	if b.Len() == 0 {
		return "id"
	}
	return b.String()
}

// BuildAllowPolicySQL builds a permissive role policy statement for standard CRUD actions.
func BuildAllowPolicySQL(schema, table, role, action string) string {
	policyName := SanitizeIdentifier(strings.ToLower(table + "_" + role + "_" + action))
	if action == "INSERT" {
		return "CREATE POLICY " + QuoteIdent(policyName) + " ON " + QuoteQualifiedTable(schema, table) +
			" FOR INSERT TO " + QuoteIdent(role) + " WITH CHECK (true)"
	}
	return "CREATE POLICY " + QuoteIdent(policyName) + " ON " + QuoteQualifiedTable(schema, table) +
		" FOR " + action + " TO " + QuoteIdent(role) + " USING (true)"
}

// SQLString returns a safely escaped SQL single-quoted literal.
func SQLString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
