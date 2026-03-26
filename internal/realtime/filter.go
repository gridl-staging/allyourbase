// Package realtime Filter provides parsing and evaluation of column-level subscription filters for realtime events.
package realtime

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Column-level subscription filters allow clients to narrow already-authorized
// realtime events based on column values. Filters are applied AFTER RLS checks
// (RLS is the security boundary; filters are convenience narrowing only).
//
// # Canonical Filter Input Format
//
// Single filter:  column=operator.value
// Multiple filters: column1=op1.val1,column2=op2.val2 (AND semantics)
//
// Supported operators:
//   - eq  : equals (string, number, boolean, null)
//   - neq : not equals
//   - gt  : greater than (numeric)
//   - gte : greater than or equal (numeric)
//   - lt  : less than (numeric)
//   - lte : less than or equal (numeric)
//   - in  : value in list (pipe-separated: status=in.pending|active|review)
//
// Value formats:
//   - Strings: passed as-is after the dot (e.g., status=eq.pending)
//   - Numbers: parsed as int64 or float64 (e.g., age=gt.18, price=lt.99.99)
//   - Booleans: "true" or "false" (e.g., active=eq.true)
//   - Null: the literal "null" (e.g., deleted_at=eq.null)
//
// Examples:
//
//	status=eq.pending
//	priority=gt.5
//	status=in.pending|active|review
//	status=eq.pending,priority=gte.5
//
// # Filter Evaluation Semantics
//
// INSERT events: Evaluate new row only. Deliver if matches.
// UPDATE events: Evaluate both old and new rows.
//   - Old matches, new doesn't → "left filter" (deliver as UPDATE for old row)
//   - Old doesn't match, new does → "entered filter" (deliver as UPDATE for new row)
//   - Both match → deliver as UPDATE
//   - Neither matches → don't deliver
//
// DELETE events: Evaluate old row only. Deliver if matches.
//
// # Edge Cases
//
// Missing column: Filter doesn't match (no error, just false).
// Null values: Compared as-is (null == null is true).
// Type mismatches: Int/float64 are coerced for comparison; other mismatches return false.
var (
	ErrInvalidFilterFormat = errors.New("invalid filter format")
	ErrUnknownOperator     = errors.New("unknown operator")
	ErrEmptyColumn         = errors.New("empty column name")
	ErrEmptyValue          = errors.New("empty filter value")
)

type Operator string

const (
	OpEq  Operator = "eq"
	OpNeq Operator = "neq"
	OpGt  Operator = "gt"
	OpGte Operator = "gte"
	OpLt  Operator = "lt"
	OpLte Operator = "lte"
	OpIn  Operator = "in"
)

type Filter struct {
	Column   string
	Operator Operator
	Operand  interface{}
}

type Filters []Filter

type ParseError struct {
	Input string
	Err   error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("filter parse error: %s: %v", e.Input, e.Err)
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

// ParseFilters parses a comma-separated string of filters. Returns nil for empty input or a ParseError if any filter is malformed.
func ParseFilters(input string) (Filters, error) {
	if input == "" {
		return nil, nil
	}

	var filters Filters
	parts := splitFilters(input)
	for _, part := range parts {
		f, err := parseFilter(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		filters = append(filters, f)
	}
	return filters, nil
}

// splitFilters splits a filter string by commas while respecting parenthesis depth to keep pipe-separated values intact.
func splitFilters(input string) []string {
	var result []string
	var current strings.Builder
	depth := 0

	for i := 0; i < len(input); i++ {
		ch := input[i]
		if ch == '(' {
			depth++
			current.WriteByte(ch)
		} else if ch == ')' {
			depth--
			current.WriteByte(ch)
		} else if ch == ',' && depth == 0 {
			result = append(result, current.String())
			current.Reset()
		} else {
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// parseFilter parses a single filter string in format 'column=operator.value'. Returns a ParseError if the format is invalid.
func parseFilter(input string) (Filter, error) {
	eqIdx := strings.Index(input, "=")
	if eqIdx == -1 {
		return Filter{}, &ParseError{Input: input, Err: ErrInvalidFilterFormat}
	}

	column := strings.TrimSpace(input[:eqIdx])
	if column == "" {
		return Filter{}, &ParseError{Input: input, Err: ErrEmptyColumn}
	}

	right := strings.TrimSpace(input[eqIdx+1:])
	if right == "" {
		return Filter{}, &ParseError{Input: input, Err: ErrEmptyValue}
	}

	dotIdx := strings.Index(right, ".")
	if dotIdx == -1 {
		return Filter{}, &ParseError{Input: input, Err: ErrInvalidFilterFormat}
	}

	opStr := strings.TrimSpace(right[:dotIdx])
	valueStr := right[dotIdx+1:]

	operator := Operator(opStr)
	switch operator {
	case OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpIn:
	default:
		return Filter{}, &ParseError{Input: input, Err: ErrUnknownOperator}
	}

	if valueStr == "" {
		return Filter{}, &ParseError{Input: input, Err: ErrEmptyValue}
	}

	var operand interface{}
	if operator == OpIn {
		operand = strings.Split(valueStr, "|")
	} else {
		operand = parseValue(valueStr)
	}

	return Filter{
		Column:   column,
		Operator: operator,
		Operand:  operand,
	}, nil
}

// parseValue converts a string to its appropriate type: nil for 'null', bool for 'true'/'false', int64 or float64 for numbers, or string otherwise.
func parseValue(s string) interface{} {
	if s == "null" {
		return nil
	}

	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}

	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}

	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}

	return s
}

type MatchResult int

const (
	MatchNone MatchResult = iota
	MatchOld
	MatchNew
	MatchBoth
)

// Matches evaluates the filters against old and new rows, returning which rows matched.
func (fs Filters) Matches(oldRow, newRow map[string]interface{}) MatchResult {
	if len(fs) == 0 {
		return MatchBoth
	}

	oldMatch := fs.matchRow(oldRow)
	newMatch := fs.matchRow(newRow)

	switch {
	case oldMatch && newMatch:
		return MatchBoth
	case oldMatch && !newMatch:
		return MatchOld
	case !oldMatch && newMatch:
		return MatchNew
	default:
		return MatchNone
	}
}

func (fs Filters) matchRow(row map[string]interface{}) bool {
	if row == nil {
		return false
	}

	for _, f := range fs {
		if !f.matchRow(row) {
			return false
		}
	}
	return true
}

func (f Filter) matchRow(row map[string]interface{}) bool {
	val, exists := row[f.Column]
	if !exists {
		return false
	}

	return f.matchValue(val)
}

// matchValue reports whether val matches the filter's operator and operand.
func (f Filter) matchValue(val interface{}) bool {
	switch f.Operator {
	case OpEq:
		return compareEqual(val, f.Operand)
	case OpNeq:
		return !compareEqual(val, f.Operand)
	case OpGt:
		return compareGreater(val, f.Operand)
	case OpGte:
		return compareGreaterOrEqual(val, f.Operand)
	case OpLt:
		return compareLess(val, f.Operand)
	case OpLte:
		return compareLessOrEqual(val, f.Operand)
	case OpIn:
		return matchIn(val, f.Operand)
	}
	return false
}

// compareEqual reports whether a and b are equal, performing numeric type coercion and treating nil values as equal to each other.
func compareEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	switch va := a.(type) {
	case int:
		switch vb := b.(type) {
		case int:
			return va == vb
		case int64:
			return int64(va) == vb
		case float64:
			return float64(va) == vb
		}
	case int64:
		switch vb := b.(type) {
		case int:
			return va == int64(vb)
		case int64:
			return va == vb
		case float64:
			return float64(va) == vb
		}
	case float64:
		switch vb := b.(type) {
		case int:
			return va == float64(vb)
		case int64:
			return va == float64(vb)
		case float64:
			return va == vb
		}
	case string:
		if vb, ok := b.(string); ok {
			return va == vb
		}
	case bool:
		if vb, ok := b.(bool); ok {
			return va == vb
		}
	}

	return false
}

func compareGreater(a, b interface{}) bool {
	cmp, ok := compareValuesStrict(a, b)
	return ok && cmp > 0
}

func compareGreaterOrEqual(a, b interface{}) bool {
	cmp, ok := compareValuesStrict(a, b)
	return ok && cmp >= 0
}

func compareLess(a, b interface{}) bool {
	cmp, ok := compareValuesStrict(a, b)
	return ok && cmp < 0
}

func compareLessOrEqual(a, b interface{}) bool {
	cmp, ok := compareValuesStrict(a, b)
	return ok && cmp <= 0
}

func compareValues(a, b interface{}) int {
	cmp, _ := compareValuesStrict(a, b)
	return cmp
}

// compareValuesStrict compares a and b, returning an ordering (-1, 0, or 1) and a bool indicating whether the comparison is valid for the given types.
func compareValuesStrict(a, b interface{}) (int, bool) {
	if a == nil && b == nil {
		return 0, true
	}
	if a == nil {
		return -1, true
	}
	if b == nil {
		return 1, true
	}

	switch va := a.(type) {
	case int:
		switch vb := b.(type) {
		case int:
			if va < vb {
				return -1, true
			} else if va > vb {
				return 1, true
			}
			return 0, true
		case int64:
			return compareInt64(int64(va), vb), true
		case float64:
			return compareFloat64(float64(va), vb), true
		}
	case int64:
		switch vb := b.(type) {
		case int:
			return compareInt64(va, int64(vb)), true
		case int64:
			return compareInt64(va, vb), true
		case float64:
			return compareFloat64(float64(va), vb), true
		}
	case float64:
		switch vb := b.(type) {
		case int:
			return compareFloat64(va, float64(vb)), true
		case int64:
			return compareFloat64(va, float64(vb)), true
		case float64:
			return compareFloat64(va, vb), true
		}
	case string:
		if vb, ok := b.(string); ok {
			if va < vb {
				return -1, true
			} else if va > vb {
				return 1, true
			}
			return 0, true
		}
	case bool:
		if vb, ok := b.(bool); ok {
			if !va && vb {
				return -1, true
			} else if va && !vb {
				return 1, true
			}
			return 0, true
		}
	}

	return 0, false
}

func compareInt64(a, b int64) int {
	if a < b {
		return -1
	} else if a > b {
		return 1
	}
	return 0
}

func compareFloat64(a, b float64) int {
	if a < b {
		return -1
	} else if a > b {
		return 1
	}
	return 0
}

func matchIn(val interface{}, operand interface{}) bool {
	values, ok := operand.([]string)
	if !ok {
		return false
	}

	for _, v := range values {
		if compareEqual(val, parseValue(v)) {
			return true
		}
	}
	return false
}

func ShouldDeliver(action string, match MatchResult) bool {
	switch action {
	case "create", "insert":
		return match == MatchNew || match == MatchBoth
	case "update":
		return match == MatchOld || match == MatchNew || match == MatchBoth
	case "delete":
		return match == MatchOld || match == MatchBoth
	}
	return false
}
