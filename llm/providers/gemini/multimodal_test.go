package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGeminiProvider_MultimodalNotSupported(t *testing.T) {
	// All previously not-supported capabilities are now implemented.
	// This test is kept as a placeholder for future not-supported checks.
}

func TestGeminiProvider_TranscribeAudio_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "generateContent")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{"content": map[string]any{"parts": []map[string]any{{"text": "Hello world"}}}},
			},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.TranscribeAudio(context.Background(), &llm.AudioTranscriptionRequest{
		File: []byte("fake-audio"), Language: "en",
	})
	require.NoError(t, err)
	assert.Equal(t, "Hello world", resp.Text)
}

func TestGeminiProvider_TranscribeAudio_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Bad request"}}`))
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.TranscribeAudio(context.Background(), &llm.AudioTranscriptionRequest{File: []byte("audio")})
	require.Error(t, err)
}

func TestGeminiProvider_CreateFineTuningJob_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1beta/tunedModels", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"name":     "tunedModels/test-123",
			"metadata": map[string]any{"totalSteps": 100},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	job, err := p.CreateFineTuningJob(context.Background(), &llm.FineTuningJobRequest{Model: "gemini-2.5-flash"})
	require.NoError(t, err)
	assert.Equal(t, "tunedModels/test-123", job.ID)
}

func TestGeminiProvider_CreateFineTuningJob_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"message":"Forbidden"}}`))
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	_, err := p.CreateFineTuningJob(context.Background(), &llm.FineTuningJobRequest{})
	require.Error(t, err)
}

func TestGeminiProvider_GenerateImage_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v1beta/models/imagen-4.0-generate-001:predict")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"predictions": []map[string]any{
				{"mimeType": "image/png", "bytesBase64Encoded": "ZmFrZS1pbWFnZQ=="},
			},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.GenerateImage(context.Background(), &llm.ImageGenerationRequest{Prompt: "a cat"})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
}

func TestGeminiProvider_GenerateImage_ModelAwareRouting(t *testing.T) {
	t.Run("imagen model uses predict schema", func(t *testing.T) {
		var capturedPath string
		var capturedBody map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			err := json.NewDecoder(r.Body).Decode(&capturedBody)
			require.NoError(t, err)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"predictions": []map[string]any{
					{"bytesBase64Encoded": "ZmFrZS1pbWFnZQ=="},
				},
			})
		}))
		t.Cleanup(server.Close)

		p := NewGeminiProvider(providers.GeminiConfig{
			BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		}, zap.NewNop())

		resp, err := p.GenerateImage(context.Background(), &llm.ImageGenerationRequest{
			Model:  "imagen-4.0-fast-generate-001",
			Prompt: "a cat",
			N:      2,
			Size:   "1024x1024",
		})
		require.NoError(t, err)
		require.Len(t, resp.Data, 1)
		assert.NotEmpty(t, resp.Data[0].B64JSON)

		assert.Equal(t, "/v1beta/models/imagen-4.0-fast-generate-001:predict", capturedPath)
		instances, ok := capturedBody["instances"].([]any)
		require.True(t, ok)
		require.Len(t, instances, 1)
		_, hasContents := capturedBody["contents"]
		assert.False(t, hasContents)
	})

	t.Run("gemini image model uses generateContent schema", func(t *testing.T) {
		var capturedPath string
		var capturedBody map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			err := json.NewDecoder(r.Body).Decode(&capturedBody)
			require.NoError(t, err)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{
									"inlineData": map[string]any{
										"mimeType": "image/png",
										"data":     "ZmFrZS1nZW1pbmktaW1hZ2U=",
									},
								},
							},
						},
					},
				},
			})
		}))
		t.Cleanup(server.Close)

		p := NewGeminiProvider(providers.GeminiConfig{
			BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
		}, zap.NewNop())

		resp, err := p.GenerateImage(context.Background(), &llm.ImageGenerationRequest{
			Model:  "gemini-3-pro-image-preview",
			Prompt: "a cat",
			N:      1,
			Size:   "1536x1024",
		})
		require.NoError(t, err)
		require.Len(t, resp.Data, 1)
		assert.NotEmpty(t, resp.Data[0].B64JSON)

		assert.Equal(t, "/v1beta/models/gemini-3-pro-image-preview:generateContent", capturedPath)
		_, hasInstances := capturedBody["instances"]
		assert.False(t, hasInstances)
		contents, ok := capturedBody["contents"].([]any)
		require.True(t, ok)
		require.NotEmpty(t, contents)
		generationConfig, ok := capturedBody["generationConfig"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(1), generationConfig["candidateCount"])
		imageConfig, ok := generationConfig["imageConfig"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "3:2", imageConfig["aspectRatio"])
	})
}

func TestGeminiProvider_GenerateVideo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "predictLongRunning")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"name": "vid-1"})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.GenerateVideo(context.Background(), &llm.VideoGenerationRequest{Prompt: "a sunset"})
	require.NoError(t, err)
	assert.Equal(t, "vid-1", resp.ID)
}

func TestGeminiProvider_GenerateVideo_ModelAwareRouting(t *testing.T) {
	var capturedPath string
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		err := json.NewDecoder(r.Body).Decode(&capturedBody)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "operations/veo-op-1",
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.GenerateVideo(context.Background(), &llm.VideoGenerationRequest{
		Model:       "veo-3.1-fast-generate-preview",
		Prompt:      "a sunset",
		Duration:    5,
		Resolution:  "1920x1080",
		AspectRatio: "16:9",
	})
	require.NoError(t, err)
	assert.Equal(t, "operations/veo-op-1", resp.ID)

	assert.Equal(t, "/v1beta/models/veo-3.1-fast-generate-preview:predictLongRunning", capturedPath)
	instances, ok := capturedBody["instances"].([]any)
	require.True(t, ok)
	require.Len(t, instances, 1)
	params, ok := capturedBody["parameters"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(5), params["durationSeconds"])
	assert.Equal(t, "1920x1080", params["resolution"])
	assert.Equal(t, "16:9", params["aspectRatio"])
}

func TestGeminiProvider_GenerateAudio_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "generateContent")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{"inlineData": map[string]any{"mimeType": "audio/wav", "data": "ZmFrZS1hdWRpbw=="}},
						},
					},
				},
			},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.GenerateAudio(context.Background(), &llm.AudioGenerationRequest{Input: "hello"})
	require.NoError(t, err)
	assert.Equal(t, []byte("fake-audio"), resp.Audio)
}

func TestGeminiProvider_CreateEmbedding_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "batchEmbedContents")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"embeddings": []map[string]any{
				{"values": []float64{0.1, 0.2}},
			},
		})
	}))
	t.Cleanup(server.Close)

	p := NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.CreateEmbedding(context.Background(), &llm.EmbeddingRequest{Input: []string{"hello"}})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
}
