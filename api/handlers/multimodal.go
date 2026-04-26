package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/pkg/storage"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

const (
	defaultReferenceBytes = 8 << 20 // 8MB
	defaultReferenceTTL   = 2 * time.Hour
)

var (
	validImageSizes     = []string{"256x256", "512x512", "1024x1024", "1024x768", "768x1024", "1536x1024", "1024x1536", "1536x1536", "1792x1024", "1024x1792"}
	validImageQualities = []string{"standard", "hd"}
	validImageStyles    = []string{"vivid", "natural"}
)

type MultimodalHandlerRuntimeDeps struct {
	DefaultImageProvider string
	DefaultVideoProvider string
	ImageProviders       []string
	VideoProviders       []string
	ReferenceMaxSize     int64
	ReferenceTTL         time.Duration
	ReferenceStore       storage.ReferenceStore
	ChatEnabled          bool
	ResolveImageProvider usecase.MultimodalProviderResolver
	ResolveVideoProvider usecase.MultimodalProviderResolver
	ImageStreamProvider  func(string) (image.StreamingProvider, bool)
}

type MultimodalHandler struct {
	BaseHandler[usecase.MultimodalService]

	defaultImageProvider string
	defaultVideoProvider string
	imageProviders       []string
	videoProviders       []string
	referenceMaxSize     int64
	referenceTTL         time.Duration
	referenceStore       storage.ReferenceStore
	chatEnabled          bool

	resolveImageProviderFn usecase.MultimodalProviderResolver
	resolveVideoProviderFn usecase.MultimodalProviderResolver
	imageStreamProviderFn  func(string) (image.StreamingProvider, bool)
}

func NewMultimodalHandler(service usecase.MultimodalService, logger *zap.Logger) (*MultimodalHandler, error) {
	if logger == nil {
		return nil, fmt.Errorf("api.MultimodalHandler: logger is required and cannot be nil")
	}
	return &MultimodalHandler{
		BaseHandler:      NewBaseHandler(service, logger.With(zap.String("handler", "multimodal"))),
		referenceMaxSize: defaultReferenceBytes,
		referenceTTL:     defaultReferenceTTL,
		referenceStore:   storage.NewMemoryReferenceStore(),
	}, nil
}

func (h *MultimodalHandler) ApplyRuntimeDeps(deps MultimodalHandlerRuntimeDeps) {
	if h == nil {
		return
	}
	if deps.ReferenceMaxSize <= 0 {
		deps.ReferenceMaxSize = defaultReferenceBytes
	}
	if deps.ReferenceTTL <= 0 {
		deps.ReferenceTTL = defaultReferenceTTL
	}
	if deps.ReferenceStore == nil {
		deps.ReferenceStore = storage.NewMemoryReferenceStore()
	}
	deps.ImageProviders = append([]string(nil), deps.ImageProviders...)
	deps.VideoProviders = append([]string(nil), deps.VideoProviders...)
	sort.Strings(deps.ImageProviders)
	sort.Strings(deps.VideoProviders)

	h.mu.Lock()
	defer h.mu.Unlock()
	h.defaultImageProvider = strings.TrimSpace(deps.DefaultImageProvider)
	h.defaultVideoProvider = strings.TrimSpace(deps.DefaultVideoProvider)
	h.imageProviders = deps.ImageProviders
	h.videoProviders = deps.VideoProviders
	h.referenceMaxSize = deps.ReferenceMaxSize
	h.referenceTTL = deps.ReferenceTTL
	h.referenceStore = deps.ReferenceStore
	h.chatEnabled = deps.ChatEnabled
	h.resolveImageProviderFn = deps.ResolveImageProvider
	h.resolveVideoProviderFn = deps.ResolveVideoProvider
	h.imageStreamProviderFn = deps.ImageStreamProvider
}

func (h *MultimodalHandler) HandleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}

	h.mu.RLock()
	imageProviders := append([]string(nil), h.imageProviders...)
	videoProviders := append([]string(nil), h.videoProviders...)
	defaultImageProvider := h.defaultImageProvider
	defaultVideoProvider := h.defaultVideoProvider
	chatEnabled := h.chatEnabled
	h.mu.RUnlock()

	WriteSuccess(w, map[string]any{
		"image_providers":        imageProviders,
		"video_providers":        videoProviders,
		"default_image_provider": defaultImageProvider,
		"default_video_provider": defaultVideoProvider,
		"features": map[string]bool{
			"reference_upload": len(imageProviders) > 0 || len(videoProviders) > 0,
			"text_to_image":    len(imageProviders) > 0,
			"image_to_image":   len(imageProviders) > 0,
			"image_stream":     len(imageProviders) > 0,
			"text_to_video":    len(videoProviders) > 0,
			"image_to_video":   len(videoProviders) > 0,
			"advanced_prompt":  true,
			"chat":             chatEnabled,
			"agent_mode":       chatEnabled,
			"plan_generation":  chatEnabled,
		},
	})
}

func (h *MultimodalHandler) HandleUploadReference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}

	h.mu.RLock()
	referenceMaxSize := h.referenceMaxSize
	referenceStore := h.referenceStore
	h.mu.RUnlock()

	if err := r.ParseMultipartForm(referenceMaxSize + (1 << 20)); err != nil {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid multipart form", h.logger)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "file is required", h.logger)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, referenceMaxSize+1))
	if err != nil {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "failed to read uploaded file", h.logger)
		return
	}
	if len(data) == 0 {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "uploaded file is empty", h.logger)
		return
	}
	if int64(len(data)) > referenceMaxSize {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, fmt.Sprintf("file too large (max %d bytes)", referenceMaxSize), h.logger)
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
	if err := referenceStore.Save(ref); err != nil {
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

func (h *MultimodalHandler) HandleImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req usecase.MultimodalImageRequest
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

	h.mu.RLock()
	hasProviders := len(h.imageProviders) > 0
	h.mu.RUnlock()
	if !hasProviders {
		WriteErrorMessage(w, http.StatusServiceUnavailable, types.ErrServiceUnavailable, "no image provider configured", h.logger)
		return
	}

	if req.Stream {
		h.handleImageStream(w, r, req)
		return
	}

	service, svcErr := h.currentServiceOrUnavailable("multimodal")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}

	result, err := service.GenerateImage(r.Context(), req)
	if err != nil {
		status, code := multimodalHTTPStatusAndCodeFrom(err)
		WriteErrorMessage(w, status, code, strings.TrimSpace(err.Error()), h.logger)
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

func (h *MultimodalHandler) handleImageStream(w http.ResponseWriter, r *http.Request, req usecase.MultimodalImageRequest) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	providerName, resolveErr := h.resolveImageProvider(req.Provider)
	if resolveErr == nil {
		h.mu.RLock()
		lookup := h.imageStreamProviderFn
		h.mu.RUnlock()
		if lookup != nil {
			if sp, ok := lookup(providerName); ok {
				h.handleImageNativeStream(w, r, req, providerName, sp)
				return
			}
		}
	}

	service, svcErr := h.currentServiceOrUnavailable("multimodal")
	if svcErr != nil {
		_ = writeMultimodalSSEEventJSON(w, "error", map[string]any{"type": "error", "code": svcErr.Code, "message": svcErr.Message})
		_ = writeMultimodalSSE(w, []byte("data: [DONE]\n\n"))
		return
	}

	result, err := service.GenerateImage(r.Context(), req)
	if err != nil {
		_, code := multimodalHTTPStatusAndCodeFrom(err)
		_ = writeMultimodalSSEEventJSON(w, "error", map[string]any{
			"type":    "error",
			"code":    code,
			"message": strings.TrimSpace(err.Error()),
		})
		_ = writeMultimodalSSE(w, []byte("data: [DONE]\n\n"))
		return
	}
	h.flushImageResult(w, req, result)
}

func (h *MultimodalHandler) handleImageNativeStream(
	w http.ResponseWriter,
	r *http.Request,
	req usecase.MultimodalImageRequest,
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

	_ = writeMultimodalSSEEventJSON(w, "image_generation.started", map[string]any{
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
			_, code := multimodalHTTPStatusAndCodeFrom(chunk.Err)
			_ = writeMultimodalSSEEventJSON(w, "error", map[string]any{
				"type":    "error",
				"code":    code,
				"message": strings.TrimSpace(chunk.Err.Error()),
			})
			if canFlush {
				flusher.Flush()
			}
		case chunk.Text != "":
			_ = writeMultimodalSSEEventJSON(w, "image_generation.thinking", map[string]any{
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
			_ = writeMultimodalSSEEventJSON(w, "image_generation.completed", payload)
			if canFlush {
				flusher.Flush()
			}
			imageIndex++
		case chunk.Done:
			_ = writeMultimodalSSEEventJSON(w, "image_generation.done", map[string]any{
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
		h.logger.Warn("multimodal image stream error", zap.Error(streamErr))
	}
	_ = writeMultimodalSSE(w, []byte("data: [DONE]\n\n"))
	if canFlush {
		flusher.Flush()
	}
}

func (h *MultimodalHandler) flushImageResult(w http.ResponseWriter, req usecase.MultimodalImageRequest, result *usecase.MultimodalImageResult) {
	_ = writeMultimodalSSEEventJSON(w, "image_generation.started", map[string]any{
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
			_ = writeMultimodalSSEEventJSON(w, "image_generation.completed", payload)
		}

		_ = writeMultimodalSSEEventJSON(w, "image_generation.done", map[string]any{
			"type":     "image_generation.done",
			"mode":     result.Mode,
			"provider": result.Provider,
			"usage":    result.Response.Usage,
		})
	} else {
		_ = writeMultimodalSSEEventJSON(w, "image_generation.done", map[string]any{
			"type":     "image_generation.done",
			"mode":     result.Mode,
			"provider": result.Provider,
		})
	}
	_ = writeMultimodalSSE(w, []byte("data: [DONE]\n\n"))
}

func (h *MultimodalHandler) HandleVideo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req usecase.MultimodalVideoRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "prompt is required", h.logger)
		return
	}
	if req.Duration != 0 && (req.Duration <= 0 || req.Duration > 300) {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "duration must be >0 and <=300", h.logger)
		return
	}
	if req.FPS != 0 && (req.FPS <= 0 || req.FPS > 60) {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "fps must be >0 and <=60", h.logger)
		return
	}

	h.mu.RLock()
	hasProviders := len(h.videoProviders) > 0
	h.mu.RUnlock()
	if !hasProviders {
		WriteErrorMessage(w, http.StatusServiceUnavailable, types.ErrServiceUnavailable, "no video provider configured", h.logger)
		return
	}

	service, svcErr := h.currentServiceOrUnavailable("multimodal")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}

	result, err := service.GenerateVideo(r.Context(), req)
	if err != nil {
		status, code := multimodalHTTPStatusAndCodeFrom(err)
		WriteErrorMessage(w, status, code, strings.TrimSpace(err.Error()), h.logger)
		return
	}

	WriteSuccess(w, map[string]any{
		"mode":             result.Mode,
		"provider":         result.Provider,
		"effective_prompt": result.EffectivePrompt,
		"response":         result.Response,
	})
}

func (h *MultimodalHandler) HandlePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	h.mu.RLock()
	chatEnabled := h.chatEnabled
	h.mu.RUnlock()
	if !chatEnabled {
		WriteErrorMessage(w, http.StatusServiceUnavailable, types.ErrServiceUnavailable, "chat provider is not configured", h.logger)
		return
	}

	var req usecase.MultimodalPlanRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "prompt is required", h.logger)
		return
	}
	if req.ShotCount > 12 {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "shot_count must be <= 12", h.logger)
		return
	}

	service, svcErr := h.currentServiceOrUnavailable("multimodal")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}

	result, err := service.GeneratePlan(r.Context(), req)
	if err != nil {
		h.writeProviderError(w, err)
		return
	}
	WriteSuccess(w, map[string]any{"plan": result.Plan})
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

	h.mu.RLock()
	chatEnabled := h.chatEnabled
	h.mu.RUnlock()
	if !chatEnabled {
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

	service, svcErr := h.currentServiceOrUnavailable("multimodal")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}

	result, err := service.Chat(r.Context(), usecase.MultimodalChatRequest{
		Model:        req.Model,
		Messages:     convertAPIMessages(messages),
		SystemPrompt: req.SystemPrompt,
		Temperature:  req.Temperature,
		Advanced:     req.Advanced,
		AgentMode:    req.AgentMode,
	})
	if err != nil {
		h.writeProviderError(w, err)
		return
	}

	payload := map[string]any{"mode": result.Mode}
	if result.Mode == "agent" {
		payload["planner_output"] = result.PlannerOutput
		payload["final_response"] = result.FinalResponse
		payload["final_text"] = result.FinalText
	} else {
		payload["response"] = result.Response
	}
	WriteSuccess(w, payload)
}

func (h *MultimodalHandler) resolveImageProvider(provider string) (string, error) {
	h.mu.RLock()
	resolver := h.resolveImageProviderFn
	h.mu.RUnlock()
	if resolver == nil {
		return "", fmt.Errorf("image provider resolver is not configured")
	}
	return resolver(provider)
}

func (h *MultimodalHandler) resolveVideoProvider(provider string) (string, error) {
	h.mu.RLock()
	resolver := h.resolveVideoProviderFn
	h.mu.RUnlock()
	if resolver == nil {
		return "", fmt.Errorf("video provider resolver is not configured")
	}
	return resolver(provider)
}

func (h *MultimodalHandler) writeProviderError(w http.ResponseWriter, err error) {
	msg := strings.TrimSpace(err.Error())
	status, code := multimodalHTTPStatusAndCodeFrom(err)
	WriteErrorMessage(w, status, code, msg, h.logger)
}

func (h *MultimodalHandler) cleanupReferences(now time.Time) {
	h.mu.RLock()
	referenceTTL := h.referenceTTL
	referenceStore := h.referenceStore
	h.mu.RUnlock()
	if referenceStore == nil {
		return
	}
	referenceStore.Cleanup(now.Add(-referenceTTL))
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
			Images:           convertMultimodalAPIImagesToTypes(msg.Images),
			Videos:           msg.Videos,
			Annotations:      msg.Annotations,
			Metadata:         msg.Metadata,
			Timestamp:        msg.Timestamp,
		})
	}
	return out
}

func convertMultimodalAPIImagesToTypes(images []api.ImageContent) []types.ImageContent {
	if len(images) == 0 {
		return nil
	}
	result := make([]types.ImageContent, len(images))
	for i, img := range images {
		result[i] = types.ImageContent{Type: img.Type, URL: img.URL, Data: img.Data}
	}
	return result
}

func multimodalHTTPStatusAndCodeFrom(err error) (int, types.ErrorCode) {
	if typed, ok := types.AsError(err); ok {
		status := typed.HTTPStatus
		if status <= 0 {
			status = multimodalErrorCodeToHTTPStatus(typed.Code)
		}
		if status <= 0 {
			status = http.StatusBadGateway
		}
		code := typed.Code
		if code == "" {
			code = types.ErrUpstreamError
		}
		return status, code
	}
	return http.StatusBadGateway, types.ErrUpstreamError
}

func multimodalErrorCodeToHTTPStatus(code types.ErrorCode) int {
	switch code {
	case types.ErrInvalidRequest, types.ErrInputValidation, types.ErrOutputValidation:
		return http.StatusBadRequest
	case types.ErrUnauthorized, types.ErrAuthentication:
		return http.StatusUnauthorized
	case types.ErrForbidden:
		return http.StatusForbidden
	case types.ErrModelNotFound:
		return http.StatusNotFound
	case types.ErrRateLimit, types.ErrQuotaExceeded:
		return http.StatusTooManyRequests
	case types.ErrTimeout, types.ErrUpstreamTimeout:
		return http.StatusGatewayTimeout
	case types.ErrServiceUnavailable, types.ErrProviderUnavailable, types.ErrRoutingUnavailable, types.ErrModelOverloaded:
		return http.StatusServiceUnavailable
	case types.ErrInternalError:
		return http.StatusInternalServerError
	case types.ErrContentFiltered:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusBadGateway
	}
}

func writeMultimodalSSE(w http.ResponseWriter, parts ...[]byte) error {
	for _, part := range parts {
		if _, err := w.Write(part); err != nil {
			return err
		}
	}
	return nil
}

func writeMultimodalSSEEventJSON(w http.ResponseWriter, event string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return writeMultimodalSSE(w, []byte("event: "), []byte(event), []byte("\n"), []byte("data: "), body, []byte("\n\n"))
}
