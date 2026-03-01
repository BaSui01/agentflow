package image

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Config tests ---

func TestDefaultOpenAIConfig(t *testing.T) {
	cfg := DefaultOpenAIConfig()
	assert.Equal(t, "https://api.openai.com", cfg.BaseURL)
	assert.Equal(t, "dall-e-3", cfg.Model)
	assert.Equal(t, 120*time.Second, cfg.Timeout)
}

func TestDefaultFluxConfig(t *testing.T) {
	cfg := DefaultFluxConfig()
	assert.Equal(t, "https://api.bfl.ml", cfg.BaseURL)
	assert.Equal(t, "flux-1.1-pro", cfg.Model)
}

func TestDefaultStabilityConfig(t *testing.T) {
	cfg := DefaultStabilityConfig()
	assert.Equal(t, "https://api.stability.ai", cfg.BaseURL)
	assert.Equal(t, "stable-diffusion-3.5-large", cfg.Model)
}

func TestDefaultGeminiConfig(t *testing.T) {
	cfg := DefaultGeminiConfig()
	assert.Equal(t, "gemini-3-pro-image-preview", cfg.Model)
}

func TestDefaultImagen4Config(t *testing.T) {
	cfg := DefaultImagen4Config()
	assert.Equal(t, "imagen-4.0-generate-preview", cfg.Model)
}

// --- OpenAI Provider tests ---

func TestNewOpenAIProvider(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}})
	assert.Equal(t, "openai-image", p.Name())
	assert.Equal(t, "dall-e-3", p.cfg.Model)
	assert.Contains(t, p.SupportedSizes(), "1024x1024")
}

func TestOpenAIProvider_Generate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/images/generations", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req dalleRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "a cat", req.Prompt)
		assert.Equal(t, "dall-e-3", req.Model)

		resp := dalleResponse{
			Created: time.Now().Unix(),
			Data: []struct {
				URL           string `json:"url,omitempty"`
				B64JSON       string `json:"b64_json,omitempty"`
				RevisedPrompt string `json:"revised_prompt,omitempty"`
			}{
				{URL: "https://example.com/img.png", RevisedPrompt: "a cute cat"},
			},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: srv.URL}})
	resp, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "a cat"})
	require.NoError(t, err)
	assert.Equal(t, "openai-image", resp.Provider)
	assert.Len(t, resp.Images, 1)
	assert.Equal(t, "https://example.com/img.png", resp.Images[0].URL)
	assert.Equal(t, "a cute cat", resp.Images[0].RevisedPrompt)
	assert.Equal(t, 1, resp.Usage.ImagesGenerated)
}

func TestOpenAIProvider_Generate_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad prompt"}`))
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dalle error")
}

func TestOpenAIProvider_Edit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/images/edits", r.URL.Path)
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")

		resp := dalleResponse{
			Data: []struct {
				URL           string `json:"url,omitempty"`
				B64JSON       string `json:"b64_json,omitempty"`
				RevisedPrompt string `json:"revised_prompt,omitempty"`
			}{
				{URL: "https://example.com/edited.png"},
			},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.Edit(context.Background(), &EditRequest{
		Image:  bytes.NewReader([]byte("fake-image")),
		Prompt: "add a hat",
	})
	require.NoError(t, err)
	assert.Len(t, resp.Images, 1)
}

func TestOpenAIProvider_Edit_NoImage(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	_, err := p.Edit(context.Background(), &EditRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image is required")
}

func TestOpenAIProvider_Edit_WithMask(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := dalleResponse{
			Data: []struct {
				URL           string `json:"url,omitempty"`
				B64JSON       string `json:"b64_json,omitempty"`
				RevisedPrompt string `json:"revised_prompt,omitempty"`
			}{
				{URL: "https://example.com/edited.png"},
			},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.Edit(context.Background(), &EditRequest{
		Image:  bytes.NewReader([]byte("image")),
		Mask:   bytes.NewReader([]byte("mask")),
		Prompt: "edit",
	})
	require.NoError(t, err)
	assert.Len(t, resp.Images, 1)
}

func TestOpenAIProvider_CreateVariation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/images/variations", r.URL.Path)
		resp := dalleResponse{
			Data: []struct {
				URL           string `json:"url,omitempty"`
				B64JSON       string `json:"b64_json,omitempty"`
				RevisedPrompt string `json:"revised_prompt,omitempty"`
			}{
				{URL: "https://example.com/variation.png"},
			},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.CreateVariation(context.Background(), &VariationRequest{
		Image: bytes.NewReader([]byte("image")),
	})
	require.NoError(t, err)
	assert.Len(t, resp.Images, 1)
}

func TestOpenAIProvider_CreateVariation_NoImage(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	_, err := p.CreateVariation(context.Background(), &VariationRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image is required")
}

// --- Gemini Provider tests ---

func TestNewGeminiProvider(t *testing.T) {
	p := NewGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}})
	assert.Equal(t, "gemini-image", p.Name())
	assert.Equal(t, "gemini-3-pro-image-preview", p.cfg.Model)
	assert.Contains(t, p.SupportedSizes(), "1024x1024")
}

// redirectTransport redirects all requests to a test server.
type redirectTransport struct {
	targetURL string
	inner     http.RoundTripper
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Replace the host with the test server
	newURL := t.targetURL + req.URL.Path + "?" + req.URL.RawQuery
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header
	return t.inner.RoundTrip(newReq)
}

func TestGeminiProvider_Generate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiImageRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Len(t, req.Contents, 1)
		assert.Equal(t, "a cat", req.Contents[0].Parts[0].Text)

		resp := geminiImageResponse{}
		resp.Candidates = append(resp.Candidates, struct {
			Content struct {
				Parts []struct {
					Text       string `json:"text,omitempty"`
					InlineData *struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData,omitempty"`
				} `json:"parts"`
			} `json:"content"`
		}{})
		resp.Candidates[0].Content.Parts = append(resp.Candidates[0].Content.Parts, struct {
			Text       string `json:"text,omitempty"`
			InlineData *struct {
				MimeType string `json:"mimeType"`
				Data     string `json:"data"`
			} `json:"inlineData,omitempty"`
		}{
			InlineData: &struct {
				MimeType string `json:"mimeType"`
				Data     string `json:"data"`
			}{MimeType: "image/png", Data: "base64imgdata"},
		})
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	p := NewGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	resp, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "a cat"})
	require.NoError(t, err)
	assert.Equal(t, "gemini-image", resp.Provider)
	assert.Len(t, resp.Images, 1)
	assert.Equal(t, "base64imgdata", resp.Images[0].B64JSON)
	assert.Equal(t, 1, resp.Usage.ImagesGenerated)
}

func TestGeminiProvider_Generate_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad"}`))
	}))
	t.Cleanup(srv.Close)

	p := NewGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gemini image error")
}

func TestGeminiProvider_Edit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := geminiImageResponse{}
		resp.Candidates = append(resp.Candidates, struct {
			Content struct {
				Parts []struct {
					Text       string `json:"text,omitempty"`
					InlineData *struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData,omitempty"`
				} `json:"parts"`
			} `json:"content"`
		}{})
		resp.Candidates[0].Content.Parts = append(resp.Candidates[0].Content.Parts, struct {
			Text       string `json:"text,omitempty"`
			InlineData *struct {
				MimeType string `json:"mimeType"`
				Data     string `json:"data"`
			} `json:"inlineData,omitempty"`
		}{
			InlineData: &struct {
				MimeType string `json:"mimeType"`
				Data     string `json:"data"`
			}{MimeType: "image/png", Data: "edited"},
		})
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	p := NewGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	resp, err := p.Edit(context.Background(), &EditRequest{
		Image:  bytes.NewReader([]byte("fake-image")),
		Prompt: "add hat",
	})
	require.NoError(t, err)
	assert.Len(t, resp.Images, 1)
	assert.Equal(t, "edited", resp.Images[0].B64JSON)
}

func TestGeminiProvider_Edit_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"fail"}`))
	}))
	t.Cleanup(srv.Close)

	p := NewGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	_, err := p.Edit(context.Background(), &EditRequest{
		Image:  bytes.NewReader([]byte("img")),
		Prompt: "edit",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gemini edit error")
}

func TestGeminiProvider_CreateVariation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := geminiImageResponse{}
		resp.Candidates = append(resp.Candidates, struct {
			Content struct {
				Parts []struct {
					Text       string `json:"text,omitempty"`
					InlineData *struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData,omitempty"`
				} `json:"parts"`
			} `json:"content"`
		}{})
		resp.Candidates[0].Content.Parts = append(resp.Candidates[0].Content.Parts, struct {
			Text       string `json:"text,omitempty"`
			InlineData *struct {
				MimeType string `json:"mimeType"`
				Data     string `json:"data"`
			} `json:"inlineData,omitempty"`
		}{
			InlineData: &struct {
				MimeType string `json:"mimeType"`
				Data     string `json:"data"`
			}{MimeType: "image/png", Data: "variation"},
		})
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	p := NewGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	resp, err := p.CreateVariation(context.Background(), &VariationRequest{
		Image: bytes.NewReader([]byte("img")),
	})
	require.NoError(t, err)
	assert.Len(t, resp.Images, 1)
}

func TestGeminiProvider_Edit_NoImage(t *testing.T) {
	p := NewGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	_, err := p.Edit(context.Background(), &EditRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image is required")
}

func TestGeminiProvider_CreateVariation_NoImage(t *testing.T) {
	p := NewGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	_, err := p.CreateVariation(context.Background(), &VariationRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image is required")
}

// --- Flux Provider tests ---

func TestNewFluxProvider(t *testing.T) {
	p := NewFluxProvider(FluxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}})
	assert.Equal(t, "flux", p.Name())
	assert.Equal(t, "flux-2-pro", p.cfg.Model)
	assert.Contains(t, p.SupportedSizes(), "1024x1024")
}

func TestFluxProvider_Edit_NotSupported(t *testing.T) {
	p := NewFluxProvider(FluxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	_, err := p.Edit(context.Background(), &EditRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not support image editing")
}

func TestFluxProvider_CreateVariation_NotSupported(t *testing.T) {
	p := NewFluxProvider(FluxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	_, err := p.CreateVariation(context.Background(), &VariationRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not support image variations")
}

func TestFluxProvider_Generate_ImmediateReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			resp := fluxResponse{
				ID:     "task-123",
				Status: "Ready",
				Result: struct {
					Sample string `json:"sample"`
				}{Sample: "https://example.com/result.jpg"},
			}
			require.NoError(t, json.NewEncoder(w).Encode(resp))
			return
		}
	}))
	t.Cleanup(srv.Close)

	p := NewFluxProvider(FluxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt: "a landscape",
		Size:   "1024x1024",
	})
	require.NoError(t, err)
	assert.Equal(t, "flux", resp.Provider)
	assert.Len(t, resp.Images, 1)
	assert.Equal(t, "https://example.com/result.jpg", resp.Images[0].URL)
}

func TestFluxProvider_Generate_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad"}`))
	}))
	t.Cleanup(srv.Close)

	p := NewFluxProvider(FluxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "flux error")
}

func TestFluxProvider_Generate_AspectRatios(t *testing.T) {
	tests := []struct {
		size          string
		expectedRatio string
	}{
		{"1024x1024", "1:1"},
		{"1920x1080", "16:9"},
		{"1080x1920", "9:16"},
		{"", "1:1"},
	}

	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			var capturedBody fluxRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
					resp := fluxResponse{Status: "Ready", Result: struct {
						Sample string `json:"sample"`
					}{Sample: "url"}}
					require.NoError(t, json.NewEncoder(w).Encode(resp))
				}
			}))
			t.Cleanup(srv.Close)

			p := NewFluxProvider(FluxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
			_, err := p.Generate(context.Background(), &GenerateRequest{
				Prompt: "test",
				Size:   tt.size,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.expectedRatio, capturedBody.AspectRatio)
		})
	}
}

// --- Interface compliance tests ---

func TestOpenAIProvider_ImplementsProvider(t *testing.T) {
	var _ Provider = (*OpenAIProvider)(nil)
}

func TestGeminiProvider_ImplementsProvider(t *testing.T) {
	var _ Provider = (*GeminiProvider)(nil)
}

func TestFluxProvider_ImplementsProvider(t *testing.T) {
	var _ Provider = (*FluxProvider)(nil)
}

