package api

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/allyourbase/ayb/internal/schema"
)

// parseFilter parses a filter expression string and returns parameterized SQL.
// Example: "status='active' && age>25" → ("status" = $1 AND "age" > $2), ["active", 25]
func parseFilter(tbl *schema.Table, input string) (string, []any, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return "", nil, err
	}
	if len(tokens) == 0 {
		return "", nil, nil
	}

	p := &parser{
		tokens: tokens,
		pos:    0,
		tbl:    tbl,
		args:   make([]any, 0),
	}

	node, err := p.parseExpression()
	if err != nil {
		return "", nil, err
	}

	if p.pos < len(p.tokens) {
		return "", nil, fmt.Errorf("unexpected token at position %d: %s", p.pos, p.tokens[p.pos].value)
	}

	sql := node.toSQL()
	return sql, p.args, nil
}

// Token types
type tokenKind int

const (
	tokIdent  tokenKind = iota // column name
	tokString                  // 'quoted string'
	tokNumber                  // 123, 45.6
	tokBool                    // true, false
	tokNull                    // null
	tokOp                      // =, !=, >, >=, <, <=, ~, !~
	tokAnd                     // &&, AND
	tokOr                      // ||, OR
	tokIn                      // IN
	tokLParen                  // (
	tokRParen                  // )
	tokComma                   // ,
)

type token struct {
	kind  tokenKind
	value string
}

// tokenize breaks the input into tokens.
func tokenize(input string) ([]token, error) {
	var tokens []token
	runes := []rune(input)
	i := 0

	for i < len(runes) {
		ch := runes[i]

		// Skip whitespace.
		if unicode.IsSpace(ch) {
			i++
			continue
		}

		// String literal.
		if ch == '\'' {
			var buf []rune
			j := i + 1
			for j < len(runes) && runes[j] != '\'' {
				if runes[j] == '\\' && j+1 < len(runes) {
					j++ // skip backslash, take next char literally
				}
				buf = append(buf, runes[j])
				j++
			}
			if j >= len(runes) {
				return nil, fmt.Errorf("unterminated string at position %d", i)
			}
			tokens = append(tokens, token{tokString, string(buf)})
			i = j + 1
			continue
		}

		// Operators and punctuation.
		if ch == '(' {
			tokens = append(tokens, token{tokLParen, "("})
			i++
			continue
		}
		if ch == ')' {
			tokens = append(tokens, token{tokRParen, ")"})
			i++
			continue
		}
		if ch == ',' {
			tokens = append(tokens, token{tokComma, ","})
			i++
			continue
		}

		// Two-char operators.
		if i+1 < len(runes) {
			two := string(runes[i : i+2])
			switch two {
			case "&&":
				tokens = append(tokens, token{tokAnd, "&&"})
				i += 2
				continue
			case "||":
				tokens = append(tokens, token{tokOr, "||"})
				i += 2
				continue
			case "!=":
				tokens = append(tokens, token{tokOp, "!="})
				i += 2
				continue
			case ">=":
				tokens = append(tokens, token{tokOp, ">="})
				i += 2
				continue
			case "<=":
				tokens = append(tokens, token{tokOp, "<="})
				i += 2
				continue
			case "!~":
				tokens = append(tokens, token{tokOp, "!~"})
				i += 2
				continue
			}
		}

		// Single-char operators.
		if ch == '=' || ch == '>' || ch == '<' || ch == '~' {
			tokens = append(tokens, token{tokOp, string(ch)})
			i++
			continue
		}

		// Numbers.
		if unicode.IsDigit(ch) || (ch == '-' && i+1 < len(runes) && unicode.IsDigit(runes[i+1])) {
			j := i
			if ch == '-' {
				j++
			}
			for j < len(runes) && (unicode.IsDigit(runes[j]) || runes[j] == '.') {
				j++
			}
			tokens = append(tokens, token{tokNumber, string(runes[i:j])})
			i = j
			continue
		}

		// Identifiers and keywords.
		if unicode.IsLetter(ch) || ch == '_' {
			j := i
			for j < len(runes) && (unicode.IsLetter(runes[j]) || unicode.IsDigit(runes[j]) || runes[j] == '_' || runes[j] == '.') {
				j++
			}
			word := string(runes[i:j])
			upper := strings.ToUpper(word)

			switch upper {
			case "AND":
				tokens = append(tokens, token{tokAnd, "AND"})
			case "OR":
				tokens = append(tokens, token{tokOr, "OR"})
			case "IN":
				tokens = append(tokens, token{tokIn, "IN"})
			case "TRUE", "FALSE":
				tokens = append(tokens, token{tokBool, strings.ToLower(word)})
			case "NULL":
				tokens = append(tokens, token{tokNull, "null"})
			default:
				tokens = append(tokens, token{tokIdent, word})
			}
			i = j
			continue
		}

		return nil, fmt.Errorf("unexpected character '%c' at position %d", ch, i)
	}

	return tokens, nil
}

// AST node types.
type filterNode interface {
	toSQL() string
}

type andNode struct {
	left, right filterNode
}

func (n *andNode) toSQL() string {
	return "(" + n.left.toSQL() + " AND " + n.right.toSQL() + ")"
}

type orNode struct {
	left, right filterNode
}

func (n *orNode) toSQL() string {
	return "(" + n.left.toSQL() + " OR " + n.right.toSQL() + ")"
}

type comparisonNode struct {
	column   string
	op       string
	paramRef string // e.g., "$1"
}

func (n *comparisonNode) toSQL() string {
	return n.column + " " + n.op + " " + n.paramRef
}

type inNode struct {
	column    string
	paramRefs []string
}

func (n *inNode) toSQL() string {
	return n.column + " IN (" + strings.Join(n.paramRefs, ", ") + ")"
}

type isNullNode struct {
	column string
	isNull bool
}

func (n *isNullNode) toSQL() string {
	if n.isNull {
		return n.column + " IS NULL"
	}
	return n.column + " IS NOT NULL"
}

const maxFilterDepth = 50 // max nesting depth for parenthesized expressions

// parser is a recursive descent parser for filter expressions.
type parser struct {
	tokens []token
	pos    int
	tbl    *schema.Table
	args   []any
	depth  int
}

func (p *parser) peek() *token {
	if p.pos >= len(p.tokens) {
		return nil
	}
	return &p.tokens[p.pos]
}

func (p *parser) advance() token {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

func (p *parser) addArg(val any) string {
	p.args = append(p.args, val)
	return fmt.Sprintf("$%d", len(p.args))
}

// expression = and_expr
func (p *parser) parseExpression() (filterNode, error) {
	return p.parseOrExpr()
}

// or_expr = and_expr (("||" | "OR") and_expr)*
func (p *parser) parseOrExpr() (filterNode, error) {
	left, err := p.parseAndExpr()
	if err != nil {
		return nil, err
	}

	for {
		t := p.peek()
		if t == nil || t.kind != tokOr {
			break
		}
		p.advance()
		right, err := p.parseAndExpr()
		if err != nil {
			return nil, err
		}
		left = &orNode{left: left, right: right}
	}

	return left, nil
}

// and_expr = primary (("&&" | "AND") primary)*
func (p *parser) parseAndExpr() (filterNode, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}

	for {
		t := p.peek()
		if t == nil || t.kind != tokAnd {
			break
		}
		p.advance()
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = &andNode{left: left, right: right}
	}

	return left, nil
}

// primary = comparison | "(" expression ")"
func (p *parser) parsePrimary() (filterNode, error) {
	t := p.peek()
	if t == nil {
		return nil, fmt.Errorf("unexpected end of filter expression")
	}

	// Parenthesized expression.
	if t.kind == tokLParen {
		p.depth++
		if p.depth > maxFilterDepth {
			return nil, fmt.Errorf("filter expression too deeply nested (max %d levels)", maxFilterDepth)
		}
		p.advance()
		node, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		closing := p.peek()
		if closing == nil || closing.kind != tokRParen {
			return nil, fmt.Errorf("expected closing parenthesis")
		}
		p.advance()
		p.depth--
		return node, nil
	}

	// Must be a comparison: identifier op value
	return p.parseComparison()
}

// comparison = identifier op value | identifier "IN" "(" value ("," value)* ")"
func (p *parser) parseComparison() (filterNode, error) {
	t := p.peek()
	if t == nil || t.kind != tokIdent {
		return nil, fmt.Errorf("expected column name, got %v", t)
	}
	ident := p.advance()

	// Validate column against schema.
	col := p.tbl.ColumnByName(ident.value)
	if col == nil {
		return nil, fmt.Errorf("unknown column: %s", ident.value)
	}
	quotedCol := quoteIdent(ident.value)

	// Check for IN.
	next := p.peek()
	if next != nil && next.kind == tokIn {
		p.advance() // consume IN

		lp := p.peek()
		if lp == nil || lp.kind != tokLParen {
			return nil, fmt.Errorf("expected '(' after IN")
		}
		p.advance()

		var paramRefs []string
		for {
			val, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			ref := p.addArg(val)
			paramRefs = append(paramRefs, ref)

			next := p.peek()
			if next == nil {
				return nil, fmt.Errorf("expected ')' to close IN list")
			}
			if next.kind == tokRParen {
				p.advance()
				break
			}
			if next.kind != tokComma {
				return nil, fmt.Errorf("expected ',' or ')' in IN list")
			}
			p.advance()
		}

		return &inNode{column: quotedCol, paramRefs: paramRefs}, nil
	}

	// Regular comparison operator.
	opTok := p.peek()
	if opTok == nil || opTok.kind != tokOp {
		return nil, fmt.Errorf("expected operator after column %s", ident.value)
	}
	op := p.advance()

	// Parse value.
	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	// Handle null comparisons specially.
	if val == nil {
		switch op.value {
		case "=":
			return &isNullNode{column: quotedCol, isNull: true}, nil
		case "!=":
			return &isNullNode{column: quotedCol, isNull: false}, nil
		default:
			return nil, fmt.Errorf("null can only be compared with = or !=")
		}
	}

	// Map ~ and !~ to LIKE/NOT LIKE (PocketBase compatibility).
	sqlOp := op.value
	switch op.value {
	case "~":
		sqlOp = "LIKE"
	case "!~":
		sqlOp = "NOT LIKE"
	}

	ref := p.addArg(val)
	return &comparisonNode{column: quotedCol, op: sqlOp, paramRef: ref}, nil
}

// parseValue parses a literal value token.
func (p *parser) parseValue() (any, error) {
	t := p.peek()
	if t == nil {
		return nil, fmt.Errorf("expected value, got end of input")
	}

	switch t.kind {
	case tokString:
		p.advance()
		return t.value, nil
	case tokNumber:
		p.advance()
		if strings.Contains(t.value, ".") {
			f, err := strconv.ParseFloat(t.value, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid number: %s", t.value)
			}
			return f, nil
		}
		n, err := strconv.ParseInt(t.value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number: %s", t.value)
		}
		return n, nil
	case tokBool:
		p.advance()
		return t.value == "true", nil
	case tokNull:
		p.advance()
		return nil, nil
	default:
		return nil, fmt.Errorf("expected value, got %s", t.value)
	}
}
