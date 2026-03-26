// Package nhostmigrate Splits and parses SQL statements to extract schema metadata including table definitions and foreign key relationships.
package nhostmigrate

import (
	"regexp"
	"strings"
)

// splitSQLStatements splits SQL text into individual statements, properly handling single-quoted strings, double-quoted identifiers, and PostgreSQL dollar-quoted strings.
func splitSQLStatements(sqlText string) []string {
	statements := make([]string, 0)
	var b strings.Builder

	inSingle := false
	inDouble := false
	dollarTag := ""

	for i := 0; i < len(sqlText); i++ {
		ch := sqlText[i]

		if dollarTag == "" && !inSingle && !inDouble && ch == '$' {
			if tag, ok := parseDollarTag(sqlText[i:]); ok {
				dollarTag = tag
				b.WriteString(tag)
				i += len(tag) - 1
				continue
			}
		}

		if dollarTag != "" {
			if strings.HasPrefix(sqlText[i:], dollarTag) {
				b.WriteString(dollarTag)
				i += len(dollarTag) - 1
				dollarTag = ""
				continue
			}
			b.WriteByte(ch)
			continue
		}

		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			b.WriteByte(ch)
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			b.WriteByte(ch)
			continue
		}

		if ch == ';' && !inSingle && !inDouble {
			stmt := strings.TrimSpace(b.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			b.Reset()
			continue
		}

		b.WriteByte(ch)
	}

	if trailing := strings.TrimSpace(b.String()); trailing != "" {
		statements = append(statements, trailing)
	}

	return statements
}

func parseDollarTag(s string) (string, bool) {
	if len(s) < 2 || s[0] != '$' {
		return "", false
	}
	for i := 1; i < len(s); i++ {
		if s[i] == '$' {
			return s[:i+1], true
		}
		if !(s[i] == '_' || s[i] >= 'a' && s[i] <= 'z' || s[i] >= 'A' && s[i] <= 'Z' || s[i] >= '0' && s[i] <= '9') {
			return "", false
		}
	}
	return "", false
}

// classifySQLStatement returns the statement kind (create_table, create_view, create_index, insert, or foreign_key) by examining the SQL, or empty string if unrecognized.
func classifySQLStatement(stmt string) string {
	normalized := strings.ToUpper(strings.TrimSpace(stmt))
	switch {
	case strings.HasPrefix(normalized, "CREATE TABLE"):
		return "create_table"
	case strings.HasPrefix(normalized, "CREATE VIEW"), strings.HasPrefix(normalized, "CREATE MATERIALIZED VIEW"):
		return "create_view"
	case strings.HasPrefix(normalized, "CREATE INDEX"), strings.HasPrefix(normalized, "CREATE UNIQUE INDEX"):
		return "create_index"
	case strings.HasPrefix(normalized, "INSERT INTO"):
		return "insert"
	case strings.HasPrefix(normalized, "ALTER TABLE") && strings.Contains(normalized, "ADD CONSTRAINT") && strings.Contains(normalized, "FOREIGN KEY"):
		return "foreign_key"
	default:
		return ""
	}
}

var (
	createTableRe = regexp.MustCompile(`(?i)^CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([^\s(]+)`)
	createIndexRe = regexp.MustCompile(`(?i)^CREATE\s+(?:UNIQUE\s+)?INDEX\s+[^\s]+\s+ON\s+([^\s(]+)`)
	insertIntoRe  = regexp.MustCompile(`(?i)^INSERT\s+INTO\s+([^\s(]+)`)
	alterTableRe  = regexp.MustCompile(`(?i)^ALTER\s+TABLE\s+(?:ONLY\s+)?([^\s]+)`)
)

func shouldSkipStatement(stmt string) bool {
	tableRef := ""
	for _, re := range []*regexp.Regexp{createTableRe, createIndexRe, insertIntoRe, alterTableRe} {
		if match := re.FindStringSubmatch(strings.TrimSpace(stmt)); len(match) > 1 {
			tableRef = match[1]
			break
		}
	}
	if tableRef == "" {
		return false
	}

	schema, name := splitQualifiedTable(tableRef)
	return shouldSkipQualifiedTable(schema, name)
}

// parseForeignKeyStatement parses an ALTER TABLE ADD CONSTRAINT FOREIGN KEY statement and returns a metadataForeignKey with the child and parent table references, along with a success indicator.
func parseForeignKeyStatement(stmt string) (metadataForeignKey, bool) {
	normalized := strings.ReplaceAll(stmt, "\n", " ")
	normalized = strings.ReplaceAll(normalized, "\t", " ")

	childMatch := alterTableRe.FindStringSubmatch(strings.TrimSpace(normalized))
	if len(childMatch) < 2 {
		return metadataForeignKey{}, false
	}
	childSchema, childTable := splitQualifiedTable(childMatch[1])
	childCol := extractFirstParenValue(normalized, "FOREIGN KEY")

	parentRef := extractAfterKeyword(normalized, "REFERENCES")
	if parentRef == "" {
		return metadataForeignKey{}, false
	}
	parentSchema, parentTable := splitQualifiedTable(parentRef)
	parentCol := extractFirstParenValue(parentRef, "")
	if childCol == "" || parentCol == "" {
		return metadataForeignKey{}, false
	}
	return metadataForeignKey{
		FromSchema: defaultSchema(childSchema),
		FromTable:  childTable,
		FromColumn: childCol,
		ToSchema:   defaultSchema(parentSchema),
		ToTable:    parentTable,
		ToColumn:   parentCol,
	}, true
}

// extractFirstParenValue returns the content of the first parenthesized value in a string, optionally searching for an anchor keyword first.
func extractFirstParenValue(s, anchor string) string {
	start := 0
	if anchor != "" {
		idx := strings.Index(strings.ToUpper(s), strings.ToUpper(anchor))
		if idx < 0 {
			return ""
		}
		start = idx
	}
	open := strings.Index(s[start:], "(")
	if open < 0 {
		return ""
	}
	open += start
	close := strings.Index(s[open:], ")")
	if close < 0 {
		return ""
	}
	close += open
	return strings.Trim(strings.TrimSpace(s[open+1:close]), `"`)
}

func extractAfterKeyword(s, keyword string) string {
	idx := strings.Index(strings.ToUpper(s), strings.ToUpper(keyword))
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(s[idx+len(keyword):])
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimSuffix(fields[0], ";")
}

// countInsertRows counts the number of rows being inserted in an INSERT statement by counting top-level parentheses in the VALUES clause, returning 1 if no VALUES clause is found.
func countInsertRows(stmt string) int {
	upper := strings.ToUpper(stmt)
	idx := strings.Index(upper, "VALUES")
	if idx < 0 {
		return 1
	}
	payload := stmt[idx+len("VALUES"):]
	rows := 0
	inSingle := false
	depth := 0
	for i := 0; i < len(payload); i++ {
		ch := payload[i]
		if ch == '\'' {
			inSingle = !inSingle
			continue
		}
		if inSingle {
			continue
		}
		if ch == '(' {
			if depth == 0 {
				rows++
			}
			depth++
		} else if ch == ')' && depth > 0 {
			depth--
		}
	}
	if rows == 0 {
		return 1
	}
	return rows
}
