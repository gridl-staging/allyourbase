// Package sqlutil provides SQL identifier quoting utilities.
//
// This is the single source of truth for quoting PostgreSQL identifiers.
// All packages that need to quote SQL identifiers should import this package
// rather than implementing their own quoting logic.
package sqlutil

import "strings"

// QuoteIdent returns a safely quoted PostgreSQL identifier.
// It wraps the identifier in double quotes, escapes embedded double quotes
// by doubling them, and strips null bytes (which PostgreSQL rejects).
func QuoteIdent(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, c := range s {
		if c == 0 {
			continue // strip null bytes — PostgreSQL rejects them
		}
		if c == '"' {
			b.WriteString(`""`)
		} else {
			b.WriteRune(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// QuoteIdentList quotes each identifier and joins them with ", ".
func QuoteIdentList(idents []string) string {
	quoted := make([]string, len(idents))
	for i, id := range idents {
		quoted[i] = QuoteIdent(id)
	}
	return strings.Join(quoted, ", ")
}

// QuoteQualifiedName returns a quoted "schema"."name" reference.
// Callers must supply both parts explicitly — no implicit defaults are applied.
func QuoteQualifiedName(schema, name string) string {
	return QuoteIdent(schema) + "." + QuoteIdent(name)
}
