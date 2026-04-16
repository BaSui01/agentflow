package llm

import "context"

// TokenCountProvider is an optional native-provider extension for counting request tokens
// using the provider's official transport or tokenizer APIs.
type TokenCountProvider interface {
	Provider
	CountTokens(ctx context.Context, req *ChatRequest) (*TokenCountResponse, error)
}

// TokenCountResponse is a normalized token-count result across native providers.
type TokenCountResponse struct {
	Model                    string `json:"model,omitempty"`
	InputTokens              int    `json:"input_tokens"`
	TotalTokens              int    `json:"total_tokens,omitempty"`
	CacheCreationInputTokens int    `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int    `json:"cache_read_input_tokens,omitempty"`
}
