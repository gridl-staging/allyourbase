package realtime

import "testing"

// ---------------------------------------------------------------------------
// parseValue
// ---------------------------------------------------------------------------

func TestParseValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  interface{}
	}{
		// Null.
		{"null", nil},

		// Booleans.
		{"true", true},
		{"false", false},

		// Integers.
		{"0", int64(0)},
		{"42", int64(42)},
		{"-7", int64(-7)},
		{"9999999999", int64(9999999999)},

		// Floats — only when not parseable as int.
		{"3.14", float64(3.14)},
		{"-0.5", float64(-0.5)},
		{"1e10", float64(1e10)}, // scientific notation is float, not int

		// Strings — anything that doesn't match the above.
		{"hello", "hello"},
		{"", ""},
		{"True", "True"},   // case-sensitive — only lowercase "true" is bool
		{"FALSE", "FALSE"}, // case-sensitive
		{"NULL", "NULL"},   // case-sensitive — only lowercase "null"
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := parseValue(tc.input)
			if got != tc.want {
				t.Errorf("parseValue(%q) = %v (%T), want %v (%T)",
					tc.input, got, got, tc.want, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// compareValuesStrict
// ---------------------------------------------------------------------------

func TestCompareValuesStrict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		a, b   interface{}
		want   int
		wantOK bool
	}{
		// Nil comparisons.
		{"nil == nil", nil, nil, 0, true},
		{"nil < non-nil", nil, int64(1), -1, true},
		{"non-nil > nil", int64(1), nil, 1, true},

		// Integer comparisons.
		{"int64 equal", int64(5), int64(5), 0, true},
		{"int64 less", int64(3), int64(7), -1, true},
		{"int64 greater", int64(9), int64(2), 1, true},

		// Cross-type numeric comparisons (int vs int64).
		{"int vs int64", int(5), int64(5), 0, true},
		{"int64 vs int", int64(3), int(7), -1, true},

		// Float comparisons.
		{"float64 equal", float64(3.14), float64(3.14), 0, true},
		{"float64 less", float64(1.0), float64(2.0), -1, true},

		// Cross-type numeric (int vs float64).
		{"int vs float64", int(5), float64(5.0), 0, true},
		{"int64 vs float64", int64(3), float64(4.0), -1, true},

		// String comparisons (lexicographic).
		{"string equal", "abc", "abc", 0, true},
		{"string less", "abc", "def", -1, true},
		{"string greater", "xyz", "abc", 1, true},

		// Boolean comparisons (false < true).
		{"bool equal true", true, true, 0, true},
		{"bool equal false", false, false, 0, true},
		{"false < true", false, true, -1, true},
		{"true > false", true, false, 1, true},

		// Incompatible types return (0, false).
		{"string vs int", "abc", int64(1), 0, false},
		{"bool vs int", true, int64(1), 0, false},
		{"int vs string", int(5), "5", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmp, ok := compareValuesStrict(tc.a, tc.b)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantOK && cmp != tc.want {
				t.Errorf("compareValuesStrict(%v, %v) = %d, want %d", tc.a, tc.b, cmp, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Comparison helpers (thin wrappers around compareValuesStrict)
// ---------------------------------------------------------------------------

func TestCompareGreater(t *testing.T) {
	t.Parallel()

	if !compareGreater(int64(5), int64(3)) {
		t.Error("5 > 3 should be true")
	}
	if compareGreater(int64(3), int64(5)) {
		t.Error("3 > 5 should be false")
	}
	if compareGreater(int64(5), int64(5)) {
		t.Error("5 > 5 should be false")
	}
	// Incompatible types return false (not ok).
	if compareGreater("abc", int64(1)) {
		t.Error("string > int should be false")
	}
}

func TestCompareGreaterOrEqual(t *testing.T) {
	t.Parallel()

	if !compareGreaterOrEqual(int64(5), int64(5)) {
		t.Error("5 >= 5 should be true")
	}
	if !compareGreaterOrEqual(int64(6), int64(5)) {
		t.Error("6 >= 5 should be true")
	}
	if compareGreaterOrEqual(int64(4), int64(5)) {
		t.Error("4 >= 5 should be false")
	}
}

func TestCompareLess(t *testing.T) {
	t.Parallel()

	if !compareLess(int64(3), int64(5)) {
		t.Error("3 < 5 should be true")
	}
	if compareLess(int64(5), int64(3)) {
		t.Error("5 < 3 should be false")
	}
	if compareLess(int64(5), int64(5)) {
		t.Error("5 < 5 should be false")
	}
}

func TestCompareLessOrEqual(t *testing.T) {
	t.Parallel()

	if !compareLessOrEqual(int64(5), int64(5)) {
		t.Error("5 <= 5 should be true")
	}
	if !compareLessOrEqual(int64(4), int64(5)) {
		t.Error("4 <= 5 should be true")
	}
	if compareLessOrEqual(int64(6), int64(5)) {
		t.Error("6 <= 5 should be false")
	}
}

// ---------------------------------------------------------------------------
// matchIn
// ---------------------------------------------------------------------------

func TestMatchIn(t *testing.T) {
	t.Parallel()

	t.Run("string value matches in list", func(t *testing.T) {
		t.Parallel()
		// matchIn expects operand to be []string.
		if !matchIn("hello", []string{"hello", "world"}) {
			t.Error("expected match")
		}
	})

	t.Run("string value not in list", func(t *testing.T) {
		t.Parallel()
		if matchIn("missing", []string{"hello", "world"}) {
			t.Error("expected no match")
		}
	})

	t.Run("int value matches parsed numeric string", func(t *testing.T) {
		t.Parallel()
		// The list contains "42" which parseValue turns into int64(42).
		if !matchIn(int64(42), []string{"10", "42", "99"}) {
			t.Error("expected int64(42) to match '42' in list")
		}
	})

	t.Run("nil matches 'null' in list", func(t *testing.T) {
		t.Parallel()
		if !matchIn(nil, []string{"null", "other"}) {
			t.Error("expected nil to match 'null' in list")
		}
	})

	t.Run("non-slice operand returns false", func(t *testing.T) {
		t.Parallel()
		// matchIn requires []string operand — anything else returns false.
		if matchIn("value", "not-a-slice") {
			t.Error("expected false for non-slice operand")
		}
	})

	t.Run("empty list returns false", func(t *testing.T) {
		t.Parallel()
		if matchIn("value", []string{}) {
			t.Error("expected false for empty list")
		}
	})
}

// ---------------------------------------------------------------------------
// MatchResult constants — guard against accidental renames
// ---------------------------------------------------------------------------

func TestMatchResultConstants(t *testing.T) {
	t.Parallel()

	// These are used in switch statements throughout the realtime package.
	if MatchNone != 0 {
		t.Errorf("MatchNone = %d, want 0", MatchNone)
	}
	if MatchOld != 1 {
		t.Errorf("MatchOld = %d, want 1", MatchOld)
	}
	if MatchNew != 2 {
		t.Errorf("MatchNew = %d, want 2", MatchNew)
	}
	if MatchBoth != 3 {
		t.Errorf("MatchBoth = %d, want 3", MatchBoth)
	}
}
