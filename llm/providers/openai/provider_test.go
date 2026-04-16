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

func TestOpenAIProvider_Completion_EndpointModeResponsesOverridesConfig(t *testing.T) {
	var calledPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(openAIResponsesResponse{
			ID:     "resp_override",
			Model:  "gpt-5.2",
			Status: "completed",
			Output: []responsesOutputItem{
				{
					Type:    "message",
					Role:    "assistant",
					Content: []responsesContent{{Type: "output_text", Text: "ok"}},
				},
			},
		}))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    false,
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		Metadata: map[string]string{"endpoint_mode": "responses"},
	})
	require.NoError(t, err)
	assert.Equal(t, "/v1/responses", calledPath)
}

func TestOpenAIProvider_Completion_EndpointModeChatOverridesConfig(t *testing.T) {
	var calledPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(providerbase.OpenAICompatResponse{
			ID:    "chatcmpl_override",
			Model: "gpt-5.2",
			Choices: []providerbase.OpenAICompatChoice{
				{Index: 0, FinishReason: "stop", Message: providerbase.OpenAICompatMessage{Role: "assistant", Content: "ok"}},
			},
		}))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		Metadata: map[string]string{"endpoint_mode": "chat_completions"},
	})
	require.NoError(t, err)
	assert.Equal(t, "/v1/chat/completions", calledPath)
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

func TestOpenAIProvider_Completion_ResponsesAPI_ReportsProviderPromptUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/responses", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(openAIResponsesResponse{
			ID:        "resp_usage",
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
						{Type: "output_text", Text: "ok"},
					},
				},
			},
		}))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	var reports []llm.ProviderPromptUsageReport
	ctx := llm.WithProviderPromptUsageReporter(context.Background(), func(report llm.ProviderPromptUsageReport) {
		reports = append(reports, report)
	})

	_, err := p.Completion(ctx, &llm.ChatRequest{
		Model: "gpt-5.2",
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: "继续扩写这一段"},
		},
		Tools: []types.ToolSchema{
			{Name: "search_docs", Description: "搜索资料", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
	})
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, "openai", reports[0].Provider)
	assert.Equal(t, "gpt-5.2", reports[0].Model)
	assert.Equal(t, "responses", reports[0].API)
	assert.Greater(t, reports[0].PromptTokens, 0)
}

func TestOpenAIProvider_Completion_ResponsesAPI_MapsReasoningSummaryAndOpaqueState(t *testing.T) {
	var reqBody openAIResponsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAIResponsesResponse{
			ID:        "resp_reasoning",
			Object:    "response",
			CreatedAt: 1700000000,
			Status:    "completed",
			Model:     "gpt-5.2",
			Output: []responsesOutputItem{
				{
					Type:             "reasoning",
					ID:               "rs_1",
					Status:           "completed",
					Summary:          []responsesContent{{Type: "summary_text", Text: "Investigating request"}},
					EncryptedContent: "enc_123",
				},
				{
					Type:   "message",
					ID:     "msg_1",
					Status: "completed",
					Role:   "assistant",
					Content: []responsesContent{
						{Type: "output_text", Text: "done"},
					},
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
		Messages:         []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		ReasoningEffort:  "medium",
		ReasoningSummary: "",
	})
	require.NoError(t, err)
	require.NotNil(t, reqBody.Reasoning)
	assert.Equal(t, "medium", reqBody.Reasoning.Effort)
	assert.Equal(t, "auto", reqBody.Reasoning.Summary)
	assert.Contains(t, reqBody.Include, "reasoning.encrypted_content")
	require.Len(t, resp.Choices, 1)
	require.NotNil(t, resp.Choices[0].Message.ReasoningContent)
	assert.Equal(t, "Investigating request", *resp.Choices[0].Message.ReasoningContent)
	require.Len(t, resp.Choices[0].Message.ReasoningSummaries, 1)
	assert.Equal(t, "summary_text", resp.Choices[0].Message.ReasoningSummaries[0].Kind)
	require.Len(t, resp.Choices[0].Message.OpaqueReasoning, 1)
	assert.Equal(t, "enc_123", resp.Choices[0].Message.OpaqueReasoning[0].State)
}

func TestOpenAIProvider_Completion_ResponsesAPI_MapsNativeWebSearchTool(t *testing.T) {
	var raw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/responses", r.URL.Path)
		err := json.NewDecoder(r.Body).Decode(&raw)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(openAIResponsesResponse{
			ID: "resp_web_search_missing", Model: "gpt-5.2", Status: "completed",
			Output: []responsesOutputItem{
				{Type: "message", Role: "assistant", Content: []responsesContent{{Type: "output_text", Text: "ok"}}},
			},
		})
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		Tools: []types.ToolSchema{
			{
				Name:        "web_search",
				Description: "native web search",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			},
		},
		WebSearchOptions: &llm.WebSearchOptions{
			SearchContextSize: "high",
			AllowedDomains:    []string{"example.com", "docs.example.com"},
			UserLocation: &llm.WebSearchLocation{
				Country: "US",
				City:    "San Francisco",
			},
		},
	})
	require.NoError(t, err)

	tool := findResponsesWebSearchTool(t, raw)
	assert.Equal(t, "high", tool["search_context_size"])
	userLocation, ok := tool["user_location"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "approximate", userLocation["type"])
	assert.Equal(t, "US", userLocation["country"])
	assert.Equal(t, "San Francisco", userLocation["city"])
	filters, ok := tool["filters"].(map[string]any)
	require.True(t, ok)
	allowedDomains, ok := filters["allowed_domains"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"example.com", "docs.example.com"}, allowedDomains)
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

func TestOpenAIProvider_Completion_ResponsesAPI_WithCustomToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAIResponsesResponse{
			ID:     "resp_custom",
			Model:  "gpt-5.4",
			Status: "completed",
			Output: []responsesOutputItem{
				{
					Type:   "custom_tool_call",
					ID:     "ct_1",
					CallID: "call_custom_1",
					Name:   "code_exec",
					Input:  "print('hi')",
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
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Run code"}},
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	require.Len(t, resp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, types.ToolTypeCustom, resp.Choices[0].Message.ToolCalls[0].Type)
	assert.Equal(t, "code_exec", resp.Choices[0].Message.ToolCalls[0].Name)
	assert.Equal(t, "print('hi')", resp.Choices[0].Message.ToolCalls[0].Input)
	assert.Equal(t, "call_custom_1", resp.Choices[0].Message.ToolCalls[0].ID)
}

func TestOpenAIProvider_Completion_ResponsesAPI_PreviousResponseID(t *testing.T) {
	var capturedPrevID string
	var capturedConversation string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody openAIResponsesRequest
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		capturedPrevID = reqBody.PreviousResponseID
		capturedConversation = reqBody.Conversation

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
		Messages:       []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		ConversationID: "conv_req_1",
	})
	require.NoError(t, err)
	assert.Equal(t, "resp_prev_ctx", capturedPrevID)
	assert.Equal(t, "conv_req_1", capturedConversation)

	// Via request field (takes precedence)
	_, err = p.Completion(ctx, &llm.ChatRequest{
		Messages:           []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		PreviousResponseID: "resp_prev_req",
	})
	require.NoError(t, err)
	assert.Equal(t, "resp_prev_req", capturedPrevID)
}

func TestBuildResponsesRequest_MergesAllInstructionMessages(t *testing.T) {
	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: "https://api.openai.com"},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	body := p.buildResponsesRequest(&llm.ChatRequest{
		Messages: []types.Message{
			{Role: llm.RoleSystem, Content: "system instruction"},
			{Role: llm.RoleDeveloper, Content: "developer instruction"},
			{Role: llm.RoleUser, Content: "hello"},
		},
	})

	assert.Equal(t, "system instruction\n\ndeveloper instruction", body.Instructions)
	inputs, ok := body.Input.([]any)
	require.True(t, ok)
	require.Len(t, inputs, 1)
	item, ok := inputs[0].(responsesInputItem)
	require.True(t, ok)
	assert.Equal(t, "user", item.Role)
	assert.Equal(t, "hello", item.Content)
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

func TestOpenAIProvider_Stream_EndpointModeChatOverridesConfig(t *testing.T) {
	var calledPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		chunk := providerbase.OpenAICompatResponse{
			ID:    "chatcmpl-stream",
			Model: "gpt-5.2",
			Choices: []providerbase.OpenAICompatChoice{
				{Index: 0, Delta: &providerbase.OpenAICompatMessage{Role: "assistant", Content: "ok"}},
			},
		}
		data, _ := json.Marshal(chunk)
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(data)
		_, _ = w.Write([]byte("\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		Metadata: map[string]string{"endpoint_mode": "chat_completions"},
	})
	require.NoError(t, err)
	for range ch {
	}

	assert.Equal(t, "/v1/chat/completions", calledPath)
}

func TestOpenAIProvider_Stream_ResponsesAPI_ToolArgsAreAccumulated(t *testing.T) {
	// 严格对标 OpenAI 官方文档：函数名通过 response.output_item.added 传递，
	// 而非 response.function_call_arguments.delta（该事件不含 name 字段）。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte("event: response.created\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.2"}}` + "\n\n"))
		// 函数名在 output_item.added 事件中
		_, _ = w.Write([]byte("event: response.output_item.added\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","name":"lookup"}}` + "\n\n"))
		// delta 事件只包含参数片段，不含 name
		_, _ = w.Write([]byte("event: response.function_call_arguments.delta\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"city\":"}` + "\n\n"))
		_, _ = w.Write([]byte("event: response.function_call_arguments.delta\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"\"NYC\"}"}` + "\n\n"))
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

func TestOpenAIProvider_Stream_ResponsesAPI_CustomToolInputIsAccumulated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			`event: response.created` + "\n" +
				`data: {"type":"response.created","response":{"id":"resp_custom_stream","model":"gpt-5.4"}}` + "\n\n",
			`event: response.output_item.added` + "\n" +
				`data: {"type":"response.output_item.added","item":{"id":"ct_1","type":"custom_tool_call","call_id":"call_custom_1","name":"code_exec"}}` + "\n\n",
			`event: response.custom_tool_call_input.delta` + "\n" +
				`data: {"type":"response.custom_tool_call_input.delta","item_id":"ct_1","delta":"print("}` + "\n\n",
			`event: response.custom_tool_call_input.delta` + "\n" +
				`data: {"type":"response.custom_tool_call_input.delta","item_id":"ct_1","delta":"\"hi\")"}` + "\n\n",
			`event: response.custom_tool_call_input.done` + "\n" +
				`data: {"type":"response.custom_tool_call_input.done","item_id":"ct_1"}` + "\n\n",
			"data: [DONE]\n\n",
		}
		for _, e := range events {
			_, _ = w.Write([]byte(e))
		}
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Run code"}},
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
	assert.Equal(t, "call_custom_1", gotToolCall.ID)
	assert.Equal(t, types.ToolTypeCustom, gotToolCall.Type)
	assert.Equal(t, "code_exec", gotToolCall.Name)
	assert.Equal(t, `print("hi")`, gotToolCall.Input)
}

func TestOpenAIProvider_Stream_ResponsesAPI_FunctionCallName_FullSequence(t *testing.T) {
	// 验证完整的函数调用流式事件序列：
	// output_item.added → arguments.delta(×N) → arguments.done → response.completed
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			`event: response.created` + "\n" +
				`data: {"type":"response.created","response":{"id":"resp_42","model":"gpt-5.2"}}` + "\n\n",
			// 非 function_call 类型的 output_item.added 应被忽略
			`event: response.output_item.added` + "\n" +
				`data: {"type":"response.output_item.added","item":{"id":"msg_1","type":"message"}}` + "\n\n",
			// 正确的 function_call output_item.added
			`event: response.output_item.added` + "\n" +
				`data: {"type":"response.output_item.added","item":{"id":"fc_abc","type":"function_call","name":"get_weather"}}` + "\n\n",
			// 参数分片 delta（不含 name 字段）
			`event: response.function_call_arguments.delta` + "\n" +
				`data: {"type":"response.function_call_arguments.delta","item_id":"fc_abc","delta":"{\"loc"}` + "\n\n",
			`event: response.function_call_arguments.delta` + "\n" +
				`data: {"type":"response.function_call_arguments.delta","item_id":"fc_abc","delta":"ation\":\"Shanghai\"}"}` + "\n\n",
			// done 事件
			`event: response.function_call_arguments.done` + "\n" +
				`data: {"type":"response.function_call_arguments.done","item_id":"fc_abc"}` + "\n\n",
			// response.completed with usage
			`event: response.completed` + "\n" +
				`data: {"type":"response.completed","response":{"id":"resp_42","model":"gpt-5.2","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}` + "\n\n",
			"data: [DONE]\n\n",
		}
		for _, e := range events {
			_, _ = w.Write([]byte(e))
		}
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "What's the weather in Shanghai?"}},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}

	// 应该有 tool_calls chunk 和 usage chunk
	require.NotEmpty(t, chunks)

	var gotToolCall *types.ToolCall
	var gotFinishReason string
	for _, c := range chunks {
		if len(c.Delta.ToolCalls) > 0 {
			tc := c.Delta.ToolCalls[0]
			gotToolCall = &tc
			gotFinishReason = c.FinishReason
		}
	}

	require.NotNil(t, gotToolCall, "should receive a tool call chunk")
	assert.Equal(t, "fc_abc", gotToolCall.ID)
	assert.Equal(t, "get_weather", gotToolCall.Name, "function name must come from output_item.added")
	assert.JSONEq(t, `{"location":"Shanghai"}`, string(gotToolCall.Arguments))
	assert.Equal(t, "tool_calls", gotFinishReason)
}

func TestOpenAIProvider_Stream_ResponsesAPI_MapsReasoningSummaryAndOpaqueState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			`event: response.created` + "\n" +
				`data: {"type":"response.created","response":{"id":"resp_reasoning","model":"gpt-5.2"}}` + "\n\n",
			`event: response.reasoning_summary_text.delta` + "\n" +
				`data: {"type":"response.reasoning_summary_text.delta","item_id":"rs_1","delta":"summary chunk"}` + "\n\n",
			`event: response.output_item.done` + "\n" +
				`data: {"type":"response.output_item.done","item":{"id":"rs_1","type":"reasoning","status":"completed","summary":[{"type":"summary_text","text":"summary chunk"}],"encrypted_content":"enc_456"}}` + "\n\n",
			`event: response.output_text.delta` + "\n" +
				`data: {"type":"response.output_text.delta","delta":"final"}` + "\n\n",
			`event: response.completed` + "\n" +
				`data: {"type":"response.completed","response":{"id":"resp_reasoning","model":"gpt-5.2","output":[{"id":"rs_1","type":"reasoning","status":"completed","summary":[{"type":"summary_text","text":"summary chunk"}],"encrypted_content":"enc_456"}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}` + "\n\n",
			"data: [DONE]\n\n",
		}
		for _, e := range events {
			_, _ = w.Write([]byte(e))
		}
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages:        []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		ReasoningEffort: "medium",
	})
	require.NoError(t, err)

	var (
		chunks          []llm.StreamChunk
		reasoningDelta  string
		structuredChunk *llm.StreamChunk
	)
	for c := range ch {
		chunks = append(chunks, c)
		if c.Delta.ReasoningContent != nil && reasoningDelta == "" {
			reasoningDelta = *c.Delta.ReasoningContent
		}
		if len(c.Delta.ReasoningSummaries) > 0 || len(c.Delta.OpaqueReasoning) > 0 {
			copied := c
			structuredChunk = &copied
		}
	}

	require.NotEmpty(t, chunks)
	assert.Equal(t, "summary chunk", reasoningDelta)
	require.NotNil(t, structuredChunk)
	require.Len(t, structuredChunk.Delta.ReasoningSummaries, 1)
	assert.Equal(t, "summary chunk", structuredChunk.Delta.ReasoningSummaries[0].Text)
	require.Len(t, structuredChunk.Delta.OpaqueReasoning, 1)
	assert.Equal(t, "enc_456", structuredChunk.Delta.OpaqueReasoning[0].State)
}

func TestOpenAIProvider_Stream_ResponsesAPI_MapsNativeWebSearchTool(t *testing.T) {
	var raw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := json.NewDecoder(r.Body).Decode(&raw)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: response.created\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.2"}}` + "\n\n"))
		_, _ = w.Write([]byte("event: response.output_text.delta\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.output_text.delta","delta":"ok"}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		Tools: []types.ToolSchema{
			{
				Name:       "web_search_preview",
				Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			},
		},
		WebSearchOptions: &llm.WebSearchOptions{
			SearchContextSize: "low",
			AllowedDomains:    []string{"news.example.com"},
			UserLocation: &llm.WebSearchLocation{
				Country: "CN",
				City:    "Shanghai",
			},
		},
	})
	require.NoError(t, err)
	for range ch {
	}

	tool := findResponsesWebSearchTool(t, raw)
	assert.Equal(t, "low", tool["search_context_size"])
	userLocation, ok := tool["user_location"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "approximate", userLocation["type"])
	assert.Equal(t, "CN", userLocation["country"])
	assert.Equal(t, "Shanghai", userLocation["city"])
	filters, ok := tool["filters"].(map[string]any)
	require.True(t, ok)
	allowedDomains, ok := filters["allowed_domains"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"news.example.com"}, allowedDomains)
}

func TestOpenAIProvider_Completion_ResponsesAPI_AutoAddsWebSearchToolFromOptions(t *testing.T) {
	var raw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := json.NewDecoder(r.Body).Decode(&raw)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAIResponsesResponse{
			ID: "resp_auto_ws", Model: "gpt-5.2", Status: "completed",
			Output: []responsesOutputItem{
				{Type: "message", Role: "assistant", Content: []responsesContent{{Type: "output_text", Text: "ok"}}},
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		WebSearchOptions: &llm.WebSearchOptions{
			SearchContextSize: "medium",
			AllowedDomains:    []string{"openai.com"},
		},
	})
	require.NoError(t, err)

	tool := findResponsesWebSearchTool(t, raw)
	assert.Equal(t, "medium", tool["search_context_size"])
	filters, ok := tool["filters"].(map[string]any)
	require.True(t, ok)
	allowedDomains, ok := filters["allowed_domains"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"openai.com"}, allowedDomains)
}

func TestOpenAIProvider_Completion_ResponsesAPI_MapsIncludeAndTruncation(t *testing.T) {
	var raw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := json.NewDecoder(r.Body).Decode(&raw)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAIResponsesResponse{
			ID: "resp_include_truncation", Model: "gpt-5.2", Status: "completed",
			Output: []responsesOutputItem{
				{Type: "message", Role: "assistant", Content: []responsesContent{{Type: "output_text", Text: "ok"}}},
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages:             []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		Include:              []string{"output_text.logprobs", "reasoning.encrypted_content"},
		Truncation:           "auto",
		PromptCacheKey:       "route-a",
		PromptCacheRetention: "in_memory",
	})
	require.NoError(t, err)

	includeRaw, ok := raw["include"].([]any)
	require.True(t, ok)
	require.Len(t, includeRaw, 2)
	assert.Equal(t, "output_text.logprobs", includeRaw[0])
	assert.Equal(t, "reasoning.encrypted_content", includeRaw[1])
	assert.Equal(t, "auto", raw["truncation"])
	assert.Equal(t, "route-a", raw["prompt_cache_key"])
	assert.Equal(t, "in_memory", raw["prompt_cache_retention"])
}

func TestOpenAIProvider_Completion_ResponsesAPI_RejectsInvalidPromptCacheRetention(t *testing.T) {
	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: "https://api.openai.com"},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages:             []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
		PromptCacheRetention: "5m",
	})
	require.Error(t, err)
	var llmErr *types.Error
	require.ErrorAs(t, err, &llmErr)
	assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code)
	assert.Contains(t, llmErr.Message, "prompt_cache_retention")
}

func TestBuildResponsesTools_PreservesStrict(t *testing.T) {
	strict := true
	tools := buildResponsesTools(&llm.ChatRequest{
		Tools: []types.ToolSchema{{
			Name:        "strict_tool",
			Description: "Strict tool",
			Parameters:  json.RawMessage(`{"type":"object"}`),
			Strict:      &strict,
		}},
	})
	require.Len(t, tools, 1)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, tool["strict"])
}

func TestBuildResponsesTools_CustomTool(t *testing.T) {
	tools := buildResponsesTools(&llm.ChatRequest{
		Tools: []types.ToolSchema{{
			Type:        types.ToolTypeCustom,
			Name:        "code_exec",
			Description: "Execute code",
			Format: &types.ToolFormat{
				Type:       "grammar",
				Syntax:     "lark",
				Definition: "start: WORD",
			},
		}},
	})
	require.Len(t, tools, 1)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "custom", tool["type"])
	assert.Equal(t, "code_exec", tool["name"])
	format, ok := tool["format"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "grammar", format["type"])
	assert.Equal(t, "lark", format["syntax"])
}

func findResponsesWebSearchTool(t *testing.T, raw map[string]any) map[string]any {
	t.Helper()

	toolsAny, ok := raw["tools"]
	require.True(t, ok, "responses request should contain tools field")

	tools, ok := toolsAny.([]any)
	require.True(t, ok, "responses tools should be array")
	require.NotEmpty(t, tools, "responses tools should not be empty")

	for _, item := range tools {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if toolType, _ := tool["type"].(string); toolType == "web_search" {
			return tool
		}
	}
	require.Fail(t, "web_search tool not found in responses request")
	return nil
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
			{Type: "reasoning", ID: "rs_1", Status: "completed",
				Summary: []responsesContent{{Type: "summary_text", Text: "summary"}}},
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
	require.NotNil(t, result.Choices[0].Message.ReasoningContent)
	assert.Equal(t, "summary", *result.Choices[0].Message.ReasoningContent)
	require.Len(t, result.Choices[0].Message.ReasoningSummaries, 1)
	assert.Equal(t, "Hello", result.Choices[0].Message.Content)
	assert.Equal(t, "tool_calls", result.Choices[0].FinishReason)
	require.Len(t, result.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "do_thing", result.Choices[0].Message.ToolCalls[0].Name)
	assert.Equal(t, 8, result.Usage.TotalTokens)
	assert.False(t, result.CreatedAt.IsZero())
}

// --- Tool Call ID Conversion for Responses API ---

func TestConvertToResponsesToolCallID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"converts call_ prefix to fc_", "call_abc123", "fc_abc123"},
		{"preserves existing fc_ prefix", "fc_xyz789", "fc_xyz789"},
		{"adds fc_ prefix to bare ID", "simple_id", "fc_simple_id"},
		{"handles empty string", "", ""},
		{"handles whitespace", "  call_spaced  ", "fc_spaced"},
		{"handles call_ with underscores", "call_test_123_abc", "fc_test_123_abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, convertToResponsesToolCallID(tt.input))
		})
	}
}

func TestOpenAIProvider_Completion_ResponsesAPI_ConvertsToolCallIDs(t *testing.T) {
	var capturedBody openAIResponsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := json.NewDecoder(r.Body).Decode(&capturedBody)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(openAIResponsesResponse{
			ID:     "resp_test",
			Model:  "gpt-5.2",
			Status: "completed",
			Output: []responsesOutputItem{
				{Type: "message", Role: "assistant", Content: []responsesContent{{Type: "output_text", Text: "Done"}}},
			},
		})
		require.NoError(t, err)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	// Send a request with tool calls in history using Chat Completions "call_" prefix format
	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: "What's the weather?"},
			{Role: llm.RoleAssistant, ToolCalls: []types.ToolCall{
				{ID: "call_abc123", Type: types.ToolTypeFunction, Name: "get_weather", Arguments: json.RawMessage(`{"city":"NYC"}`)},
			}},
			{Role: llm.RoleTool, ToolCallID: "call_abc123", Content: `{"temp":72}`},
		},
	})
	require.NoError(t, err)

	// Verify the tool call IDs were converted to "fc_" prefix in the request
	require.NotNil(t, capturedBody.Input)
	inputItems, ok := capturedBody.Input.([]any)
	require.True(t, ok, "expected input to be a slice")

	// Find the function_call item
	var foundFunctionCall bool
	for _, item := range inputItems {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if typ, _ := itemMap["type"].(string); typ == "function_call" {
			foundFunctionCall = true
			// ID should be converted from "call_abc123" to "fc_abc123"
			assert.Equal(t, "fc_abc123", itemMap["id"], "function_call ID should be converted to fc_ prefix")
			assert.Equal(t, "fc_abc123", itemMap["call_id"], "function_call call_id should be converted to fc_ prefix")
		}
		if typ, _ := itemMap["type"].(string); typ == "function_call_output" {
			// CallID should be converted from "call_abc123" to "fc_abc123"
			assert.Equal(t, "fc_abc123", itemMap["call_id"], "function_call_output call_id should be converted to fc_ prefix")
		}
	}
	assert.True(t, foundFunctionCall, "expected to find a function_call in the input")
}

func TestOpenAIProvider_Completion_ResponsesAPI_CustomToolOutputWriteback(t *testing.T) {
	var capturedBody openAIResponsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(openAIResponsesResponse{
			ID:    "resp_custom_out",
			Model: "gpt-5.2",
			Output: []responsesOutputItem{{
				Type: "message",
				ID:   "msg_1",
				Role: "assistant",
				Content: []responsesContent{{
					Type: "output_text",
					Text: "done",
				}},
			}},
		}))
	}))
	t.Cleanup(server.Close)

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{
			{Role: llm.RoleAssistant, ToolCalls: []types.ToolCall{
				{ID: "call_custom_1", Type: types.ToolTypeCustom, Name: "code_exec", Input: "print('hi')"},
			}},
			{Role: llm.RoleTool, ToolCallID: "call_custom_1", Name: "code_exec", Content: "ok"},
		},
	})
	require.NoError(t, err)

	inputItems, ok := capturedBody.Input.([]any)
	require.True(t, ok)
	var found bool
	for _, item := range inputItems {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if typ, _ := itemMap["type"].(string); typ == "custom_tool_call_output" {
			found = true
			assert.Equal(t, "fc_custom_1", itemMap["call_id"])
			assert.Equal(t, "ok", itemMap["output"])
		}
	}
	assert.True(t, found, "expected custom_tool_call_output writeback item")
}
