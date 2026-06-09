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
// Gemini generateContent API 兼容类型
// =============================================================================
// 这些类型被实现 Gemini generateContent API 格式的提供者所使用。

// GeminiCompatContent 表示 Gemini API 中的 Content（消息）。
type GeminiCompatContent struct {
	Role  string             `json:"role,omitempty"` // user or model
	Parts []GeminiCompatPart `json:"parts"`
}

// GeminiCompatPart 表示 Gemini Content 中的 Part。
type GeminiCompatPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *GeminiCompatFuncCall   `json:"functionCall,omitempty"`
	FunctionResponse *GeminiCompatFuncResp   `json:"functionResponse,omitempty"`
	InlineData       *GeminiCompatInlineData `json:"inlineData,omitempty"`
	Thought          bool                    `json:"thought,omitempty"` // boolean flag for thought content
}

// GeminiCompatFuncCall 表示 Gemini FunctionCall。
type GeminiCompatFuncCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

// GeminiCompatFuncResp 表示 Gemini FunctionResponse。
type GeminiCompatFuncResp struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

// GeminiCompatInlineData 表示内联数据（图片等）。
type GeminiCompatInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64
}

// GeminiCompatTool 表示 Gemini Tool 定义。
type GeminiCompatTool struct {
	FunctionDeclarations []GeminiCompatFuncDecl    `json:"functionDeclarations,omitempty"`
	GoogleSearch         *GeminiCompatGoogleSearch `json:"googleSearch,omitempty"`
}

// GeminiCompatFuncDecl 表示 function declaration。
type GeminiCompatFuncDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// GeminiCompatGoogleSearch represents the Google Search grounding tool.
type GeminiCompatGoogleSearch struct{}

// GeminiCompatToolConfig 表示 tool config。
type GeminiCompatToolConfig struct {
	FunctionCallingConfig *GeminiCompatFuncCallingConfig `json:"functionCallingConfig,omitempty"`
}

// GeminiCompatFuncCallingConfig 表示 function calling config。
type GeminiCompatFuncCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"` // AUTO, ANY, NONE
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

// GeminiCompatGenerationConfig 表示 generation config。
type GeminiCompatGenerationConfig struct {
	Temperature      *float32              `json:"temperature,omitempty"`
	TopP             *float32              `json:"topP,omitempty"`
	TopK             *int32                `json:"topK,omitempty"`
	MaxOutputTokens  int32                 `json:"maxOutputTokens,omitempty"`
	StopSequences    []string              `json:"stopSequences,omitempty"`
	ResponseMimeType string                `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any        `json:"responseSchema,omitempty"`
	ThinkingConfig   *GeminiCompatThinking `json:"thinkingConfig,omitempty"`
}

// GeminiCompatThinking 表示 thinking config。
type GeminiCompatThinking struct {
	IncludeThoughts bool   `json:"includeThoughts,omitempty"`
	ThinkingBudget  *int32 `json:"thinkingBudget,omitempty"`
	ThinkingLevel   string `json:"thinkingLevel,omitempty"` // minimal, low, medium, high
}

// GeminiCompatRequest 表示 Gemini generateContent 请求。
type GeminiCompatRequest struct {
	Contents          []GeminiCompatContent         `json:"contents"`
	SystemInstruction *GeminiCompatContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *GeminiCompatGenerationConfig `json:"generationConfig,omitempty"`
	Tools             []GeminiCompatTool            `json:"tools,omitempty"`
	ToolConfig        *GeminiCompatToolConfig       `json:"toolConfig,omitempty"`
}

// GeminiCompatCandidate 表示 Gemini 响应中的候选。
type GeminiCompatCandidate struct {
	Content      *GeminiCompatContent `json:"content"`
	FinishReason string               `json:"finishReason,omitempty"`
	Index        int32                `json:"index,omitempty"`
}

// GeminiCompatUsage 表示 Gemini token 用量。
type GeminiCompatUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// GeminiCompatResponse 表示 Gemini generateContent 响应。
type GeminiCompatResponse struct {
	Candidates    []GeminiCompatCandidate `json:"candidates"`
	UsageMetadata *GeminiCompatUsage      `json:"usageMetadata,omitempty"`
	ModelVersion  string                  `json:"modelVersion,omitempty"`
}

// GeminiCompatParams 聚合了 Gemini 兼容 API 调用所需的公共参数。
type GeminiCompatParams struct {
	Client           *http.Client
	BaseURL          string
	APIKey           string
	ProviderName     string
	BuildHeadersFunc func(*http.Request, string)
}

// =============================================================================
// 转换函数
// =============================================================================

// ConvertMessagesToGemini 将 types.Message 切片转换为 Gemini 兼容格式。
func ConvertMessagesToGemini(msgs []types.Message) (systemInstruction *GeminiCompatContent, contents []GeminiCompatContent) {
	toolCallTypes := BuildToolCallTypeIndex(msgs)

	for _, m := range msgs {
		// Extract system instruction
		if m.Role == llm.RoleSystem || m.Role == llm.RoleDeveloper {
			if m.Content != "" {
				systemInstruction = &GeminiCompatContent{
					Role:  "user",
					Parts: []GeminiCompatPart{{Text: m.Content}},
				}
			}
			continue
		}

		role := "user"
		if m.Role == llm.RoleAssistant {
			role = "model"
		}

		parts := make([]GeminiCompatPart, 0)

		// Reasoning / thought content for assistant
		if m.Role == llm.RoleAssistant && m.ReasoningContent != nil && strings.TrimSpace(*m.ReasoningContent) != "" {
			parts = append(parts, GeminiCompatPart{
				Text:    *m.ReasoningContent,
				Thought: true,
			})
		}

		// Handle tool output (functionResponse)
		if m.Role == llm.RoleTool && m.ToolCallID != "" {
			writeback, ok := ToolOutputFromMessage(m, toolCallTypes)
			if !ok {
				continue
			}
			parts = append(parts, GeminiCompatPart{
				FunctionResponse: &GeminiCompatFuncResp{
					Name:     writeback.Name,
					Response: BuildGeminiFunctionResponse(writeback),
				},
			})
			contents = append(contents, GeminiCompatContent{
				Role:  "user",
				Parts: parts,
			})
			continue
		}

		// Text content (non-tool)
		if m.Content != "" && m.Role != llm.RoleTool {
			parts = append(parts, GeminiCompatPart{
				Text: m.Content,
			})
		}

		// Images as inline data
		for _, img := range m.Images {
			if img.Type == "base64" && img.Data != "" {
				parts = append(parts, GeminiCompatPart{
					InlineData: &GeminiCompatInlineData{
						MimeType: "image/png",
						Data:     img.Data,
					},
				})
			}
		}

		// Tool calls (functionCall)
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				var args map[string]any
				if len(tc.Arguments) > 0 {
					if err := json.Unmarshal(tc.Arguments, &args); err != nil {
						continue
					}
				} else {
					args = map[string]any{}
				}
				parts = append(parts, GeminiCompatPart{
					FunctionCall: &GeminiCompatFuncCall{
						Name: tc.Name,
						Args: args,
					},
				})
			}
		}

		if len(parts) > 0 {
			contents = append(contents, GeminiCompatContent{
				Role:  role,
				Parts: parts,
			})
		}
	}

	return systemInstruction, contents
}

// ConvertToolsToGemini 将 types.ToolSchema 切片转换为 Gemini 兼容工具格式。
func ConvertToolsToGemini(tools []types.ToolSchema, wsOpts *llm.WebSearchOptions) []GeminiCompatTool {
	needGoogleSearch := wsOpts != nil
	declarations := make([]GeminiCompatFuncDecl, 0, len(tools))
	for _, t := range tools {
		if IsSearchToolPlaceholder(t.Name) {
			needGoogleSearch = true
			continue
		}
		params := ToolParametersSchemaMap(t.Parameters)
		if len(params) == 0 {
			continue
		}
		declarations = append(declarations, GeminiCompatFuncDecl{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  params,
		})
	}

	result := make([]GeminiCompatTool, 0, 2)
	if len(declarations) > 0 {
		result = append(result, GeminiCompatTool{
			FunctionDeclarations: declarations,
		})
	}
	if needGoogleSearch {
		result = append(result, GeminiCompatTool{
			GoogleSearch: &GeminiCompatGoogleSearch{},
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// ConvertToolChoiceToGemini 将 tool_choice 转换为 Gemini toolConfig。
func ConvertToolChoiceToGemini(toolChoice any) *GeminiCompatToolConfig {
	spec := NormalizeToolChoice(toolChoice)
	var config *GeminiCompatToolConfig

	switch spec.Mode {
	case "auto":
		config = &GeminiCompatToolConfig{
			FunctionCallingConfig: &GeminiCompatFuncCallingConfig{
				Mode: "AUTO",
			},
		}
	case "any":
		config = &GeminiCompatToolConfig{
			FunctionCallingConfig: &GeminiCompatFuncCallingConfig{
				Mode:                 "ANY",
				AllowedFunctionNames: spec.AllowedFunctionNames,
			},
		}
	case "none":
		config = &GeminiCompatToolConfig{
			FunctionCallingConfig: &GeminiCompatFuncCallingConfig{
				Mode: "NONE",
			},
		}
	case "tool":
		config = &GeminiCompatToolConfig{
			FunctionCallingConfig: &GeminiCompatFuncCallingConfig{
				Mode:                 "ANY",
				AllowedFunctionNames: []string{spec.SpecificName},
			},
		}
	}

	return config
}

// ToLLMChatResponseFromGemini 将 Gemini 兼容响应转换为 llm.ChatResponse。
func ToLLMChatResponseFromGemini(gr GeminiCompatResponse, provider string) *llm.ChatResponse {
	if len(gr.Candidates) == 0 {
		return &llm.ChatResponse{Provider: provider}
	}

	choices := make([]llm.ChatChoice, 0, len(gr.Candidates))
	for _, candidate := range gr.Candidates {
		msg := messageFromGeminiCompatCandidate(candidate, provider)
		choices = append(choices, llm.ChatChoice{
			Index:        int(candidate.Index),
			FinishReason: NormalizeFinishReason(candidate.FinishReason),
			Message:      msg,
		})
	}

	resp := &llm.ChatResponse{
		Provider: provider,
		Model:    gr.ModelVersion,
		Choices:  choices,
	}

	if gr.UsageMetadata != nil {
		resp.Usage = llm.ChatUsage{
			PromptTokens:     gr.UsageMetadata.PromptTokenCount,
			CompletionTokens: gr.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gr.UsageMetadata.TotalTokenCount,
		}
	}

	return resp
}

// StreamGeminiSSE 处理 Gemini 兼容的 SSE 流式响应。
func StreamGeminiSSE(ctx context.Context, body io.ReadCloser, providerName string) <-chan llm.StreamChunk {
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
			if line == "" {
				continue
			}

			// Gemini streaming wraps in array format: [json]\n,json]\n...
			// Strip leading '[' and trailing ']' if present, and trailing ','
			data := line
			data = strings.TrimPrefix(data, "[")
			data = strings.TrimSuffix(data, "]")
			data = strings.TrimSuffix(data, ",")
			data = strings.TrimSpace(data)
			if data == "" {
				continue
			}

			var gr GeminiCompatResponse
			if err := json.Unmarshal([]byte(data), &gr); err != nil {
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

			for _, candidate := range gr.Candidates {
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					Provider:     providerName,
					Model:        gr.ModelVersion,
					Index:        int(candidate.Index),
					FinishReason: NormalizeFinishReason(candidate.FinishReason),
					Delta:        messageFromGeminiCompatCandidate(candidate, providerName),
				}:
				}
			}

			if gr.UsageMetadata != nil {
				usage := &llm.ChatUsage{
					PromptTokens:     gr.UsageMetadata.PromptTokenCount,
					CompletionTokens: gr.UsageMetadata.CandidatesTokenCount,
					TotalTokens:      gr.UsageMetadata.TotalTokenCount,
				}
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{
					Provider: providerName,
					Model:    gr.ModelVersion,
					Usage:    usage,
				}:
				}
			}
		}
	}()
	return ch
}

// =============================================================================
// Gemini 兼容 API 辅助函数
// =============================================================================

// ListModelsGeminiCompat 通用的 Gemini 兼容 Provider 模型列表获取函数。
func ListModelsGeminiCompat(ctx context.Context, client *http.Client, baseURL, apiKey, providerName, modelsEndpoint string, buildHeadersFunc func(*http.Request, string)) ([]llm.Model, error) {
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

	var modelsResp struct {
		Models []struct {
			Name             string `json:"name"`
			DisplayName      string `json:"displayName"`
			Description      string `json:"description"`
			InputTokenLimit  int    `json:"inputTokenLimit"`
			OutputTokenLimit int    `json:"outputTokenLimit"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  providerName,
		}
	}

	models := make([]llm.Model, 0, len(modelsResp.Models))
	for _, m := range modelsResp.Models {
		modelID := strings.TrimSpace(strings.TrimPrefix(m.Name, "models/"))
		if modelID == "" {
			modelID = m.Name
		}
		models = append(models, llm.Model{
			ID:              modelID,
			Object:          "model",
			OwnedBy:         providerName,
			MaxInputTokens:  m.InputTokenLimit,
			MaxOutputTokens: m.OutputTokenLimit,
		})
	}
	return models, nil
}

// =============================================================================
// 内部辅助函数
// =============================================================================

func messageFromGeminiCompatCandidate(candidate GeminiCompatCandidate, provider string) types.Message {
	msg := types.Message{Role: llm.RoleAssistant}
	if candidate.Content == nil {
		return msg
	}

	var reasoningParts []string

	for _, part := range candidate.Content.Parts {
		if part.Thought {
			if strings.TrimSpace(part.Text) != "" {
				reasoningParts = append(reasoningParts, part.Text)
			}
			continue
		}
		if part.Text != "" {
			msg.Content += part.Text
		}
		if part.FunctionCall != nil {
			argsJSON, err := json.Marshal(part.FunctionCall.Args)
			if err == nil {
				msg.ToolCalls = append(msg.ToolCalls, NewFunctionToolCall("", part.FunctionCall.Name, argsJSON))
			}
		}
	}

	if len(reasoningParts) > 0 {
		joined := strings.Join(reasoningParts, "\n\n")
		msg.ReasoningContent = &joined
	}

	return msg
}
