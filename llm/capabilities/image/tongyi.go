package image

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
)

// TongyiProvider 使用阿里云通义万相（DashScope）执行图像生成.
// 官方: https://help.aliyun.com/zh/model-studio/   POST .../aigc/text2image/image-synthesis 或 multimodal-generation/generation
// 鉴权: Authorization: Bearer {API_KEY}
const defaultTongyiTimeout = 120 * time.Second

const defaultTongyiBaseURL = "https://dashscope.aliyuncs.com"

// 万相文生图模型: wanx2.1-t2i-turbo, wan2.6-t2i 等
const defaultTongyiModel = "wanx2.1-t2i-turbo"

type TongyiProvider struct {
	cfg    TongyiConfig
	client *http.Client
}

func NewTongyiProvider(cfg TongyiConfig) *TongyiProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultTongyiBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = defaultTongyiModel
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTongyiTimeout
	}
	return &TongyiProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(cfg.Timeout),
	}
}

func (p *TongyiProvider) Name() string { return "tongyi" }

func (p *TongyiProvider) SupportedSizes() []string {
	return []string{"1024x1024", "720x1280", "1280x720", "768x1344", "1344x768", "1280x1280"}
}

// dashscope 文生图请求: model, input.prompt, parameters.size, parameters.n
type tongyiRequest struct {
	Model string `json:"model"`
	Input struct {
		Prompt         string `json:"prompt"`
		NegativePrompt string `json:"negative_prompt,omitempty"`
	} `json:"input"`
	Parameters struct {
		Size string `json:"size,omitempty"` // e.g. "1024*1024"
		N    int    `json:"n,omitempty"`
	} `json:"parameters,omitempty"`
}

type tongyiResponse struct {
	Output struct {
		Results []struct {
			URL string `json:"url,omitempty"`
		} `json:"results,omitempty"`
		TaskStatus string `json:"task_status,omitempty"`
		TaskID     string `json:"task_id,omitempty"`
	} `json:"output"`
	RequestID string `json:"request_id,omitempty"`
}

func (p *TongyiProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	size := req.Size
	if size == "" {
		size = "1024*1024"
	} else {
		size = strings.ReplaceAll(size, "x", "*")
	}
	n := req.N
	if n <= 0 {
		n = 1
	}
	if n > 4 {
		n = 4
	}

	body := tongyiRequest{}
	body.Model = p.cfg.Model
	if req.Model != "" {
		body.Model = req.Model
	}
	body.Input.Prompt = req.Prompt
	body.Input.NegativePrompt = req.NegativePrompt
	body.Parameters.Size = size
	body.Parameters.N = n

	payload, _ := json.Marshal(body)
	// 万相文生图: text2image/image-synthesis (同步) 或 multimodal-generation/generation (wan2.6 同步)
	path := "/api/v1/services/aigc/text2image/image-synthesis"
	url := strings.TrimRight(p.cfg.BaseURL, "/") + path
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("tongyi request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("tongyi request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tongyi error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var tResp tongyiResponse
	if err := json.NewDecoder(resp.Body).Decode(&tResp); err != nil {
		return nil, fmt.Errorf("tongyi decode: %w", err)
	}

	images := make([]ImageData, 0)
	for _, r := range tResp.Output.Results {
		if r.URL != "" {
			images = append(images, ImageData{URL: r.URL})
		}
	}
	if len(images) == 0 && tResp.Output.TaskID != "" {
		return nil, fmt.Errorf("tongyi async task %s not yet ready, use async API or retry", tResp.Output.TaskID)
	}

	return &GenerateResponse{
		Provider:  p.Name(),
		Model:     body.Model,
		Images:    images,
		Usage:     ImageUsage{ImagesGenerated: len(images)},
		CreatedAt: time.Now(),
	}, nil
}

func (p *TongyiProvider) Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("tongyi does not support image editing via this API")
}

func (p *TongyiProvider) CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("tongyi does not support image variations via this API")
}
