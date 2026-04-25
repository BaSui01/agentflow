package claude

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	anthropicofficial "github.com/BaSui01/agentflow/llm/internal/anthropicofficial"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/middleware"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"github.com/BaSui01/agentflow/types"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	anthropicsdkoption "github.com/anthropics/anthropic-sdk-go/option"
	anthropicsdkparam "github.com/anthropics/anthropic-sdk-go/packages/param"
	"go.uber.org/zap"
)

// ClaudeProvider 实现 Anthropic Claude 的 LLM Provider。
// Claude API 与 OpenAI 有显著差异：
// 1. 认证使用 x-api-key 请求头而非 Bearer Token
// 2. 请求格式不同（system 消息单独传递）
// 3. 流式响应使用 SSE 格式但结构不同
// 4. ToolCall 结构和字段名称有差异
type ClaudeProvider struct {
	cfg           providers.ClaudeConfig
	client        *http.Client
	logger        *zap.Logger
	rewriterChain *middleware.RewriterChain
	keyIndex      uint64 // 多 Key 轮询索引
}

// defaultClaudeTimeout is the default HTTP client timeout for Claude API requests.
// Claude responses can be slower than other providers.
const defaultClaudeTimeout = 60 * time.Second
const defaultClaudeModel = "claude-opus-4-7"

// NewClaudeProvider 创建 Claude Provider。
func NewClaudeProvider(cfg providers.ClaudeConfig, logger *zap.Logger) *ClaudeProvider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultClaudeTimeout
	}

	// 设置默认 BaseURL
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}

	return &ClaudeProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
		logger: logger,
		rewriterChain: middleware.NewRewriterChain(
			middleware.NewXMLToolRewriter(),
			middleware.NewEmptyToolsCleaner(),
		),
	}
}

func (p *ClaudeProvider) Name() string { return "claude" }

func (p *ClaudeProvider) sdkClient(apiKey string) anthropicsdk.Client {
	return anthropicofficial.NewClient(p.cfg, apiKey, p.client)
}

func (p *ClaudeProvider) sdkRequestOptions(speed string) []anthropicsdkoption.RequestOption {
	options := make([]anthropicsdkoption.RequestOption, 0, 3)
	version := p.cfg.AnthropicVersion
	if version == "" {
		version = "2025-04-14"
	}
	options = append(options, anthropicsdkoption.WithHeader("anthropic-version", version))
	if strings.EqualFold(strings.TrimSpace(speed), "fast") {
		options = append(options, anthropicsdkoption.WithHeader("anthropic-beta", "fast-mode-2026-02-01"))
		options = append(options, anthropicsdkoption.WithQuery("beta", "true"))
	}
	return options
}

func (p *ClaudeProvider) mapSDKError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *anthropicsdk.Error
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

func (p *ClaudeProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	latency := time.Duration(0)
	client := p.sdkClient(p.resolveAPIKey(ctx))
	_, err := client.Models.List(ctx, anthropicsdk.ModelListParams{
		Limit: anthropicsdkparam.NewOpt(int64(1)),
	}, p.sdkRequestOptions("")...)
	latency = time.Since(start)
	if err != nil {
		return &llm.HealthStatus{Healthy: false, Latency: latency}, p.mapSDKError(err)
	}
	return &llm.HealthStatus{Healthy: true, Latency: latency}, nil
}

func (p *ClaudeProvider) SupportsNativeFunctionCalling() bool { return true }

// Endpoints 返回该提供者使用的所有 API 端点完整 URL。
func (p *ClaudeProvider) Endpoints() llm.ProviderEndpoints {
	base := strings.TrimRight(p.cfg.BaseURL, "/")
	return llm.ProviderEndpoints{
		Completion: base + "/v1/messages",
		Models:     base + "/v1/models",
		BaseURL:    p.cfg.BaseURL,
	}
}

// ListModels 获取 Claude 支持的模型列表
func (p *ClaudeProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	client := p.sdkClient(p.resolveAPIKey(ctx))
	page, err := client.Models.List(ctx, anthropicsdk.ModelListParams{}, p.sdkRequestOptions("")...)
	if err != nil {
		return nil, p.mapSDKError(err)
	}

	models := make([]llm.Model, 0, len(page.Data))
	for current := page; current != nil; {
		for _, m := range current.Data {
			id := strings.TrimSpace(m.ID)
			if id == "" {
				continue
			}
			models = append(models, llm.Model{
				ID:      id,
				Object:  string(m.Type),
				Created: m.CreatedAt.Unix(),
				OwnedBy: "anthropic",
			})
		}
		current, err = current.GetNextPage()
		if err != nil {
			return nil, p.mapSDKError(err)
		}
	}

	return models, nil
}

// 保留的响应结构体（用于解析 SDK 返回的 RawJSON）
type claudeContent struct {
	Type      string          `json:"type"` // text, tool_use, tool_result, image, thinking, redacted_thinking, server_tool_use, web_search_tool_result
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`  // for tool_result (string form)
	IsError   *bool           `json:"is_error,omitempty"` // for tool_result: 标记工具执行失败
	// Image source fields (for type="image")
	Source *claudeImageSource `json:"source,omitempty"`
	// Thinking fields (for type="thinking")
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
	Data      string `json:"data,omitempty"` // for type="redacted_thinking"
	// Web search fields
	Citations []claudeCitation `json:"citations,omitempty"` // text 块上的引用标注

	// web_search_tool_result: content 字段可能是 []object（搜索结果条目）
	// 使用 json.RawMessage 保存原始值，便于 round-trip
	SearchResults json.RawMessage `json:"search_results,omitempty"`

	// server_tool_use / web_search_tool_result 的 encrypted 不透明字段
	EncryptedContent string `json:"encrypted_content,omitempty"`

	// web_search_tool_result error
	ErrorType string `json:"error_type,omitempty"`
}

// claudeCitation 表示文本块上的引用标注
type claudeCitation struct {
	Type           string `json:"type"` // "web_search_result_location"
	URL            string `json:"url"`
	Title          string `json:"title"`
	CitedText      string `json:"cited_text,omitempty"`
	EncryptedIndex string `json:"encrypted_index,omitempty"`
	StartIndex     int    `json:"start_index,omitempty"`
	EndIndex       int    `json:"end_index,omitempty"`
}

// claudeServerToolUse 表示服务端工具使用计费
type claudeServerToolUse struct {
	WebSearchRequests int `json:"web_search_requests,omitempty"`
}

type claudeImageSource struct {
	Type      string `json:"type"`                 // "base64" or "url"
	MediaType string `json:"media_type,omitempty"` // e.g., "image/png"
	Data      string `json:"data,omitempty"`       // base64 data
	URL       string `json:"url,omitempty"`        // image URL
}

type claudeUsage struct {
	InputTokens              int                  `json:"input_tokens"`
	OutputTokens             int                  `json:"output_tokens"`
	CacheCreationInputTokens int                  `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int                  `json:"cache_read_input_tokens,omitempty"`
	Speed                    string               `json:"speed,omitempty"`
	ServerToolUse            *claudeServerToolUse `json:"server_tool_use,omitempty"`
}

func (u *claudeUsage) UnmarshalJSON(data []byte) error {
	var aux struct {
		InputTokens                   *int                 `json:"input_tokens"`
		InputTokensCamel              *int                 `json:"inputTokens"`
		OutputTokens                  *int                 `json:"output_tokens"`
		OutputTokensCamel             *int                 `json:"outputTokens"`
		PromptTokens                  *int                 `json:"prompt_tokens"`
		CompletionTokens              *int                 `json:"completion_tokens"`
		CacheCreationInputTokens      *int                 `json:"cache_creation_input_tokens"`
		CacheCreationInputTokensCamel *int                 `json:"cacheCreationInputTokens"`
		CacheReadInputTokens          *int                 `json:"cache_read_input_tokens"`
		CacheReadInputTokensCamel     *int                 `json:"cacheReadInputTokens"`
		Speed                         string               `json:"speed"`
		ServerToolUse                 *claudeServerToolUse `json:"server_tool_use"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	u.InputTokens = firstInt(aux.InputTokens, aux.InputTokensCamel, aux.PromptTokens)
	u.OutputTokens = firstInt(aux.OutputTokens, aux.OutputTokensCamel, aux.CompletionTokens)
	u.CacheCreationInputTokens = firstInt(aux.CacheCreationInputTokens, aux.CacheCreationInputTokensCamel)
	u.CacheReadInputTokens = firstInt(aux.CacheReadInputTokens, aux.CacheReadInputTokensCamel)
	u.Speed = strings.TrimSpace(aux.Speed)
	u.ServerToolUse = aux.ServerToolUse
	return nil
}

type claudeResponse struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"` // message, content_block_delta, etc.
	Role         string          `json:"role"`
	Content      []claudeContent `json:"content"`
	Model        string          `json:"model"`
	StopReason   string          `json:"stop_reason"`
	StopSequence string          `json:"stop_sequence,omitempty"`
	Usage        *claudeUsage    `json:"usage,omitempty"`
}

func (r *claudeResponse) UnmarshalJSON(data []byte) error {
	var aux struct {
		ID                string          `json:"id"`
		Type              string          `json:"type"`
		Role              string          `json:"role"`
		Content           json.RawMessage `json:"content"`
		Model             string          `json:"model"`
		StopReason        string          `json:"stop_reason"`
		StopReasonCamel   string          `json:"stopReason"`
		FinishReason      string          `json:"finish_reason"`
		StopSequence      string          `json:"stop_sequence"`
		StopSequenceCamel string          `json:"stopSequence"`
		Usage             *claudeUsage    `json:"usage"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	r.ID = aux.ID
	r.Type = aux.Type
	r.Role = aux.Role
	r.Model = aux.Model
	r.StopReason = strings.TrimSpace(firstNonEmpty(aux.StopReason, aux.StopReasonCamel, aux.FinishReason))
	r.StopSequence = strings.TrimSpace(firstNonEmpty(aux.StopSequence, aux.StopSequenceCamel))
	r.Usage = aux.Usage

	contentRaw := bytes.TrimSpace(aux.Content)
	if len(contentRaw) == 0 || string(contentRaw) == "null" {
		r.Content = nil
		return nil
	}

	switch contentRaw[0] {
	case '[':
		var blocks []claudeContent
		if err := json.Unmarshal(contentRaw, &blocks); err != nil {
			return err
		}
		r.Content = blocks
	case '"':
		var text string
		if err := json.Unmarshal(contentRaw, &text); err != nil {
			return err
		}
		if strings.TrimSpace(text) != "" {
			r.Content = []claudeContent{{Type: "text", Text: text}}
		}
	case '{':
		var block claudeContent
		if err := json.Unmarshal(contentRaw, &block); err != nil {
			return err
		}
		r.Content = []claudeContent{block}
	default:
		r.Content = nil
	}

	return nil
}

// 流式响应的事件类型
type claudeStreamEvent struct {
	Type         string          `json:"type"` // message_start, content_block_start, content_block_delta, content_block_stop, message_delta, message_stop
	Index        int             `json:"index,omitempty"`
	Delta        *claudeDelta    `json:"delta,omitempty"`
	ContentBlock *claudeContent  `json:"content_block,omitempty"`
	Message      *claudeResponse `json:"message,omitempty"`
	Usage        *claudeUsage    `json:"usage,omitempty"`
}

type claudeDelta struct {
	Type        string          `json:"type"` // text_delta, input_json_delta, thinking_delta, signature_delta, citations_delta
	Text        string          `json:"text,omitempty"`
	PartialJSON string          `json:"partial_json,omitempty"`
	StopReason  string          `json:"stop_reason,omitempty"`
	Thinking    string          `json:"thinking,omitempty"`  // for thinking_delta
	Signature   string          `json:"signature,omitempty"` // for signature_delta
	Citation    *claudeCitation `json:"citation,omitempty"`  // for citations_delta
}

type claudeErrorResp struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// resolveAPIKey 解析 API Key，支持上下文覆盖和多 Key 轮询
func (p *ClaudeProvider) resolveAPIKey(ctx context.Context) string {
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			return strings.TrimSpace(c.APIKey)
		}
	}
	if len(p.cfg.APIKeys) > 0 {
		idx := atomic.AddUint64(&p.keyIndex, 1) - 1
		return p.cfg.APIKeys[idx%uint64(len(p.cfg.APIKeys))].Key
	}
	return p.cfg.APIKey
}

// convertToClaudeMessages 将统一格式转换为 Claude SDK 格式
// Claude 的特殊要求：
// 1. system 消息需要单独提取到 system 字段
// 2. 消息必须是 user/assistant 交替出现
// 3. content 是数组形式，可包含文本和工具调用
func convertToClaudeMessages(msgs []types.Message) ([]anthropicsdk.TextBlockParam, []anthropicsdk.MessageParam) {
	var systemParts []anthropicsdk.TextBlockParam
	var claudeMsgs []anthropicsdk.MessageParam

	for _, m := range msgs {
		// 提取 system 消息
		if m.Role == llm.RoleSystem || m.Role == llm.RoleDeveloper {
			if m.Content != "" {
				systemParts = append(systemParts, anthropicsdk.TextBlockParam{Text: m.Content})
			}
			continue
		}

		// 处理 tool 角色（Claude 将其作为 user 的 tool_result）
		if m.Role == llm.RoleTool {
			writeback, ok := providerbase.ToolOutputFromMessage(m, nil)
			if !ok {
				continue
			}
			rawBlock := providerbase.BuildAnthropicToolResultBlock(writeback)
			content := make([]anthropicsdk.ToolResultBlockParamContentUnion, 0, 1)
			if txt, ok := rawBlock["content"].(string); ok && txt != "" {
				content = append(content, anthropicsdk.ToolResultBlockParamContentUnion{
					OfText: &anthropicsdk.TextBlockParam{Text: txt},
				})
			}
			isError := false
			if v, ok := rawBlock["is_error"].(bool); ok {
				isError = v
			}
			tr := anthropicsdk.ToolResultBlockParam{
				ToolUseID: rawBlock["tool_use_id"].(string),
				Content:   content,
			}
			if isError {
				tr.IsError = anthropicsdkparam.NewOpt(true)
			}
			claudeMsgs = append(claudeMsgs, anthropicsdk.MessageParam{
				Role:    anthropicsdk.MessageParamRoleUser,
				Content: []anthropicsdk.ContentBlockParamUnion{{OfToolResult: &tr}},
			})
			continue
		}

		// 构建普通消息
		role := anthropicsdk.MessageParamRoleUser
		if m.Role == llm.RoleAssistant {
			role = anthropicsdk.MessageParamRoleAssistant
		}
		var blocks []anthropicsdk.ContentBlockParamUnion

		// Assistant thinking blocks must round-trip as Claude thinking content blocks.
		if m.Role == llm.RoleAssistant && len(m.ThinkingBlocks) > 0 {
			for _, tb := range m.ThinkingBlocks {
				blocks = append(blocks, anthropicsdk.ContentBlockParamUnion{
					OfThinking: &anthropicsdk.ThinkingBlockParam{
						Thinking:  tb.Thinking,
						Signature: tb.Signature,
					},
				})
			}
		}
		if m.Role == llm.RoleAssistant && len(m.OpaqueReasoning) > 0 {
			for _, opaque := range m.OpaqueReasoning {
				provider := strings.TrimSpace(opaque.Provider)
				if provider != "" && provider != "anthropic" {
					continue
				}
				if strings.TrimSpace(opaque.Kind) != "redacted_thinking" || strings.TrimSpace(opaque.State) == "" {
					continue
				}
				blocks = append(blocks, anthropicsdk.NewRedactedThinkingBlock(opaque.State))
			}
		}

		// 文本内容
		if m.Content != "" {
			blocks = append(blocks, anthropicsdk.ContentBlockParamUnion{
				OfText: &anthropicsdk.TextBlockParam{Text: m.Content},
			})
		}

		// Images 转换为 Claude 的 image content blocks
		if len(m.Images) > 0 {
			for _, img := range m.Images {
				if img.Type == "base64" && img.Data != "" {
					blocks = append(blocks, anthropicsdk.ContentBlockParamUnion{
						OfImage: &anthropicsdk.ImageBlockParam{
							Source: anthropicsdk.ImageBlockParamSourceUnion{
								OfBase64: &anthropicsdk.Base64ImageSourceParam{
									MediaType: anthropicsdk.Base64ImageSourceMediaType(detectImageMediaType(img.Data)),
									Data:      img.Data,
								},
							},
						},
					})
				} else if img.Type == "url" && img.URL != "" {
					blocks = append(blocks, anthropicsdk.ContentBlockParamUnion{
						OfImage: &anthropicsdk.ImageBlockParam{
							Source: anthropicsdk.ImageBlockParamSourceUnion{
								OfURL: &anthropicsdk.URLImageSourceParam{
									URL: img.URL,
								},
							},
						},
					})
				}
			}
		}

		// ToolCall 转换
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				var input any
				if len(tc.Arguments) > 0 {
					if err := json.Unmarshal(tc.Arguments, &input); err != nil {
						input = map[string]any{}
					}
				} else {
					input = map[string]any{}
				}
				blocks = append(blocks, anthropicsdk.ContentBlockParamUnion{
					OfToolUse: &anthropicsdk.ToolUseBlockParam{
						ID:    tc.ID,
						Name:  tc.Name,
						Input: input,
					},
				})
			}
		}

		if len(blocks) > 0 {
			// 多轮回传：如果 assistant 消息的 Metadata 中包含 web search blocks，追加到 content 数组
			if m.Role == llm.RoleAssistant {
				if meta, ok := m.Metadata.(map[string]any); ok {
					if rawBlocks, ok := meta["claude_web_search_blocks"]; ok {
						switch items := rawBlocks.(type) {
						case []json.RawMessage:
							for _, raw := range items {
								var block claudeContent
								if json.Unmarshal(raw, &block) == nil {
									blocks = appendServerToolBlock(blocks, block)
								}
							}
						case []any:
							for _, item := range items {
								raw, err := json.Marshal(item)
								if err != nil {
									continue
								}
								var block claudeContent
								if json.Unmarshal(raw, &block) == nil {
									blocks = appendServerToolBlock(blocks, block)
								}
							}
						}
					}
				}
			}
			claudeMsgs = append(claudeMsgs, anthropicsdk.MessageParam{
				Role:    role,
				Content: blocks,
			})
		}
	}

	return systemParts, claudeMsgs
}

// appendServerToolBlock 将原始 server_tool_use / web_search_tool_result 块追加为 SDK 类型。
// 由于 SDK 没有暴露直接构造 ServerToolUseBlockParam 的便利函数，这里通过 JSON round-trip 构造。
func appendServerToolBlock(blocks []anthropicsdk.ContentBlockParamUnion, block claudeContent) []anthropicsdk.ContentBlockParamUnion {
	raw, err := json.Marshal(block)
	if err != nil {
		return blocks
	}
	// 尝试解析为 ServerToolUseBlockParam
	var stu anthropicsdk.ServerToolUseBlockParam
	if err := json.Unmarshal(raw, &stu); err == nil && stu.ID != "" {
		return append(blocks, anthropicsdk.ContentBlockParamUnion{OfServerToolUse: &stu})
	}
	// 尝试解析为 WebSearchToolResultBlockParam
	var wsr anthropicsdk.WebSearchToolResultBlockParam
	if err := json.Unmarshal(raw, &wsr); err == nil && wsr.ToolUseID != "" {
		return append(blocks, anthropicsdk.ContentBlockParamUnion{OfWebSearchToolResult: &wsr})
	}
	return blocks
}

// convertToClaudeTools 将统一工具列表转换为 Claude API 的混合工具数组。
// 当 wsOpts 不为 nil 或工具列表中包含 web_search 时，自动注入 web_search_20260209 服务端工具。
func convertToClaudeTools(tools []types.ToolSchema, wsOpts *llm.WebSearchOptions) []anthropicsdk.ToolUnionParam {
	hasWebSearch := wsOpts != nil
	out := make([]anthropicsdk.ToolUnionParam, 0, len(tools)+1)

	for _, t := range tools {
		// 跳过客户端传入的 web_search 占位工具（避免双重注入）
		if providerbase.IsSearchToolPlaceholder(t.Name) {
			hasWebSearch = true
			continue
		}
		var schema anthropicsdk.ToolInputSchemaParam
		if len(t.Parameters) > 0 {
			_ = json.Unmarshal(t.Parameters, &schema)
		}
		tool := anthropicsdk.ToolParam{
			Name:        t.Name,
			InputSchema: schema,
		}
		if t.Description != "" {
			tool.Description = anthropicsdkparam.NewOpt(t.Description)
		}
		if t.Strict != nil {
			tool.Strict = anthropicsdkparam.NewOpt(*t.Strict)
		}
		out = append(out, anthropicsdk.ToolUnionParam{OfTool: &tool})
	}

	// 注入 web_search_20260209 服务端工具
	if hasWebSearch {
		wsTool := anthropicsdk.WebSearchTool20260209Param{}
		if wsOpts != nil {
			if len(wsOpts.AllowedDomains) > 0 {
				wsTool.AllowedDomains = wsOpts.AllowedDomains
			}
			if len(wsOpts.BlockedDomains) > 0 {
				wsTool.BlockedDomains = wsOpts.BlockedDomains
			}
			if wsOpts.MaxUses > 0 {
				wsTool.MaxUses = anthropicsdkparam.NewOpt(int64(wsOpts.MaxUses))
			}
			if wsOpts.UserLocation != nil {
				loc := anthropicsdk.UserLocationParam{}
				if wsOpts.UserLocation.Country != "" {
					loc.Country = anthropicsdkparam.NewOpt(wsOpts.UserLocation.Country)
				}
				if wsOpts.UserLocation.Region != "" {
					loc.Region = anthropicsdkparam.NewOpt(wsOpts.UserLocation.Region)
				}
				if wsOpts.UserLocation.City != "" {
					loc.City = anthropicsdkparam.NewOpt(wsOpts.UserLocation.City)
				}
				if wsOpts.UserLocation.Timezone != "" {
					loc.Timezone = anthropicsdkparam.NewOpt(wsOpts.UserLocation.Timezone)
				}
				wsTool.UserLocation = loc
			}
		}
		out = append(out, anthropicsdk.ToolUnionParam{OfWebSearchTool20260209: &wsTool})
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// convertClaudeToolChoice 将 llm.ChatRequest.ToolChoice (any) 转换为 Claude SDK 格式。
// 支持 string ("auto"/"any"/"none") 和 map/struct 形式。
func convertClaudeToolChoice(tc any, parallelToolCalls *bool, hasTools bool) anthropicsdk.ToolChoiceUnionParam {
	spec := providerbase.NormalizeToolChoice(tc)
	var result anthropicsdk.ToolChoiceUnionParam
	switch spec.Mode {
	case "auto":
		result = anthropicsdk.ToolChoiceUnionParam{OfAuto: &anthropicsdk.ToolChoiceAutoParam{}}
	case "any":
		result = anthropicsdk.ToolChoiceUnionParam{OfAny: &anthropicsdk.ToolChoiceAnyParam{}}
	case "none":
		result = anthropicsdk.ToolChoiceUnionParam{OfNone: &anthropicsdk.ToolChoiceNoneParam{}}
	case "tool":
		result = anthropicsdk.ToolChoiceParamOfTool(spec.SpecificName)
	}
	if spec.DisableParallelToolUse != nil {
		switch {
		case result.OfAuto != nil:
			result.OfAuto.DisableParallelToolUse = anthropicsdkparam.NewOpt(*spec.DisableParallelToolUse)
		case result.OfAny != nil:
			result.OfAny.DisableParallelToolUse = anthropicsdkparam.NewOpt(*spec.DisableParallelToolUse)
		case result.OfTool != nil:
			result.OfTool.DisableParallelToolUse = anthropicsdkparam.NewOpt(*spec.DisableParallelToolUse)
		}
	}
	if parallelToolCalls != nil && !*parallelToolCalls && hasTools {
		if result.OfAuto == nil && result.OfAny == nil && result.OfTool == nil && result.OfNone == nil {
			result = anthropicsdk.ToolChoiceUnionParam{OfAuto: &anthropicsdk.ToolChoiceAutoParam{}}
		}
		switch {
		case result.OfAuto != nil:
			result.OfAuto.DisableParallelToolUse = anthropicsdkparam.NewOpt(true)
		case result.OfAny != nil:
			result.OfAny.DisableParallelToolUse = anthropicsdkparam.NewOpt(true)
		case result.OfTool != nil:
			result.OfTool.DisableParallelToolUse = anthropicsdkparam.NewOpt(true)
		}
	}
	return result
}

func decodeAnthropicSDKRawJSON(raw string, dst any) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("anthropic sdk returned empty raw json")
	}
	return json.Unmarshal([]byte(raw), dst)
}

func (p *ClaudeProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// 统一入口：应用改写器链
	rewrittenReq, err := p.rewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("request rewrite failed: %v", err),
			HTTPStatus: http.StatusBadRequest,
			Provider:   p.Name(),
		}
	}
	req = rewrittenReq

	apiKey := p.resolveAPIKey(ctx)
	system, messages := convertToClaudeMessages(req.Messages)
	model := providerbase.ChooseModel(req, p.cfg.Model, defaultClaudeModel)
	if err := validateClaudeRequest(req, model); err != nil {
		return nil, err
	}
	thinking, outputConfig, speed := buildClaudeReasoningControls(req, model)
	cacheControl, cacheErr := normalizeClaudeCacheControl(req.CacheControl)
	if cacheErr != nil {
		return nil, cacheErr
	}

	params := anthropicsdk.MessageNewParams{
		Model:     model,
		MaxTokens: int64(chooseMaxTokens(req)),
		Messages:  messages,
	}
	if len(system) > 0 {
		params.System = system
	}
	if req.Temperature != 0 {
		params.Temperature = anthropicsdkparam.NewOpt(float64(req.Temperature))
	}
	if req.TopP != 0 {
		params.TopP = anthropicsdkparam.NewOpt(float64(req.TopP))
	}
	if len(req.Stop) > 0 {
		params.StopSequences = req.Stop
	}
	tools := convertToClaudeTools(req.Tools, req.WebSearchOptions)
	if len(tools) > 0 {
		params.Tools = tools
	}
	tc := convertClaudeToolChoice(req.ToolChoice, req.ParallelToolCalls, len(req.Tools) > 0 || req.WebSearchOptions != nil)
	if tc.OfAuto != nil || tc.OfAny != nil || tc.OfTool != nil || tc.OfNone != nil {
		params.ToolChoice = tc
	}
	if thinking.OfEnabled != nil || thinking.OfAdaptive != nil || thinking.OfDisabled != nil {
		params.Thinking = thinking
	}
	if outputConfig.Effort != "" {
		params.OutputConfig = outputConfig
	}
	if cacheControl != nil {
		params.CacheControl = *cacheControl
	}

	// Claude thinking mode only supports compatible tool_choice combinations.
	if err := validateThinkingConstraints(thinking, tc); err != nil {
		return nil, err
	}

	client := p.sdkClient(apiKey)
	sdkResp, err := client.Messages.New(ctx, params, p.sdkRequestOptions(speed)...)
	if err != nil {
		if shouldRetryClaudeWithoutFastMode(sdkStatusCode(err), speed) {
			speed = ""
			sdkResp, err = client.Messages.New(ctx, params, p.sdkRequestOptions(speed)...)
			if err != nil {
				return nil, p.mapSDKError(err)
			}
		} else {
			return nil, p.mapSDKError(err)
		}
	}

	var claudeResp claudeResponse
	if err := decodeAnthropicSDKRawJSON(sdkResp.RawJSON(), &claudeResp); err != nil {
		return nil, p.mapSDKError(err)
	}
	return toClaudeChatResponse(claudeResp, p.Name()), nil
}

func (p *ClaudeProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	// 统一入口：应用改写器链
	rewrittenReq, err := p.rewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("request rewrite failed: %v", err),
			HTTPStatus: http.StatusBadRequest,
			Provider:   p.Name(),
		}
	}
	req = rewrittenReq

	apiKey := p.resolveAPIKey(ctx)
	system, messages := convertToClaudeMessages(req.Messages)
	model := providerbase.ChooseModel(req, p.cfg.Model, defaultClaudeModel)
	if err := validateClaudeRequest(req, model); err != nil {
		return nil, err
	}
	thinking, outputConfig, speed := buildClaudeReasoningControls(req, model)
	cacheControl, cacheErr := normalizeClaudeCacheControl(req.CacheControl)
	if cacheErr != nil {
		return nil, cacheErr
	}

	params := anthropicsdk.MessageNewParams{
		Model:     model,
		MaxTokens: int64(chooseMaxTokens(req)),
		Messages:  messages,
	}
	if len(system) > 0 {
		params.System = system
	}
	if req.Temperature != 0 {
		params.Temperature = anthropicsdkparam.NewOpt(float64(req.Temperature))
	}
	if req.TopP != 0 {
		params.TopP = anthropicsdkparam.NewOpt(float64(req.TopP))
	}
	if len(req.Stop) > 0 {
		params.StopSequences = req.Stop
	}
	tools := convertToClaudeTools(req.Tools, req.WebSearchOptions)
	if len(tools) > 0 {
		params.Tools = tools
	}
	tc := convertClaudeToolChoice(req.ToolChoice, req.ParallelToolCalls, len(req.Tools) > 0 || req.WebSearchOptions != nil)
	if tc.OfAuto != nil || tc.OfAny != nil || tc.OfTool != nil || tc.OfNone != nil {
		params.ToolChoice = tc
	}
	if thinking.OfEnabled != nil || thinking.OfAdaptive != nil || thinking.OfDisabled != nil {
		params.Thinking = thinking
	}
	if outputConfig.Effort != "" {
		params.OutputConfig = outputConfig
	}
	if cacheControl != nil {
		params.CacheControl = *cacheControl
	}

	// Claude thinking mode only supports compatible tool_choice combinations.
	if err := validateThinkingConstraints(thinking, tc); err != nil {
		return nil, err
	}

	client := p.sdkClient(apiKey)
	stream := client.Messages.NewStreaming(ctx, params, p.sdkRequestOptions(speed)...)
	if stream == nil {
		return nil, &types.Error{
			Code:       llm.ErrUpstreamError,
			Message:    "claude stream returned empty stream handle",
			HTTPStatus: http.StatusBadGateway,
			Provider:   p.Name(),
			Retryable:  true,
		}
	}
	if err := stream.Err(); err != nil {
		if shouldRetryClaudeWithoutFastMode(sdkStatusCode(err), speed) {
			speed = ""
			stream = client.Messages.NewStreaming(ctx, params, p.sdkRequestOptions(speed)...)
			if stream == nil {
				return nil, &types.Error{
					Code:       llm.ErrUpstreamError,
					Message:    "claude stream retry returned empty stream handle",
					HTTPStatus: http.StatusBadGateway,
					Provider:   p.Name(),
					Retryable:  true,
				}
			}
			if retryErr := stream.Err(); retryErr != nil {
				return nil, p.mapSDKError(retryErr)
			}
		} else {
			return nil, p.mapSDKError(err)
		}
	}

	ch := make(chan llm.StreamChunk)
	go func() {
		defer stream.Close()
		defer close(ch)

		// Claude 流式响应累积状态
		var currentID string
		var currentModel string
		var toolCallAccumulator = make(map[int]*types.ToolCall)  // 累积工具调用
		var startUsage *claudeUsage                              // message_start 中的初始 usage
		var webSearchBlockIndices = make(map[int]bool)           // 标记 server_tool_use / web_search_tool_result 块索引
		var citationAccumulator = make(map[int][]claudeCitation) // 累积 text 块的引用（流式中通过 content_block_stop 发送）
		type thinkingBlockState struct {
			blockType string
			thinking  strings.Builder
			signature string
			data      string
		}
		var thinkingAccumulator = make(map[int]*thinkingBlockState)

		for stream.Next() {
			current := stream.Current()
			var event claudeStreamEvent
			if err := decodeAnthropicSDKRawJSON(current.RawJSON(), &event); err != nil {
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					Err: &types.Error{
						Code:       llm.ErrUpstreamError,
						Message:    err.Error(),
						Cause:      err,
						HTTPStatus: http.StatusBadGateway,
						Retryable:  true,
						Provider:   p.Name(),
					},
				}:
				}
				return
			}

			// 处理不同事件类型
			switch event.Type {
			case "message_start":
				if event.Message != nil {
					currentID = event.Message.ID
					currentModel = event.Message.Model
					// 捕获初始 usage（包含 input_tokens）
					if event.Message.Usage != nil {
						startUsage = event.Message.Usage
					}
				}

			case "content_block_start":
				if event.ContentBlock != nil {
					switch event.ContentBlock.Type {
					case "tool_use":
						// Start an empty tool-call accumulator and build arguments through input_json_delta.
						call := providerbase.NewFunctionToolCall(event.ContentBlock.ID, event.ContentBlock.Name, nil)
						toolCallAccumulator[event.Index] = &call
					case "server_tool_use", "web_search_tool_result":
						// 标记为搜索相关块，静默跳过其增量
						webSearchBlockIndices[event.Index] = true
					case "thinking":
						thinkingAccumulator[event.Index] = &thinkingBlockState{blockType: "thinking"}
					case "redacted_thinking":
						thinkingAccumulator[event.Index] = &thinkingBlockState{
							blockType: "redacted_thinking",
							data:      event.ContentBlock.Data,
						}
					}
				}

			case "content_block_delta":
				if event.Delta != nil {
					// 跳过 web search 相关块的增量
					if webSearchBlockIndices[event.Index] {
						continue
					}

					var sendChunk bool
					chunk := llm.StreamChunk{
						ID:       currentID,
						Provider: p.Name(),
						Model:    currentModel,
						Index:    event.Index,
						Delta: types.Message{
							Role: llm.RoleAssistant,
						},
					}

					switch event.Delta.Type {
					case "text_delta":
						chunk.Delta.Content = event.Delta.Text
						sendChunk = true
					case "input_json_delta":
						// 累积工具调用参数，不发送空 chunk
						if tc, ok := toolCallAccumulator[event.Index]; ok {
							tc.Arguments = providerbase.AppendToolJSONDelta(tc.Arguments, event.Delta.PartialJSON)
						}
					case "thinking_delta":
						thinking := event.Delta.Thinking
						if state, ok := thinkingAccumulator[event.Index]; ok {
							state.thinking.WriteString(thinking)
						}
						chunk.Delta.ReasoningContent = &thinking
						sendChunk = true
					case "signature_delta":
						if state, ok := thinkingAccumulator[event.Index]; ok {
							state.signature = event.Delta.Signature
						}
						// signature_delta 用于验证 thinking 块完整性，不发送 chunk
					case "citations_delta":
						// 引用增量 — 累积到 citationAccumulator，在 content_block_stop 时发送
						if event.Delta.Citation != nil {
							citationAccumulator[event.Index] = append(citationAccumulator[event.Index], *event.Delta.Citation)
						}
					}

					if sendChunk {
						select {
						case <-ctx.Done():
							return
						case ch <- chunk:
						}
					}
				}

			case "content_block_stop":
				// 工具调用块结束，发送完整的工具调用
				if tc, ok := toolCallAccumulator[event.Index]; ok {
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{
						ID:       currentID,
						Provider: p.Name(),
						Model:    currentModel,
						Index:    event.Index,
						Delta: types.Message{
							Role:      llm.RoleAssistant,
							ToolCalls: providerbase.ToolCallChunk(*tc),
						},
					}:
					}
					delete(toolCallAccumulator, event.Index)
				}

				if state, ok := thinkingAccumulator[event.Index]; ok {
					switch state.blockType {
					case "thinking":
						block := types.ThinkingBlock{
							Thinking:  strings.TrimSpace(state.thinking.String()),
							Signature: strings.TrimSpace(state.signature),
						}
						if block.Thinking != "" || block.Signature != "" {
							select {
							case <-ctx.Done():
								return
							case ch <- llm.StreamChunk{
								ID:       currentID,
								Provider: p.Name(),
								Model:    currentModel,
								Index:    event.Index,
								Delta: types.Message{
									Role:           llm.RoleAssistant,
									ThinkingBlocks: []types.ThinkingBlock{block},
								},
							}:
							}
						}
					case "redacted_thinking":
						if strings.TrimSpace(state.data) != "" {
							select {
							case <-ctx.Done():
								return
							case ch <- llm.StreamChunk{
								ID:       currentID,
								Provider: p.Name(),
								Model:    currentModel,
								Index:    event.Index,
								Delta: types.Message{
									Role: llm.RoleAssistant,
									OpaqueReasoning: []types.OpaqueReasoning{{
										Provider:  p.Name(),
										Kind:      "redacted_thinking",
										State:     state.data,
										PartIndex: event.Index,
									}},
								},
							}:
							}
						}
					}
					delete(thinkingAccumulator, event.Index)
				}

				// text 块结束时，发送累积的引用标注
				if citations, ok := citationAccumulator[event.Index]; ok && len(citations) > 0 {
					annotations := make([]types.Annotation, 0, len(citations))
					for _, cit := range citations {
						annotations = append(annotations, types.Annotation{
							Type:       "url_citation",
							URL:        cit.URL,
							Title:      cit.Title,
							StartIndex: cit.StartIndex,
							EndIndex:   cit.EndIndex,
						})
					}
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{
						ID:       currentID,
						Provider: p.Name(),
						Model:    currentModel,
						Index:    event.Index,
						Delta: types.Message{
							Role:        llm.RoleAssistant,
							Annotations: annotations,
						},
					}:
					}
					delete(citationAccumulator, event.Index)
				}

				// 清理 web search 块索引
				delete(webSearchBlockIndices, event.Index)

			case "message_delta":
				chunk := llm.StreamChunk{
					ID:       currentID,
					Provider: p.Name(),
					Model:    currentModel,
				}
				if event.Delta != nil && event.Delta.StopReason != "" {
					chunk.FinishReason = event.Delta.StopReason
				}
				if event.Usage != nil {
					// message_delta 的 usage 可能只有 output_tokens，
					// 需要与 message_start 的 input_tokens 合并
					merged := *event.Usage
					if merged.InputTokens == 0 && startUsage != nil {
						merged.InputTokens = startUsage.InputTokens
						merged.CacheCreationInputTokens = startUsage.CacheCreationInputTokens
						merged.CacheReadInputTokens = startUsage.CacheReadInputTokens
					}
					chunk.Usage = buildStreamUsage(&merged)
				} else if startUsage != nil {
					// message_delta 没有 usage 但 message_start 有，回退使用 startUsage
					chunk.Usage = buildStreamUsage(startUsage)
				}
				select {
				case <-ctx.Done():
					return
				case ch <- chunk:
				}

			case "message_stop":
				return

			case "ping":
				// 心跳事件，忽略

			case "error":
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					Err: &types.Error{
						Code:       llm.ErrUpstreamError,
						Message:    "stream error event received",
						HTTPStatus: http.StatusBadGateway,
						Retryable:  true,
						Provider:   p.Name(),
					},
				}:
				}
				return
			}
		}

		if err := stream.Err(); err != nil {
			select {
			case <-ctx.Done():
				return
			case ch <- llm.StreamChunk{
				Err: &types.Error{
					Code:       llm.ErrUpstreamError,
					Message:    err.Error(),
					Cause:      err,
					HTTPStatus: http.StatusBadGateway,
					Retryable:  true,
					Provider:   p.Name(),
				},
			}:
			}
		}
	}()

	return ch, nil
}

func toClaudeChatResponse(cr claudeResponse, provider string) *llm.ChatResponse {
	msg := types.Message{
		Role: llm.RoleAssistant,
	}

	// 解析 content 数组
	var signatures []string
	var thinkingParts []string
	var thinkingBlocks []types.ThinkingBlock
	var opaqueReasoning []types.OpaqueReasoning
	var webSearchBlocks []json.RawMessage // 保存 server_tool_use / web_search_tool_result 原始块用于多轮回传

	for _, content := range cr.Content {
		switch content.Type {
		case "text":
			msg.Content += content.Text
			// 提取引用标注
			for _, cit := range content.Citations {
				msg.Annotations = append(msg.Annotations, types.Annotation{
					Type:       "url_citation",
					URL:        cit.URL,
					Title:      cit.Title,
					StartIndex: cit.StartIndex,
					EndIndex:   cit.EndIndex,
				})
			}
		case "tool_use":
			msg.ToolCalls = append(msg.ToolCalls, providerbase.NewFunctionToolCall(content.ID, content.Name, content.Input))
		case "thinking":
			// Extended Thinking: 收集所有推理内容和签名（interleaved thinking 可能有多个）
			if content.Thinking != "" {
				thinkingParts = append(thinkingParts, content.Thinking)
			}
			if content.Signature != "" {
				signatures = append(signatures, content.Signature)
			}
			// 保存完整的 thinking block 用于 round-trip
			thinkingBlocks = append(thinkingBlocks, types.ThinkingBlock{
				Thinking:  content.Thinking,
				Signature: content.Signature,
			})
		case "redacted_thinking":
			if strings.TrimSpace(content.Data) != "" {
				opaqueReasoning = append(opaqueReasoning, types.OpaqueReasoning{
					Provider: "anthropic",
					Kind:     "redacted_thinking",
					State:    content.Data,
				})
			}
		case "server_tool_use", "web_search_tool_result":
			// 保存原始 JSON 用于多轮 round-trip
			raw, err := json.Marshal(content)
			if err == nil {
				webSearchBlocks = append(webSearchBlocks, raw)
			}
		}
	}
	if len(thinkingParts) > 0 {
		joined := strings.Join(thinkingParts, "\n\n")
		msg.ReasoningContent = &joined
	}
	if len(thinkingBlocks) > 0 {
		msg.ThinkingBlocks = thinkingBlocks
	}
	if len(opaqueReasoning) > 0 {
		msg.OpaqueReasoning = opaqueReasoning
	}

	// 保存 web search blocks 到 metadata 用于多轮回传
	if len(webSearchBlocks) > 0 {
		msg.Metadata = map[string]any{"claude_web_search_blocks": webSearchBlocks}
	}

	resp := &llm.ChatResponse{
		ID:       cr.ID,
		Provider: provider,
		Model:    cr.Model,
		Choices: []llm.ChatChoice{{
			Index:        0,
			FinishReason: cr.StopReason,
			Message:      msg,
		}},
	}

	if cr.Usage != nil {
		resp.Usage = llm.ChatUsage{
			PromptTokens:     cr.Usage.InputTokens,
			CompletionTokens: cr.Usage.OutputTokens,
			TotalTokens:      cr.Usage.InputTokens + cr.Usage.OutputTokens,
		}
		// Preserve Anthropic cache token usage when present.
		if cr.Usage.CacheCreationInputTokens > 0 || cr.Usage.CacheReadInputTokens > 0 {
			resp.Usage.PromptTokensDetails = &llm.PromptTokensDetails{
				CachedTokens:        cr.Usage.CacheReadInputTokens,
				CacheCreationTokens: cr.Usage.CacheCreationInputTokens,
			}
		}
	}

	// 传递 thinking signatures
	if len(signatures) > 0 {
		resp.ThoughtSignatures = signatures
	}

	return resp
}

func chooseMaxTokens(req *llm.ChatRequest) int {
	if req != nil && req.MaxTokens > 0 {
		return req.MaxTokens
	}
	// Claude 要求必须提供 max_tokens
	return 4096
}

// validateThinkingConstraints 校验 thinking 模式与其他参数的兼容性。
// Claude API 约束：thinking 模式只支持 tool_choice: auto 或 none。
func validateThinkingConstraints(thinking anthropicsdk.ThinkingConfigParamUnion, toolChoice anthropicsdk.ToolChoiceUnionParam) error {
	if thinking.OfEnabled == nil && thinking.OfAdaptive == nil {
		return nil
	}
	if toolChoice.OfAuto != nil || toolChoice.OfNone != nil || (toolChoice.OfAny == nil && toolChoice.OfTool == nil) {
		return nil
	}
	tcType := ""
	if toolChoice.OfAny != nil {
		tcType = "any"
	} else if toolChoice.OfTool != nil {
		tcType = "tool"
	}
	return &types.Error{
		Code:       llm.ErrInvalidRequest,
		Message:    fmt.Sprintf("Claude thinking only supports tool_choice 'auto' or 'none', got '%s'", tcType),
		HTTPStatus: http.StatusBadRequest,
		Provider:   "claude",
	}
}

func validateClaudeRequest(req *llm.ChatRequest, model string) error {
	if req == nil {
		return nil
	}
	normModel := normalizeClaudeModelName(model)
	if req.Temperature != 0 && req.TopP != 0 {
		return &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    "Claude requests should set either temperature or top_p, but not both",
			HTTPStatus: http.StatusBadRequest,
			Provider:   "claude",
		}
	}
	if strings.Contains(normModel, "opus-4-7") || strings.Contains(normModel, "opus-4.7") {
		if req.Temperature != 0 || req.TopP != 0 {
			return &types.Error{
				Code:       llm.ErrInvalidRequest,
				Message:    "Claude Opus 4.7 requires temperature and top_p to remain unspecified",
				HTTPStatus: http.StatusBadRequest,
				Provider:   "claude",
			}
		}
	}
	mode := strings.ToLower(strings.TrimSpace(req.ReasoningMode))
	if mode != "" && mode != "disabled" && req.Temperature != 0 {
		return &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    "Claude thinking mode does not support custom temperature",
			HTTPStatus: http.StatusBadRequest,
			Provider:   "claude",
		}
	}
	return nil
}

func sdkStatusCode(err error) int {
	var apiErr *anthropicsdk.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode
	}
	return 0
}

func normalizeClaudeCacheControl(in *llm.CacheControl) (*anthropicsdk.CacheControlEphemeralParam, *types.Error) {
	if in == nil {
		return nil, nil
	}

	kind := strings.ToLower(strings.TrimSpace(in.Type))
	switch kind {
	case "", "ephemeral":
		kind = "ephemeral"
	default:
		return nil, &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("Claude cache_control.type must be \"ephemeral\", got %q", in.Type),
			HTTPStatus: http.StatusBadRequest,
			Provider:   "claude",
		}
	}

	ttl := strings.ToLower(strings.TrimSpace(in.TTL))
	switch ttl {
	case "", "5m", "1h":
	default:
		return nil, &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("Claude cache_control.ttl must be one of \"5m\" or \"1h\", got %q", in.TTL),
			HTTPStatus: http.StatusBadRequest,
			Provider:   "claude",
		}
	}

	ccp := anthropicsdk.NewCacheControlEphemeralParam()
	if ttl != "" {
		ccp.TTL = anthropicsdk.CacheControlEphemeralTTL(ttl)
	}
	return &ccp, nil
}

// buildClaudeReasoningControls maps unified reasoning options into the current Claude protocol.
// Newer Claude 4.6/Mythos models prefer adaptive thinking + output_config.effort.
// Older models gracefully fall back to manual thinking budgets or standard speed.
func buildClaudeReasoningControls(req *llm.ChatRequest, model string) (anthropicsdk.ThinkingConfigParamUnion, anthropicsdk.OutputConfigParam, string) {
	if req == nil {
		return anthropicsdk.ThinkingConfigParamUnion{}, anthropicsdk.OutputConfigParam{}, ""
	}

	normModel := normalizeClaudeModelName(model)
	speed := normalizeClaudeSpeed(req.InferenceSpeed, normModel)
	var outputConfig anthropicsdk.OutputConfigParam

	if effort := normalizeClaudeEffort(req.ReasoningEffort); effort != "" && claudeModelSupportsEffort(normModel) {
		outputConfig.Effort = anthropicsdk.OutputConfigEffort(effort)
	}

	mode := strings.ToLower(strings.TrimSpace(req.ReasoningMode))
	// ThinkingType takes priority over legacy ReasoningMode for Anthropic thinking config.
	if tt := strings.ToLower(strings.TrimSpace(req.ThinkingType)); tt != "" {
		mode = tt
	}
	if mode == "" || mode == "disabled" {
		return anthropicsdk.ThinkingConfigParamUnion{}, outputConfig, speed
	}

	display := normalizeClaudeThinkingDisplay(req.ReasoningDisplay, normModel)
	switch mode {
	case "adaptive":
		if claudeModelSupportsAdaptiveThinking(normModel) {
			var adaptive anthropicsdk.ThinkingConfigAdaptiveParam
			if display != "" {
				adaptive.Display = anthropicsdk.ThinkingConfigAdaptiveDisplay(display)
			}
			return anthropicsdk.ThinkingConfigParamUnion{OfAdaptive: &adaptive}, outputConfig, speed
		}
		mode = "extended"
	case "extended", "enabled":
	default:
		return anthropicsdk.ThinkingConfigParamUnion{}, outputConfig, speed
	}

	maxTok := chooseMaxTokens(req)
	if maxTok <= 1024 {
		return anthropicsdk.ThinkingConfigParamUnion{}, outputConfig, speed
	}

	budget := maxTok * 3 / 4
	if budget < 1024 {
		budget = 1024
	}
	if budget >= maxTok {
		budget = maxTok - 1
	}

	enabled := anthropicsdk.ThinkingConfigEnabledParam{
		BudgetTokens: int64(budget),
	}
	if display != "" {
		enabled.Display = anthropicsdk.ThinkingConfigEnabledDisplay(display)
	}
	return anthropicsdk.ThinkingConfigParamUnion{OfEnabled: &enabled}, outputConfig, speed
}

func normalizeClaudeModelName(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

func claudeModelSupportsAdaptiveThinking(model string) bool {
	if model == "" {
		return false
	}
	switch {
	case strings.Contains(model, "claude-mythos-preview"):
		return true
	case strings.Contains(model, "opus-4-7"), strings.Contains(model, "opus-4.7"):
		return true
	case strings.Contains(model, "opus-4-6"), strings.Contains(model, "opus-4.6"):
		return true
	case strings.Contains(model, "sonnet-4-6"), strings.Contains(model, "sonnet-4.6"):
		return true
	default:
		return false
	}
}

func claudeModelSupportsEffort(model string) bool {
	if model == "" {
		return false
	}
	if strings.Contains(model, "claude-mythos-preview") {
		return true
	}
	return strings.Contains(model, "-4-") || strings.Contains(model, "-4.") || strings.Contains(model, "claude-4")
}

func claudeModelSupportsFastMode(model string) bool {
	if model == "" {
		return false
	}
	return strings.Contains(model, "opus-4-6") || strings.Contains(model, "opus-4.6")
}

func normalizeClaudeEffort(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "":
		return ""
	case "minimal":
		return "low"
	case "low", "medium", "high", "max":
		return strings.ToLower(strings.TrimSpace(input))
	case "xhigh":
		return "max"
	default:
		return ""
	}
}

func normalizeClaudeThinkingDisplay(input, model string) string {
	if !claudeModelSupportsEffort(model) {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "", "default":
		return ""
	case "summary", "summarized":
		return "summarized"
	case "omit", "omitted", "hidden":
		return "omitted"
	default:
		return ""
	}
}

func normalizeClaudeSpeed(input, model string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "", "standard", "default":
		return ""
	case "fast":
		if claudeModelSupportsFastMode(model) {
			return "fast"
		}
	}
	return ""
}

func shouldRetryClaudeWithoutFastMode(status int, speed string) bool {
	return status == http.StatusTooManyRequests && strings.EqualFold(strings.TrimSpace(speed), "fast")
}

// buildStreamUsage 将 Claude 的 usage 转换为统一的 ChatUsage。
func buildStreamUsage(u *claudeUsage) *llm.ChatUsage {
	if u == nil {
		return nil
	}
	usage := &llm.ChatUsage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.InputTokens + u.OutputTokens,
	}
	if u.CacheCreationInputTokens > 0 || u.CacheReadInputTokens > 0 {
		usage.PromptTokensDetails = &llm.PromptTokensDetails{
			CachedTokens:        u.CacheReadInputTokens,
			CacheCreationTokens: u.CacheCreationInputTokens,
		}
	}
	return usage
}

// detectImageMediaType 从 base64 数据的前几个字节推断图片 MIME 类型。
// 支持 PNG、JPEG、GIF、WebP。无法识别时回退到 image/png。
func detectImageMediaType(b64Data string) string {
	// 只需解码前 16 字节即可判断 magic bytes
	raw, err := base64.StdEncoding.DecodeString(b64Data[:min(24, len(b64Data))])
	if err != nil || len(raw) < 4 {
		return "image/png"
	}
	switch {
	case raw[0] == 0x89 && raw[1] == 0x50 && raw[2] == 0x4E && raw[3] == 0x47:
		return "image/png"
	case raw[0] == 0xFF && raw[1] == 0xD8:
		return "image/jpeg"
	case raw[0] == 0x47 && raw[1] == 0x49 && raw[2] == 0x46:
		return "image/gif"
	case raw[0] == 0x52 && raw[1] == 0x49 && raw[2] == 0x46 && raw[3] == 0x46 && len(raw) >= 12 &&
		raw[8] == 0x57 && raw[9] == 0x45 && raw[10] == 0x42 && raw[11] == 0x50:
		return "image/webp"
	default:
		return "image/png"
	}
}

func firstInt(values ...*int) int {
	for _, v := range values {
		if v != nil {
			return *v
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
