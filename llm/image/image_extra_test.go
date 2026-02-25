package image

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- OpenAI Edit with all fields ---

func TestOpenAIProvider_Edit_WithAllFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(10 << 20)
		require.NoError(t, err)
		assert.Equal(t, "edit prompt", r.FormValue("prompt"))
		assert.Equal(t, "dall-e-2", r.FormValue("model"))
		assert.Equal(t, "2", r.FormValue("n"))
		assert.Equal(t, "512x512", r.FormValue("size"))

		resp := dalleResponse{
			Data: []struct {
				URL           string `json:"url,omitempty"`
				B64JSON       string `json:"b64_json,omitempty"`
				RevisedPrompt string `json:"revised_prompt,omitempty"`
			}{
				{URL: "https://example.com/1.png"},
				{URL: "https://example.com/2.png"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.Edit(context.Background(), &EditRequest{
		Image:  bytes.NewReader([]byte("img")),
		Prompt: "edit prompt",
		Model:  "dall-e-2",
		N:      2,
		Size:   "512x512",
	})
	require.NoError(t, err)
	assert.Len(t, resp.Images, 2)
}

func TestOpenAIProvider_Edit_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Edit(context.Background(), &EditRequest{
		Image:  bytes.NewReader([]byte("img")),
		Prompt: "edit",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dalle edit error")
}

// --- OpenAI CreateVariation with all fields ---

func TestOpenAIProvider_CreateVariation_WithAllFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(10 << 20)
		require.NoError(t, err)
		assert.Equal(t, "3", r.FormValue("n"))
		assert.Equal(t, "256x256", r.FormValue("size"))

		resp := dalleResponse{
			Data: []struct {
				URL           string `json:"url,omitempty"`
				B64JSON       string `json:"b64_json,omitempty"`
				RevisedPrompt string `json:"revised_prompt,omitempty"`
			}{
				{URL: "https://example.com/v1.png"},
				{URL: "https://example.com/v2.png"},
				{URL: "https://example.com/v3.png"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.CreateVariation(context.Background(), &VariationRequest{
		Image: bytes.NewReader([]byte("img")),
		N:     3,
		Size:  "256x256",
	})
	require.NoError(t, err)
	assert.Len(t, resp.Images, 3)
}

func TestOpenAIProvider_CreateVariation_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad"}`))
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.CreateVariation(context.Background(), &VariationRequest{
		Image: bytes.NewReader([]byte("img")),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dalle variation error")
}

// --- OpenAI Generate with optional fields ---

func TestOpenAIProvider_Generate_WithOptionalFields(t *testing.T) {
	var capturedBody dalleRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		resp := dalleResponse{
			Created: 1700000000,
			Data: []struct {
				URL           string `json:"url,omitempty"`
				B64JSON       string `json:"b64_json,omitempty"`
				RevisedPrompt string `json:"revised_prompt,omitempty"`
			}{
				{B64JSON: "base64data"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:         "a dog",
		Model:          "dall-e-3",
		N:              2,
		Size:           "1792x1024",
		Quality:        "hd",
		Style:          "natural",
		ResponseFormat: "b64_json",
	})
	require.NoError(t, err)
	assert.Len(t, resp.Images, 1)
	assert.Equal(t, "base64data", resp.Images[0].B64JSON)
	assert.Equal(t, "hd", capturedBody.Quality)
	assert.Equal(t, "natural", capturedBody.Style)
	assert.Equal(t, "b64_json", capturedBody.ResponseFormat)
}

// --- Flux Generate with optional fields ---

func TestFluxProvider_Generate_WithOptionalFields(t *testing.T) {
	var capturedBody fluxRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		resp := fluxResponse{
			Status: "Ready",
			Result: struct {
				Sample string `json:"sample"`
			}{Sample: "https://example.com/img.jpg"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewFluxProvider(FluxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:   "landscape",
		Steps:    30,
		CFGScale: 7.5,
		Seed:     42,
	})
	require.NoError(t, err)
	assert.Equal(t, 30, capturedBody.Steps)
	assert.Equal(t, 7.5, capturedBody.Guidance)
	assert.Equal(t, int64(42), capturedBody.Seed)
}

// --- Flux Generate with polling ---

func TestFluxProvider_Generate_WithPolling(t *testing.T) {
	var pollCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			resp := fluxResponse{
				ID:         "task-456",
				Status:     "Pending",
				PollingURL: "http://" + r.Host + "/poll/task-456",
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// GET polling endpoint
		count := atomic.AddInt32(&pollCount, 1)
		if count >= 1 {
			resp := fluxResponse{
				Status: "Ready",
				Result: struct {
					Sample string `json:"sample"`
				}{Sample: "https://example.com/polled.jpg"},
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			resp := fluxResponse{Status: "Processing"}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	t.Cleanup(srv.Close)

	p := NewFluxProvider(FluxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/polled.jpg", resp.Images[0].URL)
}

// --- Flux Generate polling with context cancellation ---

func TestFluxProvider_Generate_PollContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := fluxResponse{
			ID:         "task-789",
			Status:     "Pending",
			PollingURL: "http://" + r.Host + "/poll/task-789",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewFluxProvider(FluxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	require.Error(t, err)
}

// --- Flux Generate polling with error status ---

func TestFluxProvider_Generate_PollError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			resp := fluxResponse{
				ID:         "task-err",
				Status:     "Pending",
				PollingURL: "http://" + r.Host + "/poll/task-err",
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		resp := fluxResponse{Status: "Error"}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewFluxProvider(FluxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flux generation failed")
}
