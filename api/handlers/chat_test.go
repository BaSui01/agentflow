package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	llm "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// =============================================================================
// 🧪 模拟提供商
// =============================================================================

type mockProvider struct {
	completionFunc func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)
	streamFunc     func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error)
}

func (m *mockProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.completionFunc != nil {
		return m.completionFunc(ctx, req)
	}
	return nil, errors.New("not implemented")
}

func (m *mockProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	if m.streamFunc != nil {
		return m.streamFunc(ctx, req)
	}
	return nil, errors.New("not implemented")
}

func (m *mockProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (m *mockProvider) Name() string {
	return "mock"
}

func (m *mockProvider) SupportsNativeFunctionCalling() bool {
	return true
}

func (m *mockProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return nil, nil
}

func (m *mockProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }

func newChatHandlerForProvider(provider llm.Provider, logger *zap.Logger) *ChatHandler {
	gateway := llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Logger:       logger,
	})
	chatProvider := llmgateway.NewChatProviderAdapter(gateway, provider)
	service, err := usecase.NewDefaultChatService(
		usecase.ChatRuntime{
			Gateway:      gateway,
			ChatProvider: chatProvider,
		},
		newUsecaseChatConverter(NewDefaultChatConverter(defaultStreamTimeout)),
		logger,
	)
	if err != nil {
		panic(err)
	}
	handler, err := NewChatHandler(service, logger)
	if err != nil {
		panic(err)
	}
	return handler
}

// =============================================================================
// 🧪 ChatHandler 测试
// =============================================================================

func TestChatHandler_HandleCompletion(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name           string
		request        api.ChatRequest
		mockResponse   *llm.ChatResponse
		mockError      error
		expectedStatus int
		checkResponse  func(*testing.T, *api.ChatResponse)
	}{
		{
			name: "successful completion",
			request: api.ChatRequest{
				Model: "gpt-4",
				Messages: []api.Message{
					{Role: "user", Content: "Hello"},
				},
			},
			mockResponse: &llm.ChatResponse{
				ID:       "test-id",
				Provider: "openai",
				Model:    "gpt-4",
				Choices: []llm.ChatChoice{
					{
						Index:        0,
						FinishReason: "stop",
						Message: types.Message{
							Role:    types.RoleAssistant,
							Content: "Hi there!",
						},
					},
				},
				Usage: llm.ChatUsage{
					PromptTokens:     10,
					CompletionTokens: 5,
					TotalTokens:      15,
				},
				CreatedAt: time.Now(),
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *api.ChatResponse) {
				assert.Equal(t, "test-id", resp.ID)
				assert.Equal(t, "openai", resp.Provider)
				assert.Len(t, resp.Choices, 1)
				assert.Equal(t, "Hi there!", resp.Choices[0].Message.Content)
			},
		},
		{
			name: "missing model",
			request: api.ChatRequest{
				Messages: []api.Message{
					{Role: "user", Content: "Hello"},
				},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "empty messages",
			request: api.ChatRequest{
				Model:    "gpt-4",
				Messages: []api.Message{},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid temperature",
			request: api.ChatRequest{
				Model: "gpt-4",
				Messages: []api.Message{
					{Role: "user", Content: "Hello"},
				},
				Temperature: 3.0,
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid route policy",
			request: api.ChatRequest{
				Model: "gpt-4",
				Messages: []api.Message{
					{Role: "user", Content: "Hello"},
				},
				RoutePolicy: "fastest",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockProvider{
				completionFunc: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockResponse, nil
				},
			}

			handler := newChatHandlerForProvider(provider, logger)

			body, err := json.Marshal(tt.request)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")

			handler.HandleCompletion(w, r)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK && tt.checkResponse != nil {
				var resp Response
				err := json.NewDecoder(w.Body).Decode(&resp)
				require.NoError(t, err)

				assert.True(t, resp.Success)

				// 转换 Data 为 api.ChatResponse
				dataBytes, err := json.Marshal(resp.Data)
				require.NoError(t, err)

				var chatResp api.ChatResponse
				err = json.Unmarshal(dataBytes, &chatResp)
				require.NoError(t, err)

				tt.checkResponse(t, &chatResp)
			}
		})
	}
}

func TestChatHandler_HandleStream(t *testing.T) {
	logger := zap.NewNop()

	t.Run("successful stream", func(t *testing.T) {
		chunks := []llm.StreamChunk{
			{
				ID:       "test-id",
				Provider: "openai",
				Model:    "gpt-4",
				Index:    0,
				Delta: types.Message{
					Role:    types.RoleAssistant,
					Content: "Hello",
				},
			},
			{
				ID:       "test-id",
				Provider: "openai",
				Model:    "gpt-4",
				Index:    0,
				Delta: types.Message{
					Content: " world",
				},
				FinishReason: "stop",
			},
		}

		provider := &mockProvider{
			streamFunc: func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
				ch := make(chan llm.StreamChunk, len(chunks))
				for _, chunk := range chunks {
					ch <- chunk
				}
				close(ch)
				return ch, nil
			},
		}

		handler := newChatHandlerForProvider(provider, logger)

		request := api.ChatRequest{
			Model: "gpt-4",
			Messages: []api.Message{
				{Role: "user", Content: "Hello"},
			},
		}

		body, err := json.Marshal(request)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions/stream", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		handler.HandleStream(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), "data: [DONE]")
	})

	t.Run("invalid request", func(t *testing.T) {
		provider := &mockProvider{}
		handler := newChatHandlerForProvider(provider, logger)

		request := api.ChatRequest{
			// 缺少型号
			Messages: []api.Message{
				{Role: "user", Content: "Hello"},
			},
		}

		body, err := json.Marshal(request)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions/stream", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		handler.HandleStream(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestChatHandler_ValidateChatRequest(t *testing.T) {
	logger := zap.NewNop()
	handler := NewChatHandler(nil, logger)

	tests := []struct {
		name    string
		request *api.ChatRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: &api.ChatRequest{
				Model: "gpt-4",
				Messages: []api.Message{
					{Role: "user", Content: "Hello"},
				},
				Temperature: 0.7,
				TopP:        0.9,
			},
			wantErr: false,
		},
		{
			name: "missing model",
			request: &api.ChatRequest{
				Messages: []api.Message{
					{Role: "user", Content: "Hello"},
				},
			},
			wantErr: true,
		},
		{
			name: "empty messages",
			request: &api.ChatRequest{
				Model:    "gpt-4",
				Messages: []api.Message{},
			},
			wantErr: true,
		},
		{
			name: "invalid temperature - too low",
			request: &api.ChatRequest{
				Model: "gpt-4",
				Messages: []api.Message{
					{Role: "user", Content: "Hello"},
				},
				Temperature: -0.1,
			},
			wantErr: true,
		},
		{
			name: "invalid temperature - too high",
			request: &api.ChatRequest{
				Model: "gpt-4",
				Messages: []api.Message{
					{Role: "user", Content: "Hello"},
				},
				Temperature: 2.1,
			},
			wantErr: true,
		},
		{
			name: "invalid top_p - too low",
			request: &api.ChatRequest{
				Model: "gpt-4",
				Messages: []api.Message{
					{Role: "user", Content: "Hello"},
				},
				TopP: -0.1,
			},
			wantErr: true,
		},
		{
			name: "invalid top_p - too high",
			request: &api.ChatRequest{
				Model: "gpt-4",
				Messages: []api.Message{
					{Role: "user", Content: "Hello"},
				},
				TopP: 1.1,
			},
			wantErr: true,
		},
		{
			name: "invalid provider hint",
			request: &api.ChatRequest{
				Model: "gpt-4",
				Messages: []api.Message{
					{Role: "user", Content: "Hello"},
				},
				Provider: "bad/provider",
			},
			wantErr: true,
		},
		{
			name: "invalid route policy",
			request: &api.ChatRequest{
				Model: "gpt-4",
				Messages: []api.Message{
					{Role: "user", Content: "Hello"},
				},
				RoutePolicy: "fastest",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateChatRequest(tt.request)
			if tt.wantErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestChatHandler_ConvertToLLMRequest(t *testing.T) {
	logger := zap.NewNop()
	handler := NewChatHandler(nil, logger)
	strict := true
	includeServerSide := true
	format := &api.ToolFormat{Type: "grammar", Syntax: "lark", Definition: "start: WORD"}

	apiReq := &api.ChatRequest{
		TraceID:  "trace-123",
		TenantID: "tenant-456",
		UserID:   "user-789",
		Model:    "gpt-4",
		Messages: []api.Message{
			{
				Role:    "user",
				Content: "Hello",
				Name:    "test-user",
			},
		},
		MaxTokens:   100,
		Temperature: 0.7,
		TopP:        0.9,
		Stop:        []string{"END"},
		Tools: []api.ToolSchema{
			{
				Type:        types.ToolTypeCustom,
				Name:        "test_tool",
				Description: "A test tool",
				Parameters:  json.RawMessage(`{}`),
				Format:      format,
				Strict:      &strict,
			},
		},
		ToolChoice: "auto",
		ResponseFormat: &api.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &api.ResponseFormatJSONSchema{
				Name:   "out",
				Schema: map[string]any{"type": "object"},
			},
		},
		ParallelToolCalls:   boolPtr(true),
		ServiceTier:         stringPtr("auto"),
		User:                "openai-user",
		MaxCompletionTokens: intPtr(80),
		ReasoningEffort:     "low",
		ReasoningSummary:    "detailed",
		ReasoningDisplay:    "summarized",
		InferenceSpeed:      "fast",
		Store:               boolPtr(true),
		Modalities:          []string{"text"},
		WebSearchOptions: &api.WebSearchOptions{
			SearchContextSize: "high",
			AllowedDomains:    []string{"example.com"},
			UserLocation: &api.WebSearchLocation{
				Country: "US",
				City:    "San Francisco",
			},
		},
		PromptCacheKey:                   "pcache-key-1",
		PromptCacheRetention:             "24h",
		CacheControl:                     &api.CacheControl{Type: "ephemeral", TTL: "5m"},
		CachedContent:                    "cachedContents/session-1",
		IncludeServerSideToolInvocations: &includeServerSide,
		PreviousResponseID:               "resp_prev_1",
		ConversationID:                   "conv_req_1",
		Include:                          []string{"output_text"},
		Truncation:                       "auto",
		Timeout:                          "30s",
		Metadata:                         map[string]string{"key": "value"},
		Tags:                             []string{"test"},
	}

	llmReq := handler.convertToLLMRequest(apiReq)

	assert.Equal(t, "trace-123", llmReq.TraceID)
	assert.Equal(t, "tenant-456", llmReq.TenantID)
	assert.Equal(t, "user-789", llmReq.UserID)
	assert.Equal(t, "gpt-4", llmReq.Model)
	assert.Len(t, llmReq.Messages, 1)
	assert.Equal(t, types.RoleUser, llmReq.Messages[0].Role)
	assert.Equal(t, "Hello", llmReq.Messages[0].Content)
	assert.Equal(t, "test-user", llmReq.Messages[0].Name)
	assert.Equal(t, 100, llmReq.MaxTokens)
	assert.Equal(t, float32(0.7), llmReq.Temperature)
	assert.Equal(t, float32(0.9), llmReq.TopP)
	assert.Equal(t, []string{"END"}, llmReq.Stop)
	assert.Len(t, llmReq.Tools, 1)
	assert.Equal(t, "test_tool", llmReq.Tools[0].Name)
	assert.Equal(t, types.ToolTypeCustom, llmReq.Tools[0].Type)
	require.NotNil(t, llmReq.Tools[0].Format)
	assert.Equal(t, "grammar", llmReq.Tools[0].Format.Type)
	require.NotNil(t, llmReq.Tools[0].Strict)
	assert.True(t, *llmReq.Tools[0].Strict)
	require.NotNil(t, llmReq.ToolChoice)
	assert.Equal(t, types.ToolChoiceModeAuto, llmReq.ToolChoice.Mode)
	require.NotNil(t, llmReq.ResponseFormat)
	assert.Equal(t, llm.ResponseFormatType("json_schema"), llmReq.ResponseFormat.Type)
	require.NotNil(t, llmReq.ResponseFormat.JSONSchema)
	assert.Equal(t, "out", llmReq.ResponseFormat.JSONSchema.Name)
	require.NotNil(t, llmReq.ParallelToolCalls)
	assert.True(t, *llmReq.ParallelToolCalls)
	require.NotNil(t, llmReq.ServiceTier)
	assert.Equal(t, "auto", *llmReq.ServiceTier)
	assert.Equal(t, "openai-user", llmReq.User)
	require.NotNil(t, llmReq.MaxCompletionTokens)
	assert.Equal(t, 80, *llmReq.MaxCompletionTokens)
	assert.Equal(t, "low", llmReq.ReasoningEffort)
	assert.Equal(t, "detailed", llmReq.ReasoningSummary)
	assert.Equal(t, "summarized", llmReq.ReasoningDisplay)
	assert.Equal(t, "fast", llmReq.InferenceSpeed)
	require.NotNil(t, llmReq.Store)
	assert.True(t, *llmReq.Store)
	assert.Equal(t, []string{"text"}, llmReq.Modalities)
	require.NotNil(t, llmReq.WebSearchOptions)
	assert.Equal(t, "high", llmReq.WebSearchOptions.SearchContextSize)
	assert.Equal(t, []string{"example.com"}, llmReq.WebSearchOptions.AllowedDomains)
	require.NotNil(t, llmReq.WebSearchOptions.UserLocation)
	assert.Equal(t, "US", llmReq.WebSearchOptions.UserLocation.Country)
	assert.Equal(t, "San Francisco", llmReq.WebSearchOptions.UserLocation.City)
	assert.Equal(t, "pcache-key-1", llmReq.PromptCacheKey)
	assert.Equal(t, "24h", llmReq.PromptCacheRetention)
	require.NotNil(t, llmReq.CacheControl)
	assert.Equal(t, "ephemeral", llmReq.CacheControl.Type)
	assert.Equal(t, "5m", llmReq.CacheControl.TTL)
	assert.Equal(t, "cachedContents/session-1", llmReq.CachedContent)
	require.NotNil(t, llmReq.IncludeServerSideToolInvocations)
	assert.True(t, *llmReq.IncludeServerSideToolInvocations)
	assert.Equal(t, "resp_prev_1", llmReq.PreviousResponseID)
	assert.Equal(t, "conv_req_1", llmReq.ConversationID)
	assert.Equal(t, []string{"output_text"}, llmReq.Include)
	assert.Equal(t, "auto", llmReq.Truncation)
	assert.Equal(t, 30*time.Second, llmReq.Timeout)
	assert.Equal(t, map[string]string{"key": "value"}, llmReq.Metadata)
	assert.Equal(t, []string{"test"}, llmReq.Tags)
}

func TestChatHandler_HandleCapabilities(t *testing.T) {
	handler := NewChatHandler(&openAICompatServiceStub{}, zap.NewNop())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/chat/capabilities", nil)
	handler.HandleCapabilities(w, r)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp Response
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)
}

func boolPtr(v bool) *bool { return &v }

func intPtr(v int) *int { return &v }

func stringPtr(v string) *string { return &v }
