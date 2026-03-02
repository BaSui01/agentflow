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

// KlingProvider implements video generation using Kling AI.
type KlingProvider struct {
	cfg    KlingConfig
	client *http.Client
	logger *zap.Logger
}

const defaultKlingDuration = 5
const minKlingDuration = 3
const maxKlingDuration = 15
const defaultKlingAspectRatio = "16:9"
const klingTextToVideoPath = "/v1/videos/text2video"
const klingImageToVideoPath = "/v1/videos/image2video"
const klingTaskPathPrefix = "/v1/videos/"
const klingStatusSucceed = "succeed"
const klingStatusFailed = "failed"

var klingAllowedModels = map[string]struct{}{
	"kling-v3-pro": {},
}

// NewKlingProvider creates a new Kling AI video provider.
func NewKlingProvider(cfg KlingConfig, logger *zap.Logger) *KlingProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.klingai.com"
	}
	if cfg.Model == "" {
		cfg.Model = "kling-v3-pro"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultVideoTimeout
	}
	return &KlingProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
		logger: logger,
	}
}

func (p *KlingProvider) Name() string                    { return "kling" }
func (p *KlingProvider) SupportedFormats() []VideoFormat { return []VideoFormat{VideoFormatMP4} }
func (p *KlingProvider) SupportsGeneration() bool        { return true }

// klingTextRequest targets Kling's text2video endpoint.
type klingTextRequest struct {
	Model          string  `json:"model"`
	Prompt         string  `json:"prompt"`
	NegativePrompt string  `json:"negative_prompt,omitempty"`
	Duration       int     `json:"duration,omitempty"`
	AspectRatio    string  `json:"aspect_ratio,omitempty"`
	CfgScale       float64 `json:"cfg_scale,omitempty"`
}

// klingImageRequest is intentionally separate from klingTextRequest because
// Kling exposes dedicated text2video and image2video endpoints with different
// payload contracts.
type klingImageRequest struct {
	Model       string `json:"model"`
	Prompt      string `json:"prompt,omitempty"`
	Image       string `json:"image"` // accepts public HTTPS URL or data:image/*;base64 URI
	Duration    int    `json:"duration,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
}

type klingResponse struct {
	TaskID        string `json:"task_id"`
	TaskStatus    string `json:"task_status"` // submitted, processing, succeed, failed
	TaskStatusMsg string `json:"task_status_msg,omitempty"`
	TaskResult    struct {
		Videos []struct {
			URL      string  `json:"url"`
			Duration float64 `json:"duration"`
		} `json:"videos,omitempty"`
	} `json:"task_result,omitempty"`
}

// Analyze is not supported by the Kling provider.
func (p *KlingProvider) Analyze(ctx context.Context, req *AnalyzeRequest) (*AnalyzeResponse, error) {
	_, span := startProviderSpan(ctx, p.Name(), "analyze")
	defer span.End()

	return nil, fmt.Errorf("video analysis not supported by kling provider")
}

// Generate creates a video using Kling AI.
func (p *KlingProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
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
	if err := validateAllowedModel("kling", model, klingAllowedModels); err != nil {
		return nil, err
	}

	duration := int(req.Duration)
	if duration == 0 {
		duration = defaultKlingDuration
	}
	if duration < minKlingDuration {
		duration = minKlingDuration
	}
	if duration > maxKlingDuration {
		duration = maxKlingDuration
	}

	aspectRatio := req.AspectRatio
	if aspectRatio == "" {
		aspectRatio = defaultKlingAspectRatio
	}
	p.logger.Info("kling generate start",
		zap.String("model", model),
		zap.String("prompt", shortPromptForLog(req.Prompt)),
		zap.Int("duration", duration),
		zap.String("aspect_ratio", aspectRatio),
		zap.Bool("image_to_video", req.ImageURL != ""))

	var (
		endpoint   string
		payload    []byte
		marshalErr error
	)

	if req.ImageURL != "" {
		endpoint = p.cfg.BaseURL + klingImageToVideoPath
		body := klingImageRequest{
			Model:       model,
			Prompt:      req.Prompt,
			Image:       req.ImageURL,
			Duration:    duration,
			AspectRatio: aspectRatio,
		}
		payload, marshalErr = marshalJSONRequest("kling", body)
	} else {
		endpoint = p.cfg.BaseURL + klingTextToVideoPath
		body := klingTextRequest{
			Model:          model,
			Prompt:         req.Prompt,
			NegativePrompt: req.NegativePrompt,
			Duration:       duration,
			AspectRatio:    aspectRatio,
		}
		payload, marshalErr = marshalJSONRequest("kling", body)
	}
	if marshalErr != nil {
		return nil, marshalErr
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("kling request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, httpStatusError(p.logger, "kling", "generate", resp.StatusCode, resp.Body)
	}

	var kResp klingResponse
	if err := json.NewDecoder(resp.Body).Decode(&kResp); err != nil {
		return nil, fmt.Errorf("failed to decode kling response: %w", err)
	}

	result, err := p.pollGeneration(ctx, kResp.TaskID)
	if err != nil {
		return nil, err
	}

	var videos []VideoData
	for _, v := range result.TaskResult.Videos {
		videos = append(videos, VideoData{
			URL:      v.URL,
			Duration: v.Duration,
		})
	}

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

func (p *KlingProvider) pollGeneration(ctx context.Context, taskID string) (*klingResponse, error) {
	if err := validatePollTaskID(taskID); err != nil {
		return nil, fmt.Errorf("invalid kling task id: %w", err)
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
				return nil, fmt.Errorf("kling polling exceeded max attempts (%d)", maxVideoPollAttempts)
			}
			if attempts == pollSlowWarnThreshold {
				p.logger.Warn("kling polling is taking longer than expected",
					zap.String("task_id", taskID),
					zap.Int("attempt", attempts))
			}
			if attempts%pollProgressLogEvery == 0 {
				p.logger.Info("kling polling in progress",
					zap.String("task_id", taskID),
					zap.Int("attempt", attempts))
			}
			httpReq, err := http.NewRequestWithContext(ctx, "GET",
				p.cfg.BaseURL+klingTaskPathPrefix+taskID, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}
			httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

			resp, err := p.client.Do(httpReq)
			if err != nil {
				consecutiveErrors++
				p.logger.Warn("kling poll request failed",
					zap.String("task_id", taskID),
					zap.Int("attempt", attempts),
					zap.Int("consecutive_errors", consecutiveErrors),
					zap.Error(err))
				if consecutiveErrors >= maxVideoPollConsecutiveErrors {
					return nil, fmt.Errorf("kling polling failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				interval = nextPollInterval(interval)
				timer.Reset(interval)
				continue
			}
			if resp.StatusCode >= 400 {
				return nil, statusErrorAndClose(p.logger, "kling", "poll", resp)
			}

			var kResp klingResponse
			if err := decodeJSONAndClose(resp, &kResp); err != nil {
				consecutiveErrors++
				p.logger.Warn("kling poll decode failed",
					zap.String("task_id", taskID),
					zap.Int("attempt", attempts),
					zap.Int("consecutive_errors", consecutiveErrors),
					zap.Error(err))
				if consecutiveErrors >= maxVideoPollConsecutiveErrors {
					return nil, fmt.Errorf("kling polling decode failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				interval = nextPollInterval(interval)
				timer.Reset(interval)
				continue
			}
			consecutiveErrors = 0
			interval = defaultVideoPollInterval

			switch kResp.TaskStatus {
			case klingStatusSucceed:
				p.logger.Info("kling generate complete",
					zap.String("task_id", taskID),
					zap.Int("videos", len(kResp.TaskResult.Videos)))
				return &kResp, nil
			case klingStatusFailed:
				if kResp.TaskStatusMsg != "" {
					return nil, fmt.Errorf("kling generation failed: %s", kResp.TaskStatusMsg)
				}
				return nil, fmt.Errorf("kling generation failed")
			}
			// continue polling for submitted, processing
			timer.Reset(interval)
		}
	}
}
