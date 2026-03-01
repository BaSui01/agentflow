package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// =============================================================================
// 💬 聊天接口 Handler
// =============================================================================

// defaultStreamTimeout is the default timeout for chat completion requests
// when no explicit timeout is provided by the client.
const defaultStreamTimeout = 30 * time.Second

// ChatHandler 聊天接口处理器
type ChatHandler struct {
	provider  llm.Provider
	converter ChatConverter
	logger    *zap.Logger
}

// NewChatHandler 创建聊天处理器
func NewChatHandler(provider llm.Provider, logger *zap.Logger) *ChatHandler {
	return &ChatHandler{
		provider:  provider,
		converter: NewDefaultChatConverter(defaultStreamTimeout),
		logger:    logger,
	}
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
	// 验证 Content-Type
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	// 解码请求
	var req api.ChatRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	// 验证请求
	if err := h.validateChatRequest(&req); err != nil {
		WriteError(w, err, h.logger)
		return
	}

	// 转换为 LLM 请求
	llmReq := h.convertToLLMRequest(&req)

	// 设置超时
	ctx := r.Context()
	if llmReq.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, llmReq.Timeout)
		defer cancel()
	}

	// 调用 Provider
	start := time.Now()
	resp, err := h.provider.Completion(ctx, llmReq)
	duration := time.Since(start)

	if err != nil {
		h.handleProviderError(w, err)
		return
	}

	// 转换响应
	apiResp := h.convertToAPIResponse(resp)

	// 记录日志
	requestID := w.Header().Get("X-Request-ID")
	h.logger.Info("chat completion",
		zap.String("request_id", requestID),
		zap.String("model", req.Model),
		zap.Int("prompt_tokens", resp.Usage.PromptTokens),
		zap.Int("completion_tokens", resp.Usage.CompletionTokens),
		zap.Duration("duration", duration),
	)

	WriteSuccess(w, apiResp)
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

	// 验证请求
	if err := h.validateChatRequest(&req); err != nil {
		WriteError(w, err, h.logger)
		return
	}

	// 转换为 LLM 请求
	llmReq := h.convertToLLMRequest(&req)

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 禁用 nginx 缓冲

	// 调用 Provider 流式接口
	ctx := r.Context()
	stream, err := h.provider.Stream(ctx, llmReq)
	if err != nil {
		h.handleProviderError(w, err)
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
			// SSE 错误事件 — 使用 json.Marshal 转义错误消息，防止 JSON 注入
			errPayload, marshalErr := json.Marshal(map[string]string{"error": chunk.Err.Message})
			if marshalErr != nil {
				h.logger.Error("failed to marshal error payload", zap.Error(marshalErr))
				errPayload = []byte(`{"error":"internal error"}`)
			}
			if err := writeSSE(w, []byte("event: error\n"), []byte("data: "), errPayload, []byte("\n\n")); err != nil {
				h.logger.Error("failed to write SSE error event", zap.Error(err))
			}
			flusher.Flush()
			return
		}

		// 转换为 API 格式
		apiChunk := h.convertToAPIStreamChunk(&chunk)

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

// allowedMessageRoles is the set of valid message roles for chat requests.
var allowedMessageRoles = []string{"system", "user", "assistant", "tool"}

// validateChatRequest 验证聊天请求
func (h *ChatHandler) validateChatRequest(req *api.ChatRequest) *types.Error {
	if req.Model == "" {
		return types.NewInvalidRequestError("model is required")
	}

	if len(req.Messages) == 0 {
		return types.NewInvalidRequestError("messages cannot be empty")
	}

	// 验证 max_tokens 参数
	if req.MaxTokens < 0 {
		return types.NewInvalidRequestError("max_tokens must be non-negative")
	}

	// V-002: MaxTokens range validation
	if req.MaxTokens < 0 || req.MaxTokens > 128000 {
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

	// 验证每条消息的 role
	for i, msg := range req.Messages {
		if !ValidateEnum(msg.Role, allowedMessageRoles) {
			return types.NewInvalidRequestError(
				fmt.Sprintf("messages[%d].role must be one of: system, user, assistant, tool", i))
		}
	}

	// V-006: Validate Message.Role enum values
	validRoles := map[string]bool{
		"system": true, "user": true, "assistant": true, "tool": true,
	}

	for i, msg := range req.Messages {
		// V-006: Role validation
		if !validRoles[msg.Role] {
			return types.NewError(types.ErrInvalidRequest,
				fmt.Sprintf("messages[%d].role must be one of: system, user, assistant, tool", i))
		}

		// V-003: Content length validation per message
		if len(msg.Content) > 100000 {
			return types.NewError(types.ErrInvalidRequest,
				fmt.Sprintf("messages[%d].content exceeds maximum length of 100000 characters", i))
		}

		// V-007: ImageContent.Type enum validation
		validImageTypes := map[string]bool{"url": true, "base64": true}
		for j, img := range msg.Images {
			if !validImageTypes[img.Type] {
				return types.NewError(types.ErrInvalidRequest,
					fmt.Sprintf("messages[%d].images[%d].type must be one of: url, base64", i, j))
			}
		}
	}

	return nil
}

// convertToLLMRequest 转换为 LLM 请求
func (h *ChatHandler) convertToLLMRequest(req *api.ChatRequest) *llm.ChatRequest {
	return h.converter.ToLLMRequest(req)
}

// convertToAPIResponse 转换为 API 响应
func (h *ChatHandler) convertToAPIResponse(resp *llm.ChatResponse) *api.ChatResponse {
	return h.converter.ToAPIResponse(resp)
}

// convertChoices 转换选择列表
func (h *ChatHandler) convertChoices(choices []llm.ChatChoice) []api.ChatChoice {
	return h.converter.ToAPIChoices(choices)
}

// convertUsage 转换使用统计
func (h *ChatHandler) convertUsage(usage llm.ChatUsage) api.ChatUsage {
	return h.converter.ToAPIUsage(usage)
}

// convertToAPIStreamChunk 转换流式块
func (h *ChatHandler) convertToAPIStreamChunk(chunk *llm.StreamChunk) *api.StreamChunk {
	return h.converter.ToAPIStreamChunk(chunk)
}

// convertStreamUsage safely converts *llm.ChatUsage to *api.ChatUsage
// without relying on unsafe pointer casts between distinct types.
func convertStreamUsage(u *llm.ChatUsage) *api.ChatUsage {
	if u == nil {
		return nil
	}
	return &api.ChatUsage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
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
// copying all fields including Images, Metadata, and Timestamp.
func convertTypesMessageToAPI(msg types.Message) api.Message {
	m := api.Message{
		Role:       string(msg.Role),
		Content:    msg.Content,
		Name:       msg.Name,
		ToolCalls:  msg.ToolCalls,
		ToolCallID: msg.ToolCallID,
		Metadata:   msg.Metadata,
		Timestamp:  msg.Timestamp,
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
// because api.ToolCall is now a type alias for types.ToolCall — no conversion needed.

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

// convertTypesImagesToAPI 将 types.ImageContent 切片转换为 api.ImageContent 切片。
func convertTypesImagesToAPI(images []types.ImageContent) []api.ImageContent {
	if len(images) == 0 {
		return nil
	}
	result := make([]api.ImageContent, len(images))
	for i, img := range images {
		result[i] = api.ImageContent{
			Type: img.Type,
			URL:  img.URL,
			Data: img.Data,
		}
	}
	return result
}

// =============================================================================
// 🏥 HealthStatus 转换辅助函数
// =============================================================================

// ConvertHealthStatus 将 llm.HealthStatus 转换为 api.ProviderHealthResponse。
// 主要处理 Latency 的 time.Duration -> string 格式化。
func ConvertHealthStatus(hs *llm.HealthStatus) *api.ProviderHealthResponse {
	if hs == nil {
		return nil
	}
	return &api.ProviderHealthResponse{
		Healthy:   hs.Healthy,
		Latency:   hs.Latency.String(),
		ErrorRate: hs.ErrorRate,
		Message:   hs.Message,
	}
}
