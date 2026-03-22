package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// previousResponseIDKey 是 Responses API 中 previous_response_id 的 context key。
type previousResponseIDKey struct{}

// WithPreviousResponseID 在 ctx 中写入 previous_response_id。
func WithPreviousResponseID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, previousResponseIDKey{}, id)
}

// PreviousResponseIDFromContext 从 ctx 读取 previous_response_id。
func PreviousResponseIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(previousResponseIDKey{}).(string)
	return v, ok && v != ""
}

// OpenAIProvider 实现 OpenAI LLM 提供者.
// 支持传统 Chat Completions API 和新的 Responses API (2025).
// 传统 API 通过嵌入的 openaicompat.Provider 处理；Responses API 通过 Completion 覆写实现.
type OpenAIProvider struct {
	*openaicompat.Provider
	openaiCfg providers.OpenAIConfig
}

// NewOpenAIProvider 创建新的 OpenAI 提供者实例.
func NewOpenAIProvider(cfg providers.OpenAIConfig, logger *zap.Logger) *OpenAIProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}

	p := &OpenAIProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "openai",
			APIKey:        cfg.APIKey,
			APIKeys:       cfg.APIKeys,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "gpt-5.2", // 2026: GPT-5.2
			Timeout:       cfg.Timeout,
		}, logger),
		openaiCfg: cfg,
	}

	// Set custom headers for OpenAI (Organization support)
	p.SetBuildHeaders(func(req *http.Request, apiKey string) {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		if cfg.Organization != "" {
			req.Header.Set("OpenAI-Organization", cfg.Organization)
		}
		req.Header.Set("Content-Type", "application/json")
	})

	return p
}

// Endpoints 返回该提供者使用的所有 API 端点完整 URL。
func (p *OpenAIProvider) Endpoints() llm.ProviderEndpoints {
	ep := p.Provider.Endpoints()
	if p.openaiCfg.UseResponsesAPI {
		base := strings.TrimRight(p.openaiCfg.BaseURL, "/")
		ep.Completion = base + "/v1/responses"
	}
	return ep
}

// Completion 覆写基类方法，支持 Responses API 路由.
// 当 UseResponsesAPI 启用时走 /v1/responses，否则委托给 openaicompat.Provider.Completion.
func (p *OpenAIProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if !p.useResponsesAPIForRequest(req) {
		return p.Provider.Completion(ctx, req)
	}

	// Apply rewriter chain (与基类保持一致)
	rewrittenReq, err := p.RewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &types.Error{
			Code: llm.ErrInvalidRequest, Message: fmt.Sprintf("request rewrite failed: %v", err),
			HTTPStatus: http.StatusBadRequest, Provider: p.Name(),
		}
	}
	req = rewrittenReq

	apiKey := p.Provider.ResolveAPIKey(ctx)
	return p.completionWithResponsesAPI(ctx, req, apiKey)
}

// --- Responses API Types ---

// openAIResponsesRequest represents the POST /v1/responses request body.
type openAIResponsesRequest struct {
	Model              string              `json:"model"`
	Input              any                 `json:"input"` // string or []ResponsesInputItem
	Instructions       string              `json:"instructions,omitempty"`
	MaxOutputTokens    *int                `json:"max_output_tokens,omitempty"`
	Temperature        *float32            `json:"temperature,omitempty"`
	TopP               *float32            `json:"top_p,omitempty"`
	Tools              []any               `json:"tools,omitempty"`
	ToolChoice         any                 `json:"tool_choice,omitempty"`
	ParallelToolCalls  *bool               `json:"parallel_tool_calls,omitempty"`
	PreviousResponseID string              `json:"previous_response_id,omitempty"`
	Store              *bool               `json:"store,omitempty"`
	Metadata           map[string]string   `json:"metadata,omitempty"`
	Include            []string            `json:"include,omitempty"`
	Truncation         string              `json:"truncation,omitempty"` // "auto" or "disabled"
	Reasoning          *responsesReasoning `json:"reasoning,omitempty"`
	Text               *responsesTextParam `json:"text,omitempty"`
	ServiceTier        *string             `json:"service_tier,omitempty"`
	User               string              `json:"user,omitempty"`
	Stream             bool                `json:"stream,omitempty"`
	TopLogProbs        *int                `json:"top_logprobs,omitempty"`
}

// responsesReasoning configures reasoning for o-series and gpt-5 models.
type responsesReasoning struct {
	Effort  string `json:"effort,omitempty"`  // none/minimal/low/medium/high/xhigh
	Summary string `json:"summary,omitempty"` // auto/concise/detailed
}

// responsesTextParam configures text output format.
type responsesTextParam struct {
	Format    any    `json:"format,omitempty"`    // ResponseFormat object
	Verbosity string `json:"verbosity,omitempty"` // low/medium/high
}

// responsesInputItem represents a structured input item.
type responsesInputItem struct {
	Type    string `json:"type,omitempty"`
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []inputContentPart
}

// inputContentPart represents a content part in a structured input.
type inputContentPart struct {
	Type     string `json:"type"` // "input_text", "input_image", "input_file"
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	FileURL  string `json:"file_url,omitempty"`
	Detail   string `json:"detail,omitempty"` // "auto", "low", "high"
}

// functionCallInputItem represents a function call in the input (for multi-turn).
type functionCallInputItem struct {
	Type      string          `json:"type"` // "function_call"
	ID        string          `json:"id"`
	CallID    string          `json:"call_id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// functionCallOutputItem represents a function call output in the input.
type functionCallOutputItem struct {
	Type   string `json:"type"` // "function_call_output"
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

// --- Responses API Response Types ---

// openAIResponsesResponse represents the Responses API response.
type openAIResponsesResponse struct {
	ID          string                `json:"id"`
	Object      string                `json:"object"`
	CreatedAt   int64                 `json:"created_at"`
	Status      string                `json:"status"`
	CompletedAt *int64                `json:"completed_at,omitempty"`
	Model       string                `json:"model"`
	Output      []responsesOutputItem `json:"output"`
	Usage       *responsesUsage       `json:"usage,omitempty"`
	ServiceTier string                `json:"service_tier,omitempty"`
	Error       *responsesError       `json:"error,omitempty"`
}

// responsesUsage uses different field names than Chat Completions.
type responsesUsage struct {
	InputTokens         int                          `json:"input_tokens"`
	OutputTokens        int                          `json:"output_tokens"`
	TotalTokens         int                          `json:"total_tokens"`
	InputTokensDetails  *responsesInputTokenDetails  `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *responsesOutputTokenDetails `json:"output_tokens_details,omitempty"`
}

type responsesInputTokenDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type responsesOutputTokenDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type responsesError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// responsesOutputItem represents an item in the response output array.
type responsesOutputItem struct {
	Type    string             `json:"type"` // "message", "function_call", "reasoning"
	ID      string             `json:"id"`
	Status  string             `json:"status,omitempty"`
	Role    string             `json:"role,omitempty"`
	Content []responsesContent `json:"content,omitempty"`
	// function_call fields
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	// reasoning fields
	Summary []responsesContent `json:"summary,omitempty"`
}

// responsesContent represents a content item in the output.
type responsesContent struct {
	Type        string                `json:"type"`
	Text        string                `json:"text,omitempty"`
	Refusal     string                `json:"refusal,omitempty"`
	Annotations []responsesAnnotation `json:"annotations,omitempty"`
}

// responsesAnnotation represents a citation annotation.
type responsesAnnotation struct {
	Type       string `json:"type"`
	StartIndex int    `json:"start_index,omitempty"`
	EndIndex   int    `json:"end_index,omitempty"`
	URL        string `json:"url,omitempty"`
	Title      string `json:"title,omitempty"`
}

// --- Completion & Helper Methods ---

// completionWithResponsesAPI 使用新的 Responses API (/v1/responses).
func (p *OpenAIProvider) completionWithResponsesAPI(ctx context.Context, req *llm.ChatRequest, apiKey string) (*llm.ChatResponse, error) {
	body := p.buildResponsesRequest(req)

	// 从 context 或 request 获取 previous_response_id
	if req.PreviousResponseID != "" {
		body.PreviousResponseID = req.PreviousResponseID
	} else if prevID, ok := PreviousResponseIDFromContext(ctx); ok {
		body.PreviousResponseID = prevID
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal responses api request: %w", err)
	}

	var responsesResp openAIResponsesResponse
	if err := p.Provider.DoJSON(ctx, http.MethodPost, "/v1/responses", json.RawMessage(payload), apiKey, &responsesResp); err != nil {
		return nil, err
	}

	return toResponsesAPIChatResponse(responsesResp, p.Name()), nil
}

// buildResponsesRequest converts a ChatRequest to a Responses API request.
func (p *OpenAIProvider) buildResponsesRequest(req *llm.ChatRequest) openAIResponsesRequest {
	body := openAIResponsesRequest{
		Model:             providerbase.ChooseModel(req, p.openaiCfg.Model, "gpt-5.2"),
		ToolChoice:        req.ToolChoice,
		Store:             req.Store,
		Metadata:          req.Metadata,
		Include:           req.Include,
		Truncation:        strings.TrimSpace(req.Truncation),
		User:              req.User,
		ServiceTier:       req.ServiceTier,
		TopLogProbs:       req.TopLogProbs,
		ParallelToolCalls: req.ParallelToolCalls,
	}

	// Temperature / TopP — 使用指针避免零值被发送
	if req.Temperature != 0 {
		t := req.Temperature
		body.Temperature = &t
	}
	if req.TopP != 0 {
		tp := req.TopP
		body.TopP = &tp
	}

	// MaxOutputTokens: 优先 MaxCompletionTokens，回退 MaxTokens
	if req.MaxCompletionTokens != nil {
		body.MaxOutputTokens = req.MaxCompletionTokens
	} else if req.MaxTokens > 0 {
		mt := req.MaxTokens
		body.MaxOutputTokens = &mt
	}

	// 构建 input
	body.Input = convertMessagesToResponsesInput(req.Messages)

	// 构建 tools（支持 OpenAI Responses 原生 web_search 工具）
	body.Tools = buildResponsesTools(req)

	// Reasoning: only pass valid effort values
	if effort, ok := chooseResponsesReasoningEffort(req); ok {
		body.Reasoning = &responsesReasoning{Effort: effort}
	}

	// ResponseFormat → text.format
	if req.ResponseFormat != nil {
		body.Text = &responsesTextParam{
			Format: providerbase.ConvertResponseFormat(req.ResponseFormat),
		}
	}

	return body
}

func buildResponsesTools(req *llm.ChatRequest) []any {
	if req == nil {
		return nil
	}

	tools := make([]any, 0, len(req.Tools)+1)
	hasNativeWebSearch := false

	for _, t := range req.Tools {
		if isNativeResponsesWebSearchTool(t.Name) {
			if !hasNativeWebSearch {
				tools = append(tools, buildResponsesWebSearchTool(req.WebSearchOptions))
				hasNativeWebSearch = true
			}
			continue
		}

		tool := map[string]any{
			"type": "function",
			"name": t.Name,
		}
		if t.Description != "" {
			tool["description"] = t.Description
		}
		if len(t.Parameters) > 0 {
			var params any
			if err := json.Unmarshal(t.Parameters, &params); err == nil {
				tool["parameters"] = params
			}
		}
		tools = append(tools, tool)
	}

	// 仅设置了 web_search_options 时，自动注入原生 web_search tool。
	if req.WebSearchOptions != nil && !hasNativeWebSearch {
		tools = append(tools, buildResponsesWebSearchTool(req.WebSearchOptions))
	}

	if len(tools) == 0 {
		return nil
	}
	return tools
}

func isNativeResponsesWebSearchTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "web_search", "web_search_preview":
		return true
	default:
		return false
	}
}

func buildResponsesWebSearchTool(opts *llm.WebSearchOptions) map[string]any {
	tool := map[string]any{
		"type": "web_search",
	}
	if opts == nil {
		return tool
	}
	if size := strings.TrimSpace(opts.SearchContextSize); size != "" {
		tool["search_context_size"] = size
	}
	if loc := convertResponsesWebSearchLocation(opts.UserLocation); len(loc) > 0 {
		tool["user_location"] = loc
	}
	if domains := normalizeResponsesAllowedDomains(opts.AllowedDomains); len(domains) > 0 {
		tool["filters"] = map[string]any{
			"allowed_domains": domains,
		}
	}
	return tool
}

func normalizeResponsesAllowedDomains(domains []string) []string {
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

func convertResponsesWebSearchLocation(loc *llm.WebSearchLocation) map[string]any {
	if loc == nil {
		return nil
	}

	out := map[string]any{}
	locType := strings.TrimSpace(loc.Type)
	if locType == "" {
		locType = "approximate"
	}
	out["type"] = locType

	if v := strings.TrimSpace(loc.Country); v != "" {
		out["country"] = v
	}
	if v := strings.TrimSpace(loc.Region); v != "" {
		out["region"] = v
	}
	if v := strings.TrimSpace(loc.City); v != "" {
		out["city"] = v
	}
	if v := strings.TrimSpace(loc.Timezone); v != "" {
		out["timezone"] = v
	}
	return out
}

// convertMessagesToResponsesInput converts messages to Responses API input format.
func convertMessagesToResponsesInput(msgs []types.Message) []any {
	items := make([]any, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case llm.RoleSystem, llm.RoleDeveloper:
			items = append(items, responsesInputItem{
				Role:    string(m.Role),
				Content: buildInputContent(m),
			})
		case llm.RoleUser:
			items = append(items, responsesInputItem{
				Role:    "user",
				Content: buildInputContent(m),
			})
		case llm.RoleAssistant:
			if len(m.ToolCalls) > 0 {
				if m.Content != "" {
					items = append(items, responsesInputItem{
						Type:    "message",
						Role:    "assistant",
						Content: m.Content,
					})
				}
				for _, tc := range m.ToolCalls {
					items = append(items, functionCallInputItem{
						Type:      "function_call",
						ID:        tc.ID,
						CallID:    tc.ID,
						Name:      tc.Name,
						Arguments: tc.Arguments,
					})
				}
			} else {
				items = append(items, responsesInputItem{
					Role:    "assistant",
					Content: buildInputContent(m),
				})
			}
		case llm.RoleTool:
			items = append(items, functionCallOutputItem{
				Type:   "function_call_output",
				CallID: m.ToolCallID,
				Output: m.Content,
			})
		default:
			items = append(items, responsesInputItem{
				Role:    string(m.Role),
				Content: m.Content,
			})
		}
	}
	return items
}

// buildInputContent builds the content field for a Responses API input item.
// Returns a string for text-only, or []inputContentPart for multimodal.
func buildInputContent(m types.Message) any {
	if len(m.Images) == 0 && len(m.Videos) == 0 {
		return m.Content
	}
	parts := make([]inputContentPart, 0)
	if m.Content != "" {
		parts = append(parts, inputContentPart{
			Type: "input_text",
			Text: m.Content,
		})
	}
	for _, img := range m.Images {
		part := inputContentPart{Type: "input_image", Detail: "auto"}
		if img.Type == "url" && img.URL != "" {
			part.ImageURL = img.URL
		} else if img.Type == "base64" && img.Data != "" {
			part.ImageURL = "data:image/png;base64," + img.Data
		}
		parts = append(parts, part)
	}
	for _, video := range m.Videos {
		if video.URL == "" {
			continue
		}
		parts = append(parts, inputContentPart{
			Type:    "input_file",
			FileURL: video.URL,
		})
	}
	return parts
}

func chooseResponsesReasoningEffort(req *llm.ChatRequest) (string, bool) {
	allowed := map[string]struct{}{
		"none": {}, "minimal": {}, "low": {}, "medium": {}, "high": {}, "xhigh": {},
	}
	if req == nil {
		return "", false
	}
	if _, ok := allowed[req.ReasoningEffort]; ok {
		return req.ReasoningEffort, true
	}
	switch req.ReasoningMode {
	case "minimal", "low", "medium", "high":
		return req.ReasoningMode, true
	case "thinking":
		return "medium", true
	case "extended":
		return "high", true
	default:
		return "", false
	}
}

func (p *OpenAIProvider) useResponsesAPIForRequest(req *llm.ChatRequest) bool {
	mode := resolveEndpointMode(req)
	switch mode {
	case "responses":
		return true
	case "chat_completions":
		return false
	default:
		return p.openaiCfg.UseResponsesAPI
	}
}

func resolveEndpointMode(req *llm.ChatRequest) string {
	if req == nil || len(req.Metadata) == 0 {
		return ""
	}

	switch strings.ToLower(strings.TrimSpace(req.Metadata["endpoint_mode"])) {
	case "responses":
		return "responses"
	case "chat_completions":
		return "chat_completions"
	case "auto", "":
		return ""
	default:
		return ""
	}
}

// toResponsesAPIChatResponse 将 Responses API 响应转换为统一的 llm.ChatResponse.
func toResponsesAPIChatResponse(resp openAIResponsesResponse, provider string) *llm.ChatResponse {
	var choices []llm.ChatChoice
	choiceIdx := 0

	for _, output := range resp.Output {
		switch output.Type {
		case "message":
			msg := types.Message{Role: types.Role(output.Role)}
			for _, content := range output.Content {
				switch content.Type {
				case "output_text":
					msg.Content += content.Text
					for _, ann := range content.Annotations {
						msg.Annotations = append(msg.Annotations, types.Annotation{
							Type:       ann.Type,
							StartIndex: ann.StartIndex,
							EndIndex:   ann.EndIndex,
							URL:        ann.URL,
							Title:      ann.Title,
						})
					}
				case "refusal":
					refusal := content.Refusal
					msg.Refusal = &refusal
				}
			}
			choices = append(choices, llm.ChatChoice{
				Index: choiceIdx, FinishReason: mapResponsesStatus(resp.Status), Message: msg,
			})
			choiceIdx++

		case "function_call":
			if len(choices) == 0 || choices[len(choices)-1].Message.Role != llm.RoleAssistant {
				choices = append(choices, llm.ChatChoice{
					Index: choiceIdx, FinishReason: "tool_calls",
					Message: types.Message{Role: llm.RoleAssistant},
				})
				choiceIdx++
			}
			lastIdx := len(choices) - 1
			choices[lastIdx].Message.ToolCalls = append(choices[lastIdx].Message.ToolCalls, types.ToolCall{
				ID:        output.CallID,
				Name:      output.Name,
				Arguments: output.Arguments,
			})
			choices[lastIdx].FinishReason = "tool_calls"
		}
	}

	chatResp := &llm.ChatResponse{
		ID:          resp.ID,
		Provider:    provider,
		Model:       resp.Model,
		Choices:     choices,
		ServiceTier: resp.ServiceTier,
	}
	if resp.CreatedAt != 0 {
		chatResp.CreatedAt = time.Unix(resp.CreatedAt, 0)
	}
	if resp.Usage != nil {
		chatResp.Usage = llm.ChatUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
		if resp.Usage.InputTokensDetails != nil {
			chatResp.Usage.PromptTokensDetails = &llm.PromptTokensDetails{
				CachedTokens: resp.Usage.InputTokensDetails.CachedTokens,
			}
		}
		if resp.Usage.OutputTokensDetails != nil {
			chatResp.Usage.CompletionTokensDetails = &llm.CompletionTokensDetails{
				ReasoningTokens: resp.Usage.OutputTokensDetails.ReasoningTokens,
			}
		}
	}
	return chatResp
}

// mapResponsesStatus maps Responses API status to Chat Completions finish_reason.
func mapResponsesStatus(status string) string {
	switch status {
	case "completed":
		return "stop"
	case "failed":
		return "error"
	case "incomplete":
		return "length"
	case "cancelled":
		return "stop"
	default:
		return status
	}
}

// Stream 覆写基类方法，支持 Responses API 流式.
func (p *OpenAIProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	if !p.useResponsesAPIForRequest(req) {
		return p.Provider.Stream(ctx, req)
	}

	// Apply rewriter chain
	rewrittenReq, err := p.RewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &types.Error{
			Code: llm.ErrInvalidRequest, Message: fmt.Sprintf("request rewrite failed: %v", err),
			HTTPStatus: http.StatusBadRequest, Provider: p.Name(),
		}
	}
	req = rewrittenReq

	apiKey := p.Provider.ResolveAPIKey(ctx)

	body := p.buildResponsesRequest(req)
	body.Stream = true

	if req.PreviousResponseID != "" {
		body.PreviousResponseID = req.PreviousResponseID
	} else if prevID, ok := PreviousResponseIDFromContext(ctx); ok {
		body.PreviousResponseID = prevID
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal responses api stream request: %w", err)
	}

	httpReq, err := p.Provider.NewRequest(ctx, http.MethodPost, "/v1/responses", bytes.NewReader(payload), apiKey)
	if err != nil {
		return nil, err
	}

	resp, err := p.Provider.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	return streamResponsesSSE(ctx, resp.Body, p.Name()), nil
}

// streamResponsesSSE parses SSE events from the Responses API.
func streamResponsesSSE(ctx context.Context, body io.ReadCloser, providerName string) <-chan llm.StreamChunk {
	ch := make(chan llm.StreamChunk)
	go func() {
		defer body.Close()
		defer close(ch)
		reader := bufio.NewReader(body)

		var currentID string
		var currentModel string
		toolCallName := map[string]string{}
		toolCallArgs := map[string][]byte{}
		finishSent := false

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{Err: &types.Error{
						Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: providerName,
					}}:
					}
				}
				return
			}
			line = strings.TrimSpace(line)

			// Parse event type
			if strings.HasPrefix(line, "event:") {
				continue
			}
			if !strings.HasPrefix(line, "data:") || line == "" {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				return
			}

			var event map[string]any
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			eventType, _ := event["type"].(string)

			switch eventType {
			case "response.created", "response.in_progress":
				if resp, ok := event["response"].(map[string]any); ok {
					if id, ok := resp["id"].(string); ok {
						currentID = id
					}
					if model, ok := resp["model"].(string); ok {
						currentModel = model
					}
				}

			case "response.output_item.added":
				// 从 output_item.added 事件提取函数调用名称
				// 官方文档: 函数名在此事件的 item.name 中，而非 arguments.delta 中
				item, _ := event["item"].(map[string]any)
				if item == nil {
					continue
				}
				itemType, _ := item["type"].(string)
				if itemType != "function_call" {
					continue
				}
				itemID, _ := item["id"].(string)
				name, _ := item["name"].(string)
				if itemID != "" && name != "" {
					toolCallName[itemID] = name
				}

			case "response.output_text.delta":
				delta, _ := event["delta"].(string)
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					ID: currentID, Provider: providerName, Model: currentModel,
					Delta: types.Message{Role: llm.RoleAssistant, Content: delta},
				}:
				}

			case "response.refusal.delta":
				delta, _ := event["delta"].(string)
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					ID: currentID, Provider: providerName, Model: currentModel,
					Delta: types.Message{Role: llm.RoleAssistant, Refusal: &delta},
				}:
				}

			case "response.function_call_arguments.delta":
				delta, _ := event["delta"].(string)
				itemID, _ := event["item_id"].(string)
				if itemID == "" {
					continue
				}
				toolCallArgs[itemID] = append(toolCallArgs[itemID], []byte(delta)...)

			case "response.completed":
				if resp, ok := event["response"].(map[string]any); ok {
					if usage, ok := resp["usage"].(map[string]any); ok {
						inputTokens, _ := usage["input_tokens"].(float64)
						outputTokens, _ := usage["output_tokens"].(float64)
						totalTokens, _ := usage["total_tokens"].(float64)
						select {
						case <-ctx.Done():
							return
						case ch <- llm.StreamChunk{
							ID: currentID, Provider: providerName, Model: currentModel,
							FinishReason: func() string {
								if finishSent {
									return ""
								}
								finishSent = true
								return "stop"
							}(),
							Usage: &llm.ChatUsage{
								PromptTokens:     int(inputTokens),
								CompletionTokens: int(outputTokens),
								TotalTokens:      int(totalTokens),
							},
						}:
						}
					}
				}

			case "response.output_text.done":
				if finishSent {
					continue
				}
				finishSent = true
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					ID: currentID, Provider: providerName, Model: currentModel,
					FinishReason: "stop",
				}:
				}

			case "response.function_call_arguments.done":
				itemID, _ := event["item_id"].(string)
				if itemID == "" {
					continue
				}
				arguments := toolCallArgs[itemID]
				name := toolCallName[itemID]
				delete(toolCallArgs, itemID)
				delete(toolCallName, itemID)
				if finishSent {
					continue
				}
				finishSent = true
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					ID: currentID, Provider: providerName, Model: currentModel,
					Delta: types.Message{
						Role: llm.RoleAssistant,
						ToolCalls: []types.ToolCall{{
							ID:        itemID,
							Name:      name,
							Arguments: json.RawMessage(arguments),
						}},
					},
					FinishReason: "tool_calls",
				}:
				}

			case "error":
				errMsg, _ := event["message"].(string)
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{Err: &types.Error{
					Code: llm.ErrUpstreamError, Message: errMsg,
					HTTPStatus: http.StatusBadGateway, Provider: providerName,
				}}:
				}
				return
			}
		}
	}()
	return ch
}
