package api

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func filterTestTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "users",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "name", Position: 2, TypeName: "text"},
			{Name: "email", Position: 3, TypeName: "varchar"},
			{Name: "age", Position: 4, TypeName: "integer"},
			{Name: "status", Position: 5, TypeName: "text"},
			{Name: "active", Position: 6, TypeName: "boolean"},
		},
		PrimaryKey: []string{"id"},
	}
}

// --- Tokenizer tests ---

func TestTokenizeSimple(t *testing.T) {
	t.Parallel()
	tokens, err := tokenize("name='Alice'")
	testutil.NoError(t, err)
	testutil.SliceLen(t, tokens, 3)
	testutil.Equal(t, tokIdent, tokens[0].kind)
	testutil.Equal(t, "name", tokens[0].value)
	testutil.Equal(t, tokOp, tokens[1].kind)
	testutil.Equal(t, "=", tokens[1].value)
	testutil.Equal(t, tokString, tokens[2].kind)
	testutil.Equal(t, "Alice", tokens[2].value)
}

func TestTokenizeWithSpaces(t *testing.T) {
	t.Parallel()
	tokens, err := tokenize("age > 25")
	testutil.NoError(t, err)
	testutil.SliceLen(t, tokens, 3)
	testutil.Equal(t, "age", tokens[0].value)
	testutil.Equal(t, ">", tokens[1].value)
	testutil.Equal(t, "25", tokens[2].value)
	testutil.Equal(t, tokNumber, tokens[2].kind)
}

func TestTokenizeAnd(t *testing.T) {
	t.Parallel()
	tokens, err := tokenize("a=1 && b=2")
	testutil.NoError(t, err)
	testutil.SliceLen(t, tokens, 7)
	testutil.Equal(t, tokAnd, tokens[3].kind)
}

func TestTokenizeOr(t *testing.T) {
	t.Parallel()
	tokens, err := tokenize("a=1 || b=2")
	testutil.NoError(t, err)
	testutil.SliceLen(t, tokens, 7)
	testutil.Equal(t, tokOr, tokens[3].kind)
}

func TestTokenizeAndKeyword(t *testing.T) {
	t.Parallel()
	tokens, err := tokenize("a=1 AND b=2")
	testutil.NoError(t, err)
	testutil.SliceLen(t, tokens, 7)
	testutil.Equal(t, tokAnd, tokens[3].kind)
}

func TestTokenizeOrKeyword(t *testing.T) {
	t.Parallel()
	tokens, err := tokenize("a=1 OR b=2")
	testutil.NoError(t, err)
	testutil.SliceLen(t, tokens, 7)
	testutil.Equal(t, tokOr, tokens[3].kind)
}

func TestTokenizeParens(t *testing.T) {
	t.Parallel()
	tokens, err := tokenize("(a=1)")
	testutil.NoError(t, err)
	testutil.SliceLen(t, tokens, 5)
	testutil.Equal(t, tokLParen, tokens[0].kind)
	testutil.Equal(t, tokRParen, tokens[4].kind)
}

func TestTokenizeBool(t *testing.T) {
	t.Parallel()
	tokens, err := tokenize("active=true")
	testutil.NoError(t, err)
	testutil.SliceLen(t, tokens, 3)
	testutil.Equal(t, tokBool, tokens[2].kind)
	testutil.Equal(t, "true", tokens[2].value)
}

func TestTokenizeNull(t *testing.T) {
	t.Parallel()
	tokens, err := tokenize("name=null")
	testutil.NoError(t, err)
	testutil.SliceLen(t, tokens, 3)
	testutil.Equal(t, tokNull, tokens[2].kind)
}

func TestTokenizeIn(t *testing.T) {
	t.Parallel()
	tokens, err := tokenize("status IN ('a','b','c')")
	testutil.NoError(t, err)
	testutil.Equal(t, tokIn, tokens[1].kind)
	testutil.Equal(t, tokLParen, tokens[2].kind)
}

func TestTokenizeOperators(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		op    string
	}{
		{"a=1", "="},
		{"a!=1", "!="},
		{"a>1", ">"},
		{"a>=1", ">="},
		{"a<1", "<"},
		{"a<=1", "<="},
		{"a~'x'", "~"},
		{"a!~'x'", "!~"},
	}

	for _, tc := range tests {
		tokens, err := tokenize(tc.input)
		testutil.NoError(t, err)
		testutil.Equal(t, tokOp, tokens[1].kind)
		testutil.Equal(t, tc.op, tokens[1].value)
	}
}

func TestTokenizeFloat(t *testing.T) {
	t.Parallel()
	tokens, err := tokenize("age>3.14")
	testutil.NoError(t, err)
	testutil.Equal(t, "3.14", tokens[2].value)
	testutil.Equal(t, tokNumber, tokens[2].kind)
}

func TestTokenizeEscapedQuote(t *testing.T) {
	t.Parallel()
	tokens, err := tokenize(`name='it\'s'`)
	testutil.NoError(t, err)
	testutil.SliceLen(t, tokens, 3)
	testutil.Equal(t, tokString, tokens[2].kind)
	testutil.Equal(t, "it's", tokens[2].value)
}

func TestTokenizeEscapedBackslash(t *testing.T) {
	t.Parallel()
	tokens, err := tokenize(`name='a\\b'`)
	testutil.NoError(t, err)
	testutil.SliceLen(t, tokens, 3)
	testutil.Equal(t, `a\b`, tokens[2].value)
}

func TestTokenizeUnterminatedString(t *testing.T) {
	t.Parallel()
	_, err := tokenize("name='unterminated")
	testutil.ErrorContains(t, err, "unterminated")
}

func TestTokenizeUnexpectedChar(t *testing.T) {
	t.Parallel()
	_, err := tokenize("name=$1")
	testutil.ErrorContains(t, err, "unexpected")
}

// --- Parser tests ---

func TestParseFilterSimpleEquals(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "name='Alice'")
	testutil.NoError(t, err)
	testutil.Equal(t, `"name" = $1`, sql)
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, "Alice", args[0].(string))
}

func TestParseFilterNumber(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "age>25")
	testutil.NoError(t, err)
	testutil.Equal(t, `"age" > $1`, sql)
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, int64(25), args[0].(int64))
}

func TestParseFilterFloat(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "age>3.14")
	testutil.NoError(t, err)
	testutil.Contains(t, sql, `"age" > $1`)
	testutil.Equal(t, 3.14, args[0].(float64))
}

func TestParseFilterBool(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "active=true")
	testutil.NoError(t, err)
	testutil.Equal(t, `"active" = $1`, sql)
	testutil.Equal(t, true, args[0].(bool))
}

func TestParseFilterNull(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "name=null")
	testutil.NoError(t, err)
	testutil.Equal(t, `"name" IS NULL`, sql)
	testutil.SliceLen(t, args, 0)
}

func TestParseFilterNotNull(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "name!=null")
	testutil.NoError(t, err)
	testutil.Equal(t, `"name" IS NOT NULL`, sql)
	testutil.SliceLen(t, args, 0)
}

func TestParseFilterAnd(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "name='Alice' && age>25")
	testutil.NoError(t, err)
	testutil.Equal(t, `("name" = $1 AND "age" > $2)`, sql)
	testutil.SliceLen(t, args, 2)
}

func TestParseFilterOr(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "name='Alice' || name='Bob'")
	testutil.NoError(t, err)
	testutil.Equal(t, `("name" = $1 OR "name" = $2)`, sql)
	testutil.SliceLen(t, args, 2)
}

func TestParseFilterAndKeyword(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "name='Alice' AND age>25")
	testutil.NoError(t, err)
	testutil.Equal(t, `("name" = $1 AND "age" > $2)`, sql)
	testutil.SliceLen(t, args, 2)
}

func TestParseFilterOrKeyword(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "name='Alice' OR name='Bob'")
	testutil.NoError(t, err)
	testutil.Equal(t, `("name" = $1 OR "name" = $2)`, sql)
	testutil.SliceLen(t, args, 2)
}

func TestParseFilterParens(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "(name='Alice' || name='Bob') && age>25")
	testutil.NoError(t, err)
	testutil.Equal(t, `(("name" = $1 OR "name" = $2) AND "age" > $3)`, sql)
	testutil.SliceLen(t, args, 3)
}

func TestParseFilterLike(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "name~'%Ali%'")
	testutil.NoError(t, err)
	testutil.Equal(t, `"name" LIKE $1`, sql)
	testutil.Equal(t, "%Ali%", args[0].(string))
}

func TestParseFilterNotLike(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "name!~'%Ali%'")
	testutil.NoError(t, err)
	testutil.Equal(t, `"name" NOT LIKE $1`, sql)
	testutil.Equal(t, "%Ali%", args[0].(string))
}

func TestParseFilterIn(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "status IN ('active','inactive')")
	testutil.NoError(t, err)
	testutil.Equal(t, `"status" IN ($1, $2)`, sql)
	testutil.SliceLen(t, args, 2)
	testutil.Equal(t, "active", args[0].(string))
	testutil.Equal(t, "inactive", args[1].(string))
}

func TestParseFilterComplex(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "status='active' && (age>=18 || name='admin')")
	testutil.NoError(t, err)
	testutil.Equal(t, `("status" = $1 AND ("age" >= $2 OR "name" = $3))`, sql)
	testutil.SliceLen(t, args, 3)
}

func TestParseFilterEscapedQuote(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, `name='it\'s'`)
	testutil.NoError(t, err)
	testutil.Equal(t, `"name" = $1`, sql)
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, "it's", args[0].(string))
}

func TestParseFilterUnknownColumn(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	_, _, err := parseFilter(tbl, "nonexistent='x'")
	testutil.ErrorContains(t, err, "unknown column")
}

func TestParseFilterEmpty(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "")
	testutil.NoError(t, err)
	testutil.Equal(t, "", sql)
	testutil.Nil(t, args)
}

func TestParseFilterMissingOperator(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	_, _, err := parseFilter(tbl, "name 'Alice'")
	testutil.ErrorContains(t, err, "expected")
}

func TestParseFilterUnclosedParen(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	_, _, err := parseFilter(tbl, "(name='Alice'")
	testutil.ErrorContains(t, err, "closing parenthesis")
}

func TestParseFilterMultipleAnd(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	sql, args, err := parseFilter(tbl, "name='A' && age>1 && active=true")
	testutil.NoError(t, err)
	testutil.Equal(t, `(("name" = $1 AND "age" > $2) AND "active" = $3)`, sql)
	testutil.SliceLen(t, args, 3)
}

func TestParseFilterOperatorPrecedence(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	// OR has lower precedence than AND, so "a || b && c" should be "a || (b && c)"
	// But our grammar is: or_expr = and_expr (OR and_expr)*
	// So: a=1 || b=2 && c=3 → (a=1) OR ((b=2) AND (c=3))
	sql, args, err := parseFilter(tbl, "name='a' || age>2 && active=true")
	testutil.NoError(t, err)
	testutil.Equal(t, `("name" = $1 OR ("age" > $2 AND "active" = $3))`, sql)
	testutil.SliceLen(t, args, 3)
}

// --- parseSortSQL tests ---

func TestParseSortSQLEmpty(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	testutil.Equal(t, "", parseSortSQL(tbl, ""))
}

func TestParseSortSQLSingleAsc(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	testutil.Equal(t, `"name" ASC`, parseSortSQL(tbl, "name"))
}

func TestParseSortSQLSingleDesc(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	testutil.Equal(t, `"name" DESC`, parseSortSQL(tbl, "-name"))
}

func TestParseSortSQLExplicitAsc(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	testutil.Equal(t, `"name" ASC`, parseSortSQL(tbl, "+name"))
}

func TestParseSortSQLMultiple(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	testutil.Equal(t, `"age" DESC, "name" ASC`, parseSortSQL(tbl, "-age,+name"))
}

func TestParseSortSQLIgnoresInvalidColumns(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	testutil.Equal(t, `"name" ASC`, parseSortSQL(tbl, "-nonexistent,name"))
}

// --- Filter depth limit ---

func TestFilterDepthLimit(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	// Build deeply nested expression: (((((...(id=1)...))))
	nested := strings.Repeat("(", maxFilterDepth+1) + "id=1" + strings.Repeat(")", maxFilterDepth+1)
	_, _, err := parseFilter(tbl, nested)
	testutil.ErrorContains(t, err, "too deeply nested")
}

func TestFilterDepthAtLimit(t *testing.T) {
	t.Parallel()
	tbl := filterTestTable()
	// Build expression at exactly the limit — should succeed.
	nested := strings.Repeat("(", maxFilterDepth) + "id=1" + strings.Repeat(")", maxFilterDepth)
	sql, _, err := parseFilter(tbl, nested)
	testutil.NoError(t, err)
	testutil.True(t, len(sql) > 0, "filter at depth limit should produce SQL")
}
