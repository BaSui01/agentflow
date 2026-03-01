package gemini

import (
	"github.com/BaSui01/agentflow/types"
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

// --- Constructor ---

func TestNewGeminiProvider_Defaults(t *testing.T) {
	tests := []struct {
		name            string
		cfg             providers.GeminiConfig
		expectedBaseURL string
	}{
		{
			name:            "empty config uses default BaseURL",
			cfg:             providers.GeminiConfig{},
			expectedBaseURL: "https://generativelanguage.googleapis.com",
		},
		{
			name: "custom BaseURL is preserved",
			cfg: providers.GeminiConfig{
				BaseProviderConfig: providers.BaseProviderConfig{
					BaseURL: "https://custom.example.com",
				},
			},
			expectedBaseURL: "https://custom.example.com",
		},
		{
			name: "Vertex AI mode sets region-based BaseURL",
			cfg: providers.GeminiConfig{
				ProjectID: "my-project",
			},
			expectedBaseURL: "https://us-central1-aiplatform.googleapis.com",
		},
		{
			name: "Vertex AI with custom region",
			cfg: providers.GeminiConfig{
				ProjectID: "my-project",
				Region:    "europe-west1",
			},
			expectedBaseURL: "https://europe-west1-aiplatform.googleapis.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewGeminiProvider(tt.cfg, zap.NewNop())
			require.NotNil(t, p)
			assert.Equal(t, "gemini", p.Name())
			assert.Equal(t, tt.expectedBaseURL, p.cfg.BaseURL)
		})
	}
}

func TestGeminiProvider_VertexAI_DefaultRegion(t *testing.T) {
	p := NewGeminiProvider(providers.GeminiConfig{ProjectID: "proj"}, zap.NewNop())
	assert.Equal(t, "us-central1", p.cfg.Region)
}

func TestGeminiProvider_SupportsNativeFunctionCalling(t *testing.T) {
	p := NewGeminiProvider(providers.GeminiConfig{}, zap.NewNop())
	assert.True(t, p.SupportsNativeFunctionCalling())
}

// --- Endpoints ---

func TestGeminiProvider_Endpoints_Standard(t *testing.T) {
	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://generativelanguage.googleapis.com",
			Model:   "gemini-3-pro",
		},
	}, zap.NewNop())
	ep := p.Endpoints()
	assert.Contains(t, ep.Completion, "v1beta/models/gemini-3-pro:generateContent")
	assert.Contains(t, ep.Stream, "v1beta/models/gemini-3-pro:streamGenerateContent")
	assert.Contains(t, ep.Models, "v1beta/models")
}

func TestGeminiProvider_Endpoints_VertexAI(t *testing.T) {
	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{Model: "gemini-3-pro"},
		ProjectID:          "my-proj",
		Region:             "us-central1",
	}, zap.NewNop())
	ep := p.Endpoints()
	assert.Contains(t, ep.Completion, "projects/my-proj/locations/us-central1")
	assert.Contains(t, ep.Stream, "projects/my-proj/locations/us-central1")
}

// --- Headers ---

func TestGeminiProvider_Headers_APIKey(t *testing.T) {
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(geminiResponse{
			Candidates: []geminiCandidate{{
				Content:      geminiContent{Role: "model", Parts: []geminiPart{{Text: "ok"}}},
				FinishReason: "STOP",
			}},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "test-key", capturedHeaders.Get("x-goog-api-key"))
}

func TestGeminiProvider_Headers_OAuth(t *testing.T) {
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{Role: "model", Parts: []geminiPart{{Text: "ok"}}},
			}},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "oauth-token", BaseURL: server.URL},
		AuthType:           "oauth",
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer oauth-token", capturedHeaders.Get("Authorization"))
}

// --- convertToGeminiContents ---

func TestConvertToGeminiContents(t *testing.T) {
	msgs := []types.Message{
		{Role: llm.RoleSystem, Content: "You are helpful"},
		{Role: llm.RoleUser, Content: "Hello"},
		{Role: llm.RoleAssistant, Content: "Hi there"},
	}

	sysInstr, contents := convertToGeminiContents(msgs)
	require.NotNil(t, sysInstr)
	assert.Equal(t, "You are helpful", sysInstr.Parts[0].Text)
	require.Len(t, contents, 2)
	assert.Equal(t, "user", contents[0].Role)
	assert.Equal(t, "model", contents[1].Role) // assistant -> model
}

func TestConvertToGeminiContents_ToolRoleMappedToUser(t *testing.T) {
	msgs := []types.Message{
		{Role: llm.RoleTool, ToolCallID: "tc-1", Name: "search", Content: `{"ok":true}`},
	}
	_, contents := convertToGeminiContents(msgs)
	require.Len(t, contents, 1)
	assert.Equal(t, "user", contents[0].Role)
	require.Len(t, contents[0].Parts, 1)
	require.NotNil(t, contents[0].Parts[0].FunctionResponse)
	assert.Equal(t, "search", contents[0].Parts[0].FunctionResponse.Name)
}

// --- convertGeminiCapabilities ---

func TestConvertGeminiCapabilities(t *testing.T) {
	caps := convertGeminiCapabilities([]string{"generateContent", "embedContent", "unknownMethod"})
	assert.Contains(t, caps, "chat")
	assert.Contains(t, caps, "embedding")
	assert.Len(t, caps, 2)

	assert.Nil(t, convertGeminiCapabilities(nil))
	assert.Nil(t, convertGeminiCapabilities([]string{"unknownOnly"}))
}

// --- Completion via httptest ---

func TestGeminiProvider_Completion(t *testing.T) {
	var capturedRequest geminiRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "generateContent")

		json.NewDecoder(r.Body).Decode(&capturedRequest)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(geminiResponse{
			ResponseID: "resp-gem-1",
			Candidates: []geminiCandidate{{
				Content:      geminiContent{Role: "model", Parts: []geminiPart{{Text: "Hello from Gemini"}}},
				FinishReason: "STOP",
				Index:        0,
			}},
			UsageMetadata: &geminiUsageMetadata{
				PromptTokenCount:     8,
				CandidatesTokenCount: 4,
				TotalTokenCount:      12,
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{
			{Role: llm.RoleSystem, Content: "Be helpful"},
			{Role: llm.RoleUser, Content: "Hi"},
		},
		Temperature: 0.7,
		MaxTokens:   100,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "gemini", resp.Provider)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello from Gemini", resp.Choices[0].Message.Content)
	assert.Equal(t, "stop", resp.Choices[0].FinishReason)
	assert.Equal(t, 12, resp.Usage.TotalTokens)

	// Verify system instruction was extracted
	require.NotNil(t, capturedRequest.SystemInstruction)
	assert.Equal(t, "Be helpful", capturedRequest.SystemInstruction.Parts[0].Text)
	require.NotNil(t, capturedRequest.GenerationConfig)
	assert.InDelta(t, 0.7, capturedRequest.GenerationConfig.Temperature, 0.01)
}

func TestGeminiProvider_Completion_WithToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(geminiResponse{
			ResponseID: "resp-tc",
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role: "model",
					Parts: []geminiPart{
						{Text: "Let me check."},
						{FunctionCall: &geminiFunctionCall{
							Name: "get_weather",
							Args: map[string]any{"city": "NYC"},
						}},
					},
				},
				FinishReason: "STOP",
			}},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Weather?"}},
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Let me check.", resp.Choices[0].Message.Content)
	require.Len(t, resp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "get_weather", resp.Choices[0].Message.ToolCalls[0].Name)
	assert.Contains(t, resp.Choices[0].Message.ToolCalls[0].ID, "call_")
}

func TestGeminiProvider_Completion_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key","code":401}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "bad-key", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Completion(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrUnauthorized, llmErr.Code)
}

// --- Stream via httptest ---

func TestGeminiProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "streamGenerateContent")

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Gemini SSE format: data: {json}\n\n
		chunk1 := geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{Role: "model", Parts: []geminiPart{{Text: "Hello "}}},
				Index:   0,
			}},
		}
		chunk2 := geminiResponse{
			Candidates: []geminiCandidate{{
				Content:      geminiContent{Role: "model", Parts: []geminiPart{{Text: "world"}}},
				FinishReason: "STOP",
				Index:        0,
			}},
			UsageMetadata: &geminiUsageMetadata{
				PromptTokenCount:     5,
				CandidatesTokenCount: 3,
				TotalTokenCount:      8,
			},
		}

		data1, _ := json.Marshal(chunk1)
		data2, _ := json.Marshal(chunk2)
		fmt.Fprintf(w, "data: %s\n\n", data1)
		fmt.Fprintf(w, "data: %s\n\n", data2)
	}))
	t.Cleanup(func() { server.Close() })

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	ch, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for c := range ch {
		require.Nil(t, c.Err)
		chunks = append(chunks, c)
	}

	// Should have content chunks + usage chunk
	require.GreaterOrEqual(t, len(chunks), 2)

	var content string
	for _, c := range chunks {
		content += c.Delta.Content
	}
	assert.Equal(t, "Hello world", content)

	// Check usage in one of the chunks
	var hasUsage bool
	for _, c := range chunks {
		if c.Usage != nil {
			hasUsage = true
			assert.Equal(t, 8, c.Usage.TotalTokens)
		}
	}
	assert.True(t, hasUsage)
}

func TestGeminiProvider_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded","code":429}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.Stream(context.Background(), &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrRateLimit, llmErr.Code)
}

// --- HealthCheck ---

func TestGeminiProvider_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "models")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"models":[]}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
}

func TestGeminiProvider_HealthCheck_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"Internal error"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.Error(t, err)
	assert.False(t, status.Healthy)
}

// --- ListModels ---

func TestGeminiProvider_ListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"models":[{"name":"models/gemini-3-pro","displayName":"Gemini 3 Pro","inputTokenLimit":1000000,"outputTokenLimit":8192,"supportedGenerationMethods":["generateContent","countTokens"]}]}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	models, err := p.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "gemini-3-pro", models[0].ID) // "models/" prefix stripped
	assert.Equal(t, "google", models[0].OwnedBy)
	assert.Equal(t, 1000000, models[0].MaxInputTokens)
	assert.Contains(t, models[0].Capabilities, "chat")
	assert.Contains(t, models[0].Capabilities, "token_counting")
}

func TestGeminiProvider_ListModels_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"message":"Forbidden"}}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.ListModels(context.Background())
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrForbidden, llmErr.Code)
}

func TestGeminiProvider_HealthCheck_UsesResolveAPIKey(t *testing.T) {
	var capturedKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("x-goog-api-key")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"models":[]}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: server.URL,
			APIKeys: []providers.APIKeyEntry{{Key: "multi-key-1"}},
		},
	}, zap.NewNop())

	status, err := p.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
	assert.Equal(t, "multi-key-1", capturedKey)
}

func TestGeminiProvider_ListModels_UsesResolveAPIKey(t *testing.T) {
	var capturedKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("x-goog-api-key")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"models":[]}`))
	}))
	t.Cleanup(func() { server.Close() })

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: server.URL,
			APIKeys: []providers.APIKeyEntry{{Key: "multi-key-2"}},
		},
	}, zap.NewNop())

	_, err := p.ListModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "multi-key-2", capturedKey)
}


