// Package handlers: OpenAI 兼容层
//
// 设计说明（C-005/C-006）：OpenAI 兼容端点（/v1/chat/completions、/v1/responses）与自有接口
// 使用两套错误/响应格式，此为有意设计：
// - OpenAI 兼容：writeOpenAICompatError / writeOpenAICompatJSON，输出 OpenAI 规范格式（error.type/code/message）
// - 自有接口：WriteError / WriteSuccess，输出 api.Response 统一格式（success/error/data）
// 两套格式分别满足不同客户端契约，不做统一。
//
// TODO(C-002): OpenAI 兼容错误格式与主 API 的 api.ErrorInfo 不一致，
// 需要在文档中说明或增加适配层统一。
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

type openAICompatChatCompletionsRequest struct {
	Model               string                        `json:"model"`
	Messages            []openAICompatInboundMessage  `json:"messages"`
	MaxTokens           int                           `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int                          `json:"max_completion_tokens,omitempty"`
	Temperature         float32                       `json:"temperature,omitempty"`
	TopP                float32                       `json:"top_p,omitempty"`
	FrequencyPenalty    *float32                      `json:"frequency_penalty,omitempty"`
	PresencePenalty     *float32                      `json:"presence_penalty,omitempty"`
	RepetitionPenalty   *float32                      `json:"repetition_penalty,omitempty"`
	N                   *int                          `json:"n,omitempty"`
	LogProbs            *bool                         `json:"logprobs,omitempty"`
	TopLogProbs         *int                          `json:"top_logprobs,omitempty"`
	Stop                []string                      `json:"stop,omitempty"`
	Tools               []openAICompatInboundTool     `json:"tools,omitempty"`
	ToolChoice          any                           `json:"tool_choice,omitempty"`
	ResponseFormat      any                           `json:"response_format,omitempty"`
	Stream              bool                          `json:"stream,omitempty"`
	StreamOptions       *api.StreamOptions            `json:"stream_options,omitempty"`
	ParallelToolCalls   *bool                         `json:"parallel_tool_calls,omitempty"`
	ServiceTier         *string                       `json:"service_tier,omitempty"`
	User                string                        `json:"user,omitempty"`
	ReasoningEffort     string                        `json:"reasoning_effort,omitempty"`
	Store               *bool                         `json:"store,omitempty"`
	Modalities          []string                      `json:"modalities,omitempty"`
	WebSearchOptions    *openAICompatWebSearchOptions `json:"web_search_options,omitempty"`
	PreviousResponseID  string                        `json:"previous_response_id,omitempty"`
	Include             []string                      `json:"include,omitempty"`
	Truncation          string                        `json:"truncation,omitempty"`
	Provider            string                        `json:"provider,omitempty"`
	RoutePolicy         string                        `json:"route_policy,omitempty"`
	EndpointMode        string                        `json:"endpoint_mode,omitempty"`
	Timeout             string                        `json:"timeout,omitempty"`
	Metadata            map[string]string             `json:"metadata,omitempty"`
	Tags                []string                      `json:"tags,omitempty"`
}

type openAICompatResponsesRequest struct {
	Model              string                          `json:"model"`
	Input              any                             `json:"input"`
	Instructions       string                          `json:"instructions,omitempty"`
	MaxOutputTokens    *int                            `json:"max_output_tokens,omitempty"`
	Temperature        *float32                        `json:"temperature,omitempty"`
	TopP               *float32                        `json:"top_p,omitempty"`
	Tools              []openAICompatInboundTool       `json:"tools,omitempty"`
	ToolChoice         any                             `json:"tool_choice,omitempty"`
	Reasoning          *openAICompatResponsesReasoning `json:"reasoning,omitempty"`
	Text               *openAICompatResponsesTextParam `json:"text,omitempty"`
	ParallelToolCalls  *bool                           `json:"parallel_tool_calls,omitempty"`
	ServiceTier        *string                         `json:"service_tier,omitempty"`
	User               string                          `json:"user,omitempty"`
	Store              *bool                           `json:"store,omitempty"`
	WebSearchOptions   *openAICompatWebSearchOptions   `json:"web_search_options,omitempty"`
	PreviousResponseID string                          `json:"previous_response_id,omitempty"`
	Include            []string                        `json:"include,omitempty"`
	Truncation         string                          `json:"truncation,omitempty"`
	Stream             bool                            `json:"stream,omitempty"`
	Provider           string                          `json:"provider,omitempty"`
	RoutePolicy        string                          `json:"route_policy,omitempty"`
	EndpointMode       string                          `json:"endpoint_mode,omitempty"`
	Timeout            string                          `json:"timeout,omitempty"`
	Metadata           map[string]string               `json:"metadata,omitempty"`
	Tags               []string                        `json:"tags,omitempty"`
}

type openAICompatInboundMessage struct {
	Role       string                    `json:"role"`
	Content    any                       `json:"content,omitempty"`
	Name       string                    `json:"name,omitempty"`
	ToolCallID string                    `json:"tool_call_id,omitempty"`
	ToolCalls  []openAICompatInboundCall `json:"tool_calls,omitempty"`
}

type openAICompatInboundCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

type openAICompatInboundTool struct {
	Type        string `json:"type,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
	Function    struct {
		Name        string `json:"name,omitempty"`
		Description string `json:"description,omitempty"`
		Parameters  any    `json:"parameters,omitempty"`
	} `json:"function,omitempty"`
	SearchContextSize string                        `json:"search_context_size,omitempty"`
	UserLocation      json.RawMessage               `json:"user_location,omitempty"`
	Filters           *openAICompatWebSearchFilters `json:"filters,omitempty"`
}

type openAICompatResponsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

type openAICompatResponsesTextParam struct {
	Format any `json:"format,omitempty"`
}

type openAICompatWebSearchOptions struct {
	SearchContextSize string          `json:"search_context_size,omitempty"`
	UserLocation      json.RawMessage `json:"user_location,omitempty"`
	AllowedDomains    []string        `json:"allowed_domains,omitempty"`
	BlockedDomains    []string        `json:"blocked_domains,omitempty"`
	MaxUses           int             `json:"max_uses,omitempty"`
}

type openAICompatWebSearchFilters struct {
	AllowedDomains []string `json:"allowed_domains,omitempty"`
}

type openAICompatChatResponse struct {
	ID      string                   `json:"id"`
	Object  string                   `json:"object"`
	Created int64                    `json:"created"`
	Model   string                   `json:"model"`
	Choices []openAICompatChatChoice `json:"choices"`
	Usage   openAICompatChatUsage    `json:"usage"`
}

type openAICompatChatChoice struct {
	Index        int                     `json:"index"`
	Message      openAICompatOutboundMsg `json:"message"`
	FinishReason string                  `json:"finish_reason,omitempty"`
}

type openAICompatChatChunkResponse struct {
	ID      string                        `json:"id"`
	Object  string                        `json:"object"`
	Created int64                         `json:"created"`
	Model   string                        `json:"model"`
	Choices []openAICompatChatChunkChoice `json:"choices"`
}

type openAICompatChatChunkChoice struct {
	Index        int                     `json:"index"`
	Delta        openAICompatOutboundMsg `json:"delta"`
	FinishReason any                     `json:"finish_reason,omitempty"`
}

type openAICompatOutboundMsg struct {
	Role             string                         `json:"role,omitempty"`
	Content          string                         `json:"content,omitempty"`
	ReasoningContent *string                        `json:"reasoning_content,omitempty"`
	Refusal          *string                        `json:"refusal,omitempty"`
	Name             string                         `json:"name,omitempty"`
	ToolCallID       string                         `json:"tool_call_id,omitempty"`
	ToolCalls        []openAICompatOutboundToolCall `json:"tool_calls,omitempty"`
	Annotations      []openAICompatAnnotation       `json:"annotations,omitempty"`
}

type openAICompatOutboundToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAICompatAnnotation struct {
	Type        string                        `json:"type"`
	URLCitation *openAICompatURLCitationDetail `json:"url_citation,omitempty"`
}

type openAICompatURLCitationDetail struct {
	StartIndex int    `json:"start_index"`
	EndIndex   int    `json:"end_index"`
	URL        string `json:"url"`
	Title      string `json:"title"`
}

type openAICompatTokenDetails struct {
	CachedTokens             int `json:"cached_tokens,omitempty"`
	AudioTokens              int `json:"audio_tokens,omitempty"`
	ReasoningTokens          int `json:"reasoning_tokens,omitempty"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens,omitempty"`
}

type openAICompatChatUsage struct {
	PromptTokens            int                       `json:"prompt_tokens"`
	CompletionTokens        int                       `json:"completion_tokens"`
	TotalTokens             int                       `json:"total_tokens"`
	PromptTokensDetails     *openAICompatTokenDetails `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *openAICompatTokenDetails `json:"completion_tokens_details,omitempty"`
}

type openAICompatResponsesOutput struct {
	ID        string                         `json:"id,omitempty"`
	Type      string                         `json:"type"`
	Status    string                         `json:"status,omitempty"`
	Role      string                         `json:"role,omitempty"`
	Content   []openAICompatResponsesContent `json:"content,omitempty"`
	Name      string                         `json:"name,omitempty"`
	Arguments json.RawMessage                `json:"arguments,omitempty"`
	CallID    string                         `json:"call_id,omitempty"`
}

type openAICompatResponsesContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type openAICompatResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type openAICompatResponsesResponse struct {
	ID        string                        `json:"id"`
	Object    string                        `json:"object"`
	CreatedAt int64                         `json:"created_at"`
	Status    string                        `json:"status"`
	Model     string                        `json:"model"`
	Output    []openAICompatResponsesOutput `json:"output"`
	Usage     openAICompatResponsesUsage    `json:"usage"`
}

type openAICompatErrorEnvelope struct {
	Error openAICompatError `json:"error"`
}

type openAICompatError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

func (h *ChatHandler) HandleOpenAICompatChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpenAICompatError(w, types.NewError(types.ErrInvalidRequest, "method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed))
		return
	}
	if h.service == nil {
		writeOpenAICompatError(w, types.NewInternalError("chat service is not configured"))
		return
	}

	var req openAICompatChatCompletionsRequest
	if err := decodeOpenAICompatJSON(w, r, &req); err != nil {
		writeOpenAICompatError(w, err)
		return
	}

	apiReq, err := buildAPIChatRequestFromCompatCompletions(req)
	if err != nil {
		writeOpenAICompatError(w, err)
		return
	}
	if err := h.validateChatRequest(apiReq); err != nil {
		writeOpenAICompatError(w, err)
		return
	}

	if req.Stream {
		h.handleOpenAICompatChatCompletionsStream(w, r, apiReq)
		return
	}

	result, svcErr := h.service.Complete(r.Context(), apiReq)
	if svcErr != nil {
		writeOpenAICompatError(w, svcErr)
		return
	}
	out := toOpenAICompatChatResponse(result.Response)
	writeOpenAICompatJSON(w, http.StatusOK, out)
}

func (h *ChatHandler) HandleOpenAICompatResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpenAICompatError(w, types.NewError(types.ErrInvalidRequest, "method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed))
		return
	}
	if h.service == nil {
		writeOpenAICompatError(w, types.NewInternalError("chat service is not configured"))
		return
	}

	var req openAICompatResponsesRequest
	if err := decodeOpenAICompatJSON(w, r, &req); err != nil {
		writeOpenAICompatError(w, err)
		return
	}

	apiReq, err := buildAPIChatRequestFromCompatResponses(req)
	if err != nil {
		writeOpenAICompatError(w, err)
		return
	}
	if err := h.validateChatRequest(apiReq); err != nil {
		writeOpenAICompatError(w, err)
		return
	}

	if req.Stream {
		h.handleOpenAICompatResponsesStream(w, r, apiReq)
		return
	}

	result, svcErr := h.service.Complete(r.Context(), apiReq)
	if svcErr != nil {
		writeOpenAICompatError(w, svcErr)
		return
	}
	out := toOpenAICompatResponsesResponse(result.Response)
	writeOpenAICompatJSON(w, http.StatusOK, out)
}

func (h *ChatHandler) handleOpenAICompatChatCompletionsStream(w http.ResponseWriter, r *http.Request, req *api.ChatRequest) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOpenAICompatError(w, types.NewInternalError("streaming not supported"))
		return
	}

	stream, err := h.service.Stream(r.Context(), req)
	if err != nil {
		writeOpenAICompatError(w, err)
		return
	}

	created := time.Now().Unix()
	model := req.Model
	for chunk := range stream {
		if chunk.Err != nil {
			_ = writeSSEJSON(w, openAICompatErrorEnvelope{
				Error: openAICompatError{
					Message: chunk.Err.Message,
					Type:    "server_error",
					Code:    string(chunk.Err.Code),
				},
			})
			_ = writeSSE(w, []byte("data: [DONE]\n\n"))
			flusher.Flush()
			return
		}
		llmChunk, ok := chunk.Output.(*llm.StreamChunk)
		if !ok || llmChunk == nil {
			continue
		}
		if strings.TrimSpace(llmChunk.Model) != "" {
			model = llmChunk.Model
		}
		payload := toOpenAICompatChatChunkResponse(llmChunk, created, model)
		if err := writeSSEJSON(w, payload); err != nil {
			return
		}
		flusher.Flush()
	}

	_ = writeSSE(w, []byte("data: [DONE]\n\n"))
	flusher.Flush()
}

func (h *ChatHandler) handleOpenAICompatResponsesStream(w http.ResponseWriter, r *http.Request, req *api.ChatRequest) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOpenAICompatError(w, types.NewInternalError("streaming not supported"))
		return
	}

	stream, err := h.service.Stream(r.Context(), req)
	if err != nil {
		writeOpenAICompatError(w, err)
		return
	}

	responseID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
	model := req.Model
	createdEvent := map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":    responseID,
			"model": model,
		},
	}
	_ = writeSSEEventJSON(w, "response.created", createdEvent)
	flusher.Flush()

	for chunk := range stream {
		if chunk.Err != nil {
			_ = writeSSEEventJSON(w, "error", map[string]any{
				"type":    "error",
				"code":    string(chunk.Err.Code),
				"message": chunk.Err.Message,
			})
			_ = writeSSE(w, []byte("data: [DONE]\n\n"))
			flusher.Flush()
			return
		}
		llmChunk, ok := chunk.Output.(*llm.StreamChunk)
		if !ok || llmChunk == nil {
			continue
		}
		if strings.TrimSpace(llmChunk.Model) != "" {
			model = llmChunk.Model
		}
		for _, ev := range toOpenAICompatResponsesStreamEvents(llmChunk) {
			if err := writeSSEEventJSON(w, ev.name, ev.payload); err != nil {
				return
			}
		}
		flusher.Flush()
	}

	completedEvent := map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id":    responseID,
			"model": model,
		},
	}
	_ = writeSSEEventJSON(w, "response.completed", completedEvent)
	_ = writeSSE(w, []byte("data: [DONE]\n\n"))
	flusher.Flush()
}

type openAICompatResponsesStreamEvent struct {
	name    string
	payload any
}

func toOpenAICompatResponsesStreamEvents(chunk *llm.StreamChunk) []openAICompatResponsesStreamEvent {
	if chunk == nil {
		return nil
	}
	events := make([]openAICompatResponsesStreamEvent, 0, 4)
	content := strings.TrimSpace(chunk.Delta.Content)
	if content != "" {
		events = append(events, openAICompatResponsesStreamEvent{
			name: "response.output_text.delta",
			payload: map[string]any{
				"type":  "response.output_text.delta",
				"delta": content,
			},
		})
	}
	for _, call := range chunk.Delta.ToolCalls {
		itemID := strings.TrimSpace(call.ID)
		if itemID == "" {
			itemID = fmt.Sprintf("fc_%d", time.Now().UnixNano())
		}
		events = append(events, openAICompatResponsesStreamEvent{
			name: "response.function_call_arguments.delta",
			payload: map[string]any{
				"type":    "response.function_call_arguments.delta",
				"item_id": itemID,
				"name":    call.Name,
				"delta":   string(call.Arguments),
			},
		})
		events = append(events, openAICompatResponsesStreamEvent{
			name: "response.function_call_arguments.done",
			payload: map[string]any{
				"type":    "response.function_call_arguments.done",
				"item_id": itemID,
			},
		})
	}
	if chunk.FinishReason == "stop" {
		events = append(events, openAICompatResponsesStreamEvent{
			name: "response.output_text.done",
			payload: map[string]any{
				"type": "response.output_text.done",
			},
		})
	}
	return events
}

func buildAPIChatRequestFromCompatCompletions(req openAICompatChatCompletionsRequest) (*api.ChatRequest, *types.Error) {
	messages, err := convertOpenAICompatInboundMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	tools, wsOptionsFromTools, err := convertOpenAICompatInboundTools(req.Tools)
	if err != nil {
		return nil, err
	}
	responseFormat, err := convertOpenAICompatResponseFormat(req.ResponseFormat)
	if err != nil {
		return nil, err
	}
	wsOptions := mergeOpenAICompatWebSearchOptions(req.WebSearchOptions, wsOptionsFromTools)

	apiReq := &api.ChatRequest{
		Model:               req.Model,
		Messages:            messages,
		MaxTokens:           req.MaxTokens,
		MaxCompletionTokens: req.MaxCompletionTokens,
		Temperature:         req.Temperature,
		TopP:                req.TopP,
		FrequencyPenalty:    req.FrequencyPenalty,
		PresencePenalty:     req.PresencePenalty,
		RepetitionPenalty:   req.RepetitionPenalty,
		N:                   req.N,
		LogProbs:            req.LogProbs,
		TopLogProbs:         req.TopLogProbs,
		Stop:                req.Stop,
		Tools:               tools,
		ToolChoice:          req.ToolChoice,
		ResponseFormat:      responseFormat,
		StreamOptions:       req.StreamOptions,
		ParallelToolCalls:   req.ParallelToolCalls,
		ServiceTier:         req.ServiceTier,
		User:                req.User,
		ReasoningEffort:     strings.TrimSpace(req.ReasoningEffort),
		Store:               req.Store,
		Modalities:          req.Modalities,
		PreviousResponseID:  strings.TrimSpace(req.PreviousResponseID),
		Include:             req.Include,
		Truncation:          strings.TrimSpace(req.Truncation),
		Provider:            req.Provider,
		RoutePolicy:         req.RoutePolicy,
		EndpointMode:        req.EndpointMode,
		Timeout:             req.Timeout,
		Metadata:            req.Metadata,
		Tags:                req.Tags,
	}
	applyWebSearchOptionsToChatRequest(apiReq, wsOptions)
	return apiReq, nil
}

func buildAPIChatRequestFromCompatResponses(req openAICompatResponsesRequest) (*api.ChatRequest, *types.Error) {
	messages, err := convertOpenAICompatResponsesInput(req.Input)
	if err != nil {
		return nil, err
	}
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		messages = append([]api.Message{{Role: "system", Content: instructions}}, messages...)
	}
	tools, wsOptionsFromTools, err := convertOpenAICompatInboundTools(req.Tools)
	if err != nil {
		return nil, err
	}

	var responseFormat *api.ResponseFormat
	if req.Text != nil {
		responseFormat, err = convertOpenAICompatResponseFormat(req.Text.Format)
		if err != nil {
			return nil, err
		}
	}

	maxTokens := 0
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		maxTokens = *req.MaxOutputTokens
	}
	temp := float32(0)
	if req.Temperature != nil {
		temp = *req.Temperature
	}
	topP := float32(0)
	if req.TopP != nil {
		topP = *req.TopP
	}
	endpointMode := strings.TrimSpace(req.EndpointMode)
	if endpointMode == "" {
		endpointMode = "responses"
	}

	reasoningEffort := ""
	if req.Reasoning != nil {
		reasoningEffort = strings.TrimSpace(req.Reasoning.Effort)
	}
	wsOptions := mergeOpenAICompatWebSearchOptions(req.WebSearchOptions, wsOptionsFromTools)

	apiReq := &api.ChatRequest{
		Model:               req.Model,
		Messages:            messages,
		MaxTokens:           maxTokens,
		MaxCompletionTokens: req.MaxOutputTokens,
		Temperature:         temp,
		TopP:                topP,
		Tools:               tools,
		ToolChoice:          req.ToolChoice,
		ResponseFormat:      responseFormat,
		ParallelToolCalls:   req.ParallelToolCalls,
		ServiceTier:         req.ServiceTier,
		User:                req.User,
		ReasoningEffort:     reasoningEffort,
		Store:               req.Store,
		PreviousResponseID:  strings.TrimSpace(req.PreviousResponseID),
		Include:             req.Include,
		Truncation:          strings.TrimSpace(req.Truncation),
		Provider:            req.Provider,
		RoutePolicy:         req.RoutePolicy,
		EndpointMode:        endpointMode,
		Timeout:             req.Timeout,
		Metadata:            req.Metadata,
		Tags:                req.Tags,
	}
	applyWebSearchOptionsToChatRequest(apiReq, wsOptions)
	return apiReq, nil
}

func applyWebSearchOptionsToChatRequest(req *api.ChatRequest, opts *llm.WebSearchOptions) {
	if req == nil || opts == nil {
		return
	}
	req.WebSearchOptions = toAPIWebSearchOptions(mergeLLMWebSearchOptions(convertAPIWebSearchOptions(req.WebSearchOptions), opts))

	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	merged := convertAPIWebSearchOptions(req.WebSearchOptions)
	if merged == nil {
		return
	}
	if v := strings.TrimSpace(merged.SearchContextSize); v != "" {
		req.Metadata["web_search_context_size"] = v
	}
	if merged.UserLocation != nil {
		if v := strings.TrimSpace(merged.UserLocation.Country); v != "" {
			req.Metadata["web_search_country"] = v
		}
		if v := strings.TrimSpace(merged.UserLocation.Region); v != "" {
			req.Metadata["web_search_region"] = v
		}
		if v := strings.TrimSpace(merged.UserLocation.City); v != "" {
			req.Metadata["web_search_city"] = v
		}
		if v := strings.TrimSpace(merged.UserLocation.Timezone); v != "" {
			req.Metadata["web_search_timezone"] = v
		}
	}
	if domains := normalizeAllowedDomains(merged.AllowedDomains); len(domains) > 0 {
		req.Metadata["web_search_allowed_domains"] = strings.Join(domains, ",")
	}
	if domains := normalizeAllowedDomains(merged.BlockedDomains); len(domains) > 0 {
		req.Metadata["web_search_blocked_domains"] = strings.Join(domains, ",")
	}
	if merged.MaxUses > 0 {
		req.Metadata["web_search_max_uses"] = fmt.Sprintf("%d", merged.MaxUses)
	}
}

func convertOpenAICompatResponseFormat(raw any) (*api.ResponseFormat, *types.Error) {
	if raw == nil {
		return nil, nil
	}
	if s, ok := raw.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		return &api.ResponseFormat{Type: s}, nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, types.NewInvalidRequestError("invalid response_format")
	}
	var out api.ResponseFormat
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, types.NewInvalidRequestError("invalid response_format")
	}
	out.Type = strings.TrimSpace(out.Type)
	if out.Type == "" {
		return nil, types.NewInvalidRequestError("response_format.type is required")
	}
	return &out, nil
}

func convertOpenAICompatWebSearchOptions(in *openAICompatWebSearchOptions) *llm.WebSearchOptions {
	if in == nil {
		return nil
	}
	out := &llm.WebSearchOptions{
		SearchContextSize: strings.TrimSpace(in.SearchContextSize),
		UserLocation:      parseOpenAICompatWebSearchLocation(in.UserLocation),
		AllowedDomains:    normalizeAllowedDomains(in.AllowedDomains),
		BlockedDomains:    normalizeAllowedDomains(in.BlockedDomains),
		MaxUses:           in.MaxUses,
	}
	if out.SearchContextSize == "" && out.UserLocation == nil && len(out.AllowedDomains) == 0 &&
		len(out.BlockedDomains) == 0 && out.MaxUses == 0 {
		return nil
	}
	return out
}

func parseOpenAICompatWebSearchLocation(raw json.RawMessage) *llm.WebSearchLocation {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var payload struct {
		Type        string `json:"type,omitempty"`
		Country     string `json:"country,omitempty"`
		Region      string `json:"region,omitempty"`
		City        string `json:"city,omitempty"`
		Timezone    string `json:"timezone,omitempty"`
		Approximate *struct {
			Country  string `json:"country,omitempty"`
			Region   string `json:"region,omitempty"`
			City     string `json:"city,omitempty"`
			Timezone string `json:"timezone,omitempty"`
		} `json:"approximate,omitempty"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	out := &llm.WebSearchLocation{
		Type:     strings.TrimSpace(payload.Type),
		Country:  strings.TrimSpace(payload.Country),
		Region:   strings.TrimSpace(payload.Region),
		City:     strings.TrimSpace(payload.City),
		Timezone: strings.TrimSpace(payload.Timezone),
	}
	if payload.Approximate != nil {
		if out.Type == "" {
			out.Type = "approximate"
		}
		if out.Country == "" {
			out.Country = strings.TrimSpace(payload.Approximate.Country)
		}
		if out.Region == "" {
			out.Region = strings.TrimSpace(payload.Approximate.Region)
		}
		if out.City == "" {
			out.City = strings.TrimSpace(payload.Approximate.City)
		}
		if out.Timezone == "" {
			out.Timezone = strings.TrimSpace(payload.Approximate.Timezone)
		}
	}

	if out.Type == "" && out.Country == "" && out.Region == "" && out.City == "" && out.Timezone == "" {
		return nil
	}
	return out
}

func normalizeAllowedDomains(domains []string) []string {
	if len(domains) == 0 {
		return nil
	}
	out := make([]string, 0, len(domains))
	seen := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		v := strings.TrimSpace(d)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeOpenAICompatWebSearchOptions(topLevel *openAICompatWebSearchOptions, fromTools *llm.WebSearchOptions) *llm.WebSearchOptions {
	return mergeLLMWebSearchOptions(fromTools, convertOpenAICompatWebSearchOptions(topLevel))
}

func mergeLLMWebSearchOptions(base *llm.WebSearchOptions, override *llm.WebSearchOptions) *llm.WebSearchOptions {
	if base == nil && override == nil {
		return nil
	}
	out := &llm.WebSearchOptions{}
	if base != nil {
		out.SearchContextSize = strings.TrimSpace(base.SearchContextSize)
		out.AllowedDomains = normalizeAllowedDomains(base.AllowedDomains)
		out.BlockedDomains = normalizeAllowedDomains(base.BlockedDomains)
		out.MaxUses = base.MaxUses
		if base.UserLocation != nil {
			out.UserLocation = &llm.WebSearchLocation{
				Type:     strings.TrimSpace(base.UserLocation.Type),
				Country:  strings.TrimSpace(base.UserLocation.Country),
				Region:   strings.TrimSpace(base.UserLocation.Region),
				City:     strings.TrimSpace(base.UserLocation.City),
				Timezone: strings.TrimSpace(base.UserLocation.Timezone),
			}
		}
	}
	if override != nil {
		if v := strings.TrimSpace(override.SearchContextSize); v != "" {
			out.SearchContextSize = v
		}
		if domains := normalizeAllowedDomains(override.AllowedDomains); len(domains) > 0 {
			out.AllowedDomains = domains
		}
		if domains := normalizeAllowedDomains(override.BlockedDomains); len(domains) > 0 {
			out.BlockedDomains = domains
		}
		if override.MaxUses > 0 {
			out.MaxUses = override.MaxUses
		}
		if override.UserLocation != nil {
			if out.UserLocation == nil {
				out.UserLocation = &llm.WebSearchLocation{}
			}
			if v := strings.TrimSpace(override.UserLocation.Type); v != "" {
				out.UserLocation.Type = v
			}
			if v := strings.TrimSpace(override.UserLocation.Country); v != "" {
				out.UserLocation.Country = v
			}
			if v := strings.TrimSpace(override.UserLocation.Region); v != "" {
				out.UserLocation.Region = v
			}
			if v := strings.TrimSpace(override.UserLocation.City); v != "" {
				out.UserLocation.City = v
			}
			if v := strings.TrimSpace(override.UserLocation.Timezone); v != "" {
				out.UserLocation.Timezone = v
			}
		}
	}
	if out.SearchContextSize == "" && out.UserLocation == nil && len(out.AllowedDomains) == 0 &&
		len(out.BlockedDomains) == 0 && out.MaxUses == 0 {
		return nil
	}
	return out
}

func toAPIWebSearchOptions(in *llm.WebSearchOptions) *api.WebSearchOptions {
	if in == nil {
		return nil
	}
	out := &api.WebSearchOptions{
		SearchContextSize: strings.TrimSpace(in.SearchContextSize),
		AllowedDomains:    normalizeAllowedDomains(in.AllowedDomains),
		BlockedDomains:    normalizeAllowedDomains(in.BlockedDomains),
		MaxUses:           in.MaxUses,
	}
	if in.UserLocation != nil {
		out.UserLocation = &api.WebSearchLocation{
			Type:     strings.TrimSpace(in.UserLocation.Type),
			Country:  strings.TrimSpace(in.UserLocation.Country),
			Region:   strings.TrimSpace(in.UserLocation.Region),
			City:     strings.TrimSpace(in.UserLocation.City),
			Timezone: strings.TrimSpace(in.UserLocation.Timezone),
		}
	}
	if out.SearchContextSize == "" && out.UserLocation == nil && len(out.AllowedDomains) == 0 &&
		len(out.BlockedDomains) == 0 && out.MaxUses == 0 {
		return nil
	}
	return out
}

func convertOpenAICompatInboundMessages(in []openAICompatInboundMessage) ([]api.Message, *types.Error) {
	out := make([]api.Message, 0, len(in))
	for _, msg := range in {
		toolCalls := make([]types.ToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			args := json.RawMessage(strings.TrimSpace(tc.Function.Arguments))
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			toolCalls = append(toolCalls, types.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
			})
		}
		out = append(out, api.Message{
			Role:       msg.Role,
			Content:    flattenOpenAICompatContent(msg.Content),
			Name:       msg.Name,
			ToolCallID: msg.ToolCallID,
			ToolCalls:  toolCalls,
		})
	}
	return out, nil
}

func convertOpenAICompatResponsesInput(input any) ([]api.Message, *types.Error) {
	switch v := input.(type) {
	case string:
		return []api.Message{{Role: "user", Content: v}}, nil
	case []any:
		out := make([]api.Message, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			itemType := strings.TrimSpace(asString(m["type"]))
			if itemType == "function_call_output" {
				out = append(out, api.Message{
					Role:       "tool",
					ToolCallID: strings.TrimSpace(asString(m["call_id"])),
					Content:    flattenOpenAICompatContent(m["output"]),
				})
				continue
			}
			role := strings.TrimSpace(asString(m["role"]))
			if role == "" {
				role = "user"
			}
			out = append(out, api.Message{
				Role:    role,
				Content: flattenOpenAICompatContent(m["content"]),
			})
		}
		if len(out) == 0 {
			return nil, types.NewInvalidRequestError("responses input cannot be empty")
		}
		return out, nil
	default:
		return nil, types.NewInvalidRequestError("responses input must be string or array")
	}
}

func convertOpenAICompatInboundTools(in []openAICompatInboundTool) ([]api.ToolSchema, *llm.WebSearchOptions, *types.Error) {
	if len(in) == 0 {
		return nil, nil, nil
	}
	tools := make([]api.ToolSchema, 0, len(in))
	var wsOpts *llm.WebSearchOptions

	for _, tool := range in {
		toolType := strings.ToLower(strings.TrimSpace(tool.Type))
		switch toolType {
		case "web_search", "web_search_preview":
			tools = append(tools, api.ToolSchema{
				Name:       "web_search",
				Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			})
			candidate := &llm.WebSearchOptions{
				SearchContextSize: strings.TrimSpace(tool.SearchContextSize),
				UserLocation:      parseOpenAICompatWebSearchLocation(tool.UserLocation),
			}
			if tool.Filters != nil {
				candidate.AllowedDomains = normalizeAllowedDomains(tool.Filters.AllowedDomains)
			}
			wsOpts = mergeLLMWebSearchOptions(wsOpts, candidate)
		case "", "function":
			name := strings.TrimSpace(tool.Name)
			desc := strings.TrimSpace(tool.Description)
			params := tool.Parameters
			if strings.TrimSpace(tool.Function.Name) != "" {
				name = strings.TrimSpace(tool.Function.Name)
				desc = strings.TrimSpace(tool.Function.Description)
				params = tool.Function.Parameters
			}
			if name == "" {
				continue
			}
			paramJSON, err := json.Marshal(params)
			if err != nil {
				return nil, nil, types.NewInvalidRequestError("invalid tool parameters")
			}
			if string(paramJSON) == "null" {
				paramJSON = []byte(`{"type":"object","properties":{}}`)
			}
			tools = append(tools, api.ToolSchema{
				Name:        name,
				Description: desc,
				Parameters:  paramJSON,
			})
		default:
			continue
		}
	}
	return tools, wsOpts, nil
}

func flattenOpenAICompatContent(content any) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, p := range v {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			text := strings.TrimSpace(asString(pm["text"]))
			if text == "" {
				text = strings.TrimSpace(asString(pm["content"]))
			}
			if text == "" {
				continue
			}
			parts = append(parts, text)
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if t := strings.TrimSpace(asString(v["text"])); t != "" {
			return t
		}
		if t := strings.TrimSpace(asString(v["content"])); t != "" {
			return t
		}
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toOpenAICompatChatResponse(resp *api.ChatResponse) openAICompatChatResponse {
	out := openAICompatChatResponse{
		ID:      firstNonEmptyString(resp.ID, fmt.Sprintf("chatcmpl_%d", time.Now().UnixNano())),
		Object:  "chat.completion",
		Created: safeUnix(resp.CreatedAt),
		Model:   resp.Model,
		Choices: make([]openAICompatChatChoice, 0, len(resp.Choices)),
		Usage: openAICompatChatUsage{
			PromptTokens:            resp.Usage.PromptTokens,
			CompletionTokens:        resp.Usage.CompletionTokens,
			TotalTokens:             resp.Usage.TotalTokens,
			PromptTokensDetails:     toOpenAICompatPromptTokenDetails(resp.Usage.PromptTokensDetails),
			CompletionTokensDetails: toOpenAICompatCompletionTokenDetails(resp.Usage.CompletionTokensDetails),
		},
	}

	for _, c := range resp.Choices {
		out.Choices = append(out.Choices, openAICompatChatChoice{
			Index: c.Index,
			Message: openAICompatOutboundMsg{
				Role:             c.Message.Role,
				Content:          c.Message.Content,
				ReasoningContent: c.Message.ReasoningContent,
				Refusal:          c.Message.Refusal,
				Name:             c.Message.Name,
				ToolCallID:       c.Message.ToolCallID,
				ToolCalls:        toOpenAICompatOutboundToolCalls(c.Message.ToolCalls),
				Annotations:      toOpenAICompatAnnotations(c.Message.Annotations),
			},
			FinishReason: c.FinishReason,
		})
	}
	return out
}

func toOpenAICompatChatChunkResponse(chunk *llm.StreamChunk, created int64, model string) openAICompatChatChunkResponse {
	out := openAICompatChatChunkResponse{
		ID:      firstNonEmptyString(chunk.ID, fmt.Sprintf("chatcmpl_%d", time.Now().UnixNano())),
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   firstNonEmptyString(chunk.Model, model),
		Choices: []openAICompatChatChunkChoice{
			{
				Index: chunk.Index,
				Delta: openAICompatOutboundMsg{
					Role:             string(chunk.Delta.Role),
					Content:          chunk.Delta.Content,
					ReasoningContent: chunk.Delta.ReasoningContent,
					Refusal:          chunk.Delta.Refusal,
					Name:             chunk.Delta.Name,
					ToolCallID:       chunk.Delta.ToolCallID,
					ToolCalls:        toOpenAICompatOutboundToolCalls(chunk.Delta.ToolCalls),
				},
				FinishReason: nil,
			},
		},
	}
	if strings.TrimSpace(chunk.FinishReason) != "" {
		out.Choices[0].FinishReason = chunk.FinishReason
	}
	return out
}

func toOpenAICompatResponsesResponse(resp *api.ChatResponse) openAICompatResponsesResponse {
	out := openAICompatResponsesResponse{
		ID:        firstNonEmptyString(resp.ID, fmt.Sprintf("resp_%d", time.Now().UnixNano())),
		Object:    "response",
		CreatedAt: safeUnix(resp.CreatedAt),
		Status:    "completed",
		Model:     resp.Model,
		Output:    make([]openAICompatResponsesOutput, 0, len(resp.Choices)),
		Usage: openAICompatResponsesUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
	}

	for i, c := range resp.Choices {
		msgOut := openAICompatResponsesOutput{
			ID:     fmt.Sprintf("msg_%d", i+1),
			Type:   "message",
			Status: "completed",
			Role:   c.Message.Role,
			Content: []openAICompatResponsesContent{
				{Type: "output_text", Text: c.Message.Content},
			},
		}
		out.Output = append(out.Output, msgOut)

		for _, tc := range c.Message.ToolCalls {
			out.Output = append(out.Output, openAICompatResponsesOutput{
				Type:      "function_call",
				Name:      tc.Name,
				Arguments: tc.Arguments,
				CallID:    tc.ID,
			})
		}
	}
	return out
}

func toOpenAICompatOutboundToolCalls(calls []types.ToolCall) []openAICompatOutboundToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]openAICompatOutboundToolCall, 0, len(calls))
	for _, c := range calls {
		item := openAICompatOutboundToolCall{
			ID:   c.ID,
			Type: "function",
		}
		item.Function.Name = c.Name
		item.Function.Arguments = string(c.Arguments)
		out = append(out, item)
	}
	return out
}

func toOpenAICompatAnnotations(annotations []types.Annotation) []openAICompatAnnotation {
	if len(annotations) == 0 {
		return nil
	}
	out := make([]openAICompatAnnotation, 0, len(annotations))
	for _, a := range annotations {
		ann := openAICompatAnnotation{Type: a.Type}
		if a.URL != "" || a.Title != "" || a.StartIndex != 0 || a.EndIndex != 0 {
			ann.URLCitation = &openAICompatURLCitationDetail{
				StartIndex: a.StartIndex,
				EndIndex:   a.EndIndex,
				URL:        a.URL,
				Title:      a.Title,
			}
		}
		out = append(out, ann)
	}
	return out
}

func toOpenAICompatPromptTokenDetails(d *api.PromptTokensDetails) *openAICompatTokenDetails {
	if d == nil {
		return nil
	}
	return &openAICompatTokenDetails{
		CachedTokens: d.CachedTokens,
		AudioTokens:  d.AudioTokens,
	}
}

func toOpenAICompatCompletionTokenDetails(d *api.CompletionTokensDetails) *openAICompatTokenDetails {
	if d == nil {
		return nil
	}
	return &openAICompatTokenDetails{
		ReasoningTokens:          d.ReasoningTokens,
		AudioTokens:              d.AudioTokens,
		AcceptedPredictionTokens: d.AcceptedPredictionTokens,
		RejectedPredictionTokens: d.RejectedPredictionTokens,
	}
}

func decodeOpenAICompatJSON(w http.ResponseWriter, r *http.Request, out any) *types.Error {
	if r == nil || r.Body == nil {
		return types.NewInvalidRequestError("request body is required")
	}
	if ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return types.NewInvalidRequestError("content-type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		if err.Error() == "http: request body too large" {
			return types.NewInvalidRequestError("request body too large (max 1MB)")
		}
		return types.NewInvalidRequestError("invalid JSON body")
	}
	if dec.More() {
		return types.NewInvalidRequestError("request body must only contain a single JSON object")
	}
	return nil
}

func writeOpenAICompatJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeOpenAICompatError(w http.ResponseWriter, err *types.Error) {
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
	payload := openAICompatErrorEnvelope{
		Error: openAICompatError{
			Message: err.Message,
			Type:    openAICompatErrorType(err),
			Code:    string(err.Code),
		},
	}
	writeOpenAICompatJSON(w, status, payload)
}

func openAICompatErrorType(err *types.Error) string {
	if err == nil {
		return "server_error"
	}
	switch err.Code {
	case types.ErrInvalidRequest:
		return "invalid_request_error"
	case types.ErrUnauthorized, types.ErrAuthentication:
		return "authentication_error"
	case types.ErrRateLimit:
		return "rate_limit_error"
	default:
		return "server_error"
	}
}

func writeSSEJSON(w http.ResponseWriter, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return writeSSE(w, []byte("data: "), data, []byte("\n\n"))
}

func writeSSEEventJSON(w http.ResponseWriter, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return writeSSE(w, []byte("event: "+event+"\n"), []byte("data: "), data, []byte("\n\n"))
}

func safeUnix(t time.Time) int64 {
	if t.IsZero() {
		return time.Now().Unix()
	}
	return t.Unix()
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func extractStreamChunk(chunk llmcore.UnifiedChunk) (*llm.StreamChunk, *types.Error) {
	if chunk.Err != nil {
		return nil, chunk.Err
	}
	llmChunk, ok := chunk.Output.(*llm.StreamChunk)
	if !ok || llmChunk == nil {
		return nil, types.NewInternalError("invalid stream chunk payload")
	}
	return llmChunk, nil
}
