package video

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"go.uber.org/zap"
)

// GeminiProvider analyzes video content using Google Gemini.
type GeminiProvider struct {
	cfg    GeminiConfig
	client *http.Client
	logger *zap.Logger
}

// NewGeminiProvider creates a new Gemini video provider.
func NewGeminiProvider(cfg GeminiConfig, logger *zap.Logger) *GeminiProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.Model == "" {
		cfg.Model = "gemini-3-flash-preview"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 180 * time.Second
	}

	return &GeminiProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
		logger: logger,
	}
}

func (p *GeminiProvider) Name() string { return "gemini-video" }

func (p *GeminiProvider) SupportedFormats() []VideoFormat {
	return []VideoFormat{VideoFormatMP4, VideoFormatWebM, VideoFormatMOV, VideoFormatAVI, VideoFormatMKV}
}

func (p *GeminiProvider) SupportsGeneration() bool { return false }

type geminiVideoPart struct {
	Text       string          `json:"text,omitempty"`
	InlineData *geminiInline   `json:"inline_data,omitempty"`
	FileData   *geminiFileData `json:"file_data,omitempty"`
}

type geminiInline struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type geminiFileData struct {
	MimeType string `json:"mime_type"`
	FileURI  string `json:"file_uri"`
}

type geminiContent struct {
	Parts []geminiVideoPart `json:"parts"`
	Role  string            `json:"role,omitempty"`
}

type geminiRequest struct {
	Contents         []geminiContent  `json:"contents"`
	GenerationConfig *geminiGenConfig `json:"generationConfig,omitempty"`
}

type geminiGenConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

// Analyze processes video understanding requests with Gemini multimodal capabilities.
func (p *GeminiProvider) Analyze(ctx context.Context, req *AnalyzeRequest) (*AnalyzeResponse, error) {
	ctx, span := startProviderSpan(ctx, p.Name(), "analyze")
	defer span.End()

	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}
	p.logger.Info("gemini video analyze start",
		zap.String("model", model),
		zap.String("prompt", shortPromptForLog(req.Prompt)),
		zap.Bool("has_video_data", req.VideoData != ""),
		zap.Bool("has_video_url", req.VideoURL != ""))

	var parts []geminiVideoPart

	// Add video content.
	if req.VideoData != "" {
		mimeType := fmt.Sprintf("video/%s", req.VideoFormat)
		if req.VideoFormat == "" {
			mimeType = "video/mp4"
		}
		parts = append(parts, geminiVideoPart{
			InlineData: &geminiInline{
				MimeType: mimeType,
				Data:     req.VideoData,
			},
		})
	} else if req.VideoURL != "" {
		mimeType := fmt.Sprintf("video/%s", req.VideoFormat)
		if req.VideoFormat == "" {
			mimeType = "video/mp4"
		}
		parts = append(parts, geminiVideoPart{
			FileData: &geminiFileData{
				MimeType: mimeType,
				FileURI:  req.VideoURL,
			},
		})
	}

	// Add the user prompt.
	parts = append(parts, geminiVideoPart{Text: req.Prompt})

	body := geminiRequest{
		Contents: []geminiContent{{Parts: parts, Role: "user"}},
		GenerationConfig: &geminiGenConfig{
			Temperature:     0.4,
			MaxOutputTokens: 8192,
		},
	}

	payload, err := marshalJSONRequest("gemini video", body)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent",
		model)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini video request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, httpStatusError(p.logger, "gemini", "analyze", resp.StatusCode, resp.Body)
	}

	var gResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gResp); err != nil {
		return nil, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	content := ""
	if len(gResp.Candidates) > 0 && len(gResp.Candidates[0].Content.Parts) > 0 {
		content = gResp.Candidates[0].Content.Parts[0].Text
	}

	result := &AnalyzeResponse{
		Provider:  p.Name(),
		Model:     model,
		Content:   content,
		CreatedAt: time.Now(),
	}
	p.logger.Info("gemini video analyze complete",
		zap.String("model", model),
		zap.Int("content_length", len(content)))
	return result, nil
}

// Generate is not supported by the Gemini video provider.
func (p *GeminiProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	_, span := startProviderSpan(ctx, p.Name(), "generate")
	defer span.End()

	return nil, fmt.Errorf("video generation not supported by gemini provider")
}
