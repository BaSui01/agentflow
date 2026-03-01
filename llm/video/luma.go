package video

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"go.uber.org/zap"
)

// LumaProvider implements video generation using Luma AI Dream Machine (Ray 2).
type LumaProvider struct {
	cfg    LumaConfig
	client *http.Client
	logger *zap.Logger
}

const defaultLumaTimeout = 300 * time.Second
const defaultLumaPollInterval = 5 * time.Second

// NewLumaProvider creates a new Luma AI video provider.
func NewLumaProvider(cfg LumaConfig, logger *zap.Logger) *LumaProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.lumalabs.ai"
	}
	if cfg.Model == "" {
		cfg.Model = "ray-2"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultLumaTimeout
	}
	return &LumaProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
		logger: logger,
	}
}

func (p *LumaProvider) Name() string                 { return "luma" }
func (p *LumaProvider) SupportedFormats() []VideoFormat { return []VideoFormat{VideoFormatMP4} }
func (p *LumaProvider) SupportsGeneration() bool     { return true }

type lumaRequest struct {
	Prompt      string         `json:"prompt"`
	Model       string         `json:"model"`
	Resolution  string         `json:"resolution,omitempty"`
	Duration    string         `json:"duration,omitempty"`
	AspectRatio string         `json:"aspect_ratio,omitempty"`
	Loop        bool           `json:"loop,omitempty"`
	Keyframes   *lumaKeyframes `json:"keyframes,omitempty"`
}

type lumaKeyframes struct {
	Frame0 *lumaKeyframe `json:"frame0,omitempty"`
}

type lumaKeyframe struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type lumaResponse struct {
	ID            string `json:"id"`
	State         string `json:"state"` // queued, dreaming, completed, failed
	FailureReason string `json:"failure_reason,omitempty"`
	Assets        struct {
		Video string `json:"video,omitempty"`
	} `json:"assets,omitempty"`
	Version string `json:"version,omitempty"`
}

// Analyze is not supported by the Luma provider.
func (p *LumaProvider) Analyze(ctx context.Context, req *AnalyzeRequest) (*AnalyzeResponse, error) {
	return nil, fmt.Errorf("video analysis not supported by luma provider")
}

// Generate creates a video using Luma AI Dream Machine.
// Endpoint: POST /dream-machine/v1/generations
// Auth: Bearer token
func (p *LumaProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	if err := ValidateGenerateRequest(req); err != nil {
		return nil, err
	}

	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	duration := req.Duration
	if duration == 0 {
		duration = 5
	}
	durationStr := fmt.Sprintf("%ds", int(duration))

	resolution := req.Resolution
	if resolution == "" {
		resolution = "720p"
	}

	aspectRatio := req.AspectRatio
	if aspectRatio == "" {
		aspectRatio = "16:9"
	}

	body := lumaRequest{
		Prompt:      req.Prompt,
		Model:       model,
		Resolution:  resolution,
		Duration:    durationStr,
		AspectRatio: aspectRatio,
	}

	if req.ImageURL != "" {
		body.Keyframes = &lumaKeyframes{
			Frame0: &lumaKeyframe{
				Type: "image",
				URL:  req.ImageURL,
			},
		}
	}

	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.cfg.BaseURL+"/dream-machine/v1/generations",
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("luma request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("luma error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var lResp lumaResponse
	if err := json.NewDecoder(resp.Body).Decode(&lResp); err != nil {
		return nil, fmt.Errorf("failed to decode luma response: %w", err)
	}

	result, err := p.pollGeneration(ctx, lResp.ID)
	if err != nil {
		return nil, err
	}

	videos := []VideoData{
		{
			URL:      result.Assets.Video,
			Duration: duration,
		},
	}

	return &GenerateResponse{
		Provider: p.Name(),
		Model:    model,
		Videos:   videos,
		Usage: VideoUsage{
			VideosGenerated: len(videos),
			DurationSeconds: duration,
		},
		CreatedAt: time.Now(),
	}, nil
}

func (p *LumaProvider) pollGeneration(ctx context.Context, id string) (*lumaResponse, error) {
	ticker := time.NewTicker(defaultLumaPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			httpReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s/dream-machine/v1/generations/%s", p.cfg.BaseURL, id), nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}
			httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

			resp, err := p.client.Do(httpReq)
			if err != nil {
				continue
			}

			var lResp lumaResponse
			if err := json.NewDecoder(resp.Body).Decode(&lResp); err != nil {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			switch lResp.State {
			case "completed":
				return &lResp, nil
			case "failed":
				if lResp.FailureReason != "" {
					return nil, fmt.Errorf("luma generation failed: %s", lResp.FailureReason)
				}
				return nil, fmt.Errorf("luma generation failed")
			}
			// continue polling for queued, dreaming
		}
	}
}
