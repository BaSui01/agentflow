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

// VeoProvider使用Google Veo 3.1执行视频生成.
type VeoProvider struct {
	cfg    VeoConfig
	client *http.Client
}

// NewVeoProvider创建了一个新的Veo视频提供商.
func NewVeoProvider(cfg VeoConfig) *VeoProvider {
	if cfg.Model == "" {
		cfg.Model = "veo-3.1-generate-preview"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 300 * time.Second
	}

	return &VeoProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
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
	return nil, fmt.Errorf("video analysis not supported by veo provider, use gemini instead")
}

// 生成视频使用Veo 3.1.
func (p *VeoProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
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
		duration = 8
	}

	body := veoRequest{
		Instances: []veoInstance{instance},
		Parameters: veoParams{
			AspectRatio:     req.AspectRatio,
			NegativePrompt:  req.NegativePrompt,
			DurationSeconds: duration,
			EnhancePrompt:   true,
			GenerateAudio:   true,
		},
	}

	payload, _ := json.Marshal(body)
	// Veo 3.1使用双子座API端点:/models/{model}:generateVideos
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateVideos?key=%s",
		model, p.cfg.APIKey)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("veo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("veo error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var opResp struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&opResp); err != nil {
		return nil, fmt.Errorf("failed to decode veo response: %w", err)
	}

	// 完成投票
	result, err := p.pollOperation(ctx, opResp.Name)
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

func (p *VeoProvider) pollOperation(ctx context.Context, opName string) (*veoResponse, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/%s?key=%s", opName, p.cfg.APIKey)
			httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}

			resp, err := p.client.Do(httpReq)
			if err != nil {
				continue
			}

			var opStatus struct {
				Done     bool        `json:"done"`
				Response veoResponse `json:"response"`
				Error    *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			json.NewDecoder(resp.Body).Decode(&opStatus)
			resp.Body.Close()

			if opStatus.Error != nil {
				return nil, fmt.Errorf("veo generation failed: %s", opStatus.Error.Message)
			}
			if opStatus.Done {
				return &opStatus.Response, nil
			}
		}
	}
}
