package grok

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// 特性: 多提供者支持, 属性 1: 默认 BaseURL 配置
// 审定:要求1.2
func TestProperty1_DefaultBaseURLConfiguration(t *testing.T) {
	testCases := []struct {
		name            string
		inputBaseURL    string
		expectedBaseURL string
	}{
		{
			name:            "empty BaseURL defaults to https://api.x.ai",
			inputBaseURL:    "",
			expectedBaseURL: "https://api.x.ai",
		},
		{
			name:            "custom BaseURL is preserved",
			inputBaseURL:    "https://custom.api.com",
			expectedBaseURL: "https://custom.api.com",
		},
		{
			name:            "BaseURL with trailing slash",
			inputBaseURL:    "https://api.example.com/",
			expectedBaseURL: "https://api.example.com/",
		},
		{
			name:            "BaseURL with path",
			inputBaseURL:    "https://api.example.com/v1",
			expectedBaseURL: "https://api.example.com/v1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := providers.GrokConfig{
				BaseProviderConfig: providers.BaseProviderConfig{
					APIKey:  "test-key",
					BaseURL: tc.inputBaseURL,
				},
			}
			provider := NewGrokProvider(cfg, zap.NewNop())

			assert.Equal(t, tc.expectedBaseURL, provider.Cfg.BaseURL,
				"BaseURL should match expected value")
		})
	}
}

// 特性: 多提供者支持, 属性 2: Bearer Token 认证
// 审定:要求1.3
func TestProperty2_BearerTokenAuthentication(t *testing.T) {
	testCases := []struct {
		name   string
		apiKey string
	}{
		{"standard key", "sk-test-key-123"},
		{"long key", "sk-proj-very-long-api-key-with-many-characters-1234567890"},
		{"short key", "key"},
		{"key with special chars", "sk-test_key.with-special@chars"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requestCaptured := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				authHeader := r.Header.Get("Authorization")
				expectedAuth := "Bearer " + tc.apiKey
				assert.Equal(t, expectedAuth, authHeader,
					"Authorization header should use Bearer token format")

				contentType := r.Header.Get("Content-Type")
				assert.Equal(t, "application/json", contentType,
					"Content-Type should be application/json")

				requestCaptured = true

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
					ID:    "test-id",
					Model: "grok-beta",
					Choices: []providers.OpenAICompatChoice{
						{
							Index:        0,
							FinishReason: "stop",
							Message: providers.OpenAICompatMessage{
								Role:    "assistant",
								Content: "test response",
							},
						},
					},
				})
			}))
			defer server.Close()

			cfg := providers.GrokConfig{
				BaseProviderConfig: providers.BaseProviderConfig{
					APIKey:  tc.apiKey,
					BaseURL: server.URL,
				},
			}
			provider := NewGrokProvider(cfg, zap.NewNop())

			ctx := context.Background()
			req := &llm.ChatRequest{
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "test"},
				},
			}

			_, err := provider.Completion(ctx, req)
			assert.NoError(t, err, "Completion should succeed")
			assert.True(t, requestCaptured, "Request should have been captured by test server")
		})
	}

	t.Run("credential override from context", func(t *testing.T) {
		overriddenKey := "override-key-123"
		requestCaptured := false

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			expectedAuth := "Bearer " + overriddenKey
			assert.Equal(t, expectedAuth, authHeader,
				"Authorization header should use overridden API key")

			requestCaptured = true

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
				ID:    "test-id",
				Model: "grok-beta",
				Choices: []providers.OpenAICompatChoice{
					{
						Index:        0,
						FinishReason: "stop",
						Message: providers.OpenAICompatMessage{
							Role:    "assistant",
							Content: "test response",
						},
					},
				},
			})
		}))
		defer server.Close()

		cfg := providers.GrokConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  "original-key",
				BaseURL: server.URL,
			},
		}
		provider := NewGrokProvider(cfg, zap.NewNop())

		ctx := llm.WithCredentialOverride(context.Background(), llm.CredentialOverride{
			APIKey: overriddenKey,
		})

		req := &llm.ChatRequest{
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "test"},
			},
		}

		_, err := provider.Completion(ctx, req)
		assert.NoError(t, err, "Completion should succeed")
		assert.True(t, requestCaptured, "Request should have been captured by test server")
	})

	t.Run("health check uses bearer token", func(t *testing.T) {
		apiKey := "health-check-key"
		requestCaptured := false

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			expectedAuth := "Bearer " + apiKey
			assert.Equal(t, expectedAuth, authHeader,
				"HealthCheck should use Bearer token authentication")

			requestCaptured = true

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"id": "grok-beta"},
				},
			})
		}))
		defer server.Close()

		cfg := providers.GrokConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  apiKey,
				BaseURL: server.URL,
			},
		}
		provider := NewGrokProvider(cfg, zap.NewNop())

		ctx := context.Background()
		status, err := provider.HealthCheck(ctx)

		assert.NoError(t, err, "HealthCheck should succeed")
		assert.True(t, status.Healthy, "HealthCheck should return healthy status")
		assert.True(t, requestCaptured, "Request should have been captured by test server")
	})
}

// 特性: 多提供者支持, 属性 5: 默认模式选择优先级
// 审定:要求1.7、14.1、14.2、14.3
func TestProperty5_DefaultModelSelectionPriority(t *testing.T) {
	testCases := []struct {
		name          string
		requestModel  string
		configModel   string
		expectedModel string
	}{
		{
			name:          "request model takes priority",
			requestModel:  "grok-2",
			configModel:   "grok-1",
			expectedModel: "grok-2",
		},
		{
			name:          "config model used when request model empty",
			requestModel:  "",
			configModel:   "grok-custom",
			expectedModel: "grok-custom",
		},
		{
			name:          "default model used when both empty",
			requestModel:  "",
			configModel:   "",
			expectedModel: "grok-beta",
		},
		{
			name:          "request model used even if config has value",
			requestModel:  "grok-specific",
			configModel:   "grok-default",
			expectedModel: "grok-specific",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedModel string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var reqBody providers.OpenAICompatRequest
				json.NewDecoder(r.Body).Decode(&reqBody)
				capturedModel = reqBody.Model

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
					ID:    "test-id",
					Model: reqBody.Model,
					Choices: []providers.OpenAICompatChoice{
						{
							Index:        0,
							FinishReason: "stop",
							Message: providers.OpenAICompatMessage{
								Role:    "assistant",
								Content: "test response",
							},
						},
					},
				})
			}))
			defer server.Close()

			cfg := providers.GrokConfig{
				BaseProviderConfig: providers.BaseProviderConfig{
					APIKey:  "test-key",
					BaseURL: server.URL,
					Model:   tc.configModel,
				},
			}
			provider := NewGrokProvider(cfg, zap.NewNop())

			ctx := context.Background()
			req := &llm.ChatRequest{
				Model: tc.requestModel,
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "test"},
				},
			}

			resp, err := provider.Completion(ctx, req)
			assert.NoError(t, err, "Completion should succeed")
			assert.Equal(t, tc.expectedModel, capturedModel,
				"Model in API request should match expected priority")
			assert.Equal(t, tc.expectedModel, resp.Model,
				"Model in response should match expected priority")
		})
	}

	t.Run("model selection in streaming mode", func(t *testing.T) {
		var capturedModel string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var reqBody providers.OpenAICompatRequest
			json.NewDecoder(r.Body).Decode(&reqBody)
			capturedModel = reqBody.Model

			assert.True(t, reqBody.Stream, "Stream flag should be true")

			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			data := providers.OpenAICompatResponse{
				ID:    "test-id",
				Model: reqBody.Model,
				Choices: []providers.OpenAICompatChoice{
					{
						Index: 0,
						Delta: &providers.OpenAICompatMessage{
							Role:    "assistant",
							Content: "test",
						},
					},
				},
			}
			jsonData, _ := json.Marshal(data)
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			fmt.Fprintf(w, "data: [DONE]\n\n")
		}))
		defer server.Close()

		cfg := providers.GrokConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
				Model:   "config-model",
			},
		}
		provider := NewGrokProvider(cfg, zap.NewNop())

		ctx := context.Background()
		req := &llm.ChatRequest{
			Model: "request-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "test"},
			},
		}

		ch, err := provider.Stream(ctx, req)
		assert.NoError(t, err, "Stream should succeed")

		for chunk := range ch {
			assert.Nil(t, chunk.Err, "Stream chunk should not have error")
			assert.Equal(t, "request-model", chunk.Model,
				"Model in stream chunk should match request model")
		}

		assert.Equal(t, "request-model", capturedModel,
			"Model in API request should prioritize request model")
	})

	t.Run("ChooseModel function logic", func(t *testing.T) {
		model := providers.ChooseModel(&llm.ChatRequest{Model: "req-model"}, "cfg-model", "grok-beta")
		assert.Equal(t, "req-model", model)

		model = providers.ChooseModel(&llm.ChatRequest{Model: ""}, "cfg-model", "grok-beta")
		assert.Equal(t, "cfg-model", model)

		model = providers.ChooseModel(&llm.ChatRequest{Model: ""}, "", "grok-beta")
		assert.Equal(t, "grok-beta", model)

		model = providers.ChooseModel(nil, "cfg-model", "grok-beta")
		assert.Equal(t, "cfg-model", model)

		model = providers.ChooseModel(nil, "", "grok-beta")
		assert.Equal(t, "grok-beta", model)
	})
}
