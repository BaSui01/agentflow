package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/llm"
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
		context.Background(), server.Client(), server.URL, "key", "test", "/v1/images/generations",
		&llm.ImageGenerationRequest{Prompt: "cat"}, BearerTokenHeaders,
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
		context.Background(), server.Client(), server.URL, "key", "test", "/v1/images",
		&llm.ImageGenerationRequest{}, BearerTokenHeaders,
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
		context.Background(), server.Client(), server.URL, "key", "test", "/v1/videos",
		&llm.VideoGenerationRequest{Prompt: "sunset"}, BearerTokenHeaders,
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
		context.Background(), server.Client(), server.URL, "key", "test", "/v1/audio/speech",
		&llm.AudioGenerationRequest{Input: "hello"}, BearerTokenHeaders,
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
		context.Background(), server.Client(), server.URL, "key", "test", "/v1/audio/speech",
		&llm.AudioGenerationRequest{}, BearerTokenHeaders,
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
		context.Background(), server.Client(), server.URL, "key", "test", "/v1/embeddings",
		&llm.EmbeddingRequest{Input: []string{"hello"}}, BearerTokenHeaders,
	)
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
}

func TestNotSupportedError_Details(t *testing.T) {
	err := NotSupportedError("test-provider", "video generation")
	assert.Equal(t, llm.ErrInvalidRequest, err.Code)
	assert.Contains(t, err.Message, "not supported")
	assert.Contains(t, err.Message, "test-provider")
	assert.Equal(t, http.StatusNotImplemented, err.HTTPStatus)
}

