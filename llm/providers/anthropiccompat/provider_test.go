package anthropiccompat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/types"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// New() constructor
// ---------------------------------------------------------------------------

func TestNew_Defaults(t *testing.T) {
	tests := []struct {
		name             string
		cfg              Config
		wantEndpoint     string
		wantModels       string
		wantName         string
		wantToolsSupport bool
	}{
		{
			name:             "all defaults applied",
			cfg:              Config{ProviderName: "test-anthropic"},
			wantEndpoint:     "/v1/messages",
			wantModels:       "/v1/models",
			wantName:         "test-anthropic",
			wantToolsSupport: true,
		},
		{
			name: "custom endpoint paths preserved",
			cfg: Config{
				ProviderName:   "custom-anthropic",
				EndpointPath:   "/api/messages",
				ModelsEndpoint: "/api/models",
			},
			wantEndpoint:     "/api/messages",
			wantModels:       "/api/models",
			wantName:         "custom-anthropic",
			wantToolsSupport: true,
		},
		{
			name: "supports tools false",
			cfg: Config{
				ProviderName:  "no-tools-anthropic",
				SupportsTools: boolPtr(false),
			},
			wantEndpoint:     "/v1/messages",
			wantModels:       "/v1/models",
			wantName:         "no-tools-anthropic",
			wantToolsSupport: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.cfg, zap.NewNop())
			require.NotNil(t, p)
			assert.Equal(t, tt.wantEndpoint, p.Cfg.EndpointPath)
			assert.Equal(t, tt.wantModels, p.Cfg.ModelsEndpoint)
			assert.Equal(t, tt.wantName, p.Name())
			assert.Equal(t, tt.wantToolsSupport, p.SupportsNativeFunctionCalling())
		})
	}
}

func TestNew_TimeoutDefault(t *testing.T) {
	p := New(Config{ProviderName: "test"}, nil)
	assert.NotNil(t, p.Client)
}

func TestNew_TimeoutCustom(t *testing.T) {
	p := New(Config{ProviderName: "test", Timeout: 120 * time.Second}, nil)
	assert.NotNil(t, p.Client)
}

// ---------------------------------------------------------------------------
// Completion happy path
// ---------------------------------------------------------------------------

func TestProvider_Completion_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/messages", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body providerbase.AnthropicCompatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "test-model", body.Model)
		assert.True(t, body.MaxTokens > 0)

		response := providerbase.AnthropicCompatResponse{
			ID:    "msg_test_001",
			Type:  "message",
			Role:  "assistant",
			Model: "test-model",
			Content: []providerbase.AnthropicCompatContent{
				{Type: "text", Text: "Hello from Anthropic compat!"},
			},
			StopReason: "end_turn",
			Usage: &providerbase.AnthropicCompatUsage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-anthropic",
		BaseURL:      server.URL,
		DefaultModel: "test-model",
		APIKey:       "test-key",
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "test-anthropic", resp.Provider)
	assert.Equal(t, "msg_test_001", resp.ID)
	assert.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from Anthropic compat!", resp.Choices[0].Message.Content)
	assert.Equal(t, "end_turn", resp.Choices[0].FinishReason)
	assert.Equal(t, 10, resp.Usage.PromptTokens)
	assert.Equal(t, 5, resp.Usage.CompletionTokens)
}

// ---------------------------------------------------------------------------
// Completion HTTP error
// ---------------------------------------------------------------------------

func TestProvider_Completion_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"type":"authentication_error","message":"invalid api key"}}`))
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-anthropic",
		BaseURL:      server.URL,
		APIKey:       "bad-key",
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid api key")
}

// ---------------------------------------------------------------------------
// Completion with tool call
// ---------------------------------------------------------------------------

func TestProvider_Completion_WithToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body providerbase.AnthropicCompatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Len(t, body.Tools, 1)
		assert.Equal(t, "get_weather", body.Tools[0].Name)

		response := providerbase.AnthropicCompatResponse{
			ID:    "msg_tool_001",
			Type:  "message",
			Role:  "assistant",
			Model: "test-model",
			Content: []providerbase.AnthropicCompatContent{
				{
					Type:  "tool_use",
					ID:    "toolu_001",
					Name:  "get_weather",
					Input: json.RawMessage(`{"city":"Beijing"}`),
				},
			},
			StopReason: "tool_use",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-anthropic",
		BaseURL:      server.URL,
		DefaultModel: "test-model",
		APIKey:       "test-key",
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "What's the weather?"}},
		Tools: []types.ToolSchema{{
			Type:       types.ToolTypeFunction,
			Name:       "get_weather",
			Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		}},
	})
	require.NoError(t, err)
	assert.Len(t, resp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "get_weather", resp.Choices[0].Message.ToolCalls[0].Name)
}

// ---------------------------------------------------------------------------
// Health check
// ---------------------------------------------------------------------------

func TestProvider_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":[]}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-anthropic",
		BaseURL:      server.URL,
		APIKey:       "test-key",
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
}

func TestProvider_HealthCheck_Unhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-anthropic",
		BaseURL:      server.URL,
		APIKey:       "test-key",
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.Error(t, err)
	assert.False(t, status.Healthy)
}

// ---------------------------------------------------------------------------
// List models
// ---------------------------------------------------------------------------

func TestProvider_ListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"claude-sonnet-4-6","type":"model","display_name":"Claude Sonnet 4.6"}]}`))
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-anthropic",
		BaseURL:      server.URL,
		APIKey:       "test-key",
	}, zap.NewNop())

	models, err := p.ListModels(context.Background())
	require.NoError(t, err)
	assert.Len(t, models, 1)
	assert.Equal(t, "claude-sonnet-4-6", models[0].ID)
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

func TestProvider_Endpoints(t *testing.T) {
	p := New(Config{
		ProviderName: "test-anthropic",
		BaseURL:      "https://api.test.com/anthropic",
	}, zap.NewNop())

	ep := p.Endpoints()
	assert.Equal(t, "https://api.test.com/anthropic/v1/messages", ep.Completion)
	assert.Equal(t, "https://api.test.com/anthropic/v1/models", ep.Models)
	assert.Equal(t, "https://api.test.com/anthropic", ep.BaseURL)
}

// ---------------------------------------------------------------------------
// Credential override
// ---------------------------------------------------------------------------

func TestProvider_CredentialOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "override-key", r.Header.Get("x-api-key"))

		response := providerbase.AnthropicCompatResponse{
			ID:    "msg_override",
			Type:  "message",
			Role:  "assistant",
			Model: "test-model",
			Content: []providerbase.AnthropicCompatContent{
				{Type: "text", Text: "OK"},
			},
			StopReason: "end_turn",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-anthropic",
		BaseURL:      server.URL,
		DefaultModel: "test-model",
		APIKey:       "default-key",
	}, zap.NewNop())

	ctx := llm.WithCredentialOverride(context.Background(), llm.CredentialOverride{
		APIKey: "override-key",
	})

	resp, err := p.Completion(ctx, &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "OK", resp.Choices[0].Message.Content)
}

// ---------------------------------------------------------------------------
// Stream
// ---------------------------------------------------------------------------

func TestProvider_Stream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/messages", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		// message_start
		fmt.Fprintf(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_s_001\",\"model\":\"test-model\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":10}}}\n\n")
		flusher.Flush()

		// content_block_start
		fmt.Fprintf(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		flusher.Flush()

		// text_delta
		fmt.Fprintf(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n")
		flusher.Flush()

		// content_block_stop
		fmt.Fprintf(w, "data: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		flusher.Flush()

		// message_delta
		fmt.Fprintf(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n")
		flusher.Flush()

		// message_stop
		fmt.Fprintf(w, "data: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-anthropic",
		BaseURL:      server.URL,
		DefaultModel: "test-model",
		APIKey:       "test-key",
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var content string
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("unexpected error: %v", chunk.Err)
		}
		content += chunk.Delta.Content
	}
	assert.Contains(t, content, "Hello")
}

// ---------------------------------------------------------------------------
// Stream error
// ---------------------------------------------------------------------------

func TestProvider_Stream_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"too many requests"}}`))
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-anthropic",
		BaseURL:      server.URL,
		APIKey:       "test-key",
	}, zap.NewNop())

	_, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// API key round-robin
// ---------------------------------------------------------------------------

func TestProvider_APIKeyRoundRobin(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("x-api-key")
		if callCount == 0 {
			assert.Equal(t, "key-a", key)
		} else {
			assert.Equal(t, "key-b", key)
		}
		callCount++

		response := providerbase.AnthropicCompatResponse{
			ID:    fmt.Sprintf("msg_rr_%d", callCount),
			Type:  "message",
			Role:  "assistant",
			Model: "test-model",
			Content: []providerbase.AnthropicCompatContent{
				{Type: "text", Text: "OK"},
			},
			StopReason: "end_turn",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-anthropic",
		BaseURL:      server.URL,
		DefaultModel: "test-model",
		APIKeys: []providers.APIKeyEntry{
			{Key: "key-a"},
			{Key: "key-b"},
		},
	}, zap.NewNop())

	// First call uses key-a
	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	// Second call uses key-b
	_, err = p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	assert.Equal(t, 2, callCount)
}

// ---------------------------------------------------------------------------
// Structured output
// ---------------------------------------------------------------------------

func TestProvider_StructuredOutput(t *testing.T) {
	p := New(Config{ProviderName: "test-anthropic", APIKey: "key"}, nil)
	assert.False(t, p.SupportsStructuredOutput())
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func boolPtr(b bool) *bool {
	return &b
}
