package tokenizer

import (
	"strings"
	"testing"

	"github.com/pkoukk/tiktoken-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testBPELoader struct{}

func (testBPELoader) LoadTiktokenBpe(string) (map[string]int, error) {
	return map[string]int{
		"H": 1, "e": 2, "l": 3, "o": 4, ",": 5, " ": 6,
		"w": 7, "r": 8, "d": 9, "!": 10,
		"u": 11, "s": 12, "a": 13, "t": 14, "n": 15,
	}, nil
}

func useOfflineTiktokenLoader(t *testing.T) {
	t.Helper()
	tiktoken.SetBpeLoader(testBPELoader{})
}

func TestNewTiktokenTokenizer(t *testing.T) {
	tests := []struct {
		name              string
		model             string
		expectedEncoding  string
		expectedMaxTokens int
	}{
		{
			name:              "gpt-4o",
			model:             "gpt-4o",
			expectedEncoding:  "o200k_base",
			expectedMaxTokens: 128000,
		},
		{
			name:              "gpt-4",
			model:             "gpt-4",
			expectedEncoding:  "cl100k_base",
			expectedMaxTokens: 8192,
		},
		{
			name:              "gpt-3.5-turbo",
			model:             "gpt-3.5-turbo",
			expectedEncoding:  "cl100k_base",
			expectedMaxTokens: 16385,
		},
		{
			name:              "unknown model defaults to cl100k_base",
			model:             "unknown-model",
			expectedEncoding:  "cl100k_base",
			expectedMaxTokens: 8192,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok, err := NewTiktokenTokenizer(tt.model)
			require.NoError(t, err)
			require.NotNil(t, tok)
			assert.Equal(t, tt.expectedEncoding, tok.encoding)
			assert.Equal(t, tt.expectedMaxTokens, tok.maxTokens)
		})
	}
}



func requireTiktokenAvailable(t *testing.T, tok *TiktokenTokenizer) {
	t.Helper()
	if _, err := tok.CountTokens("health-check"); err != nil {
		if strings.Contains(err.Error(), "Forbidden") || strings.Contains(err.Error(), "403") {
			t.Skipf("tiktoken encoding download unavailable in current environment: %v", err)
		}
		require.NoError(t, err)
	}
}

func TestTiktokenTokenizer_PrefixMatch(t *testing.T) {
	// "gpt-4o-mini" should match "gpt-4o" prefix
	tok, err := NewTiktokenTokenizer("gpt-4o-mini")
	require.NoError(t, err)
	assert.Equal(t, "o200k_base", tok.encoding)
	assert.Equal(t, 128000, tok.maxTokens)
}

func TestTiktokenTokenizer_CountTokens(t *testing.T) {
	useOfflineTiktokenLoader(t)

	tok, err := NewTiktokenTokenizer("gpt-4")
	require.NoError(t, err)
	requireTiktokenAvailable(t, tok)


	count, err := tok.CountTokens("Hello, world!")
	require.NoError(t, err)
	assert.Greater(t, count, 0)
}

func TestTiktokenTokenizer_Encode_Decode(t *testing.T) {
	useOfflineTiktokenLoader(t)

	tok, err := NewTiktokenTokenizer("gpt-4")
	require.NoError(t, err)
	requireTiktokenAvailable(t, tok)


	text := "Hello, world!"
	tokens, err := tok.Encode(text)
	require.NoError(t, err)
	assert.NotEmpty(t, tokens)

	decoded, err := tok.Decode(tokens)
	require.NoError(t, err)
	assert.Equal(t, text, decoded)
}

func TestTiktokenTokenizer_CountMessages(t *testing.T) {
	useOfflineTiktokenLoader(t)

	tok, err := NewTiktokenTokenizer("gpt-4")
	require.NoError(t, err)
	requireTiktokenAvailable(t, tok)


	messages := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
	}

	count, err := tok.CountMessages(messages)
	require.NoError(t, err)
	// 2 messages * 4 overhead + tokens + 3 conversation-end
	assert.Greater(t, count, 11)
}

func TestTiktokenTokenizer_Name(t *testing.T) {
	tok, err := NewTiktokenTokenizer("gpt-4")
	require.NoError(t, err)
	requireTiktokenAvailable(t, tok)

	assert.Contains(t, tok.Name(), "tiktoken")
	assert.Contains(t, tok.Name(), "cl100k_base")
}

func TestTiktokenTokenizer_MaxTokens(t *testing.T) {
	tok, err := NewTiktokenTokenizer("gpt-4")
	require.NoError(t, err)
	requireTiktokenAvailable(t, tok)

	assert.Equal(t, 8192, tok.MaxTokens())
}

func TestRegisterOpenAITokenizers(t *testing.T) {
	RegisterOpenAITokenizers()

	for model := range modelEncodings {
		tok, err := GetTokenizer(model)
		require.NoError(t, err, "model %s should be registered", model)
		assert.NotNil(t, tok)
	}
}
