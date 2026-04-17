package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	openaiofficial "github.com/BaSui01/agentflow/llm/internal/openaiofficial"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"github.com/BaSui01/agentflow/types"
	openaisdk "github.com/openai/openai-go/v3"
	openaisdkoption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
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

func (p *OpenAIProvider) sdkClient(ctx context.Context) openaisdk.Client {
	return openaiofficial.NewClient(p.openaiCfg, p.Provider.ResolveAPIKey(ctx), p.Provider.Client)
}

func (p *OpenAIProvider) mapSDKError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *openaisdk.Error
	if errors.As(err, &apiErr) {
		return providerbase.MapHTTPError(apiErr.StatusCode, apiErr.RawJSON(), p.Name())
	}
	return &types.Error{
		Code:       llm.ErrUpstreamError,
		Message:    err.Error(),
		Cause:      err,
		HTTPStatus: http.StatusBadGateway,
		Retryable:  true,
		Provider:   p.Name(),
	}
}

func (p *OpenAIProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	client := p.sdkClient(ctx)
	_, err := client.Models.List(ctx)
	latency := time.Since(start)
	if err != nil {
		return &llm.HealthStatus{Healthy: false, Latency: latency}, p.mapSDKError(err)
	}
	return &llm.HealthStatus{Healthy: true, Latency: latency}, nil
}

func (p *OpenAIProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	client := p.sdkClient(ctx)
	page, err := client.Models.List(ctx)
	if err != nil {
		return nil, p.mapSDKError(err)
	}

	models := make([]llm.Model, 0, len(page.Data))
	for _, m := range page.Data {
		models = append(models, llm.Model{
			ID:      m.ID,
			Object:  string(m.Object),
			Created: m.Created,
			OwnedBy: m.OwnedBy,
		})
	}
	return models, nil
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

	return p.completionWithResponsesAPI(ctx, req)
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
	Conversation         string              `json:"conversation,omitempty"`
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

// responsesReasoningInputItem represents a reasoning item re-sent for manual round-tripping.
type responsesReasoningInputItem struct {
	Type             string                   `json:"type"` // "reasoning"
	ID               string                   `json:"id,omitempty"`
	Status           string                   `json:"status,omitempty"`
	Summary          []responsesReasoningPart `json:"summary"`
	Content          []responsesReasoningPart `json:"content,omitempty"`
	EncryptedContent string                   `json:"encrypted_content,omitempty"`
}

type responsesReasoningPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// --- Completion & Helper Methods ---

// completionWithResponsesAPI 使用新的 Responses API (/v1/responses).
func (p *OpenAIProvider) completionWithResponsesAPI(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	promptCacheRetention, cacheErr := providerbase.NormalizeOpenAIPromptCacheRetention(req.PromptCacheRetention, p.Name())
	if cacheErr != nil {
		return nil, cacheErr
	}

	reqCopy := *req
	reqCopy.PromptCacheRetention = promptCacheRetention
	body := p.buildResponsesRequest(&reqCopy)
	params := p.buildResponsesParams(&reqCopy, body)

	// 从 context 或 request 获取 previous_response_id
	if req.PreviousResponseID != "" {
		body.PreviousResponseID = req.PreviousResponseID
		params.PreviousResponseID = param.NewOpt(req.PreviousResponseID)
	} else if prevID, ok := PreviousResponseIDFromContext(ctx); ok {
		body.PreviousResponseID = prevID
		params.PreviousResponseID = param.NewOpt(prevID)
	}
	body.Conversation = strings.TrimSpace(req.ConversationID)
	if body.Conversation != "" {
		params.Conversation = responses.ResponseNewParamsConversationUnion{
			OfString: param.NewOpt(body.Conversation),
		}
	}
	llm.ReportProviderPromptUsage(ctx, llm.ProviderPromptUsageReport{
		Provider:     p.Name(),
		Model:        body.Model,
		API:          "responses",
		PromptTokens: countResponsesPromptTokens(body),
	})

	client := p.sdkClient(ctx)
	responsesResp, err := client.Responses.New(ctx, params, responseRequestOptions(body)...)
	if err != nil {
		return nil, p.mapSDKError(err)
	}

	return toResponsesAPIChatResponse(responsesResp, p.Name()), nil
}

func (p *OpenAIProvider) buildResponsesParams(req *llm.ChatRequest, body openAIResponsesRequest) responses.ResponseNewParams {
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(body.Model),
	}
	if body.Instructions != "" {
		params.Instructions = param.NewOpt(body.Instructions)
	}
	if body.MaxOutputTokens != nil {
		params.MaxOutputTokens = param.NewOpt(int64(*body.MaxOutputTokens))
	}
	if body.Temperature != nil {
		params.Temperature = param.NewOpt(float64(*body.Temperature))
	}
	if body.TopP != nil {
		params.TopP = param.NewOpt(float64(*body.TopP))
	}
	if body.ParallelToolCalls != nil {
		params.ParallelToolCalls = param.NewOpt(*body.ParallelToolCalls)
	}
	if body.Store != nil {
		params.Store = param.NewOpt(*body.Store)
	}
	if body.PromptCacheKey != "" {
		params.PromptCacheKey = param.NewOpt(body.PromptCacheKey)
	}
	if body.User != "" {
		params.User = param.NewOpt(body.User)
	}
	if body.ServiceTier != nil && strings.TrimSpace(*body.ServiceTier) != "" {
		params.ServiceTier = responses.ResponseNewParamsServiceTier(strings.TrimSpace(*body.ServiceTier))
	}
	if body.TopLogProbs != nil {
		params.TopLogprobs = param.NewOpt(int64(*body.TopLogProbs))
	}
	if len(body.Metadata) > 0 {
		params.Metadata = body.Metadata
	}
	if body.PromptCacheRetention != "" {
		switch strings.ToLower(strings.TrimSpace(body.PromptCacheRetention)) {
		case "in_memory", "in-memory":
			params.PromptCacheRetention = responses.ResponseNewParamsPromptCacheRetentionInMemory
		case "24h":
			params.PromptCacheRetention = responses.ResponseNewParamsPromptCacheRetention24h
		}
	}
	if body.Truncation != "" {
		params.Truncation = responses.ResponseNewParamsTruncation(body.Truncation)
	}
	if len(body.Include) > 0 {
		params.Include = make([]responses.ResponseIncludable, 0, len(body.Include))
		for _, include := range body.Include {
			include = strings.TrimSpace(include)
			if include == "" {
				continue
			}
			params.Include = append(params.Include, responses.ResponseIncludable(include))
		}
	}
	if len(body.Tools) > 0 {
		params.Tools = make([]responses.ToolUnionParam, 0, len(body.Tools))
		for _, tool := range body.Tools {
			params.Tools = append(params.Tools, decodeSDKParam[responses.ToolUnionParam](tool))
		}
	}
	if body.ToolChoice != nil {
		params.ToolChoice = buildSDKResponseToolChoice(body.ToolChoice, body.Tools)
	}
	if body.Reasoning != nil {
		params.Reasoning = shared.ReasoningParam{
			Effort:  shared.ReasoningEffort(strings.TrimSpace(body.Reasoning.Effort)),
			Summary: shared.ReasoningSummary(strings.TrimSpace(body.Reasoning.Summary)),
		}
	}
	if body.Text != nil {
		text := responses.ResponseTextConfigParam{}
		if body.Text.Verbosity != "" {
			text.Verbosity = responses.ResponseTextConfigVerbosity(body.Text.Verbosity)
		}
		if body.Text.Format != nil {
			text.Format = decodeSDKParam[responses.ResponseFormatTextConfigUnionParam](body.Text.Format)
		}
		params.Text = text
	}
	switch input := body.Input.(type) {
	case string:
		if strings.TrimSpace(input) != "" {
			params.Input = responses.ResponseNewParamsInputUnion{OfString: param.NewOpt(input)}
		}
	case []any:
		params.Input = responses.ResponseNewParamsInputUnion{
			OfInputItemList: decodeSliceSDKParam[responses.ResponseInputItemUnionParam](input),
		}
	}
	return params
}

func decodeSDKParam[T any](value any) T {
	raw, err := json.Marshal(value)
	if err != nil {
		var zero T
		return zero
	}
	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		var zero T
		return zero
	}
	return result
}

func decodeSliceSDKParam[T any](items []any) []T {
	if len(items) == 0 {
		return nil
	}
	out := make([]T, 0, len(items))
	for _, item := range items {
		out = append(out, decodeSDKParam[T](item))
	}
	return out
}

func responseRequestOptions(body openAIResponsesRequest) []openaisdkoption.RequestOption {
	if strings.TrimSpace(body.PromptCacheRetention) == "" {
		return nil
	}
	return []openaisdkoption.RequestOption{
		openaisdkoption.WithJSONSet("prompt_cache_retention", body.PromptCacheRetention),
	}
}

func buildSDKResponseToolChoice(choice any, tools []any) responses.ResponseNewParamsToolChoiceUnion {
	if choice == nil {
		return responses.ResponseNewParamsToolChoiceUnion{}
	}
	if _, ok := choice.(map[string]any); !ok {
		return decodeSDKParam[responses.ResponseNewParamsToolChoiceUnion](choice)
	}
	normalized := providerbase.NormalizeToolChoice(choice)
	switch normalized.Mode {
	case "tool":
		name := strings.TrimSpace(normalized.SpecificName)
		if name == "" {
			return decodeSDKParam[responses.ResponseNewParamsToolChoiceUnion](choice)
		}
		if toolType := findResponseToolTypeByName(tools, name); toolType == types.ToolTypeCustom {
			return responses.ResponseNewParamsToolChoiceUnion{
				OfCustomTool: &responses.ToolChoiceCustomParam{Name: name},
			}
		}
		return responses.ResponseNewParamsToolChoiceUnion{
			OfFunctionTool: &responses.ToolChoiceFunctionParam{Name: name},
		}
	case "any":
		allowedTools := buildAllowedToolsChoice(tools)
		if len(allowedTools) == 0 {
			return decodeSDKParam[responses.ResponseNewParamsToolChoiceUnion](choice)
		}
		return responses.ResponseNewParamsToolChoiceUnion{
			OfAllowedTools: &responses.ToolChoiceAllowedParam{
				Mode:  responses.ToolChoiceAllowedModeRequired,
				Tools: allowedTools,
			},
		}
	case "auto":
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
		}
	case "none":
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsNone),
		}
	default:
		return decodeSDKParam[responses.ResponseNewParamsToolChoiceUnion](choice)
	}
}

func buildAllowedToolsChoice(tools []any) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		toolMap, ok := tool.(map[string]any)
		if !ok {
			continue
		}
		entry := map[string]any{}
		if toolType, _ := toolMap["type"].(string); strings.TrimSpace(toolType) != "" {
			entry["type"] = strings.TrimSpace(toolType)
		}
		if name, _ := toolMap["name"].(string); strings.TrimSpace(name) != "" {
			entry["name"] = strings.TrimSpace(name)
		}
		if len(entry) > 0 {
			out = append(out, entry)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func findResponseToolTypeByName(tools []any, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	for _, tool := range tools {
		toolMap, ok := tool.(map[string]any)
		if !ok {
			continue
		}
		if toolName, _ := toolMap["name"].(string); strings.TrimSpace(toolName) == name {
			if toolType, _ := toolMap["type"].(string); strings.TrimSpace(toolType) != "" {
				return providerbase.NormalizeToolType(toolType)
			}
		}
	}
	return ""
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

	// 提取 system/developer message 作为 instructions
	// OpenAI Responses API 要求 instructions 参数，不能放在 input 里
	instructions, filteredMsgs := extractInstructionsFromMessages(req.Messages)
	if instructions != "" {
		body.Instructions = instructions
	}

	// 构建 input（不包含已提取的 system/developer message）
	body.Input = convertMessagesToResponsesInput(filteredMsgs)

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
		if providerbase.IsSearchToolPlaceholder(t.Name) {
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
	toolType := providerbase.NormalizeToolType(t.Type)
	switch toolType {
	case types.ToolTypeCustom:
		tool := map[string]any{
			"type": types.ToolTypeCustom,
			"name": t.Name,
		}
		if t.Description != "" {
			tool["description"] = t.Description
		}
		if format := providerbase.ConvertCustomToolFormat(t.Format); len(format) > 0 {
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
		if params := providerbase.ToolParametersSchemaMap(t.Parameters); len(params) > 0 {
			tool["parameters"] = params
		}
		if t.Strict != nil {
			tool["strict"] = *t.Strict
		}
		return tool
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

// extractInstructionsFromMessages extracts the first system/developer message as instructions
// and returns the instructions string along with the remaining messages.
// OpenAI Responses API requires the instructions parameter and rejects system messages in input.
func extractInstructionsFromMessages(msgs []types.Message) (string, []types.Message) {
	if len(msgs) == 0 {
		return "", msgs
	}

	instructions := make([]string, 0, len(msgs))
	remaining := make([]types.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == llm.RoleSystem || m.Role == llm.RoleDeveloper {
			if text := strings.TrimSpace(m.Content); text != "" {
				instructions = append(instructions, text)
			}
			continue
		}
		remaining = append(remaining, m)
	}
	if len(instructions) == 0 {
		return "", msgs
	}
	return strings.Join(instructions, "\n\n"), remaining
}

// convertMessagesToResponsesInput converts messages to Responses API input format.
// Note: system/developer messages are extracted as 'instructions' before this function
// is called, so they should be skipped here to avoid API errors.
func convertMessagesToResponsesInput(msgs []types.Message) []any {
	items := make([]any, 0, len(msgs))
	toolCallTypes := providerbase.BuildToolCallTypeIndex(msgs)
	for _, m := range msgs {
		switch m.Role {
		case llm.RoleSystem, llm.RoleDeveloper:
			// Skip: these are handled by extractInstructionsFromMessages
			// Responses API requires system prompt in 'instructions' field, not in input.
			continue
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
					responsesID := convertToResponsesToolCallID(tc.ID)
					switch providerbase.NormalizeToolType(tc.Type) {
					case types.ToolTypeCustom:
						items = append(items, customToolCallInputItem{
							Type:   "custom_tool_call",
							CallID: responsesID,
							Name:   tc.Name,
							Input:  tc.Input,
						})
					default:
						items = append(items, functionCallInputItem{
							Type:      "function_call",
							ID:        responsesID,
							CallID:    responsesID,
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
			// Skip tool outputs with empty ToolCallID to avoid API errors
			writeback, ok := providerbase.ToolOutputFromMessage(m, toolCallTypes)
			if !ok {
				continue
			}
			items = append(items, providerbase.BuildOpenAIResponsesToolOutputItem(writeback, convertToResponsesToolCallID))
		default:
			items = append(items, responsesInputItem{
				Role:    string(m.Role),
				Content: m.Content,
			})
		}
	}
	return items
}

// convertToResponsesToolCallID converts a Chat Completions tool call ID to Responses API format.
// Responses API requires function call IDs to start with "fc_" prefix.
// Chat Completions API uses "call_" prefix, so we need to convert.
func convertToResponsesToolCallID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return id
	}
	// Already in Responses API format
	if strings.HasPrefix(id, "fc_") {
		return id
	}
	// Convert Chat Completions "call_" prefix to Responses API "fc_" prefix
	if strings.HasPrefix(id, "call_") {
		return "fc_" + strings.TrimPrefix(id, "call_")
	}
	// For any other format, prefix with "fc_"
	return "fc_" + id
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
	groupedSummaries := make(map[string][]responsesReasoningPart)
	for _, summary := range m.ReasoningSummaries {
		provider := strings.TrimSpace(summary.Provider)
		if provider != "" && provider != "openai" {
			continue
		}
		id := strings.TrimSpace(summary.ID)
		groupedSummaries[id] = append(groupedSummaries[id], responsesReasoningPart{
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
			item.Summary = []responsesReasoningPart{{Type: "summary_text", Text: strings.TrimSpace(*m.ReasoningContent)}}
		}
		// OpenAI Responses API requires the summary field to be present (non-null).
		// Ensure it is never nil so JSON serialization always includes "summary": [].
		if item.Summary == nil {
			item.Summary = []responsesReasoningPart{}
		}
		items = append(items, item)
	}

	// Handle ReasoningSummaries that don't have corresponding OpaqueReasoning.
	// This can happen when the previous response didn't include encrypted_content
	// or when round-tripping from a different context.
	// Create standalone reasoning items for these orphaned summaries.
	usedIDs := make(map[string]bool)
	for _, opaque := range m.OpaqueReasoning {
		usedIDs[strings.TrimSpace(opaque.ID)] = true
	}
	for id, summaries := range groupedSummaries {
		if usedIDs[id] {
			continue // already handled above
		}
		if id == "" || len(summaries) == 0 {
			continue
		}
		items = append(items, responsesReasoningInputItem{
			Type:    "reasoning",
			ID:      id,
			Summary: summaries,
		})
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
func toResponsesAPIChatResponse(resp *responses.Response, provider string) *llm.ChatResponse {
	if resp == nil {
		return &llm.ChatResponse{Provider: provider}
	}
	var choices []llm.ChatChoice
	choiceIdx := 0

	for _, output := range resp.Output {
		switch output.Type {
		case "message":
			msg := buildResponsesMessage(output.AsMessage())
			if len(choices) > 0 && choices[len(choices)-1].Message.Role == llm.RoleAssistant &&
				choices[len(choices)-1].Message.Content == "" && len(choices[len(choices)-1].Message.ToolCalls) == 0 {
				last := &choices[len(choices)-1]
				last.Message.Content += msg.Content
				last.Message.Annotations = append(last.Message.Annotations, msg.Annotations...)
				if msg.Refusal != nil {
					last.Message.Refusal = msg.Refusal
				}
				last.FinishReason = mapResponsesStatus(string(resp.Status))
			} else {
				choices = append(choices, llm.ChatChoice{
					Index: choiceIdx, FinishReason: mapResponsesStatus(string(resp.Status)), Message: msg,
				})
				choiceIdx++
			}

		case "reasoning":
			choice := ensureResponsesAssistantChoice(&choices, &choiceIdx)
			mergeResponsesReasoningItem(&choice.Message, output.AsReasoning(), provider)
			if choice.FinishReason == "" {
				choice.FinishReason = mapResponsesStatus(string(resp.Status))
			}

		case "function_call":
			functionCall := output.AsFunctionCall()
			choice := ensureResponsesAssistantChoice(&choices, &choiceIdx)
			choice.Message.ToolCalls = append(choice.Message.ToolCalls,
				providerbase.NewFunctionToolCall(
					firstNonEmptyString(functionCall.CallID, functionCall.ID),
					functionCall.Name,
					functionCallArguments(functionCall),
				),
			)
			choice.FinishReason = "tool_calls"

		case "custom_tool_call":
			customToolCall := output.AsCustomToolCall()
			choice := ensureResponsesAssistantChoice(&choices, &choiceIdx)
			choice.Message.ToolCalls = append(choice.Message.ToolCalls,
				providerbase.NewCustomToolCall(
					firstNonEmptyString(customToolCall.CallID, customToolCall.ID),
					customToolCall.Name,
					customToolCall.Input,
				),
			)
			choice.FinishReason = "tool_calls"
		}
	}

	chatResp := &llm.ChatResponse{
		ID:          resp.ID,
		Provider:    provider,
		Model:       string(resp.Model),
		Choices:     choices,
		ServiceTier: string(resp.ServiceTier),
	}
	if resp.CreatedAt != 0 {
		chatResp.CreatedAt = time.Unix(int64(resp.CreatedAt), 0)
	}
	if usage := usageFromSDK(resp.Usage); usage != nil {
		chatResp.Usage = *usage
	}
	return chatResp
}

func functionCallArguments(output responses.ResponseFunctionToolCall) json.RawMessage {
	if strings.TrimSpace(output.Arguments) == "" {
		return nil
	}
	return json.RawMessage(output.Arguments)
}

func buildResponsesMessage(output responses.ResponseOutputMessage) types.Message {
	msg := types.Message{Role: llm.RoleAssistant}
	if role := strings.TrimSpace(string(output.Role)); role != "" {
		msg.Role = types.Role(role)
	}
	for _, content := range output.Content {
		switch content.Type {
		case "output_text":
			msg.Content += content.Text
			for _, ann := range content.Annotations {
				msg.Annotations = append(msg.Annotations, types.Annotation{
					Type:       ann.Type,
					StartIndex: int(ann.StartIndex),
					EndIndex:   int(ann.EndIndex),
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

func mergeResponsesReasoningItem(msg *types.Message, output responses.ResponseReasoningItem, provider string) {
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

func responsesReasoningDisplayText(output responses.ResponseReasoningItem) string {
	if text := joinResponsesReasoningContentText(output.Content); text != "" {
		return text
	}
	return joinResponsesReasoningSummaryText(output.Summary)
}

func responsesReasoningSummaries(output responses.ResponseReasoningItem, provider string) []types.ReasoningSummary {
	if len(output.Summary) == 0 {
		return nil
	}
	summaries := make([]types.ReasoningSummary, 0, len(output.Summary))
	for _, part := range output.Summary {
		if strings.TrimSpace(part.Text) == "" {
			continue
		}
		kind := strings.TrimSpace(string(part.Type))
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

func responsesOpaqueReasoning(output responses.ResponseReasoningItem, provider string) []types.OpaqueReasoning {
	if strings.TrimSpace(output.EncryptedContent) == "" {
		return nil
	}
	return []types.OpaqueReasoning{{
		Provider: provider,
		ID:       output.ID,
		Kind:     "encrypted_content",
		State:    output.EncryptedContent,
		Status:   string(output.Status),
	}}
}

func joinResponsesReasoningContentText(parts []responses.ResponseReasoningItemContent) string {
	var out []string
	for _, part := range parts {
		if strings.TrimSpace(part.Text) == "" {
			continue
		}
		out = append(out, part.Text)
	}
	return strings.Join(out, "\n\n")
}

func joinResponsesReasoningSummaryText(parts []responses.ResponseReasoningItemSummary) string {
	var out []string
	for _, part := range parts {
		if strings.TrimSpace(part.Text) == "" {
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

func emitResponsesReasoningChunk(
	ctx context.Context,
	ch chan<- llm.StreamChunk,
	currentID, providerName, currentModel string,
	output responses.ResponseReasoningItem,
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

	promptCacheRetention, cacheErr := providerbase.NormalizeOpenAIPromptCacheRetention(req.PromptCacheRetention, p.Name())
	if cacheErr != nil {
		return nil, cacheErr
	}

	reqCopy := *req
	reqCopy.PromptCacheRetention = promptCacheRetention

	body := p.buildResponsesRequest(&reqCopy)
	params := p.buildResponsesParams(&reqCopy, body)

	if req.PreviousResponseID != "" {
		body.PreviousResponseID = req.PreviousResponseID
		params.PreviousResponseID = param.NewOpt(req.PreviousResponseID)
	} else if prevID, ok := PreviousResponseIDFromContext(ctx); ok {
		body.PreviousResponseID = prevID
		params.PreviousResponseID = param.NewOpt(prevID)
	}
	body.Conversation = strings.TrimSpace(req.ConversationID)
	if body.Conversation != "" {
		params.Conversation = responses.ResponseNewParamsConversationUnion{OfString: param.NewOpt(body.Conversation)}
	}
	llm.ReportProviderPromptUsage(ctx, llm.ProviderPromptUsageReport{
		Provider:     p.Name(),
		Model:        body.Model,
		API:          "responses",
		PromptTokens: countResponsesPromptTokens(body),
	})

	client := p.sdkClient(ctx)
	stream := client.Responses.NewStreaming(ctx, params, responseRequestOptions(body)...)
	if err := stream.Err(); err != nil {
		return nil, p.mapSDKError(err)
	}

	return streamResponsesSDK(ctx, stream, p.Name()), nil
}

// streamResponsesSDK parses typed streaming events from the Responses API.
func streamResponsesSDK(ctx context.Context, stream interface {
	Next() bool
	Current() responses.ResponseStreamEventUnion
	Err() error
	Close() error
}, providerName string) <-chan llm.StreamChunk {
	ch := make(chan llm.StreamChunk)
	go func() {
		defer stream.Close()
		defer close(ch)

		var currentID string
		var currentModel string
		accumulator := providerbase.NewToolCallDeltaAccumulator()
		seenReasoning := map[string]bool{}
		seenToolCalls := map[string]bool{}
		finishSent := false

		for stream.Next() {
			event := stream.Current()
			switch event.Type {
			case "response.created", "response.in_progress":
				if event.Response.ID != "" {
					currentID = event.Response.ID
				}
				if event.Response.Model != "" {
					currentModel = string(event.Response.Model)
				}

			case "response.output_item.added":
				switch item := event.Item.AsAny().(type) {
				case responses.ResponseFunctionToolCall:
					if item.ID != "" && item.Name != "" {
						accumulator.Register(item.ID, types.ToolTypeFunction, item.Name, item.CallID)
					}
				case responses.ResponseCustomToolCall:
					if item.ID != "" && item.Name != "" {
						accumulator.Register(item.ID, types.ToolTypeCustom, item.Name, item.CallID)
					}
				}

			case "response.output_text.delta":
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{ID: currentID, Provider: providerName, Model: currentModel, Delta: types.Message{Role: llm.RoleAssistant, Content: event.Delta}}:
				}

			case "response.refusal.delta":
				delta := event.Delta
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{ID: currentID, Provider: providerName, Model: currentModel, Delta: types.Message{Role: llm.RoleAssistant, Refusal: &delta}}:
				}

			case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
				if strings.TrimSpace(event.Delta) == "" {
					continue
				}
				delta := event.Delta
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{ID: currentID, Provider: providerName, Model: currentModel, Delta: types.Message{Role: llm.RoleAssistant, ReasoningContent: stringPtr(delta)}}:
				}

			case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
				if event.ItemID == "" {
					continue
				}
				accumulator.Append(event.ItemID, event.Delta)

			case "response.output_item.done":
				switch item := event.Item.AsAny().(type) {
				case responses.ResponseReasoningItem:
					if emitResponsesReasoningChunk(ctx, ch, currentID, providerName, currentModel, item) {
						seenReasoning[item.ID] = true
					}
				case responses.ResponseCustomToolCall:
					callID := firstNonEmptyString(item.CallID, item.ID)
					if seenToolCalls[callID] {
						continue
					}
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{ID: currentID, Provider: providerName, Model: currentModel, Delta: types.Message{Role: llm.RoleAssistant, ToolCalls: providerbase.ToolCallChunk(providerbase.NewCustomToolCall(callID, item.Name, item.Input))}, FinishReason: "tool_calls"}:
					}
				}

			case "response.completed":
				if event.Response.ID != "" {
					currentID = event.Response.ID
				}
				if event.Response.Model != "" {
					currentModel = string(event.Response.Model)
				}
				for _, output := range event.Response.Output {
					if output.Type != "reasoning" {
						continue
					}
					reasoning := output.AsReasoning()
					if seenReasoning[reasoning.ID] {
						continue
					}
					if emitResponsesReasoningChunk(ctx, ch, currentID, providerName, currentModel, reasoning) {
						seenReasoning[reasoning.ID] = true
					}
				}
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{ID: currentID, Provider: providerName, Model: currentModel, FinishReason: func() string {
					if finishSent {
						return ""
					}
					finishSent = true
					return "stop"
				}(), Usage: usageFromSDK(event.Response.Usage)}:
				}

			case "response.output_text.done":
				if finishSent {
					continue
				}
				finishSent = true
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{ID: currentID, Provider: providerName, Model: currentModel, FinishReason: "stop"}:
				}

			case "response.function_call_arguments.done":
				if event.ItemID == "" {
					continue
				}
				toolCall, ok := accumulator.CompleteFunction(event.ItemID)
				if !ok {
					continue
				}
				callID := toolCall.ID
				seenToolCalls[callID] = true
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{ID: currentID, Provider: providerName, Model: currentModel, Delta: types.Message{Role: llm.RoleAssistant, ToolCalls: providerbase.ToolCallChunk(toolCall)}, FinishReason: func() string {
					if finishSent {
						return ""
					}
					finishSent = true
					return "tool_calls"
				}()}:
				}

			case "response.custom_tool_call_input.done":
				if event.ItemID == "" {
					continue
				}
				toolCall, ok := accumulator.CompleteCustom(event.ItemID)
				if !ok {
					continue
				}
				callID := toolCall.ID
				seenToolCalls[callID] = true
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{ID: currentID, Provider: providerName, Model: currentModel, Delta: types.Message{Role: llm.RoleAssistant, ToolCalls: providerbase.ToolCallChunk(toolCall)}, FinishReason: func() string {
					if finishSent {
						return ""
					}
					finishSent = true
					return "tool_calls"
				}()}:
				}

			case "error":
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{Err: &types.Error{Code: llm.ErrUpstreamError, Message: event.Message, HTTPStatus: http.StatusBadGateway, Provider: providerName}}:
				}
				return
			}
		}
		if err := stream.Err(); err != nil {
			select {
			case <-ctx.Done():
				return
			case ch <- llm.StreamChunk{Err: &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: providerName}}:
			}
		}
	}()
	return ch
}

func usageFromSDK(usage responses.ResponseUsage) *llm.ChatUsage {
	if !usage.JSON.TotalTokens.Valid() && !usage.JSON.InputTokens.Valid() && !usage.JSON.OutputTokens.Valid() {
		return nil
	}
	result := &llm.ChatUsage{
		PromptTokens:     int(usage.InputTokens),
		CompletionTokens: int(usage.OutputTokens),
		TotalTokens:      int(usage.TotalTokens),
	}
	if usage.JSON.InputTokensDetails.Valid() {
		result.PromptTokensDetails = &llm.PromptTokensDetails{CachedTokens: int(usage.InputTokensDetails.CachedTokens)}
	}
	if usage.JSON.OutputTokensDetails.Valid() {
		result.CompletionTokensDetails = &llm.CompletionTokensDetails{ReasoningTokens: int(usage.OutputTokensDetails.ReasoningTokens)}
	}
	return result
}
