// Package pbmigrate Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar22_pm_03_pbmigrate_hardening/allyourbase_dev/internal/pbmigrate/typemap.go.
package pbmigrate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/allyourbase/ayb/internal/sqlutil"
)

var createIndexStatementPattern = regexp.MustCompile(`(?is)^CREATE\s+(UNIQUE\s+)?INDEX\s+(IF\s+NOT\s+EXISTS\s+)?(.+?)\s+ON\s+(.+?)\s*\((.+)\)\s*;?$`)
var unquotedIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// FieldTypeToPgType converts a PocketBase field type to PostgreSQL column type
func FieldTypeToPgType(field PBField) string {
	switch field.Type {
	case "text", "email", "url", "editor":
		return "TEXT"

	case "number":
		return "DOUBLE PRECISION"

	case "bool":
		return "BOOLEAN"

	case "date":
		return "TIMESTAMP WITH TIME ZONE"

	case "select":
		// Check if maxSelect > 1 (multiple selection)
		if maxSelect := fieldMaxSelect(field); maxSelect > 1 {
			return "TEXT[]" // array for multiple select
		}
		return "TEXT" // single select

	case "json":
		return "JSONB"

	case "file":
		// Check if maxSelect > 1 (multiple files)
		if maxSelect := fieldMaxSelect(field); maxSelect > 1 {
			return "TEXT[]" // array of filenames
		}
		return "TEXT" // single filename

	case "relation":
		// Check if maxSelect > 1 (multiple relations)
		if maxSelect := fieldMaxSelect(field); maxSelect > 1 {
			return "TEXT[]" // array of IDs
		}
		return "TEXT" // single ID

	default:
		return "TEXT" // fallback to text for unknown types
	}
}

func fieldMaxSelect(field PBField) float64 {
	if field.MaxSelect > 0 {
		return field.MaxSelect
	}
	if field.Options == nil {
		return 0
	}
	if maxSelect, ok := field.Options["maxSelect"].(float64); ok {
		return maxSelect
	}
	return 0
}

// BuildCreateTableSQL generates CREATE TABLE statement for a collection
func BuildCreateTableSQL(coll PBCollection) string {
	tableName := SanitizeIdentifier(coll.Name)

	sql := fmt.Sprintf("CREATE TABLE %s (\n", tableName)

	// System fields (always present)
	sql += "  id TEXT PRIMARY KEY,\n"
	sql += "  created TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),\n"
	sql += "  updated TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()"

	// Add custom fields
	for _, field := range coll.Schema {
		if field.System {
			continue // skip system fields
		}

		pgType := FieldTypeToPgType(field)
		fieldName := SanitizeIdentifier(field.Name)

		sql += fmt.Sprintf(",\n  %s %s", fieldName, pgType)

		if field.Required {
			sql += " NOT NULL"
		}

		if field.Unique {
			sql += " UNIQUE"
		}
	}

	sql += "\n);"

	return sql
}

// BuildCreateIndexSQL generates PostgreSQL CREATE INDEX statements from a
// collection's PocketBase SQLite index definitions.
func BuildCreateIndexSQL(coll PBCollection) ([]string, error) {
	if len(coll.Indexes) == 0 {
		return nil, nil
	}

	translated := make([]string, 0, len(coll.Indexes))
	for _, sqliteStmt := range coll.Indexes {
		pgStmt, err := translateSQLiteCreateIndexStatement(sqliteStmt)
		if err != nil {
			return nil, fmt.Errorf("unsupported SQLite index definition for %s: %q: %w", coll.Name, sqliteStmt, err)
		}
		translated = append(translated, pgStmt)
	}

	return translated, nil
}

// TODO: Document translateSQLiteCreateIndexStatement.
func translateSQLiteCreateIndexStatement(sqliteStmt string) (string, error) {
	matches := createIndexStatementPattern.FindStringSubmatch(strings.TrimSpace(sqliteStmt))
	if len(matches) == 0 {
		return "", fmt.Errorf("expected CREATE INDEX syntax")
	}

	indexName, err := quoteSQLiteIdentifierToken(matches[3])
	if err != nil {
		return "", fmt.Errorf("invalid index name: %w", err)
	}

	tableName, err := quoteSQLiteIdentifierToken(matches[4])
	if err != nil {
		return "", fmt.Errorf("invalid table name: %w", err)
	}

	columns, err := translateSQLiteIndexColumns(matches[5])
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	builder.WriteString("CREATE ")
	if strings.TrimSpace(matches[1]) != "" {
		builder.WriteString("UNIQUE ")
	}
	builder.WriteString("INDEX ")
	if strings.TrimSpace(matches[2]) != "" {
		builder.WriteString("IF NOT EXISTS ")
	}
	builder.WriteString(indexName)
	builder.WriteString(" ON ")
	builder.WriteString(tableName)
	builder.WriteString(" (")
	builder.WriteString(columns)
	builder.WriteString(");")

	return builder.String(), nil
}

// TODO: Document translateSQLiteIndexColumns.
func translateSQLiteIndexColumns(columnsSpec string) (string, error) {
	rawColumns := strings.Split(columnsSpec, ",")
	if len(rawColumns) == 0 {
		return "", fmt.Errorf("index has no columns")
	}

	translatedColumns := make([]string, 0, len(rawColumns))
	for _, rawColumn := range rawColumns {
		columnSpec := strings.TrimSpace(rawColumn)
		if columnSpec == "" {
			return "", fmt.Errorf("index contains an empty column entry")
		}
		if strings.ContainsAny(columnSpec, "()") {
			return "", fmt.Errorf("expressions are not supported")
		}

		tokens := strings.Fields(columnSpec)
		if len(tokens) < 1 || len(tokens) > 2 {
			return "", fmt.Errorf("column definition %q is not supported", columnSpec)
		}

		columnName, err := quoteSQLiteIdentifierToken(tokens[0])
		if err != nil {
			return "", fmt.Errorf("invalid column name %q: %w", tokens[0], err)
		}

		columnSQL := columnName
		if len(tokens) == 2 {
			direction := strings.ToUpper(tokens[1])
			if direction != "ASC" && direction != "DESC" {
				return "", fmt.Errorf("column modifier %q is not supported", tokens[1])
			}
			columnSQL += " " + direction
		}

		translatedColumns = append(translatedColumns, columnSQL)
	}

	return strings.Join(translatedColumns, ", "), nil
}

// TODO: Document quoteSQLiteIdentifierToken.
func quoteSQLiteIdentifierToken(token string) (string, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return "", fmt.Errorf("identifier is empty")
	}
	if strings.Contains(trimmed, ".") {
		return "", fmt.Errorf("qualified identifiers are not supported")
	}

	if strings.HasPrefix(trimmed, "`") || strings.HasPrefix(trimmed, `"`) {
		if len(trimmed) < 2 {
			return "", fmt.Errorf("identifier quote is not closed")
		}
		quote := trimmed[:1]
		if !strings.HasSuffix(trimmed, quote) {
			return "", fmt.Errorf("identifier quote is not closed")
		}

		unquoted := trimmed[1 : len(trimmed)-1]
		if quote == `"` {
			unquoted = strings.ReplaceAll(unquoted, `""`, `"`)
		}
		return sqlutil.QuoteIdent(unquoted), nil
	}

	if !unquotedIdentifierPattern.MatchString(trimmed) {
		return "", fmt.Errorf("identifier %q must be quoted", trimmed)
	}

	return sqlutil.QuoteIdent(trimmed), nil
}

// BuildCreateViewSQL generates CREATE VIEW statement for a view collection.
// The source view query must remain a single SELECT/WITH statement so a tampered
// PocketBase export cannot inject extra SQL into the migration transaction.
func BuildCreateViewSQL(coll PBCollection) (string, error) {
	viewName := SanitizeIdentifier(coll.Name)
	query, err := normalizeSingleStatementViewQuery(coll.ViewQuery)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("CREATE VIEW %s AS %s;", viewName, query), nil
}

func normalizeSingleStatementViewQuery(query string) (string, error) {
	normalized, err := normalizeEmbeddedSQLFragment(query, true)
	if err != nil {
		return "", fmt.Errorf("unsafe view query: %w", err)
	}
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return "", fmt.Errorf("unsafe view query: SQL fragment is empty")
	}
	first := strings.ToUpper(fields[0])
	if first != "SELECT" && first != "WITH" {
		return "", fmt.Errorf("unsafe view query: must start with SELECT or WITH")
	}
	return normalized, nil
}

func validateEmbeddedSQLExpression(expression string) error {
	if _, err := normalizeEmbeddedSQLFragment(expression, false); err != nil {
		return fmt.Errorf("unsafe SQL expression: %w", err)
	}
	return nil
}

// TODO: Document normalizeEmbeddedSQLFragment.
func normalizeEmbeddedSQLFragment(fragment string, allowTrailingSemicolon bool) (string, error) {
	trimmed := strings.TrimSpace(fragment)
	if trimmed == "" {
		return "", fmt.Errorf("SQL fragment is empty")
	}

	trailingSemicolon := -1
	inSingleQuote := false
	inDoubleQuote := false

	for i := 0; i < len(trimmed); i++ {
		switch trimmed[i] {
		case '\'':
			if inDoubleQuote {
				continue
			}
			if inSingleQuote && i+1 < len(trimmed) && trimmed[i+1] == '\'' {
				i++
				continue
			}
			inSingleQuote = !inSingleQuote
		case '"':
			if inSingleQuote {
				continue
			}
			if inDoubleQuote && i+1 < len(trimmed) && trimmed[i+1] == '"' {
				i++
				continue
			}
			inDoubleQuote = !inDoubleQuote
		default:
			if inSingleQuote || inDoubleQuote {
				continue
			}
			if i+1 < len(trimmed) {
				switch trimmed[i : i+2] {
				case "--", "/*", "*/":
					return "", fmt.Errorf("SQL comments are not allowed")
				}
			}
			if trimmed[i] == ';' {
				if !allowTrailingSemicolon || strings.TrimSpace(trimmed[i+1:]) != "" {
					return "", fmt.Errorf("multiple SQL statements are not allowed")
				}
				trailingSemicolon = i
			}
		}
	}

	if inSingleQuote || inDoubleQuote {
		return "", fmt.Errorf("unterminated quoted SQL fragment")
	}
	if trailingSemicolon >= 0 {
		trimmed = strings.TrimSpace(trimmed[:trailingSemicolon])
	}
	return trimmed, nil
}

// SanitizeIdentifier ensures a PostgreSQL identifier is safe by quoting it
// and escaping any embedded double quotes (per SQL standard: " becomes "").
//
// Deprecated: use sqlutil.QuoteIdent directly for new code.
func SanitizeIdentifier(name string) string {
	return sqlutil.QuoteIdent(name)
}

// pgReservedWords is a simplified set of common PostgreSQL reserved words.
var pgReservedWords = map[string]bool{
	"user": true, "table": true, "select": true, "where": true,
	"from": true, "order": true, "group": true, "limit": true,
	"offset": true, "join": true, "union": true, "all": true,
}

// IsReservedWord checks if a name is a PostgreSQL reserved word
func IsReservedWord(name string) bool {
	return pgReservedWords[name]
}
