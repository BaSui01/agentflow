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
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/types"
)

type openAICompatChatCompletionsRequest struct {
	Model                string                        `json:"model"`
	Messages             []openAICompatInboundMessage  `json:"messages"`
	MaxTokens            int                           `json:"max_tokens,omitempty"`
	MaxCompletionTokens  *int                          `json:"max_completion_tokens,omitempty"`
	Temperature          float32                       `json:"temperature,omitempty"`
	TopP                 float32                       `json:"top_p,omitempty"`
	FrequencyPenalty     *float32                      `json:"frequency_penalty,omitempty"`
	PresencePenalty      *float32                      `json:"presence_penalty,omitempty"`
	RepetitionPenalty    *float32                      `json:"repetition_penalty,omitempty"`
	N                    *int                          `json:"n,omitempty"`
	LogProbs             *bool                         `json:"logprobs,omitempty"`
	TopLogProbs          *int                          `json:"top_logprobs,omitempty"`
	Stop                 []string                      `json:"stop,omitempty"`
	Tools                []openAICompatInboundTool     `json:"tools,omitempty"`
	ToolChoice           any                           `json:"tool_choice,omitempty"`
	ResponseFormat       any                           `json:"response_format,omitempty"`
	Stream               bool                          `json:"stream,omitempty"`
	StreamOptions        *api.StreamOptions            `json:"stream_options,omitempty"`
	ParallelToolCalls    *bool                         `json:"parallel_tool_calls,omitempty"`
	ServiceTier          *string                       `json:"service_tier,omitempty"`
	User                 string                        `json:"user,omitempty"`
	ReasoningEffort      string                        `json:"reasoning_effort,omitempty"`
	Store                *bool                         `json:"store,omitempty"`
	Modalities           []string                      `json:"modalities,omitempty"`
	WebSearchOptions     *openAICompatWebSearchOptions `json:"web_search_options,omitempty"`
	PromptCacheKey       string                        `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention string                        `json:"prompt_cache_retention,omitempty"`
	PreviousResponseID   string                        `json:"previous_response_id,omitempty"`
	ConversationID       string                        `json:"conversation_id,omitempty"`
	Include              []string                      `json:"include,omitempty"`
	Truncation           string                        `json:"truncation,omitempty"`
	Provider             string                        `json:"provider,omitempty"`
	RoutePolicy          string                        `json:"route_policy,omitempty"`
	EndpointMode         string                        `json:"endpoint_mode,omitempty"`
	Timeout              string                        `json:"timeout,omitempty"`
	Metadata             map[string]string             `json:"metadata,omitempty"`
	Tags                 []string                      `json:"tags,omitempty"`
}

type openAICompatResponsesRequest struct {
	Model                string                          `json:"model"`
	Input                any                             `json:"input"`
	Instructions         string                          `json:"instructions,omitempty"`
	MaxOutputTokens      *int                            `json:"max_output_tokens,omitempty"`
	Temperature          *float32                        `json:"temperature,omitempty"`
	TopP                 *float32                        `json:"top_p,omitempty"`
	Tools                []openAICompatInboundTool       `json:"tools,omitempty"`
	ToolChoice           any                             `json:"tool_choice,omitempty"`
	Reasoning            *openAICompatResponsesReasoning `json:"reasoning,omitempty"`
	Text                 *openAICompatResponsesTextParam `json:"text,omitempty"`
	ParallelToolCalls    *bool                           `json:"parallel_tool_calls,omitempty"`
	ServiceTier          *string                         `json:"service_tier,omitempty"`
	User                 string                          `json:"user,omitempty"`
	Store                *bool                           `json:"store,omitempty"`
	WebSearchOptions     *openAICompatWebSearchOptions   `json:"web_search_options,omitempty"`
	PromptCacheKey       string                          `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention string                          `json:"prompt_cache_retention,omitempty"`
	PreviousResponseID   string                          `json:"previous_response_id,omitempty"`
	Conversation         string                          `json:"conversation,omitempty"`
	ConversationID       string                          `json:"conversation_id,omitempty"`
	Include              []string                        `json:"include,omitempty"`
	Truncation           string                          `json:"truncation,omitempty"`
	Stream               bool                            `json:"stream,omitempty"`
	Provider             string                          `json:"provider,omitempty"`
	RoutePolicy          string                          `json:"route_policy,omitempty"`
	EndpointMode         string                          `json:"endpoint_mode,omitempty"`
	Timeout              string                          `json:"timeout,omitempty"`
	Metadata             map[string]string               `json:"metadata,omitempty"`
	Tags                 []string                        `json:"tags,omitempty"`
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
	Custom struct {
		Name  string `json:"name,omitempty"`
		Input string `json:"input,omitempty"`
	} `json:"custom,omitempty"`
}

type openAICompatInboundTool struct {
	Type        string `json:"type,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
	Format      any    `json:"format,omitempty"`
	Strict      *bool  `json:"strict,omitempty"`
	Function    struct {
		Name        string `json:"name,omitempty"`
		Description string `json:"description,omitempty"`
		Parameters  any    `json:"parameters,omitempty"`
		Strict      *bool  `json:"strict,omitempty"`
	} `json:"function,omitempty"`
	Custom struct {
		Name        string `json:"name,omitempty"`
		Description string `json:"description,omitempty"`
		Format      any    `json:"format,omitempty"`
	} `json:"custom,omitempty"`
	SearchContextSize string                        `json:"search_context_size,omitempty"`
	UserLocation      json.RawMessage               `json:"user_location,omitempty"`
	Filters           *openAICompatWebSearchFilters `json:"filters,omitempty"`
}

type openAICompatResponsesReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
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
	Function *struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function,omitempty"`
	Custom *struct {
		Name  string `json:"name"`
		Input string `json:"input"`
	} `json:"custom,omitempty"`
}

type openAICompatAnnotation struct {
	Type        string                         `json:"type"`
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
	ID               string                         `json:"id,omitempty"`
	Type             string                         `json:"type"`
	Status           string                         `json:"status,omitempty"`
	Role             string                         `json:"role,omitempty"`
	Content          []openAICompatResponsesContent `json:"content,omitempty"`
	Summary          []openAICompatResponsesContent `json:"summary,omitempty"`
	EncryptedContent string                         `json:"encrypted_content,omitempty"`
	Name             string                         `json:"name,omitempty"`
	Arguments        json.RawMessage                `json:"arguments,omitempty"`
	CallID           string                         `json:"call_id,omitempty"`
	Input            string                         `json:"input,omitempty"`
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
	service, svcErr := h.currentServiceOrUnavailable("chat")
	if svcErr != nil {
		writeOpenAICompatError(w, svcErr)
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

	usecaseReq := h.converter.ToUsecaseRequest(apiReq)
	result, svcErr := service.Complete(r.Context(), usecaseReq)
	if svcErr != nil {
		writeOpenAICompatError(w, svcErr)
		return
	}
	out := toOpenAICompatChatResponse(h.converter.ToAPIResponseFromUsecase(result.Response))
	writeOpenAICompatJSON(w, http.StatusOK, out)
}

func (h *ChatHandler) HandleOpenAICompatResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpenAICompatError(w, types.NewError(types.ErrInvalidRequest, "method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed))
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("chat")
	if svcErr != nil {
		writeOpenAICompatError(w, svcErr)
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

	usecaseReq := h.converter.ToUsecaseRequest(apiReq)
	result, svcErr := service.Complete(r.Context(), usecaseReq)
	if svcErr != nil {
		writeOpenAICompatError(w, svcErr)
		return
	}
	out := toOpenAICompatResponsesResponse(h.converter.ToAPIResponseFromUsecase(result.Response))
	writeOpenAICompatJSON(w, http.StatusOK, out)
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
