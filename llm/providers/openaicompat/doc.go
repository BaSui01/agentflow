// Package openaicompat provides a shared base implementation for all
// OpenAI-compatible LLM providers.
//
// Providers like DeepSeek, Qwen, GLM, Grok, Doubao, and MiniMax share the
// same API format (OpenAI Chat Completions). Instead of duplicating ~400 lines
// of HTTP handling, SSE parsing, message conversion, and error mapping in each
// provider, they embed openaicompat.Provider and only override what differs:
//
//   - Provider name and default model
//   - Base URL
//   - Custom headers (if any)
//   - Request hooks for provider-specific fields
//
// Usage:
//
//	p := openaicompat.New(openaicompat.Config{
//	    ProviderName:  "deepseek",
//	    APIKey:        cfg.APIKey,
//	    BaseURL:       "https://api.deepseek.com",
//	    DefaultModel:  "deepseek-chat",
//	    FallbackModel: "deepseek-chat",
//	    RequestHook: func(req *llm.ChatRequest, body *providers.OpenAICompatRequest) {
//	        if req.ReasoningMode == "thinking" {
//	            body.Model = "deepseek-reasoner"
//	        }
//	    },
//	}, logger)
package openaicompat
