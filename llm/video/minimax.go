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

// MiniMaxVideoProvider implements video generation using MiniMax Hailuo AI.
type MiniMaxVideoProvider struct {
	cfg    MiniMaxVideoConfig
	client *http.Client
	logger *zap.Logger
}

const defaultMiniMaxVideoTimeout = 300 * time.Second
const defaultMiniMaxVideoPollInterval = 5 * time.Second

// NewMiniMaxVideoProvider creates a new MiniMax video provider.
func NewMiniMaxVideoProvider(cfg MiniMaxVideoConfig, logger *zap.Logger) *MiniMaxVideoProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.minimax.chat"
	}
	if cfg.Model == "" {
		cfg.Model = "video-01"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultMiniMaxVideoTimeout
	}
	return &MiniMaxVideoProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
		logger: logger,
	}
}

func (p *MiniMaxVideoProvider) Name() string                 { return "minimax-video" }
func (p *MiniMaxVideoProvider) SupportedFormats() []VideoFormat { return []VideoFormat{VideoFormatMP4} }
func (p *MiniMaxVideoProvider) SupportsGeneration() bool     { return true }

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
	return nil, fmt.Errorf("video analysis not supported by minimax-video provider")
}

// Generate creates a video using MiniMax Hailuo AI.
func (p *MiniMaxVideoProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	if err := ValidateGenerateRequest(req); err != nil {
		return nil, err
	}

	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	body := minimaxVideoRequest{
		Model:  model,
		Prompt: req.Prompt,
	}
	if req.ImageURL != "" {
		body.FirstFrameImage = req.ImageURL
	}

	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.cfg.BaseURL+"/v1/video_generation",
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
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("minimax error: status=%d body=%s", resp.StatusCode, string(errBody))
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

	downloadURL, err := p.retrieveFileURL(ctx, queryResp.FileID)
	if err != nil {
		return nil, err
	}

	videos := []VideoData{{URL: downloadURL}}
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
	ticker := time.NewTicker(defaultMiniMaxVideoPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			httpReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s/v1/query/video_generation?task_id=%s", p.cfg.BaseURL, taskID), nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create poll request: %w", err)
			}
			httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

			resp, err := p.client.Do(httpReq)
			if err != nil {
				continue
			}

			var qResp minimaxVideoQueryResponse
			if err := json.NewDecoder(resp.Body).Decode(&qResp); err != nil {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			switch qResp.Status {
			case "Success":
				return &qResp, nil
			case "Fail":
				return nil, fmt.Errorf("minimax generation failed for task %s", taskID)
			}
			// continue polling on Queueing or Processing
		}
	}
}

func (p *MiniMaxVideoProvider) retrieveFileURL(ctx context.Context, fileID string) (string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/v1/files/retrieve?file_id=%s", p.cfg.BaseURL, fileID), nil)
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
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("minimax file retrieve error: status=%d body=%s", resp.StatusCode, string(errBody))
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

