package realtime

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestParseFilters(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      Filters
		wantErr   bool
		errSubstr string
	}{
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "single eq filter",
			input: "status=eq.pending",
			want: Filters{
				{Column: "status", Operator: OpEq, Operand: "pending"},
			},
		},
		{
			name:  "single neq filter",
			input: "status=neq.done",
			want: Filters{
				{Column: "status", Operator: OpNeq, Operand: "done"},
			},
		},
		{
			name:  "single gt filter",
			input: "age=gt.18",
			want: Filters{
				{Column: "age", Operator: OpGt, Operand: int64(18)},
			},
		},
		{
			name:  "single gte filter",
			input: "score=gte.100",
			want: Filters{
				{Column: "score", Operator: OpGte, Operand: int64(100)},
			},
		},
		{
			name:  "single lt filter",
			input: "price=lt.50.5",
			want: Filters{
				{Column: "price", Operator: OpLt, Operand: 50.5},
			},
		},
		{
			name:  "single lte filter",
			input: "count=lte.0",
			want: Filters{
				{Column: "count", Operator: OpLte, Operand: int64(0)},
			},
		},
		{
			name:  "single in filter",
			input: "status=in.pending|active|review",
			want: Filters{
				{Column: "status", Operator: OpIn, Operand: []string{"pending", "active", "review"}},
			},
		},
		{
			name:  "in filter single value",
			input: "status=in.pending",
			want: Filters{
				{Column: "status", Operator: OpIn, Operand: []string{"pending"}},
			},
		},
		{
			name:  "boolean true",
			input: "active=eq.true",
			want: Filters{
				{Column: "active", Operator: OpEq, Operand: true},
			},
		},
		{
			name:  "boolean false",
			input: "deleted=eq.false",
			want: Filters{
				{Column: "deleted", Operator: OpEq, Operand: false},
			},
		},
		{
			name:  "null value",
			input: "deleted_at=eq.null",
			want: Filters{
				{Column: "deleted_at", Operator: OpEq, Operand: nil},
			},
		},
		{
			name:  "string with spaces",
			input: "name=eq.John Doe",
			want: Filters{
				{Column: "name", Operator: OpEq, Operand: "John Doe"},
			},
		},
		{
			name:  "multiple filters with AND",
			input: "status=eq.pending,priority=gt.5",
			want: Filters{
				{Column: "status", Operator: OpEq, Operand: "pending"},
				{Column: "priority", Operator: OpGt, Operand: int64(5)},
			},
		},
		{
			name:  "multiple filters with in operator",
			input: "status=in.pending|active,priority=gte.5",
			want: Filters{
				{Column: "status", Operator: OpIn, Operand: []string{"pending", "active"}},
				{Column: "priority", Operator: OpGte, Operand: int64(5)},
			},
		},
		{
			name:      "missing operator",
			input:     "status.pending",
			wantErr:   true,
			errSubstr: "invalid filter format",
		},
		{
			name:      "unknown operator",
			input:     "status=like.pending",
			wantErr:   true,
			errSubstr: "unknown operator",
		},
		{
			name:      "empty column",
			input:     "=eq.pending",
			wantErr:   true,
			errSubstr: "empty column name",
		},
		{
			name:      "empty value",
			input:     "status=eq.",
			wantErr:   true,
			errSubstr: "empty filter value",
		},
		{
			name:      "missing equals",
			input:     "statuseq.pending",
			wantErr:   true,
			errSubstr: "invalid filter format",
		},
		{
			name:      "missing dot after operator",
			input:     "status=eqpending",
			wantErr:   true,
			errSubstr: "invalid filter format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFilters(tt.input)
			if tt.wantErr {
				testutil.Error(t, err)
				if tt.errSubstr != "" {
					testutil.True(t, err.Error() != "", "error should have message")
					testutil.Contains(t, err.Error(), tt.errSubstr)
				}
				return
			}
			testutil.NoError(t, err)
			testutil.Equal(t, len(tt.want), len(got))
			for i := range tt.want {
				testutil.Equal(t, tt.want[i].Column, got[i].Column)
				testutil.Equal(t, tt.want[i].Operator, got[i].Operator)
				switch want := tt.want[i].Operand.(type) {
				case []string:
					got, ok := got[i].Operand.([]string)
					testutil.True(t, ok, "operand should be []string")
					testutil.Equal(t, len(want), len(got))
					for j := range want {
						testutil.Equal(t, want[j], got[j])
					}
				default:
					testutil.Equal(t, tt.want[i].Operand, got[i].Operand)
				}
			}
		})
	}
}

func TestFilterMatches(t *testing.T) {
	tests := []struct {
		name    string
		filters Filters
		oldRow  map[string]interface{}
		newRow  map[string]interface{}
		want    MatchResult
	}{
		{
			name:    "no filters matches both",
			filters: nil,
			oldRow:  map[string]interface{}{"id": 1},
			newRow:  map[string]interface{}{"id": 1},
			want:    MatchBoth,
		},
		{
			name:    "empty filters matches both",
			filters: Filters{},
			oldRow:  map[string]interface{}{"id": 1},
			newRow:  map[string]interface{}{"id": 1},
			want:    MatchBoth,
		},
		{
			name:    "nil rows no match",
			filters: Filters{{Column: "status", Operator: OpEq, Operand: "pending"}},
			oldRow:  nil,
			newRow:  nil,
			want:    MatchNone,
		},
		{
			name:    "eq filter match new only",
			filters: Filters{{Column: "status", Operator: OpEq, Operand: "pending"}},
			oldRow:  map[string]interface{}{"id": 1, "status": "done"},
			newRow:  map[string]interface{}{"id": 1, "status": "pending"},
			want:    MatchNew,
		},
		{
			name:    "eq filter match old only",
			filters: Filters{{Column: "status", Operator: OpEq, Operand: "pending"}},
			oldRow:  map[string]interface{}{"id": 1, "status": "pending"},
			newRow:  map[string]interface{}{"id": 1, "status": "done"},
			want:    MatchOld,
		},
		{
			name:    "eq filter match both",
			filters: Filters{{Column: "status", Operator: OpEq, Operand: "pending"}},
			oldRow:  map[string]interface{}{"id": 1, "status": "pending"},
			newRow:  map[string]interface{}{"id": 1, "status": "pending"},
			want:    MatchBoth,
		},
		{
			name:    "eq filter match none",
			filters: Filters{{Column: "status", Operator: OpEq, Operand: "pending"}},
			oldRow:  map[string]interface{}{"id": 1, "status": "done"},
			newRow:  map[string]interface{}{"id": 1, "status": "done"},
			want:    MatchNone,
		},
		{
			name:    "missing column no match",
			filters: Filters{{Column: "status", Operator: OpEq, Operand: "pending"}},
			oldRow:  map[string]interface{}{"id": 1},
			newRow:  map[string]interface{}{"id": 1},
			want:    MatchNone,
		},
		{
			name:    "in filter matches",
			filters: Filters{{Column: "status", Operator: OpIn, Operand: []string{"pending", "active"}}},
			oldRow:  map[string]interface{}{"id": 1, "status": "pending"},
			newRow:  map[string]interface{}{"id": 1, "status": "active"},
			want:    MatchBoth,
		},
		{
			name:    "in filter partial match",
			filters: Filters{{Column: "status", Operator: OpIn, Operand: []string{"pending", "active"}}},
			oldRow:  map[string]interface{}{"id": 1, "status": "pending"},
			newRow:  map[string]interface{}{"id": 1, "status": "done"},
			want:    MatchOld,
		},
		{
			name:    "multiple filters AND semantics",
			filters: Filters{{Column: "status", Operator: OpEq, Operand: "pending"}, {Column: "priority", Operator: OpGt, Operand: int64(5)}},
			oldRow:  map[string]interface{}{"id": 1, "status": "pending", "priority": 6},
			newRow:  map[string]interface{}{"id": 1, "status": "pending", "priority": 7},
			want:    MatchBoth,
		},
		{
			name:    "multiple filters AND fails on one",
			filters: Filters{{Column: "status", Operator: OpEq, Operand: "pending"}, {Column: "priority", Operator: OpGt, Operand: int64(5)}},
			oldRow:  map[string]interface{}{"id": 1, "status": "pending", "priority": 3},
			newRow:  map[string]interface{}{"id": 1, "status": "done", "priority": 7},
			want:    MatchNone,
		},
		{
			name:    "null comparison",
			filters: Filters{{Column: "deleted_at", Operator: OpEq, Operand: nil}},
			oldRow:  map[string]interface{}{"id": 1, "deleted_at": nil},
			newRow:  map[string]interface{}{"id": 1, "deleted_at": "2024-01-01"},
			want:    MatchOld,
		},
		{
			name:    "neq operator",
			filters: Filters{{Column: "status", Operator: OpNeq, Operand: "done"}},
			oldRow:  map[string]interface{}{"id": 1, "status": "pending"},
			newRow:  map[string]interface{}{"id": 1, "status": "done"},
			want:    MatchOld,
		},
		{
			name:    "comparison operators",
			filters: Filters{{Column: "age", Operator: OpGte, Operand: int64(18)}},
			oldRow:  map[string]interface{}{"id": 1, "age": 17},
			newRow:  map[string]interface{}{"id": 1, "age": 18},
			want:    MatchNew,
		},
		{
			name:    "gte type mismatch does not match",
			filters: Filters{{Column: "age", Operator: OpGte, Operand: int64(18)}},
			oldRow:  map[string]interface{}{"id": 1, "age": "17"},
			newRow:  map[string]interface{}{"id": 1, "age": "18"},
			want:    MatchNone,
		},
		{
			name:    "lte type mismatch does not match",
			filters: Filters{{Column: "age", Operator: OpLte, Operand: int64(18)}},
			oldRow:  map[string]interface{}{"id": 1, "age": "17"},
			newRow:  map[string]interface{}{"id": 1, "age": "18"},
			want:    MatchNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filters.Matches(tt.oldRow, tt.newRow)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestCompareEqual(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		{"both nil", nil, nil, true},
		{"a nil", nil, "x", false},
		{"b nil", "x", nil, false},
		{"int int", 5, 5, true},
		{"int int64", 5, int64(5), true},
		{"int64 int", int64(5), 5, true},
		{"int64 int64", int64(5), int64(5), true},
		{"float64 int", 5.0, 5, true},
		{"int float64", 5, 5.0, true},
		{"string string", "hello", "hello", true},
		{"bool bool", true, true, true},
		{"mixed types", 5, "5", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareEqual(tt.a, tt.b)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestCompareValues(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want int
	}{
		{"both nil", nil, nil, 0},
		{"a nil", nil, 1, -1},
		{"b nil", 1, nil, 1},
		{"int less", 3, 5, -1},
		{"int equal", 5, 5, 0},
		{"int greater", 5, 3, 1},
		{"string less", "a", "b", -1},
		{"string equal", "x", "x", 0},
		{"string greater", "b", "a", 1},
		{"bool false true", false, true, -1},
		{"bool equal", true, true, 0},
		{"bool true false", true, false, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareValues(tt.a, tt.b)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestShouldDeliver(t *testing.T) {
	tests := []struct {
		name   string
		action string
		match  MatchResult
		want   bool
	}{
		{"insert match new", "insert", MatchNew, true},
		{"insert match both", "insert", MatchBoth, true},
		{"insert match old", "insert", MatchOld, false},
		{"insert match none", "insert", MatchNone, false},
		{"insert create variant", "create", MatchNew, true},
		{"update match old", "update", MatchOld, true},
		{"update match new", "update", MatchNew, true},
		{"update match both", "update", MatchBoth, true},
		{"update match none", "update", MatchNone, false},
		{"delete match old", "delete", MatchOld, true},
		{"delete match both", "delete", MatchBoth, true},
		{"delete match new", "delete", MatchNew, false},
		{"delete match none", "delete", MatchNone, false},
		{"unknown action", "unknown", MatchBoth, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldDeliver(tt.action, tt.match)
			testutil.Equal(t, tt.want, got)
		})
	}
}
