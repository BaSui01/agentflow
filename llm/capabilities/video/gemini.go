package video

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	googlegenai "github.com/BaSui01/agentflow/llm/internal/googlegenai"
	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"go.uber.org/zap"
	"google.golang.org/genai"
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

	client, err := googlegenai.NewClient(ctx, googlegenai.ClientConfig{
		APIKey:     p.cfg.APIKey,
		BaseURL:    p.cfg.BaseURL,
		ProjectID:  p.cfg.ProjectID,
		Region:     p.cfg.Location,
		Timeout:    p.cfg.Timeout,
		HTTPClient: p.client,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create google genai client: %w", err)
	}

	parts := make([]*genai.Part, 0, 2)
	mimeType := fmt.Sprintf("video/%s", req.VideoFormat)
	if req.VideoFormat == "" {
		mimeType = "video/mp4"
	}
	if req.VideoData != "" {
		videoBytes, err := base64.StdEncoding.DecodeString(req.VideoData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode video data: %w", err)
		}
		parts = append(parts, genai.NewPartFromBytes(videoBytes, mimeType))
	} else if req.VideoURL != "" {
		parts = append(parts, genai.NewPartFromURI(req.VideoURL, mimeType))
	}
	parts = append(parts, genai.NewPartFromText(req.Prompt))

	resp, err := client.Models.GenerateContent(ctx, model, []*genai.Content{
		genai.NewContentFromParts(parts, genai.RoleUser),
	}, &genai.GenerateContentConfig{
		Temperature:     genai.Ptr(float32(0.4)),
		MaxOutputTokens: 8192,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini video request failed: %w", err)
	}

	result := &AnalyzeResponse{
		Provider:  p.Name(),
		Model:     model,
		Content:   strings.TrimSpace(resp.Text()),
		CreatedAt: time.Now(),
	}
	p.logger.Info("gemini video analyze complete",
		zap.String("model", model),
		zap.Int("content_length", len(result.Content)))
	return result, nil
}

// Generate is not supported by the Gemini video provider.
func (p *GeminiProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	_, span := startProviderSpan(ctx, p.Name(), "generate")
	defer span.End()

	return nil, fmt.Errorf("video generation not supported by gemini provider")
}
