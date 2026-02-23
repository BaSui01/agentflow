package video

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Config tests ---

func TestDefaultGeminiConfig(t *testing.T) {
	cfg := DefaultGeminiConfig()
	assert.Equal(t, "gemini-3-flash-preview", cfg.Model)
	assert.Equal(t, "us-central1", cfg.Location)
	assert.Equal(t, 180*time.Second, cfg.Timeout)
}

func TestDefaultVeoConfig(t *testing.T) {
	cfg := DefaultVeoConfig()
	assert.Equal(t, "veo-3.1-generate-preview", cfg.Model)
	assert.Equal(t, 300*time.Second, cfg.Timeout)
}

func TestDefaultRunwayConfig(t *testing.T) {
	cfg := DefaultRunwayConfig()
	assert.Equal(t, "https://api.runwayml.com", cfg.BaseURL)
	assert.Equal(t, "gen-4.5", cfg.Model)
	assert.Equal(t, 300*time.Second, cfg.Timeout)
}

// redirectTransport redirects all requests to a test server.
type redirectTransport struct {
	targetURL string
	inner     http.RoundTripper
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	newURL := t.targetURL + req.URL.Path + "?" + req.URL.RawQuery
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header
	return t.inner.RoundTrip(newReq)
}

// --- Gemini Provider tests ---

func TestNewGeminiProvider(t *testing.T) {
	p := NewGeminiProvider(GeminiConfig{APIKey: "test-key"})
	assert.Equal(t, "gemini-video", p.Name())
	assert.Equal(t, "gemini-3-flash-preview", p.cfg.Model)
	assert.False(t, p.SupportsGeneration())
	assert.Contains(t, p.SupportedFormats(), VideoFormatMP4)
	assert.Contains(t, p.SupportedFormats(), VideoFormatWebM)
	assert.Len(t, p.SupportedFormats(), 5)
}

func TestNewGeminiProvider_Defaults(t *testing.T) {
	p := NewGeminiProvider(GeminiConfig{})
	assert.Equal(t, "gemini-3-flash-preview", p.cfg.Model)
}

func TestGeminiProvider_Generate_NotSupported(t *testing.T) {
	p := NewGeminiProvider(GeminiConfig{APIKey: "k"})
	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestGeminiProvider_Analyze_WithVideoData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiRequest
		json.NewDecoder(r.Body).Decode(&req)
		assert.Len(t, req.Contents, 1)
		assert.Len(t, req.Contents[0].Parts, 2) // video + prompt
		assert.NotNil(t, req.Contents[0].Parts[0].InlineData)
		assert.Equal(t, "video/mp4", req.Contents[0].Parts[0].InlineData.MimeType)
		assert.Equal(t, "describe this", req.Contents[0].Parts[1].Text)

		resp := geminiResponse{}
		resp.Candidates = append(resp.Candidates, struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		}{})
		resp.Candidates[0].Content.Parts = append(resp.Candidates[0].Content.Parts, struct {
			Text string `json:"text"`
		}{Text: "A video of a cat"})
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewGeminiProvider(GeminiConfig{APIKey: "test-key"})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	resp, err := p.Analyze(context.Background(), &AnalyzeRequest{
		VideoData: "base64videodata",
		Prompt:    "describe this",
	})
	require.NoError(t, err)
	assert.Equal(t, "gemini-video", resp.Provider)
	assert.Equal(t, "A video of a cat", resp.Content)
}

func TestGeminiProvider_Analyze_WithVideoURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiRequest
		json.NewDecoder(r.Body).Decode(&req)
		assert.NotNil(t, req.Contents[0].Parts[0].FileData)
		assert.Equal(t, "gs://bucket/video.mp4", req.Contents[0].Parts[0].FileData.FileURI)

		resp := geminiResponse{}
		resp.Candidates = append(resp.Candidates, struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		}{})
		resp.Candidates[0].Content.Parts = append(resp.Candidates[0].Content.Parts, struct {
			Text string `json:"text"`
		}{Text: "analysis result"})
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewGeminiProvider(GeminiConfig{APIKey: "k"})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	resp, err := p.Analyze(context.Background(), &AnalyzeRequest{
		VideoURL: "gs://bucket/video.mp4",
		Prompt:   "analyze",
	})
	require.NoError(t, err)
	assert.Equal(t, "analysis result", resp.Content)
}

func TestGeminiProvider_Analyze_WithFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiRequest
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "video/webm", req.Contents[0].Parts[0].InlineData.MimeType)

		resp := geminiResponse{}
		resp.Candidates = append(resp.Candidates, struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		}{})
		resp.Candidates[0].Content.Parts = append(resp.Candidates[0].Content.Parts, struct {
			Text string `json:"text"`
		}{Text: "ok"})
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewGeminiProvider(GeminiConfig{APIKey: "k"})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	resp, err := p.Analyze(context.Background(), &AnalyzeRequest{
		VideoData:   "data",
		VideoFormat: "webm",
		Prompt:      "test",
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Content)
}

func TestGeminiProvider_Analyze_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	t.Cleanup(srv.Close)

	p := NewGeminiProvider(GeminiConfig{APIKey: "k"})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	_, err := p.Analyze(context.Background(), &AnalyzeRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gemini video error")
}

func TestGeminiProvider_Analyze_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := geminiResponse{} // no candidates
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewGeminiProvider(GeminiConfig{APIKey: "k"})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	resp, err := p.Analyze(context.Background(), &AnalyzeRequest{Prompt: "test"})
	require.NoError(t, err)
	assert.Equal(t, "", resp.Content)
}

func TestGeminiProvider_Analyze_CustomModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "custom-model")
		resp := geminiResponse{}
		resp.Candidates = append(resp.Candidates, struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		}{})
		resp.Candidates[0].Content.Parts = append(resp.Candidates[0].Content.Parts, struct {
			Text string `json:"text"`
		}{Text: "ok"})
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewGeminiProvider(GeminiConfig{APIKey: "k"})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	resp, err := p.Analyze(context.Background(), &AnalyzeRequest{
		Model:  "custom-model",
		Prompt: "test",
	})
	require.NoError(t, err)
	assert.Equal(t, "custom-model", resp.Model)
}

// --- Runway Provider tests ---

func TestNewRunwayProvider(t *testing.T) {
	p := NewRunwayProvider(RunwayConfig{APIKey: "test-key"})
	assert.Equal(t, "runway", p.Name())
	assert.Equal(t, "gen4_turbo", p.cfg.Model)
	assert.True(t, p.SupportsGeneration())
	assert.Equal(t, []VideoFormat{VideoFormatMP4}, p.SupportedFormats())
}

func TestNewRunwayProvider_Defaults(t *testing.T) {
	p := NewRunwayProvider(RunwayConfig{})
	assert.Equal(t, "https://api.runwayml.com", p.cfg.BaseURL)
	assert.Equal(t, "gen4_turbo", p.cfg.Model)
}

func TestRunwayProvider_Analyze_NotSupported(t *testing.T) {
	p := NewRunwayProvider(RunwayConfig{APIKey: "k"})
	_, err := p.Analyze(context.Background(), &AnalyzeRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestRunwayProvider_Generate_SubmitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad"}`))
	}))
	t.Cleanup(srv.Close)

	p := NewRunwayProvider(RunwayConfig{APIKey: "k", BaseURL: srv.URL})
	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "runway error")
}

func TestRunwayProvider_Generate_RequestParams(t *testing.T) {
	var capturedReq runwayRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
			assert.Equal(t, "2024-11-06", r.Header.Get("X-Runway-Version"))
			json.NewDecoder(r.Body).Decode(&capturedReq)
			resp := runwayResponse{ID: "t", Status: "PENDING"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Poll returns SUCCEEDED immediately
		resp := runwayResponse{Status: "SUCCEEDED", Output: []string{"https://example.com/video.mp4"}}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewRunwayProvider(RunwayConfig{APIKey: "test-key", BaseURL: srv.URL})
	result, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:      "a sunset",
		Duration:    0,
		AspectRatio: "16:9",
		ImageURL:    "https://example.com/img.png",
		Seed:        42,
	})
	require.NoError(t, err)

	// Verify request params
	assert.Equal(t, "a sunset", capturedReq.PromptText)
	assert.Equal(t, 5, capturedReq.Duration) // 0 -> default 5
	assert.Equal(t, "1280:720", capturedReq.Ratio)
	assert.Equal(t, "https://example.com/img.png", capturedReq.PromptImage)
	assert.Equal(t, int64(42), capturedReq.Seed)

	// Verify response
	assert.Equal(t, "runway", result.Provider)
	assert.Len(t, result.Videos, 1)
	assert.Equal(t, "https://example.com/video.mp4", result.Videos[0].URL)
}

func TestRunwayProvider_Generate_DurationClamping(t *testing.T) {
	tests := []struct {
		name     string
		duration float64
		expected int
	}{
		{"zero defaults to 5", 0, 5},
		{"below min clamps to 2", 1, 2},
		{"above max clamps to 10", 15, 10},
		{"normal value", 7, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedReq runwayRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					json.NewDecoder(r.Body).Decode(&capturedReq)
					// Return error to avoid polling
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"error":"test"}`))
					return
				}
			}))
			t.Cleanup(srv.Close)

			p := NewRunwayProvider(RunwayConfig{APIKey: "k", BaseURL: srv.URL})
			_, _ = p.Generate(context.Background(), &GenerateRequest{
				Prompt:   "test",
				Duration: tt.duration,
			})
			assert.Equal(t, tt.expected, capturedReq.Duration)
		})
	}
}

func TestRunwayProvider_Generate_AspectRatioMapping(t *testing.T) {
	// Test all aspect ratio mappings in a single test to minimize polling waits
	ratioTests := []struct {
		input    string
		expected string
	}{
		{"16:9", "1280:720"},
		{"9:16", "720:1280"},
		{"1:1", "960:960"},
		{"custom:ratio", "custom:ratio"},
	}

	for _, tt := range ratioTests {
		t.Run(tt.input, func(t *testing.T) {
			var capturedReq runwayRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					json.NewDecoder(r.Body).Decode(&capturedReq)
					// Return error to stop quickly
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"error":"test"}`))
					return
				}
			}))
			t.Cleanup(srv.Close)

			p := NewRunwayProvider(RunwayConfig{APIKey: "k", BaseURL: srv.URL})
			_, _ = p.Generate(context.Background(), &GenerateRequest{
				Prompt:      "test",
				AspectRatio: tt.input,
			})
			assert.Equal(t, tt.expected, capturedReq.Ratio)
		})
	}
}

func TestRunwayProvider_PollGeneration_Failed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			resp := runwayResponse{ID: "t", Status: "PENDING"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Poll returns FAILED immediately
		resp := runwayResponse{Status: "FAILED", Failure: "content policy violation"}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewRunwayProvider(RunwayConfig{APIKey: "k", BaseURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "content policy violation")
}

func TestRunwayProvider_PollGeneration_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			resp := runwayResponse{ID: "t", Status: "PENDING"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Always return PENDING
		resp := runwayResponse{Status: "PENDING"}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewRunwayProvider(RunwayConfig{APIKey: "k", BaseURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
}

// --- Veo Provider tests ---

func TestNewVeoProvider(t *testing.T) {
	p := NewVeoProvider(VeoConfig{APIKey: "test-key"})
	assert.Equal(t, "veo", p.Name())
	assert.Equal(t, "veo-3.1-generate-preview", p.cfg.Model)
	assert.True(t, p.SupportsGeneration())
	assert.Equal(t, []VideoFormat{VideoFormatMP4}, p.SupportedFormats())
}

func TestNewVeoProvider_Defaults(t *testing.T) {
	p := NewVeoProvider(VeoConfig{})
	assert.Equal(t, "veo-3.1-generate-preview", p.cfg.Model)
}

func TestVeoProvider_Analyze_NotSupported(t *testing.T) {
	p := NewVeoProvider(VeoConfig{APIKey: "k"})
	_, err := p.Analyze(context.Background(), &AnalyzeRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestVeoProvider_Generate(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Method == "POST" {
			var req veoRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, "a sunset", req.Instances[0].Prompt)
			assert.Equal(t, "16:9", req.Parameters.AspectRatio)
			assert.Equal(t, 8, req.Parameters.DurationSeconds) // default

			resp := map[string]string{"name": "operations/op-123"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// GET poll - return done immediately
		resp := map[string]any{
			"done": true,
			"response": map[string]any{
				"predictions": []map[string]string{
					{"video": "base64videodata"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewVeoProvider(VeoConfig{APIKey: "test-key"})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := p.Generate(ctx, &GenerateRequest{
		Prompt:      "a sunset",
		AspectRatio: "16:9",
	})
	require.NoError(t, err)
	assert.Equal(t, "veo", result.Provider)
	assert.Len(t, result.Videos, 1)
	assert.Equal(t, "base64videodata", result.Videos[0].B64JSON)
	assert.Equal(t, 1, result.Usage.VideosGenerated)
}

func TestVeoProvider_Generate_WithImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req veoRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.NotNil(t, req.Instances[0].Image)
			assert.Equal(t, "imgdata", req.Instances[0].Image.BytesBase64Encoded)

			resp := map[string]string{"name": "operations/op-456"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		resp := map[string]any{
			"done": true,
			"response": map[string]any{
				"predictions": []map[string]string{{"video": "v"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewVeoProvider(VeoConfig{APIKey: "k"})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, &GenerateRequest{
		Prompt: "test",
		Image:  "imgdata",
	})
	require.NoError(t, err)
}

func TestVeoProvider_Generate_WithImageURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req veoRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.NotNil(t, req.Instances[0].Image)
			assert.Equal(t, "gs://bucket/img.png", req.Instances[0].Image.GcsURI)

			resp := map[string]string{"name": "operations/op-789"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		resp := map[string]any{
			"done": true,
			"response": map[string]any{
				"predictions": []map[string]string{{"video": "v"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewVeoProvider(VeoConfig{APIKey: "k"})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, &GenerateRequest{
		Prompt:   "test",
		ImageURL: "gs://bucket/img.png",
	})
	require.NoError(t, err)
}

func TestVeoProvider_Generate_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad"}`))
	}))
	t.Cleanup(srv.Close)

	p := NewVeoProvider(VeoConfig{APIKey: "k"})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "veo error")
}

func TestVeoProvider_PollOperation_Failed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			resp := map[string]string{"name": "operations/op-fail"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Poll returns error
		resp := map[string]any{
			"done": false,
			"error": map[string]string{
				"message": "content policy violation",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewVeoProvider(VeoConfig{APIKey: "k"})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "content policy violation")
}

func TestVeoProvider_PollOperation_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			resp := map[string]string{"name": "operations/op-slow"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Always return not done
		resp := map[string]any{"done": false}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewVeoProvider(VeoConfig{APIKey: "k"})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
}

func TestVeoProvider_Generate_CustomDuration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req veoRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, 12, req.Parameters.DurationSeconds)

			resp := map[string]string{"name": "operations/op-dur"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		resp := map[string]any{
			"done": true,
			"response": map[string]any{
				"predictions": []map[string]string{{"video": "v"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewVeoProvider(VeoConfig{APIKey: "k"})
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, &GenerateRequest{
		Prompt:   "test",
		Duration: 12,
	})
	require.NoError(t, err)
}

// --- Interface compliance tests ---

func TestGeminiProvider_ImplementsProvider(t *testing.T) {
	var _ Provider = (*GeminiProvider)(nil)
}

func TestRunwayProvider_ImplementsProvider(t *testing.T) {
	var _ Provider = (*RunwayProvider)(nil)
}

func TestVeoProvider_ImplementsProvider(t *testing.T) {
	var _ Provider = (*VeoProvider)(nil)
}
