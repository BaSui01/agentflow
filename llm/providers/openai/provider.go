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

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
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
	p := &OpenAIProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "openai",
			APIKey:        cfg.APIKey,
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
	if !p.openaiCfg.UseResponsesAPI {
		return p.Provider.Completion(ctx, req)
	}

	// Apply rewriter chain (与基类保持一致)
	rewrittenReq, err := p.RewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &llm.Error{
			Code: llm.ErrInvalidRequest, Message: fmt.Sprintf("request rewrite failed: %v", err),
			HTTPStatus: http.StatusBadRequest, Provider: p.Name(),
		}
	}
	req = rewrittenReq

	apiKey := p.Provider.Cfg.APIKey
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			apiKey = strings.TrimSpace(c.APIKey)
		}
	}

	return p.completionWithResponsesAPI(ctx, req, apiKey)
}

// --- Responses API Types ---

// openAIResponsesRequest represents the POST /v1/responses request body.
type openAIResponsesRequest struct {
	Model              string            `json:"model"`
	Input              any               `json:"input"`                         // string or []ResponsesInputItem
	Instructions       string            `json:"instructions,omitempty"`
	MaxOutputTokens    *int              `json:"max_output_tokens,omitempty"`
	Temperature        *float32          `json:"temperature,omitempty"`
	TopP               *float32          `json:"top_p,omitempty"`
	Tools              []any             `json:"tools,omitempty"`
	ToolChoice         any               `json:"tool_choice,omitempty"`
	ParallelToolCalls  *bool             `json:"parallel_tool_calls,omitempty"`
	PreviousResponseID string            `json:"previous_response_id,omitempty"`
	Store              *bool             `json:"store,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Truncation         string            `json:"truncation,omitempty"` // "auto" or "disabled"
	Reasoning          *responsesReasoning `json:"reasoning,omitempty"`
	Text               *responsesTextParam `json:"text,omitempty"`
	ServiceTier        *string           `json:"service_tier,omitempty"`
	User               string            `json:"user,omitempty"`
	Stream             bool              `json:"stream,omitempty"`
	TopLogProbs        *int              `json:"top_logprobs,omitempty"`
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
	Type     string `json:"type"`               // "input_text", "input_image", "input_file"
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	Detail   string `json:"detail,omitempty"` // "auto", "low", "high"
}

// functionCallInputItem represents a function call in the input (for multi-turn).
type functionCallInputItem struct {
	Type      string          `json:"type"`      // "function_call"
	ID        string          `json:"id"`
	CallID    string          `json:"call_id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// functionCallOutputItem represents a function call output in the input.
type functionCallOutputItem struct {
	Type   string `json:"type"`    // "function_call_output"
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
	Type        string               `json:"type"`
	Text        string               `json:"text,omitempty"`
	Refusal     string               `json:"refusal,omitempty"`
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

	endpoint := fmt.Sprintf("%s/v1/responses", strings.TrimRight(p.openaiCfg.BaseURL, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if p.Provider.Cfg.BuildHeaders != nil {
		p.Provider.Cfg.BuildHeaders(httpReq, apiKey)
	}

	resp, err := p.Provider.Client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code: llm.ErrUpstreamError, Message: err.Error(),
			HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providers.ReadErrorMessage(resp.Body)
		return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var responsesResp openAIResponsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&responsesResp); err != nil {
		return nil, &llm.Error{
			Code: llm.ErrUpstreamError, Message: err.Error(),
			HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name(),
		}
	}

	return toResponsesAPIChatResponse(responsesResp, p.Name()), nil
}

// buildResponsesRequest converts a ChatRequest to a Responses API request.
func (p *OpenAIProvider) buildResponsesRequest(req *llm.ChatRequest) openAIResponsesRequest {
	body := openAIResponsesRequest{
		Model:             providers.ChooseModel(req, p.openaiCfg.Model, "gpt-5.2"),
		ToolChoice:        req.ToolChoice,
		Store:             req.Store,
		Metadata:          req.Metadata,
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

	// 构建 tools
	if len(req.Tools) > 0 {
		tools := make([]any, 0, len(req.Tools))
		for _, t := range req.Tools {
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
		body.Tools = tools
	}

	// Reasoning
	if req.ReasoningEffort != "" || req.ReasoningMode != "" {
		r := &responsesReasoning{}
		if req.ReasoningEffort != "" {
			r.Effort = req.ReasoningEffort
		} else if req.ReasoningMode != "" {
			r.Effort = req.ReasoningMode
		}
		body.Reasoning = r
	}

	// ResponseFormat → text.format
	if req.ResponseFormat != nil {
		body.Text = &responsesTextParam{
			Format: providers.ConvertResponseFormat(req.ResponseFormat),
		}
	}

	return body
}

// convertMessagesToResponsesInput converts messages to Responses API input format.
func convertMessagesToResponsesInput(msgs []llm.Message) []any {
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
func buildInputContent(m llm.Message) any {
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
	return parts
}

// toResponsesAPIChatResponse 将 Responses API 响应转换为统一的 llm.ChatResponse.
func toResponsesAPIChatResponse(resp openAIResponsesResponse, provider string) *llm.ChatResponse {
	var choices []llm.ChatChoice
	choiceIdx := 0

	for _, output := range resp.Output {
		switch output.Type {
		case "message":
			msg := llm.Message{Role: llm.Role(output.Role)}
			for _, content := range output.Content {
				switch content.Type {
				case "output_text":
					msg.Content += content.Text
					for _, ann := range content.Annotations {
						msg.Annotations = append(msg.Annotations, llm.Annotation{
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
					Message: llm.Message{Role: llm.RoleAssistant},
				})
				choiceIdx++
			}
			lastIdx := len(choices) - 1
			choices[lastIdx].Message.ToolCalls = append(choices[lastIdx].Message.ToolCalls, llm.ToolCall{
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
	if !p.openaiCfg.UseResponsesAPI {
		return p.Provider.Stream(ctx, req)
	}

	// Apply rewriter chain
	rewrittenReq, err := p.RewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &llm.Error{
			Code: llm.ErrInvalidRequest, Message: fmt.Sprintf("request rewrite failed: %v", err),
			HTTPStatus: http.StatusBadRequest, Provider: p.Name(),
		}
	}
	req = rewrittenReq

	apiKey := p.Provider.Cfg.APIKey
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			apiKey = strings.TrimSpace(c.APIKey)
		}
	}

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

	endpoint := fmt.Sprintf("%s/v1/responses", strings.TrimRight(p.openaiCfg.BaseURL, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if p.Provider.Cfg.BuildHeaders != nil {
		p.Provider.Cfg.BuildHeaders(httpReq, apiKey)
	}

	resp, err := p.Provider.Client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code: llm.ErrUpstreamError, Message: err.Error(),
			HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name(),
		}
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		msg := providers.ReadErrorMessage(resp.Body)
		return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
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

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{Err: &llm.Error{
						Code: llm.ErrUpstreamError, Message: err.Error(),
						HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: providerName,
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

			case "response.output_text.delta":
				delta, _ := event["delta"].(string)
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					ID: currentID, Provider: providerName, Model: currentModel,
					Delta: llm.Message{Role: llm.RoleAssistant, Content: delta},
				}:
				}

			case "response.refusal.delta":
				delta, _ := event["delta"].(string)
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					ID: currentID, Provider: providerName, Model: currentModel,
					Delta: llm.Message{Role: llm.RoleAssistant, Refusal: &delta},
				}:
				}

			case "response.function_call_arguments.delta":
				delta, _ := event["delta"].(string)
				name, _ := event["name"].(string)
				itemID, _ := event["item_id"].(string)
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					ID: currentID, Provider: providerName, Model: currentModel,
					Delta: llm.Message{
						Role: llm.RoleAssistant,
						ToolCalls: []llm.ToolCall{{
							ID:        itemID,
							Name:      name,
							Arguments: json.RawMessage(delta),
						}},
					},
				}:
				}

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
							FinishReason: "stop",
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
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					ID: currentID, Provider: providerName, Model: currentModel,
					FinishReason: "stop",
				}:
				}

			case "response.function_call_arguments.done":
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					ID: currentID, Provider: providerName, Model: currentModel,
					FinishReason: "tool_calls",
				}:
				}

			case "error":
				errMsg, _ := event["message"].(string)
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{Err: &llm.Error{
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
