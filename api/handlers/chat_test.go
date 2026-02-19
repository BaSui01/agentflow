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
	"github.com/BaSui01/agentflow/llm"
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

			handler := NewChatHandler(provider, logger)

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

		handler := NewChatHandler(provider, logger)

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
		handler := NewChatHandler(provider, logger)

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
				Name:        "test_tool",
				Description: "A test tool",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		},
		ToolChoice: "auto",
		Timeout:    "30s",
		Metadata:   map[string]string{"key": "value"},
		Tags:       []string{"test"},
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
	assert.Equal(t, "auto", llmReq.ToolChoice)
	assert.Equal(t, 30*time.Second, llmReq.Timeout)
	assert.Equal(t, map[string]string{"key": "value"}, llmReq.Metadata)
	assert.Equal(t, []string{"test"}, llmReq.Tags)
}
