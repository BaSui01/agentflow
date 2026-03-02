package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/structured"
	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/image"
	"github.com/BaSui01/agentflow/llm/multimodal"
	"github.com/BaSui01/agentflow/llm/providers"
	vendorprofile "github.com/BaSui01/agentflow/llm/providers/vendor"
	"github.com/BaSui01/agentflow/llm/video"
	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

const (
	defaultReferenceBytes = 8 << 20 // 8MB
	defaultReferenceTTL   = 2 * time.Hour
	defaultChatModel      = "gpt-4o-mini"
	defaultNegativeText   = "blurry, low quality, watermark, text, logo, signature, bad anatomy, deformed, mutated"
)

var blockedReferenceIPPrefixes = buildBlockedReferenceIPPrefixes()

// PromptContext carries modality-specific prompt build options.
type PromptContext struct {
	Modality       string
	BasePrompt     string
	Advanced       bool
	StyleTokens    []string
	QualityTokens  []string
	NegativePrompt string
	Camera         string
	Mood           string
}

// PromptResult is the normalized prompt output.
type PromptResult struct {
	Prompt         string
	NegativePrompt string
}

// PromptPipeline allows framework users to inject custom prompt composition logic.
type PromptPipeline interface {
	Build(ctx context.Context, in PromptContext) (PromptResult, error)
}

// DefaultPromptPipeline provides a generic, domain-agnostic prompt composition strategy.
type DefaultPromptPipeline struct{}

func (p *DefaultPromptPipeline) Build(ctx context.Context, in PromptContext) (PromptResult, error) {
	_ = ctx
	if !in.Advanced {
		return PromptResult{
			Prompt:         strings.TrimSpace(in.BasePrompt),
			NegativePrompt: strings.TrimSpace(in.NegativePrompt),
		}, nil
	}

	pieces := []string{}
	switch in.Modality {
	case "image":
		pieces = append(pieces, strings.Join(in.StyleTokens, ", "))
		pieces = append(pieces, strings.TrimSpace(in.BasePrompt))
		pieces = append(pieces, strings.Join(in.QualityTokens, ", "))
	case "video":
		if strings.TrimSpace(in.Camera) != "" {
			pieces = append(pieces, "Camera: "+strings.TrimSpace(in.Camera))
		}
		if strings.TrimSpace(in.Mood) != "" {
			pieces = append(pieces, "Mood: "+strings.TrimSpace(in.Mood))
		}
		if len(in.StyleTokens) > 0 {
			pieces = append(pieces, "Style: "+strings.Join(in.StyleTokens, ", "))
		}
		pieces = append(pieces, strings.TrimSpace(in.BasePrompt))
	default:
		pieces = append(pieces, strings.TrimSpace(in.BasePrompt))
	}

	return PromptResult{
		Prompt:         strings.Join(filterEmptyStrings(pieces), ". "),
		NegativePrompt: strings.TrimSpace(in.NegativePrompt),
	}, nil
}

type MultimodalHandlerConfig struct {
	ChatProvider         llm.Provider
	OpenAIAPIKey         string
	OpenAIBaseURL        string
	GoogleAPIKey         string
	GoogleBaseURL        string
	RunwayAPIKey         string
	RunwayBaseURL        string
	VeoAPIKey            string
	VeoBaseURL           string
	SoraAPIKey           string
	SoraBaseURL          string
	KlingAPIKey          string
	KlingBaseURL         string
	LumaAPIKey           string
	LumaBaseURL          string
	MiniMaxAPIKey        string
	MiniMaxBaseURL       string
	DefaultImageProvider string
	DefaultVideoProvider string
	ReferenceMaxSize     int64
	ReferenceTTL         time.Duration
	ReferenceStore       ReferenceStore
	Pipeline             PromptPipeline
}

type referenceAsset struct {
	ID        string    `json:"id"`
	FileName  string    `json:"file_name"`
	MimeType  string    `json:"mime_type"`
	Size      int       `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	Data      []byte    `json:"-"`
}

type MultimodalHandler struct {
	logger       *zap.Logger
	chatProvider llm.Provider
	router       *multimodal.Router
	pipeline     PromptPipeline

	defaultImageProvider string
	defaultVideoProvider string
	imageProviders       []string
	videoProviders       []string

	promptEnhancer  *agent.PromptEnhancer
	promptOptimizer *agent.PromptOptimizer

	referenceMaxSize int64
	referenceTTL     time.Duration
	referenceStore   ReferenceStore
}

func NewMultimodalHandlerFromConfig(cfg MultimodalHandlerConfig, logger *zap.Logger) *MultimodalHandler {
	if logger == nil {
		logger = zap.NewNop()
	}

	imageProviders := map[string]image.Provider{}
	videoProviders := map[string]video.Provider{}

	defaultImage := strings.TrimSpace(cfg.DefaultImageProvider)
	defaultVideo := strings.TrimSpace(cfg.DefaultVideoProvider)

	if cfg.OpenAIAPIKey != "" {
		openaiProfile := vendorprofile.NewOpenAIProfile(vendorprofile.OpenAIConfig{
			APIKey:  cfg.OpenAIAPIKey,
			BaseURL: cfg.OpenAIBaseURL,
		}, logger)
		imageProviders["openai"] = openaiProfile.Image
		if defaultImage == "" {
			defaultImage = "openai"
		}
	}
	if cfg.GoogleAPIKey != "" {
		geminiProfile := vendorprofile.NewGeminiProfile(vendorprofile.GeminiConfig{
			APIKey:  cfg.GoogleAPIKey,
			BaseURL: cfg.GoogleBaseURL,
		}, logger)
		imageProviders["gemini"] = geminiProfile.Image
		videoProviders["veo"] = geminiProfile.Video
		if defaultImage == "" {
			defaultImage = "gemini"
		}
		if defaultVideo == "" {
			defaultVideo = "veo"
		}
	}
	if cfg.VeoAPIKey != "" {
		videoProviders["veo"] = video.NewVeoProvider(video.VeoConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.VeoAPIKey,
				BaseURL: cfg.VeoBaseURL,
			},
		}, logger)
		if defaultVideo == "" {
			defaultVideo = "veo"
		}
	}
	if cfg.RunwayAPIKey != "" {
		videoProviders["runway"] = video.NewRunwayProvider(video.RunwayConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.RunwayAPIKey,
				BaseURL: cfg.RunwayBaseURL,
			},
		}, logger)
		if defaultVideo == "" {
			defaultVideo = "runway"
		}
	}
	if cfg.SoraAPIKey != "" {
		videoProviders["sora"] = video.NewSoraProvider(video.SoraConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.SoraAPIKey,
				BaseURL: cfg.SoraBaseURL,
			},
		}, logger)
		if defaultVideo == "" {
			defaultVideo = "sora"
		}
	}
	if cfg.KlingAPIKey != "" {
		videoProviders["kling"] = video.NewKlingProvider(video.KlingConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.KlingAPIKey,
				BaseURL: cfg.KlingBaseURL,
			},
		}, logger)
		if defaultVideo == "" {
			defaultVideo = "kling"
		}
	}
	if cfg.LumaAPIKey != "" {
		videoProviders["luma"] = video.NewLumaProvider(video.LumaConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.LumaAPIKey,
				BaseURL: cfg.LumaBaseURL,
			},
		}, logger)
		if defaultVideo == "" {
			defaultVideo = "luma"
		}
	}
	if cfg.MiniMaxAPIKey != "" {
		videoProviders["minimax-video"] = video.NewMiniMaxVideoProvider(video.MiniMaxVideoConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.MiniMaxAPIKey,
				BaseURL: cfg.MiniMaxBaseURL,
			},
		}, logger)
		if defaultVideo == "" {
			defaultVideo = "minimax-video"
		}
	}

	if defaultImage != "" {
		if _, ok := imageProviders[defaultImage]; !ok {
			logger.Warn("configured default multimodal image provider is unavailable, fallback to auto selection",
				zap.String("provider", defaultImage))
			defaultImage = ""
		}
	}
	if defaultVideo != "" {
		if _, ok := videoProviders[defaultVideo]; !ok {
			logger.Warn("configured default multimodal video provider is unavailable, fallback to auto selection",
				zap.String("provider", defaultVideo))
			defaultVideo = ""
		}
	}

	return NewMultimodalHandlerWithProviders(
		cfg.ChatProvider,
		imageProviders,
		videoProviders,
		defaultImage,
		defaultVideo,
		cfg.Pipeline,
		cfg.ReferenceMaxSize,
		cfg.ReferenceTTL,
		cfg.ReferenceStore,
		logger,
	)
}

func NewMultimodalHandlerWithProviders(
	chatProvider llm.Provider,
	imageProviders map[string]image.Provider,
	videoProviders map[string]video.Provider,
	defaultImage string,
	defaultVideo string,
	pipeline PromptPipeline,
	referenceMaxSize int64,
	referenceTTL time.Duration,
	referenceStore ReferenceStore,
	logger *zap.Logger,
) *MultimodalHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	if pipeline == nil {
		pipeline = &DefaultPromptPipeline{}
	}
	if referenceMaxSize <= 0 {
		referenceMaxSize = defaultReferenceBytes
	}
	if referenceTTL <= 0 {
		referenceTTL = defaultReferenceTTL
	}
	if referenceStore == nil {
		referenceStore = NewMemoryReferenceStore()
	}

	router := multimodal.NewRouter()
	imageNames := make([]string, 0, len(imageProviders))
	videoNames := make([]string, 0, len(videoProviders))

	for name := range imageProviders {
		imageNames = append(imageNames, name)
	}
	sort.Strings(imageNames)
	if defaultImage == "" && len(imageNames) > 0 {
		defaultImage = imageNames[0]
	}
	for _, name := range imageNames {
		router.RegisterImage(name, imageProviders[name], name == defaultImage)
	}

	for name := range videoProviders {
		videoNames = append(videoNames, name)
	}
	sort.Strings(videoNames)
	if defaultVideo == "" && len(videoNames) > 0 {
		defaultVideo = videoNames[0]
	}
	for _, name := range videoNames {
		router.RegisterVideo(name, videoProviders[name], name == defaultVideo)
	}

	return &MultimodalHandler{
		logger:               logger.With(zap.String("handler", "multimodal")),
		chatProvider:         chatProvider,
		router:               router,
		pipeline:             pipeline,
		defaultImageProvider: defaultImage,
		defaultVideoProvider: defaultVideo,
		imageProviders:       imageNames,
		videoProviders:       videoNames,
		promptEnhancer:       agent.NewPromptEnhancer(*agent.DefaultPromptEnhancerConfig()),
		promptOptimizer:      agent.NewPromptOptimizer(),
		referenceMaxSize:     referenceMaxSize,
		referenceTTL:         referenceTTL,
		referenceStore:       referenceStore,
	}
}

func (h *MultimodalHandler) HandleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	WriteSuccess(w, map[string]any{
		"image_providers":        h.imageProviders,
		"video_providers":        h.videoProviders,
		"default_image_provider": h.defaultImageProvider,
		"default_video_provider": h.defaultVideoProvider,
		"features": map[string]bool{
			"reference_upload": len(h.imageProviders) > 0 || len(h.videoProviders) > 0,
			"text_to_image":    len(h.imageProviders) > 0,
			"image_to_image":   len(h.imageProviders) > 0,
			"text_to_video":    len(h.videoProviders) > 0,
			"image_to_video":   len(h.videoProviders) > 0,
			"advanced_prompt":  true,
			"chat":             h.chatProvider != nil,
			"agent_mode":       h.chatProvider != nil,
			"plan_generation":  h.chatProvider != nil,
		},
	})
}

func (h *MultimodalHandler) HandleUploadReference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if err := r.ParseMultipartForm(h.referenceMaxSize + (1 << 20)); err != nil {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid multipart form", h.logger)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "file is required", h.logger)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, h.referenceMaxSize+1))
	if err != nil {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "failed to read uploaded file", h.logger)
		return
	}
	if len(data) == 0 {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "uploaded file is empty", h.logger)
		return
	}
	if int64(len(data)) > h.referenceMaxSize {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, fmt.Sprintf("file too large (max %d bytes)", h.referenceMaxSize), h.logger)
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	if mediaType, _, parseErr := mime.ParseMediaType(mimeType); parseErr == nil {
		mimeType = mediaType
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "only image files are supported", h.logger)
		return
	}

	refID := fmt.Sprintf("ref_%d", time.Now().UnixNano())
	ref := &referenceAsset{
		ID:        refID,
		FileName:  header.Filename,
		MimeType:  mimeType,
		Size:      len(data),
		CreatedAt: time.Now(),
		Data:      data,
	}

	h.cleanupReferences(time.Now())
	if err := h.referenceStore.Save(ref); err != nil {
		h.logger.Error("failed to persist multimodal reference", zap.Error(err))
		WriteErrorMessage(w, http.StatusServiceUnavailable, types.ErrServiceUnavailable, "failed to persist reference", h.logger)
		return
	}

	WriteSuccess(w, map[string]any{
		"reference_id": refID,
		"file_name":    ref.FileName,
		"mime_type":    ref.MimeType,
		"size":         ref.Size,
		"created_at":   ref.CreatedAt,
	})
}

type multimodalImageRequest struct {
	Prompt            string   `json:"prompt"`
	NegativePrompt    string   `json:"negative_prompt,omitempty"`
	Model             string   `json:"model,omitempty"`
	Provider          string   `json:"provider,omitempty"`
	N                 int      `json:"n,omitempty"`
	Size              string   `json:"size,omitempty"`
	Quality           string   `json:"quality,omitempty"`
	Style             string   `json:"style,omitempty"`
	ResponseFormat    string   `json:"response_format,omitempty"`
	Advanced          bool     `json:"advanced,omitempty"`
	StyleTokens       []string `json:"style_tokens,omitempty"`
	QualityTokens     []string `json:"quality_tokens,omitempty"`
	ReferenceID       string   `json:"reference_id,omitempty"`
	ReferenceImageURL string   `json:"reference_image_url,omitempty"`
}

func (h *MultimodalHandler) HandleImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req multimodalImageRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "prompt is required", h.logger)
		return
	}
	if req.N < 0 {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "n must be non-negative", h.logger)
		return
	}
	if len(h.imageProviders) == 0 {
		WriteErrorMessage(w, http.StatusServiceUnavailable, types.ErrServiceUnavailable, "no image provider configured", h.logger)
		return
	}

	providerName, err := h.resolveImageProvider(req.Provider)
	if err != nil {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, err.Error(), h.logger)
		return
	}

	negative := strings.TrimSpace(req.NegativePrompt)
	if negative == "" {
		negative = defaultNegativeText
	}

	promptResult, err := h.pipeline.Build(r.Context(), PromptContext{
		Modality:       "image",
		BasePrompt:     req.Prompt,
		Advanced:       req.Advanced,
		StyleTokens:    req.StyleTokens,
		QualityTokens:  req.QualityTokens,
		NegativePrompt: negative,
	})
	if err != nil {
		h.writeProviderError(w, err)
		return
	}

	imageProvider, err := h.router.Image(providerName)
	if err != nil {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, err.Error(), h.logger)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	var mode string
	var resp *image.GenerateResponse

	if req.ReferenceID != "" || strings.TrimSpace(req.ReferenceImageURL) != "" {
		mode = "image-to-image"
		var data []byte
		if req.ReferenceID != "" {
			var ok bool
			data, _, ok = h.getReference(req.ReferenceID)
			if !ok {
				WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "reference_id not found or expired", h.logger)
				return
			}
		} else {
			validatedURL, urlErr := validatePublicReferenceImageURL(ctx, req.ReferenceImageURL)
			if urlErr != nil {
				WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, urlErr.Error(), h.logger)
				return
			}
			var dlErr error
			data, _, dlErr = h.downloadReferenceImage(ctx, validatedURL)
			if dlErr != nil {
				WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, dlErr.Error(), h.logger)
				return
			}
		}
		resp, err = imageProvider.Edit(ctx, &image.EditRequest{
			Image:          bytes.NewReader(data),
			Prompt:         promptResult.Prompt,
			Model:          req.Model,
			N:              req.N,
			Size:           req.Size,
			ResponseFormat: req.ResponseFormat,
		})
	} else {
		mode = "text-to-image"
		resp, err = h.router.GenerateImage(ctx, &image.GenerateRequest{
			Prompt:         promptResult.Prompt,
			NegativePrompt: promptResult.NegativePrompt,
			Model:          req.Model,
			N:              req.N,
			Size:           req.Size,
			Quality:        req.Quality,
			Style:          req.Style,
			ResponseFormat: req.ResponseFormat,
		}, providerName)
	}
	if err != nil {
		h.writeProviderError(w, err)
		return
	}

	WriteSuccess(w, map[string]any{
		"mode":             mode,
		"provider":         providerName,
		"effective_prompt": promptResult.Prompt,
		"negative_prompt":  promptResult.NegativePrompt,
		"response":         resp,
	})
}

type multimodalVideoRequest struct {
	Prompt            string   `json:"prompt"`
	NegativePrompt    string   `json:"negative_prompt,omitempty"`
	Model             string   `json:"model,omitempty"`
	Provider          string   `json:"provider,omitempty"`
	Duration          float64  `json:"duration,omitempty"`
	AspectRatio       string   `json:"aspect_ratio,omitempty"`
	Resolution        string   `json:"resolution,omitempty"`
	FPS               int      `json:"fps,omitempty"`
	Seed              int64    `json:"seed,omitempty"`
	ResponseFormat    string   `json:"response_format,omitempty"`
	Advanced          bool     `json:"advanced,omitempty"`
	StyleTokens       []string `json:"style_tokens,omitempty"`
	Camera            string   `json:"camera,omitempty"`
	Mood              string   `json:"mood,omitempty"`
	ReferenceID       string   `json:"reference_id,omitempty"`
	ReferenceImageURL string   `json:"reference_image_url,omitempty"`
}

func (h *MultimodalHandler) HandleVideo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req multimodalVideoRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "prompt is required", h.logger)
		return
	}
	if len(h.videoProviders) == 0 {
		WriteErrorMessage(w, http.StatusServiceUnavailable, types.ErrServiceUnavailable, "no video provider configured", h.logger)
		return
	}

	providerName, err := h.resolveVideoProvider(req.Provider)
	if err != nil {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, err.Error(), h.logger)
		return
	}

	promptResult, err := h.pipeline.Build(r.Context(), PromptContext{
		Modality:       "video",
		BasePrompt:     req.Prompt,
		Advanced:       req.Advanced,
		StyleTokens:    req.StyleTokens,
		NegativePrompt: req.NegativePrompt,
		Camera:         req.Camera,
		Mood:           req.Mood,
	})
	if err != nil {
		h.writeProviderError(w, err)
		return
	}

	genReq := &video.GenerateRequest{
		Prompt:         promptResult.Prompt,
		NegativePrompt: promptResult.NegativePrompt,
		Model:          req.Model,
		Duration:       req.Duration,
		AspectRatio:    req.AspectRatio,
		Resolution:     req.Resolution,
		FPS:            req.FPS,
		Seed:           req.Seed,
		ResponseFormat: req.ResponseFormat,
	}

	mode := "text-to-video"
	if req.ReferenceID != "" || strings.TrimSpace(req.ReferenceImageURL) != "" {
		mode = "image-to-video"
		if req.ReferenceID != "" {
			data, mimeType, ok := h.getReference(req.ReferenceID)
			if !ok {
				WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "reference_id not found or expired", h.logger)
				return
			}
			h.attachReferenceImage(providerName, genReq, data, mimeType)
		} else {
			validatedURL, urlErr := validatePublicReferenceImageURL(r.Context(), req.ReferenceImageURL)
			if urlErr != nil {
				WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, urlErr.Error(), h.logger)
				return
			}
			genReq.ImageURL = validatedURL
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Minute)
	defer cancel()

	resp, err := h.router.GenerateVideo(ctx, genReq, providerName)
	if err != nil {
		h.writeProviderError(w, err)
		return
	}

	WriteSuccess(w, map[string]any{
		"mode":             mode,
		"provider":         providerName,
		"effective_prompt": promptResult.Prompt,
		"response":         resp,
	})
}

type multimodalPlanRequest struct {
	Prompt    string `json:"prompt"`
	ShotCount int    `json:"shot_count,omitempty"`
	Advanced  bool   `json:"advanced,omitempty"`
}

type visualPlan struct {
	Goal  string       `json:"goal"`
	Shots []visualShot `json:"shots"`
}

type visualShot struct {
	ID          int    `json:"id"`
	Purpose     string `json:"purpose"`
	Visual      string `json:"visual"`
	Action      string `json:"action"`
	Camera      string `json:"camera"`
	DurationSec int    `json:"duration_sec"`
}

func (h *MultimodalHandler) HandlePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}
	if h.chatProvider == nil {
		WriteErrorMessage(w, http.StatusServiceUnavailable, types.ErrServiceUnavailable, "chat provider is not configured", h.logger)
		return
	}

	var req multimodalPlanRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "prompt is required", h.logger)
		return
	}
	if req.ShotCount <= 0 {
		req.ShotCount = 6
	}
	if req.ShotCount > 12 {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "shot_count must be <= 12", h.logger)
		return
	}

	prompt := fmt.Sprintf(`Create a visual production plan with %d shots.
User intent: %s

Requirements:
1. Output concise, production-ready shots.
2. Keep character/style continuity across shots.
3. Each shot needs purpose, visual, action, camera and duration_sec.
4. duration_sec should be 1-8.`, req.ShotCount, strings.TrimSpace(req.Prompt))
	if req.Advanced {
		prompt = h.promptEnhancer.EnhanceUserPrompt(prompt, "")
	}
	prompt = h.promptOptimizer.OptimizePrompt(prompt)

	so, err := structured.NewStructuredOutput[visualPlan](h.chatProvider)
	if err != nil {
		h.writeProviderError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	out, err := so.Generate(ctx, prompt)
	if err != nil {
		h.writeProviderError(w, err)
		return
	}
	if out == nil {
		WriteErrorMessage(w, http.StatusBadGateway, types.ErrUpstreamError, "empty plan result", h.logger)
		return
	}
	for i := range out.Shots {
		if out.Shots[i].ID <= 0 {
			out.Shots[i].ID = i + 1
		}
		if out.Shots[i].DurationSec <= 0 {
			out.Shots[i].DurationSec = 3
		}
	}

	WriteSuccess(w, map[string]any{"plan": out})
}

type multimodalChatRequest struct {
	Model        string        `json:"model,omitempty"`
	Message      string        `json:"message,omitempty"`
	Messages     []api.Message `json:"messages,omitempty"`
	SystemPrompt string        `json:"system_prompt,omitempty"`
	Temperature  float32       `json:"temperature,omitempty"`
	Advanced     bool          `json:"advanced,omitempty"`
	AgentMode    bool          `json:"agent_mode,omitempty"`
}

func (h *MultimodalHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}
	if h.chatProvider == nil {
		WriteErrorMessage(w, http.StatusServiceUnavailable, types.ErrServiceUnavailable, "chat provider is not configured", h.logger)
		return
	}

	var req multimodalChatRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	messages := req.Messages
	if len(messages) == 0 {
		if strings.TrimSpace(req.Message) == "" {
			WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "message or messages is required", h.logger)
			return
		}
		messages = []api.Message{{Role: "user", Content: strings.TrimSpace(req.Message)}}
	}

	llmMessages := convertAPIMessages(messages)
	if req.Advanced {
		sys := strings.TrimSpace(req.SystemPrompt)
		if sys == "" {
			sys = "You are a multimodal agent framework assistant. Produce clear, executable and structured outputs."
		}
		llmMessages = append([]types.Message{{Role: types.RoleSystem, Content: sys}}, llmMessages...)
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = defaultChatModel
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	if !req.AgentMode {
		resp, err := h.chatProvider.Completion(ctx, &llm.ChatRequest{
			Model:       model,
			Messages:    llmMessages,
			Temperature: req.Temperature,
		})
		if err != nil {
			h.writeProviderError(w, err)
			return
		}
		WriteSuccess(w, map[string]any{
			"mode":     "single",
			"response": resp,
		})
		return
	}

	userText := latestUserText(messages)
	planResp, err := h.chatProvider.Completion(ctx, &llm.ChatRequest{
		Model: model,
		Messages: []types.Message{
			{
				Role:    types.RoleSystem,
				Content: "You are an orchestration planner. Return 3-6 concise action steps.",
			},
			{
				Role:    types.RoleUser,
				Content: userText,
			},
		},
		Temperature: 0.2,
	})
	if err != nil {
		h.writeProviderError(w, err)
		return
	}

	planText := firstChoice(planResp)
	finalMessages := append([]types.Message{
		{
			Role:    types.RoleSystem,
			Content: "You are an executor agent. Execute the provided plan and produce final answer.",
		},
	}, llmMessages...)
	finalMessages = append(finalMessages, types.Message{
		Role:    types.RoleUser,
		Content: "Planner output:\n" + planText,
	})

	finalResp, err := h.chatProvider.Completion(ctx, &llm.ChatRequest{
		Model:       model,
		Messages:    finalMessages,
		Temperature: req.Temperature,
	})
	if err != nil {
		h.writeProviderError(w, err)
		return
	}

	WriteSuccess(w, map[string]any{
		"mode":           "agent",
		"planner_output": planText,
		"final_response": finalResp,
		"final_text":     firstChoice(finalResp),
	})
}

func (h *MultimodalHandler) resolveImageProvider(provider string) (string, error) {
	name := strings.TrimSpace(provider)
	if name == "" {
		name = h.defaultImageProvider
	}
	if name == "" {
		return "", fmt.Errorf("no default image provider available")
	}
	if _, err := h.router.Image(name); err != nil {
		return "", fmt.Errorf("image provider %q not found", name)
	}
	return name, nil
}

func (h *MultimodalHandler) resolveVideoProvider(provider string) (string, error) {
	name := strings.TrimSpace(provider)
	if name == "" {
		name = h.defaultVideoProvider
	}
	if name == "" {
		return "", fmt.Errorf("no default video provider available")
	}
	if _, err := h.router.Video(name); err != nil {
		return "", fmt.Errorf("video provider %q not found", name)
	}
	return name, nil
}

func (h *MultimodalHandler) writeProviderError(w http.ResponseWriter, err error) {
	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)

	switch {
	case strings.Contains(lower, "invalid"),
		strings.Contains(lower, "required"),
		strings.Contains(lower, "unsupported"),
		strings.Contains(lower, "not support"):
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, msg, h.logger)
	default:
		WriteErrorMessage(w, http.StatusBadGateway, types.ErrUpstreamError, msg, h.logger)
	}
}

func (h *MultimodalHandler) getReference(id string) ([]byte, string, bool) {
	ref, ok := h.referenceStore.Get(id)
	if !ok || ref == nil {
		return nil, "", false
	}
	if time.Since(ref.CreatedAt) > h.referenceTTL {
		h.referenceStore.Delete(id)
		return nil, "", false
	}
	return append([]byte(nil), ref.Data...), ref.MimeType, true
}

func (h *MultimodalHandler) cleanupReferences(now time.Time) {
	h.referenceStore.Cleanup(now.Add(-h.referenceTTL))
}

func buildBlockedReferenceIPPrefixes() []netip.Prefix {
	raw := []string{
		"0.0.0.0/8",       // "this network"
		"100.64.0.0/10",   // carrier-grade NAT
		"192.0.0.0/24",    // IETF protocol assignments
		"192.0.2.0/24",    // TEST-NET-1
		"198.18.0.0/15",   // benchmark testing
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"224.0.0.0/4",     // multicast
		"240.0.0.0/4",     // reserved
		"2001:db8::/32",   // documentation
	}
	prefixes := make([]netip.Prefix, 0, len(raw))
	for _, cidr := range raw {
		p, err := netip.ParsePrefix(cidr)
		if err != nil {
			continue
		}
		prefixes = append(prefixes, p)
	}
	return prefixes
}

func validatePublicReferenceImageURL(ctx context.Context, rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if !ValidateURL(trimmed) {
		return "", fmt.Errorf("reference_image_url must be a valid http/https URL")
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("reference_image_url must be a valid http/https URL")
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("reference_image_url must include a valid host")
	}

	resolveCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := validatePublicHost(resolveCtx, host); err != nil {
		return "", err
	}

	return u.String(), nil
}

func validatePublicHost(ctx context.Context, host string) error {
	_, err := resolvePublicHostIPs(ctx, host)
	return err
}

func resolvePublicHostIPs(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if isDisallowedReferenceIP(ip) {
			return nil, fmt.Errorf("reference_image_url must resolve to a public internet address")
		}
		return []net.IP{ip}, nil
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve reference_image_url host")
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("failed to resolve reference_image_url host")
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if isDisallowedReferenceIP(addr.IP) {
			return nil, fmt.Errorf("reference_image_url must resolve to a public internet address")
		}
		ips = append(ips, addr.IP)
	}
	return ips, nil
}

func isDisallowedReferenceIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	if !addr.IsValid() ||
		addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return true
	}
	for _, p := range blockedReferenceIPPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func (h *MultimodalHandler) downloadReferenceImage(ctx context.Context, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create image download request: %w", err)
	}
	client := newReferenceDownloadHTTPClient(20 * time.Second)
	client.CheckRedirect = func(redirReq *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects while downloading reference image")
		}
		_, validateErr := validatePublicReferenceImageURL(ctx, redirReq.URL.String())
		return validateErr
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download reference image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("failed to download reference image: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, h.referenceMaxSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("failed to read reference image: %w", err)
	}
	if int64(len(data)) > h.referenceMaxSize {
		return nil, "", fmt.Errorf("reference image is too large (max %d bytes)", h.referenceMaxSize)
	}

	mimeType := resp.Header.Get("Content-Type")
	if mediaType, _, parseErr := mime.ParseMediaType(mimeType); parseErr == nil {
		mimeType = mediaType
	}
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return nil, "", fmt.Errorf("reference URL does not point to an image")
	}
	return data, mimeType, nil
}

func newReferenceDownloadHTTPClient(timeout time.Duration) *http.Client {
	base := tlsutil.SecureHTTPClient(timeout)
	transport, ok := base.Transport.(*http.Transport)
	if !ok || transport == nil {
		return base
	}

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	cloned := transport.Clone()
	cloned.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid reference image host: %w", err)
		}
		host = strings.TrimSpace(strings.Trim(host, "[]"))
		ips, err := resolvePublicHostIPs(ctx, host)
		if err != nil {
			return nil, err
		}

		var lastErr error
		for _, ip := range ips {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr != nil {
			return nil, fmt.Errorf("failed to connect to reference image host: %w", lastErr)
		}
		return nil, fmt.Errorf("failed to connect to reference image host")
	}

	clientCopy := *base
	clientCopy.Transport = cloned
	return &clientCopy
}

func (h *MultimodalHandler) attachReferenceImage(providerName string, req *video.GenerateRequest, data []byte, mimeType string) {
	b64 := base64.StdEncoding.EncodeToString(data)
	if providerName == "veo" {
		req.Image = b64
		return
	}
	if mimeType == "" {
		mimeType = "image/png"
	}
	req.ImageURL = fmt.Sprintf("data:%s;base64,%s", mimeType, b64)
}

func convertAPIMessages(messages []api.Message) []types.Message {
	out := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		out = append(out, types.Message{
			Role:       types.Role(role),
			Content:    msg.Content,
			Name:       msg.Name,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
			Metadata:   msg.Metadata,
			Timestamp:  msg.Timestamp,
		})
	}
	return out
}

func latestUserText(messages []api.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && strings.TrimSpace(messages[i].Content) != "" {
			return messages[i].Content
		}
	}
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1].Content
}

func firstChoice(resp *llm.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content)
}

func filterEmptyStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}
