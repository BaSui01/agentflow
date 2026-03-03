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
	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/multimodal"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

const (
	defaultReferenceBytes = 8 << 20 // 8MB
	defaultReferenceTTL   = 2 * time.Hour
	defaultChatModel      = "gpt-4o-mini"
	defaultNegativeText   = "blurry, low quality, watermark, text, logo, signature, bad anatomy, deformed, mutated"
)

type MultimodalHandlerConfig struct {
	ChatProvider         llm.Provider
	PolicyManager        *llmpolicy.Manager
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
	Pipeline             multimodal.PromptPipeline
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
	referenceStore   ReferenceStore
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
		RunwayAPIKey:         cfg.RunwayAPIKey,
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
		DefaultImageProvider: cfg.DefaultImageProvider,
		DefaultVideoProvider: cfg.DefaultVideoProvider,
	}, logger)

	return NewMultimodalHandlerWithProviders(
		cfg.ChatProvider,
		cfg.PolicyManager,
		providerSet.ImageProviders,
		providerSet.VideoProviders,
		providerSet.DefaultImage,
		providerSet.DefaultVideo,
		cfg.Pipeline,
		cfg.ReferenceMaxSize,
		cfg.ReferenceTTL,
		cfg.ReferenceStore,
		logger,
	)
}

func NewMultimodalHandlerWithProviders(
	chatProvider llm.Provider,
	policyManager *llmpolicy.Manager,
	imageProviders map[string]image.Provider,
	videoProviders map[string]video.Provider,
	defaultImage string,
	defaultVideo string,
	pipeline multimodal.PromptPipeline,
	referenceMaxSize int64,
	referenceTTL time.Duration,
	referenceStore ReferenceStore,
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

	gw := llmgateway.New(llmgateway.Config{
		ChatProvider:  chatProvider,
		Capabilities:  capabilities.NewEntry(router),
		PolicyManager: policyManager,
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
	if !ValidateNonNegative(float64(req.N)) {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "n must be non-negative", h.logger)
		return
	}
	if len(h.imageProviders) == 0 {
		WriteErrorMessage(w, http.StatusServiceUnavailable, types.ErrServiceUnavailable, "no image provider configured", h.logger)
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
		model = defaultChatModel
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
