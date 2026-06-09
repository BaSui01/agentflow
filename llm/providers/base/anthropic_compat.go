package providerbase

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

// =============================================================================
// Anthropic Messages API 兼容类型
// =============================================================================
// 这些类型被实现 Anthropic Messages API 格式的提供者所使用。
// 例如 DeepSeek 的 https://api.deepseek.com/anthropic 端点。

// AnthropicCompatContent 表示 Anthropic Messages API 中的 content block。
type AnthropicCompatContent struct {
	Type             string                      `json:"type"`                        // text, tool_use, image, thinking, redacted_thinking, server_tool_use, web_search_tool_result
	Text             string                      `json:"text,omitempty"`              // for type=text, thinking
	ID               string                      `json:"id,omitempty"`                // for type=tool_use
	Name             string                      `json:"name,omitempty"`              // for type=tool_use
	Input            json.RawMessage             `json:"input,omitempty"`             // for type=tool_use
	ToolUseID        string                      `json:"tool_use_id,omitempty"`       // for type=tool_result
	Content          string                      `json:"content,omitempty"`           // for type=tool_result (string form)
	IsError          *bool                       `json:"is_error,omitempty"`          // for type=tool_result
	Source           *AnthropicCompatImageSource `json:"source,omitempty"`            // for type=image
	Thinking         string                      `json:"thinking,omitempty"`          // for type=thinking
	Signature        string                      `json:"signature,omitempty"`         // for type=thinking
	Data             string                      `json:"data,omitempty"`              // for type=redacted_thinking
	Citations        []AnthropicCompatCitation   `json:"citations,omitempty"`         // for type=text (URL citations)
	SearchResults    json.RawMessage             `json:"search_results,omitempty"`    // for type=web_search_tool_result
	EncryptedContent string                      `json:"encrypted_content,omitempty"` // for server_tool_use/web_search_tool_result
	ErrorType        string                      `json:"error_type,omitempty"`        // for web_search_tool_result
}

// AnthropicCompatImageSource 表示图片块的数据源。
type AnthropicCompatImageSource struct {
	Type      string `json:"type"`                 // base64 or url
	MediaType string `json:"media_type,omitempty"` // e.g., "image/png"
	Data      string `json:"data,omitempty"`       // base64 data
	URL       string `json:"url,omitempty"`        // image URL
}

// AnthropicCompatCitation 表示文本块上的引用标注。
type AnthropicCompatCitation struct {
	Type           string `json:"type"` // "url_citation"
	URL            string `json:"url"`
	Title          string `json:"title"`
	CitedText      string `json:"cited_text,omitempty"`
	EncryptedIndex string `json:"encrypted_index,omitempty"`
	StartIndex     int    `json:"start_index,omitempty"`
	EndIndex       int    `json:"end_index,omitempty"`
}

// AnthropicCompatTool 表示 Anthropic Messages API 中的工具定义。
type AnthropicCompatTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"` // JSON Schema for parameters
}

// AnthropicCompatRequest 表示 Anthropic Messages API 请求。
type AnthropicCompatRequest struct {
	Model         string                     `json:"model"`
	Messages      []AnthropicCompatMessage   `json:"messages"`
	System        []AnthropicCompatTextBlock `json:"system,omitempty"`
	MaxTokens     int                        `json:"max_tokens"`
	Temperature   *float64                   `json:"temperature,omitempty"`
	TopP          *float64                   `json:"top_p,omitempty"`
	TopK          *int                       `json:"top_k,omitempty"`
	StopSequences []string                   `json:"stop_sequences,omitempty"`
	Tools         []AnthropicCompatTool      `json:"tools,omitempty"`
	ToolChoice    any                        `json:"tool_choice,omitempty"`
	Thinking      any                        `json:"thinking,omitempty"`
	Stream        bool                       `json:"stream,omitempty"`
	Metadata      map[string]any             `json:"metadata,omitempty"`
}

// AnthropicCompatMessage 表示 Anthropic Messages API 中的消息。
type AnthropicCompatMessage struct {
	Role    string                   `json:"role"`    // user or assistant
	Content []AnthropicCompatContent `json:"content"` // array of content blocks
}

// AnthropicCompatTextBlock 表示 system 提示中的文本块。
type AnthropicCompatTextBlock struct {
	Type string `json:"type"` // text
	Text string `json:"text"`
}

// AnthropicCompatUsage 表示 Anthropic Messages API 中的 token 用量。
type AnthropicCompatUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// AnthropicCompatResponse 表示 Anthropic Messages API 响应。
type AnthropicCompatResponse struct {
	ID           string                   `json:"id"`
	Type         string                   `json:"type"` // message
	Role         string                   `json:"role"` // assistant
	Content      []AnthropicCompatContent `json:"content"`
	Model        string                   `json:"model"`
	StopReason   string                   `json:"stop_reason"`
	StopSequence string                   `json:"stop_sequence,omitempty"`
	Usage        *AnthropicCompatUsage    `json:"usage,omitempty"`
}

// AnthropicCompatStreamEvent 表示 Anthropic Messages API 流式事件。
type AnthropicCompatStreamEvent struct {
	Type         string                   `json:"type"` // message_start, content_block_start, content_block_delta, content_block_stop, message_delta, message_stop, ping
	Index        int                      `json:"index,omitempty"`
	Delta        *AnthropicCompatDelta    `json:"delta,omitempty"`
	ContentBlock *AnthropicCompatContent  `json:"content_block,omitempty"`
	Message      *AnthropicCompatResponse `json:"message,omitempty"`
	Usage        *AnthropicCompatUsage    `json:"usage,omitempty"`
}

// AnthropicCompatDelta 表示流式响应中的增量。
type AnthropicCompatDelta struct {
	Type        string                   `json:"type"` // text_delta, input_json_delta, thinking_delta, signature_delta, citations_delta
	Text        string                   `json:"text,omitempty"`
	PartialJSON string                   `json:"partial_json,omitempty"`
	StopReason  string                   `json:"stop_reason,omitempty"`
	Thinking    string                   `json:"thinking,omitempty"`
	Signature   string                   `json:"signature,omitempty"`
	Citation    *AnthropicCompatCitation `json:"citation,omitempty"`
}

// AnthropicCompatErrorResp 表示 Anthropic Messages API 错误响应。
type AnthropicCompatErrorResp struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// AnthropicCompatParams 聚合了 Anthropic 兼容 API 调用所需的公共参数。
type AnthropicCompatParams struct {
	Client           *http.Client
	BaseURL          string
	APIKey           string
	ProviderName     string
	BuildHeadersFunc func(*http.Request, string)
}

// =============================================================================
// 转换函数
// =============================================================================

// ConvertMessagesToAnthropic 将 types.Message 切片转换为 Anthropic 兼容格式。
func ConvertMessagesToAnthropic(msgs []types.Message) (system []AnthropicCompatTextBlock, messages []AnthropicCompatMessage) {
	toolCallTypes := BuildToolCallTypeIndex(msgs)

	for _, m := range msgs {
		// 提取 system 消息
		if m.Role == llm.RoleSystem || m.Role == llm.RoleDeveloper {
			if m.Content != "" {
				system = append(system, AnthropicCompatTextBlock{
					Type: "text",
					Text: m.Content,
				})
			}
			continue
		}

		role := "user"
		if m.Role == llm.RoleAssistant {
			role = "assistant"
		}

		var blocks []AnthropicCompatContent

		// 处理 tool 角色 (作为 user 的 tool_result)
		if m.Role == llm.RoleTool {
			writeback, ok := ToolOutputFromMessage(m, toolCallTypes)
			if !ok {
				continue
			}
			tr := AnthropicCompatContent{
				Type:      "tool_result",
				ToolUseID: writeback.CallID,
				Content:   writeback.Content,
			}
			if writeback.IsError {
				isError := true
				tr.IsError = &isError
			}
			blocks = append(blocks, tr)
			messages = append(messages, AnthropicCompatMessage{
				Role:    "user",
				Content: blocks,
			})
			continue
		}

		// 处理 thinking blocks (round-trip)
		if m.Role == llm.RoleAssistant && len(m.ThinkingBlocks) > 0 {
			for _, tb := range m.ThinkingBlocks {
				blocks = append(blocks, AnthropicCompatContent{
					Type:      "thinking",
					Thinking:  tb.Thinking,
					Signature: tb.Signature,
				})
			}
		}

		// 处理 opaque reasoning (redacted_thinking)
		if m.Role == llm.RoleAssistant && len(m.OpaqueReasoning) > 0 {
			for _, opaque := range m.OpaqueReasoning {
				provider := strings.TrimSpace(opaque.Provider)
				if provider != "" && provider != "anthropic" {
					continue
				}
				if strings.TrimSpace(opaque.Kind) != "redacted_thinking" || strings.TrimSpace(opaque.State) == "" {
					continue
				}
				blocks = append(blocks, AnthropicCompatContent{
					Type: "redacted_thinking",
					Data: opaque.State,
				})
			}
		}

		// 文本内容
		if m.Content != "" {
			blocks = append(blocks, AnthropicCompatContent{
				Type: "text",
				Text: m.Content,
			})
		}

		// Images
		if len(m.Images) > 0 {
			for _, img := range m.Images {
				imgBlock := AnthropicCompatContent{
					Type: "image",
				}
				if img.Type == "base64" && img.Data != "" {
					imgBlock.Source = &AnthropicCompatImageSource{
						Type:      "base64",
						MediaType: "image/png",
						Data:      img.Data,
					}
				} else if img.Type == "url" && img.URL != "" {
					imgBlock.Source = &AnthropicCompatImageSource{
						Type: "url",
						URL:  img.URL,
					}
				}
				blocks = append(blocks, imgBlock)
			}
		}

		// Tool calls
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
				blocks = append(blocks, AnthropicCompatContent{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: json.RawMessage(mustMarshalJSON(input)),
				})
			}
		}

		if len(blocks) > 0 {
			messages = append(messages, AnthropicCompatMessage{
				Role:    role,
				Content: blocks,
			})
		}
	}

	return system, messages
}

// ConvertToolsToAnthropic 将 types.ToolSchema 切片转换为 Anthropic 兼容工具格式。
func ConvertToolsToAnthropic(tools []types.ToolSchema) []AnthropicCompatTool {
	if len(tools) == 0 {
		return nil
	}

	out := make([]AnthropicCompatTool, 0, len(tools))
	for _, t := range tools {
		toolType := NormalizeToolType(t.Type)
		if toolType != types.ToolTypeFunction {
			continue // Anthropic only supports function tools natively
		}
		if IsSearchToolPlaceholder(t.Name) {
			continue
		}

		params := ToolParametersSchemaMap(t.Parameters)
		paramsJSON := json.RawMessage("{}")
		if len(params) > 0 {
			paramsJSON = json.RawMessage(mustMarshalJSON(params))
		}

		out = append(out, AnthropicCompatTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: paramsJSON,
		})
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// ToLLMChatResponseFromAnthropic 将 Anthropic 兼容响应转换为 llm.ChatResponse。
func ToLLMChatResponseFromAnthropic(ar AnthropicCompatResponse, provider string) *llm.ChatResponse {
	msg := types.Message{
		Role: llm.RoleAssistant,
	}

	var thinkingParts []string
	var thinkingBlocks []types.ThinkingBlock
	var opaqueReasoning []types.OpaqueReasoning

	for _, content := range ar.Content {
		switch content.Type {
		case "text":
			msg.Content += content.Text
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
			msg.ToolCalls = append(msg.ToolCalls, NewFunctionToolCall(content.ID, content.Name, content.Input))
		case "thinking":
			if content.Thinking != "" {
				thinkingParts = append(thinkingParts, content.Thinking)
			}
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

	resp := &llm.ChatResponse{
		ID:       ar.ID,
		Provider: provider,
		Model:    ar.Model,
		Choices: []llm.ChatChoice{{
			Index:        0,
			FinishReason: NormalizeFinishReason(ar.StopReason),
			Message:      msg,
		}},
	}

	if ar.Usage != nil {
		resp.Usage = llm.ChatUsage{
			PromptTokens:     ar.Usage.InputTokens,
			CompletionTokens: ar.Usage.OutputTokens,
			TotalTokens:      ar.Usage.InputTokens + ar.Usage.OutputTokens,
		}
		if ar.Usage.CacheCreationInputTokens > 0 || ar.Usage.CacheReadInputTokens > 0 {
			resp.Usage.PromptTokensDetails = &llm.PromptTokensDetails{
				CachedTokens:        ar.Usage.CacheReadInputTokens,
				CacheCreationTokens: ar.Usage.CacheCreationInputTokens,
			}
		}
	}

	return resp
}

// StreamAnthropicSSE 处理 Anthropic 兼容的 SSE 流式响应。
func StreamAnthropicSSE(ctx context.Context, body io.ReadCloser, providerName string) <-chan llm.StreamChunk {
	ch := make(chan llm.StreamChunk)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				select {
				case <-ctx.Done():
				case ch <- llm.StreamChunk{Err: &types.Error{
					Code: llm.ErrUpstreamError, Message: fmt.Sprintf("stream parse panic: %v", r),
					HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: providerName,
				}}:
				}
			}
		}()
		defer body.Close()
		defer close(ch)

		reader := bufio.NewReader(body)

		type thinkingBlockState struct {
			blockType string
			thinking  strings.Builder
			signature string
			data      string
		}

		var currentID string
		var currentModel string
		var toolCallAccumulator = make(map[int]*types.ToolCall)
		var startUsage *AnthropicCompatUsage
		var thinkingAccumulator = make(map[int]*thinkingBlockState)
		var citationAccumulator = make(map[int][]AnthropicCompatCitation)

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{Err: &types.Error{
						Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err,
						HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: providerName,
					}}:
					}
				}
				return
			}
			line = strings.TrimSpace(line)
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				return
			}

			var event AnthropicCompatStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{Err: &types.Error{
					Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err,
					HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: providerName,
				}}:
				}
				return
			}

			switch event.Type {
			case "message_start":
				if event.Message != nil {
					currentID = event.Message.ID
					currentModel = event.Message.Model
					if event.Message.Usage != nil {
						startUsage = event.Message.Usage
					}
				}

			case "content_block_start":
				if event.ContentBlock != nil {
					switch event.ContentBlock.Type {
					case "tool_use":
						call := NewFunctionToolCall(event.ContentBlock.ID, event.ContentBlock.Name, nil)
						toolCallAccumulator[event.Index] = &call
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
					var sendChunk bool
					chunk := llm.StreamChunk{
						ID:       currentID,
						Provider: providerName,
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
						if tc, ok := toolCallAccumulator[event.Index]; ok {
							tc.Arguments = AppendToolJSONDelta(tc.Arguments, event.Delta.PartialJSON)
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
					case "citations_delta":
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
				if tc, ok := toolCallAccumulator[event.Index]; ok {
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{
						ID:       currentID,
						Provider: providerName,
						Model:    currentModel,
						Index:    event.Index,
						Delta: types.Message{
							Role:      llm.RoleAssistant,
							ToolCalls: ToolCallChunk(*tc),
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
								Provider: providerName,
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
								Provider: providerName,
								Model:    currentModel,
								Index:    event.Index,
								Delta: types.Message{
									Role: llm.RoleAssistant,
									OpaqueReasoning: []types.OpaqueReasoning{{
										Provider:  providerName,
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
						Provider: providerName,
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

			case "message_delta":
				chunk := llm.StreamChunk{
					ID:       currentID,
					Provider: providerName,
					Model:    currentModel,
				}
				if event.Delta != nil && event.Delta.StopReason != "" {
					chunk.FinishReason = NormalizeFinishReason(event.Delta.StopReason)
				}
				if event.Usage != nil {
					chunk.Usage = buildAnthropicCompatStreamUsage(event.Usage, startUsage)
				} else if startUsage != nil {
					chunk.Usage = buildAnthropicCompatStreamUsage(startUsage, nil)
				}
				select {
				case <-ctx.Done():
					return
				case ch <- chunk:
				}

			case "message_stop":
				return

			case "ping":
				// heartbeat, ignore

			case "error":
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{Err: &types.Error{
					Code:       llm.ErrUpstreamError,
					Message:    "stream error event received",
					HTTPStatus: http.StatusBadGateway,
					Retryable:  true,
					Provider:   providerName,
				}}:
				}
				return
			}
		}
	}()
	return ch
}

// =============================================================================
// Anthropic 兼容 API 辅助函数
// =============================================================================

// ListModelsAnthropicCompat 通用的 Anthropic 兼容 Provider 模型列表获取函数。
func ListModelsAnthropicCompat(ctx context.Context, client *http.Client, baseURL, apiKey, providerName, modelsEndpoint string, buildHeadersFunc func(*http.Request, string)) ([]llm.Model, error) {
	endpoint := fmt.Sprintf("%s%s", strings.TrimRight(baseURL, "/"), modelsEndpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	buildHeadersFunc(httpReq, apiKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  providerName,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := ReadErrorMessage(resp.Body)
		return nil, MapHTTPError(resp.StatusCode, msg, providerName)
	}

	type modelData struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		CreatedAt   string `json:"created_at"`
		DisplayName string `json:"display_name"`
	}

	var modelsResp struct {
		Data    []modelData `json:"data"`
		HasMore bool        `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  providerName,
		}
	}

	models := make([]llm.Model, 0, len(modelsResp.Data))
	for _, m := range modelsResp.Data {
		models = append(models, llm.Model{
			ID:      m.ID,
			Object:  m.Type,
			OwnedBy: providerName,
		})
	}
	return models, nil
}

// NormalizeAnthropicToolChoice 规范化 tool_choice 参数。
func NormalizeAnthropicToolChoice(tc any) any {
	if tc == nil {
		return nil
	}
	spec := NormalizeToolChoice(tc)
	switch spec.Mode {
	case "auto":
		return map[string]string{"type": "auto"}
	case "any":
		return map[string]string{"type": "any"}
	case "none":
		return map[string]string{"type": "none"}
	case "tool":
		return map[string]string{"type": "tool", "name": spec.SpecificName}
	default:
		return nil
	}
}

// NormalizeAnthropicThinking 规范化 thinking 参数。
func NormalizeAnthropicThinking(thinkingType string, budgetTokens int) any {
	switch strings.ToLower(strings.TrimSpace(thinkingType)) {
	case "enabled":
		return map[string]any{
			"type":          "enabled",
			"budget_tokens": budgetTokens,
		}
	case "disabled":
		return map[string]string{"type": "disabled"}
	case "adaptive":
		return map[string]string{"type": "adaptive"}
	default:
		return nil
	}
}

// =============================================================================
// 内部辅助函数
// =============================================================================

func mustMarshalJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func buildAnthropicCompatStreamUsage(u *AnthropicCompatUsage, startUsage *AnthropicCompatUsage) *llm.ChatUsage {
	if u == nil {
		return nil
	}
	inputTokens := u.InputTokens
	if inputTokens == 0 && startUsage != nil {
		inputTokens = startUsage.InputTokens
	}
	outputTokens := u.OutputTokens
	usage := &llm.ChatUsage{
		PromptTokens:     inputTokens,
		CompletionTokens: outputTokens,
		TotalTokens:      inputTokens + outputTokens,
	}
	cacheCreation := u.CacheCreationInputTokens
	if cacheCreation == 0 && startUsage != nil {
		cacheCreation = startUsage.CacheCreationInputTokens
	}
	cacheRead := u.CacheReadInputTokens
	if cacheRead == 0 && startUsage != nil {
		cacheRead = startUsage.CacheReadInputTokens
	}
	if cacheCreation > 0 || cacheRead > 0 {
		usage.PromptTokensDetails = &llm.PromptTokensDetails{
			CachedTokens:        cacheRead,
			CacheCreationTokens: cacheCreation,
		}
	}
	return usage
}
