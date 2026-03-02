package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Constructor ---

func TestNewOpenAIProvider_Defaults(t *testing.T) {
	tests := []struct {
		name         string
		cfg          providers.OpenAIConfig
		expectedName string
	}{
		{
			name:         "empty config",
			cfg:          providers.OpenAIConfig{},
			expectedName: "openai",
		},
		{
			name: "with organization",
			cfg: providers.OpenAIConfig{
				Organization: "org-test",
			},
			expectedName: "openai",
		},
		{
			name: "with responses API enabled",
			cfg: providers.OpenAIConfig{
				UseResponsesAPI: true,
			},
			expectedName: "openai",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewOpenAIProvider(tt.cfg, zap.NewNop())
			require.NotNil(t, p)
			assert.Equal(t, tt.expectedName, p.Name())
			assert.Equal(t, "https://api.openai.com", p.Cfg.BaseURL)
		})
	}
}

func TestOpenAIProvider_FallbackModel(t *testing.T) {
	p := NewOpenAIProvider(providers.OpenAIConfig{}, zap.NewNop())
	assert.Equal(t, "gpt-5.2", p.Cfg.FallbackModel)
}

func TestOpenAIProvider_OrganizationHeader(t *testing.T) {
	var capturedOrgHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedOrgHeader = r.Header.Get("OpenAI-Organization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providerbase.OpenAICompatResponse{
			ID:    "resp-1",
			Model: "gpt-5.2",
			Choices: []providerbase.OpenAICompatChoice{
				{Index: 0, FinishReason: "stop", Message: providerbase.OpenAICompatMessage{Role: "assistant", Content: "ok"}},
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		Organization:       "org-abc",
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "org-abc", capturedOrgHeader)
}

// --- PreviousResponseID context ---

func TestWithPreviousResponseID(t *testing.T) {
	ctx := context.Background()

	// Empty string should not set value
	ctx2 := WithPreviousResponseID(ctx, "")
	_, ok := PreviousResponseIDFromContext(ctx2)
	assert.False(t, ok)

	// Non-empty string should set value
	ctx3 := WithPreviousResponseID(ctx, "resp_abc")
	id, ok := PreviousResponseIDFromContext(ctx3)
	assert.True(t, ok)
	assert.Equal(t, "resp_abc", id)
}

// --- Endpoints ---

func TestOpenAIProvider_Endpoints(t *testing.T) {
	t.Run("standard API", func(t *testing.T) {
		p := NewOpenAIProvider(providers.OpenAIConfig{
			BaseProviderConfig: providers.BaseProviderConfig{BaseURL: "https://api.openai.com"},
		}, zap.NewNop())
		ep := p.Endpoints()
		assert.Contains(t, ep.Completion, "/v1/chat/completions")
	})

	t.Run("responses API", func(t *testing.T) {
		p := NewOpenAIProvider(providers.OpenAIConfig{
			BaseProviderConfig: providers.BaseProviderConfig{BaseURL: "https://api.openai.com"},
			UseResponsesAPI:    true,
		}, zap.NewNop())
		ep := p.Endpoints()
		assert.Contains(t, ep.Completion, "/v1/responses")
	})
}

// --- Completion (standard Chat Completions API) ---

func TestOpenAIProvider_Completion_Standard(t *testing.T) {
	var capturedRequest providerbase.OpenAICompatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer test-key")

		err := json.NewDecoder(r.Body).Decode(&capturedRequest)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providerbase.OpenAICompatResponse{
			ID:    "chatcmpl-1",
			Model: "gpt-5.2",
			Choices: []providerbase.OpenAICompatChoice{
				{
					Index:        0,
					FinishReason: "stop",
					Message:      providerbase.OpenAICompatMessage{Role: "assistant", Content: "Hello from OpenAI"},
				},
			},
			Usage: &providerbase.OpenAICompatUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "openai", resp.Provider)
	assert.Equal(t, "gpt-5.2", resp.Model)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from OpenAI", resp.Choices[0].Message.Content)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
}

// --- Completion (Responses API) ---

func TestOpenAIProvider_Completion_ResponsesAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/responses", r.URL.Path)

		var reqBody openAIResponsesRequest
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		assert.NotNil(t, reqBody.Store)
		assert.True(t, *reqBody.Store)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIResponsesResponse{
			ID:        "resp_abc",
			Object:    "response",
			CreatedAt: 1700000000,
			Status:    "completed",
			Model:     "gpt-5.2",
			Output: []responsesOutputItem{
				{
					Type:   "message",
					ID:     "msg_1",
					Status: "completed",
					Role:   "assistant",
					Content: []responsesContent{
						{Type: "output_text", Text: "Hello from Responses API"},
					},
				},
			},
			Usage: &responsesUsage{InputTokens: 8, OutputTokens: 6, TotalTokens: 14},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	storeTrue := true
	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		Store:    &storeTrue,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "resp_abc", resp.ID)
	assert.Equal(t, "openai", resp.Provider)
	assert.Equal(t, "gpt-5.2", resp.Model)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from Responses API", resp.Choices[0].Message.Content)
	assert.Equal(t, 14, resp.Usage.TotalTokens)
}

func TestOpenAIProvider_Completion_ResponsesAPI_WithToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIResponsesResponse{
			ID:     "resp_tc",
			Model:  "gpt-5.2",
			Status: "completed",
			Output: []responsesOutputItem{
				{
					Type:   "message",
					ID:     "msg_1",
					Status: "completed",
					Role:   "assistant",
					Content: []responsesContent{
						{Type: "output_text", Text: "Let me check the weather."},
					},
				},
				{
					Type:      "function_call",
					ID:        "fc_1",
					CallID:    "call_1",
					Name:      "get_weather",
					Arguments: json.RawMessage(`{"city":"NYC"}`),
				},
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Weather?"}},
	})
	require.NoError(t, err)
	// message output + function_call output get merged into one choice
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Let me check the weather.", resp.Choices[0].Message.Content)
	require.Len(t, resp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "get_weather", resp.Choices[0].Message.ToolCalls[0].Name)
	assert.Equal(t, "call_1", resp.Choices[0].Message.ToolCalls[0].ID)
}

func TestOpenAIProvider_Completion_ResponsesAPI_PreviousResponseID(t *testing.T) {
	var capturedPrevID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody openAIResponsesRequest
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		capturedPrevID = reqBody.PreviousResponseID

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(openAIResponsesResponse{
			ID: "resp_2", Model: "gpt-5.2",
			Output: []responsesOutputItem{{Type: "message", Role: "assistant", Content: []responsesContent{{Type: "output_text", Text: "ok"}}}},
		})
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	// Via context
	ctx := WithPreviousResponseID(context.Background(), "resp_prev_ctx")
	_, err := p.Completion(ctx, &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "resp_prev_ctx", capturedPrevID)

	// Via request field (takes precedence)
	_, err = p.Completion(ctx, &llm.ChatRequest{
		Messages:           []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		PreviousResponseID: "resp_prev_req",
	})
	require.NoError(t, err)
	assert.Equal(t, "resp_prev_req", capturedPrevID)
}

// --- Completion errors ---

func TestOpenAIProvider_Completion_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrRateLimit, llmErr.Code)
	assert.True(t, llmErr.Retryable)
}

func TestOpenAIProvider_Completion_ResponsesAPI_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Invalid request","type":"invalid_request_error"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code)
}

// --- Stream ---

func TestOpenAIProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunk := providerbase.OpenAICompatResponse{
			ID: "chatcmpl-stream", Model: "gpt-5.2",
			Choices: []providerbase.OpenAICompatChoice{
				{Index: 0, Delta: &providerbase.OpenAICompatMessage{Role: "assistant", Content: "Hello"}},
			},
		}
		data, _ := json.Marshal(chunk)
		w.Write([]byte("data: "))
		w.Write(data)
		w.Write([]byte("\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}

	require.Len(t, chunks, 1)
	assert.Equal(t, "Hello", chunks[0].Delta.Content)
	assert.Equal(t, "openai", chunks[0].Provider)
}

func TestOpenAIProvider_Stream_ResponsesAPI_ToolArgsAreAccumulated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte("event: response.created\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.2"}}` + "\n\n"))
		_, _ = w.Write([]byte("event: response.function_call_arguments.delta\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","name":"lookup","delta":"{\"city\":"}` + "\n\n"))
		_, _ = w.Write([]byte("event: response.function_call_arguments.delta\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","name":"lookup","delta":"\"NYC\"}"}` + "\n\n"))
		_, _ = w.Write([]byte("event: response.function_call_arguments.done\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.function_call_arguments.done","item_id":"fc_1"}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Weather?"}},
	})
	require.NoError(t, err)

	var gotToolCall *types.ToolCall
	for c := range ch {
		if len(c.Delta.ToolCalls) > 0 {
			tc := c.Delta.ToolCalls[0]
			gotToolCall = &tc
		}
	}

	require.NotNil(t, gotToolCall)
	assert.Equal(t, "fc_1", gotToolCall.ID)
	assert.Equal(t, "lookup", gotToolCall.Name)
	assert.JSONEq(t, `{"city":"NYC"}`, string(gotToolCall.Arguments))
}

func TestBuildInputContent_IncludesVideoAsInputFile(t *testing.T) {
	content := buildInputContent(types.Message{
		Role:    llm.RoleUser,
		Content: "analyze this video",
		Videos:  []types.VideoContent{{URL: "https://example.com/video.mp4"}},
	})

	parts, ok := content.([]inputContentPart)
	require.True(t, ok)
	require.Len(t, parts, 2)
	assert.Equal(t, "input_text", parts[0].Type)
	assert.Equal(t, "input_file", parts[1].Type)
	assert.Equal(t, "https://example.com/video.mp4", parts[1].FileURL)
}

func TestChooseResponsesReasoningEffort(t *testing.T) {
	effort, ok := chooseResponsesReasoningEffort(&llm.ChatRequest{ReasoningEffort: "low"})
	require.True(t, ok)
	assert.Equal(t, "low", effort)

	effort, ok = chooseResponsesReasoningEffort(&llm.ChatRequest{ReasoningMode: "extended"})
	require.True(t, ok)
	assert.Equal(t, "high", effort)

	_, ok = chooseResponsesReasoningEffort(&llm.ChatRequest{ReasoningMode: "unsupported"})
	assert.False(t, ok)
}

func TestOpenAIProvider_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "bad-key", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrUnauthorized, llmErr.Code)
}

// --- toResponsesAPIChatResponse ---

func TestToResponsesAPIChatResponse(t *testing.T) {
	resp := openAIResponsesResponse{
		ID:        "resp_test",
		Model:     "gpt-5.2",
		CreatedAt: 1700000000,
		Status:    "completed",
		Output: []responsesOutputItem{
			{Type: "message", ID: "msg_1", Status: "completed", Role: "assistant",
				Content: []responsesContent{{Type: "output_text", Text: "Hello"}}},
			{Type: "function_call", ID: "fc_1", CallID: "call_1", Name: "do_thing",
				Arguments: json.RawMessage(`{}`)},
		},
		Usage: &responsesUsage{InputTokens: 5, OutputTokens: 3, TotalTokens: 8},
	}

	result := toResponsesAPIChatResponse(resp, "openai")
	assert.Equal(t, "resp_test", result.ID)
	assert.Equal(t, "openai", result.Provider)
	// message + function_call merged into one choice
	require.Len(t, result.Choices, 1)
	assert.Equal(t, "Hello", result.Choices[0].Message.Content)
	assert.Equal(t, "tool_calls", result.Choices[0].FinishReason)
	require.Len(t, result.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "do_thing", result.Choices[0].Message.ToolCalls[0].Name)
	assert.Equal(t, 8, result.Usage.TotalTokens)
	assert.False(t, result.CreatedAt.IsZero())
}
