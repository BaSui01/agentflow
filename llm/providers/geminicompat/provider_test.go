package geminicompat

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
		wantModels       string
		wantName         string
		wantToolsSupport bool
	}{
		{
			name:             "all defaults applied",
			cfg:              Config{ProviderName: "test-gemini"},
			wantModels:       "/v1beta/models",
			wantName:         "test-gemini",
			wantToolsSupport: true,
		},
		{
			name: "custom models endpoint",
			cfg: Config{
				ProviderName:   "custom-gemini",
				ModelsEndpoint: "/api/models",
			},
			wantModels:       "/api/models",
			wantName:         "custom-gemini",
			wantToolsSupport: true,
		},
		{
			name: "supports tools false",
			cfg: Config{
				ProviderName:  "no-tools-gemini",
				SupportsTools: boolPtr(false),
			},
			wantModels:       "/v1beta/models",
			wantName:         "no-tools-gemini",
			wantToolsSupport: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.cfg, zap.NewNop())
			require.NotNil(t, p)
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
		assert.Contains(t, r.URL.Path, ":generateContent")
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body providerbase.GeminiCompatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.NotEmpty(t, body.Contents)

		response := providerbase.GeminiCompatResponse{
			ModelVersion: "gemini-2.5-flash",
			Candidates: []providerbase.GeminiCompatCandidate{
				{
					Index: 0,
					Content: &providerbase.GeminiCompatContent{
						Role: "model",
						Parts: []providerbase.GeminiCompatPart{
							{Text: "Hello from Gemini compat!"},
						},
					},
					FinishReason: "STOP",
				},
			},
			UsageMetadata: &providerbase.GeminiCompatUsage{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
				TotalTokenCount:      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-gemini",
		BaseURL:      server.URL,
		DefaultModel: "gemini-2.5-flash",
		APIKey:       "test-key",
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "test-gemini", resp.Provider)
	assert.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from Gemini compat!", resp.Choices[0].Message.Content)
	assert.Equal(t, 10, resp.Usage.PromptTokens)
	assert.Equal(t, 5, resp.Usage.CompletionTokens)
}

// ---------------------------------------------------------------------------
// Completion HTTP error
// ---------------------------------------------------------------------------

func TestProvider_Completion_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"code":401,"message":"invalid api key","status":"UNAUTHENTICATED"}}`))
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-gemini",
		BaseURL:      server.URL,
		APIKey:       "bad-key",
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Completion with tool call
// ---------------------------------------------------------------------------

func TestProvider_Completion_WithToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body providerbase.GeminiCompatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Len(t, body.Tools, 1)
		assert.Len(t, body.Tools[0].FunctionDeclarations, 1)
		assert.Equal(t, "get_weather", body.Tools[0].FunctionDeclarations[0].Name)

		response := providerbase.GeminiCompatResponse{
			ModelVersion: "gemini-2.5-flash",
			Candidates: []providerbase.GeminiCompatCandidate{
				{
					Index: 0,
					Content: &providerbase.GeminiCompatContent{
						Role: "model",
						Parts: []providerbase.GeminiCompatPart{
							{
								FunctionCall: &providerbase.GeminiCompatFuncCall{
									Name: "get_weather",
									Args: map[string]any{"city": "Beijing"},
								},
							},
						},
					},
					FinishReason: "STOP",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-gemini",
		BaseURL:      server.URL,
		DefaultModel: "gemini-2.5-flash",
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
		if r.URL.Path == "/v1beta/models" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"models":[]}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-gemini",
		BaseURL:      server.URL,
		APIKey:       "test-key",
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
}

// ---------------------------------------------------------------------------
// List models
// ---------------------------------------------------------------------------

func TestProvider_ListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"models":[{"name":"models/gemini-2.5-flash","displayName":"Gemini 2.5 Flash","inputTokenLimit":1048576,"outputTokenLimit":8192}]}`))
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-gemini",
		BaseURL:      server.URL,
		APIKey:       "test-key",
	}, zap.NewNop())

	models, err := p.ListModels(context.Background())
	require.NoError(t, err)
	assert.Len(t, models, 1)
	assert.Equal(t, "gemini-2.5-flash", models[0].ID)
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

func TestProvider_Endpoints(t *testing.T) {
	p := New(Config{
		ProviderName:  "test-gemini",
		BaseURL:       "https://api.test.com/gemini",
		DefaultModel:  "gemini-2.5-pro",
	}, zap.NewNop())

	ep := p.Endpoints()
	assert.Contains(t, ep.Completion, ":generateContent")
	assert.Contains(t, ep.Models, "/v1beta/models")
	assert.Equal(t, "https://api.test.com/gemini", ep.BaseURL)
}

// ---------------------------------------------------------------------------
// Credential override
// ---------------------------------------------------------------------------

func TestProvider_CredentialOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "override-key", r.Header.Get("x-goog-api-key"))

		response := providerbase.GeminiCompatResponse{
			ModelVersion: "gemini-2.5-flash",
			Candidates: []providerbase.GeminiCompatCandidate{
				{
					Index: 0,
					Content: &providerbase.GeminiCompatContent{
						Role: "model",
						Parts: []providerbase.GeminiCompatPart{
							{Text: "OK"},
						},
					},
					FinishReason: "STOP",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-gemini",
		BaseURL:      server.URL,
		DefaultModel: "gemini-2.5-flash",
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
		assert.Contains(t, r.URL.Path, "streamGenerateContent")
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		response := providerbase.GeminiCompatResponse{
			ModelVersion: "gemini-2.5-flash",
			Candidates: []providerbase.GeminiCompatCandidate{
				{
					Index: 0,
					Content: &providerbase.GeminiCompatContent{
						Role: "model",
						Parts: []providerbase.GeminiCompatPart{
							{Text: "Hello"},
						},
					},
				},
			},
		}
		data, _ := json.Marshal(response)
		fmt.Fprintf(w, "[%s]\n", data)
		flusher.Flush()
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-gemini",
		BaseURL:      server.URL,
		DefaultModel: "gemini-2.5-flash",
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
// API key round-robin
// ---------------------------------------------------------------------------

func TestProvider_APIKeyRoundRobin(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("x-goog-api-key")
		if callCount == 0 {
			assert.Equal(t, "key-a", key)
		} else {
			assert.Equal(t, "key-b", key)
		}
		callCount++

		response := providerbase.GeminiCompatResponse{
			ModelVersion: "gemini-2.5-flash",
			Candidates: []providerbase.GeminiCompatCandidate{
				{
					Index: 0,
					Content: &providerbase.GeminiCompatContent{
						Role: "model",
						Parts: []providerbase.GeminiCompatPart{
							{Text: "OK"},
						},
					},
					FinishReason: "STOP",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	p := New(Config{
		ProviderName: "test-gemini",
		BaseURL:      server.URL,
		DefaultModel: "gemini-2.5-flash",
		APIKeys: []providers.APIKeyEntry{
			{Key: "key-a"},
			{Key: "key-b"},
		},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

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
	p := New(Config{ProviderName: "test-gemini", APIKey: "key"}, nil)
	assert.True(t, p.SupportsStructuredOutput())
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func boolPtr(b bool) *bool {
	return &b
}
