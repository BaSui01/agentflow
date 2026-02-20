package deepseek

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// 特性: 多提供者支持, 属性 6: 从上下文获取证书
// 核实:所需经费5.8
func TestProperty6_CredentialOverrideFromContext(t *testing.T) {
	testCases := []struct {
		name           string
		configAPIKey   string
		contextAPIKey  string
		expectedAPIKey string
	}{
		{
			name:           "context API key overrides config",
			configAPIKey:   "config-key-123",
			contextAPIKey:  "context-key-456",
			expectedAPIKey: "context-key-456",
		},
		{
			name:           "empty context key uses config key",
			configAPIKey:   "config-key-123",
			contextAPIKey:  "",
			expectedAPIKey: "config-key-123",
		},
		{
			name:           "whitespace context key uses config key",
			configAPIKey:   "config-key-123",
			contextAPIKey:  "   ",
			expectedAPIKey: "config-key-123",
		},
		{
			name:           "context key with whitespace is trimmed",
			configAPIKey:   "config-key-123",
			contextAPIKey:  "  context-key-789  ",
			expectedAPIKey: "context-key-789",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建测试服务器以捕获 API 密钥
			var capturedAPIKey string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// 从授权头提取 API 密钥
				authHeader := r.Header.Get("Authorization")
				if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
					capturedAPIKey = authHeader[7:]
				}

				// 返回有效的响应
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
					ID:    "test-id",
					Model: "deepseek-chat",
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

			// 以配置 API 密钥创建提供者
			cfg := providers.DeepSeekConfig{
				BaseProviderConfig: providers.BaseProviderConfig{
					APIKey:  tc.configAPIKey,
					BaseURL: server.URL,
				},
			}
			provider := NewDeepSeekProvider(cfg, zap.NewNop())

			// 创建包含或不包含证书覆盖的上下文
			ctx := context.Background()
			if tc.contextAPIKey != "" {
				ctx = llm.WithCredentialOverride(ctx, llm.CredentialOverride{
					APIKey: tc.contextAPIKey,
				})
			}

			// 提出完成请求
			req := &llm.ChatRequest{
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "test"},
				},
			}

			_, err := provider.Completion(ctx, req)
			assert.NoError(t, err, "Completion should succeed")

			// 校验正确的 API 密钥
			assert.Equal(t, tc.expectedAPIKey, capturedAPIKey,
				"API key should match expected value")
		})
	}

	// 串流模式中的测试证书覆盖
	t.Run("credential override in streaming mode", func(t *testing.T) {
		var capturedAPIKey string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				capturedAPIKey = authHeader[7:]
			}

			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			// 发送简单的 SSE 响应
			data := providers.OpenAICompatResponse{
				ID:    "test-id",
				Model: "deepseek-chat",
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
			w.Write([]byte("data: "))
			w.Write(jsonData)
			w.Write([]byte("\n\ndata: [DONE]\n\n"))
		}))
		defer server.Close()

		cfg := providers.DeepSeekConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  "config-key",
				BaseURL: server.URL,
			},
		}
		provider := NewDeepSeekProvider(cfg, zap.NewNop())

		ctx := llm.WithCredentialOverride(context.Background(), llm.CredentialOverride{
			APIKey: "override-key",
		})

		req := &llm.ChatRequest{
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "test"},
			},
		}

		ch, err := provider.Stream(ctx, req)
		assert.NoError(t, err, "Stream should succeed")

		// 控制溪流
		for chunk := range ch {
			assert.Nil(t, chunk.Err, "Stream chunk should not have error")
		}

		assert.Equal(t, "override-key", capturedAPIKey,
			"Override API key should be used in streaming mode")
	})

	// 测试没有覆盖保存配置密钥
	t.Run("no override uses config key", func(t *testing.T) {
		var capturedAPIKey string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				capturedAPIKey = authHeader[7:]
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
				ID:    "test-id",
				Model: "deepseek-chat",
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

		cfg := providers.DeepSeekConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  "config-key-only",
				BaseURL: server.URL,
			},
		}
		provider := NewDeepSeekProvider(cfg, zap.NewNop())

		// 上下文中没有证书覆盖
		ctx := context.Background()

		req := &llm.ChatRequest{
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "test"},
			},
		}

		_, err := provider.Completion(ctx, req)
		assert.NoError(t, err, "Completion should succeed")

		assert.Equal(t, "config-key-only", capturedAPIKey,
			"Config API key should be used when no override is present")
	})
}
