package handlers

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/structured"
	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/capabilities"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/multimodal"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	"github.com/BaSui01/agentflow/pkg/storage"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

const (
	defaultReferenceBytes = 8 << 20 // 8MB
	defaultReferenceTTL   = 2 * time.Hour
	defaultChatModelFallback = "gpt-4o-mini"
	defaultNegativeText   = "blurry, low quality, watermark, text, logo, signature, bad anatomy, deformed, mutated"
)

var (
	validImageSizes   = []string{"256x256", "512x512", "1024x1024", "1024x768", "768x1024", "1536x1024", "1024x1536", "1536x1536", "1792x1024", "1024x1792"}
	validImageQualities = []string{"standard", "hd"}
	validImageStyles    = []string{"vivid", "natural"}
)

type MultimodalHandlerConfig struct {
	ChatProvider         llm.Provider
	PolicyManager        *llmpolicy.Manager
	Ledger               observability.Ledger
	OpenAIAPIKey         string
	OpenAIBaseURL        string
	GoogleAPIKey         string
	GoogleBaseURL        string
	FluxAPIKey           string
	FluxBaseURL          string
	StabilityAPIKey       string
	StabilityBaseURL      string
	IdeogramAPIKey        string
	IdeogramBaseURL       string
	TongyiAPIKey          string
	TongyiBaseURL         string
	ZhipuAPIKey           string
	ZhipuBaseURL          string
	BaiduAPIKey           string
	BaiduSecretKey        string
	BaiduBaseURL          string
	DoubaoAPIKey          string
	DoubaoBaseURL         string
	TencentSecretId       string
	TencentSecretKey      string
	TencentBaseURL        string
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
	SeedanceAPIKey       string
	SeedanceBaseURL      string
	DefaultImageProvider string
	DefaultVideoProvider string
	ReferenceMaxSize     int64
	ReferenceTTL         time.Duration
	ReferenceStore       storage.ReferenceStore
	Pipeline             multimodal.PromptPipeline
	DefaultChatModel     string
}

type MultimodalHandler struct {
	logger             *zap.Logger
	router             *multimodal.Router
	gateway            llmcore.Gateway
	structuredProvider llm.Provider
	pipeline           multimodal.PromptPipeline
	service            multimodalService

	defaultImageProvider string
	defaultVideoProvider string
	imageProviders       []string
	videoProviders       []string

	promptEnhancer  *agent.PromptEnhancer
	promptOptimizer *agent.PromptOptimizer

	referenceMaxSize int64
	referenceTTL     time.Duration
	referenceStore   storage.ReferenceStore

	defaultChatModel string
}

func NewMultimodalHandlerFromConfig(cfg MultimodalHandlerConfig, logger *zap.Logger) *MultimodalHandler {
	if logger == nil {
		logger = zap.NewNop()
	}

	providerSet := multimodal.BuildProvidersFromConfig(multimodal.ProviderBuilderConfig{
		OpenAIAPIKey:         cfg.OpenAIAPIKey,
		OpenAIBaseURL:        cfg.OpenAIBaseURL,
		GoogleAPIKey:         cfg.GoogleAPIKey,
		GoogleBaseURL:        cfg.GoogleBaseURL,
		FluxAPIKey:           cfg.FluxAPIKey,
		FluxBaseURL:          cfg.FluxBaseURL,
		StabilityAPIKey:      cfg.StabilityAPIKey,
		StabilityBaseURL:     cfg.StabilityBaseURL,
		IdeogramAPIKey:       cfg.IdeogramAPIKey,
		IdeogramBaseURL:      cfg.IdeogramBaseURL,
		TongyiAPIKey:         cfg.TongyiAPIKey,
		TongyiBaseURL:        cfg.TongyiBaseURL,
		ZhipuAPIKey:         cfg.ZhipuAPIKey,
		ZhipuBaseURL:        cfg.ZhipuBaseURL,
		BaiduAPIKey:         cfg.BaiduAPIKey,
		BaiduSecretKey:      cfg.BaiduSecretKey,
		BaiduBaseURL:        cfg.BaiduBaseURL,
		DoubaoAPIKey:        cfg.DoubaoAPIKey,
		DoubaoBaseURL:       cfg.DoubaoBaseURL,
		TencentSecretId:     cfg.TencentSecretId,
		TencentSecretKey:    cfg.TencentSecretKey,
		TencentBaseURL:      cfg.TencentBaseURL,
		RunwayAPIKey:        cfg.RunwayAPIKey,
		RunwayBaseURL:        cfg.RunwayBaseURL,
		VeoAPIKey:            cfg.VeoAPIKey,
		VeoBaseURL:           cfg.VeoBaseURL,
		SoraAPIKey:           cfg.SoraAPIKey,
		SoraBaseURL:          cfg.SoraBaseURL,
		KlingAPIKey:          cfg.KlingAPIKey,
		KlingBaseURL:         cfg.KlingBaseURL,
		LumaAPIKey:           cfg.LumaAPIKey,
		LumaBaseURL:          cfg.LumaBaseURL,
		MiniMaxAPIKey:        cfg.MiniMaxAPIKey,
		MiniMaxBaseURL:       cfg.MiniMaxBaseURL,
		SeedanceAPIKey:       cfg.SeedanceAPIKey,
		SeedanceBaseURL:      cfg.SeedanceBaseURL,
		DefaultImageProvider: cfg.DefaultImageProvider,
		DefaultVideoProvider: cfg.DefaultVideoProvider,
	}, logger)

	return NewMultimodalHandlerWithProviders(
		cfg.ChatProvider,
		cfg.PolicyManager,
		cfg.Ledger,
		providerSet.ImageProviders,
		providerSet.VideoProviders,
		providerSet.DefaultImage,
		providerSet.DefaultVideo,
		cfg.Pipeline,
		cfg.ReferenceMaxSize,
		cfg.ReferenceTTL,
		cfg.ReferenceStore,
		cfg.DefaultChatModel,
		logger,
	)
}

// NewMultimodalHandlerWithProviders 使用已构建的 image/video providers 创建 Handler。
// referenceStore 为 nil 时使用内存实现，仅建议在测试或开发环境使用；生产环境应由组合根注入 Redis 等持久化实现。
func NewMultimodalHandlerWithProviders(
	chatProvider llm.Provider,
	policyManager *llmpolicy.Manager,
	ledger observability.Ledger,
	imageProviders map[string]image.Provider,
	videoProviders map[string]video.Provider,
	defaultImage string,
	defaultVideo string,
	pipeline multimodal.PromptPipeline,
	referenceMaxSize int64,
	referenceTTL time.Duration,
	referenceStore storage.ReferenceStore,
	defaultChatModel string,
	logger *zap.Logger,
) *MultimodalHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	if pipeline == nil {
		pipeline = &multimodal.DefaultPromptPipeline{}
	}
	if referenceMaxSize <= 0 {
		referenceMaxSize = defaultReferenceBytes
	}
	if referenceTTL <= 0 {
		referenceTTL = defaultReferenceTTL
	}
	if referenceStore == nil {
		referenceStore = storage.NewMemoryReferenceStore()
	}
	if defaultChatModel == "" {
		defaultChatModel = defaultChatModelFallback
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

	gw := llmgateway.New(llmgateway.Config{
		ChatProvider:  chatProvider,
		Capabilities:  capabilities.NewEntry(router),
		PolicyManager: policyManager,
		Ledger:        ledger,
		Logger:        logger,
	})
	var structuredProvider llm.Provider
	if chatProvider != nil {
		structuredProvider = llmgateway.NewChatProviderAdapter(gw, chatProvider)
	}

	handler := &MultimodalHandler{
		logger:               logger.With(zap.String("handler", "multimodal")),
		router:               router,
		gateway:              gw,
		structuredProvider:   structuredProvider,
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
		defaultChatModel:     defaultChatModel,
	}
	handler.service = newDefaultMultimodalService(
		handler.gateway,
		handler.pipeline,
		handler.resolveImageProvider,
		handler.resolveVideoProvider,
		handler.getReference,
		handler.referenceMaxSize,
	)
	return handler
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
			"image_stream":     len(h.imageProviders) > 0,
			"text_to_video":    len(h.videoProviders) > 0,
			"image_to_video":   len(h.videoProviders) > 0,
			"advanced_prompt":  true,
			"chat":             h.structuredProvider != nil,
			"agent_mode":       h.structuredProvider != nil,
			"plan_generation":  h.structuredProvider != nil,
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
	ref := &storage.ReferenceAsset{
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
	Stream            bool     `json:"stream,omitempty"`
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
	if !ValidateNonNegative(float64(req.N)) {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "n must be non-negative", h.logger)
		return
	}
	// V-015: Size/Quality/Style enum validation
	if req.Size != "" && !ValidateEnum(req.Size, validImageSizes) {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "size must be one of: 256x256, 512x512, 1024x1024, 1024x768, 768x1024, 1536x1024, 1024x1536, 1536x1536, 1792x1024, 1024x1792", h.logger)
		return
	}
	if req.Quality != "" && !ValidateEnum(req.Quality, validImageQualities) {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "quality must be one of: standard, hd", h.logger)
		return
	}
	if req.Style != "" && !ValidateEnum(req.Style, validImageStyles) {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "style must be one of: vivid, natural", h.logger)
		return
	}
	if len(h.imageProviders) == 0 {
		WriteErrorMessage(w, http.StatusServiceUnavailable, types.ErrServiceUnavailable, "no image provider configured", h.logger)
		return
	}

	if req.Stream {
		h.handleImageStream(w, r, req)
		return
	}

	result, err := h.service.GenerateImage(r.Context(), req)
	if err != nil {
		WriteErrorMessage(w, toHTTPStatus(err), errorCodeFrom(err, types.ErrUpstreamError), strings.TrimSpace(err.Error()), h.logger)
		return
	}

	WriteSuccess(w, map[string]any{
		"mode":             result.Mode,
		"provider":         result.Provider,
		"effective_prompt": result.EffectivePrompt,
		"negative_prompt":  result.NegativePrompt,
		"response":         result.Response,
	})
}

// handleImageStream 以 SSE 流式返回图片生成结果，事件命名与 payload 对齐 OpenAI 官方规范：
// image_generation.started -> [image_generation.thinking]* -> image_generation.completed (每张) -> image_generation.done -> [DONE]
// 参考: https://platform.openai.com/docs/api-reference/images-streaming
//
// 对于实现了 image.StreamingProvider 的 provider（如 Gemini），会走原生 SSE 流式路径，
// 在图像生成过程中实时推送 image_generation.thinking 文字事件；
// 其余 provider 走同步生成路径（阻塞直到图像就绪后一次性推送）。
func (h *MultimodalHandler) handleImageStream(w http.ResponseWriter, r *http.Request, req multimodalImageRequest) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// 尝试解析 provider，检查是否支持原生流式
	providerName, resolveErr := h.resolveImageProvider(req.Provider)
	if resolveErr == nil {
		if p, routerErr := h.router.Image(providerName); routerErr == nil {
			if sp, ok := p.(image.StreamingProvider); ok {
				h.handleImageNativeStream(w, r, req, providerName, sp)
				return
			}
		}
	}

	// 回退：同步生成后包装为 SSE
	result, err := h.service.GenerateImage(r.Context(), req)
	if err != nil {
		code := errorCodeFrom(err, types.ErrUpstreamError)
		_ = writeSSEEventJSON(w, "error", map[string]any{
			"type":    "error",
			"code":    code,
			"message": strings.TrimSpace(err.Error()),
		})
		_ = writeSSE(w, []byte("data: [DONE]\n\n"))
		return
	}
	h.flushImageResult(w, req, result)
}

// handleImageNativeStream 使用 image.StreamingProvider 原生 SSE 流式生成，
// 在图像生成过程中实时推送文字 token（image_generation.thinking 事件）。
func (h *MultimodalHandler) handleImageNativeStream(
	w http.ResponseWriter,
	r *http.Request,
	req multimodalImageRequest,
	providerName string,
	sp image.StreamingProvider,
) {
	flusher, canFlush := w.(http.Flusher)

	genReq := &image.GenerateRequest{
		Prompt:         req.Prompt,
		NegativePrompt: req.NegativePrompt,
		Model:          req.Model,
		Size:           req.Size,
		Quality:        req.Quality,
		Style:          req.Style,
		ResponseFormat: req.ResponseFormat,
	}

	_ = writeSSEEventJSON(w, "image_generation.started", map[string]any{
		"type":     "image_generation.started",
		"provider": providerName,
		"mode":     "text_to_image",
	})
	if canFlush {
		flusher.Flush()
	}

	var (
		collectedImages []image.ImageData
		imageIndex      int
	)

	streamErr := sp.GenerateStream(r.Context(), genReq, func(chunk image.StreamChunk) {
		switch {
		case chunk.Err != nil:
			code := errorCodeFrom(chunk.Err, types.ErrUpstreamError)
			_ = writeSSEEventJSON(w, "error", map[string]any{
				"type":    "error",
				"code":    code,
				"message": strings.TrimSpace(chunk.Err.Error()),
			})
			if canFlush {
				flusher.Flush()
			}
		case chunk.Text != "":
			_ = writeSSEEventJSON(w, "image_generation.thinking", map[string]any{
				"type": "image_generation.thinking",
				"text": chunk.Text,
			})
			if canFlush {
				flusher.Flush()
			}
		case chunk.Image != nil:
			collectedImages = append(collectedImages, *chunk.Image)
			payload := map[string]any{
				"type":          "image_generation.completed",
				"index":         imageIndex,
				"output_format": "png",
				"quality":       req.Quality,
				"size":          req.Size,
			}
			if chunk.Image.URL != "" {
				payload["url"] = chunk.Image.URL
			}
			if chunk.Image.B64JSON != "" {
				payload["b64_json"] = chunk.Image.B64JSON
			}
			_ = writeSSEEventJSON(w, "image_generation.completed", payload)
			if canFlush {
				flusher.Flush()
			}
			imageIndex++
		case chunk.Done:
			_ = writeSSEEventJSON(w, "image_generation.done", map[string]any{
				"type":     "image_generation.done",
				"provider": providerName,
				"usage": image.ImageUsage{
					ImagesGenerated: len(collectedImages),
				},
			})
			if canFlush {
				flusher.Flush()
			}
		}
	})

	if streamErr != nil && r.Context().Err() == nil {
		h.logger.Warn("gemini stream error", zap.Error(streamErr))
	}
	_ = writeSSE(w, []byte("data: [DONE]\n\n"))
	if canFlush {
		flusher.Flush()
	}
}

// flushImageResult 将同步生成的 result 包装成 SSE 事件推送给客户端.
func (h *MultimodalHandler) flushImageResult(w http.ResponseWriter, req multimodalImageRequest, result *multimodalImageResult) {
	_ = writeSSEEventJSON(w, "image_generation.started", map[string]any{
		"type":             "image_generation.started",
		"mode":             result.Mode,
		"provider":         result.Provider,
		"effective_prompt": result.EffectivePrompt,
		"negative_prompt":  result.NegativePrompt,
	})

	if result.Response != nil {
		createdAt := int64(0)
		if !result.Response.CreatedAt.IsZero() {
			createdAt = result.Response.CreatedAt.Unix()
		}
		outputFormat := req.ResponseFormat
		if outputFormat == "" {
			outputFormat = "png"
		}
		quality := req.Quality
		if quality == "" {
			quality = "standard"
		}
		size := req.Size
		if size == "" {
			size = "1024x1024"
		}

		for i, img := range result.Response.Images {
			payload := map[string]any{
				"type":          "image_generation.completed",
				"index":         i,
				"created_at":    createdAt,
				"output_format": outputFormat,
				"quality":       quality,
				"size":          size,
			}
			if img.URL != "" {
				payload["url"] = img.URL
			}
			if img.B64JSON != "" {
				payload["b64_json"] = img.B64JSON
			}
			if img.RevisedPrompt != "" {
				payload["revised_prompt"] = img.RevisedPrompt
			}
			if img.Seed != 0 {
				payload["seed"] = img.Seed
			}
			if i == len(result.Response.Images)-1 && result.Response.Usage.ImagesGenerated > 0 {
				payload["usage"] = result.Response.Usage
			}
			_ = writeSSEEventJSON(w, "image_generation.completed", payload)
		}

		_ = writeSSEEventJSON(w, "image_generation.done", map[string]any{
			"type":     "image_generation.done",
			"mode":     result.Mode,
			"provider": result.Provider,
			"usage":    result.Response.Usage,
		})
	} else {
		_ = writeSSEEventJSON(w, "image_generation.done", map[string]any{
			"type":     "image_generation.done",
			"mode":     result.Mode,
			"provider": result.Provider,
		})
	}
	_ = writeSSE(w, []byte("data: [DONE]\n\n"))
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
	CallbackURL       string   `json:"callback_url,omitempty"` // 可灵等异步视频：任务完成后回调地址
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
	// V-012: Duration >0 and <=300, FPS >0 and <=60
	if req.Duration != 0 && (req.Duration <= 0 || req.Duration > 300) {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "duration must be >0 and <=300", h.logger)
		return
	}
	if req.FPS != 0 && (req.FPS <= 0 || req.FPS > 60) {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "fps must be >0 and <=60", h.logger)
		return
	}
	if len(h.videoProviders) == 0 {
		WriteErrorMessage(w, http.StatusServiceUnavailable, types.ErrServiceUnavailable, "no video provider configured", h.logger)
		return
	}

	result, err := h.service.GenerateVideo(r.Context(), req)
	if err != nil {
		WriteErrorMessage(w, toHTTPStatus(err), errorCodeFrom(err, types.ErrUpstreamError), strings.TrimSpace(err.Error()), h.logger)
		return
	}

	WriteSuccess(w, map[string]any{
		"mode":             result.Mode,
		"provider":         result.Provider,
		"effective_prompt": result.EffectivePrompt,
		"response":         result.Response,
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
	if h.structuredProvider == nil {
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

	so, err := structured.NewStructuredOutput[visualPlan](h.structuredProvider)
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
	if h.structuredProvider == nil {
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
		model = h.defaultChatModel
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	if !req.AgentMode {
		resp, err := h.invokeChat(ctx, &llm.ChatRequest{
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
	planResp, err := h.invokeChat(ctx, &llm.ChatRequest{
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

	finalResp, err := h.invokeChat(ctx, &llm.ChatRequest{
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

func (h *MultimodalHandler) invokeChat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if h.gateway == nil {
		return nil, types.NewServiceUnavailableError("llm gateway is not configured")
	}

	resp, err := h.gateway.Invoke(ctx, &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		ModelHint:  req.Model,
		TraceID:    req.TraceID,
		Payload:    req,
	})
	if err != nil {
		return nil, err
	}

	chatResp, ok := resp.Output.(*llm.ChatResponse)
	if !ok || chatResp == nil {
		return nil, types.NewInternalError("invalid chat gateway response")
	}
	return chatResp, nil
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
	status, code := httpStatusAndCodeFrom(err)
	WriteErrorMessage(w, status, code, msg, h.logger)
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
