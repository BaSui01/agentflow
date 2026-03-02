package providerbase

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// MapHTTPError tests (complement existing error_mapping_test.go)
// =============================================================================

func TestMapHTTPError_LimitKeyword(t *testing.T) {
	// "limit" keyword in 400 should map to QuotaExceeded
	err := MapHTTPError(http.StatusBadRequest, "Rate limit exceeded for your plan", "test")
	assert.Equal(t, llm.ErrQuotaExceeded, err.Code)
}

// =============================================================================
// ReadErrorMessage tests
// =============================================================================

func TestReadErrorMessage(t *testing.T) {
	t.Run("JSON error with message and type", func(t *testing.T) {
		body := `{"error":{"message":"Invalid API key","type":"auth_error"}}`
		msg := ReadErrorMessage(strings.NewReader(body))
		assert.Equal(t, "Invalid API key (type: auth_error)", msg)
	})

	t.Run("JSON error with message only", func(t *testing.T) {
		body := `{"error":{"message":"Something went wrong"}}`
		msg := ReadErrorMessage(strings.NewReader(body))
		assert.Equal(t, "Something went wrong", msg)
	})

	t.Run("non-JSON body", func(t *testing.T) {
		body := `Internal Server Error`
		msg := ReadErrorMessage(strings.NewReader(body))
		assert.Equal(t, "Internal Server Error", msg)
	})

	t.Run("empty JSON error message falls back to raw", func(t *testing.T) {
		body := `{"error":{"message":""}}`
		msg := ReadErrorMessage(strings.NewReader(body))
		assert.Equal(t, `{"error":{"message":""}}`, msg)
	})

	t.Run("read error", func(t *testing.T) {
		msg := ReadErrorMessage(&failReader{})
		assert.Equal(t, "failed to read error response", msg)
	})
}

type failReader struct{}

func (f *failReader) Read(p []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

// =============================================================================
// ConvertMessagesToOpenAI tests
// =============================================================================

func TestConvertMessagesToOpenAI(t *testing.T) {
	t.Run("simple text messages", func(t *testing.T) {
		msgs := []types.Message{
			{Role: llm.RoleSystem, Content: "You are helpful"},
			{Role: llm.RoleUser, Content: "Hello"},
			{Role: llm.RoleAssistant, Content: "Hi there"},
		}
		result := ConvertMessagesToOpenAI(msgs)
		require.Len(t, result, 3)
		assert.Equal(t, "system", result[0].Role)
		assert.Equal(t, "You are helpful", result[0].Content)
		assert.Equal(t, "user", result[1].Role)
		assert.Equal(t, "assistant", result[2].Role)
	})

	t.Run("message with tool calls", func(t *testing.T) {
		msgs := []types.Message{
			{
				Role:    llm.RoleAssistant,
				Content: "",
				ToolCalls: []types.ToolCall{
					{ID: "tc1", Name: "search", Arguments: json.RawMessage(`{"q":"test"}`)},
				},
			},
		}
		result := ConvertMessagesToOpenAI(msgs)
		require.Len(t, result, 1)
		require.Len(t, result[0].ToolCalls, 1)
		assert.Equal(t, "tc1", result[0].ToolCalls[0].ID)
		assert.Equal(t, "function", result[0].ToolCalls[0].Type)
		assert.Equal(t, "search", result[0].ToolCalls[0].Function.Name)
	})

	t.Run("message with tool call ID", func(t *testing.T) {
		msgs := []types.Message{
			{Role: llm.RoleTool, Content: "result", ToolCallID: "tc1"},
		}
		result := ConvertMessagesToOpenAI(msgs)
		require.Len(t, result, 1)
		assert.Equal(t, "tc1", result[0].ToolCallID)
	})

	t.Run("message with images (URL)", func(t *testing.T) {
		msgs := []types.Message{
			{
				Role:    llm.RoleUser,
				Content: "What is this?",
				Images: []types.ImageContent{
					{Type: "url", URL: "https://example.com/img.png"},
				},
			},
		}
		result := ConvertMessagesToOpenAI(msgs)
		require.Len(t, result, 1)
		assert.Empty(t, result[0].Content) // text content moved to MultiContent
		require.Len(t, result[0].MultiContent, 2)
		assert.Equal(t, "text", result[0].MultiContent[0]["type"])
		assert.Equal(t, "image_url", result[0].MultiContent[1]["type"])
	})

	t.Run("message with images (base64)", func(t *testing.T) {
		msgs := []types.Message{
			{
				Role: llm.RoleUser,
				Images: []types.ImageContent{
					{Type: "base64", Data: "abc123"},
				},
			},
		}
		result := ConvertMessagesToOpenAI(msgs)
		require.Len(t, result, 1)
		require.Len(t, result[0].MultiContent, 1)
		imgURL := result[0].MultiContent[0]["image_url"].(map[string]string)
		assert.Contains(t, imgURL["url"], "data:image/png;base64,abc123")
	})

	t.Run("empty messages", func(t *testing.T) {
		result := ConvertMessagesToOpenAI(nil)
		assert.Empty(t, result)
	})
}

// =============================================================================
// ConvertToolsToOpenAI tests
// =============================================================================

func TestConvertToolsToOpenAI(t *testing.T) {
	t.Run("nil tools", func(t *testing.T) {
		assert.Nil(t, ConvertToolsToOpenAI(nil))
	})

	t.Run("empty tools", func(t *testing.T) {
		assert.Nil(t, ConvertToolsToOpenAI([]types.ToolSchema{}))
	})

	t.Run("converts tools", func(t *testing.T) {
		tools := []types.ToolSchema{
			{Name: "search", Description: "Search the web", Parameters: json.RawMessage(`{"type":"object"}`)},
		}
		result := ConvertToolsToOpenAI(tools)
		require.Len(t, result, 1)
		assert.Equal(t, "function", result[0].Type)
		assert.Equal(t, "search", result[0].Function.Name)
		assert.Equal(t, "Search the web", result[0].Function.Description)
		assert.JSONEq(t, `{"type":"object"}`, string(result[0].Function.Parameters))
	})
}

// =============================================================================
// ToLLMChatResponse tests
// =============================================================================

func TestToLLMChatResponse(t *testing.T) {
	t.Run("basic response", func(t *testing.T) {
		oa := OpenAICompatResponse{
			ID:    "resp-1",
			Model: "gpt-4",
			Choices: []OpenAICompatChoice{
				{
					Index:        0,
					FinishReason: "stop",
					Message: OpenAICompatMessage{
						Role:    "assistant",
						Content: "Hello!",
					},
				},
			},
			Usage: &OpenAICompatUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
			Created: 1700000000,
		}
		resp := ToLLMChatResponse(oa, "openai")
		assert.Equal(t, "resp-1", resp.ID)
		assert.Equal(t, "openai", resp.Provider)
		assert.Equal(t, "gpt-4", resp.Model)
		require.Len(t, resp.Choices, 1)
		assert.Equal(t, "Hello!", resp.Choices[0].Message.Content)
		assert.Equal(t, "stop", resp.Choices[0].FinishReason)
		assert.Equal(t, 15, resp.Usage.TotalTokens)
		assert.False(t, resp.CreatedAt.IsZero())
	})

	t.Run("response with tool calls", func(t *testing.T) {
		oa := OpenAICompatResponse{
			Choices: []OpenAICompatChoice{
				{
					Message: OpenAICompatMessage{
						Role: "assistant",
						ToolCalls: []OpenAICompatToolCall{
							{
								ID:   "tc1",
								Type: "function",
								Function: OpenAICompatFunction{
									Name:      "search",
									Arguments: json.RawMessage(`{"q":"test"}`),
								},
							},
						},
					},
				},
			},
		}
		resp := ToLLMChatResponse(oa, "test")
		require.Len(t, resp.Choices, 1)
		require.Len(t, resp.Choices[0].Message.ToolCalls, 1)
		assert.Equal(t, "tc1", resp.Choices[0].Message.ToolCalls[0].ID)
		assert.Equal(t, "search", resp.Choices[0].Message.ToolCalls[0].Name)
	})

	t.Run("nil usage", func(t *testing.T) {
		oa := OpenAICompatResponse{
			Choices: []OpenAICompatChoice{{Message: OpenAICompatMessage{Content: "ok"}}},
		}
		resp := ToLLMChatResponse(oa, "test")
		assert.Equal(t, 0, resp.Usage.TotalTokens)
	})

	t.Run("zero created", func(t *testing.T) {
		oa := OpenAICompatResponse{Created: 0}
		resp := ToLLMChatResponse(oa, "test")
		assert.True(t, resp.CreatedAt.IsZero())
	})
}

// =============================================================================
// ConvertResponseFormat tests
// =============================================================================

func TestConvertResponseFormat(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		assert.Nil(t, ConvertResponseFormat(nil))
	})

	t.Run("text format", func(t *testing.T) {
		rf := &llm.ResponseFormat{Type: llm.ResponseFormatText}
		result := ConvertResponseFormat(rf)
		m := result.(map[string]any)
		assert.Equal(t, "text", m["type"])
	})

	t.Run("json_object format", func(t *testing.T) {
		rf := &llm.ResponseFormat{Type: llm.ResponseFormatJSONObject}
		result := ConvertResponseFormat(rf)
		m := result.(map[string]any)
		assert.Equal(t, "json_object", m["type"])
	})

	t.Run("json_schema format with schema", func(t *testing.T) {
		strict := true
		rf := &llm.ResponseFormat{
			Type: llm.ResponseFormatJSONSchema,
			JSONSchema: &llm.JSONSchemaParam{
				Name:        "my_schema",
				Description: "A test schema",
				Schema:      map[string]any{"type": "object"},
				Strict:      &strict,
			},
		}
		result := ConvertResponseFormat(rf)
		m := result.(map[string]any)
		assert.Equal(t, "json_schema", m["type"])
		schema := m["json_schema"].(map[string]any)
		assert.Equal(t, "my_schema", schema["name"])
		assert.Equal(t, "A test schema", schema["description"])
		assert.Equal(t, true, schema["strict"])
	})

	t.Run("json_schema without schema param", func(t *testing.T) {
		rf := &llm.ResponseFormat{Type: llm.ResponseFormatJSONSchema}
		result := ConvertResponseFormat(rf)
		m := result.(map[string]any)
		assert.Equal(t, "json_schema", m["type"])
		_, hasSchema := m["json_schema"]
		assert.False(t, hasSchema)
	})

	t.Run("unknown format returns nil", func(t *testing.T) {
		rf := &llm.ResponseFormat{Type: "unknown"}
		assert.Nil(t, ConvertResponseFormat(rf))
	})
}

// =============================================================================
// BearerTokenHeaders tests
// =============================================================================

func TestBearerTokenHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	BearerTokenHeaders(req, "sk-test-key")
	assert.Equal(t, "Bearer sk-test-key", req.Header.Get("Authorization"))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
}

// =============================================================================
// SafeCloseBody tests
// =============================================================================

func TestSafeCloseBody(t *testing.T) {
	t.Run("nil body", func(t *testing.T) {
		SafeCloseBody(nil) // should not panic
	})

	t.Run("valid body", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader("test"))
		SafeCloseBody(body) // should not panic
	})
}

// =============================================================================
// NotSupportedError tests
// =============================================================================

func TestNotSupportedError(t *testing.T) {
	err := NotSupportedError("test-provider", "image generation")
	assert.Equal(t, llm.ErrInvalidRequest, err.Code)
	assert.Contains(t, err.Message, "image generation")
	assert.Contains(t, err.Message, "test-provider")
	assert.Equal(t, http.StatusNotImplemented, err.HTTPStatus)
}

// =============================================================================
// OpenAICompatMessage MarshalJSON tests
// =============================================================================

func TestOpenAICompatMessage_MarshalJSON(t *testing.T) {
	t.Run("text content", func(t *testing.T) {
		msg := OpenAICompatMessage{Role: "user", Content: "hello"}
		data, err := json.Marshal(msg)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		assert.Equal(t, "user", m["role"])
		assert.Equal(t, "hello", m["content"])
	})

	t.Run("multimodal content", func(t *testing.T) {
		msg := OpenAICompatMessage{
			Role: "user",
			MultiContent: []map[string]any{
				{"type": "text", "text": "What is this?"},
				{"type": "image_url", "image_url": map[string]string{"url": "http://img.png"}},
			},
		}
		data, err := json.Marshal(msg)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		content, ok := m["content"].([]any)
		require.True(t, ok)
		assert.Len(t, content, 2)
	})
}

// =============================================================================
// ListModelsOpenAICompat tests
// =============================================================================

func TestListModelsOpenAICompat(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"object": "list",
				"data": []map[string]any{
					{"id": "gpt-4", "object": "model"},
					{"id": "gpt-3.5", "object": "model"},
				},
			})
		}))
		defer srv.Close()

		models, err := ListModelsOpenAICompat(
			context.Background(), srv.Client(),
			srv.URL, "test-key", "test", "/v1/models",
			BearerTokenHeaders,
		)
		require.NoError(t, err)
		assert.Len(t, models, 2)
		assert.Equal(t, "gpt-4", models[0].ID)
	})

	t.Run("HTTP error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"message":"Invalid key"}}`))
		}))
		defer srv.Close()

		_, err := ListModelsOpenAICompat(
			context.Background(), srv.Client(),
			srv.URL, "bad-key", "test", "/v1/models",
			BearerTokenHeaders,
		)
		require.Error(t, err)
		llmErr, ok := err.(*types.Error)
		require.True(t, ok)
		assert.Equal(t, llm.ErrUnauthorized, llmErr.Code)
	})
}

// =============================================================================
// detectImageMIME tests
