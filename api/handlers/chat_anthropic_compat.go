package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/types"
)

type anthropicCompatMessagesRequest struct {
	Model          string                          `json:"model"`
	MaxTokens      int                             `json:"max_tokens"`
	Messages       []anthropicCompatInboundMessage `json:"messages"`
	System         any                             `json:"system,omitempty"`
	Temperature    *float32                        `json:"temperature,omitempty"`
	TopP           *float32                        `json:"top_p,omitempty"`
	TopK           *int                            `json:"top_k,omitempty"`
	StopSequences  []string                        `json:"stop_sequences,omitempty"`
	Tools          []anthropicCompatInboundTool    `json:"tools,omitempty"`
	ToolChoice     any                             `json:"tool_choice,omitempty"`
	Stream         bool                            `json:"stream,omitempty"`
	Metadata       *anthropicCompatMetadata        `json:"metadata,omitempty"`
	Thinking       *anthropicCompatThinking        `json:"thinking,omitempty"`
	ServiceTier    *string                         `json:"service_tier,omitempty"`
	InferenceSpeed string                          `json:"inference_speed,omitempty"`
}

type anthropicCompatMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

type anthropicCompatThinking struct {
	Type         string `json:"type,omitempty"`
	BudgetTokens *int   `json:"budget_tokens,omitempty"`
	Display      string `json:"display,omitempty"`
}

type anthropicCompatInboundMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicCompatInboundTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema,omitempty"`
}

type anthropicCompatMessageResponse struct {
	ID           string                        `json:"id"`
	Type         string                        `json:"type"`
	Role         string                        `json:"role"`
	Content      []anthropicCompatContentBlock `json:"content"`
	Model        string                        `json:"model"`
	StopReason   string                        `json:"stop_reason,omitempty"`
	StopSequence *string                       `json:"stop_sequence,omitempty"`
	Usage        anthropicCompatUsage          `json:"usage"`
}

type anthropicCompatUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicCompatContentBlock struct {
	Type      string                      `json:"type"`
	Text      string                      `json:"text,omitempty"`
	ID        string                      `json:"id,omitempty"`
	Name      string                      `json:"name,omitempty"`
	Input     any                         `json:"input,omitempty"`
	Thinking  string                      `json:"thinking,omitempty"`
	Signature string                      `json:"signature,omitempty"`
	Source    *anthropicCompatImageSource `json:"source,omitempty"`
}

type anthropicCompatImageSource struct {
	Type      string `json:"type,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type anthropicCompatErrorEnvelope struct {
	Type  string               `json:"type"`
	Error anthropicCompatError `json:"error"`
}

type anthropicCompatError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (h *ChatHandler) HandleAnthropicCompatMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAnthropicCompatError(w, types.NewError(types.ErrInvalidRequest, "method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed))
		return
	}

	service, svcErr := h.currentServiceOrUnavailable("chat")
	if svcErr != nil {
		writeAnthropicCompatError(w, svcErr)
		return
	}

	var req anthropicCompatMessagesRequest
	if err := decodeOpenAICompatJSON(w, r, &req); err != nil {
		writeAnthropicCompatError(w, err)
		return
	}

	apiReq, err := buildAPIChatRequestFromAnthropicMessages(req)
	if err != nil {
		writeAnthropicCompatError(w, err)
		return
	}
	if err := h.validateChatRequest(apiReq); err != nil {
		writeAnthropicCompatError(w, err)
		return
	}

	if req.Stream {
		h.handleAnthropicCompatMessagesStream(w, r, apiReq)
		return
	}

	result, svcErr := service.Complete(r.Context(), h.converter.ToUsecaseRequest(apiReq))
	if svcErr != nil {
		writeAnthropicCompatError(w, svcErr)
		return
	}

	out := toAnthropicCompatMessageResponse(h.converter.ToAPIResponseFromUsecase(result.Response))
	writeAnthropicCompatJSON(w, http.StatusOK, out)
}

func (h *ChatHandler) handleAnthropicCompatMessagesStream(w http.ResponseWriter, r *http.Request, req *api.ChatRequest) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAnthropicCompatError(w, types.NewInternalError("streaming not supported"))
		return
	}

	service, svcErr := h.currentServiceOrUnavailable("chat")
	if svcErr != nil {
		writeAnthropicCompatError(w, svcErr)
		return
	}
	stream, err := service.Stream(r.Context(), h.converter.ToUsecaseRequest(req))
	if err != nil {
		writeAnthropicCompatError(w, err)
		return
	}

	messageID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	model := req.Model
	_ = writeSSEEventJSON(w, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":      messageID,
			"type":    "message",
			"role":    "assistant",
			"content": []any{},
			"model":   model,
		},
	})
	flusher.Flush()

	textBlockStarted := false
	const textBlockIndex = 0
	nextBlockIndex := 1

	for item := range stream {
		if item.Err != nil {
			_ = writeSSEEventJSON(w, "error", anthropicCompatErrorEnvelope{
				Type: "error",
				Error: anthropicCompatError{
					Type:    anthropicCompatErrorType(item.Err),
					Message: item.Err.Message,
				},
			})
			flusher.Flush()
			return
		}
		if item.Chunk == nil {
			continue
		}
		chunk := item.Chunk
		if strings.TrimSpace(chunk.Model) != "" {
			model = chunk.Model
		}

		if content := chunk.Delta.Content; strings.TrimSpace(content) != "" {
			if !textBlockStarted {
				_ = writeSSEEventJSON(w, "content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": textBlockIndex,
					"content_block": map[string]any{
						"type": "text",
						"text": "",
					},
				})
				textBlockStarted = true
			}
			_ = writeSSEEventJSON(w, "content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": textBlockIndex,
				"delta": map[string]any{
					"type": "text_delta",
					"text": content,
				},
			})
		}

		for _, call := range chunk.Delta.ToolCalls {
			index := nextBlockIndex
			nextBlockIndex++
			callID := firstNonEmptyString(strings.TrimSpace(call.ID), fmt.Sprintf("toolu_%d", index))
			_ = writeSSEEventJSON(w, "content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": index,
				"content_block": map[string]any{
					"type":  "tool_use",
					"id":    callID,
					"name":  call.Name,
					"input": map[string]any{},
				},
			})
			if partial := anthropicCompatToolInputDelta(call); partial != "" {
				_ = writeSSEEventJSON(w, "content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": index,
					"delta": map[string]any{
						"type":         "input_json_delta",
						"partial_json": partial,
					},
				})
			}
			_ = writeSSEEventJSON(w, "content_block_stop", map[string]any{
				"type":  "content_block_stop",
				"index": index,
			})
		}

		if strings.TrimSpace(chunk.FinishReason) != "" || chunk.Usage != nil {
			if textBlockStarted {
				_ = writeSSEEventJSON(w, "content_block_stop", map[string]any{
					"type":  "content_block_stop",
					"index": textBlockIndex,
				})
				textBlockStarted = false
			}
			payload := map[string]any{
				"type": "message_delta",
				"delta": map[string]any{
					"stop_reason":   anthropicCompatStopReason(chunk.FinishReason),
					"stop_sequence": nil,
				},
			}
			if chunk.Usage != nil {
				payload["usage"] = map[string]any{
					"input_tokens":  chunk.Usage.PromptTokens,
					"output_tokens": chunk.Usage.CompletionTokens,
				}
			}
			_ = writeSSEEventJSON(w, "message_delta", payload)
		}
		flusher.Flush()
	}

	if textBlockStarted {
		_ = writeSSEEventJSON(w, "content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": textBlockIndex,
		})
	}
	_ = writeSSEEventJSON(w, "message_stop", map[string]any{
		"type":  "message_stop",
		"id":    messageID,
		"model": model,
	})
	flusher.Flush()
}

func buildAPIChatRequestFromAnthropicMessages(req anthropicCompatMessagesRequest) (*api.ChatRequest, *types.Error) {
	if req.MaxTokens <= 0 {
		return nil, types.NewInvalidRequestError("max_tokens is required and must be greater than 0")
	}

	systemMessages, err := convertAnthropicCompatSystem(req.System)
	if err != nil {
		return nil, err
	}
	inboundMessages, err := convertAnthropicCompatInboundMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	tools, err := convertAnthropicCompatInboundTools(req.Tools)
	if err != nil {
		return nil, err
	}

	temperature := float32(0)
	if req.Temperature != nil {
		temperature = *req.Temperature
	}
	topP := float32(0)
	if req.TopP != nil {
		topP = *req.TopP
	}

	metadata := make(map[string]string)
	if req.TopK != nil && *req.TopK > 0 {
		metadata["anthropic_top_k"] = fmt.Sprintf("%d", *req.TopK)
	}
	reasoningDisplay := ""
	if req.Thinking != nil {
		if mode := strings.ToLower(strings.TrimSpace(req.Thinking.Type)); mode != "" {
			metadata["reasoning_mode"] = mode
		}
		if req.Thinking.BudgetTokens != nil && *req.Thinking.BudgetTokens > 0 {
			metadata["anthropic_thinking_budget_tokens"] = fmt.Sprintf("%d", *req.Thinking.BudgetTokens)
		}
		reasoningDisplay = strings.TrimSpace(req.Thinking.Display)
	}
	if len(metadata) == 0 {
		metadata = nil
	}

	user := ""
	if req.Metadata != nil {
		user = strings.TrimSpace(req.Metadata.UserID)
	}

	return &api.ChatRequest{
		Model:            req.Model,
		Messages:         append(systemMessages, inboundMessages...),
		MaxTokens:        req.MaxTokens,
		Temperature:      temperature,
		TopP:             topP,
		Stop:             append([]string(nil), req.StopSequences...),
		Tools:            tools,
		ToolChoice:       req.ToolChoice,
		User:             user,
		ReasoningDisplay: reasoningDisplay,
		InferenceSpeed:   strings.TrimSpace(req.InferenceSpeed),
		ServiceTier:      req.ServiceTier,
		Metadata:         metadata,
	}, nil
}

func convertAnthropicCompatSystem(raw any) ([]api.Message, *types.Error) {
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		return []api.Message{{Role: string(types.RoleSystem), Content: v}}, nil
	case map[string]any:
		return convertAnthropicCompatSystemBlocks([]any{v})
	case []any:
		return convertAnthropicCompatSystemBlocks(v)
	default:
		return nil, types.NewInvalidRequestError("system must be a string or array of text blocks")
	}
}

func convertAnthropicCompatSystemBlocks(blocks []any) ([]api.Message, *types.Error) {
	out := make([]api.Message, 0, len(blocks))
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			return nil, types.NewInvalidRequestError("system blocks must be objects")
		}
		blockType := strings.ToLower(strings.TrimSpace(stringValue(block["type"])))
		if blockType == "" {
			blockType = "text"
		}
		if blockType != "text" {
			return nil, types.NewInvalidRequestError("system only supports text blocks")
		}
		text := stringValue(block["text"])
		if strings.TrimSpace(text) == "" {
			continue
		}
		out = append(out, api.Message{Role: string(types.RoleSystem), Content: text})
	}
	return out, nil
}

func convertAnthropicCompatInboundMessages(in []anthropicCompatInboundMessage) ([]api.Message, *types.Error) {
	out := make([]api.Message, 0, len(in))
	for i, msg := range in {
		converted, err := convertAnthropicCompatInboundMessage(msg, i)
		if err != nil {
			return nil, err
		}
		out = append(out, converted...)
	}
	return out, nil
}

func convertAnthropicCompatInboundMessage(msg anthropicCompatInboundMessage, index int) ([]api.Message, *types.Error) {
	role := strings.ToLower(strings.TrimSpace(msg.Role))
	switch role {
	case string(types.RoleUser), string(types.RoleAssistant), string(types.RoleTool), string(types.RoleSystem), string(types.RoleDeveloper):
	default:
		return nil, types.NewInvalidRequestError(fmt.Sprintf("messages[%d].role is invalid", index))
	}

	blocks, err := anthropicCompatContentAsBlocks(msg.Content)
	if err != nil {
		return nil, types.NewInvalidRequestError(fmt.Sprintf("messages[%d].content is invalid", index))
	}

	current := api.Message{Role: role}
	var reasoningParts []string
	var toolMessages []api.Message

	for blockIndex, block := range blocks {
		blockType := strings.ToLower(strings.TrimSpace(stringValue(block["type"])))
		switch blockType {
		case "", "text":
			appendAnthropicCompatText(&current.Content, stringValue(block["text"]))
		case "image":
			image, ok := anthropicCompatImageFromBlock(block)
			if !ok {
				return nil, types.NewInvalidRequestError(fmt.Sprintf("messages[%d].content[%d].image is invalid", index, blockIndex))
			}
			current.Images = append(current.Images, image)
		case "tool_use":
			current.ToolCalls = append(current.ToolCalls, types.ToolCall{
				ID:        strings.TrimSpace(stringValue(block["id"])),
				Type:      types.ToolTypeFunction,
				Name:      strings.TrimSpace(stringValue(block["name"])),
				Arguments: normalizeAnthropicCompatJSONValue(block["input"]),
			})
		case "tool_result":
			toolMessages = append(toolMessages, api.Message{
				Role:        string(types.RoleTool),
				Content:     anthropicCompatStringifyValue(block["content"]),
				ToolCallID:  strings.TrimSpace(stringValue(block["tool_use_id"])),
				IsToolError: boolValue(block["is_error"]),
			})
		case "thinking":
			thinking := stringValue(block["thinking"])
			if strings.TrimSpace(thinking) != "" {
				reasoningParts = append(reasoningParts, thinking)
				current.ThinkingBlocks = append(current.ThinkingBlocks, types.ThinkingBlock{
					Thinking:  thinking,
					Signature: strings.TrimSpace(stringValue(block["signature"])),
				})
			}
		case "redacted_thinking":
			state := anthropicCompatStringifyValue(firstNonNil(block["data"], block["encrypted_content"]))
			if strings.TrimSpace(state) != "" {
				current.OpaqueReasoning = append(current.OpaqueReasoning, types.OpaqueReasoning{
					Provider: "anthropic",
					Kind:     "redacted_thinking",
					State:    state,
				})
			}
		default:
			return nil, types.NewInvalidRequestError(fmt.Sprintf("messages[%d].content[%d].type %q is not supported", index, blockIndex, blockType))
		}
	}

	if len(reasoningParts) > 0 {
		reasoning := strings.Join(reasoningParts, "\n\n")
		current.ReasoningContent = &reasoning
	}

	out := make([]api.Message, 0, 1+len(toolMessages))
	if anthropicCompatHasMessageContent(current) {
		out = append(out, current)
	}
	out = append(out, toolMessages...)
	if len(out) == 0 {
		return nil, types.NewInvalidRequestError(fmt.Sprintf("messages[%d].content cannot be empty", index))
	}
	return out, nil
}

func convertAnthropicCompatInboundTools(in []anthropicCompatInboundTool) ([]api.ToolSchema, *types.Error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]api.ToolSchema, 0, len(in))
	for i, tool := range in {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			return nil, types.NewInvalidRequestError(fmt.Sprintf("tools[%d].name is required", i))
		}
		out = append(out, api.ToolSchema{
			Type:        types.ToolTypeFunction,
			Name:        name,
			Description: strings.TrimSpace(tool.Description),
			Parameters:  normalizeAnthropicCompatJSONValue(tool.InputSchema),
		})
	}
	return out, nil
}

func anthropicCompatContentAsBlocks(raw any) ([]map[string]any, error) {
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case string:
		return []map[string]any{{"type": "text", "text": v}}, nil
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("content block must be object")
			}
			out = append(out, m)
		}
		return out, nil
	case map[string]any:
		return []map[string]any{v}, nil
	default:
		return nil, fmt.Errorf("unsupported content type")
	}
}

func anthropicCompatImageFromBlock(block map[string]any) (api.ImageContent, bool) {
	source, ok := block["source"].(map[string]any)
	if !ok {
		return api.ImageContent{}, false
	}
	sourceType := strings.ToLower(strings.TrimSpace(stringValue(source["type"])))
	switch sourceType {
	case "base64":
		data := strings.TrimSpace(stringValue(source["data"]))
		if data == "" {
			return api.ImageContent{}, false
		}
		return api.ImageContent{Type: "base64", Data: data}, true
	case "url":
		url := strings.TrimSpace(stringValue(source["url"]))
		if url == "" {
			return api.ImageContent{}, false
		}
		return api.ImageContent{Type: "url", URL: url}, true
	default:
		return api.ImageContent{}, false
	}
}

func anthropicCompatHasMessageContent(msg api.Message) bool {
	return strings.TrimSpace(msg.Content) != "" ||
		msg.ReasoningContent != nil ||
		len(msg.ToolCalls) > 0 ||
		len(msg.Images) > 0 ||
		len(msg.ThinkingBlocks) > 0 ||
		len(msg.OpaqueReasoning) > 0
}

func appendAnthropicCompatText(dst *string, text string) {
	if dst == nil || strings.TrimSpace(text) == "" {
		return
	}
	if strings.TrimSpace(*dst) == "" {
		*dst = text
		return
	}
	*dst += "\n\n" + text
}

func normalizeAnthropicCompatJSONValue(raw any) json.RawMessage {
	if raw == nil {
		return json.RawMessage(`{}`)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(data)
}

func anthropicCompatStringifyValue(raw any) string {
	switch v := raw.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", raw)
		}
		return string(data)
	}
}

func stringValue(raw any) string {
	switch v := raw.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprintf("%v", raw)
	}
}

func boolValue(raw any) bool {
	v, ok := raw.(bool)
	return ok && v
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func toAnthropicCompatMessageResponse(resp *api.ChatResponse) anthropicCompatMessageResponse {
	out := anthropicCompatMessageResponse{
		ID:    firstNonEmptyString(resp.ID, fmt.Sprintf("msg_%d", time.Now().UnixNano())),
		Type:  "message",
		Role:  "assistant",
		Model: resp.Model,
		Usage: anthropicCompatUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}
	if len(resp.Choices) == 0 {
		out.Content = []anthropicCompatContentBlock{{Type: "text", Text: ""}}
		return out
	}

	choice := resp.Choices[0]
	out.Role = firstNonEmptyString(strings.TrimSpace(choice.Message.Role), "assistant")
	out.StopReason = anthropicCompatStopReason(choice.FinishReason)
	out.Content = toAnthropicCompatOutboundContent(choice.Message)
	if len(out.Content) == 0 {
		out.Content = []anthropicCompatContentBlock{{Type: "text", Text: ""}}
	}
	return out
}

func toAnthropicCompatOutboundContent(msg api.Message) []anthropicCompatContentBlock {
	out := make([]anthropicCompatContentBlock, 0, len(msg.ThinkingBlocks)+1+len(msg.ToolCalls))
	for _, block := range msg.ThinkingBlocks {
		if strings.TrimSpace(block.Thinking) == "" {
			continue
		}
		out = append(out, anthropicCompatContentBlock{
			Type:      "thinking",
			Thinking:  block.Thinking,
			Signature: strings.TrimSpace(block.Signature),
		})
	}
	if len(msg.ThinkingBlocks) == 0 && msg.ReasoningContent != nil && strings.TrimSpace(*msg.ReasoningContent) != "" {
		out = append(out, anthropicCompatContentBlock{
			Type:     "thinking",
			Thinking: *msg.ReasoningContent,
		})
	}
	if strings.TrimSpace(msg.Content) != "" || len(msg.ToolCalls) == 0 {
		out = append(out, anthropicCompatContentBlock{
			Type: "text",
			Text: msg.Content,
		})
	}
	for _, call := range msg.ToolCalls {
		out = append(out, anthropicCompatContentBlock{
			Type:  "tool_use",
			ID:    firstNonEmptyString(strings.TrimSpace(call.ID), fmt.Sprintf("toolu_%d", len(out)+1)),
			Name:  call.Name,
			Input: anthropicCompatToolInput(call),
		})
	}
	return out
}

func anthropicCompatToolInput(call types.ToolCall) any {
	if len(call.Arguments) > 0 {
		var out any
		if err := json.Unmarshal(call.Arguments, &out); err == nil {
			return out
		}
		return string(call.Arguments)
	}
	if strings.TrimSpace(call.Input) == "" {
		return map[string]any{}
	}
	var out any
	if err := json.Unmarshal([]byte(call.Input), &out); err == nil {
		return out
	}
	return call.Input
}

func anthropicCompatToolInputDelta(call types.ToolCall) string {
	if len(call.Arguments) > 0 {
		return strings.TrimSpace(string(call.Arguments))
	}
	if strings.TrimSpace(call.Input) == "" {
		return ""
	}
	if json.Valid([]byte(call.Input)) {
		return strings.TrimSpace(call.Input)
	}
	data, err := json.Marshal(call.Input)
	if err != nil {
		return ""
	}
	return string(data)
}

func anthropicCompatStopReason(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "end_turn", "stop":
		return "end_turn"
	case "length", "max_tokens":
		return "max_tokens"
	case "tool_calls", "tool_use", "function_call":
		return "tool_use"
	case "stop_sequence":
		return "stop_sequence"
	default:
		return strings.TrimSpace(raw)
	}
}

func writeAnthropicCompatJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAnthropicCompatError(w http.ResponseWriter, err *types.Error) {
	if err == nil {
		err = types.NewInternalError("internal error")
	}
	status := err.HTTPStatus
	if status == 0 {
		status = mapErrorCodeToHTTPStatus(err.Code)
	}
	if status == 0 {
		status = http.StatusInternalServerError
	}
	writeAnthropicCompatJSON(w, status, anthropicCompatErrorEnvelope{
		Type: "error",
		Error: anthropicCompatError{
			Type:    anthropicCompatErrorType(err),
			Message: err.Message,
		},
	})
}

func anthropicCompatErrorType(err *types.Error) string {
	if err == nil {
		return "api_error"
	}
	switch err.Code {
	case types.ErrInvalidRequest:
		return "invalid_request_error"
	case types.ErrUnauthorized, types.ErrAuthentication:
		return "authentication_error"
	case types.ErrForbidden:
		return "permission_error"
	case types.ErrRateLimit:
		return "rate_limit_error"
	default:
		return "api_error"
	}
}
