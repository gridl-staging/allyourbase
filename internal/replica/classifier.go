// Package replica Classifies SQL queries and removes leading comments to enable routing decisions between primary and replica database connections.
package replica

import (
	"strings"
	"unicode"
)

type QueryType int

const (
	QueryRead QueryType = iota
	QueryWrite
	QueryTransaction
)

// ClassifyQuery determines whether a SQL query is a read, write, or transaction query by examining its keywords after removing leading comments.
func ClassifyQuery(sql string) QueryType {
	cleaned := stripLeadingSQLComments(sql)
	if cleaned == "" {
		return QueryWrite
	}

	upper := strings.ToUpper(cleaned)

	switch {
	case hasKeywordPrefix(upper, "SELECT"),
		hasKeywordPrefix(upper, "WITH"),
		hasKeywordPrefix(upper, "EXPLAIN"),
		hasKeywordPrefix(upper, "SHOW"):
		return QueryRead
	case hasKeywordPrefix(upper, "INSERT"),
		hasKeywordPrefix(upper, "UPDATE"),
		hasKeywordPrefix(upper, "DELETE"),
		hasKeywordPrefix(upper, "CREATE"),
		hasKeywordPrefix(upper, "ALTER"),
		hasKeywordPrefix(upper, "DROP"),
		hasKeywordPrefix(upper, "TRUNCATE"),
		hasKeywordPrefix(upper, "COPY"):
		return QueryWrite
	case hasKeywordPrefix(upper, "BEGIN"),
		hasKeywordPrefix(upper, "COMMIT"),
		hasKeywordPrefix(upper, "ROLLBACK"),
		hasKeywordPrefix(upper, "SAVEPOINT"),
		hasKeywordPrefix(upper, "RELEASE"),
		hasKeywordPrefix(upper, "SET"):
		return QueryTransaction
	default:
		return QueryWrite
	}
}

// stripLeadingSQLComments removes leading whitespace and SQL comments from a query string, handling both line comments (--) and nested block comments (/* */).
func stripLeadingSQLComments(sql string) string {
	trimmed := strings.TrimLeftFunc(sql, unicode.IsSpace)

	for {
		switch {
		case strings.HasPrefix(trimmed, "--"):
			commentEnd := findLineCommentEnd(trimmed)
			if commentEnd == -1 {
				return ""
			}
			trimmed = trimmed[commentEnd:]
		case strings.HasPrefix(trimmed, "/*"):
			commentEnd := findBlockCommentEnd(trimmed)
			if commentEnd == -1 {
				return ""
			}
			trimmed = trimmed[commentEnd:]
		default:
			return trimmed
		}

		trimmed = strings.TrimLeftFunc(trimmed, unicode.IsSpace)
	}
}

func findLineCommentEnd(query string) int {
	for i := 2; i < len(query); i++ {
		if query[i] == '\n' {
			return i + 1
		}
		if query[i] == '\r' {
			if i+1 < len(query) && query[i+1] == '\n' {
				return i + 2
			}
			return i + 1
		}
	}
	return -1
}

// findBlockCommentEnd returns the byte position immediately after a block comment's closing delimiter, accounting for nesting, or -1 if no valid closing is found.
func findBlockCommentEnd(query string) int {
	depth := 0

	for i := 0; i+1 < len(query); {
		switch {
		case query[i] == '/' && query[i+1] == '*':
			depth++
			i += 2
		case query[i] == '*' && query[i+1] == '/':
			depth--
			i += 2
			if depth == 0 {
				return i
			}
		default:
			i++
		}
	}

	return -1
}

func hasKeywordPrefix(query string, keyword string) bool {
	if !strings.HasPrefix(query, keyword) {
		return false
	}
	if len(query) == len(keyword) {
		return true
	}

	next := query[len(keyword)]
	return !isIdentifierChar(next)
}

func isIdentifierChar(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_'
}
