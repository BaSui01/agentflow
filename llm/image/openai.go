package image

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// OpenAIProvider使用OpenAI DALL-E执行图像生成.
type OpenAIProvider struct {
	cfg    OpenAIConfig
	client *http.Client
}

// 新OpenAIProvider创建了新的OpenAI图像提供商.
func NewOpenAIProvider(cfg OpenAIConfig) *OpenAIProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if cfg.Model == "" {
		cfg.Model = "dall-e-3"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	return &OpenAIProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *OpenAIProvider) Name() string { return "openai-image" }

func (p *OpenAIProvider) SupportedSizes() []string {
	return []string{"1024x1024", "1792x1024", "1024x1792"}
}

type dalleRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	Style          string `json:"style,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type dalleResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		URL           string `json:"url,omitempty"`
		B64JSON       string `json:"b64_json,omitempty"`
		RevisedPrompt string `json:"revised_prompt,omitempty"`
	} `json:"data"`
}

// 从文本提示生成图像 。
func (p *OpenAIProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	body := dalleRequest{
		Model:  model,
		Prompt: req.Prompt,
		N:      req.N,
		Size:   req.Size,
	}
	if body.N == 0 {
		body.N = 1
	}
	if body.Size == "" {
		body.Size = "1024x1024"
	}
	if req.Quality != "" {
		body.Quality = req.Quality
	}
	if req.Style != "" {
		body.Style = req.Style
	}
	if req.ResponseFormat != "" {
		body.ResponseFormat = req.ResponseFormat
	}

	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(p.cfg.BaseURL, "/")+"/v1/images/generations",
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("dalle request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dalle error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var dResp dalleResponse
	if err := json.NewDecoder(resp.Body).Decode(&dResp); err != nil {
		return nil, fmt.Errorf("failed to decode dalle response: %w", err)
	}

	images := make([]ImageData, len(dResp.Data))
	for i, d := range dResp.Data {
		images[i] = ImageData{
			URL:           d.URL,
			B64JSON:       d.B64JSON,
			RevisedPrompt: d.RevisedPrompt,
		}
	}

	return &GenerateResponse{
		Provider: p.Name(),
		Model:    model,
		Images:   images,
		Usage: ImageUsage{
			ImagesGenerated: len(images),
		},
		CreatedAt: time.Unix(dResp.Created, 0),
	}, nil
}

// 编辑修改已存在的图像。
func (p *OpenAIProvider) Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error) {
	if req.Image == nil {
		return nil, fmt.Errorf("image is required")
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// 添加图像
	part, err := writer.CreateFormFile("image", "image.png")
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, req.Image); err != nil {
		return nil, err
	}

	// 提供后添加口罩
	if req.Mask != nil {
		maskPart, err := writer.CreateFormFile("mask", "mask.png")
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(maskPart, req.Mask); err != nil {
			return nil, err
		}
	}

	_ = writer.WriteField("prompt", req.Prompt)
	if req.Model != "" {
		_ = writer.WriteField("model", req.Model)
	}
	if req.N > 0 {
		_ = writer.WriteField("n", fmt.Sprintf("%d", req.N))
	}
	if req.Size != "" {
		_ = writer.WriteField("size", req.Size)
	}

	writer.Close()

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(p.cfg.BaseURL, "/")+"/v1/images/edits",
		&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("dalle edit request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dalle edit error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var dResp dalleResponse
	if err := json.NewDecoder(resp.Body).Decode(&dResp); err != nil {
		return nil, err
	}

	images := make([]ImageData, len(dResp.Data))
	for i, d := range dResp.Data {
		images[i] = ImageData{URL: d.URL, B64JSON: d.B64JSON}
	}

	return &GenerateResponse{
		Provider:  p.Name(),
		Model:     req.Model,
		Images:    images,
		CreatedAt: time.Now(),
	}, nil
}

// Create Variation 创建图像的变体 。
func (p *OpenAIProvider) CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error) {
	if req.Image == nil {
		return nil, fmt.Errorf("image is required")
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("image", "image.png")
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, req.Image); err != nil {
		return nil, err
	}

	if req.N > 0 {
		_ = writer.WriteField("n", fmt.Sprintf("%d", req.N))
	}
	if req.Size != "" {
		_ = writer.WriteField("size", req.Size)
	}

	writer.Close()

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(p.cfg.BaseURL, "/")+"/v1/images/variations",
		&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dalle variation error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var dResp dalleResponse
	if err := json.NewDecoder(resp.Body).Decode(&dResp); err != nil {
		return nil, err
	}

	images := make([]ImageData, len(dResp.Data))
	for i, d := range dResp.Data {
		images[i] = ImageData{URL: d.URL, B64JSON: d.B64JSON}
	}

	return &GenerateResponse{
		Provider:  p.Name(),
		Images:    images,
		CreatedAt: time.Now(),
	}, nil
}
