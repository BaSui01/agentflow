package video

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Runway Provider执行视频生成,使用Runway ML Gen-4.
// API 文件: https://docs.dev.runwayml.com/api/
type RunwayProvider struct {
	cfg    RunwayConfig
	client *http.Client
}

// NewRunway Provider创建了新的跑道视频提供商.
func NewRunwayProvider(cfg RunwayConfig) *RunwayProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.runwayml.com"
	}
	if cfg.Model == "" {
		// 可用: gen4 turbo, gen3a turbo, veo3.1, veo3.1 快活, veo3
		cfg.Model = "gen4_turbo"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 300 * time.Second
	}

	return &RunwayProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
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
	return nil, fmt.Errorf("video analysis not supported by runway provider")
}

// 利用跑道Gen-4生成视频.
// 终点: POST /v1/image to video
// Auth: 熊克令牌 + X- Runway-Version 信头
func (p *RunwayProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	duration := int(req.Duration)
	if duration == 0 {
		duration = 5
	}
	if duration < 2 {
		duration = 2
	}
	if duration > 10 {
		duration = 10
	}

	// 转换宽比格式
	ratio := "1280:720" // default 16:9
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

	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.cfg.BaseURL+"/v1/image_to_video",
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
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("runway error: status=%d body=%s", resp.StatusCode, string(errBody))
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
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			httpReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s/v1/tasks/%s", p.cfg.BaseURL, id), nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}
			httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
			httpReq.Header.Set("X-Runway-Version", "2024-11-06")

			resp, err := p.client.Do(httpReq)
			if err != nil {
				continue
			}

			var rResp runwayResponse
			json.NewDecoder(resp.Body).Decode(&rResp)
			resp.Body.Close()

			switch rResp.Status {
			case "SUCCEEDED":
				return &rResp, nil
			case "FAILED":
				if rResp.Failure != "" {
					return nil, fmt.Errorf("runway generation failed: %s", rResp.Failure)
				}
				return nil, fmt.Errorf("runway generation failed")
			}
			// 继续投票 PENDING,跑
		}
	}
}
