package image

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// 双子公司利用Google双子公司本土多模式能力实现图像生成.
type GeminiProvider struct {
	cfg    GeminiConfig
	client *http.Client
}

// NewGemini Provider创建了新的双子星图像提供者.
func NewGeminiProvider(cfg GeminiConfig) *GeminiProvider {
	if cfg.Model == "" {
		cfg.Model = "gemini-3-pro-image-preview"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	return &GeminiProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *GeminiProvider) Name() string { return "gemini-image" }

func (p *GeminiProvider) SupportedSizes() []string {
	return []string{"1024x1024", "1536x1536", "1024x1536", "1536x1024"}
}

type geminiPart struct {
	Text       string        `json:"text,omitempty"`
	InlineData *geminiInline `json:"inlineData,omitempty"`
}

type geminiInline struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role,omitempty"`
}

type geminiImageRequest struct {
	Contents         []geminiContent       `json:"contents"`
	GenerationConfig *geminiImageGenConfig `json:"generationConfig,omitempty"`
}

type geminiImageGenConfig struct {
	ResponseMimeType   string   `json:"responseMimeType,omitempty"`
	ResponseModalities []string `json:"responseModalities,omitempty"`
}

type geminiImageResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text       string `json:"text,omitempty"`
				InlineData *struct {
					MimeType string `json:"mimeType"`
					Data     string `json:"data"`
				} `json:"inlineData,omitempty"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

// 利用双子座的原生图像生成图像.
func (p *GeminiProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	body := geminiImageRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{{Text: req.Prompt}},
				Role:  "user",
			},
		},
		GenerationConfig: &geminiImageGenConfig{
			ResponseModalities: []string{"IMAGE"},
		},
	}

	payload, _ := json.Marshal(body)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		model, p.cfg.APIKey)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini image request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini image error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var gResp geminiImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&gResp); err != nil {
		return nil, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	var images []ImageData
	for _, candidate := range gResp.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil {
				images = append(images, ImageData{
					B64JSON: part.InlineData.Data,
				})
			}
		}
	}

	return &GenerateResponse{
		Provider: p.Name(),
		Model:    model,
		Images:   images,
		Usage: ImageUsage{
			ImagesGenerated: len(images),
		},
		CreatedAt: time.Now(),
	}, nil
}

// 利用双子座多模式能力编辑修改已存在的图像.
func (p *GeminiProvider) Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error) {
	if req.Image == nil {
		return nil, fmt.Errorf("image is required")
	}

	imageData, err := io.ReadAll(req.Image)
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}

	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	body := geminiImageRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{
						InlineData: &geminiInline{
							MimeType: "image/png",
							Data:     base64.StdEncoding.EncodeToString(imageData),
						},
					},
					{Text: req.Prompt},
				},
				Role: "user",
			},
		},
		GenerationConfig: &geminiImageGenConfig{
			ResponseModalities: []string{"IMAGE"},
		},
	}

	payload, _ := json.Marshal(body)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		model, p.cfg.APIKey)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini edit request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini edit error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var gResp geminiImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&gResp); err != nil {
		return nil, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	var images []ImageData
	for _, candidate := range gResp.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil {
				images = append(images, ImageData{
					B64JSON: part.InlineData.Data,
				})
			}
		}
	}

	return &GenerateResponse{
		Provider:  p.Name(),
		Model:     model,
		Images:    images,
		CreatedAt: time.Now(),
	}, nil
}

// 创建变异使用双子座创建变异.
func (p *GeminiProvider) CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error) {
	if req.Image == nil {
		return nil, fmt.Errorf("image is required")
	}

	editReq := &EditRequest{
		Image:  req.Image,
		Prompt: "Create a variation of this image with similar style and composition but different details",
		Model:  req.Model,
	}

	return p.Edit(ctx, editReq)
}
