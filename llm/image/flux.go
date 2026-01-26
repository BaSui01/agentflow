package image

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FluxProvider implements image generation using Black Forest Labs Flux.
// API Docs: https://docs.bfl.ai/quick_start/generating_images
type FluxProvider struct {
	cfg    FluxConfig
	client *http.Client
}

// NewFluxProvider creates a new Flux image provider.
func NewFluxProvider(cfg FluxConfig) *FluxProvider {
	if cfg.BaseURL == "" {
		// Primary global endpoint (recommended)
		// Regional: api.eu.bfl.ai (EU), api.us.bfl.ai (US)
		cfg.BaseURL = "https://api.bfl.ai"
	}
	if cfg.Model == "" {
		// Available: flux-2-pro, flux-2-max, flux-2-flex, flux-2-klein-4b, flux-2-klein-9b
		// flux-kontext-max, flux-kontext-pro, flux-pro-1.1-ultra, flux-pro-1.1
		cfg.Model = "flux-2-pro"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	return &FluxProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *FluxProvider) Name() string { return "flux" }

func (p *FluxProvider) SupportedSizes() []string {
	return []string{"1024x1024", "1024x768", "768x1024", "1536x1024", "1024x1536"}
}

type fluxRequest struct {
	Prompt          string  `json:"prompt"`
	AspectRatio     string  `json:"aspect_ratio,omitempty"` // e.g., "1:1", "16:9", "9:16"
	Width           int     `json:"width,omitempty"`
	Height          int     `json:"height,omitempty"`
	Steps           int     `json:"steps,omitempty"`
	Guidance        float64 `json:"guidance,omitempty"`
	Seed            int64   `json:"seed,omitempty"`
	SafetyTolerance int     `json:"safety_tolerance,omitempty"`
	OutputFormat    string  `json:"output_format,omitempty"`
}

type fluxResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	PollingURL string `json:"polling_url,omitempty"` // Must use this URL for polling
	Result     struct {
		Sample string `json:"sample"` // Signed URL (valid 10 min)
	} `json:"result,omitempty"`
}

// Generate creates images using Flux.
// Endpoint: POST /v1/{model} (e.g., /v1/flux-2-pro)
// Auth: x-key header
func (p *FluxProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	body := fluxRequest{
		Prompt:       req.Prompt,
		OutputFormat: "jpeg",
	}

	// Prefer aspect_ratio over width/height for Flux 2.x
	if req.Size != "" {
		var width, height int
		fmt.Sscanf(req.Size, "%dx%d", &width, &height)
		// Convert to aspect ratio
		if width == height {
			body.AspectRatio = "1:1"
		} else if width > height {
			body.AspectRatio = "16:9"
		} else {
			body.AspectRatio = "9:16"
		}
	} else {
		body.AspectRatio = "1:1"
	}

	if req.Steps > 0 {
		body.Steps = req.Steps
	}
	if req.CFGScale > 0 {
		body.Guidance = req.CFGScale
	}
	if req.Seed > 0 {
		body.Seed = req.Seed
	}

	// Submit generation request
	payload, _ := json.Marshal(body)
	endpoint := fmt.Sprintf("%s/v1/%s", strings.TrimRight(p.cfg.BaseURL, "/"), model)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	httpReq.Header.Set("x-key", p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("accept", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("flux request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("flux error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var fResp fluxResponse
	if err := json.NewDecoder(resp.Body).Decode(&fResp); err != nil {
		return nil, err
	}

	// Poll for result using polling_url (required for global endpoint)
	if fResp.Status == "Pending" || fResp.Status == "Processing" || fResp.Status == "" {
		pollingURL := fResp.PollingURL
		if pollingURL == "" {
			// Fallback for legacy endpoints
			pollingURL = fmt.Sprintf("%s/v1/get_result?id=%s", strings.TrimRight(p.cfg.BaseURL, "/"), fResp.ID)
		}
		result, err := p.pollResult(ctx, pollingURL)
		if err != nil {
			return nil, err
		}
		fResp = *result
	}

	images := []ImageData{{
		URL:  fResp.Result.Sample,
		Seed: req.Seed,
	}}

	return &GenerateResponse{
		Provider:  p.Name(),
		Model:     model,
		Images:    images,
		CreatedAt: time.Now(),
	}, nil
}

// pollResult polls for async generation result using the polling URL.
// Note: Signed URLs in result.sample are only valid for 10 minutes.
func (p *FluxProvider) pollResult(ctx context.Context, pollingURL string) (*fluxResponse, error) {
	for i := 0; i < 120; i++ { // Max 120 attempts (4 minutes)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}

		httpReq, _ := http.NewRequestWithContext(ctx, "GET", pollingURL, nil)
		httpReq.Header.Set("x-key", p.cfg.APIKey)
		httpReq.Header.Set("accept", "application/json")

		resp, err := p.client.Do(httpReq)
		if err != nil {
			continue
		}

		var fResp fluxResponse
		json.NewDecoder(resp.Body).Decode(&fResp)
		resp.Body.Close()

		switch fResp.Status {
		case "Ready":
			return &fResp, nil
		case "Error", "Failed":
			return nil, fmt.Errorf("flux generation failed")
		}
		// Continue polling for Pending, Processing, etc.
	}

	return nil, fmt.Errorf("flux generation timeout")
}

// Edit is not supported by Flux.
func (p *FluxProvider) Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("flux does not support image editing")
}

// CreateVariation is not supported by Flux.
func (p *FluxProvider) CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("flux does not support image variations")
}
