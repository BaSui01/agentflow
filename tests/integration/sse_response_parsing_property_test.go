package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/deepseek"
	"github.com/BaSui01/agentflow/llm/providers/glm"
	"github.com/BaSui01/agentflow/llm/providers/grok"
	"github.com/BaSui01/agentflow/llm/providers/minimax"
	"github.com/BaSui01/agentflow/llm/providers/qwen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"pgregory.net/rapid"
)

// Feature: multi-provider-support, Property 14: SSE Response Parsing
// **Validates: Requirements 10.2, 10.3**
//
// This property test verifies that for any provider streaming response,
// when receiving SSE data lines starting with "data: ", the provider should
// parse the JSON content and emit corresponding StreamChunk messages.

// sseChunkData represents the data for a single SSE chunk
type sseChunkData struct {
	ID           string
	Model        string
	Content      string
	FinishReason string
}

// mockSSEServer creates a test server that returns SSE formatted responses
func mockSSEServer(chunks []sseChunkData) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		// Send each chunk as SSE data line
		for i, chunk := range chunks {
			sseData := map[string]any{
				"id":    chunk.ID,
				"model": chunk.Model,
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role":    "assistant",
							"content": chunk.Content,
						},
					},
				},
			}
			if i == len(chunks)-1 && chunk.FinishReason != "" {
				sseData["choices"].([]map[string]any)[0]["finish_reason"] = chunk.FinishReason
			}

			data, _ := json.Marshal(sseData)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}

		// Send [DONE] marker
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

// TestProperty14_SSEResponseParsing verifies that SSE data lines are correctly
// parsed into StreamChunk messages for all providers.
func TestProperty14_SSEResponseParsing(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate random SSE chunks
		numChunks := rapid.IntRange(1, 5).Draw(rt, "numChunks")
		chunks := make([]sseChunkData, numChunks)
		expectedContents := make([]string, numChunks)

		for i := range numChunks {
			content := rapid.StringMatching(`[a-zA-Z0-9 ]{1,20}`).Draw(rt, fmt.Sprintf("content_%d", i))
			chunks[i] = sseChunkData{
				ID:      fmt.Sprintf("chatcmpl-%s", rapid.StringMatching(`[a-z0-9]{8}`).Draw(rt, fmt.Sprintf("id_%d", i))),
				Model:   rapid.StringMatching(`[a-z0-9-]{3,15}`).Draw(rt, fmt.Sprintf("model_%d", i)),
				Content: content,
			}
			expectedContents[i] = content
			if i == numChunks-1 {
				chunks[i].FinishReason = "stop"
			}
		}

		// Select a random provider
		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		server := mockSSEServer(chunks)
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

		// Collect all chunks from stream
		var receivedContents []string
		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors for provider %s", providerName)
			if chunk.Delta.Content != "" {
				receivedContents = append(receivedContents, chunk.Delta.Content)
			}
		}

		// Verify all SSE data lines were parsed and emitted as StreamChunks
		require.Equal(t, len(expectedContents), len(receivedContents),
			"Should receive same number of chunks as sent for provider %s", providerName)

		for i, expected := range expectedContents {
			assert.Equal(t, expected, receivedContents[i],
				"Content at index %d should match for provider %s", i, providerName)
		}
	})
}

// TestProperty14_SSEResponseParsing_AllProviders provides table-driven tests
// to ensure minimum 100 iterations across all providers.
func TestProperty14_SSEResponseParsing_AllProviders(t *testing.T) {
	logger := zap.NewNop()

	type testCase struct {
		name         string
		providerName string
		chunks       []sseChunkData
	}

	var testCases []testCase

	providerList := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
	contentVariants := []string{
		"Hello", "World", "Test", "Response", "Chunk",
		"Data", "Stream", "Parse", "JSON", "SSE",
		"Message", "Content", "Delta", "Model", "Provider",
		"API", "Request", "Reply", "Token", "Complete",
	}

	// Generate 100+ test cases
	idx := 0
	for _, provider := range providerList {
		for i, content := range contentVariants {
			// Single chunk test
			testCases = append(testCases, testCase{
				name:         fmt.Sprintf("%s_single_%d", provider, idx),
				providerName: provider,
				chunks: []sseChunkData{
					{ID: fmt.Sprintf("id_%d", idx), Model: "test-model", Content: content, FinishReason: "stop"},
				},
			})
			idx++

			// Multi-chunk test (2 chunks)
			if i+1 < len(contentVariants) {
				testCases = append(testCases, testCase{
					name:         fmt.Sprintf("%s_multi_%d", provider, idx),
					providerName: provider,
					chunks: []sseChunkData{
						{ID: fmt.Sprintf("id_%d", idx), Model: "test-model", Content: content},
						{ID: fmt.Sprintf("id_%d", idx), Model: "test-model", Content: contentVariants[i+1], FinishReason: "stop"},
					},
				})
				idx++
			}
		}
	}

	// Ensure we have at least 100 test cases
	require.GreaterOrEqual(t, len(testCases), 100, "Should have at least 100 test cases")

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := mockSSEServer(tc.chunks)
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

			var receivedContents []string
			for chunk := range streamCh {
				require.Nil(t, chunk.Err, "Stream should not have errors")
				if chunk.Delta.Content != "" {
					receivedContents = append(receivedContents, chunk.Delta.Content)
				}
			}

			// Verify all chunks were parsed
			require.Equal(t, len(tc.chunks), len(receivedContents), "Should receive all chunks")
			for i, expected := range tc.chunks {
				assert.Equal(t, expected.Content, receivedContents[i], "Content should match at index %d", i)
			}
		})
	}
}

// TestProperty14_SSEResponseParsing_DataLineFormat verifies that only lines
// starting with "data: " are parsed as SSE events.
func TestProperty14_SSEResponseParsing_DataLineFormat(t *testing.T) {
	logger := zap.NewNop()

	// Create a server that sends various line formats
	mockServerWithFormats := func(validChunks []sseChunkData) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// Send some non-data lines (should be ignored)
			fmt.Fprintf(w, ": this is a comment\n\n")
			flusher.Flush()

			fmt.Fprintf(w, "event: message\n\n")
			flusher.Flush()

			// Send valid data lines
			for i, chunk := range validChunks {
				sseData := map[string]any{
					"id":    chunk.ID,
					"model": chunk.Model,
					"choices": []map[string]any{
						{
							"index": 0,
							"delta": map[string]any{
								"role":    "assistant",
								"content": chunk.Content,
							},
						},
					},
				}
				if i == len(validChunks)-1 {
					sseData["choices"].([]map[string]any)[0]["finish_reason"] = "stop"
				}

				data, _ := json.Marshal(sseData)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}

			// Send empty lines (should be ignored)
			fmt.Fprintf(w, "\n\n")
			flusher.Flush()

			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	rapid.Check(t, func(rt *rapid.T) {
		numChunks := rapid.IntRange(1, 3).Draw(rt, "numChunks")
		chunks := make([]sseChunkData, numChunks)

		for i := range numChunks {
			chunks[i] = sseChunkData{
				ID:      fmt.Sprintf("id_%d", i),
				Model:   "test-model",
				Content: rapid.StringMatching(`[a-zA-Z]{3,10}`).Draw(rt, fmt.Sprintf("content_%d", i)),
			}
		}

		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		server := mockServerWithFormats(chunks)
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

		var receivedContents []string
		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors")
			if chunk.Delta.Content != "" {
				receivedContents = append(receivedContents, chunk.Delta.Content)
			}
		}

		// Only valid data lines should be parsed
		require.Equal(t, len(chunks), len(receivedContents),
			"Should only receive chunks from valid data: lines for provider %s", providerName)
	})
}

// TestProperty14_SSEResponseParsing_JSONContent verifies that JSON content
// in SSE data lines is correctly parsed into StreamChunk fields.
func TestProperty14_SSEResponseParsing_JSONContent(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate random chunk data
		chunkID := rapid.StringMatching(`chatcmpl-[a-z0-9]{8}`).Draw(rt, "chunkID")
		chunkModel := rapid.StringMatching(`[a-z0-9-]{3,15}`).Draw(rt, "chunkModel")
		chunkContent := rapid.StringMatching(`[a-zA-Z0-9 ]{1,30}`).Draw(rt, "chunkContent")

		chunks := []sseChunkData{
			{ID: chunkID, Model: chunkModel, Content: chunkContent, FinishReason: "stop"},
		}

		providerIndex := rapid.IntRange(0, 4).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}
		providerName := providerNames[providerIndex]

		server := mockSSEServer(chunks)
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

		var receivedChunks []llm.StreamChunk
		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors")
			receivedChunks = append(receivedChunks, chunk)
		}

		// Verify JSON fields were correctly parsed
		require.Len(t, receivedChunks, 1, "Should receive exactly one chunk")
		chunk := receivedChunks[0]

		assert.Equal(t, chunkID, chunk.ID, "Chunk ID should be parsed from JSON for provider %s", providerName)
		assert.Equal(t, chunkModel, chunk.Model, "Chunk Model should be parsed from JSON for provider %s", providerName)
		assert.Equal(t, chunkContent, chunk.Delta.Content, "Chunk Content should be parsed from JSON for provider %s", providerName)
		assert.Equal(t, "stop", chunk.FinishReason, "FinishReason should be parsed from JSON for provider %s", providerName)
	})
}

// TestProperty14_SSEResponseParsing_WithToolCalls verifies that SSE responses
// containing tool calls are correctly parsed for OpenAI-compatible providers.
// Note: MiniMax uses XML-based tool calls and is tested separately.
func TestProperty14_SSEResponseParsing_WithToolCalls(t *testing.T) {
	logger := zap.NewNop()

	// Create a server that returns SSE with tool calls (OpenAI format)
	mockSSEServerWithToolCalls := func(toolCallID, toolName, toolArgs string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			sseData := map[string]any{
				"id":    "chatcmpl-test",
				"model": "test-model",
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   toolCallID,
									"type": "function",
									"function": map[string]any{
										"name":      toolName,
										"arguments": toolArgs,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}

			data, _ := json.Marshal(sseData)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	rapid.Check(t, func(rt *rapid.T) {
		toolCallID := rapid.StringMatching(`call_[a-z0-9]{8}`).Draw(rt, "toolCallID")
		toolName := rapid.StringMatching(`[a-z_]{3,15}`).Draw(rt, "toolName")
		toolArgs := fmt.Sprintf(`{"param": "%s"}`, rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "paramValue"))

		// Only test OpenAI-compatible providers (exclude minimax which uses XML format)
		providerIndex := rapid.IntRange(0, 3).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm"}
		providerName := providerNames[providerIndex]

		server := mockSSEServerWithToolCalls(toolCallID, toolName, toolArgs)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test"},
			},
			Tools: []llm.ToolSchema{
				{Name: toolName, Parameters: json.RawMessage(`{}`)},
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
		}

		require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

		var receivedToolCalls []llm.ToolCall
		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors")
			if len(chunk.Delta.ToolCalls) > 0 {
				receivedToolCalls = append(receivedToolCalls, chunk.Delta.ToolCalls...)
			}
		}

		// Verify tool calls were parsed from SSE
		require.Len(t, receivedToolCalls, 1, "Should receive one tool call for provider %s", providerName)
		assert.Equal(t, toolCallID, receivedToolCalls[0].ID, "Tool call ID should match for provider %s", providerName)
		assert.Equal(t, toolName, receivedToolCalls[0].Name, "Tool call name should match for provider %s", providerName)
	})
}

// TestProperty14_SSEResponseParsing_MiniMaxXMLToolCalls verifies that MiniMax's
// XML-based tool calls in SSE responses are correctly parsed.
func TestProperty14_SSEResponseParsing_MiniMaxXMLToolCalls(t *testing.T) {
	logger := zap.NewNop()

	// Create a server that returns SSE with MiniMax XML tool calls
	mockMiniMaxSSEServerWithToolCalls := func(toolName, toolArgs string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// MiniMax uses XML format for tool calls in content
			xmlContent := fmt.Sprintf("<tool_calls>\n{\"name\":\"%s\",\"arguments\":%s}\n</tool_calls>", toolName, toolArgs)

			sseData := map[string]any{
				"id":    "chatcmpl-test",
				"model": "test-model",
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role":    "assistant",
							"content": xmlContent,
						},
						"finish_reason": "tool_calls",
					},
				},
			}

			data, _ := json.Marshal(sseData)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	rapid.Check(t, func(rt *rapid.T) {
		toolName := rapid.StringMatching(`[a-z_]{3,15}`).Draw(rt, "toolName")
		toolArgs := fmt.Sprintf(`{"param": "%s"}`, rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "paramValue"))

		server := mockMiniMaxSSEServerWithToolCalls(toolName, toolArgs)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test"},
			},
			Tools: []llm.ToolSchema{
				{Name: toolName, Parameters: json.RawMessage(`{}`)},
			},
		}

		ctx := context.Background()
		cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
		p := minimax.NewMiniMaxProvider(cfg, logger)
		streamCh, err := p.Stream(ctx, req)

		require.NoError(t, err, "Stream() should not return error for MiniMax")

		var receivedToolCalls []llm.ToolCall
		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors")
			if len(chunk.Delta.ToolCalls) > 0 {
				receivedToolCalls = append(receivedToolCalls, chunk.Delta.ToolCalls...)
			}
		}

		// Verify tool calls were parsed from XML in SSE
		require.Len(t, receivedToolCalls, 1, "Should receive one tool call for MiniMax")
		assert.Equal(t, toolName, receivedToolCalls[0].Name, "Tool call name should match for MiniMax")
	})
}
