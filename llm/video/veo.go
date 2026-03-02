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

// VeoProvider使用Google Veo 3.1执行视频生成.
type VeoProvider struct {
	cfg    VeoConfig
	client *http.Client
	logger *zap.Logger
}

const defaultVeoDuration = 8
const minVeoDuration = 4
const maxVeoDuration = 20
const defaultVeoAspectRatio = "16:9"
const defaultVeoBaseURL = "https://generativelanguage.googleapis.com"

// NewVeoProvider创建了一个新的Veo视频提供商.
func NewVeoProvider(cfg VeoConfig, logger *zap.Logger) *VeoProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.Model == "" {
		cfg.Model = "veo-3.1-generate-preview"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultVeoBaseURL
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultVideoTimeout
	}

	return &VeoProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
		logger: logger,
	}
}

func (p *VeoProvider) Name() string { return "veo" }

func (p *VeoProvider) SupportedFormats() []VideoFormat {
	return []VideoFormat{VideoFormatMP4}
}

func (p *VeoProvider) SupportsGeneration() bool { return true }

type veoRequest struct {
	Instances  []veoInstance `json:"instances"`
	Parameters veoParams     `json:"parameters,omitempty"`
}

type veoInstance struct {
	Prompt string `json:"prompt"`
	Image  *struct {
		BytesBase64Encoded string `json:"bytesBase64Encoded,omitempty"`
		GcsURI             string `json:"gcsUri,omitempty"`
	} `json:"image,omitempty"`
}

type veoParams struct {
	AspectRatio      string `json:"aspectRatio,omitempty"`
	NegativePrompt   string `json:"negativePrompt,omitempty"`
	PersonGeneration string `json:"personGeneration,omitempty"`
	DurationSeconds  int    `json:"durationSeconds,omitempty"`
	EnhancePrompt    bool   `json:"enhancePrompt,omitempty"`
	GenerateAudio    bool   `json:"generateAudio,omitempty"`
}

type veoResponse struct {
	Predictions []struct {
		Video string `json:"video"`
		Audio string `json:"audio,omitempty"`
	} `json:"predictions"`
}

// Veo不支持分析。
func (p *VeoProvider) Analyze(ctx context.Context, req *AnalyzeRequest) (*AnalyzeResponse, error) {
	_, span := startProviderSpan(ctx, p.Name(), "analyze")
	defer span.End()

	return nil, fmt.Errorf("video analysis not supported by veo provider, use gemini instead")
}

// 生成视频使用Veo 3.1.
func (p *VeoProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
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

	instance := veoInstance{Prompt: req.Prompt}
	if req.Image != "" || req.ImageURL != "" {
		instance.Image = &struct {
			BytesBase64Encoded string `json:"bytesBase64Encoded,omitempty"`
			GcsURI             string `json:"gcsUri,omitempty"`
		}{}
		if req.Image != "" {
			instance.Image.BytesBase64Encoded = req.Image
		} else {
			instance.Image.GcsURI = req.ImageURL
		}
	}

	duration := int(req.Duration)
	if duration == 0 {
		duration = defaultVeoDuration
	}
	if duration < minVeoDuration {
		duration = minVeoDuration
	}
	if duration > maxVeoDuration {
		duration = maxVeoDuration
	}

	aspectRatio := req.AspectRatio
	if aspectRatio == "" {
		aspectRatio = defaultVeoAspectRatio
	}
	p.logger.Info("veo generate start",
		zap.String("model", model),
		zap.String("prompt", shortPromptForLog(req.Prompt)),
		zap.Int("duration", duration),
		zap.String("aspect_ratio", aspectRatio),
		zap.Bool("image_to_video", req.Image != "" || req.ImageURL != ""))

	body := veoRequest{
		Instances: []veoInstance{instance},
		Parameters: veoParams{
			AspectRatio:     aspectRatio,
			NegativePrompt:  req.NegativePrompt,
			DurationSeconds: duration,
			EnhancePrompt:   true,
			GenerateAudio:   true,
		},
	}

	payload, err := marshalJSONRequest("veo", body)
	if err != nil {
		return nil, err
	}
	// Veo 3.1 endpoint: /v1beta/models/{model}:generateVideos.
	url := fmt.Sprintf("%s/models/%s:generateVideos", veoAPIBase(p.cfg.BaseURL),
		model)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("veo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, httpStatusError(p.logger, "veo", "generate", resp.StatusCode, resp.Body)
	}

	var opResp struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&opResp); err != nil {
		return nil, fmt.Errorf("failed to decode veo response: %w", err)
	}

	// 完成投票
	result, err := p.pollGeneration(ctx, opResp.Name)
	if err != nil {
		return nil, err
	}

	var videos []VideoData
	for _, pred := range result.Predictions {
		videos = append(videos, VideoData{
			B64JSON:  pred.Video,
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

func (p *VeoProvider) pollGeneration(ctx context.Context, opName string) (*veoResponse, error) {
	if err := validatePollOperationName(opName); err != nil {
		return nil, fmt.Errorf("invalid veo operation name: %w", err)
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
				return nil, fmt.Errorf("veo polling exceeded max attempts (%d)", maxVideoPollAttempts)
			}
			if attempts == pollSlowWarnThreshold {
				p.logger.Warn("veo polling is taking longer than expected",
					zap.String("operation", opName),
					zap.Int("attempt", attempts))
			}
			if attempts%pollProgressLogEvery == 0 {
				p.logger.Info("veo polling in progress",
					zap.String("operation", opName),
					zap.Int("attempt", attempts))
			}
			url := fmt.Sprintf("%s/%s", veoAPIBase(p.cfg.BaseURL), opName)
			httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}
			httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

			resp, err := p.client.Do(httpReq)
			if err != nil {
				consecutiveErrors++
				p.logger.Warn("veo poll request failed",
					zap.String("operation", opName),
					zap.Int("attempt", attempts),
					zap.Int("consecutive_errors", consecutiveErrors),
					zap.Error(err))
				if consecutiveErrors >= maxVideoPollConsecutiveErrors {
					return nil, fmt.Errorf("veo polling failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				interval = nextPollInterval(interval)
				timer.Reset(interval)
				continue
			}
			if resp.StatusCode >= 400 {
				return nil, statusErrorAndClose(p.logger, "veo", "poll", resp)
			}

			var opStatus struct {
				Done     bool        `json:"done"`
				Response veoResponse `json:"response"`
				Error    *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := decodeJSONAndClose(resp, &opStatus); err != nil {
				consecutiveErrors++
				p.logger.Warn("veo poll decode failed",
					zap.String("operation", opName),
					zap.Int("attempt", attempts),
					zap.Int("consecutive_errors", consecutiveErrors),
					zap.Error(err))
				if consecutiveErrors >= maxVideoPollConsecutiveErrors {
					return nil, fmt.Errorf("veo polling decode failed after %d consecutive errors: %w", consecutiveErrors, err)
				}
				interval = nextPollInterval(interval)
				timer.Reset(interval)
				continue
			}
			consecutiveErrors = 0
			interval = defaultVideoPollInterval

			if opStatus.Error != nil {
				return nil, fmt.Errorf("veo generation failed: %s", opStatus.Error.Message)
			}
			if opStatus.Done {
				p.logger.Info("veo generate complete",
					zap.String("operation", opName),
					zap.Int("videos", len(opStatus.Response.Predictions)))
				return &opStatus.Response, nil
			}
			timer.Reset(interval)
		}
	}
}

func veoAPIBase(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		trimmed = defaultVeoBaseURL
	}
	if strings.HasSuffix(trimmed, "/v1beta") {
		return trimmed
	}
	return trimmed + "/v1beta"
}
