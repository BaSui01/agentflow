package tokenizer

// RAGAdapter adapts the shared tokenizer contract to the RAG chunking tokenizer
// shape, which intentionally returns plain values instead of errors.
type RAGAdapter struct {
	inner Tokenizer
}

// NewRAGAdapter creates a RAG tokenizer adapter over the shared contract.
func NewRAGAdapter(inner Tokenizer) *RAGAdapter {
	return &RAGAdapter{inner: inner}
}

// CountTokens counts tokens and falls back to fallbackCount on tokenizer errors.
func (a *RAGAdapter) CountTokens(text string) int {
	count, err := a.inner.CountTokens(text)
	if err != nil {
		return fallbackCount(text)
	}
	return count
}

// Encode encodes text and falls back to a deterministic pseudo-token sequence
// on tokenizer errors.
func (a *RAGAdapter) Encode(text string) []int {
	tokens, err := a.inner.Encode(text)
	if err != nil {
		return fallbackEncode(text)
	}
	return tokens
}

func fallbackCount(text string) int {
	return len(text) / 4
}

func fallbackEncode(text string) []int {
	result := make([]int, fallbackCount(text))
	for i := range result {
		result[i] = i
	}
	return result
}
