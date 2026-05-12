// Package tokenizer defines the shared tokenizer contract used by adapter
// packages to validate token counting and encode/decode consistency.
package tokenizer

// Tokenizer is the provider-neutral tokenizer contract for cross-package
// contract tests. It intentionally contains only primitive types so lower
// layers can depend on it without importing LLM, RAG, or framework types.
type Tokenizer interface {
	CountTokens(text string) (int, error)
	Encode(text string) ([]int, error)
	Decode(tokens []int) (string, error)
	MaxTokens() int
	Name() string
}

// ContractCases configures shared tokenizer contract test inputs.
type ContractCases struct {
	Texts []string
}
