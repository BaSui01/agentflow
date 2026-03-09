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

// ZhipuProvider 使用智谱 AI GLM-Image/CogView 执行图像生成.
// 官方: https://open.bigmodel.cn  POST /api/paas/v4/images/generations, Authorization: Bearer {API_KEY}
const defaultZhipuTimeout = 120 * time.Second

const defaultZhipuBaseURL = "https://open.bigmodel.cn"

const defaultZhipuModel = "glm-image"

type ZhipuProvider struct {
	cfg    ZhipuConfig
	client *http.Client
}

func NewZhipuProvider(cfg ZhipuConfig) *ZhipuProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultZhipuBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = defaultZhipuModel
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultZhipuTimeout
	}
	return &ZhipuProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(cfg.Timeout),
	}
}

func (p *ZhipuProvider) Name() string { return "zhipu" }

func (p *ZhipuProvider) SupportedSizes() []string {
	return []string{"1280x1280", "1568x1056", "1056x1568", "1472x1088", "1088x1472", "1728x960", "960x1728", "1024x1024"}
}

type zhipuRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Size   string `json:"size,omitempty"`
	N      int    `json:"n,omitempty"`
}

type zhipuResponse struct {
	Data []struct {
		URL     string `json:"url,omitempty"`
		B64JSON string `json:"b64_json,omitempty"`
	} `json:"data"`
}

func (p *ZhipuProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}
	size := req.Size
	if size == "" {
		size = "1280x1280"
	}
	n := req.N
	if n <= 0 {
		n = 1
	}
	if n > 4 {
		n = 4
	}

	body := zhipuRequest{
		Model:  model,
		Prompt: req.Prompt,
		Size:   size,
		N:      n,
	}

	payload, _ := json.Marshal(body)
	path := "/api/paas/v4/images/generations"
	url := strings.TrimRight(p.cfg.BaseURL, "/") + path
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("zhipu request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("zhipu request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("zhipu error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var zResp zhipuResponse
	if err := json.NewDecoder(resp.Body).Decode(&zResp); err != nil {
		return nil, fmt.Errorf("zhipu decode: %w", err)
	}

	images := make([]ImageData, 0, len(zResp.Data))
	for _, d := range zResp.Data {
		if d.URL != "" {
			images = append(images, ImageData{URL: d.URL})
		} else if d.B64JSON != "" {
			images = append(images, ImageData{B64JSON: d.B64JSON})
		}
	}

	return &GenerateResponse{
		Provider:  p.Name(),
		Model:     model,
		Images:    images,
		Usage:     ImageUsage{ImagesGenerated: len(images)},
		CreatedAt: time.Now(),
	}, nil
}

func (p *ZhipuProvider) Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("zhipu does not support image editing via this API")
}

func (p *ZhipuProvider) CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("zhipu does not support image variations via this API")
}
