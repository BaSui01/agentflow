package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
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
		logger           *zap.Logger
		wantEndpoint     string
		wantModels       string
		wantName         string
		wantToolsSupport bool
	}{
		{
			name:             "all defaults applied",
			cfg:              Config{ProviderName: "test"},
			logger:           nil,
			wantEndpoint:     "/v1/chat/completions",
			wantModels:       "/v1/models",
			wantName:         "test",
			wantToolsSupport: true,
		},
		{
			name: "custom endpoint paths preserved",
			cfg: Config{
				ProviderName:   "custom",
				EndpointPath:   "/api/chat",
				ModelsEndpoint: "/api/models",
			},
			logger:           zap.NewNop(),
			wantEndpoint:     "/api/chat",
			wantModels:       "/api/models",
			wantName:         "custom",
			wantToolsSupport: true,
		},
		{
			name: "supports tools false",
			cfg: Config{
				ProviderName:  "no-tools",
				SupportsTools: boolPtr(false),
			},
			logger:           zap.NewNop(),
			wantEndpoint:     "/v1/chat/completions",
			wantModels:       "/v1/models",
			wantName:         "no-tools",
			wantToolsSupport: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.cfg, tt.logger)
			require.NotNil(t, p)
			assert.Equal(t, tt.wantEndpoint, p.Cfg.EndpointPath)
			assert.Equal(t, tt.wantModels, p.Cfg.ModelsEndpoint)
			assert.Equal(t, tt.wantName, p.Name())
			assert.Equal(t, tt.wantToolsSupport, p.SupportsNativeFunctionCalling())
			assert.NotNil(t, p.Client)
			assert.NotNil(t, p.Logger)
			assert.NotNil(t, p.RewriterChain)
		})
	}
}

func TestNew_TimeoutDefault(t *testing.T) {
	p := New(Config{ProviderName: "t"}, nil)
	assert.Equal(t, 30*time.Second, p.Client.Timeout)
}

func TestNew_TimeoutCustom(t *testing.T) {
	p := New(Config{ProviderName: "t", Timeout: 10 * time.Second}, nil)
	assert.Equal(t, 10*time.Second, p.Client.Timeout)
}

// ---------------------------------------------------------------------------
// SetBuildHeaders
// ---------------------------------------------------------------------------

func TestSetBuildHeaders(t *testing.T) {
	p := New(Config{ProviderName: "test", APIKey: "key"}, nil)

	called := false
	p.SetBuildHeaders(func(r *http.Request, apiKey string) {
		called = true
		r.Header.Set("X-Custom", "yes")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	p.buildHeaders(req, "key")
	assert.True(t, called)
	assert.Equal(t, "yes", req.Header.Get("X-Custom"))
}

// ---------------------------------------------------------------------------
// Completion
// ---------------------------------------------------------------------------

func TestProvider_Completion_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer test-key")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID:    "resp-1",
			Model: "gpt-test",
			Choices: []providers.OpenAICompatChoice{
				{
					Index:        0,
					FinishReason: "stop",
					Message: providers.OpenAICompatMessage{
						Role:    "assistant",
						Content: "Hello!",
					},
				},
			},
			Usage: &providers.OpenAICompatUsage{
				PromptTokens:     5,
				CompletionTokens: 2,
				TotalTokens:      7,
			},
			Created: 1700000000,
		})
	}))
	t.Cleanup(server.Close)

	p := New(Config{
		ProviderName: "test",
		APIKey:       "test-key",
		BaseURL:      server.URL,
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "resp-1", resp.ID)
	assert.Equal(t, "test", resp.Provider)
	assert.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello!", resp.Choices[0].Message.Content)
	assert.Equal(t, 7, resp.Usage.TotalTokens)
	assert.False(t, resp.CreatedAt.IsZero())
}

func TestProvider_Completion_HTTPError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantCode   llm.ErrorCode
	}{
		{
			name:       "401 unauthorized",
			statusCode: http.StatusUnauthorized,
			body:       `{"error":{"message":"invalid key","type":"auth"}}`,
			wantCode:   llm.ErrUnauthorized,
		},
		{
			name:       "429 rate limited",
			statusCode: http.StatusTooManyRequests,
			body:       `{"error":{"message":"slow down"}}`,
			wantCode:   llm.ErrRateLimited,
		},
		{
			name:       "500 server error",
			statusCode: http.StatusInternalServerError,
			body:       `{"error":{"message":"oops"}}`,
			wantCode:   llm.ErrUpstreamError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				fmt.Fprint(w, tt.body)
			}))
			t.Cleanup(server.Close)

			p := New(Config{
				ProviderName: "test",
				APIKey:       "key",
				BaseURL:      server.URL,
			}, zap.NewNop())

			_, err := p.Completion(context.Background(), &llm.ChatRequest{
				Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
			})
			require.Error(t, err)
			var llmErr *llm.Error
			require.ErrorAs(t, err, &llmErr)
			assert.Equal(t, tt.wantCode, llmErr.Code)
		})
	}
}

func TestProvider_Completion_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "not json")
	}))
	t.Cleanup(server.Close)

	p := New(Config{
		ProviderName: "test",
		APIKey:       "key",
		BaseURL:      server.URL,
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	var llmErr *llm.Error
	require.ErrorAs(t, err, &llmErr)
	assert.Equal(t, llm.ErrUpstreamError, llmErr.Code)
}

func TestProvider_Completion_CredentialOverride(t *testing.T) {
	var capturedKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if len(auth) > 7 {
			capturedKey = auth[7:]
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID:    "r1",
			Model: "m",
			Choices: []providers.OpenAICompatChoice{
				{Index: 0, FinishReason: "stop", Message: providers.OpenAICompatMessage{Role: "assistant", Content: "ok"}},
			},
		})
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "cfg-key", BaseURL: server.URL}, zap.NewNop())

	ctx := llm.WithCredentialOverride(context.Background(), llm.CredentialOverride{APIKey: "override-key"})
	_, err := p.Completion(ctx, &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "override-key", capturedKey)
}

func TestProvider_Completion_RequestHook(t *testing.T) {
	var receivedModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body providers.OpenAICompatRequest
		json.NewDecoder(r.Body).Decode(&body)
		receivedModel = body.Model
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID: "r1", Model: receivedModel,
			Choices: []providers.OpenAICompatChoice{
				{Index: 0, FinishReason: "stop", Message: providers.OpenAICompatMessage{Role: "assistant", Content: "ok"}},
			},
		})
	}))
	t.Cleanup(server.Close)

	p := New(Config{
		ProviderName:  "test",
		APIKey:        "key",
		BaseURL:       server.URL,
		DefaultModel:  "default-model",
		RequestHook: func(req *llm.ChatRequest, body *providers.OpenAICompatRequest) {
			body.Model = "hooked-model"
		},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "hooked-model", receivedModel)
}

// ---------------------------------------------------------------------------
// Stream
// ---------------------------------------------------------------------------

func TestProvider_Stream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []providers.OpenAICompatResponse{
			{
				ID: "s1", Model: "m",
				Choices: []providers.OpenAICompatChoice{
					{Index: 0, Delta: &providers.OpenAICompatMessage{Role: "assistant", Content: "Hel"}},
				},
			},
			{
				ID: "s1", Model: "m",
				Choices: []providers.OpenAICompatChoice{
					{Index: 0, Delta: &providers.OpenAICompatMessage{Content: "lo"}},
				},
			},
			{
				ID: "s1", Model: "m",
				Choices: []providers.OpenAICompatChoice{
					{Index: 0, FinishReason: "stop", Delta: &providers.OpenAICompatMessage{}},
				},
			},
		}
		for _, c := range chunks {
			data, _ := json.Marshal(c)
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var content string
	var lastFinish string
	for chunk := range ch {
		require.Nil(t, chunk.Err)
		content += chunk.Delta.Content
		if chunk.FinishReason != "" {
			lastFinish = chunk.FinishReason
		}
	}
	assert.Equal(t, "Hello", content)
	assert.Equal(t, "stop", lastFinish)
}

func TestProvider_Stream_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"rate limited"}}`)
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())

	_, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	var llmErr *llm.Error
	require.ErrorAs(t, err, &llmErr)
	assert.Equal(t, llm.ErrRateLimited, llmErr.Code)
}

func TestProvider_Stream_ToolCallDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := providers.OpenAICompatResponse{
			ID: "s1", Model: "m",
			Choices: []providers.OpenAICompatChoice{
				{
					Index: 0,
					Delta: &providers.OpenAICompatMessage{
						ToolCalls: []providers.OpenAICompatToolCall{
							{ID: "tc1", Type: "function", Function: providers.OpenAICompatFunction{Name: "calc", Arguments: json.RawMessage(`{"x":1}`)}},
						},
					},
				},
			},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var toolCalls []llm.ToolCall
	for chunk := range ch {
		require.Nil(t, chunk.Err)
		toolCalls = append(toolCalls, chunk.Delta.ToolCalls...)
	}
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "calc", toolCalls[0].Name)
	assert.Equal(t, "tc1", toolCalls[0].ID)
}

// ---------------------------------------------------------------------------
// HealthCheck
// ---------------------------------------------------------------------------

func TestProvider_HealthCheck_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"object":"list","data":[]}`)
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
	assert.True(t, status.Latency >= 0)
}

func TestProvider_HealthCheck_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"bad key"}}`)
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	status, err := p.HealthCheck(context.Background())
	require.Error(t, err)
	assert.False(t, status.Healthy)
}

// ---------------------------------------------------------------------------
// ListModels
// ---------------------------------------------------------------------------

func TestProvider_ListModels_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Contains(t, r.URL.Path, "/v1/models")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Object string     `json:"object"`
			Data   []llm.Model `json:"data"`
		}{
			Object: "list",
			Data: []llm.Model{
				{ID: "model-a", OwnedBy: "test"},
				{ID: "model-b", OwnedBy: "test"},
			},
		})
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	models, err := p.ListModels(context.Background())
	require.NoError(t, err)
	assert.Len(t, models, 2)
	assert.Equal(t, "model-a", models[0].ID)
}

func TestProvider_ListModels_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"message":"forbidden"}}`)
	}))
	t.Cleanup(server.Close)

	p := New(Config{ProviderName: "test", APIKey: "key", BaseURL: server.URL}, zap.NewNop())
	_, err := p.ListModels(context.Background())
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// resolveAPIKey
// ---------------------------------------------------------------------------

func TestProvider_resolveAPIKey(t *testing.T) {
	p := New(Config{ProviderName: "test", APIKey: "cfg-key"}, nil)

	// No override
	assert.Equal(t, "cfg-key", p.resolveAPIKey(context.Background()))

	// With override
	ctx := llm.WithCredentialOverride(context.Background(), llm.CredentialOverride{APIKey: "ctx-key"})
	assert.Equal(t, "ctx-key", p.resolveAPIKey(ctx))

	// Whitespace override falls back
	ctx = llm.WithCredentialOverride(context.Background(), llm.CredentialOverride{APIKey: "   "})
	assert.Equal(t, "cfg-key", p.resolveAPIKey(ctx))
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func boolPtr(b bool) *bool { return &b }
