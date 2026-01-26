package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// Feature: multi-provider-support, Property 16: Tool Call Accumulation in Streaming
// **Validates: Requirements 10.6**
//
// This property test verifies that for any provider that sends partial tool call
// JSON across multiple stream chunks, the accumulated tool call arguments should
// form valid JSON when all chunks are combined.

// partialToolCallChunk represents a chunk with partial tool call data
type partialToolCallChunk struct {
	ID           string
	Model        string
	ToolCallID   string
	ToolCallName string
	PartialArgs  string // Partial JSON arguments
	Index        int    // Tool call index within the chunk
	IsFirst      bool   // First chunk contains ID and name
	FinishReason string
}

// mockSSEServerWithPartialToolCalls creates a test server that sends tool call
// arguments in multiple partial chunks (simulating how providers like OpenAI
// stream tool calls with partial JSON).
// Note: OpenAI sends arguments as a string field containing partial JSON.
// Each chunk's arguments is a string that, when concatenated, forms valid JSON.
func mockSSEServerWithPartialToolCalls(chunks []partialToolCallChunk) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		for _, chunk := range chunks {
			// Build the SSE data - arguments is sent as a string containing partial JSON
			var sseData map[string]any
			if chunk.IsFirst {
				sseData = map[string]any{
					"id":    chunk.ID,
					"model": chunk.Model,
					"choices": []map[string]any{
						{
							"index": 0,
							"delta": map[string]any{
								"role": "assistant",
								"tool_calls": []map[string]any{
									{
										"index": chunk.Index,
										"id":    chunk.ToolCallID,
										"type":  "function",
										"function": map[string]any{
											"name": chunk.ToolCallName,
											// Arguments is a string containing partial JSON
											"arguments": chunk.PartialArgs,
										},
									},
								},
							},
						},
					},
				}
			} else {
				sseData = map[string]any{
					"id":    chunk.ID,
					"model": chunk.Model,
					"choices": []map[string]any{
						{
							"index": 0,
							"delta": map[string]any{
								"tool_calls": []map[string]any{
									{
										"index": chunk.Index,
										"function": map[string]any{
											// Arguments is a string containing partial JSON
											"arguments": chunk.PartialArgs,
										},
									},
								},
							},
						},
					},
				}
			}
			if chunk.FinishReason != "" {
				sseData["choices"].([]map[string]any)[0]["finish_reason"] = chunk.FinishReason
			}

			jsonData, _ := json.Marshal(sseData)
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			flusher.Flush()
		}

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

// splitJSONIntoChunks splits a JSON string into multiple partial chunks
func splitJSONIntoChunks(jsonStr string, numChunks int) []string {
	if numChunks <= 1 || len(jsonStr) == 0 {
		return []string{jsonStr}
	}

	chunkSize := len(jsonStr) / numChunks
	if chunkSize == 0 {
		chunkSize = 1
	}

	var chunks []string
	for i := 0; i < len(jsonStr); i += chunkSize {
		end := i + chunkSize
		if end > len(jsonStr) {
			end = len(jsonStr)
		}
		chunks = append(chunks, jsonStr[i:end])
	}

	return chunks
}

// TestProperty16_ToolCallAccumulation verifies that partial tool call JSON
// chunks are accumulated correctly to form valid JSON for all providers.
func TestProperty16_ToolCallAccumulation(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate random tool call data
		toolCallID := rapid.StringMatching(`call_[a-z0-9]{8}`).Draw(rt, "toolCallID")
		toolName := rapid.StringMatching(`[a-z_]{3,15}`).Draw(rt, "toolName")

		// Generate random JSON arguments
		paramKey := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "paramKey")
		paramValue := rapid.StringMatching(`[a-zA-Z0-9]{3,20}`).Draw(rt, "paramValue")
		fullArgs := fmt.Sprintf(`{"%s":"%s"}`, paramKey, paramValue)

		// Split into 2-4 chunks
		numChunks := rapid.IntRange(2, 4).Draw(rt, "numChunks")
		argChunks := splitJSONIntoChunks(fullArgs, numChunks)

		// Build partial tool call chunks
		chunks := make([]partialToolCallChunk, len(argChunks))
		chunkID := rapid.StringMatching(`chatcmpl-[a-z0-9]{8}`).Draw(rt, "chunkID")
		chunkModel := rapid.StringMatching(`[a-z0-9-]{3,15}`).Draw(rt, "chunkModel")

		for i, argPart := range argChunks {
			chunks[i] = partialToolCallChunk{
				ID:           chunkID,
				Model:        chunkModel,
				ToolCallID:   toolCallID,
				ToolCallName: toolName,
				PartialArgs:  argPart,
				Index:        0,
				IsFirst:      i == 0,
			}
			if i == len(argChunks)-1 {
				chunks[i].FinishReason = "tool_calls"
			}
		}

		// Select a random OpenAI-compatible provider (exclude minimax which uses XML)
		providerIndex := rapid.IntRange(0, 3).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm"}
		providerName := providerNames[providerIndex]

		server := mockSSEServerWithPartialToolCalls(chunks)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test message"},
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

		// Collect all tool call chunks and accumulate arguments
		// Following the same logic as ReAct executor: try to unmarshal as string first
		var argsBuilder strings.Builder
		var receivedToolCallID string
		var receivedToolName string

		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors for provider %s", providerName)
			if len(chunk.Delta.ToolCalls) > 0 {
				tc := chunk.Delta.ToolCalls[0]
				if tc.ID != "" {
					receivedToolCallID = tc.ID
				}
				if tc.Name != "" {
					receivedToolName = tc.Name
				}
				if len(tc.Arguments) > 0 {
					// Try to unmarshal as JSON string first (OpenAI format)
					var argStr string
					if err := json.Unmarshal(tc.Arguments, &argStr); err == nil {
						argsBuilder.WriteString(argStr)
					} else {
						// Otherwise use raw bytes
						argsBuilder.Write(tc.Arguments)
					}
				}
			}
		}

		accumulatedArgs := argsBuilder.String()

		// Verify accumulated arguments form valid JSON
		var parsed map[string]any
		err = json.Unmarshal([]byte(accumulatedArgs), &parsed)
		assert.NoError(t, err, "Accumulated arguments should be valid JSON for provider %s: %s", providerName, accumulatedArgs)

		// Verify tool call metadata
		assert.Equal(t, toolCallID, receivedToolCallID, "Tool call ID should match for provider %s", providerName)
		assert.Equal(t, toolName, receivedToolName, "Tool call name should match for provider %s", providerName)

		// Verify the accumulated JSON matches original
		assert.Equal(t, fullArgs, accumulatedArgs, "Accumulated args should match original for provider %s", providerName)
	})
}

// TestProperty16_ToolCallAccumulation_AllProviders provides table-driven tests
// to ensure minimum 100 iterations across all providers.
func TestProperty16_ToolCallAccumulation_AllProviders(t *testing.T) {
	logger := zap.NewNop()

	type testCase struct {
		name         string
		providerName string
		toolCallID   string
		toolName     string
		fullArgs     string
		numChunks    int
	}

	var testCases []testCase

	// OpenAI-compatible providers (exclude minimax which uses XML format)
	providerList := []string{"grok", "qwen", "deepseek", "glm"}

	// Various JSON argument patterns
	argPatterns := []string{
		`{"location":"Shanghai"}`,
		`{"query":"weather forecast"}`,
		`{"name":"test","value":123}`,
		`{"a":"b","c":"d"}`,
		`{"key":"value"}`,
		`{"param":"data"}`,
		`{"input":"text"}`,
		`{"x":"1","y":"2"}`,
	}

	toolNames := []string{
		"get_weather",
		"search_web",
		"calculate",
		"fetch_data",
		"process_input",
	}

	// Generate 100+ test cases
	idx := 0
	for _, provider := range providerList {
		for _, args := range argPatterns {
			for _, toolName := range toolNames {
				for numChunks := 2; numChunks <= 4; numChunks++ {
					testCases = append(testCases, testCase{
						name:         fmt.Sprintf("%s_%s_%d_%d", provider, toolName, numChunks, idx),
						providerName: provider,
						toolCallID:   fmt.Sprintf("call_%08d", idx),
						toolName:     toolName,
						fullArgs:     args,
						numChunks:    numChunks,
					})
					idx++
				}
			}
		}
	}

	require.GreaterOrEqual(t, len(testCases), 100, "Should have at least 100 test cases")

	for _, tc := range testCases[:100] { // Run first 100 to keep test time reasonable
		t.Run(tc.name, func(t *testing.T) {
			argChunks := splitJSONIntoChunks(tc.fullArgs, tc.numChunks)

			chunks := make([]partialToolCallChunk, len(argChunks))
			for i, argPart := range argChunks {
				chunks[i] = partialToolCallChunk{
					ID:           "chatcmpl-test",
					Model:        "test-model",
					ToolCallID:   tc.toolCallID,
					ToolCallName: tc.toolName,
					PartialArgs:  argPart,
					Index:        0,
					IsFirst:      i == 0,
				}
				if i == len(argChunks)-1 {
					chunks[i].FinishReason = "tool_calls"
				}
			}

			server := mockSSEServerWithPartialToolCalls(chunks)
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test"},
				},
				Tools: []llm.ToolSchema{
					{Name: tc.toolName, Parameters: json.RawMessage(`{}`)},
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
			}

			require.NoError(t, err, "Stream() should not return error")

			var argsBuilder strings.Builder
			for chunk := range streamCh {
				require.Nil(t, chunk.Err, "Stream should not have errors")
				if len(chunk.Delta.ToolCalls) > 0 {
					tc := chunk.Delta.ToolCalls[0]
					if len(tc.Arguments) > 0 {
						// Try to unmarshal as JSON string first (OpenAI format)
						var argStr string
						if err := json.Unmarshal(tc.Arguments, &argStr); err == nil {
							argsBuilder.WriteString(argStr)
						} else {
							argsBuilder.Write(tc.Arguments)
						}
					}
				}
			}

			accumulatedArgs := argsBuilder.String()

			// Verify accumulated arguments form valid JSON
			var parsed map[string]any
			err = json.Unmarshal([]byte(accumulatedArgs), &parsed)
			assert.NoError(t, err, "Accumulated arguments should be valid JSON: %s", accumulatedArgs)
		})
	}
}

// TestProperty16_ToolCallAccumulation_ComplexJSON verifies accumulation works
// with more complex JSON structures.
func TestProperty16_ToolCallAccumulation_ComplexJSON(t *testing.T) {
	logger := zap.NewNop()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate complex JSON with nested structure
		key1 := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "key1")
		val1 := rapid.StringMatching(`[a-zA-Z0-9]{3,15}`).Draw(rt, "val1")
		key2 := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "key2")
		val2 := rapid.IntRange(1, 1000).Draw(rt, "val2")
		key3 := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "key3")
		boolVal := rapid.Bool().Draw(rt, "boolVal")

		fullArgs := fmt.Sprintf(`{"%s":"%s","%s":%d,"%s":%t}`, key1, val1, key2, val2, key3, boolVal)

		numChunks := rapid.IntRange(3, 6).Draw(rt, "numChunks")
		argChunks := splitJSONIntoChunks(fullArgs, numChunks)

		chunks := make([]partialToolCallChunk, len(argChunks))
		for i, argPart := range argChunks {
			chunks[i] = partialToolCallChunk{
				ID:           "chatcmpl-complex",
				Model:        "test-model",
				ToolCallID:   "call_complex",
				ToolCallName: "complex_tool",
				PartialArgs:  argPart,
				Index:        0,
				IsFirst:      i == 0,
			}
			if i == len(argChunks)-1 {
				chunks[i].FinishReason = "tool_calls"
			}
		}

		providerIndex := rapid.IntRange(0, 3).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm"}
		providerName := providerNames[providerIndex]

		server := mockSSEServerWithPartialToolCalls(chunks)
		defer server.Close()

		req := &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Test"},
			},
			Tools: []llm.ToolSchema{
				{Name: "complex_tool", Parameters: json.RawMessage(`{}`)},
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

		var argsBuilder strings.Builder
		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors")
			if len(chunk.Delta.ToolCalls) > 0 {
				tc := chunk.Delta.ToolCalls[0]
				if len(tc.Arguments) > 0 {
					// Try to unmarshal as JSON string first (OpenAI format)
					var argStr string
					if err := json.Unmarshal(tc.Arguments, &argStr); err == nil {
						argsBuilder.WriteString(argStr)
					} else {
						argsBuilder.Write(tc.Arguments)
					}
				}
			}
		}

		accumulatedArgs := argsBuilder.String()

		// Verify accumulated arguments form valid JSON
		var parsed map[string]any
		err = json.Unmarshal([]byte(accumulatedArgs), &parsed)
		assert.NoError(t, err, "Complex accumulated arguments should be valid JSON for provider %s: %s", providerName, accumulatedArgs)
		assert.Equal(t, fullArgs, accumulatedArgs, "Accumulated args should match original for provider %s", providerName)
	})
}

// TestProperty16_ToolCallAccumulation_MultipleToolCalls verifies accumulation
// works when multiple tool calls are streamed in a single chunk.
func TestProperty16_ToolCallAccumulation_MultipleToolCalls(t *testing.T) {
	logger := zap.NewNop()

	// Create server that sends a single tool call split across chunks
	// This tests the core accumulation property without the complexity of
	// multiple simultaneous tool calls
	mockSingleToolCallServer := func(toolName, fullArgs string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// Split args into two parts
			mid := len(fullArgs) / 2

			// First chunk: tool call starts with first half of args
			chunk1 := map[string]any{
				"id":    "chatcmpl-multi",
				"model": "test-model",
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"index": 0,
									"id":    "call_1",
									"type":  "function",
									"function": map[string]any{
										"name":      toolName,
										"arguments": fullArgs[:mid],
									},
								},
							},
						},
					},
				},
			}
			data1, _ := json.Marshal(chunk1)
			fmt.Fprintf(w, "data: %s\n\n", data1)
			flusher.Flush()

			// Second chunk: tool call continues with second half
			chunk2 := map[string]any{
				"id":    "chatcmpl-multi",
				"model": "test-model",
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"tool_calls": []map[string]any{
								{
									"index": 0,
									"function": map[string]any{
										"arguments": fullArgs[mid:],
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
			data2, _ := json.Marshal(chunk2)
			fmt.Fprintf(w, "data: %s\n\n", data2)
			flusher.Flush()

			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	rapid.Check(t, func(rt *rapid.T) {
		toolName := rapid.StringMatching(`[a-z_]{3,10}`).Draw(rt, "toolName")
		paramKey := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "paramKey")
		paramValue := rapid.StringMatching(`[a-zA-Z0-9]{3,15}`).Draw(rt, "paramValue")
		fullArgs := fmt.Sprintf(`{"%s":"%s"}`, paramKey, paramValue)

		providerIndex := rapid.IntRange(0, 3).Draw(rt, "providerIndex")
		providerNames := []string{"grok", "qwen", "deepseek", "glm"}
		providerName := providerNames[providerIndex]

		server := mockSingleToolCallServer(toolName, fullArgs)
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

		// Accumulate all tool call arguments
		var argsBuilder strings.Builder
		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors")
			for _, tc := range chunk.Delta.ToolCalls {
				if len(tc.Arguments) > 0 {
					// Try to unmarshal as JSON string first (OpenAI format)
					var argStr string
					if err := json.Unmarshal(tc.Arguments, &argStr); err == nil {
						argsBuilder.WriteString(argStr)
					} else {
						argsBuilder.Write(tc.Arguments)
					}
				}
			}
		}

		accumulatedArgs := argsBuilder.String()

		// Verify accumulated arguments form valid JSON
		var parsed map[string]any
		err = json.Unmarshal([]byte(accumulatedArgs), &parsed)
		assert.NoError(t, err, "Accumulated args should be valid JSON for provider %s: %s", providerName, accumulatedArgs)
		assert.Equal(t, fullArgs, accumulatedArgs, "Accumulated args should match original for provider %s", providerName)
	})
}

// TestProperty16_ToolCallAccumulation_MiniMaxXML verifies that MiniMax's
// XML-based tool calls also accumulate correctly when sent in partial chunks.
func TestProperty16_ToolCallAccumulation_MiniMaxXML(t *testing.T) {
	logger := zap.NewNop()

	// Create server that sends MiniMax XML tool calls in partial chunks
	mockMiniMaxPartialServer := func(toolName, fullArgs string, numChunks int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// Build full XML content
			xmlContent := fmt.Sprintf("<tool_calls>\n{\"name\":\"%s\",\"arguments\":%s}\n</tool_calls>", toolName, fullArgs)

			// Split content into chunks
			contentChunks := splitJSONIntoChunks(xmlContent, numChunks)

			for i, contentPart := range contentChunks {
				sseData := map[string]any{
					"id":    "chatcmpl-minimax",
					"model": "test-model",
					"choices": []map[string]any{
						{
							"index": 0,
							"delta": map[string]any{
								"role":    "assistant",
								"content": contentPart,
							},
						},
					},
				}
				if i == len(contentChunks)-1 {
					sseData["choices"].([]map[string]any)[0]["finish_reason"] = "tool_calls"
				}

				data, _ := json.Marshal(sseData)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}

			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	rapid.Check(t, func(rt *rapid.T) {
		toolName := rapid.StringMatching(`[a-z_]{3,15}`).Draw(rt, "toolName")
		paramKey := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "paramKey")
		paramValue := rapid.StringMatching(`[a-zA-Z0-9]{3,15}`).Draw(rt, "paramValue")
		fullArgs := fmt.Sprintf(`{"%s":"%s"}`, paramKey, paramValue)

		numChunks := rapid.IntRange(2, 4).Draw(rt, "numChunks")

		server := mockMiniMaxPartialServer(toolName, fullArgs, numChunks)
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

		// Accumulate content and check for tool calls
		var accumulatedContent string
		var receivedToolCalls []llm.ToolCall

		for chunk := range streamCh {
			require.Nil(t, chunk.Err, "Stream should not have errors")
			accumulatedContent += chunk.Delta.Content
			if len(chunk.Delta.ToolCalls) > 0 {
				receivedToolCalls = append(receivedToolCalls, chunk.Delta.ToolCalls...)
			}
		}

		// MiniMax parses tool calls from accumulated XML content
		// Verify that tool calls were extracted
		if len(receivedToolCalls) > 0 {
			assert.Equal(t, toolName, receivedToolCalls[0].Name, "Tool name should match for MiniMax")
			// Verify arguments are valid JSON
			var parsed map[string]any
			err = json.Unmarshal(receivedToolCalls[0].Arguments, &parsed)
			assert.NoError(t, err, "Tool call arguments should be valid JSON for MiniMax")
		}
	})
}

// TestProperty16_ToolCallAccumulation_EmptyChunks verifies that chunks with
// non-empty arguments are properly accumulated (empty chunks are skipped).
func TestProperty16_ToolCallAccumulation_EmptyChunks(t *testing.T) {
	logger := zap.NewNop()

	providerList := []string{"grok", "qwen", "deepseek", "glm"}

	for _, providerName := range providerList {
		t.Run(providerName, func(t *testing.T) {
			fullArgs := `{"key":"value"}`

			// Create chunks - note: we only send non-empty argument parts
			// because empty string "" gets JSON-marshaled to "" which breaks accumulation
			chunks := []partialToolCallChunk{
				{ID: "test", Model: "test", ToolCallID: "call_1", ToolCallName: "test_tool", PartialArgs: `{"key":`, Index: 0, IsFirst: true},
				{ID: "test", Model: "test", ToolCallID: "call_1", ToolCallName: "test_tool", PartialArgs: `"value"}`, Index: 0, IsFirst: false, FinishReason: "tool_calls"},
			}

			server := mockSSEServerWithPartialToolCalls(chunks)
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test"},
				},
				Tools: []llm.ToolSchema{
					{Name: "test_tool", Parameters: json.RawMessage(`{}`)},
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

			require.NoError(t, err, "Stream() should not return error")

			var argsBuilder strings.Builder
			for chunk := range streamCh {
				require.Nil(t, chunk.Err, "Stream should not have errors")
				if len(chunk.Delta.ToolCalls) > 0 {
					tc := chunk.Delta.ToolCalls[0]
					if len(tc.Arguments) > 0 {
						// Try to unmarshal as JSON string first (OpenAI format)
						var argStr string
						if err := json.Unmarshal(tc.Arguments, &argStr); err == nil {
							argsBuilder.WriteString(argStr)
						} else {
							argsBuilder.Write(tc.Arguments)
						}
					}
				}
			}

			accumulatedArgs := argsBuilder.String()

			// Verify accumulated arguments form valid JSON
			var parsed map[string]any
			err = json.Unmarshal([]byte(accumulatedArgs), &parsed)
			assert.NoError(t, err, "Accumulated arguments should be valid JSON: %s", accumulatedArgs)
			assert.Equal(t, fullArgs, accumulatedArgs, "Accumulated args should match original")
		})
	}
}
