package openaicompat

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
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// SupportsStructuredOutput
// ---------------------------------------------------------------------------

func TestProvider_SupportsStructuredOutput(t *testing.T) {
	p := New(Config{ProviderName: "test"}, nil)
	assert.True(t, p.SupportsStructuredOutput())
}

// ---------------------------------------------------------------------------
// ApplyHeaders
// ---------------------------------------------------------------------------

func TestProvider_ApplyHeaders_DefaultBearer(t *testing.T) {
	p := New(Config{ProviderName: "test"}, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	p.ApplyHeaders(req, "my-key")
	assert.Equal(t, "Bearer my-key", req.Header.Get("Authorization"))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
}

func TestProvider_ApplyHeaders_CustomAuthHeaderName(t *testing.T) {
	p := New(Config{ProviderName: "test", AuthHeaderName: "X-Api-Key"}, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	p.ApplyHeaders(req, "my-key")
	assert.Equal(t, "my-key", req.Header.Get("X-Api-Key"))
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestProvider_ApplyHeaders_CustomBuildHeaders(t *testing.T) {
	p := New(Config{ProviderName: "test"}, nil)
	p.SetBuildHeaders(func(r *http.Request, apiKey string) {
		r.Header.Set("X-Custom-Auth", apiKey)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	p.ApplyHeaders(req, "k")
	assert.Equal(t, "k", req.Header.Get("X-Custom-Auth"))
	assert.Empty(t, req.Header.Get("Authorization"))
}

// ---------------------------------------------------------------------------
// ResolveAPIKey
// ---------------------------------------------------------------------------

func TestProvider_ResolveAPIKey_RoundRobin(t *testing.T) {
	p := New(Config{
		ProviderName: "test",
		APIKey:       "fallback",
		APIKeys: []providers.APIKeyEntry{
			{Key: "k1"},
			{Key: "k2"},
			{Key: "k3"},
		},
	}, nil)

	ctx := context.Background()
	assert.Equal(t, "k1", p.ResolveAPIKey(ctx))
	assert.Equal(t, "k2", p.ResolveAPIKey(ctx))
	assert.Equal(t, "k3", p.ResolveAPIKey(ctx))
	assert.Equal(t, "k1", p.ResolveAPIKey(ctx)) // wraps around
}

func TestProvider_ResolveAPIKey_ContextOverrideTakesPrecedence(t *testing.T) {
	p := New(Config{
		ProviderName: "test",
		APIKeys:      []providers.APIKeyEntry{{Key: "k1"}},
	}, nil)

	ctx := llm.WithCredentialOverride(context.Background(), llm.CredentialOverride{APIKey: "override"})
	assert.Equal(t, "override", p.ResolveAPIKey(ctx))
}

func TestProvider_ResolveAPIKey_FallbackToSingleKey(t *testing.T) {
	p := New(Config{ProviderName: "test", APIKey: "single"}, nil)
	assert.Equal(t, "single", p.ResolveAPIKey(context.Background()))
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

func TestProvider_Endpoints(t *testing.T) {
	p := New(Config{
		ProviderName: "test",
		BaseURL:      "https://api.example.com",
	}, nil)

	ep := p.Endpoints()
	assert.Equal(t, "https://api.example.com/v1/chat/completions", ep.Completion)
	assert.Equal(t, "https://api.example.com/v1/models", ep.Models)
	assert.Equal(t, "https://api.example.com", ep.BaseURL)
}

func TestProvider_Endpoints_CustomPaths(t *testing.T) {
	p := New(Config{
		ProviderName:   "test",
		BaseURL:        "https://api.example.com/",
		EndpointPath:   "/api/chat",
		ModelsEndpoint: "/api/models",
	}, nil)

	ep := p.Endpoints()
	assert.Equal(t, "https://api.example.com/api/chat", ep.Completion)
	assert.Equal(t, "https://api.example.com/api/models", ep.Models)
}

// ---------------------------------------------------------------------------
// DoJSON error paths
// ---------------------------------------------------------------------------

func TestProvider_DoJSON_NilPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	err := p.DoJSON(context.Background(), http.MethodGet, "/v1/models", nil, "key", nil)
	require.NoError(t, err)
}

func TestProvider_DoJSON_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"message":"forbidden"}}`)
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	var out map[string]any
	err := p.DoJSON(context.Background(), http.MethodPost, "/v1/chat/completions", map[string]string{"a": "b"}, "key", &out)
	require.Error(t, err)
}

func TestProvider_DoJSON_InvalidResponseJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `not-json`)
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	var out map[string]any
	err := p.DoJSON(context.Background(), http.MethodPost, "/v1/chat/completions", map[string]string{"a": "b"}, "key", &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid character")
}

func TestProvider_DoJSON_NilOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	err := p.DoJSON(context.Background(), http.MethodPost, "/v1/chat/completions", map[string]string{"a": "b"}, "key", nil)
	require.NoError(t, err)
}

func TestProvider_DoJSON_NetworkError(t *testing.T) {
	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: "http://127.0.0.1:1"}, zap.NewNop())
	var out map[string]any
	err := p.DoJSON(context.Background(), http.MethodPost, "/v1/chat/completions", map[string]string{"a": "b"}, "key", &out)
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// HealthCheck edge cases
// ---------------------------------------------------------------------------

func TestProvider_HealthCheck_NetworkError(t *testing.T) {
	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: "http://127.0.0.1:1"}, zap.NewNop())
	status, err := p.HealthCheck(context.Background())
	require.Error(t, err)
	assert.False(t, status.Healthy)
	assert.True(t, status.Latency >= 0)
}

func TestProvider_HealthCheck_CustomAuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-key", r.Header.Get("X-Api-Key"))
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"object":"list","data":[]}`)
	}))
	t.Cleanup(server.Close)

	p := New(Config{
		ProviderName:   "test",
		APIKey:         "test-key",
		BaseURL:        server.URL,
		AuthHeaderName: "X-Api-Key",
	}, zap.NewNop())
	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
}

// ---------------------------------------------------------------------------
// Do - network error wrapping
// ---------------------------------------------------------------------------

func TestProvider_Do_NetworkError(t *testing.T) {
	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: "http://127.0.0.1:1"}, zap.NewNop())
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:1/test", nil)
	require.NoError(t, err)
	_, err = p.Do(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connect")
}

// ---------------------------------------------------------------------------
// NewRequest - invalid URL
// ---------------------------------------------------------------------------

func TestProvider_NewRequest_Success(t *testing.T) {
	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: "https://api.example.com"}, nil)
	req, err := p.NewRequest(context.Background(), http.MethodPost, "/v1/chat/completions", nil, "key")
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/v1/chat/completions", req.URL.String())
	assert.Equal(t, "Bearer key", req.Header.Get("Authorization"))
}

// ---------------------------------------------------------------------------
// Stream with StreamOptions
// ---------------------------------------------------------------------------

func TestProvider_Stream_WithStreamOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.True(t, body["stream"].(bool))
		assert.NotNil(t, body["stream_options"])

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
		StreamOptions: &llm.StreamOptions{
			IncludeUsage: true,
		},
	})
	require.NoError(t, err)
	for range ch {
	}
}

// ---------------------------------------------------------------------------
// Stream with ReasoningEffort
// ---------------------------------------------------------------------------

func TestProvider_Stream_WithReasoningEffort(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages:        []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
		ReasoningEffort: "high",
	})
	require.NoError(t, err)
	for range ch {
	}
	assert.Equal(t, "high", capturedBody["reasoning_effort"])
}

// ---------------------------------------------------------------------------
// Completion with ReasoningEffort
// ---------------------------------------------------------------------------

func TestProvider_Completion_WithReasoningEffort(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "r1", "model": "m",
			"choices": []map[string]any{
				{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages:        []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
		ReasoningEffort: "medium",
	})
	require.NoError(t, err)
	assert.Equal(t, "medium", capturedBody["reasoning_effort"])
}

// ---------------------------------------------------------------------------
// Stream usage-only chunk (no choices)
// ---------------------------------------------------------------------------

func TestProvider_Stream_UsageOnlyChunk(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// chunk with usage but no choices
		usageChunk := `{"id":"s1","model":"m","usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15},"choices":[]}`
		fmt.Fprintf(w, "data: %s\n\n", usageChunk)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var gotUsage bool
	for chunk := range ch {
		require.Nil(t, chunk.Err)
		if chunk.Usage != nil {
			gotUsage = true
			assert.Equal(t, 15, chunk.Usage.TotalTokens)
		}
	}
	assert.True(t, gotUsage)
}

func TestProvider_Completion_PreservesReasoningContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":    "r_reasoning",
			"model": "m",
			"choices": []map[string]any{
				{
					"index":         0,
					"finish_reason": "stop",
					"message": map[string]any{
						"role":              "assistant",
						"content":           "ok",
						"reasoning_content": "compat reasoning",
					},
				},
			},
		})
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	require.NotNil(t, resp.Choices[0].Message.ReasoningContent)
	assert.Equal(t, "compat reasoning", *resp.Choices[0].Message.ReasoningContent)
}

func TestProvider_Stream_PreservesReasoningContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := `{"id":"s1","model":"m","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"compat delta"},"finish_reason":""}]}`
		fmt.Fprintf(w, "data: %s\n\n", chunk)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var got *string
	for chunk := range ch {
		require.Nil(t, chunk.Err)
		if chunk.Delta.ReasoningContent != nil {
			got = chunk.Delta.ReasoningContent
		}
	}
	require.NotNil(t, got)
	assert.Equal(t, "compat delta", *got)
}

// ---------------------------------------------------------------------------
// convertWebSearchOptions nil
// ---------------------------------------------------------------------------

func TestConvertWebSearchOptions_Nil(t *testing.T) {
	assert.Nil(t, convertWebSearchOptions(nil))
}

func TestConvertWebSearchOptions_NoLocation(t *testing.T) {
	result := convertWebSearchOptions(&llm.WebSearchOptions{
		SearchContextSize: "low",
	})
	require.NotNil(t, result)
	assert.Equal(t, "low", result.SearchContextSize)
	assert.Nil(t, result.UserLocation)
}
