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

// LumaProvider implements video generation using Luma AI Dream Machine (Ray 2).
// 官方端点（可被配置 BaseURL 覆盖）：Base https://api.lumalabs.ai
// POST /dream-machine/v1/generations（提交）→ GET /dream-machine/v1/generations/{id}（轮询）
type LumaProvider struct {
	cfg    LumaConfig
	client *http.Client
	logger *zap.Logger
}

const defaultLumaDuration = 5
const minLumaDuration = 4
const maxLumaDuration = 20
const defaultLumaResolution = "720p"
const defaultLumaAspectRatio = "16:9"
const defaultLumaBaseURL = "https://api.lumalabs.ai"
const lumaGenerationPath = "/dream-machine/v1/generations"
const lumaStateCompleted = "completed"
const lumaStateFailed = "failed"
const lumaStateQueued = "queued"
const lumaStateDreaming = "dreaming"

var lumaAllowedModels = map[string]struct{}{
	"ray-2": {},
}

// NewLumaProvider creates a new Luma AI video provider.
func NewLumaProvider(cfg LumaConfig, logger *zap.Logger) *LumaProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultLumaBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = "ray-2"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultVideoTimeout
	}
	return &LumaProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
		logger: logger,
	}
}

func (p *LumaProvider) Name() string                    { return "luma" }
func (p *LumaProvider) SupportedFormats() []VideoFormat { return []VideoFormat{VideoFormatMP4} }
func (p *LumaProvider) SupportsGeneration() bool        { return true }

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
	_, span := startProviderSpan(ctx, p.Name(), "analyze")
	defer span.End()

	return nil, fmt.Errorf("video analysis not supported by luma provider")
}

// Generate creates a video using Luma AI Dream Machine.
// Endpoint: POST /dream-machine/v1/generations
// Auth: Bearer token
func (p *LumaProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
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
	if err := validateAllowedModel("luma", model, lumaAllowedModels); err != nil {
		return nil, err
	}

	duration := req.Duration
	if duration == 0 {
		duration = defaultLumaDuration
	}
	if duration < minLumaDuration {
		duration = minLumaDuration
	}
	if duration > maxLumaDuration {
		duration = maxLumaDuration
	}
	durationStr := fmt.Sprintf("%ds", int(duration))

	resolution := req.Resolution
	if resolution == "" {
		resolution = defaultLumaResolution
	}

	aspectRatio := req.AspectRatio
	if aspectRatio == "" {
		aspectRatio = defaultLumaAspectRatio
	}
	p.logger.Info("luma generate start",
		zap.String("model", model),
		zap.String("prompt", shortPromptForLog(req.Prompt)),
		zap.Float64("duration", duration),
		zap.String("aspect_ratio", aspectRatio),
		zap.String("resolution", resolution),
		zap.Bool("image_to_video", req.ImageURL != ""))

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

	payload, err := marshalJSONRequest("luma", body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.cfg.BaseURL+lumaGenerationPath,
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
		return nil, httpStatusError(p.logger, "luma", "generate", resp.StatusCode, resp.Body)
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
	if err := validatePollTaskID(id); err != nil {
		return nil, fmt.Errorf("invalid luma generation id: %w", err)
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
				return nil, fmt.Errorf("luma polling exceeded max attempts (%d)", maxVideoPollAttempts)
			}
			if attempts == pollSlowWarnThreshold {
				p.logger.Warn("luma polling is taking longer than expected",
					zap.String("generation_id", id),
					zap.Int("attempt", attempts))
			}
			if attempts%pollProgressLogEvery == 0 {
				p.logger.Info("luma polling in progress",
					zap.String("generation_id", id),
					zap.Int("attempt", attempts))
			}
			httpReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s%s/%s", p.cfg.BaseURL, lumaGenerationPath, id), nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}
			httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

			resp, err := p.client.Do(httpReq)
			if err != nil {
				consecutiveErrors++
				p.logger.Warn("luma poll request failed",
					zap.String("generation_id", id),
					zap.Int("attempt", attempts),
					zap.Int("consecutive_errors", consecutiveErrors),
					zap.Error(err))
				if consecutiveErrors >= maxVideoPollConsecutiveErrors {
					return nil, fmt.Errorf("luma polling failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				interval = nextPollInterval(interval)
				timer.Reset(interval)
				continue
			}
			if resp.StatusCode >= 400 {
				return nil, statusErrorAndClose(p.logger, "luma", "poll", resp)
			}

			var lResp lumaResponse
			if err := decodeJSONAndClose(resp, &lResp); err != nil {
				consecutiveErrors++
				p.logger.Warn("luma poll decode failed",
					zap.String("generation_id", id),
					zap.Int("attempt", attempts),
					zap.Int("consecutive_errors", consecutiveErrors),
					zap.Error(err))
				if consecutiveErrors >= maxVideoPollConsecutiveErrors {
					return nil, fmt.Errorf("luma polling decode failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				interval = nextPollInterval(interval)
				timer.Reset(interval)
				continue
			}
			consecutiveErrors = 0
			interval = defaultVideoPollInterval

			switch lResp.State {
			case lumaStateCompleted:
				p.logger.Info("luma generate complete",
					zap.String("generation_id", id),
					zap.String("status", lResp.State))
				return &lResp, nil
			case lumaStateFailed:
				if lResp.FailureReason != "" {
					return nil, fmt.Errorf("luma generation failed: %s", lResp.FailureReason)
				}
				return nil, fmt.Errorf("luma generation failed")
			case lumaStateQueued, lumaStateDreaming:
				// Continue polling.
			}
			timer.Reset(interval)
		}
	}
}
