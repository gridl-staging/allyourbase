// Package graphql Provides utilities for evaluating GraphQL where clauses against row data, supporting logical operators and SQL-like comparison operations.
package graphql

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
)

// matchesGraphQLWhere evaluates whether a row matches a GraphQL where clause, supporting logical operators (_and, _or, _not) and column-level comparison operations.
func matchesGraphQLWhere(where map[string]any, row map[string]any) bool {
	if len(where) == 0 {
		return true
	}
	if row == nil {
		return false
	}

	for key, raw := range where {
		switch key {
		case "_and":
			items, ok := raw.([]any)
			if !ok {
				return false
			}
			for _, item := range items {
				itemMap, ok := item.(map[string]any)
				if !ok || !matchesGraphQLWhere(itemMap, row) {
					return false
				}
			}
		case "_or":
			items, ok := raw.([]any)
			if !ok {
				return false
			}
			matched := false
			for _, item := range items {
				itemMap, ok := item.(map[string]any)
				if ok && matchesGraphQLWhere(itemMap, row) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "_not":
			itemMap, ok := raw.(map[string]any)
			if !ok || matchesGraphQLWhere(itemMap, row) {
				return false
			}
		default:
			ops, ok := raw.(map[string]any)
			if !ok {
				return false
			}
			rowValue, exists := row[key]
			if !exists {
				return false
			}
			if !matchesGraphQLColumnOps(rowValue, ops) {
				return false
			}
		}
	}
	return true
}

// matchesGraphQLColumnOps evaluates whether a value matches all specified GraphQL column operations such as _eq, _neq, _gt, _in, and _like.
func matchesGraphQLColumnOps(value any, ops map[string]any) bool {
	for op, operand := range ops {
		switch op {
		case "_eq":
			if !valuesEqual(value, operand) {
				return false
			}
		case "_neq":
			if valuesEqual(value, operand) {
				return false
			}
		case "_gt":
			if !compareNumeric(value, operand, func(a, b float64) bool { return a > b }) {
				return false
			}
		case "_gte":
			if !compareNumeric(value, operand, func(a, b float64) bool { return a >= b }) {
				return false
			}
		case "_lt":
			if !compareNumeric(value, operand, func(a, b float64) bool { return a < b }) {
				return false
			}
		case "_lte":
			if !compareNumeric(value, operand, func(a, b float64) bool { return a <= b }) {
				return false
			}
		case "_is_null":
			want, _ := operand.(bool)
			if (value == nil) != want {
				return false
			}
		case "_in":
			if !isInList(value, operand, true) {
				return false
			}
		case "_nin":
			if !isInList(value, operand, false) {
				return false
			}
		case "_like":
			if !matchesLike(value, operand, false) {
				return false
			}
		case "_ilike":
			if !matchesLike(value, operand, true) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func matchesLike(value any, operand any, caseInsensitive bool) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	pattern, ok := operand.(string)
	if !ok {
		return false
	}
	return sqlLikeMatch(text, pattern, caseInsensitive)
}

// sqlLikeMatch matches a value against a SQL LIKE pattern by converting percent to .* and underscore to . for regex matching, with optional case-insensitive comparison.
func sqlLikeMatch(value, pattern string, caseInsensitive bool) bool {
	var b strings.Builder
	if caseInsensitive {
		b.WriteString("(?i)")
	}
	b.WriteByte('^')

	escaped := false
	for _, r := range pattern {
		if escaped {
			b.WriteString(regexp.QuoteMeta(string(r)))
			escaped = false
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteByte('.')
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	if escaped {
		b.WriteString(`\\`)
	}
	b.WriteByte('$')

	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(value)
}

func compareNumeric(a, b any, fn func(float64, float64) bool) bool {
	left, ok := toFloat64(a)
	if !ok {
		return false
	}
	right, ok := toFloat64(b)
	if !ok {
		return false
	}
	return fn(left, right)
}

// toFloat64 converts a value to float64 if it is a numeric type, supporting int, int32, int64, float32, float64, and json.Number.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// isInList checks whether a value appears in a list, returning true if found when wantIn is true, or if not found when wantIn is false.
func isInList(value any, operand any, wantIn bool) bool {
	values, ok := operand.([]any)
	if !ok {
		return false
	}
	found := false
	for _, item := range values {
		if valuesEqual(value, item) {
			found = true
			break
		}
	}
	if wantIn {
		return found
	}
	return !found
}

func valuesEqual(a, b any) bool {
	left, okLeft := toFloat64(a)
	right, okRight := toFloat64(b)
	if okLeft && okRight {
		return left == right
	}
	return reflect.DeepEqual(a, b)
}
