package doubao

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/types"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDoubaoProvider_CreateContextCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v3/context/create", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req ContextCacheRequest
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "test-model", req.Model)
		assert.Equal(t, "session", req.Mode)
		assert.Equal(t, 3600, req.TTL)
		assert.Len(t, req.Messages, 1)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ContextCacheResponse{
			ID:        "ctx-123",
			Model:     "test-model",
			CreatedAt: 1700000000,
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := newDoubaoProvider(providers.DoubaoConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}, zap.NewNop())

	resp, err := p.CreateContextCache(context.Background(), "test-model", []types.Message{
		{Role: llm.RoleSystem, Content: "You are helpful"},
	}, "session", 3600)
	require.NoError(t, err)
	assert.Equal(t, "ctx-123", resp.ID)
}

func TestDoubaoProvider_CompletionWithContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v3/context/chat/completions", r.URL.Path)

		var req ContextChatRequest
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "ctx-123", req.ContextID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providerbase.OpenAICompatResponse{
			ID:    "resp-1",
			Model: "Doubao-1.5-pro-32k",
			Choices: []providerbase.OpenAICompatChoice{
				{
					Index:        0,
					FinishReason: "stop",
					Message: providerbase.OpenAICompatMessage{
						Role:    "assistant",
						Content: "Hello with context",
					},
				},
			},
			Usage: &providerbase.OpenAICompatUsage{
				PromptTokens:     5,
				CompletionTokens: 3,
				TotalTokens:      8,
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := newDoubaoProvider(providers.DoubaoConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}, zap.NewNop())

	resp, err := p.CompletionWithContext(context.Background(), "ctx-123", &llm.ChatRequest{
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: "Hi"},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "Hello with context", resp.Choices[0].Message.Content)
}

func TestDoubaoProvider_CreateContextCache_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "invalid model",
			},
		})
	}))
	t.Cleanup(func() { server.Close() })

	p := newDoubaoProvider(providers.DoubaoConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		},
	}, zap.NewNop())

	_, err := p.CreateContextCache(context.Background(), "bad-model", nil, "", 0)
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code)
}
