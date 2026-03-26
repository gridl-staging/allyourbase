// Package edgefunc Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/edgefunc/query_executor_postgres_rawsql.go.
package edgefunc

import (
	"fmt"
	"strings"
)

type sqlTableReferenceOptions struct {
	allowTableColumnList bool
	allowOnly            bool
	allowLateral         bool
	rejectDerivedSources bool
}

type sqlTableReferenceRule struct {
	firstKeyword  string
	secondKeyword string
	options       sqlTableReferenceOptions
}

var sqlTableReferenceRules = []sqlTableReferenceRule{
	{
		firstKeyword: "from",
		options:      sqlTableReferenceOptions{allowOnly: true, allowLateral: true, rejectDerivedSources: true},
	},
	{
		firstKeyword: "join",
		options:      sqlTableReferenceOptions{allowOnly: true, allowLateral: true, rejectDerivedSources: true},
	},
	{
		firstKeyword: "using",
		options:      sqlTableReferenceOptions{allowOnly: true, allowLateral: true, rejectDerivedSources: true},
	},
	{
		firstKeyword: "update",
		options:      sqlTableReferenceOptions{allowOnly: true},
	},
	{
		firstKeyword:  "insert",
		secondKeyword: "into",
		options:       sqlTableReferenceOptions{allowTableColumnList: true},
	},
	{
		firstKeyword:  "delete",
		secondKeyword: "from",
		options:       sqlTableReferenceOptions{allowOnly: true},
	},
}

// TODO: Document extractReferencedTablesFromSQL.
func extractReferencedTablesFromSQL(sql string) ([]string, error) {
	lowerSQL := strings.ToLower(sql)
	tables := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)

	for idx := 0; idx < len(sql); {
		switch {
		case isSQLWhitespace(sql[idx]):
			idx++
		case sql[idx] == '\'':
			idx = skipSingleQuotedSQLString(sql, idx)
		case sql[idx] == '"':
			idx = skipDoubleQuotedSQLIdentifier(sql, idx)
		case hasSQLPrefix(sql, idx, "--"):
			idx = skipSQLLineComment(sql, idx)
		case hasSQLPrefix(sql, idx, "/*"):
			idx = skipSQLBlockComment(sql, idx)
		case sql[idx] == ';':
			nextIdx := skipSQLWhitespaceAndComments(sql, idx+1)
			if nextIdx < len(sql) {
				return nil, fmt.Errorf("raw SQL allowlist does not support multiple statements")
			}
			idx = nextIdx
		default:
			if dollarQuoteTag, ok := readDollarQuoteTag(sql, idx); ok {
				idx = skipSQLDollarQuotedString(sql, idx, dollarQuoteTag)
				continue
			}

			if sqlKeywordAt(lowerSQL, idx, "with") {
				return nil, fmt.Errorf("raw SQL allowlist does not support WITH clauses")
			}

			tableName, nextIdx, err := scanSQLTableReference(sql, lowerSQL, idx)
			if err != nil {
				return nil, err
			}
			if tableName != "" {
				tables = appendUniqueSQLTable(tables, seen, tableName)
			}
			if nextIdx > idx {
				idx = nextIdx
				continue
			}

			idx++
		}
	}

	if len(tables) == 0 {
		return nil, fmt.Errorf("unable to determine target table for raw SQL allowlist")
	}
	return tables, nil
}

// TODO: Document scanSQLTableReference.
func scanSQLTableReference(sql, lowerSQL string, idx int) (string, int, error) {
	for _, rule := range sqlTableReferenceRules {
		var (
			tableName string
			nextIdx   int
			ok        bool
			err       error
		)

		if rule.secondKeyword == "" {
			tableName, nextIdx, ok, err = extractTableAfterKeyword(sql, lowerSQL, idx, rule.firstKeyword, rule.options)
		} else {
			tableName, nextIdx, ok, err = extractTableAfterCompoundKeyword(
				sql,
				lowerSQL,
				idx,
				rule.firstKeyword,
				rule.secondKeyword,
				rule.options,
			)
		}
		if err != nil {
			return "", 0, err
		}
		if nextIdx <= idx {
			continue
		}
		if ok {
			return tableName, nextIdx, nil
		}
		return "", nextIdx, nil
	}
	return "", idx, nil
}

// TODO: Document extractTableAfterKeyword.
func extractTableAfterKeyword(sql, lowerSQL string, idx int, keyword string, opts sqlTableReferenceOptions) (string, int, bool, error) {
	if !sqlKeywordAt(lowerSQL, idx, keyword) {
		return "", idx, false, nil
	}
	nextIdx := skipSQLWhitespaceAndComments(sql, idx+len(keyword))

	if opts.allowLateral && sqlKeywordAt(lowerSQL, nextIdx, "lateral") {
		nextIdx = skipSQLWhitespaceAndComments(sql, nextIdx+len("lateral"))
	}
	if opts.allowOnly && sqlKeywordAt(lowerSQL, nextIdx, "only") {
		nextIdx = skipSQLWhitespaceAndComments(sql, nextIdx+len("only"))
	}
	if nextIdx < len(sql) && sql[nextIdx] == '(' {
		if opts.rejectDerivedSources {
			return "", nextIdx, false, fmt.Errorf("raw SQL allowlist does not support derived table sources")
		}
		return "", nextIdx + 1, false, nil
	}

	tableName, afterTable, ok := readQualifiedSQLIdentifier(sql, nextIdx)
	if ok {
		if opts.rejectDerivedSources {
			afterTableIdx := skipSQLWhitespaceAndComments(sql, afterTable)
			if afterTableIdx < len(sql) && sql[afterTableIdx] == '(' {
				return "", afterTableIdx, false, fmt.Errorf("raw SQL allowlist does not support function table sources")
			}
		}
		return tableName, afterTable, true, nil
	}
	return "", nextIdx, false, nil
}

func extractTableAfterCompoundKeyword(sql, lowerSQL string, idx int, firstKeyword, secondKeyword string, opts sqlTableReferenceOptions) (string, int, bool, error) {
	if !sqlKeywordAt(lowerSQL, idx, firstKeyword) {
		return "", idx, false, nil
	}
	nextIdx := skipSQLWhitespaceAndComments(sql, idx+len(firstKeyword))
	if !sqlKeywordAt(lowerSQL, nextIdx, secondKeyword) {
		return "", nextIdx, false, nil
	}
	return extractTableAfterKeyword(sql, lowerSQL, nextIdx, secondKeyword, opts)
}

func sqlKeywordAt(lowerSQL string, idx int, keyword string) bool {
	if idx < 0 || idx+len(keyword) > len(lowerSQL) || !strings.HasPrefix(lowerSQL[idx:], keyword) {
		return false
	}
	if idx > 0 && isSQLIdentifierChar(lowerSQL[idx-1]) {
		return false
	}
	endIdx := idx + len(keyword)
	return endIdx == len(lowerSQL) || !isSQLIdentifierChar(lowerSQL[endIdx])
}

func readQualifiedSQLIdentifier(sql string, idx int) (string, int, bool) {
	firstPart, nextIdx, ok := readSQLIdentifierPart(sql, idx)
	if !ok {
		return "", idx, false
	}
	if nextIdx >= len(sql) || sql[nextIdx] != '.' {
		return firstPart, nextIdx, true
	}

	secondPart, endIdx, ok := readSQLIdentifierPart(sql, nextIdx+1)
	if !ok {
		return "", idx, false
	}
	return firstPart + "." + secondPart, endIdx, true
}

// TODO: Document readSQLIdentifierPart.
func readSQLIdentifierPart(sql string, idx int) (string, int, bool) {
	if idx >= len(sql) {
		return "", idx, false
	}
	if sql[idx] == '"' {
		var builder strings.Builder
		for pos := idx + 1; pos < len(sql); pos++ {
			if sql[pos] != '"' {
				builder.WriteByte(sql[pos])
				continue
			}
			if pos+1 < len(sql) && sql[pos+1] == '"' {
				builder.WriteByte('"')
				pos++
				continue
			}
			return builder.String(), pos + 1, true
		}
		return "", len(sql), false
	}
	if !isSQLIdentifierStart(sql[idx]) {
		return "", idx, false
	}
	endIdx := idx + 1
	for endIdx < len(sql) && isSQLIdentifierChar(sql[endIdx]) {
		endIdx++
	}
	return sql[idx:endIdx], endIdx, true
}

func skipSQLWhitespaceAndComments(sql string, idx int) int {
	for idx < len(sql) {
		switch {
		case isSQLWhitespace(sql[idx]):
			idx++
		case hasSQLPrefix(sql, idx, "--"):
			idx = skipSQLLineComment(sql, idx)
		case hasSQLPrefix(sql, idx, "/*"):
			idx = skipSQLBlockComment(sql, idx)
		default:
			return idx
		}
	}
	return idx
}

func skipSingleQuotedSQLString(sql string, idx int) int {
	for idx++; idx < len(sql); idx++ {
		if sql[idx] != '\'' {
			continue
		}
		if idx+1 < len(sql) && sql[idx+1] == '\'' {
			idx++
			continue
		}
		return idx + 1
	}
	return len(sql)
}

func skipDoubleQuotedSQLIdentifier(sql string, idx int) int {
	for idx++; idx < len(sql); idx++ {
		if sql[idx] != '"' {
			continue
		}
		if idx+1 < len(sql) && sql[idx+1] == '"' {
			idx++
			continue
		}
		return idx + 1
	}
	return len(sql)
}

func skipSQLLineComment(sql string, idx int) int {
	for idx += 2; idx < len(sql); idx++ {
		if sql[idx] == '\n' {
			return idx + 1
		}
	}
	return len(sql)
}

// TODO: Document skipSQLBlockComment.
func skipSQLBlockComment(sql string, idx int) int {
	depth := 1
	for pos := idx + 2; pos < len(sql); pos++ {
		if pos+1 >= len(sql) {
			continue
		}
		switch {
		case sql[pos] == '/' && sql[pos+1] == '*':
			depth++
			pos++
		case sql[pos] == '*' && sql[pos+1] == '/':
			depth--
			pos++
			if depth == 0 {
				return pos + 1
			}
		}
	}
	return len(sql)
}

// TODO: Document readDollarQuoteTag.
func readDollarQuoteTag(sql string, idx int) (string, bool) {
	if idx >= len(sql) || sql[idx] != '$' {
		return "", false
	}
	endIdx := idx + 1
	for endIdx < len(sql) && sql[endIdx] != '$' {
		if !isDollarQuoteTagChar(sql[endIdx], endIdx == idx+1) {
			return "", false
		}
		endIdx++
	}
	if endIdx >= len(sql) || sql[endIdx] != '$' {
		return "", false
	}
	return sql[idx : endIdx+1], true
}

func skipSQLDollarQuotedString(sql string, idx int, opener string) int {
	searchStart := idx + len(opener)
	closingIdx := strings.Index(sql[searchStart:], opener)
	if closingIdx == -1 {
		return len(sql)
	}
	return searchStart + closingIdx + len(opener)
}

func hasSQLPrefix(sql string, idx int, prefix string) bool {
	return idx >= 0 && idx+len(prefix) <= len(sql) && sql[idx:idx+len(prefix)] == prefix
}

func isSQLIdentifierStart(ch byte) bool {
	return ch == '_' || ch == '$' || ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z')
}

func isSQLIdentifierChar(ch byte) bool {
	return isSQLIdentifierStart(ch) || ('0' <= ch && ch <= '9')
}

func isDollarQuoteTagChar(ch byte, first bool) bool {
	if first {
		return ch == '_' || ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z')
	}
	return isSQLIdentifierChar(ch)
}

func appendUniqueSQLTable(tables []string, seen map[string]struct{}, tableName string) []string {
	if _, ok := seen[tableName]; ok {
		return tables
	}
	seen[tableName] = struct{}{}
	return append(tables, tableName)
}

func isSQLWhitespace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
}
