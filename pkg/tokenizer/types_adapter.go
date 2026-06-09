package tokenizer

import (
	"github.com/BaSui01/agentflow/types"
)

// TypesAdapter adapts the shared tokenizer contract to types.Tokenizer.
type TypesAdapter struct {
	inner Tokenizer
}

// NewTypesAdapter creates a types.Tokenizer adapter over the shared contract.
func NewTypesAdapter(inner Tokenizer) *TypesAdapter {
	return &TypesAdapter{inner: inner}
}

// CountTokens counts tokens and falls back to zero on tokenizer errors.
func (a *TypesAdapter) CountTokens(text string) int {
	count, err := a.inner.CountTokens(text)
	if err != nil {
		return 0
	}
	return count
}

// CountMessageTokens counts tokens for a single framework message.
func (a *TypesAdapter) CountMessageTokens(msg types.Message) int {
	return a.CountTokens(msg.Content)
}

// CountMessagesTokens counts tokens for framework messages.
func (a *TypesAdapter) CountMessagesTokens(msgs []types.Message) int {
	total := 0
	for _, msg := range msgs {
		total += a.CountMessageTokens(msg)
	}
	return total
}

// EstimateToolTokens estimates tool schema tokens using shared text counting.
func (a *TypesAdapter) EstimateToolTokens(tools []types.ToolSchema) int {
	total := 0
	for _, tool := range tools {
		total += a.CountTokens(tool.Name)
		total += a.CountTokens(tool.Description)
		total += len(tool.Parameters) / 4
		total += 10
	}
	return total
}
