package video

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"go.uber.org/zap"
)

// MiniMaxVideoProvider implements video generation using MiniMax Hailuo AI.
// 官方端点（可被配置 BaseURL 覆盖）：Base https://api.minimax.chat
// POST /v1/video_generation（提交）→ GET /v1/query/video_generation?task_id= （轮询）→ GET /v1/files/retrieve?file_id= （取下载 URL）
type MiniMaxVideoProvider struct {
	cfg    MiniMaxVideoConfig
	client *http.Client
	logger *zap.Logger
}

const minimaxCreatePath = "/v1/video_generation"
const minimaxQueryPath = "/v1/query/video_generation"
const minimaxRetrievePath = "/v1/files/retrieve"
const defaultMiniMaxBaseURL = "https://api.minimax.chat"
const minimaxStatusSuccess = "Success"
const minimaxStatusFail = "Fail"

var minimaxAllowedModels = map[string]struct{}{
	"video-01": {},
}

// NewMiniMaxVideoProvider creates a new MiniMax video provider.
func NewMiniMaxVideoProvider(cfg MiniMaxVideoConfig, logger *zap.Logger) *MiniMaxVideoProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultMiniMaxBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = "video-01"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultVideoTimeout
	}
	return &MiniMaxVideoProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
		logger: logger,
	}
}

func (p *MiniMaxVideoProvider) Name() string                    { return "minimax-video" }
func (p *MiniMaxVideoProvider) SupportedFormats() []VideoFormat { return []VideoFormat{VideoFormatMP4} }
func (p *MiniMaxVideoProvider) SupportsGeneration() bool        { return true }

type minimaxVideoRequest struct {
	Model           string `json:"model"`
	Prompt          string `json:"prompt"`
	FirstFrameImage string `json:"first_frame_image,omitempty"`
}

type minimaxVideoCreateResponse struct {
	TaskID   string `json:"task_id"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

type minimaxVideoQueryResponse struct {
	TaskID   string `json:"task_id"`
	Status   string `json:"status"` // Queueing, Processing, Success, Fail
	FileID   string `json:"file_id,omitempty"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

type minimaxFileResponse struct {
	File struct {
		DownloadURL string `json:"download_url"`
	} `json:"file"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

// Analyze is not supported by the MiniMax video provider.
func (p *MiniMaxVideoProvider) Analyze(ctx context.Context, req *AnalyzeRequest) (*AnalyzeResponse, error) {
	_, span := startProviderSpan(ctx, p.Name(), "analyze")
	defer span.End()

	return nil, fmt.Errorf("video analysis not supported by minimax-video provider")
}

// Generate creates a video using MiniMax Hailuo AI.
func (p *MiniMaxVideoProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
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
	if err := validateAllowedModel("minimax", model, minimaxAllowedModels); err != nil {
		return nil, err
	}
	if req.Duration != 0 || req.AspectRatio != "" || req.Resolution != "" {
		p.logger.Warn("minimax generate ignores unsupported fields",
			zap.Float64("duration", req.Duration),
			zap.String("aspect_ratio", req.AspectRatio),
			zap.String("resolution", req.Resolution))
	}
	p.logger.Info("minimax generate start",
		zap.String("model", model),
		zap.String("prompt", shortPromptForLog(req.Prompt)),
		zap.Bool("image_to_video", req.ImageURL != ""))

	body := minimaxVideoRequest{
		Model:  model,
		Prompt: req.Prompt,
	}
	if req.ImageURL != "" {
		body.FirstFrameImage = req.ImageURL
	}

	payload, err := marshalJSONRequest("minimax", body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.cfg.BaseURL+minimaxCreatePath,
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("minimax request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, httpStatusError(p.logger, "minimax", "generate", resp.StatusCode, resp.Body)
	}

	var createResp minimaxVideoCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return nil, fmt.Errorf("failed to decode minimax create response: %w", err)
	}
	if createResp.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("minimax create error: %s", createResp.BaseResp.StatusMsg)
	}

	queryResp, err := p.pollGeneration(ctx, createResp.TaskID)
	if err != nil {
		return nil, err
	}
	if queryResp.FileID == "" {
		return nil, fmt.Errorf("minimax generation succeeded but returned empty file_id")
	}

	downloadURL, err := p.retrieveFileURL(ctx, queryResp.FileID)
	if err != nil {
		return nil, err
	}

	videos := []VideoData{{URL: downloadURL}}
	p.logger.Info("minimax generate complete",
		zap.String("task_id", queryResp.TaskID),
		zap.Int("videos", len(videos)))
	return &GenerateResponse{
		Provider: p.Name(),
		Model:    model,
		Videos:   videos,
		Usage: VideoUsage{
			VideosGenerated: len(videos),
		},
		CreatedAt: time.Now(),
	}, nil
}

func (p *MiniMaxVideoProvider) pollGeneration(ctx context.Context, taskID string) (*minimaxVideoQueryResponse, error) {
	if err := validatePollTaskID(taskID); err != nil {
		return nil, fmt.Errorf("invalid minimax task id: %w", err)
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
				return nil, fmt.Errorf("minimax polling exceeded max attempts (%d)", maxVideoPollAttempts)
			}
			if attempts == pollSlowWarnThreshold {
				p.logger.Warn("minimax polling is taking longer than expected",
					zap.String("task_id", taskID),
					zap.Int("attempt", attempts))
			}
			if attempts%pollProgressLogEvery == 0 {
				p.logger.Info("minimax polling in progress",
					zap.String("task_id", taskID),
					zap.Int("attempt", attempts))
			}
			httpReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s%s?task_id=%s", p.cfg.BaseURL, minimaxQueryPath, url.QueryEscape(taskID)), nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create poll request: %w", err)
			}
			httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

			resp, err := p.client.Do(httpReq)
			if err != nil {
				consecutiveErrors++
				p.logger.Warn("minimax poll request failed",
					zap.String("task_id", taskID),
					zap.Int("attempt", attempts),
					zap.Int("consecutive_errors", consecutiveErrors),
					zap.Error(err))
				if consecutiveErrors >= maxVideoPollConsecutiveErrors {
					return nil, fmt.Errorf("minimax polling failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				interval = nextPollInterval(interval)
				timer.Reset(interval)
				continue
			}
			if resp.StatusCode >= 400 {
				return nil, statusErrorAndClose(p.logger, "minimax", "poll", resp)
			}

			var qResp minimaxVideoQueryResponse
			if err := decodeJSONAndClose(resp, &qResp); err != nil {
				consecutiveErrors++
				p.logger.Warn("minimax poll decode failed",
					zap.String("task_id", taskID),
					zap.Int("attempt", attempts),
					zap.Int("consecutive_errors", consecutiveErrors),
					zap.Error(err))
				if consecutiveErrors >= maxVideoPollConsecutiveErrors {
					return nil, fmt.Errorf("minimax polling decode failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				interval = nextPollInterval(interval)
				timer.Reset(interval)
				continue
			}
			consecutiveErrors = 0
			interval = defaultVideoPollInterval

			switch qResp.Status {
			case minimaxStatusSuccess:
				return &qResp, nil
			case minimaxStatusFail:
				p.logger.Error("minimax generation failed",
					zap.String("task_id", taskID),
					zap.String("status", qResp.Status),
					zap.String("status_msg", qResp.BaseResp.StatusMsg))
				return nil, fmt.Errorf("minimax generation failed")
			}
			// continue polling on Queueing or Processing
			timer.Reset(interval)
		}
	}
}

func (p *MiniMaxVideoProvider) retrieveFileURL(ctx context.Context, fileID string) (string, error) {
	// MiniMax video generation is a provider-specific three-stage workflow:
	// create task -> poll task status -> retrieve file URL by file_id.
	if err := validatePollTaskID(fileID); err != nil {
		return "", fmt.Errorf("invalid minimax file id: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s%s?file_id=%s", p.cfg.BaseURL, minimaxRetrievePath, url.QueryEscape(fileID)), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create file retrieve request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("minimax file retrieve request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", httpStatusError(p.logger, "minimax", "retrieve_file", resp.StatusCode, resp.Body)
	}

	var fileResp minimaxFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&fileResp); err != nil {
		return "", fmt.Errorf("failed to decode minimax file response: %w", err)
	}
	if fileResp.BaseResp.StatusCode != 0 {
		return "", fmt.Errorf("minimax file retrieve error: %s", fileResp.BaseResp.StatusMsg)
	}

	return fileResp.File.DownloadURL, nil
}
