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
	Model                string              `json:"model"`
	Input                any                 `json:"input"` // string or []ResponsesInputItem
	Instructions         string              `json:"instructions,omitempty"`
	MaxOutputTokens      *int                `json:"max_output_tokens,omitempty"`
	Temperature          *float32            `json:"temperature,omitempty"`
	TopP                 *float32            `json:"top_p,omitempty"`
	Tools                []any               `json:"tools,omitempty"`
	ToolChoice           any                 `json:"tool_choice,omitempty"`
	ParallelToolCalls    *bool               `json:"parallel_tool_calls,omitempty"`
	PreviousResponseID   string              `json:"previous_response_id,omitempty"`
	Store                *bool               `json:"store,omitempty"`
	Metadata             map[string]string   `json:"metadata,omitempty"`
	PromptCacheKey       string              `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention string              `json:"prompt_cache_retention,omitempty"`
	Include              []string            `json:"include,omitempty"`
	Truncation           string              `json:"truncation,omitempty"` // "auto" or "disabled"
	Reasoning            *responsesReasoning `json:"reasoning,omitempty"`
	Text                 *responsesTextParam `json:"text,omitempty"`
	ServiceTier          *string             `json:"service_tier,omitempty"`
	User                 string              `json:"user,omitempty"`
	Stream               bool                `json:"stream,omitempty"`
	TopLogProbs          *int                `json:"top_logprobs,omitempty"`
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
	Type      string `json:"type"` // "function_call"
	ID        string `json:"id"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // Responses API 要求 string，不是 object
}

// customToolCallInputItem represents a custom tool call in the input.
type customToolCallInputItem struct {
	Type   string `json:"type"` // "custom_tool_call"
	CallID string `json:"call_id"`
	Name   string `json:"name"`
	Input  string `json:"input"`
}

// functionCallOutputItem represents a function call output in the input.
type functionCallOutputItem struct {
	Type   string `json:"type"` // "function_call_output"
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

// customToolCallOutputItem represents a custom tool call output in the input.
type customToolCallOutputItem struct {
	Type   string `json:"type"` // "custom_tool_call_output"
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

// responsesReasoningInputItem represents a reasoning item re-sent for manual round-tripping.
type responsesReasoningInputItem struct {
	Type             string             `json:"type"` // "reasoning"
	ID               string             `json:"id,omitempty"`
	Status           string             `json:"status,omitempty"`
	Summary          []responsesContent `json:"summary,omitempty"`
	Content          []responsesContent `json:"content,omitempty"`
	EncryptedContent string             `json:"encrypted_content,omitempty"`
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
	Type             string             `json:"type"` // "message", "function_call", "custom_tool_call", "reasoning"
	ID               string             `json:"id"`
	Status           string             `json:"status,omitempty"`
	Role             string             `json:"role,omitempty"`
	Content          []responsesContent `json:"content,omitempty"`
	EncryptedContent string             `json:"encrypted_content,omitempty"`
	// function_call fields
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Input     string          `json:"input,omitempty"`
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
	promptCacheRetention, cacheErr := providerbase.NormalizeOpenAIPromptCacheRetention(req.PromptCacheRetention, p.Name())
	if cacheErr != nil {
		return nil, cacheErr
	}

	reqCopy := *req
	reqCopy.PromptCacheRetention = promptCacheRetention
	body := p.buildResponsesRequest(&reqCopy)

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
		Model:                providerbase.ChooseModel(req, p.openaiCfg.Model, "gpt-5.2"),
		ToolChoice:           req.ToolChoice,
		Store:                req.Store,
		Metadata:             req.Metadata,
		PromptCacheKey:       strings.TrimSpace(req.PromptCacheKey),
		PromptCacheRetention: strings.TrimSpace(req.PromptCacheRetention),
		Include:              append([]string(nil), req.Include...),
		Truncation:           strings.TrimSpace(req.Truncation),
		User:                 req.User,
		ServiceTier:          req.ServiceTier,
		TopLogProbs:          req.TopLogProbs,
		ParallelToolCalls:    req.ParallelToolCalls,
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

	// Reasoning: preserve official Responses reasoning controls and request opaque state for round-trip.
	if effort, ok := chooseResponsesReasoningEffort(req); ok {
		body.Reasoning = &responsesReasoning{
			Effort:  effort,
			Summary: chooseResponsesReasoningSummary(req),
		}
		body.Include = ensureString(body.Include, "reasoning.encrypted_content")
	} else if summary := chooseResponsesReasoningSummary(req); summary != "" {
		body.Reasoning = &responsesReasoning{Summary: summary}
		body.Include = ensureString(body.Include, "reasoning.encrypted_content")
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

		tool := buildResponsesToolDefinition(t)
		if len(tool) == 0 {
			continue
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

func buildResponsesToolDefinition(t types.ToolSchema) map[string]any {
	toolType := strings.TrimSpace(t.Type)
	if toolType == "" {
		toolType = types.ToolTypeFunction
	}
	switch toolType {
	case types.ToolTypeCustom:
		tool := map[string]any{
			"type": types.ToolTypeCustom,
			"name": t.Name,
		}
		if t.Description != "" {
			tool["description"] = t.Description
		}
		if format := convertCustomToolFormat(t.Format); len(format) > 0 {
			tool["format"] = format
		}
		return tool
	default:
		tool := map[string]any{
			"type": types.ToolTypeFunction,
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
		if t.Strict != nil {
			tool["strict"] = *t.Strict
		}
		return tool
	}
}

func convertCustomToolFormat(format *types.ToolFormat) map[string]any {
	if format == nil {
		return nil
	}
	out := map[string]any{}
	if v := strings.TrimSpace(format.Type); v != "" {
		out["type"] = v
	}
	if v := strings.TrimSpace(format.Syntax); v != "" {
		out["syntax"] = v
	}
	if v := strings.TrimSpace(format.Definition); v != "" {
		out["definition"] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
	toolCallTypes := buildToolCallTypeIndex(msgs)
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
			for _, reasoningItem := range buildOpenAIResponsesReasoningItems(m) {
				items = append(items, reasoningItem)
			}
			if len(m.ToolCalls) > 0 {
				if m.Content != "" {
					items = append(items, responsesInputItem{
						Type:    "message",
						Role:    "assistant",
						Content: m.Content,
					})
				}
				for _, tc := range m.ToolCalls {
					switch normalizeToolType(tc.Type) {
					case types.ToolTypeCustom:
						items = append(items, customToolCallInputItem{
							Type:   "custom_tool_call",
							CallID: tc.ID,
							Name:   tc.Name,
							Input:  tc.Input,
						})
					default:
						items = append(items, functionCallInputItem{
							Type:      "function_call",
							ID:        tc.ID,
							CallID:    tc.ID,
							Name:      tc.Name,
							Arguments: string(tc.Arguments),
						})
					}
				}
			} else {
				items = append(items, responsesInputItem{
					Role:    "assistant",
					Content: buildInputContent(m),
				})
			}
		case llm.RoleTool:
			switch toolCallTypes[m.ToolCallID] {
			case types.ToolTypeCustom:
				items = append(items, customToolCallOutputItem{
					Type:   "custom_tool_call_output",
					CallID: m.ToolCallID,
					Output: m.Content,
				})
			default:
				items = append(items, functionCallOutputItem{
					Type:   "function_call_output",
					CallID: m.ToolCallID,
					Output: m.Content,
				})
			}
		default:
			items = append(items, responsesInputItem{
				Role:    string(m.Role),
				Content: m.Content,
			})
		}
	}
	return items
}

func buildToolCallTypeIndex(msgs []types.Message) map[string]string {
	out := make(map[string]string)
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			if strings.TrimSpace(tc.ID) == "" {
				continue
			}
			out[tc.ID] = normalizeToolType(tc.Type)
		}
	}
	return out
}

func normalizeToolType(toolType string) string {
	switch strings.ToLower(strings.TrimSpace(toolType)) {
	case "", types.ToolTypeFunction:
		return types.ToolTypeFunction
	case types.ToolTypeCustom:
		return types.ToolTypeCustom
	default:
		return types.ToolTypeFunction
	}
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func buildOpenAIResponsesReasoningItems(m types.Message) []responsesReasoningInputItem {
	var items []responsesReasoningInputItem
	groupedSummaries := make(map[string][]responsesContent)
	for _, summary := range m.ReasoningSummaries {
		provider := strings.TrimSpace(summary.Provider)
		if provider != "" && provider != "openai" {
			continue
		}
		id := strings.TrimSpace(summary.ID)
		groupedSummaries[id] = append(groupedSummaries[id], responsesContent{
			Type: "summary_text",
			Text: summary.Text,
		})
	}

	for _, opaque := range m.OpaqueReasoning {
		provider := strings.TrimSpace(opaque.Provider)
		if provider != "" && provider != "openai" {
			continue
		}
		if strings.TrimSpace(opaque.Kind) != "encrypted_content" || strings.TrimSpace(opaque.State) == "" {
			continue
		}
		itemID := strings.TrimSpace(opaque.ID)
		if itemID == "" {
			continue
		}
		item := responsesReasoningInputItem{
			Type:             "reasoning",
			ID:               itemID,
			Status:           strings.TrimSpace(opaque.Status),
			Summary:          groupedSummaries[itemID],
			EncryptedContent: opaque.State,
		}
		if len(item.Summary) == 0 && m.ReasoningContent != nil && strings.TrimSpace(*m.ReasoningContent) != "" {
			item.Summary = []responsesContent{{Type: "summary_text", Text: strings.TrimSpace(*m.ReasoningContent)}}
		}
		items = append(items, item)
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

func chooseResponsesReasoningSummary(req *llm.ChatRequest) string {
	if req == nil {
		return ""
	}
	switch strings.TrimSpace(req.ReasoningSummary) {
	case "auto", "concise", "detailed":
		return strings.TrimSpace(req.ReasoningSummary)
	}
	if _, ok := chooseResponsesReasoningEffort(req); ok {
		return "auto"
	}
	return ""
}

func ensureString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.TrimSpace(existing) == value {
			return values
		}
	}
	return append(values, value)
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
			msg := buildResponsesMessage(output)
			if len(choices) > 0 && choices[len(choices)-1].Message.Role == llm.RoleAssistant &&
				choices[len(choices)-1].Message.Content == "" && len(choices[len(choices)-1].Message.ToolCalls) == 0 {
				last := &choices[len(choices)-1]
				last.Message.Content += msg.Content
				last.Message.Annotations = append(last.Message.Annotations, msg.Annotations...)
				if msg.Refusal != nil {
					last.Message.Refusal = msg.Refusal
				}
				last.FinishReason = mapResponsesStatus(resp.Status)
			} else {
				choices = append(choices, llm.ChatChoice{
					Index: choiceIdx, FinishReason: mapResponsesStatus(resp.Status), Message: msg,
				})
				choiceIdx++
			}

		case "reasoning":
			choice := ensureResponsesAssistantChoice(&choices, &choiceIdx)
			mergeResponsesReasoningItem(&choice.Message, output, provider)
			if choice.FinishReason == "" {
				choice.FinishReason = mapResponsesStatus(resp.Status)
			}

		case "function_call":
			choice := ensureResponsesAssistantChoice(&choices, &choiceIdx)
			choice.Message.ToolCalls = append(choice.Message.ToolCalls, types.ToolCall{
				ID:        firstNonEmptyString(output.CallID, output.ID),
				Type:      types.ToolTypeFunction,
				Name:      output.Name,
				Arguments: output.Arguments,
			})
			choice.FinishReason = "tool_calls"

		case "custom_tool_call":
			choice := ensureResponsesAssistantChoice(&choices, &choiceIdx)
			choice.Message.ToolCalls = append(choice.Message.ToolCalls, types.ToolCall{
				ID:    firstNonEmptyString(output.CallID, output.ID),
				Type:  types.ToolTypeCustom,
				Name:  output.Name,
				Input: output.Input,
			})
			choice.FinishReason = "tool_calls"
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

func buildResponsesMessage(output responsesOutputItem) types.Message {
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
	return msg
}

func ensureResponsesAssistantChoice(choices *[]llm.ChatChoice, choiceIdx *int) *llm.ChatChoice {
	if len(*choices) == 0 || (*choices)[len(*choices)-1].Message.Role != llm.RoleAssistant {
		*choices = append(*choices, llm.ChatChoice{
			Index: *choiceIdx,
			Message: types.Message{
				Role: llm.RoleAssistant,
			},
		})
		*choiceIdx = *choiceIdx + 1
	}
	return &(*choices)[len(*choices)-1]
}

func mergeResponsesReasoningItem(msg *types.Message, output responsesOutputItem, provider string) {
	if msg == nil {
		return
	}
	if msg.Role == "" {
		msg.Role = llm.RoleAssistant
	}
	if summaries := responsesReasoningSummaries(output, provider); len(summaries) > 0 {
		msg.ReasoningSummaries = append(msg.ReasoningSummaries, summaries...)
	}
	if opaque := responsesOpaqueReasoning(output, provider); len(opaque) > 0 {
		msg.OpaqueReasoning = append(msg.OpaqueReasoning, opaque...)
	}
	if text := responsesReasoningDisplayText(output); text != "" {
		appendReasoningText(msg, text)
	}
}

func responsesReasoningDisplayText(output responsesOutputItem) string {
	if text := joinResponsesContentText(output.Content, "reasoning_text"); text != "" {
		return text
	}
	return joinResponsesContentText(output.Summary, "summary_text")
}

func responsesReasoningSummaries(output responsesOutputItem, provider string) []types.ReasoningSummary {
	if len(output.Summary) == 0 {
		return nil
	}
	summaries := make([]types.ReasoningSummary, 0, len(output.Summary))
	for _, part := range output.Summary {
		if strings.TrimSpace(part.Text) == "" {
			continue
		}
		kind := strings.TrimSpace(part.Type)
		if kind == "" {
			kind = "summary_text"
		}
		summaries = append(summaries, types.ReasoningSummary{
			Provider: provider,
			ID:       output.ID,
			Kind:     kind,
			Text:     part.Text,
		})
	}
	return summaries
}

func responsesOpaqueReasoning(output responsesOutputItem, provider string) []types.OpaqueReasoning {
	if strings.TrimSpace(output.EncryptedContent) == "" {
		return nil
	}
	return []types.OpaqueReasoning{{
		Provider: provider,
		ID:       output.ID,
		Kind:     "encrypted_content",
		State:    output.EncryptedContent,
		Status:   output.Status,
	}}
}

func joinResponsesContentText(parts []responsesContent, partType string) string {
	var out []string
	for _, part := range parts {
		if strings.TrimSpace(part.Text) == "" {
			continue
		}
		if partType != "" && strings.TrimSpace(part.Type) != partType {
			continue
		}
		out = append(out, part.Text)
	}
	return strings.Join(out, "\n\n")
}

func appendReasoningText(msg *types.Message, text string) {
	text = strings.TrimSpace(text)
	if msg == nil || text == "" {
		return
	}
	if msg.ReasoningContent == nil || strings.TrimSpace(*msg.ReasoningContent) == "" {
		msg.ReasoningContent = stringPtr(text)
		return
	}
	joined := strings.TrimSpace(*msg.ReasoningContent)
	if joined == text {
		return
	}
	joined = strings.TrimSpace(joined + "\n\n" + text)
	msg.ReasoningContent = stringPtr(joined)
}

func stringPtr(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	out := s
	return &out
}

func responsesOutputItemFromAny(v any) (responsesOutputItem, bool) {
	raw, err := json.Marshal(v)
	if err != nil {
		return responsesOutputItem{}, false
	}
	var item responsesOutputItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return responsesOutputItem{}, false
	}
	return item, true
}

func responsesResponseFromAny(v any) (openAIResponsesResponse, bool) {
	raw, err := json.Marshal(v)
	if err != nil {
		return openAIResponsesResponse{}, false
	}
	var resp openAIResponsesResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return openAIResponsesResponse{}, false
	}
	return resp, true
}

func emitResponsesReasoningChunk(
	ctx context.Context,
	ch chan<- llm.StreamChunk,
	currentID, providerName, currentModel string,
	output responsesOutputItem,
) bool {
	delta := types.Message{Role: llm.RoleAssistant}
	mergeResponsesReasoningItem(&delta, output, providerName)
	if delta.ReasoningContent == nil && len(delta.ReasoningSummaries) == 0 && len(delta.OpaqueReasoning) == 0 {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case ch <- llm.StreamChunk{
		ID:       currentID,
		Provider: providerName,
		Model:    currentModel,
		Delta:    delta,
	}:
		return true
	}
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
	promptCacheRetention, cacheErr := providerbase.NormalizeOpenAIPromptCacheRetention(req.PromptCacheRetention, p.Name())
	if cacheErr != nil {
		return nil, cacheErr
	}

	reqCopy := *req
	reqCopy.PromptCacheRetention = promptCacheRetention

	body := p.buildResponsesRequest(&reqCopy)
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
		toolCallType := map[string]string{}
		toolCallID := map[string]string{}
		toolCallArgs := map[string][]byte{}
		seenReasoning := map[string]bool{}
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
				// 从 output_item.added 事件提取工具调用元数据。
				item, _ := event["item"].(map[string]any)
				if item == nil {
					continue
				}
				itemType, _ := item["type"].(string)
				switch itemType {
				case "function_call":
					itemID, _ := item["id"].(string)
					name, _ := item["name"].(string)
					callID, _ := item["call_id"].(string)
					if itemID != "" && name != "" {
						toolCallName[itemID] = name
						toolCallType[itemID] = types.ToolTypeFunction
						if callID != "" {
							toolCallID[itemID] = callID
						}
					}
				case "custom_tool_call":
					itemID, _ := item["id"].(string)
					name, _ := item["name"].(string)
					callID, _ := item["call_id"].(string)
					if itemID != "" && name != "" {
						toolCallName[itemID] = name
						toolCallType[itemID] = types.ToolTypeCustom
						if callID != "" {
							toolCallID[itemID] = callID
						}
					}
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

			case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
				delta, _ := event["delta"].(string)
				if strings.TrimSpace(delta) == "" {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					ID: currentID, Provider: providerName, Model: currentModel,
					Delta: types.Message{Role: llm.RoleAssistant, ReasoningContent: stringPtr(delta)},
				}:
				}

			case "response.function_call_arguments.delta":
				delta, _ := event["delta"].(string)
				itemID, _ := event["item_id"].(string)
				if itemID == "" {
					continue
				}
				toolCallArgs[itemID] = append(toolCallArgs[itemID], []byte(delta)...)

			case "response.custom_tool_call_input.delta":
				delta, _ := event["delta"].(string)
				itemID, _ := event["item_id"].(string)
				if itemID == "" {
					continue
				}
				toolCallArgs[itemID] = append(toolCallArgs[itemID], []byte(delta)...)

			case "response.output_item.done":
				item, _ := event["item"].(map[string]any)
				if item == nil {
					continue
				}
				itemType, _ := item["type"].(string)
				output, ok := responsesOutputItemFromAny(item)
				if !ok {
					continue
				}
				switch itemType {
				case "reasoning":
					if emitResponsesReasoningChunk(ctx, ch, currentID, providerName, currentModel, output) {
						seenReasoning[output.ID] = true
					}
				case "custom_tool_call":
					callID := firstNonEmptyString(output.CallID, output.ID)
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{
						ID: currentID, Provider: providerName, Model: currentModel,
						Delta: types.Message{
							Role: llm.RoleAssistant,
							ToolCalls: []types.ToolCall{{
								ID:    callID,
								Type:  types.ToolTypeCustom,
								Name:  output.Name,
								Input: output.Input,
							}},
						},
						FinishReason: "tool_calls",
					}:
					}
				}

			case "response.completed":
				if resp, ok := event["response"].(map[string]any); ok {
					if completedResp, ok := responsesResponseFromAny(resp); ok {
						if completedResp.ID != "" {
							currentID = completedResp.ID
						}
						if completedResp.Model != "" {
							currentModel = completedResp.Model
						}
						for _, output := range completedResp.Output {
							if output.Type != "reasoning" {
								continue
							}
							if seenReasoning[output.ID] {
								continue
							}
							if emitResponsesReasoningChunk(ctx, ch, currentID, providerName, currentModel, output) {
								seenReasoning[output.ID] = true
							}
						}
					}
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
				callID := firstNonEmptyString(toolCallID[itemID], itemID)
				delete(toolCallArgs, itemID)
				delete(toolCallName, itemID)
				delete(toolCallType, itemID)
				delete(toolCallID, itemID)
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
							ID:        callID,
							Type:      types.ToolTypeFunction,
							Name:      name,
							Arguments: json.RawMessage(arguments),
						}},
					},
					FinishReason: "tool_calls",
				}:
				}

			case "response.custom_tool_call_input.done":
				itemID, _ := event["item_id"].(string)
				if itemID == "" {
					continue
				}
				input := string(toolCallArgs[itemID])
				name := toolCallName[itemID]
				callID := firstNonEmptyString(toolCallID[itemID], itemID)
				delete(toolCallArgs, itemID)
				delete(toolCallName, itemID)
				delete(toolCallType, itemID)
				delete(toolCallID, itemID)
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
							ID:    callID,
							Type:  types.ToolTypeCustom,
							Name:  name,
							Input: input,
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
