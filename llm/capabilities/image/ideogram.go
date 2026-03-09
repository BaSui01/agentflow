package image

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
)

// IdeogramProvider 使用 Ideogram 3.0 执行图像生成.
// 官方: https://developer.ideogram.ai  POST /v1/ideogram-v3/generate (multipart/form-data), Api-Key header.
const defaultIdeogramTimeout = 120 * time.Second

const defaultIdeogramBaseURL = "https://api.ideogram.ai"

type IdeogramProvider struct {
	cfg    IdeogramConfig
	client *http.Client
}

func NewIdeogramProvider(cfg IdeogramConfig) *IdeogramProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultIdeogramBaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultIdeogramTimeout
	}
	return &IdeogramProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(cfg.Timeout),
	}
}

func (p *IdeogramProvider) Name() string { return "ideogram" }

func (p *IdeogramProvider) SupportedSizes() []string {
	return []string{"1024x1024", "1024x768", "768x1024", "832x1216", "1216x832", "896x1152", "1152x896"}
}

// ideogramResponse: created, data[] with url, prompt, seed
type ideogramResponse struct {
	Created string `json:"created"`
	Data    []struct {
		URL    string `json:"url"`
		Prompt string `json:"prompt,omitempty"`
		Seed   int64  `json:"seed,omitempty"`
	} `json:"data"`
}

func (p *IdeogramProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	_ = w.WriteField("prompt", req.Prompt)
	n := req.N
	if n <= 0 {
		n = 1
	}
	if n > 4 {
		n = 4
	}
	_ = w.WriteField("num_images", strconv.Itoa(n))
	if req.NegativePrompt != "" {
		_ = w.WriteField("negative_prompt", req.NegativePrompt)
	}
	aspectRatio := sizeToIdeogramAspectRatio(req.Size)
	_ = w.WriteField("aspect_ratio", aspectRatio)
	if req.Seed != 0 {
		_ = w.WriteField("seed", strconv.FormatInt(req.Seed, 10))
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("ideogram multipart: %w", err)
	}

	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1/ideogram-v3/generate"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, fmt.Errorf("ideogram request: %w", err)
	}
	httpReq.Header.Set("Api-Key", p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ideogram request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ideogram error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var iResp ideogramResponse
	if err := json.NewDecoder(resp.Body).Decode(&iResp); err != nil {
		return nil, fmt.Errorf("ideogram decode: %w", err)
	}

	images := make([]ImageData, 0, len(iResp.Data))
	for _, d := range iResp.Data {
		images = append(images, ImageData{URL: d.URL, RevisedPrompt: d.Prompt, Seed: d.Seed})
	}

	createdAt := time.Now()
	if iResp.Created != "" {
		if t, err := time.Parse(time.RFC3339, iResp.Created); err == nil {
			createdAt = t
		}
	}

	return &GenerateResponse{
		Provider:  p.Name(),
		Model:     p.cfg.Model,
		Images:    images,
		Usage:     ImageUsage{ImagesGenerated: len(images)},
		CreatedAt: createdAt,
	}, nil
}

func sizeToIdeogramAspectRatio(size string) string {
	if size == "" {
		return "1x1"
	}
	parts := strings.SplitN(size, "x", 2)
	if len(parts) != 2 {
		return "1x1"
	}
	w, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	if w <= 0 || h <= 0 {
		return "1x1"
	}
	if w == h {
		return "1x1"
	}
	if w > h {
		if w*3 == h*4 {
			return "4x3"
		}
		if w*9 == h*16 {
			return "16x9"
		}
		return "4x3"
	}
	if w*4 == h*3 {
		return "3x4"
	}
	if w*16 == h*9 {
		return "9x16"
	}
	return "3x4"
}

func (p *IdeogramProvider) Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("ideogram does not support image editing via this API")
}

func (p *IdeogramProvider) CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("ideogram does not support image variations via this API")
}
