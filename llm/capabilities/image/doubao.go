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

// DoubaoProvider 使用火山引擎/豆包 Seedream 执行图像生成.
// 官方: 火山方舟 https://www.volcengine.com/docs/82379/1666945  POST {endpoint}/v1/images/generations, Authorization: Bearer {API_KEY}
const defaultDoubaoTimeout = 120 * time.Second

const defaultDoubaoBaseURL = "https://ark.cn-beijing.volces.com"

const defaultDoubaoModel = "doubao-seedream-4-0-250828"

type DoubaoProvider struct {
	cfg    DoubaoConfig
	client *http.Client
}

func NewDoubaoProvider(cfg DoubaoConfig) *DoubaoProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultDoubaoBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = defaultDoubaoModel
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultDoubaoTimeout
	}
	return &DoubaoProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(cfg.Timeout),
	}
}

func (p *DoubaoProvider) Name() string { return "doubao" }

func (p *DoubaoProvider) SupportedSizes() []string {
	return []string{"1024x1024", "1280x720", "720x1280", "2048x2048"}
}

type doubaoRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Size   string `json:"size,omitempty"`
	N      int    `json:"n,omitempty"`
}

type doubaoResponse struct {
	Data []struct {
		URL     string `json:"url,omitempty"`
		B64JSON string `json:"b64_json,omitempty"`
	} `json:"data"`
}

func (p *DoubaoProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}
	size := req.Size
	if size == "" {
		size = "1024x1024"
	}
	n := req.N
	if n <= 0 {
		n = 1
	}
	if n > 4 {
		n = 4
	}

	body := doubaoRequest{
		Model:  model,
		Prompt: req.Prompt,
		Size:   size,
		N:      n,
	}

	payload, _ := json.Marshal(body)
	path := "/v1/images/generations"
	url := strings.TrimRight(p.cfg.BaseURL, "/") + path
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("doubao request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("doubao request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("doubao error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var dResp doubaoResponse
	if err := json.NewDecoder(resp.Body).Decode(&dResp); err != nil {
		return nil, fmt.Errorf("doubao decode: %w", err)
	}

	images := make([]ImageData, 0, len(dResp.Data))
	for _, d := range dResp.Data {
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

func (p *DoubaoProvider) Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("doubao does not support image editing via this API")
}

func (p *DoubaoProvider) CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("doubao does not support image variations via this API")
}
