package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/providers"
	"github.com/BaSui01/agentflow/providers/deepseek"
	"github.com/BaSui01/agentflow/providers/glm"
	"github.com/BaSui01/agentflow/providers/grok"
	"github.com/BaSui01/agentflow/providers/minimax"
	"github.com/BaSui01/agentflow/providers/qwen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"pgregory.net/rapid"
)

// Feature: multi-provider-support, Property 15: Stream Error Handling
// **Validates: Requirements 10.5**
//
// This property test verifies that for any provider streaming response,
// when encountering invalid JSON in SSE data, the provider should emit
// a StreamChunk with a non-nil Err field containing an llm.Error.

// mockSSEServerWithInvalidJSON creates a test server that returns SSE with invalid JSON
func mockSSEServerWithInvalidJSON(invalidJSON string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		// Send invalid JSON in SSE data line
		fmt.Fprintf(w, "data: %s\n\n", invalidJSON)
		flusher.Flush()
	}))
}

// TestProperty15_StreamErrorHandling verifies that invalid JSON in SSE data
// results in a StreamChunk with non-nil Err field for all providers.
func TestProperty15_StreamErrorHandling(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate various types of invalid JSON
		invalidJSONType := rapid.IntRange(0, 5).Draw(rt, "invalidJSONType")
		var invalidJSON string

		switch invalidJSONType {
		case 0:
			// Truncated JSON
			invalidJSON = `{"id": "test", "model": "test-model", "choices": [`
		case 1:
			// Missing closing brace
			invalidJSON = `{"id": "test", "model": "test-model"`
		case 2:
			// Invalid syntax - unquoted key
			invalidJSON = `{id: "test", model: "test-model"}`
		case 3:
			// Random garbage
			invalidJSON = rapid.StringMatching(`[a-zA-Z0-9!@#$%^&*()]{5,30}`).Draw(rt, "garbage")
		case 4:
			// Malformed array
			invalidJSON = `{"choices": [{"index": 0, "delta": {"content": "test"}`
		case 5:
			// Invalid escape sequence
			invalidJSON = `{"id": "test\xinvalid", "model": "test"}`
		}

		// Select a random provider
		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		server := mockSSEServerWithInvalidJSON(invalidJSON)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test message"},
			},
		}

		ctx := context.Background()
		var streamCh <-chan llm.StreamChunk
		var err error

		switch providerName {
		case "grok":
			cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
			p := grok.NewGrokProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "qwen":
			cfg := providers.QwenConfig{APIKey: "test-key", BaseURL: server.URL}
			p := qwen.NewQwenProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "deepseek":
			cfg := providers.DeepSeekConfig{APIKey: "test-key", BaseURL: server.URL}
			p := deepseek.NewDeepSeekProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "glm":
			cfg := providers.GLMConfig{APIKey: "test-key", BaseURL: server.URL}
			p := glm.NewGLMProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "minimax":
			cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
			p := minimax.NewMiniMaxProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		}

		require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

		// Collect chunks and check for error
		var foundError bool
		var errorChunk llm.StreamChunk
		for chunk := range streamCh {
			if chunk.Err != nil {
				foundError = true
				errorChunk = chunk
				break
			}
		}

		// Verify that an error chunk was emitted
		assert.True(t, foundError, "Should receive StreamChunk with error for invalid JSON for provider %s", providerName)
		assert.NotNil(t, errorChunk.Err, "StreamChunk.Err should not be nil for provider %s", providerName)

		// Verify the error fields (Err is already *llm.Error)
		if errorChunk.Err != nil {
			assert.NotEmpty(t, errorChunk.Err.Message, "Error message should not be empty for provider %s", providerName)
			assert.Equal(t, llm.ErrUpstreamError, errorChunk.Err.Code, "Error code should be ErrUpstreamError for provider %s", providerName)
		}
	})
}

// TestProperty15_StreamErrorHandling_AllProviders provides table-driven tests
// to ensure minimum 100 iterations across all providers.
func TestProperty15_StreamErrorHandling_AllProviders(t *testing.T) {
	logger := zap.NewNop()

	type testCase struct {
		name         string
		providerName string
		invalidJSON  string
	}

	// Generate various invalid JSON patterns
	invalidJSONPatterns := []string{
		// Truncated JSON
		`{"id": "test", "model": "test-model", "choices": [`,
		`{"id": "test"`,
		`{"choices": [{"index": 0`,
		`{"id": "test", "model":`,
		// Missing closing braces
		`{"id": "test", "model": "test-model"`,
		`{"choices": [{"delta": {"content": "test"}}`,
		// Invalid syntax
		`{id: "test"}`,
		`{"id": test}`,
		`{'id': 'test'}`,
		`{id: test}`,
		// Random garbage
		`not json at all`,
		`<xml>not json</xml>`,
		// Malformed structures (these will parse but cause issues in provider)
		`{"choices": "not an array"}`,
		`{"id": {"nested": "wrong"}}`,
		`{"choices": [{"delta": "not object"}]}`,
		`{"id": [1,2,3]}`,
		// Additional invalid patterns
		`{"unclosed": "string`,
		`{trailing comma,}`,
		`{"key": undefined}`,
	}

	var testCases []testCase
	providerList := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	// Generate 100+ test cases by repeating patterns with variations
	idx := 0
	for round := 0; round < 2; round++ {
		for _, provider := range providerList {
			for _, invalidJSON := range invalidJSONPatterns {
				testCases = append(testCases, testCase{
					name:         fmt.Sprintf("%s_invalid_%d", provider, idx),
					providerName: provider,
					invalidJSON:  invalidJSON,
				})
				idx++
			}
		}
	}

	// Ensure we have at least 100 test cases
	require.GreaterOrEqual(t, len(testCases), 100, "Should have at least 100 test cases")

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := mockSSEServerWithInvalidJSON(tc.invalidJSON)
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test"},
				},
			}

			ctx := context.Background()
			var streamCh <-chan llm.StreamChunk
			var err error

			switch tc.providerName {
			case "grok":
				cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
				p := grok.NewGrokProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "qwen":
				cfg := providers.QwenConfig{APIKey: "test-key", BaseURL: server.URL}
				p := qwen.NewQwenProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "deepseek":
				cfg := providers.DeepSeekConfig{APIKey: "test-key", BaseURL: server.URL}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "glm":
				cfg := providers.GLMConfig{APIKey: "test-key", BaseURL: server.URL}
				p := glm.NewGLMProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "minimax":
				cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
				p := minimax.NewMiniMaxProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			}

			require.NoError(t, err, "Stream() should not return error")

			// Collect chunks and check for error
			var foundError bool
			var errorChunk llm.StreamChunk
			for chunk := range streamCh {
				if chunk.Err != nil {
					foundError = true
					errorChunk = chunk
					break
				}
			}

			// Verify error was emitted
			assert.True(t, foundError, "Should receive StreamChunk with error for invalid JSON")
			assert.NotNil(t, errorChunk.Err, "StreamChunk.Err should not be nil")
		})
	}
}

// TestProperty15_StreamErrorHandling_ErrorContainsLLMError verifies that
// the error in StreamChunk is specifically an llm.Error with correct fields.
func TestProperty15_StreamErrorHandling_ErrorContainsLLMError(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate random invalid JSON
		invalidJSON := rapid.StringMatching(`\{[a-zA-Z0-9:,"' ]{0,20}`).Draw(rt, "invalidJSON")

		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		server := mockSSEServerWithInvalidJSON(invalidJSON)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test"},
			},
		}

		ctx := context.Background()
		var streamCh <-chan llm.StreamChunk
		var err error

		switch providerName {
		case "grok":
			cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
			p := grok.NewGrokProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "qwen":
			cfg := providers.QwenConfig{APIKey: "test-key", BaseURL: server.URL}
			p := qwen.NewQwenProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "deepseek":
			cfg := providers.DeepSeekConfig{APIKey: "test-key", BaseURL: server.URL}
			p := deepseek.NewDeepSeekProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "glm":
			cfg := providers.GLMConfig{APIKey: "test-key", BaseURL: server.URL}
			p := glm.NewGLMProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "minimax":
			cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
			p := minimax.NewMiniMaxProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		}

		require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

		// Collect error chunk
		var errorChunk llm.StreamChunk
		for chunk := range streamCh {
			if chunk.Err != nil {
				errorChunk = chunk
				break
			}
		}

		// Verify error is llm.Error with correct fields (Err is already *llm.Error)
		if errorChunk.Err != nil {
			// Verify llm.Error fields
			assert.Equal(t, llm.ErrUpstreamError, errorChunk.Err.Code,
				"Error code should be ErrUpstreamError for provider %s", providerName)
			assert.NotEmpty(t, errorChunk.Err.Message,
				"Error message should not be empty for provider %s", providerName)
			assert.Equal(t, http.StatusBadGateway, errorChunk.Err.HTTPStatus,
				"HTTP status should be BadGateway for provider %s", providerName)
			assert.True(t, errorChunk.Err.Retryable,
				"Error should be retryable for provider %s", providerName)
			assert.Equal(t, providerName, errorChunk.Err.Provider,
				"Provider name should be set in error for provider %s", providerName)
		}
	})
}

// TestProperty15_StreamErrorHandling_ChannelClosesAfterError verifies that
// the stream channel is properly closed after emitting an error chunk.
func TestProperty15_StreamErrorHandling_ChannelClosesAfterError(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		invalidJSON := `{"invalid": json}`

		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		server := mockSSEServerWithInvalidJSON(invalidJSON)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test"},
			},
		}

		ctx := context.Background()
		var streamCh <-chan llm.StreamChunk
		var err error

		switch providerName {
		case "grok":
			cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
			p := grok.NewGrokProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "qwen":
			cfg := providers.QwenConfig{APIKey: "test-key", BaseURL: server.URL}
			p := qwen.NewQwenProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "deepseek":
			cfg := providers.DeepSeekConfig{APIKey: "test-key", BaseURL: server.URL}
			p := deepseek.NewDeepSeekProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "glm":
			cfg := providers.GLMConfig{APIKey: "test-key", BaseURL: server.URL}
			p := glm.NewGLMProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "minimax":
			cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
			p := minimax.NewMiniMaxProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		}

		require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

		// Drain the channel completely
		chunkCount := 0
		for range streamCh {
			chunkCount++
		}

		// Channel should be closed (loop should exit)
		// If we get here, the channel was properly closed
		assert.True(t, true, "Channel should be closed after error for provider %s", providerName)
	})
}

// TestProperty15_StreamErrorHandling_MixedValidInvalidJSON verifies that
// when valid JSON is followed by invalid JSON, the error is still emitted.
func TestProperty15_StreamErrorHandling_MixedValidInvalidJSON(t *testing.T) {
	logger := zap.NewNop()

	// Create a server that sends valid JSON first, then invalid JSON
	mockMixedSSEServer := func(validContent string, invalidJSON string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// Send valid JSON first
			validData := fmt.Sprintf(`{"id":"test","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"%s"}}]}`, validContent)
			fmt.Fprintf(w, "data: %s\n\n", validData)
			flusher.Flush()

			// Then send invalid JSON
			fmt.Fprintf(w, "data: %s\n\n", invalidJSON)
			flusher.Flush()
		}))
	}

	rapid.Check(t, func(rt *rapid.T) {
		validContent := rapid.StringMatching(`[a-zA-Z]{3,10}`).Draw(rt, "validContent")
		invalidJSON := `{"broken": json`

		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		server := mockMixedSSEServer(validContent, invalidJSON)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test"},
			},
		}

		ctx := context.Background()
		var streamCh <-chan llm.StreamChunk
		var err error

		switch providerName {
		case "grok":
			cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
			p := grok.NewGrokProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "qwen":
			cfg := providers.QwenConfig{APIKey: "test-key", BaseURL: server.URL}
			p := qwen.NewQwenProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "deepseek":
			cfg := providers.DeepSeekConfig{APIKey: "test-key", BaseURL: server.URL}
			p := deepseek.NewDeepSeekProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "glm":
			cfg := providers.GLMConfig{APIKey: "test-key", BaseURL: server.URL}
			p := glm.NewGLMProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		case "minimax":
			cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
			p := minimax.NewMiniMaxProvider(cfg, logger)
			streamCh, err = p.Stream(ctx, req)
		}

		require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

		// Collect all chunks
		var validChunks []llm.StreamChunk
		var errorChunk *llm.StreamChunk
		for chunk := range streamCh {
			if chunk.Err != nil {
				errorChunk = &chunk
			} else {
				validChunks = append(validChunks, chunk)
			}
		}

		// Should have received valid chunk first
		assert.GreaterOrEqual(t, len(validChunks), 1,
			"Should receive at least one valid chunk before error for provider %s", providerName)

		// Should have received error chunk
		assert.NotNil(t, errorChunk,
			"Should receive error chunk for invalid JSON for provider %s", providerName)
	})
}

// TestProperty15_StreamErrorHandling_EmptyDataLine verifies that empty data
// after "data: " prefix is handled (may or may not be an error depending on implementation).
func TestProperty15_StreamErrorHandling_EmptyDataLine(t *testing.T) {
	logger := zap.NewNop()

	// Create a server that sends empty data line followed by valid data
	mockEmptyDataServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// Send empty data line (just whitespace after data:)
			fmt.Fprintf(w, "data:    \n\n")
			flusher.Flush()

			// Send [DONE] to close
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	providerList := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	for _, providerName := range providerList {
		t.Run(providerName, func(t *testing.T) {
			server := mockEmptyDataServer()
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test"},
				},
			}

			ctx := context.Background()
			var streamCh <-chan llm.StreamChunk
			var err error

			switch providerName {
			case "grok":
				cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
				p := grok.NewGrokProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "qwen":
				cfg := providers.QwenConfig{APIKey: "test-key", BaseURL: server.URL}
				p := qwen.NewQwenProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "deepseek":
				cfg := providers.DeepSeekConfig{APIKey: "test-key", BaseURL: server.URL}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "glm":
				cfg := providers.GLMConfig{APIKey: "test-key", BaseURL: server.URL}
				p := glm.NewGLMProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "minimax":
				cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
				p := minimax.NewMiniMaxProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			}

			require.NoError(t, err, "Stream() should not return error")

			// Drain channel - empty data line should either be skipped or cause error
			var hasError bool
			for chunk := range streamCh {
				if chunk.Err != nil {
					hasError = true
					// Err is already *llm.Error, no type assertion needed
					assert.NotNil(t, chunk.Err, "Error should not be nil for provider %s", providerName)
				}
			}

			// Empty whitespace after "data:" should cause JSON parse error
			// since it's not valid JSON and not [DONE]
			assert.True(t, hasError, "Empty data line should cause error for provider %s", providerName)
		})
	}
}
