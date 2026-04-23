package dsl

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// exprEvaluator is a lightweight condition expression evaluator.
// It supports comparison operators, logical operators, field access, and literals.
type exprEvaluator struct{}

// Evaluate evaluates an expression string against the given variables and returns a boolean result.
// Supported operators: ==, !=, >, <, >=, <=, &&, ||, !
// Supported literals: numbers, quoted strings, true, false
// Supports dot-notation field access: result.score looks up vars["result"].(map[string]any)["score"]
func (e *exprEvaluator) Evaluate(expr string, vars map[string]any) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false, nil
	}

	tokens, err := tokenize(expr)
	if err != nil {
		return false, err
	}
	if len(tokens) == 0 {
		return false, nil
	}

	p := &exprParser{tokens: tokens, pos: 0, vars: vars}
	val, err := p.parseOr()
	if err != nil {
		return false, err
	}
	if p.pos < len(p.tokens) {
		return false, fmt.Errorf("unexpected token %q at position %d", p.tokens[p.pos].value, p.pos)
	}
	return toBool(val), nil
}

// --- Token types ---

type tokenKind int

const (
	tkNumber tokenKind = iota // 42, 0.8, -3.14
	tkString                  // "hello"
	tkIdent                   // variable name or true/false
	tkOp                      // ==, !=, >, <, >=, <=, &&, ||, !
	tkLParen                  // (
	tkRParen                  // )
)

type token struct {
	kind  tokenKind
	value string
}

// --- Tokenizer ---

func tokenize(expr string) ([]token, error) {
	var tokens []token
	i := 0
	runes := []rune(expr)

	for i < len(runes) {
		ch := runes[i]

		// Skip whitespace
		if unicode.IsSpace(ch) {
			i++
			continue
		}

		// Parentheses
		if ch == '(' {
			tokens = append(tokens, token{tkLParen, "("})
			i++
			continue
		}
		if ch == ')' {
			tokens = append(tokens, token{tkRParen, ")"})
			i++
			continue
		}

		// String literal
		if ch == '"' {
			s, n, err := readString(runes, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{tkString, s})
			i = n
			continue
		}

		// Two-character operators
		if i+1 < len(runes) {
			two := string(runes[i : i+2])
			switch two {
			case "==", "!=", ">=", "<=", "&&", "||":
				tokens = append(tokens, token{tkOp, two})
				i += 2
				continue
			}
		}

		// Single-character operators
		if ch == '>' || ch == '<' || ch == '!' {
			tokens = append(tokens, token{tkOp, string(ch)})
			i++
			continue
		}

		// Number (including negative: only if preceded by an operator or start)
		if isDigit(ch) || (ch == '-' && i+1 < len(runes) && isDigit(runes[i+1]) && isNumberStart(tokens)) {
			num, n := readNumber(runes, i)
			tokens = append(tokens, token{tkNumber, num})
			i = n
			continue
		}

		// Identifier (variable name, true, false)
		if isIdentStart(ch) {
			ident, n := readIdent(runes, i)
			tokens = append(tokens, token{tkIdent, ident})
			i = n
			continue
		}

		return nil, fmt.Errorf("unexpected character %q at position %d", string(ch), i)
	}

	return tokens, nil
}

func readString(runes []rune, start int) (string, int, error) {
	i := start + 1 // skip opening quote
	var sb strings.Builder
	for i < len(runes) {
		if runes[i] == '\\' && i+1 < len(runes) {
			sb.WriteRune(runes[i+1])
			i += 2
			continue
		}
		if runes[i] == '"' {
			return sb.String(), i + 1, nil
		}
		sb.WriteRune(runes[i])
		i++
	}
	return "", 0, fmt.Errorf("unterminated string starting at position %d", start)
}

func readNumber(runes []rune, start int) (string, int) {
	i := start
	if i < len(runes) && runes[i] == '-' {
		i++
	}
	for i < len(runes) && isDigit(runes[i]) {
		i++
	}
	if i < len(runes) && runes[i] == '.' {
		i++
		for i < len(runes) && isDigit(runes[i]) {
			i++
		}
	}
	return string(runes[start:i]), i
}

func readIdent(runes []rune, start int) (string, int) {
	i := start
	for i < len(runes) && isIdentPart(runes[i]) {
		i++
	}
	return string(runes[start:i]), i
}

func isDigit(ch rune) bool      { return ch >= '0' && ch <= '9' }
func isIdentStart(ch rune) bool { return unicode.IsLetter(ch) || ch == '_' }
func isIdentPart(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '.'
}

// isNumberStart returns true if a '-' should be treated as a negative number prefix
// rather than a subtraction operator. This is the case at the start of the expression
// or after an operator or opening parenthesis.
func isNumberStart(preceding []token) bool {
	if len(preceding) == 0 {
		return true
	}
	last := preceding[len(preceding)-1]
	return last.kind == tkOp || last.kind == tkLParen
}

// --- Recursive descent parser ---

type exprParser struct {
	tokens []token
	pos    int
	vars   map[string]any
}

func (p *exprParser) peek() *token {
	if p.pos < len(p.tokens) {
		return &p.tokens[p.pos]
	}
	return nil
}

func (p *exprParser) advance() token {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

// parseOr handles: expr || expr
func (p *exprParser) parseOr() (any, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek() != nil && p.peek().kind == tkOp && p.peek().value == "||" {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = toBool(left) || toBool(right)
	}
	return left, nil
}

// parseAnd handles: expr && expr
func (p *exprParser) parseAnd() (any, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	for p.peek() != nil && p.peek().kind == tkOp && p.peek().value == "&&" {
		p.advance()
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		left = toBool(left) && toBool(right)
	}
	return left, nil
}

// parseComparison handles: expr (==|!=|>|<|>=|<=) expr
func (p *exprParser) parseComparison() (any, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	if p.peek() != nil && p.peek().kind == tkOp {
		op := p.peek().value
		switch op {
		case "==", "!=", ">", "<", ">=", "<=":
			p.advance()
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return evalComparison(left, op, right), nil
		}
	}
	return left, nil
}

// parseUnary handles: !expr, primary
func (p *exprParser) parseUnary() (any, error) {
	if p.peek() != nil && p.peek().kind == tkOp && p.peek().value == "!" {
		p.advance()
		val, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return !toBool(val), nil
	}
	return p.parsePrimary()
}

// parsePrimary handles: literals, identifiers, parenthesized expressions
func (p *exprParser) parsePrimary() (any, error) {
	t := p.peek()
	if t == nil {
		return nil, fmt.Errorf("unexpected end of expression")
	}

	switch t.kind {
	case tkNumber:
		p.advance()
		return parseNumber(t.value)

	case tkString:
		p.advance()
		return t.value, nil

	case tkIdent:
		p.advance()
		switch t.value {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return resolveVar(t.value, p.vars), nil
		}

	case tkLParen:
		p.advance()
		val, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek() == nil || p.peek().kind != tkRParen {
			return nil, fmt.Errorf("expected closing parenthesis")
		}
		p.advance()
		return val, nil

	default:
		return nil, fmt.Errorf("unexpected token %q", t.value)
	}
}

// --- Evaluation helpers ---

// resolveVar resolves a dot-notation variable path from the vars map.
// "status" -> vars["status"]
// "result.score" -> vars["result"].(map[string]any)["score"]
func resolveVar(path string, vars map[string]any) any {
	parts := strings.Split(path, ".")
	var current any = vars

	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = m[part]
		if !ok {
			return nil
		}
	}
	return current
}

// evalComparison evaluates a comparison between two values.
// nil is treated as less than any non-nil value; two nils are equal.
func evalComparison(left any, op string, right any) bool {
	// Handle nil: nil compared to anything non-nil is only equal via ==
	if left == nil && right == nil {
		return op == "==" || op == ">=" || op == "<="
	}
	if left == nil || right == nil {
		if op == "!=" {
			return true
		}
		if op == "==" {
			return false
		}
		// For ordering comparisons, nil is "less than" any value
		if left == nil {
			return op == "<" || op == "<="
		}
		return op == ">" || op == ">="
	}

	// Try numeric comparison first
	lf, lok := toFloat64(left)
	rf, rok := toFloat64(right)
	if lok && rok {
		switch op {
		case "==":
			return lf == rf
		case "!=":
			return lf != rf
		case ">":
			return lf > rf
		case "<":
			return lf < rf
		case ">=":
			return lf >= rf
		case "<=":
			return lf <= rf
		}
	}

	// Fall back to string comparison
	ls := fmt.Sprintf("%v", left)
	rs := fmt.Sprintf("%v", right)
	switch op {
	case "==":
		return ls == rs
	case "!=":
		return ls != rs
	case ">":
		return ls > rs
	case "<":
		return ls < rs
	case ">=":
		return ls >= rs
	case "<=":
		return ls <= rs
	}
	return false
}

// toBool converts a value to boolean.
func toBool(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case int:
		return val != 0
	case string:
		return val != "" && val != "false" && val != "0"
	default:
		return true
	}
}

// toFloat64 attempts to convert a value to float64.
func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case float32:
		return float64(val), true
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err == nil {
			return f, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// parseNumber parses a number string to float64.
func parseNumber(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
