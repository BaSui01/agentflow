package tokenizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEstimatorTokenizer(t *testing.T) {
	tests := []struct {
		name              string
		model             string
		maxTokens         int
		expectedMaxTokens int
	}{
		{
			name:              "positive max tokens",
			model:             "test-model",
			maxTokens:         8192,
			expectedMaxTokens: 8192,
		},
		{
			name:              "zero max tokens defaults to 4096",
			model:             "test-model",
			maxTokens:         0,
			expectedMaxTokens: 4096,
		},
		{
			name:              "negative max tokens defaults to 4096",
			model:             "test-model",
			maxTokens:         -1,
			expectedMaxTokens: 4096,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEstimatorTokenizer(tt.model, tt.maxTokens)
			require.NotNil(t, e)
			assert.Equal(t, tt.expectedMaxTokens, e.MaxTokens())
			assert.Equal(t, "estimator", e.Name())
		})
	}
}

func TestEstimatorTokenizer_WithCharsPerToken(t *testing.T) {
	e := NewEstimatorTokenizer("test", 4096).WithCharsPerToken(3.0)
	assert.Equal(t, 3.0, e.charsPerToken)
}

func TestEstimatorTokenizer_CountTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		minCount int
		maxCount int
	}{
		{
			name:     "empty string",
			text:     "",
			minCount: 0,
			maxCount: 0,
		},
		{
			name:     "ascii text",
			text:     "Hello world this is a test",
			minCount: 1,
			maxCount: 100,
		},
		{
			name:     "CJK text",
			text:     "你好世界",
			minCount: 1,
			maxCount: 100,
		},
		{
			name:     "mixed CJK and ASCII",
			text:     "Hello 你好 World 世界",
			minCount: 1,
			maxCount: 100,
		},
		{
			name:     "single character",
			text:     "a",
			minCount: 1,
			maxCount: 1,
		},
	}

	e := NewEstimatorTokenizer("test", 4096)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := e.CountTokens(tt.text)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, count, tt.minCount)
			assert.LessOrEqual(t, count, tt.maxCount)
		})
	}
}

func TestEstimatorTokenizer_CountTokens_CJKMoreTokens(t *testing.T) {
	e := NewEstimatorTokenizer("test", 4096)

	// CJK characters should produce more tokens per character than ASCII
	cjkCount, err := e.CountTokens("你好世界测试")
	require.NoError(t, err)

	// 6 CJK chars / 1.5 = 4 tokens
	assert.Equal(t, 4, cjkCount)

	asciiCount, err := e.CountTokens("abcdef")
	require.NoError(t, err)

	// 6 ASCII chars / 4.0 = 1.5 -> 1 token
	assert.Equal(t, 1, asciiCount)

	assert.Greater(t, cjkCount, asciiCount)
}

func TestEstimatorTokenizer_CountMessages(t *testing.T) {
	e := NewEstimatorTokenizer("test", 4096)

	messages := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
	}

	count, err := e.CountMessages(messages)
	require.NoError(t, err)
	// Each message: tokens(content) + 4 overhead, plus 3 conversation-end
	assert.Greater(t, count, 0)

	// Empty messages should still have conversation-end overhead
	emptyCount, err := e.CountMessages(nil)
	require.NoError(t, err)
	assert.Equal(t, 3, emptyCount)
}

func TestEstimatorTokenizer_Encode(t *testing.T) {
	e := NewEstimatorTokenizer("test", 4096)

	tokens, err := e.Encode("Hello world")
	require.NoError(t, err)
	assert.NotEmpty(t, tokens)

	// Tokens should be sequential pseudo-IDs
	for i, tok := range tokens {
		assert.Equal(t, i, tok)
	}

	// Empty string
	tokens, err = e.Encode("")
	require.NoError(t, err)
	assert.Empty(t, tokens)
}

func TestEstimatorTokenizer_Decode(t *testing.T) {
	e := NewEstimatorTokenizer("test", 4096)

	_, err := e.Decode([]int{1, 2, 3})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not support decode")
}

func TestIsCJK(t *testing.T) {
	tests := []struct {
		name     string
		r        rune
		expected bool
	}{
		{"CJK Unified Ideograph", '中', true},
		{"CJK Extension A", '\u3400', true},
		{"CJK Symbols", '\u3000', true},
		{"Fullwidth Form", '\uFF01', true},
		{"ASCII letter", 'A', false},
		{"ASCII digit", '0', false},
		{"Latin Extended", '\u00E9', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isCJK(tt.r))
		})
	}
}

func TestRegisterAndGetTokenizer(t *testing.T) {
	// Clean up after test
	t.Cleanup(func() {
		modelTokenizersMu.Lock()
		delete(modelTokenizers, "test-register-model")
		modelTokenizersMu.Unlock()
	})

	e := NewEstimatorTokenizer("test-register-model", 4096)
	RegisterTokenizer("test-register-model", e)

	got, err := GetTokenizer("test-register-model")
	require.NoError(t, err)
	assert.Equal(t, e, got)
}

func TestGetTokenizer_NotFound(t *testing.T) {
	_, err := GetTokenizer("nonexistent-model-xyz-12345")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no tokenizer registered")
}

func TestGetTokenizer_PrefixMatch(t *testing.T) {
	t.Cleanup(func() {
		modelTokenizersMu.Lock()
		delete(modelTokenizers, "test-prefix")
		modelTokenizersMu.Unlock()
	})

	e := NewEstimatorTokenizer("test-prefix", 4096)
	RegisterTokenizer("test-prefix", e)

	got, err := GetTokenizer("test-prefix-extended")
	require.NoError(t, err)
	assert.Equal(t, e, got)
}

func TestGetTokenizerOrEstimator(t *testing.T) {
	// When no tokenizer registered, should return estimator
	tok := GetTokenizerOrEstimator("nonexistent-model-abc-99999")
	require.NotNil(t, tok)
	assert.Equal(t, "estimator", tok.Name())

	// When tokenizer is registered, should return it
	t.Cleanup(func() {
		modelTokenizersMu.Lock()
		delete(modelTokenizers, "test-or-estimator")
		modelTokenizersMu.Unlock()
	})

	e := NewEstimatorTokenizer("test-or-estimator", 8192)
	RegisterTokenizer("test-or-estimator", e)

	tok = GetTokenizerOrEstimator("test-or-estimator")
	assert.Equal(t, e, tok)
}
