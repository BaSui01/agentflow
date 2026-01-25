package deepseek

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/providers"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// Feature: multi-provider-support, Property 6: Credential Override from Context
// Validates: Requirements 5.8
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
			// Create a test server to capture the API key
			var capturedAPIKey string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Extract API key from Authorization header
				authHeader := r.Header.Get("Authorization")
				if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
					capturedAPIKey = authHeader[7:]
				}

				// Return a valid response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(openAIResponse{
					ID:    "test-id",
					Model: "deepseek-chat",
					Choices: []openAIChoice{
						{
							Index:        0,
							FinishReason: "stop",
							Message: openAIMessage{
								Role:    "assistant",
								Content: "test response",
							},
						},
					},
				})
			}))
			defer server.Close()

			// Create provider with config API key
			cfg := providers.DeepSeekConfig{
				APIKey:  tc.configAPIKey,
				BaseURL: server.URL,
			}
			provider := NewDeepSeekProvider(cfg, zap.NewNop())

			// Create context with or without credential override
			ctx := context.Background()
			if tc.contextAPIKey != "" {
				ctx = llm.WithCredentialOverride(ctx, llm.CredentialOverride{
					APIKey: tc.contextAPIKey,
				})
			}

			// Make a completion request
			req := &llm.ChatRequest{
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "test"},
				},
			}

			_, err := provider.Completion(ctx, req)
			assert.NoError(t, err, "Completion should succeed")

			// Verify the correct API key was used
			assert.Equal(t, tc.expectedAPIKey, capturedAPIKey,
				"API key should match expected value")
		})
	}

	// Test credential override in streaming mode
	t.Run("credential override in streaming mode", func(t *testing.T) {
		var capturedAPIKey string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				capturedAPIKey = authHeader[7:]
			}

			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			// Send a simple SSE response
			data := openAIResponse{
				ID:    "test-id",
				Model: "deepseek-chat",
				Choices: []openAIChoice{
					{
						Index: 0,
						Delta: &openAIMessage{
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
			APIKey:  "config-key",
			BaseURL: server.URL,
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

		// Consume the stream
		for chunk := range ch {
			assert.Nil(t, chunk.Err, "Stream chunk should not have error")
		}

		assert.Equal(t, "override-key", capturedAPIKey,
			"Override API key should be used in streaming mode")
	})

	// Test that no override preserves config key
	t.Run("no override uses config key", func(t *testing.T) {
		var capturedAPIKey string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				capturedAPIKey = authHeader[7:]
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(openAIResponse{
				ID:    "test-id",
				Model: "deepseek-chat",
				Choices: []openAIChoice{
					{
						Index:        0,
						FinishReason: "stop",
						Message: openAIMessage{
							Role:    "assistant",
							Content: "test response",
						},
					},
				},
			})
		}))
		defer server.Close()

		cfg := providers.DeepSeekConfig{
			APIKey:  "config-key-only",
			BaseURL: server.URL,
		}
		provider := NewDeepSeekProvider(cfg, zap.NewNop())

		// No credential override in context
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
