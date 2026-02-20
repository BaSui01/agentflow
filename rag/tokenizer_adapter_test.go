package rag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	lltok "github.com/BaSui01/agentflow/llm/tokenizer"
)

// --- hand-written mock for llm/tokenizer.Tokenizer ---

type mockLLMTokenizer struct {
	countResult  int
	countErr     error
	encodeResult []int
	encodeErr    error
}

func (m *mockLLMTokenizer) CountTokens(text string) (int, error) {
	return m.countResult, m.countErr
}

func (m *mockLLMTokenizer) CountMessages(_ []lltok.Message) (int, error) {
	return 0, nil
}

func (m *mockLLMTokenizer) Encode(text string) ([]int, error) {
	return m.encodeResult, m.encodeErr
}

func (m *mockLLMTokenizer) Decode(_ []int) (string, error) {
	return "", nil
}

func (m *mockLLMTokenizer) MaxTokens() int { return 4096 }
func (m *mockLLMTokenizer) Name() string   { return "mock" }

// --- Tests ---

func TestLLMTokenizerAdapter_ImplementsTokenizer(t *testing.T) {
	var _ Tokenizer = (*LLMTokenizerAdapter)(nil)
}

func TestLLMTokenizerAdapter_CountTokens(t *testing.T) {
	tests := []struct {
		name     string
		mock     *mockLLMTokenizer
		input    string
		expected int
	}{
		{
			name:     "successful count",
			mock:     &mockLLMTokenizer{countResult: 42},
			input:    "hello world",
			expected: 42,
		},
		{
			name:     "zero tokens for empty string",
			mock:     &mockLLMTokenizer{countResult: 0},
			input:    "",
			expected: 0,
		},
		{
			name:     "error falls back to len/4",
			mock:     &mockLLMTokenizer{countErr: assert.AnError},
			input:    "twelve chars", // len=12, 12/4=3
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewLLMTokenizerAdapter(tt.mock, zap.NewNop())
			got := adapter.CountTokens(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestLLMTokenizerAdapter_Encode(t *testing.T) {
	tests := []struct {
		name     string
		mock     *mockLLMTokenizer
		input    string
		expected []int
	}{
		{
			name:     "successful encode",
			mock:     &mockLLMTokenizer{encodeResult: []int{100, 200, 300}},
			input:    "hello world",
			expected: []int{100, 200, 300},
		},
		{
			name:     "empty input returns empty",
			mock:     &mockLLMTokenizer{encodeResult: []int{}},
			input:    "",
			expected: []int{},
		},
		{
			name:     "error falls back to pseudo tokens",
			mock:     &mockLLMTokenizer{encodeErr: assert.AnError},
			input:    "twelve chars", // len=12, 12/4=3 pseudo tokens
			expected: []int{0, 1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewLLMTokenizerAdapter(tt.mock, zap.NewNop())
			got := adapter.Encode(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestLLMTokenizerAdapter_NilLogger(t *testing.T) {
	mock := &mockLLMTokenizer{countResult: 10}
	adapter := NewLLMTokenizerAdapter(mock, nil)
	require.NotNil(t, adapter)
	assert.Equal(t, 10, adapter.CountTokens("test"))
}

func TestNewTiktokenAdapter(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		wantErr bool
	}{
		{
			name:    "known model gpt-4",
			model:   "gpt-4",
			wantErr: false,
		},
		{
			name:    "known model gpt-4o",
			model:   "gpt-4o",
			wantErr: false,
		},
		{
			name:    "unknown model falls back to default encoding",
			model:   "unknown-model",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok, err := NewTiktokenAdapter(tt.model, zap.NewNop())
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, tok)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tok)
			}
		})
	}
}

func TestNewTiktokenAdapter_CountTokens(t *testing.T) {
	tok, err := NewTiktokenAdapter("gpt-4", zap.NewNop())
	require.NoError(t, err)

	// "Hello, world!" should produce a reasonable token count (not len/4 estimate)
	count := tok.CountTokens("Hello, world!")
	assert.Greater(t, count, 0)
	// tiktoken should give a more precise count than len/4
	// "Hello, world!" is typically 4 tokens in cl100k_base
	assert.LessOrEqual(t, count, 10)
}

func TestNewTiktokenAdapter_Encode(t *testing.T) {
	tok, err := NewTiktokenAdapter("gpt-4", zap.NewNop())
	require.NoError(t, err)

	tokens := tok.Encode("Hello, world!")
	assert.Greater(t, len(tokens), 0)
	// Each token ID should be a positive integer
	for _, id := range tokens {
		assert.GreaterOrEqual(t, id, 0)
	}
}

// Integration test: use tiktoken adapter with DocumentChunker
func TestDocumentChunker_WithTiktokenAdapter(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tok, err := NewTiktokenAdapter("gpt-4", logger)
	require.NoError(t, err)

	config := ChunkingConfig{
		Strategy:     ChunkingRecursive,
		ChunkSize:    50,
		ChunkOverlap: 10,
		MinChunkSize: 5,
	}

	chunker := NewDocumentChunker(config, tok, logger)

	doc := Document{
		ID: "integration-test",
		Content: `Artificial intelligence has transformed many industries.
Machine learning models can now process natural language with remarkable accuracy.
Deep learning architectures continue to evolve rapidly.
The field of computer vision has seen tremendous progress in recent years.`,
	}

	chunks := chunker.ChunkDocument(doc)
	assert.Greater(t, len(chunks), 0)

	for i, chunk := range chunks {
		assert.NotEmpty(t, chunk.Content, "chunk %d should have content", i)
		assert.Greater(t, chunk.TokenCount, 0, "chunk %d should have positive token count", i)
		// Token count from tiktoken should be more accurate than len/4
		simpleEstimate := len(chunk.Content) / 4
		// Allow some variance, but they should be in the same ballpark
		assert.InDelta(t, simpleEstimate, chunk.TokenCount, float64(simpleEstimate)+20,
			"chunk %d token count should be reasonable", i)
	}
}
