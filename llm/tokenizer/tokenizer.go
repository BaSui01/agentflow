package tokenizer

import (
	"fmt"
	"sync"
)

// Tokenizer is the unified token counting interface.
type Tokenizer interface {
	// CountTokens returns the number of tokens in the given text.
	CountTokens(text string) (int, error)

	// CountMessages returns the total token count for a message list,
	// including per-message overhead (role markers, separators, etc.).
	CountMessages(messages []Message) (int, error)

	// Encode converts text into a list of token IDs.
	Encode(text string) ([]int, error)

	// Decode converts token IDs back into text.
	Decode(tokens []int) (string, error)

	// MaxTokens returns the model's maximum context length.
	MaxTokens() int

	// Name returns a human-readable tokenizer name.
	Name() string
}

// Message is a lightweight message struct used by the tokenizer package
// to avoid circular dependencies with the llm package.
type Message struct {
	Role    string
	Content string
}

// Global tokenizer registry.
var (
	modelTokenizers   = make(map[string]Tokenizer)
	modelTokenizersMu sync.RWMutex
)

// RegisterTokenizer registers a tokenizer for the given model name.
func RegisterTokenizer(model string, t Tokenizer) {
	modelTokenizersMu.Lock()
	defer modelTokenizersMu.Unlock()
	modelTokenizers[model] = t
}

// GetTokenizer returns the tokenizer registered for the given model.
// It also attempts prefix matching (e.g. "gpt-4o" matches "gpt-4o-mini").
func GetTokenizer(model string) (Tokenizer, error) {
	modelTokenizersMu.RLock()
	defer modelTokenizersMu.RUnlock()

	if t, ok := modelTokenizers[model]; ok {
		return t, nil
	}

	// Try prefix matching.
	for prefix, t := range modelTokenizers {
		if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
			return t, nil
		}
	}

	return nil, fmt.Errorf("no tokenizer registered for model: %s", model)
}

// GetTokenizerOrEstimator returns the registered tokenizer for the model,
// falling back to a generic estimator if none is registered.
func GetTokenizerOrEstimator(model string) Tokenizer {
	t, err := GetTokenizer(model)
	if err != nil {
		return NewEstimatorTokenizer(model, 0)
	}
	return t
}
