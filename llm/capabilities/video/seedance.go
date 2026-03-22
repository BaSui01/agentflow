package video

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"go.uber.org/zap"
)

// SeedanceProvider 实现即梦/Seedance（字节跳动）视频生成.
// 官方异步流程：POST 提交任务 → GET /v2/tasks/{task_id} 查询状态 → GET /v2/tasks/{task_id}/result 取结果.
// 参考：火山引擎/即梦 API（Base URL 可能随官方发布调整，可配置覆盖）
const (
	defaultSeedanceBaseURL   = "https://api.seedance.ai"
	seedanceTextGeneratePath = "/v2/generate/text"
	seedanceImageGeneratePath = "/v2/generate/image"
	seedanceTaskPathPrefix   = "/v2/tasks/"
	seedanceResultSuffix     = "/result"
	defaultSeedanceDuration  = 8
	minSeedanceDuration      = 5
	maxSeedanceDuration      = 15
	defaultSeedanceAspect    = "16:9"
)

// SeedanceProvider implements video generation using Seedance (即梦 / ByteDance).
type SeedanceProvider struct {
	cfg    SeedanceConfig
	client *http.Client
	logger *zap.Logger
}

// NewSeedanceProvider creates a new Seedance video provider.
func NewSeedanceProvider(cfg SeedanceConfig, logger *zap.Logger) *SeedanceProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultSeedanceBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = "seedance-2"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultVideoTimeout
	}
	return &SeedanceProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(cfg.Timeout),
		logger: logger,
	}
}

func (p *SeedanceProvider) Name() string                    { return "seedance" }
func (p *SeedanceProvider) SupportedFormats() []VideoFormat { return []VideoFormat{VideoFormatMP4} }
func (p *SeedanceProvider) SupportsGeneration() bool        { return true }

// Analyze is not supported.
func (p *SeedanceProvider) Analyze(ctx context.Context, req *AnalyzeRequest) (*AnalyzeResponse, error) {
	_, span := startProviderSpan(ctx, p.Name(), "analyze")
	defer span.End()
	return nil, fmt.Errorf("video analysis not supported by seedance provider")
}

type seedanceSubmitReq struct {
	Prompt      string `json:"prompt"`
	ImageURL    string `json:"image_url,omitempty"`
	Duration    int    `json:"duration,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Seed        int64  `json:"seed,omitempty"`
}

type seedanceTaskResp struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

type seedanceResultResp struct {
	Status string   `json:"status"`
	Videos []string `json:"videos,omitempty"`
}

// Generate creates a video using Seedance (即梦).
func (p *SeedanceProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	_, span := startProviderSpan(ctx, p.Name(), "generate")
	defer span.End()

	if err := ValidateGenerateRequest(req); err != nil {
		return nil, err
	}
	release, err := acquireVideoGenerateSlot(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	duration := int(req.Duration)
	if duration == 0 {
		duration = defaultSeedanceDuration
	}
	if duration < minSeedanceDuration {
		duration = minSeedanceDuration
	}
	if duration > maxSeedanceDuration {
		duration = maxSeedanceDuration
	}
	aspect := req.AspectRatio
	if aspect == "" {
		aspect = defaultSeedanceAspect
	}

	path := seedanceTextGeneratePath
	body := seedanceSubmitReq{Prompt: req.Prompt, Duration: duration, AspectRatio: aspect, Seed: req.Seed}
	if req.ImageURL != "" {
		path = seedanceImageGeneratePath
		body.ImageURL = req.ImageURL
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("seedance marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.cfg.BaseURL, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("seedance request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, httpStatusError(p.logger, "seedance", "generate", resp.StatusCode, resp.Body)
	}

	var submitResp seedanceTaskResp
	if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
		return nil, fmt.Errorf("seedance decode submit response: %w", err)
	}
	if submitResp.TaskID == "" {
		return nil, fmt.Errorf("seedance returned empty task_id")
	}

	result, err := p.pollAndGetResult(ctx, submitResp.TaskID)
	if err != nil {
		return nil, err
	}

	var videos []VideoData
	for _, u := range result.Videos {
		videos = append(videos, VideoData{URL: u, Duration: float64(duration)})
	}
	if len(videos) == 0 {
		return nil, fmt.Errorf("seedance generation completed but no video URL")
	}

	return &GenerateResponse{
		Provider:  p.Name(),
		Model:     p.cfg.Model,
		Videos:    videos,
		Usage:     VideoUsage{VideosGenerated: len(videos), DurationSeconds: float64(duration)},
		CreatedAt: time.Now(),
	}, nil
}

func (p *SeedanceProvider) pollAndGetResult(ctx context.Context, taskID string) (*seedanceResultResp, error) {
	if err := validatePollTaskID(taskID); err != nil {
		return nil, fmt.Errorf("invalid seedance task_id: %w", err)
	}
	base := strings.TrimRight(p.cfg.BaseURL, "/")
	taskURL := base + seedanceTaskPathPrefix + taskID
	resultURL := taskURL + seedanceResultSuffix

	timer := time.NewTimer(defaultVideoPollInterval)
	defer timer.Stop()
	interval := defaultVideoPollInterval
	attempts := 0

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			attempts++
			if attempts > maxVideoPollAttempts {
				return nil, fmt.Errorf("seedance polling exceeded max attempts (%d)", maxVideoPollAttempts)
			}

			statusReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, taskURL, nil)
			statusReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
			statusResp, err := p.client.Do(statusReq)
			if err != nil {
				timer.Reset(nextPollInterval(interval))
				continue
			}
			var status struct {
				Status string `json:"status"`
			}
			_ = json.NewDecoder(statusResp.Body).Decode(&status)
			statusResp.Body.Close()

			switch status.Status {
			case "succeeded", "completed", "success":
				resultReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, resultURL, nil)
				resultReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
				resultResp, err := p.client.Do(resultReq)
				if err != nil {
					return nil, fmt.Errorf("seedance get result: %w", err)
				}
				defer resultResp.Body.Close()
				if resultResp.StatusCode >= 400 {
					return nil, httpStatusError(p.logger, "seedance", "result", resultResp.StatusCode, resultResp.Body)
				}
				var result seedanceResultResp
				if err := json.NewDecoder(resultResp.Body).Decode(&result); err != nil {
					return nil, fmt.Errorf("seedance decode result: %w", err)
				}
				return &result, nil
			case "failed", "error":
				return nil, fmt.Errorf("seedance generation failed")
			}
			timer.Reset(interval)
		}
	}
}
