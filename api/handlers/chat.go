package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/pkg/telemetry"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// =============================================================================
// 💬 聊天接口 Handler
// =============================================================================

// defaultStreamTimeout is the default timeout for chat completion requests
// when no explicit timeout is provided by the client.
const defaultStreamTimeout = 30 * time.Second

// maxTokensUpperBound is the maximum allowed value for max_tokens (e.g. GPT-4 context limit).
const maxTokensUpperBound = 128000

// ChatHandler 聊天接口处理器
type ChatHandler struct {
	BaseHandler[usecase.ChatService]
	converter ChatConverter
}

// NewChatHandler 创建聊天处理器
func NewChatHandler(service usecase.ChatService, logger *zap.Logger) (*ChatHandler, error) {
	if logger == nil {
		return nil, fmt.Errorf("api.ChatHandler: logger is required and cannot be nil")
	}
	return &ChatHandler{
		BaseHandler: NewBaseHandler(service, logger),
		converter:   NewDefaultChatConverter(defaultStreamTimeout),
	}, nil
}

// HandleCompletion 处理聊天补全请求
// @Summary 聊天完成
// @Description 发送聊天完成请求
// @Tags 聊天
// @Accept json
// @Produce json
// @Param request body api.ChatRequest true "聊天请求"
// @Success 200 {object} api.ChatResponse "聊天响应"
// @Failure 400 {object} Response "无效请求"
// @Failure 500 {object} Response "内部错误"
// @Security ApiKeyAuth
// @Router /api/v1/chat/completions [post]
func (h *ChatHandler) HandleCompletion(w http.ResponseWriter, r *http.Request) {
	var req api.ChatRequest
	if !ValidateRequest(w, r, &req, h.logger) {
		return
	}

	// 从 JWT 上下文强制覆盖身份字段，防止水平越权
	enforceTenantID(r, &req)

	// 验证请求
	if err := h.validateChatRequest(&req); err != nil {
		WriteError(w, err, h.logger)
		return
	}

	service, svcErr := h.currentServiceOrUnavailable("chat")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}

	result, err := service.Complete(r.Context(), h.converter.ToUsecaseRequest(&req))
	if err != nil {
		WriteError(w, err, h.logger)
		return
	}

	requestID := w.Header().Get("X-Request-ID")
	traceLogger := telemetry.LoggerWithTrace(r.Context(), h.logger)
	traceLogger.Info("chat completion",
		zap.String("request_id", requestID),
		zap.String("model", req.Model),
		zap.Int("prompt_tokens", result.Raw.Usage.PromptTokens),
		zap.Int("completion_tokens", result.Raw.Usage.CompletionTokens),
		zap.Duration("duration", result.Duration),
	)

	WriteSuccess(w, h.converter.ToAPIResponseFromUsecase(result.Response))
}

// HandleStream 处理流式聊天请求
// @Summary 流式聊天完成
// @Description 发送流式聊天完成请求
// @Tags 聊天
// @Accept json
// @Produce text/event-stream
// @Param request body api.ChatRequest true "聊天请求"
// @Success 200 {string} string "SSE 流"
// @Failure 400 {object} Response "无效请求"
// @Failure 500 {object} Response "内部错误"
// @Security ApiKeyAuth
// @Router /api/v1/chat/completions/stream [post]
func (h *ChatHandler) HandleStream(w http.ResponseWriter, r *http.Request) {
	// 验证 Content-Type
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	// 解码请求
	var req api.ChatRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	// 从 JWT 上下文强制覆盖身份字段，防止水平越权
	enforceTenantID(r, &req)

	// 验证请求
	if err := h.validateChatRequest(&req); err != nil {
		WriteError(w, err, h.logger)
		return
	}

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 禁用 nginx 缓冲

	service, svcErr := h.currentServiceOrUnavailable("chat")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}

	stream, err := service.Stream(r.Context(), h.converter.ToUsecaseRequest(&req))
	if err != nil {
		WriteError(w, err, h.logger)
		return
	}

	// 发送流式数据
	flusher, ok := w.(http.Flusher)
	if !ok {
		err := types.NewInternalError("streaming not supported")
		WriteError(w, err, h.logger)
		return
	}

	requestID := w.Header().Get("X-Request-ID")

	for chunk := range stream {
		if chunk.Err != nil {
			h.logger.Error("stream error",
				zap.String("request_id", requestID),
				zap.Error(chunk.Err),
			)
			// SSE 错误事件 — 包含 code 与 message，使用 json.Marshal 转义防止 JSON 注入
			errPayload, marshalErr := json.Marshal(map[string]string{
				"code":    string(chunk.Err.Code),
				"message": chunk.Err.Message,
			})
			if marshalErr != nil {
				h.logger.Error("failed to marshal error payload", zap.Error(marshalErr))
				errPayload = []byte(`{"code":"INTERNAL_ERROR","message":"internal error"}`)
			}
			if err := writeSSE(w, []byte("event: error\n"), []byte("data: "), errPayload, []byte("\n\n")); err != nil {
				h.logger.Error("failed to write SSE error event", zap.Error(err))
			}
			flusher.Flush()
			return
		}

		if chunk.Chunk == nil {
			h.logger.Error("invalid stream chunk payload",
				zap.String("request_id", requestID),
			)
			WriteError(w, types.NewInternalError("invalid stream chunk payload"), h.logger)
			return
		}

		// 转换为 API 格式
		apiChunk := h.convertToAPIStreamChunk(chunk.Chunk)

		// 发送 SSE 事件
		if err := writeSSE(w, []byte("data: ")); err != nil {
			h.logger.Error("failed to write SSE data prefix", zap.Error(err))
			return
		}
		if err := writeJSON(w, apiChunk); err != nil {
			h.logger.Error("failed to write chunk",
				zap.String("request_id", requestID),
				zap.Error(err),
			)
			return
		}
		if err := writeSSE(w, []byte("\n\n")); err != nil {
			h.logger.Error("failed to write SSE data suffix", zap.Error(err))
			return
		}
		flusher.Flush()
	}

	// 发送结束标记
	if err := writeSSE(w, []byte("data: [DONE]\n\n")); err != nil {
		h.logger.Error("failed to write SSE done marker", zap.Error(err))
	}
	flusher.Flush()
}

// =============================================================================
// 🔧 辅助函数
// =============================================================================

// validateChatRequest 验证聊天请求
func (h *ChatHandler) validateChatRequest(req *api.ChatRequest) *types.Error {
	if req.Model == "" {
		return types.NewInvalidRequestError("model is required")
	}

	if len(req.Messages) == 0 {
		return types.NewInvalidRequestError("messages cannot be empty")
	}

	// 验证 max_tokens 参数
	if !ValidateNonNegative(float64(req.MaxTokens)) || req.MaxTokens > maxTokensUpperBound {
		return types.NewError(types.ErrInvalidRequest, "max_tokens must be between 0 and 128000")
	}

	// 验证温度参数
	if req.Temperature < 0 || req.Temperature > 2 {
		return types.NewInvalidRequestError("temperature must be between 0 and 2")
	}

	// 验证 TopP 参数
	if req.TopP < 0 || req.TopP > 1 {
		return types.NewInvalidRequestError("top_p must be between 0 and 1")
	}

	if _, err := usecase.NormalizeProviderHint(req.Provider); err != nil {
		return err
	}
	if _, err := usecase.NormalizeRoutePolicy(req.RoutePolicy); err != nil {
		return err
	}
	if _, err := usecase.NormalizeEndpointMode(req.EndpointMode); err != nil {
		return err
	}

	validRoles := []string{"system", "developer", "user", "assistant", "tool"}
	validImageTypes := []string{"url", "base64"}

	for i, msg := range req.Messages {
		// V-006: Role validation
		if !ValidateEnum(msg.Role, validRoles) {
			return types.NewError(types.ErrInvalidRequest,
				fmt.Sprintf("messages[%d].role must be one of: system, developer, user, assistant, tool", i))
		}

		// V-003: Content length validation per message
		if len(msg.Content) > 100000 {
			return types.NewError(types.ErrInvalidRequest,
				fmt.Sprintf("messages[%d].content exceeds maximum length of 100000 characters", i))
		}

		// V-007: ImageContent.Type enum validation
		for j, img := range msg.Images {
			if !ValidateEnum(img.Type, validImageTypes) {
				return types.NewError(types.ErrInvalidRequest,
					fmt.Sprintf("messages[%d].images[%d].type must be one of: url, base64", i, j))
			}
		}
	}

	return nil
}

// HandleCapabilities handles GET /api/v1/chat/capabilities
func (h *ChatHandler) HandleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("chat")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}

	WriteSuccess(w, map[string]any{
		"route_params":         []string{"provider", "model", "route_policy", "endpoint_mode", "tags", "metadata"},
		"route_policies":       service.SupportedRoutePolicies(),
		"default_route_policy": service.DefaultRoutePolicy(),
		"notes": []string{
			"provider/model/route_policy/endpoint_mode are routed by service layer",
			"provider hint effectiveness depends on runtime provider implementation",
		},
	})
}

// convertToAPIStreamChunk 转换流式块
func (h *ChatHandler) convertToAPIStreamChunk(chunk *usecase.ChatStreamChunk) *api.StreamChunk {
	return h.converter.ToAPIStreamChunkFromUsecase(chunk)
}

// handleProviderError 处理 Provider 错误
func (h *ChatHandler) handleProviderError(w http.ResponseWriter, err error) {
	if typedErr, ok := err.(*types.Error); ok {
		WriteError(w, typedErr, h.logger)
		return
	}

	// 未知错误，包装为内部错误
	internalErr := types.NewInternalError("provider error").
		WithCause(err)

	WriteError(w, internalErr, h.logger)
}

// writeJSON 写入 JSON（不包含响应头）
func writeJSON(w http.ResponseWriter, data any) error {
	encoder := json.NewEncoder(w)
	return encoder.Encode(data)
}

// writeSSE 将多个字节片段依次写入 ResponseWriter，任一写入失败立即返回错误。
func writeSSE(w http.ResponseWriter, parts ...[]byte) error {
	for _, p := range parts {
		if _, err := w.Write(p); err != nil {
			return err
		}
	}
	return nil
}

// =============================================================================
// 🔄 类型转换辅助函数
// =============================================================================

// convertTypesMessageToAPI converts a types.Message to an api.Message,
// copying all fields including reasoning/thinking/tool/multimodal metadata.
func convertTypesMessageToAPI(msg types.Message) api.Message {
	m := api.Message{
		Role:               string(msg.Role),
		Content:            msg.Content,
		ReasoningContent:   msg.ReasoningContent,
		ReasoningSummaries: msg.ReasoningSummaries,
		OpaqueReasoning:    msg.OpaqueReasoning,
		ThinkingBlocks:     msg.ThinkingBlocks,
		Refusal:            msg.Refusal,
		Name:               msg.Name,
		ToolCalls:          msg.ToolCalls,
		ToolCallID:         msg.ToolCallID,
		IsToolError:        msg.IsToolError,
		Videos:             msg.Videos,
		Annotations:        msg.Annotations,
		Metadata:           msg.Metadata,
		Timestamp:          msg.Timestamp,
	}
	if len(msg.Images) > 0 {
		images := make([]api.ImageContent, len(msg.Images))
		for i, img := range msg.Images {
			images[i] = api.ImageContent{
				Type: img.Type,
				URL:  img.URL,
				Data: img.Data,
			}
		}
		m.Images = images
	}
	return m
}

// Note: convertAPIToolCallsToTypes and convertTypesToolCallsToAPI were removed
// because api.ToolCall and types.ToolCall are identical aliases, so no conversion is needed.

// =============================================================================
// 🖼️ Image 转换辅助函数
// =============================================================================

// convertAPIImagesToTypes 将 api.ImageContent 切片转换为 types.ImageContent 切片。
// 两者字段相同但属于不同包的独立类型定义，需要逐字段拷贝。
func convertAPIImagesToTypes(images []api.ImageContent) []types.ImageContent {
	if len(images) == 0 {
		return nil
	}
	result := make([]types.ImageContent, len(images))
	for i, img := range images {
		result[i] = types.ImageContent{
			Type: img.Type,
			URL:  img.URL,
			Data: img.Data,
		}
	}
	return result
}
