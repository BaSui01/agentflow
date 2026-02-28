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
	provider llm.Provider
	logger   *zap.Logger
}

// NewChatHandler 创建聊天处理器
func NewChatHandler(provider llm.Provider, logger *zap.Logger) *ChatHandler {
	return &ChatHandler{
		provider: provider,
		logger:   logger,
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
// @Router /v1/chat/completions [post]
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
// @Router /v1/chat/completions/stream [post]
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
		err := types.NewError(types.ErrInternalError, "streaming not supported")
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

// validateChatRequest 验证聊天请求
func (h *ChatHandler) validateChatRequest(req *api.ChatRequest) *types.Error {
	if req.Model == "" {
		return types.NewError(types.ErrInvalidRequest, "model is required")
	}

	if len(req.Messages) == 0 {
		return types.NewError(types.ErrInvalidRequest, "messages cannot be empty")
	}

	// V-002: MaxTokens range validation
	if req.MaxTokens < 0 || req.MaxTokens > 128000 {
		return types.NewError(types.ErrInvalidRequest, "max_tokens must be between 0 and 128000")
	}

	// 验证温度参数
	if req.Temperature < 0 || req.Temperature > 2 {
		return types.NewError(types.ErrInvalidRequest, "temperature must be between 0 and 2")
	}

	// 验证 TopP 参数
	if req.TopP < 0 || req.TopP > 1 {
		return types.NewError(types.ErrInvalidRequest, "top_p must be between 0 and 1")
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
	// 解析超时
	timeout := defaultStreamTimeout
	if req.Timeout != "" {
		if d, err := time.ParseDuration(req.Timeout); err == nil {
			timeout = d
		}
	}

	// 转换 Messages（api.Message -> types.Message）
	messages := make([]types.Message, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = types.Message{
			Role:       types.Role(msg.Role),
			Content:    msg.Content,
			Name:       msg.Name,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
			Images:     convertAPIImagesToTypes(msg.Images),
			Metadata:   msg.Metadata,
			Timestamp:  msg.Timestamp,
		}
	}

	// 转换 Tools（api.ToolSchema -> types.ToolSchema）
	tools := make([]types.ToolSchema, len(req.Tools))
	for i, tool := range req.Tools {
		tools[i] = types.ToolSchema{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		}
	}

	return &llm.ChatRequest{
		TraceID:     req.TraceID,
		TenantID:    req.TenantID,
		UserID:      req.UserID,
		Model:       req.Model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
		Tools:       tools,
		ToolChoice:  req.ToolChoice,
		Timeout:     timeout,
		Metadata:    req.Metadata,
		Tags:        req.Tags,
	}
}

// convertToAPIResponse 转换为 API 响应
func (h *ChatHandler) convertToAPIResponse(resp *llm.ChatResponse) *api.ChatResponse {
	return &api.ChatResponse{
		ID:        resp.ID,
		Provider:  resp.Provider,
		Model:     resp.Model,
		Choices:   h.convertChoices(resp.Choices),
		Usage:     h.convertUsage(resp.Usage),
		CreatedAt: resp.CreatedAt,
	}
}

// convertChoices 转换选择列表
func (h *ChatHandler) convertChoices(choices []llm.ChatChoice) []api.ChatChoice {
	result := make([]api.ChatChoice, len(choices))
	for i, choice := range choices {
		result[i] = api.ChatChoice{
			Index:        choice.Index,
			FinishReason: choice.FinishReason,
			Message: api.Message{
				Role:       string(choice.Message.Role),
				Content:    choice.Message.Content,
				Name:       choice.Message.Name,
				ToolCalls:  choice.Message.ToolCalls,
				ToolCallID: choice.Message.ToolCallID,
				Images:     convertTypesImagesToAPI(choice.Message.Images),
				Metadata:   choice.Message.Metadata,
				Timestamp:  choice.Message.Timestamp,
			},
		}
	}
	return result
}

// convertUsage 转换使用统计
func (h *ChatHandler) convertUsage(usage llm.ChatUsage) api.ChatUsage {
	return api.ChatUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
}

// convertToAPIStreamChunk 转换流式块
func (h *ChatHandler) convertToAPIStreamChunk(chunk *llm.StreamChunk) *api.StreamChunk {
	result := &api.StreamChunk{
		ID:       chunk.ID,
		Provider: chunk.Provider,
		Model:    chunk.Model,
		Index:    chunk.Index,
		Delta: api.Message{
			Role:       string(chunk.Delta.Role),
			Content:    chunk.Delta.Content,
			Name:       chunk.Delta.Name,
			ToolCalls:  chunk.Delta.ToolCalls,
			ToolCallID: chunk.Delta.ToolCallID,
			Images:     convertTypesImagesToAPI(chunk.Delta.Images),
			Metadata:   chunk.Delta.Metadata,
			Timestamp:  chunk.Delta.Timestamp,
		},
		FinishReason: chunk.FinishReason,
		Usage:        convertStreamUsage(chunk.Usage),
	}

	// 映射 Err -> Error（注意：流式场景中 Err 通常已在调用方作为 SSE error 事件处理，
	// 但如果 chunk 携带了非致命错误信息，仍需映射到 API 层的 ErrorDetail）
	if chunk.Err != nil {
		result.Error = &api.ErrorDetail{
			Code:      string(chunk.Err.Code),
			Message:   chunk.Err.Message,
			Retryable: chunk.Err.Retryable,
			Provider:  chunk.Err.Provider,
		}
	}

	return result
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
	internalErr := types.NewError(types.ErrInternalError, "provider error").
		WithCause(err).
		WithRetryable(false)

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
