package glm

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

// Feature: multi-provider-support, Property 12: HTTP Status to Error Code Mapping
// Validates: Requirements 2.8, 9.1-9.8
func TestProperty12_HTTPStatusToErrorCodeMapping(t *testing.T) {
	testCases := []struct {
		name          string
		httpStatus    int
		errorMessage  string
		expectedCode  llm.ErrorCode
		expectedRetry bool
	}{
		{
			name:          "401 Unauthorized",
			httpStatus:    http.StatusUnauthorized,
			errorMessage:  "Invalid API key",
			expectedCode:  llm.ErrUnauthorized,
			expectedRetry: false,
		},
		{
			name:          "403 Forbidden",
			httpStatus:    http.StatusForbidden,
			errorMessage:  "Access denied",
			expectedCode:  llm.ErrForbidden,
			expectedRetry: false,
		},
		{
			name:          "429 Rate Limited",
			httpStatus:    http.StatusTooManyRequests,
			errorMessage:  "Too many requests",
			expectedCode:  llm.ErrRateLimited,
			expectedRetry: true,
		},
		{
			name:          "400 Bad Request",
			httpStatus:    http.StatusBadRequest,
			errorMessage:  "Invalid request",
			expectedCode:  llm.ErrInvalidRequest,
			expectedRetry: false,
		},
		{
			name:          "400 Quota Exceeded",
			httpStatus:    http.StatusBadRequest,
			errorMessage:  "Quota exceeded for this month",
			expectedCode:  llm.ErrQuotaExceeded,
			expectedRetry: false,
		},
		{
			name:          "400 Credit Exhausted",
			httpStatus:    http.StatusBadRequest,
			errorMessage:  "Insufficient credit balance",
			expectedCode:  llm.ErrQuotaExceeded,
			expectedRetry: false,
		},
		{
			name:          "503 Service Unavailable",
			httpStatus:    http.StatusServiceUnavailable,
			errorMessage:  "Service temporarily unavailable",
			expectedCode:  llm.ErrUpstreamError,
			expectedRetry: true,
		},
		{
			name:          "502 Bad Gateway",
			httpStatus:    http.StatusBadGateway,
			errorMessage:  "Bad gateway",
			expectedCode:  llm.ErrUpstreamError,
			expectedRetry: true,
		},
		{
			name:          "504 Gateway Timeout",
			httpStatus:    http.StatusGatewayTimeout,
			errorMessage:  "Gateway timeout",
			expectedCode:  llm.ErrUpstreamError,
			expectedRetry: true,
		},
		{
			name:          "529 Model Overloaded",
			httpStatus:    529,
			errorMessage:  "Model is overloaded",
			expectedCode:  llm.ErrModelOverloaded,
			expectedRetry: true,
		},
		{
			name:          "500 Internal Server Error",
			httpStatus:    http.StatusInternalServerError,
			errorMessage:  "Internal server error",
			expectedCode:  llm.ErrUpstreamError,
			expectedRetry: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test server that returns the specified error
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.httpStatus)
				json.NewEncoder(w).Encode(openAIErrorResp{
					Error: struct {
						Message string `json:"message"`
						Type    string `json:"type"`
						Code    any    `json:"code"`
						Param   string `json:"param"`
					}{
						Message: tc.errorMessage,
						Type:    "error",
					},
				})
			}))
			defer server.Close()

			// Create provider with test server URL
			cfg := providers.GLMConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}
			provider := NewGLMProvider(cfg, zap.NewNop())

			// Make a completion request
			ctx := context.Background()
			req := &llm.ChatRequest{
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "test"},
				},
			}

			_, err := provider.Completion(ctx, req)

			// Verify error is returned
			assert.Error(t, err, "Should return an error")

			// Verify error is of type llm.Error
			llmErr, ok := err.(*llm.Error)
			assert.True(t, ok, "Error should be of type *llm.Error")

			// Verify error code
			assert.Equal(t, tc.expectedCode, llmErr.Code,
				"Error code should match expected value")

			// Verify HTTP status
			assert.Equal(t, tc.httpStatus, llmErr.HTTPStatus,
				"HTTP status should match")

			// Verify retryable flag
			assert.Equal(t, tc.expectedRetry, llmErr.Retryable,
				"Retryable flag should match expected value")

			// Verify provider name
			assert.Equal(t, "glm", llmErr.Provider,
				"Provider name should be 'glm'")

			// Verify error message contains original message
			assert.Contains(t, llmErr.Message, tc.errorMessage,
				"Error message should contain original error message")
		})
	}

	// Test error mapping in streaming mode
	t.Run("error mapping in streaming mode", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(openAIErrorResp{
				Error: struct {
					Message string `json:"message"`
					Type    string `json:"type"`
					Code    any    `json:"code"`
					Param   string `json:"param"`
				}{
					Message: "Invalid API key",
					Type:    "error",
				},
			})
		}))
		defer server.Close()

		cfg := providers.GLMConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		}
		provider := NewGLMProvider(cfg, zap.NewNop())

		ctx := context.Background()
		req := &llm.ChatRequest{
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "test"},
			},
		}

		_, err := provider.Stream(ctx, req)

		// Verify error is returned
		assert.Error(t, err, "Should return an error")

		// Verify error is of type llm.Error
		llmErr, ok := err.(*llm.Error)
		assert.True(t, ok, "Error should be of type *llm.Error")

		// Verify error code
		assert.Equal(t, llm.ErrUnauthorized, llmErr.Code,
			"Error code should be ErrUnauthorized")
	})
}
