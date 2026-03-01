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

// SoraProvider implements video generation using OpenAI Sora 2.
type SoraProvider struct {
	cfg    SoraConfig
	client *http.Client
	logger *zap.Logger
}

const defaultSoraTimeout = 300 * time.Second
const defaultSoraPollInterval = 5 * time.Second

// NewSoraProvider creates a new Sora video provider.
func NewSoraProvider(cfg SoraConfig, logger *zap.Logger) *SoraProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if cfg.Model == "" {
		cfg.Model = "sora-2"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultSoraTimeout
	}
	return &SoraProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
		logger: logger,
	}
}

func (p *SoraProvider) Name() string                 { return "sora" }
func (p *SoraProvider) SupportedFormats() []VideoFormat { return []VideoFormat{VideoFormatMP4} }
func (p *SoraProvider) SupportsGeneration() bool     { return true }

type soraRequest struct {
	Model       string `json:"model"`
	Prompt      string `json:"prompt"`
	Duration    int    `json:"duration,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Resolution  string `json:"resolution,omitempty"`
	Image       string `json:"image,omitempty"` // image URL for image-to-video
}

type soraResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"` // processing, completed, failed
	Output struct {
		VideoURL string `json:"video_url,omitempty"`
	} `json:"output,omitempty"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Analyze is not supported by the Sora provider.
func (p *SoraProvider) Analyze(ctx context.Context, req *AnalyzeRequest) (*AnalyzeResponse, error) {
	return nil, fmt.Errorf("video analysis not supported by sora provider")
}

// Generate creates a video using OpenAI Sora 2.
func (p *SoraProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	if err := ValidateGenerateRequest(req); err != nil {
		return nil, err
	}

	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	duration := int(req.Duration)
	if duration == 0 {
		duration = 8
	}
	if duration < 4 {
		duration = 4
	}
	if duration > 20 {
		duration = 20
	}

	aspectRatio := req.AspectRatio
	if aspectRatio == "" {
		aspectRatio = "16:9"
	}

	resolution := req.Resolution
	if resolution == "" {
		resolution = "720p"
	}

	body := soraRequest{
		Model:       model,
		Prompt:      req.Prompt,
		Duration:    duration,
		AspectRatio: aspectRatio,
		Resolution:  resolution,
	}
	if req.ImageURL != "" {
		body.Image = req.ImageURL
	}

	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.cfg.BaseURL+"/v1/video/generations",
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sora request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sora error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var sResp soraResponse
	if err := json.NewDecoder(resp.Body).Decode(&sResp); err != nil {
		return nil, fmt.Errorf("failed to decode sora response: %w", err)
	}

	result, err := p.pollGeneration(ctx, sResp.ID)
	if err != nil {
		return nil, err
	}

	videos := []VideoData{{
		URL:      result.Output.VideoURL,
		Duration: float64(duration),
	}}

	return &GenerateResponse{
		Provider: p.Name(),
		Model:    model,
		Videos:   videos,
		Usage: VideoUsage{
			VideosGenerated: len(videos),
			DurationSeconds: float64(duration),
		},
		CreatedAt: time.Now(),
	}, nil
}

func (p *SoraProvider) pollGeneration(ctx context.Context, id string) (*soraResponse, error) {
	ticker := time.NewTicker(defaultSoraPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			httpReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s/v1/video/generations/%s", p.cfg.BaseURL, id), nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}
			httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

			resp, err := p.client.Do(httpReq)
			if err != nil {
				continue
			}

			var sResp soraResponse
			if err := json.NewDecoder(resp.Body).Decode(&sResp); err != nil {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			switch sResp.Status {
			case "completed":
				return &sResp, nil
			case "failed":
				if sResp.Error != nil {
					return nil, fmt.Errorf("sora generation failed: %s", sResp.Error.Message)
				}
				return nil, fmt.Errorf("sora generation failed")
			}
			// continue polling while "processing"
		}
	}
}

