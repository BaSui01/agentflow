package video

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
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

func TestNewProvider_DefaultConfig(t *testing.T) {
	p, err := NewProvider("sora", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "sora", p.Name())
}

func TestNewProvider_Alias(t *testing.T) {
	p, err := NewProvider("minimax", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "minimax-video", p.Name())
}

func TestNewProvider_InvalidConfigType(t *testing.T) {
	_, err := NewProvider("runway", GeminiConfig{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config type")
}

func TestNewProvider_UnknownProvider(t *testing.T) {
	_, err := NewProvider("unknown-video", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown video provider")
}

func TestValidateGenerateRequest_AspectRatioAndResolution(t *testing.T) {
	err := ValidateGenerateRequest(&GenerateRequest{
		Prompt:      "test",
		AspectRatio: "bad",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aspect_ratio")

	err = ValidateGenerateRequest(&GenerateRequest{
		Prompt:     "test",
		Resolution: "2k",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolution")

	err = ValidateGenerateRequest(&GenerateRequest{
		Prompt:      "test",
		AspectRatio: "16:9",
		Resolution:  "1080p",
	})
	require.NoError(t, err)
}

func TestValidateGenerateRequest_AllowsDataImageURL(t *testing.T) {
	err := ValidateGenerateRequest(&GenerateRequest{
		Prompt:   "test",
		ImageURL: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA",
	})
	require.NoError(t, err)
}

func TestValidateGenerateRequest_PromptTooLong(t *testing.T) {
	err := ValidateGenerateRequest(&GenerateRequest{
		Prompt: strings.Repeat("a", maxVideoPromptLength+1),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max length")
}

func TestAcquireVideoGenerateSlot_RespectsLimit(t *testing.T) {
	var releases []func()
	for i := 0; i < maxVideoGenerateConcurrency; i++ {
		release, err := acquireVideoGenerateSlot(context.Background())
		require.NoError(t, err)
		releases = append(releases, release)
	}
	defer func() {
		for _, release := range releases {
			release()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := acquireVideoGenerateSlot(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// --- Gemini Provider tests ---

func TestNewGeminiProvider(t *testing.T) {
	p := newGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}}, nil)
	assert.Equal(t, "gemini-video", p.Name())
	assert.Equal(t, "gemini-3-flash-preview", p.cfg.Model)
	assert.False(t, p.SupportsGeneration())
	assert.Contains(t, p.SupportedFormats(), VideoFormatMP4)
	assert.Contains(t, p.SupportedFormats(), VideoFormatWebM)
	assert.Len(t, p.SupportedFormats(), 5)
}

func TestNewGeminiProvider_Defaults(t *testing.T) {
	p := newGeminiProvider(GeminiConfig{}, nil)
	assert.Equal(t, "gemini-3-flash-preview", p.cfg.Model)
}

func TestGeminiProvider_Generate_NotSupported(t *testing.T) {
	p := newGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
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

	p := newGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}}, nil)
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

	p := newGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
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

	p := newGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
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

	p := newGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	_, err := p.Analyze(context.Background(), &AnalyzeRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gemini error")
}

func TestGeminiProvider_Analyze_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := geminiResponse{} // no candidates
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := newGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
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

	p := newGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
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
	p := newRunwayProvider(RunwayConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}}, nil)
	assert.Equal(t, "runway", p.Name())
	assert.Equal(t, "gen-4.5", p.cfg.Model)
	assert.True(t, p.SupportsGeneration())
	assert.Equal(t, []VideoFormat{VideoFormatMP4}, p.SupportedFormats())
}

func TestNewRunwayProvider_Defaults(t *testing.T) {
	p := newRunwayProvider(RunwayConfig{}, nil)
	assert.Equal(t, "https://api.runwayml.com", p.cfg.BaseURL)
	assert.Equal(t, "gen-4.5", p.cfg.Model)
}

func TestRunwayProvider_Analyze_NotSupported(t *testing.T) {
	p := newRunwayProvider(RunwayConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
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

	p := newRunwayProvider(RunwayConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
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

	p := newRunwayProvider(RunwayConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: srv.URL}})
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

			p := newRunwayProvider(RunwayConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
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
		{"4:3", "4:3"},
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

			p := newRunwayProvider(RunwayConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
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

	p := newRunwayProvider(RunwayConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
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

	p := newRunwayProvider(RunwayConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
}

// --- Veo Provider tests ---

func TestNewVeoProvider(t *testing.T) {
	p := newVeoProvider(VeoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}}, nil)
	assert.Equal(t, "veo", p.Name())
	assert.Equal(t, "veo-3.1-generate-preview", p.cfg.Model)
	assert.True(t, p.SupportsGeneration())
	assert.Equal(t, []VideoFormat{VideoFormatMP4}, p.SupportedFormats())
}

func TestNewVeoProvider_Defaults(t *testing.T) {
	p := newVeoProvider(VeoConfig{}, nil)
	assert.Equal(t, "veo-3.1-generate-preview", p.cfg.Model)
}

func TestVeoProvider_Analyze_NotSupported(t *testing.T) {
	p := newVeoProvider(VeoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
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

	p := newVeoProvider(VeoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}}, nil)
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

	p := newVeoProvider(VeoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
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
			assert.Equal(t, "https://example.com/img.png", req.Instances[0].Image.GcsURI)

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

	p := newVeoProvider(VeoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, &GenerateRequest{
		Prompt:   "test",
		ImageURL: "https://example.com/img.png",
	})
	require.NoError(t, err)
}

func TestVeoProvider_Generate_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad"}`))
	}))
	t.Cleanup(srv.Close)

	p := newVeoProvider(VeoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
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

	p := newVeoProvider(VeoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
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

	p := newVeoProvider(VeoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
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

	p := newVeoProvider(VeoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
	p.client = &http.Client{Transport: &redirectTransport{targetURL: srv.URL, inner: http.DefaultTransport}}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, &GenerateRequest{
		Prompt:   "test",
		Duration: 12,
	})
	require.NoError(t, err)
}

// --- Sora Provider tests ---

func TestDefaultSoraConfig(t *testing.T) {
	cfg := DefaultSoraConfig()
	assert.Equal(t, "https://api.openai.com", cfg.BaseURL)
	assert.Equal(t, "sora-2", cfg.Model)
	assert.Equal(t, 300*time.Second, cfg.Timeout)
}

func TestNewSoraProvider(t *testing.T) {
	p := newSoraProvider(SoraConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}}, nil)
	assert.Equal(t, "sora", p.Name())
	assert.Equal(t, "sora-2", p.cfg.Model)
	assert.True(t, p.SupportsGeneration())
	assert.Equal(t, []VideoFormat{VideoFormatMP4}, p.SupportedFormats())
}

func TestNewSoraProvider_Defaults(t *testing.T) {
	p := newSoraProvider(SoraConfig{}, nil)
	assert.Equal(t, "https://api.openai.com", p.cfg.BaseURL)
	assert.Equal(t, "sora-2", p.cfg.Model)
}

func TestSoraProvider_Generate_InvalidModel(t *testing.T) {
	p := newSoraProvider(SoraConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
	_, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt: "test",
		Model:  "sora-injected-model",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestSoraProvider_Analyze_NotSupported(t *testing.T) {
	p := newSoraProvider(SoraConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
	_, err := p.Analyze(context.Background(), &AnalyzeRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestSoraProvider_Generate_SubmitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad"}`))
	}))
	t.Cleanup(srv.Close)

	p := newSoraProvider(SoraConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sora error")
}

func TestSoraProvider_Generate_RequestParams(t *testing.T) {
	var capturedReq soraRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
			json.NewDecoder(r.Body).Decode(&capturedReq)
			json.NewEncoder(w).Encode(soraResponse{ID: "t", Status: "processing"})
			return
		}
		json.NewEncoder(w).Encode(soraResponse{
			ID:     "t",
			Status: "completed",
			Output: struct {
				VideoURL string `json:"video_url,omitempty"`
			}{VideoURL: "https://example.com/video.mp4"},
		})
	}))
	t.Cleanup(srv.Close)

	p := newSoraProvider(SoraConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: srv.URL}})
	result, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:   "a sunset",
		ImageURL: "https://example.com/img.png",
	})
	require.NoError(t, err)
	assert.Equal(t, "a sunset", capturedReq.Prompt)
	assert.Equal(t, 8, capturedReq.Duration)
	assert.Equal(t, "16:9", capturedReq.AspectRatio)
	assert.Equal(t, "720p", capturedReq.Resolution)
	assert.Equal(t, "https://example.com/img.png", capturedReq.Image)
	assert.Equal(t, "sora", result.Provider)
	assert.Len(t, result.Videos, 1)
	assert.Equal(t, "https://example.com/video.mp4", result.Videos[0].URL)
}

func TestSoraProvider_Generate_DurationClamping(t *testing.T) {
	tests := []struct {
		name     string
		duration float64
		expected int
	}{
		{"zero defaults to 8", 0, 8},
		{"below min clamps to 4", 2, 4},
		{"above max clamps to 20", 30, 20},
		{"normal value", 12, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedReq soraRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					json.NewDecoder(r.Body).Decode(&capturedReq)
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"error":"test"}`))
					return
				}
			}))
			t.Cleanup(srv.Close)
			p := newSoraProvider(SoraConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
			_, _ = p.Generate(context.Background(), &GenerateRequest{Prompt: "test", Duration: tt.duration})
			assert.Equal(t, tt.expected, capturedReq.Duration)
		})
	}
}

func TestSoraProvider_PollGeneration_Failed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			json.NewEncoder(w).Encode(soraResponse{ID: "t", Status: "processing"})
			return
		}
		json.NewEncoder(w).Encode(soraResponse{
			ID:     "t",
			Status: "failed",
			Error: &struct {
				Message string `json:"message"`
			}{Message: "content policy"},
		})
	}))
	t.Cleanup(srv.Close)
	p := newSoraProvider(SoraConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "content policy")
}

func TestSoraProvider_ImplementsProvider(t *testing.T) {
	var _ Provider = (*SoraProvider)(nil)
}

// --- Luma Provider tests ---

func TestDefaultLumaConfig(t *testing.T) {
	cfg := DefaultLumaConfig()
	assert.Equal(t, "https://api.lumalabs.ai", cfg.BaseURL)
	assert.Equal(t, "ray-2", cfg.Model)
	assert.Equal(t, 300*time.Second, cfg.Timeout)
}

func TestNewLumaProvider(t *testing.T) {
	p := newLumaProvider(LumaConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}}, nil)
	assert.Equal(t, "luma", p.Name())
	assert.Equal(t, "ray-2", p.cfg.Model)
	assert.True(t, p.SupportsGeneration())
	assert.Equal(t, []VideoFormat{VideoFormatMP4}, p.SupportedFormats())
}

func TestNewLumaProvider_Defaults(t *testing.T) {
	p := newLumaProvider(LumaConfig{}, nil)
	assert.Equal(t, "https://api.lumalabs.ai", p.cfg.BaseURL)
	assert.Equal(t, "ray-2", p.cfg.Model)
}

func TestLumaProvider_Generate_InvalidModel(t *testing.T) {
	p := newLumaProvider(LumaConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
	_, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt: "test",
		Model:  "ray-custom",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestLumaProvider_Analyze_NotSupported(t *testing.T) {
	p := newLumaProvider(LumaConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
	_, err := p.Analyze(context.Background(), &AnalyzeRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestLumaProvider_Generate_SubmitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad"}`))
	}))
	t.Cleanup(srv.Close)
	p := newLumaProvider(LumaConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "luma error")
}

func TestLumaProvider_Generate_RequestParams(t *testing.T) {
	var capturedReq lumaRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
			json.NewDecoder(r.Body).Decode(&capturedReq)
			json.NewEncoder(w).Encode(lumaResponse{ID: "t", State: "queued"})
			return
		}
		json.NewEncoder(w).Encode(lumaResponse{
			ID:    "t",
			State: "completed",
			Assets: struct {
				Video string `json:"video,omitempty"`
			}{Video: "https://example.com/video.mp4"},
		})
	}))
	t.Cleanup(srv.Close)
	p := newLumaProvider(LumaConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: srv.URL}})
	result, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:   "a sunset",
		ImageURL: "https://example.com/img.png",
	})
	require.NoError(t, err)
	assert.Equal(t, "a sunset", capturedReq.Prompt)
	assert.Equal(t, "ray-2", capturedReq.Model)
	assert.Equal(t, "5s", capturedReq.Duration)
	assert.Equal(t, "720p", capturedReq.Resolution)
	assert.Equal(t, "16:9", capturedReq.AspectRatio)
	assert.NotNil(t, capturedReq.Keyframes)
	assert.Equal(t, "image", capturedReq.Keyframes.Frame0.Type)
	assert.Equal(t, "https://example.com/img.png", capturedReq.Keyframes.Frame0.URL)
	assert.Equal(t, "luma", result.Provider)
	assert.Len(t, result.Videos, 1)
	assert.Equal(t, "https://example.com/video.mp4", result.Videos[0].URL)
}

func TestLumaProvider_PollGeneration_Failed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			json.NewEncoder(w).Encode(lumaResponse{ID: "t", State: "queued"})
			return
		}
		json.NewEncoder(w).Encode(lumaResponse{ID: "t", State: "failed", FailureReason: "content policy"})
	}))
	t.Cleanup(srv.Close)
	p := newLumaProvider(LumaConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "content policy")
}

func TestLumaProvider_ImplementsProvider(t *testing.T) {
	var _ Provider = (*LumaProvider)(nil)
}

// --- Kling Provider tests ---

func TestDefaultKlingConfig(t *testing.T) {
	cfg := DefaultKlingConfig()
	assert.Equal(t, "https://api.klingai.com", cfg.BaseURL)
	assert.Equal(t, "kling-v3-pro", cfg.Model)
	assert.Equal(t, 300*time.Second, cfg.Timeout)
}

func TestNewKlingProvider(t *testing.T) {
	p := newKlingProvider(KlingConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}}, nil)
	assert.Equal(t, "kling", p.Name())
	assert.Equal(t, "kling-v3-pro", p.cfg.Model)
	assert.True(t, p.SupportsGeneration())
	assert.Equal(t, []VideoFormat{VideoFormatMP4}, p.SupportedFormats())
}

func TestNewKlingProvider_Defaults(t *testing.T) {
	p := newKlingProvider(KlingConfig{}, nil)
	assert.Equal(t, "https://api.klingai.com", p.cfg.BaseURL)
	assert.Equal(t, "kling-v3-pro", p.cfg.Model)
}

func TestKlingProvider_Generate_InvalidModel(t *testing.T) {
	p := newKlingProvider(KlingConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
	_, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt: "test",
		Model:  "kling-custom",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestKlingProvider_Analyze_NotSupported(t *testing.T) {
	p := newKlingProvider(KlingConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
	_, err := p.Analyze(context.Background(), &AnalyzeRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestKlingProvider_Generate_SubmitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad"}`))
	}))
	t.Cleanup(srv.Close)
	p := newKlingProvider(KlingConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kling error")
}

func TestKlingProvider_Generate_TextToVideo(t *testing.T) {
	var capturedReq klingTextRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			assert.Contains(t, r.URL.Path, "text2video")
			assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
			json.NewDecoder(r.Body).Decode(&capturedReq)
			json.NewEncoder(w).Encode(klingResponse{TaskID: "t", TaskStatus: "submitted"})
			return
		}
		resp := klingResponse{TaskID: "t", TaskStatus: "succeed"}
		resp.TaskResult.Videos = []struct {
			URL      string  `json:"url"`
			Duration float64 `json:"duration"`
		}{{URL: "https://example.com/video.mp4", Duration: 5}}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	p := newKlingProvider(KlingConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: srv.URL}})
	result, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:         "a sunset",
		NegativePrompt: "blur",
		AspectRatio:    "9:16",
	})
	require.NoError(t, err)
	assert.Equal(t, "a sunset", capturedReq.Prompt)
	assert.Equal(t, "blur", capturedReq.NegativePrompt)
	assert.Equal(t, 5, capturedReq.Duration)
	assert.Equal(t, "9:16", capturedReq.AspectRatio)
	assert.Equal(t, "kling", result.Provider)
	assert.Len(t, result.Videos, 1)
	assert.Equal(t, "https://example.com/video.mp4", result.Videos[0].URL)
}

func TestKlingProvider_Generate_ImageToVideo(t *testing.T) {
	var capturedReq klingImageRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			assert.Contains(t, r.URL.Path, "image2video")
			json.NewDecoder(r.Body).Decode(&capturedReq)
			json.NewEncoder(w).Encode(klingResponse{TaskID: "t", TaskStatus: "submitted"})
			return
		}
		resp := klingResponse{TaskID: "t", TaskStatus: "succeed"}
		resp.TaskResult.Videos = []struct {
			URL      string  `json:"url"`
			Duration float64 `json:"duration"`
		}{{URL: "https://example.com/video.mp4", Duration: 5}}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	p := newKlingProvider(KlingConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:   "animate this",
		ImageURL: "https://example.com/img.png",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/img.png", capturedReq.Image)
}

func TestKlingProvider_Generate_DurationClamping(t *testing.T) {
	tests := []struct {
		name     string
		duration float64
		expected int
	}{
		{"zero defaults to 5", 0, 5},
		{"below min clamps to 3", 1, 3},
		{"above max clamps to 15", 20, 15},
		{"normal value", 10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedReq klingTextRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					json.NewDecoder(r.Body).Decode(&capturedReq)
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"error":"test"}`))
					return
				}
			}))
			t.Cleanup(srv.Close)
			p := newKlingProvider(KlingConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
			_, _ = p.Generate(context.Background(), &GenerateRequest{Prompt: "test", Duration: tt.duration})
			assert.Equal(t, tt.expected, capturedReq.Duration)
		})
	}
}

func TestKlingProvider_PollGeneration_Failed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			json.NewEncoder(w).Encode(klingResponse{TaskID: "t", TaskStatus: "submitted"})
			return
		}
		json.NewEncoder(w).Encode(klingResponse{TaskID: "t", TaskStatus: "failed", TaskStatusMsg: "content policy"})
	}))
	t.Cleanup(srv.Close)
	p := newKlingProvider(KlingConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "content policy")
}

func TestKlingProvider_ImplementsProvider(t *testing.T) {
	var _ Provider = (*KlingProvider)(nil)
}

// --- MiniMax Video Provider tests ---

func TestDefaultMiniMaxVideoConfig(t *testing.T) {
	cfg := DefaultMiniMaxVideoConfig()
	assert.Equal(t, "https://api.minimax.chat", cfg.BaseURL)
	assert.Equal(t, "video-01", cfg.Model)
	assert.Equal(t, 300*time.Second, cfg.Timeout)
}

func TestNewMiniMaxVideoProvider(t *testing.T) {
	p := newMiniMaxVideoProvider(MiniMaxVideoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}}, nil)
	assert.Equal(t, "minimax-video", p.Name())
	assert.Equal(t, "video-01", p.cfg.Model)
	assert.True(t, p.SupportsGeneration())
	assert.Equal(t, []VideoFormat{VideoFormatMP4}, p.SupportedFormats())
}

func TestNewMiniMaxVideoProvider_Defaults(t *testing.T) {
	p := newMiniMaxVideoProvider(MiniMaxVideoConfig{}, nil)
	assert.Equal(t, "https://api.minimax.chat", p.cfg.BaseURL)
	assert.Equal(t, "video-01", p.cfg.Model)
}

func TestMiniMaxVideoProvider_Generate_InvalidModel(t *testing.T) {
	p := newMiniMaxVideoProvider(MiniMaxVideoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
	_, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt: "test",
		Model:  "video-x",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestMiniMaxVideoProvider_Analyze_NotSupported(t *testing.T) {
	p := newMiniMaxVideoProvider(MiniMaxVideoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}}, nil)
	_, err := p.Analyze(context.Background(), &AnalyzeRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestMiniMaxVideoProvider_Generate_SubmitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad"}`))
	}))
	t.Cleanup(srv.Close)
	p := newMiniMaxVideoProvider(MiniMaxVideoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "minimax error")
}

func TestMiniMaxVideoProvider_Generate_Success(t *testing.T) {
	var capturedReq minimaxVideoRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/video_generation":
			json.NewDecoder(r.Body).Decode(&capturedReq)
			json.NewEncoder(w).Encode(minimaxVideoCreateResponse{
				TaskID: "task-1",
				BaseResp: struct {
					StatusCode int    `json:"status_code"`
					StatusMsg  string `json:"status_msg"`
				}{StatusCode: 0, StatusMsg: "success"},
			})
		case r.URL.Path == "/v1/query/video_generation":
			json.NewEncoder(w).Encode(minimaxVideoQueryResponse{
				TaskID: "task-1",
				Status: "Success",
				FileID: "file-1",
				BaseResp: struct {
					StatusCode int    `json:"status_code"`
					StatusMsg  string `json:"status_msg"`
				}{StatusCode: 0, StatusMsg: "success"},
			})
		case r.URL.Path == "/v1/files/retrieve":
			json.NewEncoder(w).Encode(minimaxFileResponse{
				File: struct {
					DownloadURL string `json:"download_url"`
				}{DownloadURL: "https://example.com/video.mp4"},
				BaseResp: struct {
					StatusCode int    `json:"status_code"`
					StatusMsg  string `json:"status_msg"`
				}{StatusCode: 0, StatusMsg: "success"},
			})
		}
	}))
	t.Cleanup(srv.Close)
	p := newMiniMaxVideoProvider(MiniMaxVideoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: srv.URL}})
	result, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:   "a sunset",
		ImageURL: "https://example.com/img.png",
	})
	require.NoError(t, err)
	assert.Equal(t, "a sunset", capturedReq.Prompt)
	assert.Equal(t, "video-01", capturedReq.Model)
	assert.Equal(t, "https://example.com/img.png", capturedReq.FirstFrameImage)
	assert.Equal(t, "minimax-video", result.Provider)
	assert.Len(t, result.Videos, 1)
	assert.Equal(t, "https://example.com/video.mp4", result.Videos[0].URL)
}

func TestMiniMaxVideoProvider_PollGeneration_Failed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			json.NewEncoder(w).Encode(minimaxVideoCreateResponse{
				TaskID: "task-1",
				BaseResp: struct {
					StatusCode int    `json:"status_code"`
					StatusMsg  string `json:"status_msg"`
				}{StatusCode: 0, StatusMsg: "success"},
			})
			return
		}
		json.NewEncoder(w).Encode(minimaxVideoQueryResponse{
			TaskID: "task-1",
			Status: "Fail",
			BaseResp: struct {
				StatusCode int    `json:"status_code"`
				StatusMsg  string `json:"status_msg"`
			}{StatusCode: 0, StatusMsg: "success"},
		})
	}))
	t.Cleanup(srv.Close)
	p := newMiniMaxVideoProvider(MiniMaxVideoConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "minimax generation failed")
}

func TestMiniMaxVideoProvider_ImplementsProvider(t *testing.T) {
	var _ Provider = (*MiniMaxVideoProvider)(nil)
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
