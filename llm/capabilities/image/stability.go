package image

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
)

// StabilityProvider 使用 Stability AI (Stable Diffusion) 执行图像生成.
// 官方: https://platform.stability.ai  REST: POST /v1/generation/{engine_id}/text-to-image
const defaultStabilityTimeout = 120 * time.Second

// stabilityEngine 与 config.Model 对应，如 stable-diffusion-3.5-large、stable-diffusion-xl-1024-v1-0
const defaultStabilityEngine = "stable-diffusion-xl-1024-v1-0"

type StabilityProvider struct {
	cfg    StabilityConfig
	client *http.Client
}

func NewStabilityProvider(cfg StabilityConfig) *StabilityProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.stability.ai"
	}
	if cfg.Model == "" {
		cfg.Model = defaultStabilityEngine
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultStabilityTimeout
	}
	return &StabilityProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(cfg.Timeout),
	}
}

func (p *StabilityProvider) Name() string { return "stability" }

func (p *StabilityProvider) SupportedSizes() []string {
	return []string{"512x512", "768x768", "1024x1024", "1024x768", "768x1024", "1280x768", "768x1280"}
}

// Stability 文档: text_prompts, cfg_scale, height, width, samples, steps, seed
type stabilityRequest struct {
	TextPrompts []stabilityTextPrompt `json:"text_prompts"`
	CFGScale    float64               `json:"cfg_scale,omitempty"`
	Height      int                   `json:"height,omitempty"`
	Width       int                   `json:"width,omitempty"`
	Samples     int                   `json:"samples,omitempty"`
	Steps       int                   `json:"steps,omitempty"`
	Seed        int64                 `json:"seed,omitempty"`
}

type stabilityTextPrompt struct {
	Text   string  `json:"text"`
	Weight float64 `json:"weight,omitempty"`
}

type stabilityResponse struct {
	Artifacts []struct {
		Base64 string `json:"base64"`
		Seed   int64  `json:"seed,omitempty"`
	} `json:"artifacts"`
}

func (p *StabilityProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	engineID := req.Model
	if engineID == "" {
		engineID = p.cfg.Model
	}

	width, height := 1024, 1024
	if req.Size != "" {
		parts := strings.SplitN(req.Size, "x", 2)
		if len(parts) == 2 {
			if w, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil && w > 0 {
				width = w
			}
			if h, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && h > 0 {
				height = h
			}
		}
	}

	body := stabilityRequest{
		TextPrompts: []stabilityTextPrompt{{Text: req.Prompt, Weight: 1}},
		CFGScale:    7,
		Height:      height,
		Width:       width,
		Samples:     1,
		Steps:       30,
	}
	if req.NegativePrompt != "" {
		body.TextPrompts = append(body.TextPrompts, stabilityTextPrompt{Text: req.NegativePrompt, Weight: -1})
	}
	if req.CFGScale > 0 {
		body.CFGScale = req.CFGScale
	}
	if req.Steps > 0 {
		body.Steps = req.Steps
	}
	if req.Seed != 0 {
		body.Seed = req.Seed
	}
	if req.N > 0 && req.N <= 4 {
		body.Samples = req.N
	}

	payload, _ := json.Marshal(body)
	path := fmt.Sprintf("/v1/generation/%s/text-to-image", engineID)
	url := strings.TrimRight(p.cfg.BaseURL, "/") + path
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("stability request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("stability request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("stability error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var sResp stabilityResponse
	if err := json.NewDecoder(resp.Body).Decode(&sResp); err != nil {
		return nil, fmt.Errorf("stability decode: %w", err)
	}

	images := make([]ImageData, 0, len(sResp.Artifacts))
	for _, a := range sResp.Artifacts {
		img := ImageData{B64JSON: a.Base64, Seed: a.Seed}
		if a.Seed == 0 && req.Seed != 0 {
			img.Seed = req.Seed
		}
		images = append(images, img)
	}

	return &GenerateResponse{
		Provider:  p.Name(),
		Model:     engineID,
		Images:    images,
		Usage:     ImageUsage{ImagesGenerated: len(images)},
		CreatedAt: time.Now(),
	}, nil
}

func (p *StabilityProvider) Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("stability does not support image editing via this API")
}

func (p *StabilityProvider) CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("stability does not support image variations via this API")
}
