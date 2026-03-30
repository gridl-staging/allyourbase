package graphql

import (
	"encoding/json"
	"testing"
)

func TestMatchesGraphQLWhereEmpty(t *testing.T) {
	// Empty where matches everything.
	if !matchesGraphQLWhere(nil, map[string]any{"id": 1}) {
		t.Error("nil where should match any row")
	}
	if !matchesGraphQLWhere(map[string]any{}, map[string]any{"id": 1}) {
		t.Error("empty where should match any row")
	}
}

func TestMatchesGraphQLWhereNilRow(t *testing.T) {
	// Non-empty where against nil row should not match.
	if matchesGraphQLWhere(map[string]any{"id": map[string]any{"_eq": 1}}, nil) {
		t.Error("non-empty where should not match nil row")
	}
}

func TestMatchesGraphQLWhereEq(t *testing.T) {
	row := map[string]any{"name": "alice", "age": float64(30)}
	where := map[string]any{"name": map[string]any{"_eq": "alice"}}
	if !matchesGraphQLWhere(where, row) {
		t.Error("_eq should match equal string value")
	}

	where = map[string]any{"name": map[string]any{"_eq": "bob"}}
	if matchesGraphQLWhere(where, row) {
		t.Error("_eq should not match different string value")
	}
}

func TestMatchesGraphQLWhereNeq(t *testing.T) {
	row := map[string]any{"status": "active"}
	where := map[string]any{"status": map[string]any{"_neq": "deleted"}}
	if !matchesGraphQLWhere(where, row) {
		t.Error("_neq should match when values differ")
	}

	where = map[string]any{"status": map[string]any{"_neq": "active"}}
	if matchesGraphQLWhere(where, row) {
		t.Error("_neq should not match when values are equal")
	}
}

func TestMatchesGraphQLWhereNumericComparisons(t *testing.T) {
	row := map[string]any{"age": float64(25)}

	tests := []struct {
		name  string
		op    string
		value float64
		want  bool
	}{
		{"gt true", "_gt", 20, true},
		{"gt false", "_gt", 30, false},
		{"gt boundary", "_gt", 25, false},
		{"gte true", "_gte", 25, true},
		{"gte false", "_gte", 26, false},
		{"lt true", "_lt", 30, true},
		{"lt false", "_lt", 20, false},
		{"lt boundary", "_lt", 25, false},
		{"lte true", "_lte", 25, true},
		{"lte false", "_lte", 24, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			where := map[string]any{"age": map[string]any{tt.op: tt.value}}
			got := matchesGraphQLWhere(where, row)
			if got != tt.want {
				t.Errorf("where age %s %v: got %v, want %v", tt.op, tt.value, got, tt.want)
			}
		})
	}
}

func TestMatchesGraphQLWhereIsNull(t *testing.T) {
	row := map[string]any{"email": nil, "name": "alice"}

	where := map[string]any{"email": map[string]any{"_is_null": true}}
	if !matchesGraphQLWhere(where, row) {
		t.Error("_is_null=true should match nil value")
	}

	where = map[string]any{"name": map[string]any{"_is_null": true}}
	if matchesGraphQLWhere(where, row) {
		t.Error("_is_null=true should not match non-nil value")
	}

	where = map[string]any{"name": map[string]any{"_is_null": false}}
	if !matchesGraphQLWhere(where, row) {
		t.Error("_is_null=false should match non-nil value")
	}
}

func TestMatchesGraphQLWhereIn(t *testing.T) {
	row := map[string]any{"status": "active"}

	where := map[string]any{"status": map[string]any{"_in": []any{"active", "pending"}}}
	if !matchesGraphQLWhere(where, row) {
		t.Error("_in should match when value is in list")
	}

	where = map[string]any{"status": map[string]any{"_in": []any{"deleted", "suspended"}}}
	if matchesGraphQLWhere(where, row) {
		t.Error("_in should not match when value is not in list")
	}
}

func TestMatchesGraphQLWhereNin(t *testing.T) {
	row := map[string]any{"status": "active"}

	where := map[string]any{"status": map[string]any{"_nin": []any{"deleted", "suspended"}}}
	if !matchesGraphQLWhere(where, row) {
		t.Error("_nin should match when value is not in list")
	}

	where = map[string]any{"status": map[string]any{"_nin": []any{"active", "pending"}}}
	if matchesGraphQLWhere(where, row) {
		t.Error("_nin should not match when value is in list")
	}
}

func TestMatchesGraphQLWhereLike(t *testing.T) {
	row := map[string]any{"name": "Alice Smith"}

	tests := []struct {
		name    string
		pattern string
		want    bool
	}{
		{"prefix", "Alice%", true},
		{"suffix", "%Smith", true},
		{"contains", "%ice%", true},
		{"exact", "Alice Smith", true},
		{"no match", "Bob%", false},
		{"underscore wildcard", "Alice_Smith", true},
		{"underscore no match", "A_ce Smith", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			where := map[string]any{"name": map[string]any{"_like": tt.pattern}}
			got := matchesGraphQLWhere(where, row)
			if got != tt.want {
				t.Errorf("_like %q: got %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestMatchesGraphQLWhereIlike(t *testing.T) {
	row := map[string]any{"name": "Alice Smith"}

	where := map[string]any{"name": map[string]any{"_ilike": "alice%"}}
	if !matchesGraphQLWhere(where, row) {
		t.Error("_ilike should be case-insensitive")
	}

	where = map[string]any{"name": map[string]any{"_ilike": "ALICE%"}}
	if !matchesGraphQLWhere(where, row) {
		t.Error("_ilike should match uppercase pattern")
	}
}

func TestMatchesGraphQLWhereAnd(t *testing.T) {
	row := map[string]any{"name": "alice", "age": float64(30)}

	where := map[string]any{
		"_and": []any{
			map[string]any{"name": map[string]any{"_eq": "alice"}},
			map[string]any{"age": map[string]any{"_gt": float64(20)}},
		},
	}
	if !matchesGraphQLWhere(where, row) {
		t.Error("_and should match when all conditions are true")
	}

	where = map[string]any{
		"_and": []any{
			map[string]any{"name": map[string]any{"_eq": "alice"}},
			map[string]any{"age": map[string]any{"_gt": float64(40)}},
		},
	}
	if matchesGraphQLWhere(where, row) {
		t.Error("_and should not match when one condition is false")
	}
}

func TestMatchesGraphQLWhereOr(t *testing.T) {
	row := map[string]any{"name": "alice", "age": float64(30)}

	where := map[string]any{
		"_or": []any{
			map[string]any{"name": map[string]any{"_eq": "bob"}},
			map[string]any{"age": map[string]any{"_eq": float64(30)}},
		},
	}
	if !matchesGraphQLWhere(where, row) {
		t.Error("_or should match when at least one condition is true")
	}

	where = map[string]any{
		"_or": []any{
			map[string]any{"name": map[string]any{"_eq": "bob"}},
			map[string]any{"age": map[string]any{"_eq": float64(99)}},
		},
	}
	if matchesGraphQLWhere(where, row) {
		t.Error("_or should not match when all conditions are false")
	}
}

func TestMatchesGraphQLWhereNot(t *testing.T) {
	row := map[string]any{"status": "active"}

	where := map[string]any{
		"_not": map[string]any{"status": map[string]any{"_eq": "deleted"}},
	}
	if !matchesGraphQLWhere(where, row) {
		t.Error("_not should match when inner condition is false")
	}

	where = map[string]any{
		"_not": map[string]any{"status": map[string]any{"_eq": "active"}},
	}
	if matchesGraphQLWhere(where, row) {
		t.Error("_not should not match when inner condition is true")
	}
}

func TestMatchesGraphQLWhereMissingColumn(t *testing.T) {
	row := map[string]any{"name": "alice"}
	where := map[string]any{"age": map[string]any{"_eq": float64(30)}}
	if matchesGraphQLWhere(where, row) {
		t.Error("should not match when column does not exist in row")
	}
}

func TestMatchesGraphQLWhereUnknownOperator(t *testing.T) {
	row := map[string]any{"name": "alice"}
	where := map[string]any{"name": map[string]any{"_regex": "a.*"}}
	if matchesGraphQLWhere(where, row) {
		t.Error("unknown operator should not match")
	}
}

func TestMatchesGraphQLWhereMultipleColumnConditions(t *testing.T) {
	row := map[string]any{"name": "alice", "age": float64(30), "active": true}
	// Implicit AND across multiple top-level keys.
	where := map[string]any{
		"name":   map[string]any{"_eq": "alice"},
		"age":    map[string]any{"_gte": float64(18)},
		"active": map[string]any{"_eq": true},
	}
	if !matchesGraphQLWhere(where, row) {
		t.Error("multiple column conditions should be ANDed")
	}

	where = map[string]any{
		"name": map[string]any{"_eq": "alice"},
		"age":  map[string]any{"_gt": float64(50)}, // false
	}
	if matchesGraphQLWhere(where, row) {
		t.Error("should not match when one column condition fails")
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want float64
		ok   bool
	}{
		{"int", 42, 42.0, true},
		{"int32", int32(42), 42.0, true},
		{"int64", int64(42), 42.0, true},
		{"float32", float32(3.14), float64(float32(3.14)), true},
		{"float64", 3.14, 3.14, true},
		{"json.Number", json.Number("42.5"), 42.5, true},
		{"string", "42", 0, false},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toFloat64(tt.val)
			if ok != tt.ok {
				t.Errorf("toFloat64(%v) ok = %v, want %v", tt.val, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("toFloat64(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

func TestSqlLikeMatch(t *testing.T) {
	tests := []struct {
		name            string
		value           string
		pattern         string
		caseInsensitive bool
		want            bool
	}{
		{"exact match", "hello", "hello", false, true},
		{"percent prefix", "hello world", "hello%", false, true},
		{"percent suffix", "hello world", "%world", false, true},
		{"percent both", "hello world", "%lo wo%", false, true},
		{"underscore", "abc", "a_c", false, true},
		{"underscore no match", "abbc", "a_c", false, false},
		{"escaped percent", "100%", `100\%`, false, true},
		{"case sensitive miss", "Hello", "hello", false, false},
		{"case insensitive match", "Hello", "hello", true, true},
		{"empty pattern", "", "", false, true},
		{"percent empty", "", "%", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sqlLikeMatch(tt.value, tt.pattern, tt.caseInsensitive)
			if got != tt.want {
				t.Errorf("sqlLikeMatch(%q, %q, %v) = %v, want %v",
					tt.value, tt.pattern, tt.caseInsensitive, got, tt.want)
			}
		})
	}
}

func TestValuesEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b any
		want bool
	}{
		{"same string", "hello", "hello", true},
		{"diff string", "hello", "world", false},
		{"int and float64", 42, float64(42), true},
		{"diff numbers", 42, float64(43), false},
		{"nil nil", nil, nil, true},
		{"nil non-nil", nil, "hello", false},
		{"bool true", true, true, true},
		{"bool diff", true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := valuesEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("valuesEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
