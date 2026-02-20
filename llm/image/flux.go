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

	"github.com/BaSui01/agentflow/internal/tlsutil"
)

// Flux Provider使用 Black Forest Labs Flux执行图像生成.
// API 文件:https://docs.bfl.ai/quick start/生成 images
type FluxProvider struct {
	cfg    FluxConfig
	client *http.Client
}

// NewFlux Provider创建了一个新的Flux图像提供者.
func NewFluxProvider(cfg FluxConfig) *FluxProvider {
	if cfg.BaseURL == "" {
		// 主要全球终点(建议)
		// 区域:api.eu.bfl.ai(欧盟),api.us.bfl.ai(美国)
		cfg.BaseURL = "https://api.bfl.ai"
	}
	if cfg.Model == "" {
		// 可用:通量-2-pro,通量-2-最大,通量-2-弹性,通量-2-克林-4b,通量-2-克林-9b
		// 通通-kontext-max,通通-kontext-pro,通通-pro-1.1-ultra,通通-pro-1.1
		cfg.Model = "flux-2-pro"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	return &FluxProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
	}
}

func (p *FluxProvider) Name() string { return "flux" }

func (p *FluxProvider) SupportedSizes() []string {
	return []string{"1024x1024", "1024x768", "768x1024", "1536x1024", "1024x1536"}
}

type fluxRequest struct {
	Prompt          string  `json:"prompt"`
	AspectRatio     string  `json:"aspect_ratio,omitempty"` // e.g., "1:1", "16:9", "9:16"
	Width           int     `json:"width,omitempty"`
	Height          int     `json:"height,omitempty"`
	Steps           int     `json:"steps,omitempty"`
	Guidance        float64 `json:"guidance,omitempty"`
	Seed            int64   `json:"seed,omitempty"`
	SafetyTolerance int     `json:"safety_tolerance,omitempty"`
	OutputFormat    string  `json:"output_format,omitempty"`
}

type fluxResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	PollingURL string `json:"polling_url,omitempty"` // Must use this URL for polling
	Result     struct {
		Sample string `json:"sample"` // Signed URL (valid 10 min)
	} `json:"result,omitempty"`
}

// 生成使用Flux创建图像.
// 终点:POST /v1/{型号}(例如:/v1/flux-2-pro)
// Auth: x- key 头
func (p *FluxProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	body := fluxRequest{
		Prompt:       req.Prompt,
		OutputFormat: "jpeg",
	}

	// 优于宽度/ 高度的一面 2. x
	if req.Size != "" {
		var width, height int
		fmt.Sscanf(req.Size, "%dx%d", &width, &height)
		// 转换为宽度比
		if width == height {
			body.AspectRatio = "1:1"
		} else if width > height {
			body.AspectRatio = "16:9"
		} else {
			body.AspectRatio = "9:16"
		}
	} else {
		body.AspectRatio = "1:1"
	}

	if req.Steps > 0 {
		body.Steps = req.Steps
	}
	if req.CFGScale > 0 {
		body.Guidance = req.CFGScale
	}
	if req.Seed > 0 {
		body.Seed = req.Seed
	}

	// 提交生成请求
	payload, _ := json.Marshal(body)
	endpoint := fmt.Sprintf("%s/v1/%s", strings.TrimRight(p.cfg.BaseURL, "/"), model)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("x-key", p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("accept", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("flux request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("flux error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var fResp fluxResponse
	if err := json.NewDecoder(resp.Body).Decode(&fResp); err != nil {
		return nil, err
	}

	// 使用投票站(全球终点)进行结果投票
	if fResp.Status == "Pending" || fResp.Status == "Processing" || fResp.Status == "" {
		pollingURL := fResp.PollingURL
		if pollingURL == "" {
			// 遗留终点的倒计时
			pollingURL = fmt.Sprintf("%s/v1/get_result?id=%s", strings.TrimRight(p.cfg.BaseURL, "/"), fResp.ID)
		}
		result, err := p.pollResult(ctx, pollingURL)
		if err != nil {
			return nil, err
		}
		fResp = *result
	}

	images := []ImageData{{
		URL:  fResp.Result.Sample,
		Seed: req.Seed,
	}}

	return &GenerateResponse{
		Provider:  p.Name(),
		Model:     model,
		Images:    images,
		CreatedAt: time.Now(),
	}, nil
}

// 使用投票URL生成合成结果。
// 注意: 已签名的 URLs in result. sample 只有效10分钟.
func (p *FluxProvider) pollResult(ctx context.Context, pollingURL string) (*fluxResponse, error) {
	for i := 0; i < 120; i++ { // Max 120 attempts (4 minutes)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}

		httpReq, err := http.NewRequestWithContext(ctx, "GET", pollingURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		httpReq.Header.Set("x-key", p.cfg.APIKey)
		httpReq.Header.Set("accept", "application/json")

		resp, err := p.client.Do(httpReq)
		if err != nil {
			continue
		}

		var fResp fluxResponse
		json.NewDecoder(resp.Body).Decode(&fResp)
		resp.Body.Close()

		switch fResp.Status {
		case "Ready":
			return &fResp, nil
		case "Error", "Failed":
			return nil, fmt.Errorf("flux generation failed")
		}
		// 继续投票等待、处理等。
	}

	return nil, fmt.Errorf("flux generation timeout")
}

// Flux 不支持编辑 。
func (p *FluxProvider) Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("flux does not support image editing")
}

// CreateVariation不由Flux支持.
func (p *FluxProvider) CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("flux does not support image variations")
}
