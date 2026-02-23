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
	h.logger.Info("chat completion",
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
		err := types.NewError(types.ErrInternalError, "streaming not supported")
		WriteError(w, err, h.logger)
		return
	}

	for chunk := range stream {
		if chunk.Err != nil {
			h.logger.Error("stream error", zap.Error(chunk.Err))
			// SSE 错误事件 — 使用 json.Marshal 转义错误消息，防止 JSON 注入
			errPayload, _ := json.Marshal(map[string]string{"error": chunk.Err.Message})
			w.Write([]byte("event: error\n"))
			w.Write([]byte("data: "))
			w.Write(errPayload)
			w.Write([]byte("\n\n"))
			flusher.Flush()
			return
		}

		// 转换为 API 格式
		apiChunk := h.convertToAPIStreamChunk(&chunk)

		// 发送 SSE 事件
		w.Write([]byte("data: "))
		if err := writeJSON(w, apiChunk); err != nil {
			h.logger.Error("failed to write chunk", zap.Error(err))
			return
		}
		w.Write([]byte("\n\n"))
		flusher.Flush()
	}

	// 发送结束标记
	w.Write([]byte("data: [DONE]\n\n"))
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
		return types.NewError(types.ErrInvalidRequest, "model is required")
	}

	if len(req.Messages) == 0 {
		return types.NewError(types.ErrInvalidRequest, "messages cannot be empty")
	}

	// 验证 max_tokens 参数
	if req.MaxTokens < 0 {
		return types.NewError(types.ErrInvalidRequest, "max_tokens must be non-negative")
	}

	// 验证温度参数
	if req.Temperature < 0 || req.Temperature > 2 {
		return types.NewError(types.ErrInvalidRequest, "temperature must be between 0 and 2")
	}

	// 验证 TopP 参数
	if req.TopP < 0 || req.TopP > 1 {
		return types.NewError(types.ErrInvalidRequest, "top_p must be between 0 and 1")
	}

	// 验证每条消息的 role
	for i, msg := range req.Messages {
		if !ValidateEnum(msg.Role, allowedMessageRoles) {
			return types.NewError(types.ErrInvalidRequest,
				fmt.Sprintf("messages[%d].role must be one of: system, user, assistant, tool", i))
		}
	}

	return nil
}

// convertToLLMRequest 转换为 LLM 请求
func (h *ChatHandler) convertToLLMRequest(req *api.ChatRequest) *llm.ChatRequest {
	// 解析超时
	timeout := 30 * time.Second
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
			Metadata:   msg.Metadata,
			Timestamp:  msg.Timestamp,
		}
		// 复制 Images 字段（api.ImageContent → types.ImageContent）
		if len(msg.Images) > 0 {
			images := make([]types.ImageContent, len(msg.Images))
			for j, img := range msg.Images {
				images[j] = types.ImageContent{
					Type: img.Type,
					URL:  img.URL,
					Data: img.Data,
				}
			}
			messages[i].Images = images
		}
	}

	// 转换 Tools（api.ToolSchema -> types.ToolSchema）
	tools := make([]types.ToolSchema, len(req.Tools))
	for i, tool := range req.Tools {
		tools[i] = types.ToolSchema{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
			Version:     tool.Version,
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
			Message:      convertTypesMessageToAPI(choice.Message),
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
	return &api.StreamChunk{
		ID:           chunk.ID,
		Provider:     chunk.Provider,
		Model:        chunk.Model,
		Index:        chunk.Index,
		Delta:        convertTypesMessageToAPI(chunk.Delta),
		FinishReason: chunk.FinishReason,
		Usage:        convertStreamUsage(chunk.Usage),
	}
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
