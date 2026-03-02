package video

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

const defaultSoraDuration = 8
const minSoraDuration = 4
const maxSoraDuration = 20
const defaultSoraAspectRatio = "16:9"
const defaultSoraResolution = "720p"
const soraGenerationPath = "/v1/video/generations"
const soraStatusCompleted = "completed"
const soraStatusFailed = "failed"

var soraAllowedModels = map[string]struct{}{
	"sora-2": {},
}

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
		timeout = defaultVideoTimeout
	}
	return &SoraProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
		logger: logger,
	}
}

func (p *SoraProvider) Name() string                    { return "sora" }
func (p *SoraProvider) SupportedFormats() []VideoFormat { return []VideoFormat{VideoFormatMP4} }
func (p *SoraProvider) SupportsGeneration() bool        { return true }

type soraRequest struct {
	Model       string `json:"model"`
	Prompt      string `json:"prompt"`
	Duration    int    `json:"duration,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Resolution  string `json:"resolution,omitempty"`
	Image       string `json:"image,omitempty"` // image URL for image-to-video; prefer public HTTPS URL
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
	_, span := startProviderSpan(ctx, p.Name(), "analyze")
	defer span.End()

	return nil, fmt.Errorf("video analysis not supported by sora provider")
}

// Generate creates a video using OpenAI Sora 2.
func (p *SoraProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	ctx, span := startProviderSpan(ctx, p.Name(), "generate")
	defer span.End()

	if err := ValidateGenerateRequest(req); err != nil {
		return nil, err
	}
	release, err := acquireVideoGenerateSlot(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}
	if err := validateAllowedModel("sora", model, soraAllowedModels); err != nil {
		return nil, err
	}

	duration := int(req.Duration)
	if duration == 0 {
		duration = defaultSoraDuration
	}
	if duration < minSoraDuration {
		duration = minSoraDuration
	}
	if duration > maxSoraDuration {
		duration = maxSoraDuration
	}

	aspectRatio := req.AspectRatio
	if aspectRatio == "" {
		aspectRatio = defaultSoraAspectRatio
	}

	resolution := req.Resolution
	if resolution == "" {
		resolution = defaultSoraResolution
	}
	p.logger.Info("sora generate start",
		zap.String("model", model),
		zap.String("prompt", shortPromptForLog(req.Prompt)),
		zap.Int("duration", duration),
		zap.String("aspect_ratio", aspectRatio),
		zap.String("resolution", resolution),
		zap.Bool("image_to_video", req.ImageURL != ""))

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

	payload, err := marshalJSONRequest("sora", body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.cfg.BaseURL+soraGenerationPath,
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
		return nil, httpStatusError(p.logger, "sora", "generate", resp.StatusCode, resp.Body)
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
	if err := validatePollTaskID(id); err != nil {
		return nil, fmt.Errorf("invalid sora generation id: %w", err)
	}
	timer := time.NewTimer(defaultVideoPollInterval)
	defer timer.Stop()
	interval := defaultVideoPollInterval
	attempts := 0
	consecutiveErrors := 0

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			attempts++
			if attempts > maxVideoPollAttempts {
				return nil, fmt.Errorf("sora polling exceeded max attempts (%d)", maxVideoPollAttempts)
			}
			if attempts == pollSlowWarnThreshold {
				p.logger.Warn("sora polling is taking longer than expected",
					zap.String("generation_id", id),
					zap.Int("attempt", attempts))
			}
			if attempts%pollProgressLogEvery == 0 {
				p.logger.Info("sora polling in progress",
					zap.String("generation_id", id),
					zap.Int("attempt", attempts))
			}
			httpReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s%s/%s", p.cfg.BaseURL, soraGenerationPath, id), nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}
			httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

			resp, err := p.client.Do(httpReq)
			if err != nil {
				consecutiveErrors++
				p.logger.Warn("sora poll request failed",
					zap.String("generation_id", id),
					zap.Int("attempt", attempts),
					zap.Int("consecutive_errors", consecutiveErrors),
					zap.Error(err))
				if consecutiveErrors >= maxVideoPollConsecutiveErrors {
					return nil, fmt.Errorf("sora polling failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				interval = nextPollInterval(interval)
				timer.Reset(interval)
				continue
			}
			if resp.StatusCode >= 400 {
				return nil, statusErrorAndClose(p.logger, "sora", "poll", resp)
			}

			var sResp soraResponse
			if err := decodeJSONAndClose(resp, &sResp); err != nil {
				consecutiveErrors++
				p.logger.Warn("sora poll decode failed",
					zap.String("generation_id", id),
					zap.Int("attempt", attempts),
					zap.Int("consecutive_errors", consecutiveErrors),
					zap.Error(err))
				if consecutiveErrors >= maxVideoPollConsecutiveErrors {
					return nil, fmt.Errorf("sora polling decode failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				interval = nextPollInterval(interval)
				timer.Reset(interval)
				continue
			}
			consecutiveErrors = 0
			interval = defaultVideoPollInterval

			switch sResp.Status {
			case soraStatusCompleted:
				p.logger.Info("sora generate complete",
					zap.String("generation_id", id),
					zap.String("status", sResp.Status))
				return &sResp, nil
			case soraStatusFailed:
				if sResp.Error != nil {
					return nil, fmt.Errorf("sora generation failed: %s", sResp.Error.Message)
				}
				return nil, fmt.Errorf("sora generation failed")
			}
			// continue polling while "processing"
			timer.Reset(interval)
		}
	}
}
