package grok

import (
	"context"
	"encoding/json"
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

func TestGrokProvider_GenerateVideo_Success(t *testing.T) {
	pollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/videos/generations" {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
				"id":     "vid-123",
				"status": "pending",
			}))
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == "/v1/videos/generations/vid-123" {
			pollCount++
			w.Header().Set("Content-Type", "application/json")
			if pollCount >= 2 {
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"id":         "vid-123",
					"status":     "completed",
					"created_at": 1700000000,
					"data":       []map[string]string{{"url": "https://example.com/video.mp4"}},
				}))
			} else {
				require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
					"id":     "vid-123",
					"status": "processing",
				}))
			}
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	p := NewGrokProvider(providers.GrokConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.GenerateVideo(context.Background(), &llm.VideoGenerationRequest{
		Model:  "grok-video",
		Prompt: "a city skyline at night",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "vid-123", resp.ID)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "https://example.com/video.mp4", resp.Data[0].URL)
}

func TestGrokProvider_GenerateVideo_UpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid prompt"}}`))
	}))
	t.Cleanup(server.Close)

	p := NewGrokProvider(providers.GrokConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.GenerateVideo(context.Background(), &llm.VideoGenerationRequest{Prompt: ""})
	require.Error(t, err)
}

func TestGrokProvider_GenerateVideo_PollTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
				"id":     "vid-timeout",
				"status": "pending",
			}))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
			"id":     "vid-timeout",
			"status": "processing",
		}))
	}))
	t.Cleanup(server.Close)

	p := NewGrokProvider(providers.GrokConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := p.GenerateVideo(ctx, &llm.VideoGenerationRequest{Prompt: "test"})
	require.Error(t, err)
}

