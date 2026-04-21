package handlers

import (
	"context"
	"errors"
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
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/pkg/storage"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

const (
	defaultReferenceBytes    = 8 << 20 // 8MB
	defaultReferenceTTL      = 2 * time.Hour
	defaultChatModelFallback = "gpt-4o-mini"
)

var (
	validImageSizes     = []string{"256x256", "512x512", "1024x1024", "1024x768", "768x1024", "1536x1024", "1024x1536", "1536x1536", "1792x1024", "1024x1792"}
	validImageQualities = []string{"standard", "hd"}
	validImageStyles    = []string{"vivid", "natural"}
)

type MultimodalHandler struct {
	logger  *zap.Logger
	service usecase.MultimodalService
	runtime MultimodalHandlerRuntimeDeps

	promptEnhancer  *agent.PromptEnhancer
	promptOptimizer *agent.PromptOptimizer
}

// MultimodalHandlerRuntimeDeps 为 handler 注入只读运行时依赖。
// 由 bootstrap 在启动装配阶段构建并注入，handler 本身不负责 provider/router/gateway 组装。
type MultimodalHandlerRuntimeDeps struct {
	DefaultImageProvider string
	DefaultVideoProvider string
	ImageProviders       []string
	VideoProviders       []string
	ReferenceMaxSize     int64
	ReferenceTTL         time.Duration
	ReferenceStore       storage.ReferenceStore
	DefaultChatModel     string
	StructuredChat       bool
	StructuredProvider   llm.Provider
	ResolveImageProvider func(provider string) (string, error)
	ResolveVideoProvider func(provider string) (string, error)
	ImageStreamProvider  func(provider string) (image.StreamingProvider, bool)
	InvokeChat           func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)
}

type unavailableMultimodalService struct{}

func (s *unavailableMultimodalService) GenerateImage(ctx context.Context, req usecase.MultimodalImageRequest) (*usecase.MultimodalImageResult, error) {
	_ = ctx
	_ = req
	return nil, types.NewServiceUnavailableError("multimodal service is not configured")
}

func (s *unavailableMultimodalService) GenerateVideo(ctx context.Context, req usecase.MultimodalVideoRequest) (*usecase.MultimodalVideoResult, error) {
	_ = ctx
	_ = req
	return nil, types.NewServiceUnavailableError("multimodal service is not configured")
}

// NewMultimodalHandler 创建多模态 Handler。
// 业务执行仅依赖 usecase.MultimodalService；provider/router/gateway 等运行时依赖由 bootstrap 注入。
func NewMultimodalHandler(service usecase.MultimodalService, logger *zap.Logger) *MultimodalHandler {
	if logger == nil {
		logger = zap.NewNop()
	}

	if service == nil {
		service = &unavailableMultimodalService{}
	}

	handler := &MultimodalHandler{
		logger:          logger.With(zap.String("handler", "multimodal")),
		service:         service,
		promptEnhancer:  agent.NewPromptEnhancer(*agent.DefaultPromptEnhancerConfig()),
		promptOptimizer: agent.NewPromptOptimizer(),
	}

	handler.ApplyRuntimeDeps(MultimodalHandlerRuntimeDeps{})
	return handler
}

// ApplyRuntimeDeps 注入 handler 的只读运行时依赖。
// referenceStore 为 nil 时使用内存实现，仅建议在测试或开发环境使用；生产环境应由组合根注入 Redis 等持久化实现。
func (h *MultimodalHandler) ApplyRuntimeDeps(deps MultimodalHandlerRuntimeDeps) {
	imageNames := append([]string(nil), deps.ImageProviders...)
	videoNames := append([]string(nil), deps.VideoProviders...)
	sort.Strings(imageNames)
	sort.Strings(videoNames)

	normalized := deps
	normalized.ImageProviders = imageNames
	normalized.VideoProviders = videoNames
	if normalized.ReferenceMaxSize <= 0 {
		normalized.ReferenceMaxSize = defaultReferenceBytes
	}
	if normalized.ReferenceTTL <= 0 {
		normalized.ReferenceTTL = defaultReferenceTTL
	}
	if normalized.ReferenceStore == nil {
		normalized.ReferenceStore = storage.NewMemoryReferenceStore()
	}
	if normalized.DefaultChatModel == "" {
		normalized.DefaultChatModel = defaultChatModelFallback
	}
	if normalized.DefaultImageProvider == "" && len(imageNames) > 0 {
		normalized.DefaultImageProvider = imageNames[0]
	}
	if normalized.DefaultVideoProvider == "" && len(videoNames) > 0 {
		normalized.DefaultVideoProvider = videoNames[0]
	}
	h.runtime = normalized
}

func (h *MultimodalHandler) HandleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	WriteSuccess(w, map[string]any{
		"image_providers":        h.runtime.ImageProviders,
		"video_providers":        h.runtime.VideoProviders,
		"default_image_provider": h.runtime.DefaultImageProvider,
		"default_video_provider": h.runtime.DefaultVideoProvider,
		"features": map[string]bool{
			"reference_upload": len(h.runtime.ImageProviders) > 0 || len(h.runtime.VideoProviders) > 0,
			"text_to_image":    len(h.runtime.ImageProviders) > 0,
			"image_to_image":   len(h.runtime.ImageProviders) > 0,
			"image_stream":     len(h.runtime.ImageProviders) > 0,
			"text_to_video":    len(h.runtime.VideoProviders) > 0,
			"image_to_video":   len(h.runtime.VideoProviders) > 0,
			"advanced_prompt":  true,
			"chat":             h.runtime.StructuredChat,
			"agent_mode":       h.runtime.StructuredChat,
			"plan_generation":  h.runtime.StructuredChat,
		},
	})
}

func (h *MultimodalHandler) HandleUploadReference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if err := r.ParseMultipartForm(h.runtime.ReferenceMaxSize + (1 << 20)); err != nil {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid multipart form", h.logger)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "file is required", h.logger)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, h.runtime.ReferenceMaxSize+1))
	if err != nil {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "failed to read uploaded file", h.logger)
		return
	}
	if len(data) == 0 {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "uploaded file is empty", h.logger)
		return
	}
	if int64(len(data)) > h.runtime.ReferenceMaxSize {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, fmt.Sprintf("file too large (max %d bytes)", h.runtime.ReferenceMaxSize), h.logger)
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
	if err := h.runtime.ReferenceStore.Save(ref); err != nil {
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

type multimodalImageRequest = usecase.MultimodalImageRequest

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
	if len(h.runtime.ImageProviders) == 0 {
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
		if h.runtime.ImageStreamProvider != nil {
			if sp, ok := h.runtime.ImageStreamProvider(providerName); ok {
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
func (h *MultimodalHandler) flushImageResult(w http.ResponseWriter, req multimodalImageRequest, result *usecase.MultimodalImageResult) {
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

type multimodalVideoRequest = usecase.MultimodalVideoRequest

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
	if len(h.runtime.VideoProviders) == 0 {
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
	if !h.runtime.StructuredChat || h.runtime.InvokeChat == nil {
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

	so, err := structured.NewStructuredOutput[visualPlan](h.runtime.StructuredProvider)
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
	if !h.runtime.StructuredChat || h.runtime.InvokeChat == nil {
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
		model = h.runtime.DefaultChatModel
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
	if h.runtime.InvokeChat == nil {
		return nil, types.NewServiceUnavailableError("llm gateway is not configured")
	}
	return h.runtime.InvokeChat(ctx, req)
}

func (h *MultimodalHandler) resolveImageProvider(provider string) (string, error) {
	if h.runtime.ResolveImageProvider == nil {
		return "", types.NewServiceUnavailableError("image provider resolver is not configured")
	}
	return h.runtime.ResolveImageProvider(provider)
}

func (h *MultimodalHandler) resolveVideoProvider(provider string) (string, error) {
	if h.runtime.ResolveVideoProvider == nil {
		return "", types.NewServiceUnavailableError("video provider resolver is not configured")
	}
	return h.runtime.ResolveVideoProvider(provider)
}

func (h *MultimodalHandler) writeProviderError(w http.ResponseWriter, err error) {
	msg := strings.TrimSpace(err.Error())
	status, code := httpStatusAndCodeFrom(err)
	WriteErrorMessage(w, status, code, msg, h.logger)
}

func (h *MultimodalHandler) cleanupReferences(now time.Time) {
	h.runtime.ReferenceStore.Cleanup(now.Add(-h.runtime.ReferenceTTL))
}

func convertAPIMessages(messages []api.Message) []types.Message {
	out := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		out = append(out, types.Message{
			Role:             types.Role(role),
			Content:          msg.Content,
			ReasoningContent: msg.ReasoningContent,
			ThinkingBlocks:   msg.ThinkingBlocks,
			Refusal:          msg.Refusal,
			Name:             msg.Name,
			ToolCalls:        msg.ToolCalls,
			ToolCallID:       msg.ToolCallID,
			IsToolError:      msg.IsToolError,
			Images:           convertAPIImagesToTypes(msg.Images),
			Videos:           msg.Videos,
			Annotations:      msg.Annotations,
			Metadata:         msg.Metadata,
			Timestamp:        msg.Timestamp,
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

// httpStatusAndCodeFrom 将 error 映射为 HTTP 状态码与错误码，供 multimodal handler 共用。
// 优先使用 types.Error 的 Code 与 HTTPStatus；非 types.Error 时根据错误文案推断 400 或 502。
func httpStatusAndCodeFrom(err error) (status int, code types.ErrorCode) {
	var typedErr *types.Error
	if errors.As(err, &typedErr) && typedErr != nil {
		code = typedErr.Code
		if typedErr.HTTPStatus != 0 {
			status = typedErr.HTTPStatus
		} else {
			status = errorCodeToHTTPStatus(typedErr.Code)
		}
		return status, code
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(msg, "invalid") ||
		strings.Contains(msg, "required") ||
		strings.Contains(msg, "unsupported") ||
		strings.Contains(msg, "not support") {
		return http.StatusBadRequest, types.ErrInvalidRequest
	}
	return http.StatusBadGateway, types.ErrUpstreamError
}

func errorCodeToHTTPStatus(code types.ErrorCode) int {
	switch code {
	case types.ErrInvalidRequest, types.ErrAuthentication, types.ErrUnauthorized,
		types.ErrForbidden, types.ErrToolValidation, types.ErrInputValidation,
		types.ErrOutputValidation, types.ErrGuardrailsViolated:
		return http.StatusBadRequest
	case types.ErrModelNotFound, types.ErrAgentNotFound:
		return http.StatusNotFound
	case types.ErrRateLimit, types.ErrQuotaExceeded:
		return http.StatusTooManyRequests
	case types.ErrServiceUnavailable, types.ErrProviderUnavailable, types.ErrRoutingUnavailable:
		return http.StatusServiceUnavailable
	case types.ErrUpstreamTimeout, types.ErrTimeout:
		return http.StatusGatewayTimeout
	case types.ErrInternalError:
		return http.StatusInternalServerError
	default:
		return http.StatusBadGateway
	}
}

func toHTTPStatus(err error) int {
	status, _ := httpStatusAndCodeFrom(err)
	return status
}

func errorCodeFrom(err error, fallback types.ErrorCode) types.ErrorCode {
	_, code := httpStatusAndCodeFrom(err)
	if code != "" {
		return code
	}
	return fallback
}
