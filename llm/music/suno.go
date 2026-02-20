package music

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/internal/tlsutil"
)

// SunoProvider使用Suno API执行音乐生成.
type SunoProvider struct {
	cfg    SunoConfig
	client *http.Client
}

// NewSunoProvider创建了新的Suno音乐提供商.
func NewSunoProvider(cfg SunoConfig) *SunoProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.sunoapi.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "suno-v5"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 300 * time.Second
	}
	return &SunoProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
	}
}

func (p *SunoProvider) Name() string { return "suno" }

type sunoRequest struct {
	Prompt       string `json:"prompt"`
	Style        string `json:"style,omitempty"`
	Model        string `json:"model,omitempty"`
	Instrumental bool   `json:"instrumental,omitempty"`
	Duration     int    `json:"duration,omitempty"`
}

type sunoResponse struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
	Data   []struct {
		ID       string  `json:"id"`
		AudioURL string  `json:"audio_url"`
		Duration float64 `json:"duration"`
		Title    string  `json:"title"`
		Lyrics   string  `json:"lyrics"`
		Style    string  `json:"style"`
	} `json:"data"`
}

// 生成音乐使用Suno API创建.
func (p *SunoProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	body := sunoRequest{
		Prompt:       req.Prompt,
		Style:        req.Style,
		Model:        model,
		Instrumental: req.Instrumental,
		Duration:     int(req.Duration),
	}

	payload, _ := json.Marshal(body)
	endpoint := fmt.Sprintf("%s/suno/create", strings.TrimRight(p.cfg.BaseURL, "/"))

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("suno request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("suno error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var sResp sunoResponse
	if err := json.NewDecoder(resp.Body).Decode(&sResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Async 需要填写
	if sResp.Status == "pending" || sResp.Status == "processing" {
		result, err := p.pollTask(ctx, sResp.TaskID)
		if err != nil {
			return nil, err
		}
		sResp = *result
	}

	var tracks []MusicData
	var totalDuration float64
	for _, d := range sResp.Data {
		tracks = append(tracks, MusicData{
			ID:       d.ID,
			URL:      d.AudioURL,
			Duration: d.Duration,
			Title:    d.Title,
			Lyrics:   d.Lyrics,
			Style:    d.Style,
		})
		totalDuration += d.Duration
	}

	return &GenerateResponse{
		Provider: p.Name(),
		Model:    model,
		Tracks:   tracks,
		Usage: MusicUsage{
			TracksGenerated: len(tracks),
			DurationSeconds: totalDuration,
		},
		CreatedAt: time.Now(),
	}, nil
}

func (p *SunoProvider) pollTask(ctx context.Context, taskID string) (*sunoResponse, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			endpoint := fmt.Sprintf("%s/suno/task/%s", strings.TrimRight(p.cfg.BaseURL, "/"), taskID)
			httpReq, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}
			httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

			resp, err := p.client.Do(httpReq)
			if err != nil {
				continue
			}

			var sResp sunoResponse
			json.NewDecoder(resp.Body).Decode(&sResp)
			resp.Body.Close()

			if sResp.Status == "completed" || sResp.Status == "success" {
				return &sResp, nil
			}
			if sResp.Status == "failed" || sResp.Status == "error" {
				return nil, fmt.Errorf("suno generation failed")
			}
		}
	}
}
