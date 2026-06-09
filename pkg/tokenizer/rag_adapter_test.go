package tokenizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRAGAdapterAdaptsSharedTokenizerContract(t *testing.T) {
	adapter := NewRAGAdapter(fakeSharedTokenizer{})

	assert.Equal(t, 5, adapter.CountTokens("hello"))
	assert.Equal(t, []int{0, 0, 0, 0, 0}, adapter.Encode("hello"))
}

func TestRAGAdapterFallsBackOnTokenizerErrors(t *testing.T) {
	adapter := NewRAGAdapter(fakeSharedTokenizer{countErr: true})

	assert.Equal(t, 2, adapter.CountTokens("12345678"))
	assert.Equal(t, []int{0, 1}, adapter.Encode("12345678"))
}
