package providerbase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- MapHTTPError additional coverage ---

func TestMapHTTPError_Forbidden(t *testing.T) {
	err := MapHTTPError(http.StatusForbidden, "forbidden", "test")
	assert.Equal(t, llm.ErrForbidden, err.Code)
	assert.False(t, err.Retryable)
}

func TestMapHTTPError_ServiceUnavailable(t *testing.T) {
	err := MapHTTPError(http.StatusServiceUnavailable, "unavailable", "test")
	assert.Equal(t, llm.ErrUpstreamError, err.Code)
	assert.True(t, err.Retryable)
}

func TestMapHTTPError_BadGateway(t *testing.T) {
	err := MapHTTPError(http.StatusBadGateway, "bad gateway", "test")
	assert.Equal(t, llm.ErrUpstreamError, err.Code)
	assert.True(t, err.Retryable)
}

func TestMapHTTPError_GatewayTimeout(t *testing.T) {
	err := MapHTTPError(http.StatusGatewayTimeout, "timeout", "test")
	assert.Equal(t, llm.ErrUpstreamError, err.Code)
	assert.True(t, err.Retryable)
}

func TestMapHTTPError_ModelOverloaded529(t *testing.T) {
	err := MapHTTPError(529, "overloaded", "test")
	assert.Equal(t, llm.ErrModelOverloaded, err.Code)
	assert.True(t, err.Retryable)
}

func TestMapHTTPError_BadRequest_Quota(t *testing.T) {
	err := MapHTTPError(http.StatusBadRequest, "Quota exceeded", "test")
	assert.Equal(t, llm.ErrQuotaExceeded, err.Code)
}

func TestMapHTTPError_BadRequest_Credit(t *testing.T) {
	err := MapHTTPError(http.StatusBadRequest, "Insufficient credit balance", "test")
	assert.Equal(t, llm.ErrQuotaExceeded, err.Code)
}

func TestMapHTTPError_BadRequest_Limit(t *testing.T) {
	err := MapHTTPError(http.StatusBadRequest, "Rate limit reached", "test")
	assert.Equal(t, llm.ErrQuotaExceeded, err.Code)
}

func TestMapHTTPError_UnknownServerError(t *testing.T) {
	err := MapHTTPError(599, "unknown", "test")
	assert.Equal(t, llm.ErrUpstreamError, err.Code)
	assert.True(t, err.Retryable)
}

func TestMapHTTPError_UnknownClientError(t *testing.T) {
	err := MapHTTPError(418, "teapot", "test")
	assert.Equal(t, llm.ErrUpstreamError, err.Code)
	assert.False(t, err.Retryable)
}

// --- multimodal_helpers.go tests ---

func TestGenerateImageOpenAICompat_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/images/generations", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(llm.ImageGenerationResponse{
			Created: 1700000000,
			Data:    []llm.Image{{URL: "https://example.com/img.png"}},
		})
	}))
	t.Cleanup(server.Close)

	resp, err := GenerateImageOpenAICompat(
		context.Background(), OpenAICompatParams{Client: server.Client(), BaseURL: server.URL, APIKey: "key", ProviderName: "test", Endpoint: "/v1/images/generations", BuildHeadersFunc: BearerTokenHeaders},
		&llm.ImageGenerationRequest{Prompt: "cat"},
	)
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
}

func TestGenerateImageOpenAICompat_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"bad"}}`))
	}))
	t.Cleanup(server.Close)

	_, err := GenerateImageOpenAICompat(
		context.Background(), OpenAICompatParams{Client: server.Client(), BaseURL: server.URL, APIKey: "key", ProviderName: "test", Endpoint: "/v1/images", BuildHeadersFunc: BearerTokenHeaders},
		&llm.ImageGenerationRequest{},
	)
	require.Error(t, err)
}

func TestGenerateVideoOpenAICompat_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(llm.VideoGenerationResponse{ID: "vid-1"})
	}))
	t.Cleanup(server.Close)

	resp, err := GenerateVideoOpenAICompat(
		context.Background(), OpenAICompatParams{Client: server.Client(), BaseURL: server.URL, APIKey: "key", ProviderName: "test", Endpoint: "/v1/videos", BuildHeadersFunc: BearerTokenHeaders},
		&llm.VideoGenerationRequest{Prompt: "sunset"},
	)
	require.NoError(t, err)
	assert.Equal(t, "vid-1", resp.ID)
}

func TestGenerateAudioOpenAICompat_Success(t *testing.T) {
	audioData := []byte("fake-audio-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(audioData)
	}))
	t.Cleanup(server.Close)

	resp, err := GenerateAudioOpenAICompat(
		context.Background(), OpenAICompatParams{Client: server.Client(), BaseURL: server.URL, APIKey: "key", ProviderName: "test", Endpoint: "/v1/audio/speech", BuildHeadersFunc: BearerTokenHeaders},
		&llm.AudioGenerationRequest{Input: "hello"},
	)
	require.NoError(t, err)
	assert.Equal(t, audioData, resp.Audio)
}

func TestGenerateAudioOpenAICompat_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"fail"}}`))
	}))
	t.Cleanup(server.Close)

	_, err := GenerateAudioOpenAICompat(
		context.Background(), OpenAICompatParams{Client: server.Client(), BaseURL: server.URL, APIKey: "key", ProviderName: "test", Endpoint: "/v1/audio/speech", BuildHeadersFunc: BearerTokenHeaders},
		&llm.AudioGenerationRequest{},
	)
	require.Error(t, err)
}

func TestCreateEmbeddingOpenAICompat_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(llm.EmbeddingResponse{
			Object: "list",
			Data:   []llm.Embedding{{Embedding: []float64{0.1, 0.2}}},
		})
	}))
	t.Cleanup(server.Close)

	resp, err := CreateEmbeddingOpenAICompat(
		context.Background(), OpenAICompatParams{Client: server.Client(), BaseURL: server.URL, APIKey: "key", ProviderName: "test", Endpoint: "/v1/embeddings", BuildHeadersFunc: BearerTokenHeaders},
		&llm.EmbeddingRequest{Input: []string{"hello"}},
	)
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
}

func TestFineTuningOpenAICompat_Success(t *testing.T) {
	var seenPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPaths = append(seenPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/fine_tuning/jobs":
			json.NewEncoder(w).Encode(llm.FineTuningJob{ID: "ft-job-1"})
		case "GET /v1/fine_tuning/jobs":
			json.NewEncoder(w).Encode(struct {
				Data []llm.FineTuningJob `json:"data"`
			}{Data: []llm.FineTuningJob{{ID: "ft-job-1"}}})
		case "GET /v1/fine_tuning/jobs/ft-job-1":
			json.NewEncoder(w).Encode(llm.FineTuningJob{ID: "ft-job-1"})
		case "POST /v1/fine_tuning/jobs/ft-job-1/cancel":
			json.NewEncoder(w).Encode(llm.FineTuningJob{ID: "ft-job-1"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	params := OpenAICompatParams{Client: server.Client(), BaseURL: server.URL, APIKey: "key", ProviderName: "test", Endpoint: "/v1/fine_tuning/jobs", BuildHeadersFunc: BearerTokenHeaders}

	created, err := CreateFineTuningJobOpenAICompat(context.Background(), params, &llm.FineTuningJobRequest{Model: "m", TrainingFile: "file-1"})
	require.NoError(t, err)
	assert.Equal(t, "ft-job-1", created.ID)

	jobs, err := ListFineTuningJobsOpenAICompat(context.Background(), params)
	require.NoError(t, err)
	require.Len(t, jobs, 1)

	got, err := GetFineTuningJobOpenAICompat(context.Background(), params, "ft-job-1")
	require.NoError(t, err)
	assert.Equal(t, "ft-job-1", got.ID)

	err = CancelFineTuningJobOpenAICompat(context.Background(), params, "ft-job-1")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"/v1/fine_tuning/jobs",
		"/v1/fine_tuning/jobs",
		"/v1/fine_tuning/jobs/ft-job-1",
		"/v1/fine_tuning/jobs/ft-job-1/cancel",
	}, seenPaths)
}

func TestNotSupportedError_Details(t *testing.T) {
	err := NotSupportedError("test-provider", "video generation")
	assert.Equal(t, llm.ErrInvalidRequest, err.Code)
	assert.Contains(t, err.Message, "not supported")
	assert.Contains(t, err.Message, "test-provider")
	assert.Equal(t, http.StatusNotImplemented, err.HTTPStatus)
}
