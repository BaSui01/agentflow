package dsl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// exprEvaluator unit tests
// =============================================================================

func TestExprEvaluator_Evaluate(t *testing.T) {
	eval := &exprEvaluator{}

	tests := []struct {
		name     string
		expr     string
		vars     map[string]any
		expected bool
		wantErr  bool
	}{
		// --- Comparison operators ---
		{
			name:     "greater than true",
			expr:     `score > 0.8`,
			vars:     map[string]any{"score": 0.9},
			expected: true,
		},
		{
			name:     "greater than false",
			expr:     `score > 0.8`,
			vars:     map[string]any{"score": 0.5},
			expected: false,
		},
		{
			name:     "equal string",
			expr:     `status == "active"`,
			vars:     map[string]any{"status": "active"},
			expected: true,
		},
		{
			name:     "equal string false",
			expr:     `status == "active"`,
			vars:     map[string]any{"status": "inactive"},
			expected: false,
		},
		{
			name:     "not equal true",
			expr:     `count != 0`,
			vars:     map[string]any{"count": 5},
			expected: true,
		},
		{
			name:     "not equal false",
			expr:     `count != 0`,
			vars:     map[string]any{"count": 0},
			expected: false,
		},
		{
			name:     "greater than or equal true",
			expr:     `count >= 10`,
			vars:     map[string]any{"count": 10},
			expected: true,
		},
		{
			name:     "greater than or equal false",
			expr:     `count >= 10`,
			vars:     map[string]any{"count": 9},
			expected: false,
		},
		{
			name:     "less than or equal true",
			expr:     `count <= 5`,
			vars:     map[string]any{"count": 3},
			expected: true,
		},
		{
			name:     "less than true",
			expr:     `count < 5`,
			vars:     map[string]any{"count": 3},
			expected: true,
		},
		{
			name:     "less than false",
			expr:     `count < 5`,
			vars:     map[string]any{"count": 5},
			expected: false,
		},

		// --- Logical operators ---
		{
			name:     "and both true",
			expr:     `score > 0.8 && status == "active"`,
			vars:     map[string]any{"score": 0.9, "status": "active"},
			expected: true,
		},
		{
			name:     "and one false",
			expr:     `score > 0.8 && status == "active"`,
			vars:     map[string]any{"score": 0.5, "status": "active"},
			expected: false,
		},
		{
			name:     "or one true",
			expr:     `score > 0.8 || status == "active"`,
			vars:     map[string]any{"score": 0.5, "status": "active"},
			expected: true,
		},
		{
			name:     "or both false",
			expr:     `score > 0.8 || status == "active"`,
			vars:     map[string]any{"score": 0.5, "status": "inactive"},
			expected: false,
		},
		{
			name:     "not false becomes true",
			expr:     `!done`,
			vars:     map[string]any{"done": false},
			expected: true,
		},
		{
			name:     "not true becomes false",
			expr:     `!done`,
			vars:     map[string]any{"done": true},
			expected: false,
		},
		// --- Field access (dot notation) ---
		{
			name: "nested field access",
			expr: `result.score > 0.8`,
			vars: map[string]any{
				"result": map[string]any{"score": 0.9},
			},
			expected: true,
		},
		{
			name: "deep nested field access",
			expr: `a.b.c == "deep"`,
			vars: map[string]any{
				"a": map[string]any{
					"b": map[string]any{"c": "deep"},
				},
			},
			expected: true,
		},
		{
			name: "nested field not found",
			expr: `result.missing > 0`,
			vars: map[string]any{
				"result": map[string]any{"score": 0.9},
			},
			expected: false,
		},

		// --- Parentheses ---
		{
			name:     "parenthesized and",
			expr:     `(a > 1) && (b < 10)`,
			vars:     map[string]any{"a": 2, "b": 5},
			expected: true,
		},
		{
			name:     "parentheses change precedence",
			expr:     `(a > 1 || b > 10) && c == "yes"`,
			vars:     map[string]any{"a": 2, "b": 5, "c": "yes"},
			expected: true,
		},

		// --- Literals ---
		{
			name:     "boolean literal true",
			expr:     `true`,
			vars:     map[string]any{},
			expected: true,
		},
		{
			name:     "boolean literal false",
			expr:     `false`,
			vars:     map[string]any{},
			expected: false,
		},
		{
			name:     "number literal nonzero is true",
			expr:     `42`,
			vars:     map[string]any{},
			expected: true,
		},
		{
			name:     "number literal zero is false",
			expr:     `0`,
			vars:     map[string]any{},
			expected: false,
		},

		// --- Edge cases ---
		{
			name:     "empty expression",
			expr:     ``,
			vars:     map[string]any{},
			expected: false,
		},
		{
			name:     "undefined variable",
			expr:     `unknown_var > 0`,
			vars:     map[string]any{},
			expected: false,
		},
		{
			name:     "integer variable comparison",
			expr:     `count == 42`,
			vars:     map[string]any{"count": 42},
			expected: true,
		},
		{
			name:     "negative number",
			expr:     `temp > -10`,
			vars:     map[string]any{"temp": 5},
			expected: true,
		},
		{
			name:     "string with spaces",
			expr:     `name == "hello world"`,
			vars:     map[string]any{"name": "hello world"},
			expected: true,
		},

		// --- Error cases ---
		{
			name:    "unterminated string",
			expr:    `status == "active`,
			vars:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "missing closing paren",
			expr:    `(a > 1`,
			vars:    map[string]any{"a": 2},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Evaluate(tt.expr, tt.vars)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// tokenize unit tests
// =============================================================================

func TestTokenize(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected []token
		wantErr  bool
	}{
		{
			name: "simple comparison",
			expr: `score > 0.8`,
			expected: []token{
				{tkIdent, "score"},
				{tkOp, ">"},
				{tkNumber, "0.8"},
			},
		},
		{
			name: "string literal",
			expr: `status == "active"`,
			expected: []token{
				{tkIdent, "status"},
				{tkOp, "=="},
				{tkString, "active"},
			},
		},
		{
			name: "logical and",
			expr: `a && b`,
			expected: []token{
				{tkIdent, "a"},
				{tkOp, "&&"},
				{tkIdent, "b"},
			},
		},
		{
			name: "parentheses",
			expr: `(a > 1)`,
			expected: []token{
				{tkLParen, "("},
				{tkIdent, "a"},
				{tkOp, ">"},
				{tkNumber, "1"},
				{tkRParen, ")"},
			},
		},
		{
			name: "dot notation identifier",
			expr: `result.score`,
			expected: []token{
				{tkIdent, "result.score"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := tokenize(tt.expr)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, tokens)
		})
	}
}

// =============================================================================
// resolveVar unit tests
// =============================================================================

func TestResolveVar(t *testing.T) {
	vars := map[string]any{
		"simple": "hello",
		"nested": map[string]any{
			"value": 42,
			"deep": map[string]any{
				"item": "found",
			},
		},
	}

	tests := []struct {
		name     string
		path     string
		expected any
	}{
		{"simple key", "simple", "hello"},
		{"nested key", "nested.value", 42},
		{"deep nested key", "nested.deep.item", "found"},
		{"missing key", "missing", nil},
		{"missing nested key", "nested.missing", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveVar(tt.path, vars)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// Integration: parseSimpleExpression with runtime input
// =============================================================================

func TestParseSimpleExpression_Integration(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		expr     string
		vars     map[string]any
		input    any
		expected bool
	}{
		{
			name:     "runtime input map merged",
			expr:     `score > 0.5`,
			vars:     map[string]any{},
			input:    map[string]any{"score": 0.9},
			expected: true,
		},
		{
			name:     "static vars take precedence in merge",
			expr:     `mode == "test"`,
			vars:     map[string]any{},
			input:    map[string]any{"mode": "test"},
			expected: true,
		},
		{
			name:     "non-map input stored as input key",
			expr:     `input == "hello"`,
			vars:     map[string]any{},
			input:    "hello",
			expected: true,
		},
		{
			name:     "backward compat: simple truthy string",
			expr:     `some_value`,
			vars:     map[string]any{"some_value": "yes"},
			input:    nil,
			expected: true,
		},
		{
			name:     "backward compat: false string",
			expr:     `false`,
			vars:     map[string]any{},
			input:    nil,
			expected: false,
		},
		{
			name:     "backward compat: zero string",
			expr:     `0`,
			vars:     map[string]any{},
			input:    nil,
			expected: false,
		},
		{
			name:     "variable interpolation then evaluate",
			expr:     `${threshold}`,
			vars:     map[string]any{"threshold": "0.5"},
			input:    nil,
			expected: true, // "0.5" is truthy (non-empty, not "false", not "0")
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condFn, err := p.parseSimpleExpression(tt.expr, tt.vars)
			require.NoError(t, err)

			result, err := condFn(context.Background(), tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
