package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/middleware"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"github.com/BaSui01/agentflow/types"
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

func (p *ClaudeProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	endpoint := fmt.Sprintf("%s/v1/models", strings.TrimRight(p.cfg.BaseURL, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		return &llm.HealthStatus{Healthy: false, Latency: latency}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return &llm.HealthStatus{Healthy: false, Latency: latency}, fmt.Errorf("claude health check failed: status=%d msg=%s", resp.StatusCode, msg)
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
	endpoint := fmt.Sprintf("%s/v1/models", strings.TrimRight(p.cfg.BaseURL, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	type claudeModelPayload struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Model       string `json:"model"`
		DisplayName string `json:"display_name"`
		CreatedAt   string `json:"created_at"`
		Created     int64  `json:"created"`
		Type        string `json:"type"`
		OwnedBy     string `json:"owned_by"`
	}
	var modelsResp struct {
		Data   []claudeModelPayload `json:"data"`
		Models []claudeModelPayload `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}

	source := modelsResp.Data
	if len(source) == 0 && len(modelsResp.Models) > 0 {
		source = modelsResp.Models
	}

	models := make([]llm.Model, 0, len(source))
	for _, m := range source {
		id := strings.TrimSpace(firstNonEmpty(m.ID, m.Model, strings.TrimPrefix(m.Name, "models/")))
		if id == "" {
			continue
		}
		var created int64
		if t, err := time.Parse(time.RFC3339, m.CreatedAt); err == nil {
			created = t.Unix()
		} else if m.Created > 0 {
			created = m.Created
		}
		models = append(models, llm.Model{
			ID:      id,
			Object:  "model",
			Created: created,
			OwnedBy: strings.TrimSpace(firstNonEmpty(m.OwnedBy, "anthropic")),
		})
	}

	return models, nil
}

// Claude 的消息结构与 OpenAI 不同
type claudeMessage struct {
	Role    string          `json:"role"` // user 或 assistant
	Content []claudeContent `json:"content"`
}

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

type claudeTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"` // JSON Schema
}

type claudeToolChoice struct {
	Type                   string `json:"type"`                                // "auto", "any", "tool", "none"
	Name                   string `json:"name,omitempty"`                      // 仅 type="tool" 时
	DisableParallelToolUse *bool  `json:"disable_parallel_tool_use,omitempty"` // 可选
}

// claudeThinking 控制 Claude 的 Extended Thinking 功能。
type claudeThinking struct {
	Type         string `json:"type"`                    // "enabled" or "disabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // type="enabled" 时必填，最小 1024
}

type claudeRequest struct {
	Model       string            `json:"model"`
	Messages    []claudeMessage   `json:"messages"`
	System      any               `json:"system,omitempty"` // string 或 []claudeSystemBlock（支持 cache_control）
	MaxTokens   int               `json:"max_tokens"`
	Temperature *float32          `json:"temperature,omitempty"` // 指针类型：区分 "未设置" 和 "显式设为 0"
	TopP        *float32          `json:"top_p,omitempty"`       // 指针类型：同上
	StopSeq     []string          `json:"stop_sequences,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
	Tools       []json.RawMessage `json:"tools,omitempty"` // 混合类型：普通 function tool + server tool (web_search)
	ToolChoice  *claudeToolChoice `json:"tool_choice,omitempty"`
	Thinking    *claudeThinking   `json:"thinking,omitempty"` // Extended Thinking
}

type claudeUsage struct {
	InputTokens              int                  `json:"input_tokens"`
	OutputTokens             int                  `json:"output_tokens"`
	CacheCreationInputTokens int                  `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int                  `json:"cache_read_input_tokens,omitempty"`
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
		ServerToolUse                 *claudeServerToolUse `json:"server_tool_use"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	u.InputTokens = firstInt(aux.InputTokens, aux.InputTokensCamel, aux.PromptTokens)
	u.OutputTokens = firstInt(aux.OutputTokens, aux.OutputTokensCamel, aux.CompletionTokens)
	u.CacheCreationInputTokens = firstInt(aux.CacheCreationInputTokens, aux.CacheCreationInputTokensCamel)
	u.CacheReadInputTokens = firstInt(aux.CacheReadInputTokens, aux.CacheReadInputTokensCamel)
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

func (p *ClaudeProvider) buildHeaders(req *http.Request, apiKey string) {
	req.Header.Set("x-api-key", apiKey)
	// API 版本：可配置，默认 2025-04-14（支持 web_search 服务端工具）
	version := p.cfg.AnthropicVersion
	if version == "" {
		version = "2025-04-14"
	}
	req.Header.Set("anthropic-version", version)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
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

// convertToClaudeMessages 将统一格式转换为 Claude 格式
// Claude 的特殊要求：
// 1. system 消息需要单独提取到 system 字段
// 2. 消息必须是 user/assistant 交替出现
// 3. content 是数组形式，可包含文本和工具调用
func convertToClaudeMessages(msgs []types.Message) (string, []claudeMessage) {
	var systemParts []string
	var claudeMsgs []claudeMessage

	for _, m := range msgs {
		// 提取 system 消息（多条拼接）
		if m.Role == llm.RoleSystem || m.Role == llm.RoleDeveloper {
			if m.Content != "" {
				systemParts = append(systemParts, m.Content)
			}
			continue
		}

		// 处理 tool 角色（Claude 将其作为 assistant 的 tool_result）
		if m.Role == llm.RoleTool {
			// Tool 结果需要包装成 user 消息
			tr := claudeContent{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   m.Content,
			}
			// 问题 4: 传递 is_error 标记，让模型知道工具执行失败
			if m.IsToolError {
				isErr := true
				tr.IsError = &isErr
			}
			claudeMsgs = append(claudeMsgs, claudeMessage{
				Role:    "user",
				Content: []claudeContent{tr},
			})
			continue
		}

		// 构建普通消息
		role := "user"
		if m.Role == llm.RoleAssistant {
			role = "assistant"
		}
		cm := claudeMessage{Role: role}

		// 问题 1: assistant 消息的 ThinkingBlocks 需要回传为 thinking content blocks
		if m.Role == llm.RoleAssistant && len(m.ThinkingBlocks) > 0 {
			for _, tb := range m.ThinkingBlocks {
				cm.Content = append(cm.Content, claudeContent{
					Type:      "thinking",
					Thinking:  tb.Thinking,
					Signature: tb.Signature,
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
				cm.Content = append(cm.Content, claudeContent{
					Type: "redacted_thinking",
					Data: opaque.State,
				})
			}
		}

		// 文本内容
		if m.Content != "" {
			cm.Content = append(cm.Content, claudeContent{
				Type: "text",
				Text: m.Content,
			})
		}

		// Images 转换为 Claude 的 image content blocks
		if len(m.Images) > 0 {
			for _, img := range m.Images {
				if img.Type == "base64" && img.Data != "" {
					cm.Content = append(cm.Content, claudeContent{
						Type: "image",
						Source: &claudeImageSource{
							Type:      "base64",
							MediaType: detectImageMediaType(img.Data),
							Data:      img.Data,
						},
					})
				} else if img.Type == "url" && img.URL != "" {
					cm.Content = append(cm.Content, claudeContent{
						Type: "image",
						Source: &claudeImageSource{
							Type: "url",
							URL:  img.URL,
						},
					})
				}
			}
		}

		// ToolCall 转换
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				cm.Content = append(cm.Content, claudeContent{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: tc.Arguments,
				})
			}
		}

		if len(cm.Content) > 0 {
			// 多轮回传：如果 assistant 消息的 Metadata 中包含 web search blocks，追加到 content 数组
			if m.Role == llm.RoleAssistant {
				if meta, ok := m.Metadata.(map[string]any); ok {
					if rawBlocks, ok := meta["claude_web_search_blocks"]; ok {
						switch blocks := rawBlocks.(type) {
						case []json.RawMessage:
							for _, raw := range blocks {
								var block claudeContent
								if json.Unmarshal(raw, &block) == nil {
									cm.Content = append(cm.Content, block)
								}
							}
						case []any:
							// 经过 JSON round-trip 后可能变成 []any
							for _, item := range blocks {
								raw, err := json.Marshal(item)
								if err != nil {
									continue
								}
								var block claudeContent
								if json.Unmarshal(raw, &block) == nil {
									cm.Content = append(cm.Content, block)
								}
							}
						}
					}
				}
			}
			claudeMsgs = append(claudeMsgs, cm)
		}
	}

	return strings.Join(systemParts, "\n\n"), claudeMsgs
}

// convertToClaudeTools 将统一工具列表转换为 Claude API 的混合工具数组。
// 当 wsOpts 不为 nil 或工具列表中包含 web_search 时，自动注入 web_search_20250305 服务端工具。
func convertToClaudeTools(tools []types.ToolSchema, wsOpts *llm.WebSearchOptions) []json.RawMessage {
	hasWebSearch := wsOpts != nil
	out := make([]json.RawMessage, 0, len(tools)+1)

	for _, t := range tools {
		// 跳过客户端传入的 web_search 占位工具（避免双重注入）
		if t.Name == "web_search" || t.Name == "google_search" {
			hasWebSearch = true
			continue
		}
		raw, err := json.Marshal(claudeTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
		if err != nil {
			continue
		}
		out = append(out, raw)
	}

	// 注入 web_search_20250305 服务端工具
	if hasWebSearch {
		ws := map[string]any{
			"type": "web_search_20250305",
			"name": "web_search",
		}
		if wsOpts != nil {
			if len(wsOpts.AllowedDomains) > 0 {
				ws["allowed_domains"] = wsOpts.AllowedDomains
			}
			if len(wsOpts.BlockedDomains) > 0 {
				ws["blocked_domains"] = wsOpts.BlockedDomains
			}
			if wsOpts.MaxUses > 0 {
				ws["max_uses"] = wsOpts.MaxUses
			}
			if wsOpts.UserLocation != nil {
				loc := map[string]string{}
				if wsOpts.UserLocation.Type != "" {
					loc["type"] = wsOpts.UserLocation.Type
				}
				if wsOpts.UserLocation.Country != "" {
					loc["country"] = wsOpts.UserLocation.Country
				}
				if wsOpts.UserLocation.Region != "" {
					loc["region"] = wsOpts.UserLocation.Region
				}
				if wsOpts.UserLocation.City != "" {
					loc["city"] = wsOpts.UserLocation.City
				}
				if wsOpts.UserLocation.Timezone != "" {
					loc["timezone"] = wsOpts.UserLocation.Timezone
				}
				if len(loc) > 0 {
					ws["user_location"] = loc
				}
			}
		}
		raw, _ := json.Marshal(ws)
		out = append(out, raw)
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// convertClaudeToolChoice 将 llm.ChatRequest.ToolChoice (any) 转换为 Claude 格式。
// 支持 string ("auto"/"any"/"none") 和 map/struct 形式。
func convertClaudeToolChoice(tc any) *claudeToolChoice {
	if tc == nil {
		return nil
	}
	switch v := tc.(type) {
	case string:
		switch v {
		case "auto":
			return &claudeToolChoice{Type: "auto"}
		case "any", "required":
			return &claudeToolChoice{Type: "any"}
		case "none":
			return &claudeToolChoice{Type: "none"}
		case "":
			return nil
		default:
			// 假设是具体工具名
			return &claudeToolChoice{Type: "tool", Name: v}
		}
	case map[string]any:
		result := &claudeToolChoice{}
		if t, ok := v["type"].(string); ok {
			result.Type = t
		}
		if n, ok := v["name"].(string); ok {
			result.Name = n
		}
		return result
	default:
		return nil
	}
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

	body := claudeRequest{
		Model:       providerbase.ChooseModel(req, p.cfg.Model, "claude-opus-4.5-20260105"),
		Messages:    messages,
		System:      system,
		MaxTokens:   chooseMaxTokens(req),
		Temperature: float32PtrIfSet(req.Temperature),
		TopP:        float32PtrIfSet(req.TopP),
		StopSeq:     req.Stop,
		Tools:       convertToClaudeTools(req.Tools, req.WebSearchOptions),
		ToolChoice:  convertClaudeToolChoice(req.ToolChoice),
		Thinking:    buildThinking(req),
	}

	// 问题 3: thinking + tool_choice 冲突校验
	if err := validateThinkingConstraints(&body); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	endpoint := fmt.Sprintf("%s/v1/messages", strings.TrimRight(p.cfg.BaseURL, "/"))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var claudeResp claudeResponse
	if err := json.NewDecoder(resp.Body).Decode(&claudeResp); err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
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

	body := claudeRequest{
		Model:       providerbase.ChooseModel(req, p.cfg.Model, "claude-opus-4.5-20260105"),
		Messages:    messages,
		System:      system,
		MaxTokens:   chooseMaxTokens(req),
		Temperature: float32PtrIfSet(req.Temperature),
		TopP:        float32PtrIfSet(req.TopP),
		StopSeq:     req.Stop,
		Stream:      true,
		Tools:       convertToClaudeTools(req.Tools, req.WebSearchOptions),
		ToolChoice:  convertClaudeToolChoice(req.ToolChoice),
		Thinking:    buildThinking(req),
	}

	// 问题 3: thinking + tool_choice 冲突校验
	if err := validateThinkingConstraints(&body); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("failed to marshal request: %v", err),
			HTTPStatus: http.StatusBadRequest,
			Provider:   p.Name(),
			Cause:      err,
		}
	}
	endpoint := fmt.Sprintf("%s/v1/messages", strings.TrimRight(p.cfg.BaseURL, "/"))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, &types.Error{
			Code:       llm.ErrInternalError,
			Message:    fmt.Sprintf("failed to create request: %v", err),
			HTTPStatus: http.StatusInternalServerError,
			Provider:   p.Name(),
			Cause:      err,
		}
	}
	p.buildHeaders(httpReq, apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	ch := make(chan llm.StreamChunk)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		reader := bufio.NewReader(resp.Body)

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

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{
						Err: &types.Error{
							Code:    llm.ErrUpstreamError,
							Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
							Retryable: true,
							Provider:  p.Name(),
						},
					}:
					}
				}
				return
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Claude SSE 格式：event: <type>\ndata: <json>
			if strings.HasPrefix(line, "event:") {
				continue
			}

			if !strings.HasPrefix(line, "data:") {
				continue
			}

			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

			var event claudeStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					Err: &types.Error{
						Code:    llm.ErrUpstreamError,
						Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
						Retryable: true,
						Provider:  p.Name(),
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
						// Bug1 fix: 初始化 Arguments 为 nil，由 input_json_delta 逐步构建
						toolCallAccumulator[event.Index] = &types.ToolCall{
							ID:   event.ContentBlock.ID,
							Name: event.ContentBlock.Name,
						}
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
							tc.Arguments = append(tc.Arguments, []byte(event.Delta.PartialJSON)...)
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
							ToolCalls: []types.ToolCall{*tc},
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
				// 流式错误事件，解析实际错误信息
				errMsg := "stream error event received"
				var errEvent claudeErrorResp
				if json.Unmarshal([]byte(data), &errEvent) == nil && errEvent.Error.Message != "" {
					errMsg = fmt.Sprintf("%s: %s", errEvent.Error.Type, errEvent.Error.Message)
				}
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					Err: &types.Error{
						Code:       llm.ErrUpstreamError,
						Message:    errMsg,
						HTTPStatus: http.StatusBadGateway,
						Retryable:  true,
						Provider:   p.Name(),
					},
				}:
				}
				return
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
			msg.ToolCalls = append(msg.ToolCalls, types.ToolCall{
				ID:        content.ID,
				Name:      content.Name,
				Arguments: content.Input,
			})
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
		// Bug7 fix: 映射 cache token 用量
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

// float32PtrIfSet 将 float32 转为指针。零值返回 nil（不发送），非零值返回指针。
// 注意：这意味着无法通过 ChatRequest.Temperature=0 显式发送 temperature:0。
// 这是 ChatRequest 使用非指针 float32 的已知限制。
func float32PtrIfSet(v float32) *float32 {
	if v == 0 {
		return nil
	}
	return &v
}

// validateThinkingConstraints 校验 thinking 模式与其他参数的兼容性。
// Claude API 约束：thinking 模式只支持 tool_choice: auto 或 none。
func validateThinkingConstraints(body *claudeRequest) error {
	if body.Thinking == nil || body.Thinking.Type != "enabled" {
		return nil
	}
	if body.ToolChoice != nil {
		switch body.ToolChoice.Type {
		case "auto", "none":
			// 允许
		default:
			return &types.Error{
				Code:       llm.ErrInvalidRequest,
				Message:    fmt.Sprintf("extended thinking only supports tool_choice 'auto' or 'none', got '%s'", body.ToolChoice.Type),
				HTTPStatus: http.StatusBadRequest,
				Provider:   "claude",
			}
		}
	}
	return nil
}

// buildThinking 将统一的 ReasoningMode 转换为 Claude 的 Thinking 参数。
// 约束：budget_tokens 必须 < max_tokens 且 >= 1024。
// 如果 max_tokens 太小无法满足最低 budget，则不启用 thinking。
func buildThinking(req *llm.ChatRequest) *claudeThinking {
	if req == nil || req.ReasoningMode == "" {
		return nil
	}
	switch req.ReasoningMode {
	case "extended":
		maxTok := chooseMaxTokens(req)
		// budget_tokens 必须 < max_tokens，且最小 1024
		// 如果 max_tokens <= 1024，无法满足约束，不启用 thinking
		if maxTok <= 1024 {
			return nil
		}
		budget := maxTok * 3 / 4
		if budget < 1024 {
			budget = 1024
		}
		// 确保 budget < max_tokens
		if budget >= maxTok {
			budget = maxTok - 1
		}
		return &claudeThinking{
			Type:         "enabled",
			BudgetTokens: budget,
		}
	default:
		return nil
	}
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
