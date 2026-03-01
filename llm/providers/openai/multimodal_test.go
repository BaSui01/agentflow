package openai

import (
	"github.com/BaSui01/agentflow/types"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// helper to create a provider pointing at a test server.
func newTestProvider(t *testing.T, serverURL string, useResponsesAPI bool) *OpenAIProvider {
	t.Helper()
	return NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: serverURL},
		UseResponsesAPI:    useResponsesAPI,
	}, zap.NewNop())
}

// --- GenerateImage ---

func TestOpenAIProvider_GenerateImage_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/images/generations", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var req llm.ImageGenerationRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "dall-e-3", req.Model)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(llm.ImageGenerationResponse{
			Created: 1700000000,
			Data: []llm.Image{
				{URL: "https://example.com/image.png", RevisedPrompt: "a cat"},
			},
		}))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	resp, err := p.GenerateImage(context.Background(), &llm.ImageGenerationRequest{
		Model:  "dall-e-3",
		Prompt: "a cat",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(1700000000), resp.Created)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "https://example.com/image.png", resp.Data[0].URL)
}

func TestOpenAIProvider_GenerateImage_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid prompt"}}`))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	_, err := p.GenerateImage(context.Background(), &llm.ImageGenerationRequest{Prompt: ""})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code)
}

// --- GenerateVideo ---

func TestOpenAIProvider_GenerateVideo_NotSupported(t *testing.T) {
	p := newTestProvider(t, "http://unused", false)
	_, err := p.GenerateVideo(context.Background(), &llm.VideoGenerationRequest{})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code)
	assert.Contains(t, llmErr.Message, "not supported")
}

// --- GenerateAudio ---

func TestOpenAIProvider_GenerateAudio_Success(t *testing.T) {
	audioBytes := []byte("fake-audio-data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/audio/speech", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(audioBytes)
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	resp, err := p.GenerateAudio(context.Background(), &llm.AudioGenerationRequest{
		Model: "tts-1",
		Input: "Hello world",
		Voice: "alloy",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, audioBytes, resp.Audio)
}

func TestOpenAIProvider_GenerateAudio_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"Server error"}}`))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	_, err := p.GenerateAudio(context.Background(), &llm.AudioGenerationRequest{Model: "tts-1", Input: "Hi"})
	require.Error(t, err)
}

// --- TranscribeAudio ---

func TestOpenAIProvider_TranscribeAudio_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/audio/transcriptions", r.URL.Path)
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")

		err := r.ParseMultipartForm(10 << 20)
		require.NoError(t, err)
		file, _, err := r.FormFile("file")
		require.NoError(t, err)
		data, readErr := io.ReadAll(file)
		require.NoError(t, readErr)
		assert.Equal(t, []byte("fake-audio"), data)
		assert.Equal(t, "whisper-1", r.FormValue("model"))
		assert.Equal(t, "en", r.FormValue("language"))

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(llm.AudioTranscriptionResponse{
			Text:     "Hello world",
			Language: "en",
			Duration: 2.5,
		}))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	resp, err := p.TranscribeAudio(context.Background(), &llm.AudioTranscriptionRequest{
		Model:    "whisper-1",
		File:     []byte("fake-audio"),
		Language: "en",
	})
	require.NoError(t, err)
	assert.Equal(t, "Hello world", resp.Text)
	assert.Equal(t, "en", resp.Language)
}

func TestOpenAIProvider_TranscribeAudio_WithAllFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(10<<20))
		assert.Equal(t, "whisper-1", r.FormValue("model"))
		assert.Equal(t, "zh", r.FormValue("language"))
		assert.Equal(t, "some prompt", r.FormValue("prompt"))
		assert.Equal(t, "json", r.FormValue("response_format"))
		assert.NotEmpty(t, r.FormValue("temperature"))

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(llm.AudioTranscriptionResponse{Text: "ok"}))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	_, err := p.TranscribeAudio(context.Background(), &llm.AudioTranscriptionRequest{
		Model:          "whisper-1",
		File:           []byte("audio"),
		Language:       "zh",
		Prompt:         "some prompt",
		ResponseFormat: "json",
		Temperature:    0.5,
	})
	require.NoError(t, err)
}

func TestOpenAIProvider_TranscribeAudio_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	_, err := p.TranscribeAudio(context.Background(), &llm.AudioTranscriptionRequest{
		Model: "whisper-1",
		File:  []byte("audio"),
	})
	require.Error(t, err)
}

// --- CreateEmbedding ---

func TestOpenAIProvider_CreateEmbedding_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/embeddings", r.URL.Path)
		var req llm.EmbeddingRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "text-embedding-3-small", req.Model)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(llm.EmbeddingResponse{
			Object: "list",
			Model:  "text-embedding-3-small",
			Data: []llm.Embedding{
				{Object: "embedding", Index: 0, Embedding: []float64{0.1, 0.2, 0.3}},
			},
			Usage: llm.ChatUsage{PromptTokens: 5, TotalTokens: 5},
		}))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	resp, err := p.CreateEmbedding(context.Background(), &llm.EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []string{"hello"},
	})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, []float64{0.1, 0.2, 0.3}, resp.Data[0].Embedding)
}

func TestOpenAIProvider_CreateEmbedding_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"Rate limit"}}`))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	_, err := p.CreateEmbedding(context.Background(), &llm.EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []string{"hello"},
	})
	require.Error(t, err)
	llmErr, ok := err.(*types.Error)
	require.True(t, ok)
	assert.Equal(t, llm.ErrRateLimit, llmErr.Code)
}

// --- CreateFineTuningJob ---

func TestOpenAIProvider_CreateFineTuningJob_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/fine_tuning/jobs", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(llm.FineTuningJob{
			ID:     "ftjob-123",
			Model:  "gpt-4o-mini-2024-07-18",
			Status: "queued",
		}))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	job, err := p.CreateFineTuningJob(context.Background(), &llm.FineTuningJobRequest{
		Model:        "gpt-4o-mini-2024-07-18",
		TrainingFile: "file-abc",
	})
	require.NoError(t, err)
	assert.Equal(t, "ftjob-123", job.ID)
	assert.Equal(t, "queued", job.Status)
}

func TestOpenAIProvider_CreateFineTuningJob_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid file"}}`))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	_, err := p.CreateFineTuningJob(context.Background(), &llm.FineTuningJobRequest{})
	require.Error(t, err)
}

// --- ListFineTuningJobs ---

func TestOpenAIProvider_ListFineTuningJobs_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/fine_tuning/jobs", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(struct {
			Data []llm.FineTuningJob `json:"data"`
		}{
			Data: []llm.FineTuningJob{
				{ID: "ftjob-1", Status: "succeeded"},
				{ID: "ftjob-2", Status: "running"},
			},
		}))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	jobs, err := p.ListFineTuningJobs(context.Background())
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	assert.Equal(t, "ftjob-1", jobs[0].ID)
}

func TestOpenAIProvider_ListFineTuningJobs_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"Forbidden"}}`))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	_, err := p.ListFineTuningJobs(context.Background())
	require.Error(t, err)
}

// --- GetFineTuningJob ---

func TestOpenAIProvider_GetFineTuningJob_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/fine_tuning/jobs/ftjob-123", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(llm.FineTuningJob{
			ID:     "ftjob-123",
			Status: "succeeded",
			Model:  "gpt-4o-mini-2024-07-18",
		}))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	job, err := p.GetFineTuningJob(context.Background(), "ftjob-123")
	require.NoError(t, err)
	assert.Equal(t, "ftjob-123", job.ID)
	assert.Equal(t, "succeeded", job.Status)
}

func TestOpenAIProvider_GetFineTuningJob_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"Not found"}}`))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	_, err := p.GetFineTuningJob(context.Background(), "ftjob-nonexistent")
	require.Error(t, err)
}

// --- CancelFineTuningJob ---

func TestOpenAIProvider_CancelFineTuningJob_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/fine_tuning/jobs/ftjob-123/cancel", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	err := p.CancelFineTuningJob(context.Background(), "ftjob-123")
	require.NoError(t, err)
}

func TestOpenAIProvider_CancelFineTuningJob_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"message":"Job already completed"}}`))
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL, false)
	err := p.CancelFineTuningJob(context.Background(), "ftjob-done")
	require.Error(t, err)
}

// --- Completion with credential override ---

func TestOpenAIProvider_Completion_CredentialOverride(t *testing.T) {
	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(providers.OpenAICompatResponse{
			ID: "resp-1", Model: "gpt-5.2",
			Choices: []providers.OpenAICompatChoice{
				{Index: 0, FinishReason: "stop", Message: providers.OpenAICompatMessage{Role: "assistant", Content: "ok"}},
			},
		}))
	}))
	t.Cleanup(server.Close)

	p := NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "default-key", BaseURL: server.URL},
		UseResponsesAPI:    true,
	}, zap.NewNop())

	ctx := llm.WithCredentialOverride(context.Background(), llm.CredentialOverride{APIKey: "override-key"})
	_, err := p.Completion(ctx, &llm.ChatRequest{
		Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer override-key", capturedAuth)
}


