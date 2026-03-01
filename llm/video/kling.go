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

// KlingProvider implements video generation using Kling AI.
type KlingProvider struct {
	cfg    KlingConfig
	client *http.Client
	logger *zap.Logger
}

const defaultKlingTimeout = 300 * time.Second
const defaultKlingPollInterval = 5 * time.Second

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
		timeout = defaultKlingTimeout
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

type klingTextRequest struct {
	Model          string  `json:"model"`
	Prompt         string  `json:"prompt"`
	NegativePrompt string  `json:"negative_prompt,omitempty"`
	Duration       int     `json:"duration,omitempty"`
	AspectRatio    string  `json:"aspect_ratio,omitempty"`
	CfgScale       float64 `json:"cfg_scale,omitempty"`
}

type klingImageRequest struct {
	Model       string `json:"model"`
	Prompt      string `json:"prompt,omitempty"`
	Image       string `json:"image"`
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
	return nil, fmt.Errorf("video analysis not supported by kling provider")
}

// Generate creates a video using Kling AI.
func (p *KlingProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	if err := ValidateGenerateRequest(req); err != nil {
		return nil, err
	}

	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	duration := int(req.Duration)
	if duration == 0 {
		duration = 5
	}
	if duration < 3 {
		duration = 3
	}
	if duration > 15 {
		duration = 15
	}

	aspectRatio := req.AspectRatio
	if aspectRatio == "" {
		aspectRatio = "16:9"
	}

	var (
		endpoint string
		payload  []byte
	)

	if req.ImageURL != "" {
		endpoint = p.cfg.BaseURL + "/v1/videos/image2video"
		body := klingImageRequest{
			Model:       model,
			Prompt:      req.Prompt,
			Image:       req.ImageURL,
			Duration:    duration,
			AspectRatio: aspectRatio,
		}
		payload, _ = json.Marshal(body)
	} else {
		endpoint = p.cfg.BaseURL + "/v1/videos/text2video"
		body := klingTextRequest{
			Model:          model,
			Prompt:         req.Prompt,
			NegativePrompt: req.NegativePrompt,
			Duration:       duration,
			AspectRatio:    aspectRatio,
		}
		payload, _ = json.Marshal(body)
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
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kling error: status=%d body=%s", resp.StatusCode, string(errBody))
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
	ticker := time.NewTicker(defaultKlingPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			httpReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s/v1/videos/%s", p.cfg.BaseURL, taskID), nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}
			httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

			resp, err := p.client.Do(httpReq)
			if err != nil {
				continue
			}

			var kResp klingResponse
			if err := json.NewDecoder(resp.Body).Decode(&kResp); err != nil {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			switch kResp.TaskStatus {
			case "succeed":
				return &kResp, nil
			case "failed":
				if kResp.TaskStatusMsg != "" {
					return nil, fmt.Errorf("kling generation failed: %s", kResp.TaskStatusMsg)
				}
				return nil, fmt.Errorf("kling generation failed")
			}
			// continue polling for submitted, processing
		}
	}
}

