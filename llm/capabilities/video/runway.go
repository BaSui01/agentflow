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

// Runway Provider执行视频生成,使用Runway ML Gen-4.
// API 文件: https://docs.dev.runwayml.com/api/
type RunwayProvider struct {
	cfg    RunwayConfig
	client *http.Client
	logger *zap.Logger
}

const defaultRunwayDuration = 5
const minRunwayDuration = 2
const maxRunwayDuration = 10
const defaultRunwayRatio = "1280:720"
const runwayCreatePath = "/v1/image_to_video"
const runwayTaskPathPrefix = "/v1/tasks/"
const runwayStatusSucceeded = "SUCCEEDED"
const runwayStatusFailed = "FAILED"

var runwayAllowedModels = map[string]struct{}{
	"gen-4.5": {},
}

// NewRunway Provider创建了新的跑道视频提供商.
func NewRunwayProvider(cfg RunwayConfig, logger *zap.Logger) *RunwayProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.runwayml.com"
	}
	if cfg.Model == "" {
		// 可用: gen4 turbo, gen3a turbo, veo3.1, veo3.1 快活, veo3
		cfg.Model = "gen-4.5"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultVideoTimeout
	}

	return &RunwayProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
		logger: logger,
	}
}

func (p *RunwayProvider) Name() string { return "runway" }

func (p *RunwayProvider) SupportedFormats() []VideoFormat {
	return []VideoFormat{VideoFormatMP4}
}

func (p *RunwayProvider) SupportsGeneration() bool { return true }

type runwayRequest struct {
	Model       string `json:"model"`
	PromptText  string `json:"promptText,omitempty"`
	PromptImage string `json:"promptImage,omitempty"` // HTTPS URL or data URI
	Ratio       string `json:"ratio,omitempty"`       // e.g., "1280:720", "720:1280"
	Duration    int    `json:"duration,omitempty"`    // 2-10 seconds
	Seed        int64  `json:"seed,omitempty"`
}

type runwayResponse struct {
	ID          string   `json:"id"`
	Status      string   `json:"status"` // PENDING, RUNNING, SUCCEEDED, FAILED
	Output      []string `json:"output,omitempty"`
	CreatedAt   string   `json:"createdAt"`
	Failure     string   `json:"failure,omitempty"`
	FailureCode string   `json:"failureCode,omitempty"`
}

// 分析不由跑道支持.
func (p *RunwayProvider) Analyze(ctx context.Context, req *AnalyzeRequest) (*AnalyzeResponse, error) {
	_, span := startProviderSpan(ctx, p.Name(), "analyze")
	defer span.End()

	return nil, fmt.Errorf("video analysis not supported by runway provider")
}

// 利用跑道Gen-4生成视频.
// 终点: POST /v1/image to video
// Auth: 熊克令牌 + X- Runway-Version 信头
func (p *RunwayProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
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
	if err := validateAllowedModel("runway", model, runwayAllowedModels); err != nil {
		return nil, err
	}

	duration := int(req.Duration)
	if duration == 0 {
		duration = defaultRunwayDuration
	}
	if duration < minRunwayDuration {
		duration = minRunwayDuration
	}
	if duration > maxRunwayDuration {
		duration = maxRunwayDuration
	}

	// 转换宽比格式
	ratio := defaultRunwayRatio // default 16:9
	if req.AspectRatio != "" {
		switch req.AspectRatio {
		case "16:9":
			ratio = "1280:720"
		case "9:16":
			ratio = "720:1280"
		case "1:1":
			ratio = "960:960"
		default:
			ratio = req.AspectRatio
		}
	}
	p.logger.Info("runway generate start",
		zap.String("model", model),
		zap.String("prompt", shortPromptForLog(req.Prompt)),
		zap.Int("duration", duration),
		zap.String("ratio", ratio),
		zap.Bool("image_to_video", req.ImageURL != ""))

	body := runwayRequest{
		Model:      model,
		PromptText: req.Prompt,
		Ratio:      ratio,
		Duration:   duration,
		Seed:       req.Seed,
	}
	if req.ImageURL != "" {
		body.PromptImage = req.ImageURL
	}

	payload, err := marshalJSONRequest("runway", body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.cfg.BaseURL+runwayCreatePath,
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Runway-Version", "2024-11-06")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("runway request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, httpStatusError(p.logger, "runway", "generate", resp.StatusCode, resp.Body)
	}

	var rResp runwayResponse
	if err := json.NewDecoder(resp.Body).Decode(&rResp); err != nil {
		return nil, fmt.Errorf("failed to decode runway response: %w", err)
	}

	// 完成投票
	result, err := p.pollGeneration(ctx, rResp.ID)
	if err != nil {
		return nil, err
	}

	var videos []VideoData
	for _, url := range result.Output {
		videos = append(videos, VideoData{
			URL:      url,
			Duration: float64(duration),
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

func (p *RunwayProvider) pollGeneration(ctx context.Context, id string) (*runwayResponse, error) {
	if err := validatePollTaskID(id); err != nil {
		return nil, fmt.Errorf("invalid runway task id: %w", err)
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
				return nil, fmt.Errorf("runway polling exceeded max attempts (%d)", maxVideoPollAttempts)
			}
			if attempts == pollSlowWarnThreshold {
				p.logger.Warn("runway polling is taking longer than expected",
					zap.String("task_id", id),
					zap.Int("attempt", attempts))
			}
			if attempts%pollProgressLogEvery == 0 {
				p.logger.Info("runway polling in progress",
					zap.String("task_id", id),
					zap.Int("attempt", attempts))
			}
			httpReq, err := http.NewRequestWithContext(ctx, "GET",
				p.cfg.BaseURL+runwayTaskPathPrefix+id, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}
			httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
			httpReq.Header.Set("X-Runway-Version", "2024-11-06")

			resp, err := p.client.Do(httpReq)
			if err != nil {
				consecutiveErrors++
				p.logger.Warn("runway poll request failed",
					zap.String("task_id", id),
					zap.Int("attempt", attempts),
					zap.Int("consecutive_errors", consecutiveErrors),
					zap.Error(err))
				if consecutiveErrors >= maxVideoPollConsecutiveErrors {
					return nil, fmt.Errorf("runway polling failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				interval = nextPollInterval(interval)
				timer.Reset(interval)
				continue
			}

			if resp.StatusCode >= 400 {
				return nil, statusErrorAndClose(p.logger, "runway", "poll", resp)
			}

			var rResp runwayResponse
			if err := decodeJSONAndClose(resp, &rResp); err != nil {
				consecutiveErrors++
				p.logger.Warn("runway poll decode failed",
					zap.String("task_id", id),
					zap.Int("attempt", attempts),
					zap.Int("consecutive_errors", consecutiveErrors),
					zap.Error(err))
				if consecutiveErrors >= maxVideoPollConsecutiveErrors {
					return nil, fmt.Errorf("runway polling decode failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				interval = nextPollInterval(interval)
				timer.Reset(interval)
				continue
			}
			consecutiveErrors = 0
			interval = defaultVideoPollInterval

			switch rResp.Status {
			case runwayStatusSucceeded:
				p.logger.Info("runway generate complete",
					zap.String("task_id", id),
					zap.Int("videos", len(rResp.Output)))
				return &rResp, nil
			case runwayStatusFailed:
				if rResp.Failure != "" {
					return nil, fmt.Errorf("runway generation failed: %s", rResp.Failure)
				}
				return nil, fmt.Errorf("runway generation failed")
			}
			// Continue polling while generation is pending/running.
			timer.Reset(interval)
		}
	}
}
