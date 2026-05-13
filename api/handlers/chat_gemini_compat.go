package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/llm/internal/googlegenai"
	"github.com/BaSui01/agentflow/types"
)

// =============================================================================
// Gemini generateContent API compatible types
// =============================================================================

type geminiCompatGenerateRequest struct {
	Contents          []geminiCompatContent         `json:"contents"`
	SystemInstruction *geminiCompatContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiCompatGenerationConfig `json:"generationConfig,omitempty"`
	Tools             []geminiCompatTool            `json:"tools,omitempty"`
	ToolConfig        *geminiCompatToolConfig       `json:"toolConfig,omitempty"`
}

type geminiCompatContent struct {
	Role  string             `json:"role,omitempty"`
	Parts []geminiCompatPart `json:"parts"`
}

type geminiCompatPart struct {
	Text             string                    `json:"text,omitempty"`
	FunctionCall     *geminiCompatFuncCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiCompatFuncResponse `json:"functionResponse,omitempty"`
	InlineData       *geminiCompatInlineData   `json:"inlineData,omitempty"`
}

type geminiCompatFuncCall struct {
	Name string         `json:"name,omitempty"`
	Args map[string]any `json:"args,omitempty"`
}

type geminiCompatFuncResponse struct {
	Name     string         `json:"name,omitempty"`
	Response map[string]any `json:"response,omitempty"`
}

type geminiCompatInlineData struct {
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
}

type geminiCompatTool struct {
	FunctionDeclarations []geminiCompatFuncDecl    `json:"functionDeclarations,omitempty"`
	GoogleSearch         *geminiCompatGoogleSearch `json:"googleSearch,omitempty"`
}

type geminiCompatGoogleSearch struct{}

type geminiCompatFuncDecl struct {
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type geminiCompatToolConfig struct {
	FunctionCallingConfig *geminiCompatFuncCallingConfig `json:"functionCallingConfig,omitempty"`
}

type geminiCompatFuncCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type geminiCompatGenerationConfig struct {
	Temperature      *float32              `json:"temperature,omitempty"`
	TopP             *float32              `json:"topP,omitempty"`
	TopK             *int32                `json:"topK,omitempty"`
	MaxOutputTokens  int32                 `json:"maxOutputTokens,omitempty"`
	StopSequences    []string              `json:"stopSequences,omitempty"`
	ResponseMimeType string                `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any        `json:"responseSchema,omitempty"`
	ThinkingConfig   *geminiCompatThinking `json:"thinkingConfig,omitempty"`
}

type geminiCompatThinking struct {
	IncludeThoughts *bool  `json:"includeThoughts,omitempty"`
	ThinkingBudget  *int32 `json:"thinkingBudget,omitempty"`
	ThinkingLevel   string `json:"thinkingLevel,omitempty"`
}

type geminiCompatGenerateResponse struct {
	Candidates    []geminiCompatCandidate    `json:"candidates"`
	UsageMetadata *geminiCompatUsageMetadata `json:"usageMetadata,omitempty"`
	ModelVersion  string                     `json:"modelVersion,omitempty"`
}

type geminiCompatCandidate struct {
	Content      *geminiCompatContent `json:"content,omitempty"`
	FinishReason string               `json:"finishReason,omitempty"`
	Index        int32                `json:"index,omitempty"`
}

type geminiCompatUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount,omitempty"`
	CandidatesTokenCount int `json:"candidatesTokenCount,omitempty"`
	TotalTokenCount      int `json:"totalTokenCount,omitempty"`
}

// =============================================================================
// HandleGeminiCompatGenerateContent
// =============================================================================

// geminiCompatRoutePattern matches URLs like /v1beta/models/gemini-2.5-flash:generateContent
var geminiCompatRoutePattern = regexp.MustCompile(`^` + googlegenai.GeminiCompatHTTPRoutePath + `(.+):(` + googlegenai.GeminiCompatStreamAction + `|` + googlegenai.GeminiCompatGenerateAction + `)$`)


// HandleGeminiCompatDispatch routes to the correct handler based on the URL suffix.
func (h *ChatHandler) HandleGeminiCompatDispatch(w http.ResponseWriter, r *http.Request) {
	matches := geminiCompatRoutePattern.FindStringSubmatch(r.URL.Path)
	if len(matches) != 3 {
		h.writeGeminiCompatError(w, types.NewError(types.ErrInvalidRequest, "invalid Gemini API path: expect " + googlegenai.GeminiCompatHTTPRoutePath + "{model}:" + googlegenai.GeminiCompatGenerateAction + " or :" + googlegenai.GeminiCompatStreamAction).WithHTTPStatus(http.StatusNotFound))
		return
	}
	switch matches[2] {
	case googlegenai.GeminiCompatStreamAction:
		h.HandleGeminiCompatStreamGenerateContent(w, r)
	case googlegenai.GeminiCompatGenerateAction:
		h.HandleGeminiCompatGenerateContent(w, r)
	default:
		h.writeGeminiCompatError(w, types.NewError(types.ErrInvalidRequest, "unknown Gemini API action").WithHTTPStatus(http.StatusNotFound))
	}
}

func (h *ChatHandler) HandleGeminiCompatGenerateContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeGeminiCompatError(w, types.NewError(types.ErrInvalidRequest, "method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed))
		return
	}

	service, svcErr := h.currentServiceOrUnavailable("chat")
	if svcErr != nil {
		h.writeGeminiCompatError(w, svcErr)
		return
	}

	var req geminiCompatGenerateRequest
	if err := decodeOpenAICompatJSON(w, r, &req); err != nil {
		h.writeGeminiCompatError(w, err)
		return
	}

	apiReq, apiErr := buildAPIChatRequestFromGeminiCompat(req)
	if apiErr != nil {
		h.writeGeminiCompatError(w, apiErr)
		return
	}
	if err := h.validateChatRequest(apiReq); err != nil {
		h.writeGeminiCompatError(w, err)
		return
	}

	result, svcErr := service.Complete(r.Context(), h.converter.ToUsecaseRequest(apiReq))
	if svcErr != nil {
		h.writeGeminiCompatError(w, svcErr)
		return
	}

	out := toGeminiCompatGenerateResponse(h.converter.ToAPIResponseFromUsecase(result.Response))
	h.writeGeminiCompatJSON(w, http.StatusOK, out)
}

// =============================================================================
// HandleGeminiCompatStreamGenerateContent
// =============================================================================

func (h *ChatHandler) HandleGeminiCompatStreamGenerateContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeGeminiCompatError(w, types.NewError(types.ErrInvalidRequest, "method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed))
		return
	}

	service, svcErr := h.currentServiceOrUnavailable("chat")
	if svcErr != nil {
		h.writeGeminiCompatError(w, svcErr)
		return
	}

	var req geminiCompatGenerateRequest
	if err := decodeOpenAICompatJSON(w, r, &req); err != nil {
		h.writeGeminiCompatError(w, err)
		return
	}

	apiReq, apiErr := buildAPIChatRequestFromGeminiCompat(req)
	if apiErr != nil {
		h.writeGeminiCompatError(w, apiErr)
		return
	}
	if err := h.validateChatRequest(apiReq); err != nil {
		h.writeGeminiCompatError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeGeminiCompatError(w, types.NewInternalError("streaming not supported"))
		return
	}

	stream, svcErr := service.Stream(r.Context(), h.converter.ToUsecaseRequest(apiReq))
	if svcErr != nil {
		h.writeGeminiCompatError(w, svcErr)
		return
	}

	model := apiReq.Model

	for item := range stream {
		if item.Err != nil {
			_, payload := geminiCompatErrorEnvelopeFromTypes(item.Err)
			data, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			return
		}
		if item.Chunk == nil {
			continue
		}
		chunk := item.Chunk
		if strings.TrimSpace(chunk.Model) != "" {
			model = chunk.Model
		}

		parts := geminiCompatPartsFromDelta(chunk.Delta)
		candidate := geminiCompatCandidate{
			Index: int32(chunk.Index),
			Content: &geminiCompatContent{
				Role:  "model",
				Parts: parts,
			},
			FinishReason: geminiCompatFinishReason(chunk.FinishReason),
		}

		resp := geminiCompatGenerateResponse{
			ModelVersion: model,
			Candidates:   []geminiCompatCandidate{candidate},
		}
		if chunk.Usage != nil {
			resp.UsageMetadata = &geminiCompatUsageMetadata{
				PromptTokenCount:     chunk.Usage.PromptTokens,
				CandidatesTokenCount: chunk.Usage.CompletionTokens,
				TotalTokenCount:      chunk.Usage.TotalTokens,
			}
		}

		data, _ := json.Marshal(resp)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
}

// =============================================================================
// Request conversion
// =============================================================================

func buildAPIChatRequestFromGeminiCompat(req geminiCompatGenerateRequest) (*api.ChatRequest, *types.Error) {
	messages := make([]api.Message, 0, len(req.Contents)+1)

	if req.SystemInstruction != nil {
		sysText := geminiCompatContentText(*req.SystemInstruction)
		if strings.TrimSpace(sysText) != "" {
			messages = append(messages, api.Message{
				Role:    string(types.RoleSystem),
				Content: sysText,
			})
		}
	}

	for _, c := range req.Contents {
		msgs, err := convertGeminiCompatContent(c)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msgs...)
	}

	tools := make([]api.ToolSchema, 0)
	for _, tool := range req.Tools {
		if len(tool.FunctionDeclarations) > 0 {
			for _, fd := range tool.FunctionDeclarations {
				name := strings.TrimSpace(fd.Name)
				if name == "" {
					continue
				}
				tools = append(tools, api.ToolSchema{
					Type:        types.ToolTypeFunction,
					Name:        name,
					Description: strings.TrimSpace(fd.Description),
					Parameters:  normalizeGeminiCompatJSONValue(fd.Parameters),
				})
			}
		}
		if tool.GoogleSearch != nil {
			tools = append(tools, api.ToolSchema{
				Type: types.ToolTypeFunction,
				Name: "web_search",
			})
		}
	}

	cfg := req.GenerationConfig
	temperature := float32(0)
	maxTokens := 0
	topP := float32(0)
	var stopSequences []string
	if cfg != nil {
		if cfg.Temperature != nil {
			temperature = *cfg.Temperature
		}
		if cfg.MaxOutputTokens > 0 {
			maxTokens = int(cfg.MaxOutputTokens)
		}
		if cfg.TopP != nil {
			topP = *cfg.TopP
		}
		if len(cfg.StopSequences) > 0 {
			stopSequences = cfg.StopSequences
		}
	}

	metadata := make(map[string]string)
	if cfg != nil && cfg.ResponseMimeType != "" {
		metadata["response_mime_type"] = cfg.ResponseMimeType
	}

	var toolChoice any
	if req.ToolConfig != nil && req.ToolConfig.FunctionCallingConfig != nil {
		fcc := req.ToolConfig.FunctionCallingConfig
		switch strings.ToUpper(fcc.Mode) {
		case "AUTO":
			toolChoice = &types.ToolChoice{Mode: types.ToolChoiceModeAuto}
		case "ANY":
			toolChoice = &types.ToolChoice{Mode: types.ToolChoiceModeRequired}
		case "NONE":
			toolChoice = &types.ToolChoice{Mode: types.ToolChoiceModeNone}
		}
	}

	var includeThoughts *bool
	var thinkingBudget *int32
	var thinkingLevel string
	if cfg != nil && cfg.ThinkingConfig != nil {
		thinkCfg := cfg.ThinkingConfig
		if thinkCfg.IncludeThoughts != nil {
			includeThoughts = thinkCfg.IncludeThoughts
		}
		thinkingBudget = thinkCfg.ThinkingBudget
		thinkingLevel = strings.TrimSpace(thinkCfg.ThinkingLevel)
	}

	if len(metadata) == 0 {
		metadata = nil
	}

	return &api.ChatRequest{
		Messages:        messages,
		MaxTokens:       maxTokens,
		Temperature:     temperature,
		TopP:            topP,
		Stop:            stopSequences,
		Tools:           tools,
		ToolChoice:      toolChoice,
		IncludeThoughts: includeThoughts,
		ThinkingBudget:  thinkingBudget,
		ThinkingLevel:   thinkingLevel,
		Metadata:        metadata,
	}, nil
}

func convertGeminiCompatContent(c geminiCompatContent) ([]api.Message, *types.Error) {
	role := strings.ToLower(strings.TrimSpace(c.Role))
	switch role {
	case "user", "model", "function":
	default:
		if role == "" {
			role = "user"
		} else {
			return nil, types.NewInvalidRequestError(fmt.Sprintf("invalid role: %s", role))
		}
	}

	internalRole := role
	if role == "model" {
		internalRole = string(types.RoleAssistant)
	}
	if role == "function" {
		internalRole = string(types.RoleTool)
	}

	var out []api.Message
	var reasoningParts []string

	for _, part := range c.Parts {
		if part.FunctionCall != nil {
			fc := part.FunctionCall
			args := normalizeGeminiCompatJSONValue(fc.Args)
			out = append(out, api.Message{
				Role: string(types.RoleAssistant),
				ToolCalls: []types.ToolCall{{
					Type:      types.ToolTypeFunction,
					Name:      strings.TrimSpace(fc.Name),
					Arguments: args,
				}},
			})
			continue
		}

		if part.FunctionResponse != nil {
			fr := part.FunctionResponse
			content := geminiCompatMarshalToString(fr.Response)
			out = append(out, api.Message{
				Role:    string(types.RoleTool),
				Content: content,
				Name:    strings.TrimSpace(fr.Name),
			})
			continue
		}

		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}

		if role == "model" {
			reasoningParts = append(reasoningParts, text)
		} else {
			out = append(out, api.Message{
				Role:    internalRole,
				Content: text,
			})
		}
	}

	if len(reasoningParts) > 0 {
		joined := strings.Join(reasoningParts, "\n\n")
		if len(out) > 0 && out[len(out)-1].Role == string(types.RoleAssistant) {
			out[len(out)-1].ReasoningContent = &joined
		} else {
			out = append(out, api.Message{
				Role:             string(types.RoleAssistant),
				ReasoningContent: &joined,
			})
		}
	}

	if len(out) == 0 {
		return nil, types.NewInvalidRequestError("content must not be empty")
	}
	return out, nil
}

// =============================================================================
// Response conversion
// =============================================================================

func toGeminiCompatGenerateResponse(resp *api.ChatResponse) geminiCompatGenerateResponse {
	out := geminiCompatGenerateResponse{
		ModelVersion: resp.Model,
	}
	if len(resp.Choices) == 0 {
		return out
	}

	for _, choice := range resp.Choices {
		parts := geminiCompatPartsFromAPIMessage(choice.Message)
		candidate := geminiCompatCandidate{
			Index: int32(choice.Index),
			Content: &geminiCompatContent{
				Role:  "model",
				Parts: parts,
			},
			FinishReason: geminiCompatFinishReason(choice.FinishReason),
		}
		out.Candidates = append(out.Candidates, candidate)
	}

	if resp.Usage.TotalTokens > 0 {
		out.UsageMetadata = &geminiCompatUsageMetadata{
			PromptTokenCount:     resp.Usage.PromptTokens,
			CandidatesTokenCount: resp.Usage.CompletionTokens,
			TotalTokenCount:      resp.Usage.TotalTokens,
		}
	}

	return out
}

func geminiCompatPartsFromDelta(delta usecase.Message) []geminiCompatPart {
	var parts []geminiCompatPart

	if delta.ReasoningContent != nil && strings.TrimSpace(*delta.ReasoningContent) != "" {
		parts = append(parts, geminiCompatPart{
			Text: *delta.ReasoningContent,
		})
	}

	if strings.TrimSpace(delta.Content) != "" {
		parts = append(parts, geminiCompatPart{
			Text: delta.Content,
		})
	}

	for _, tc := range delta.ToolCalls {
		var args map[string]any
		if len(tc.Arguments) > 0 {
			_ = json.Unmarshal(tc.Arguments, &args)
		}
		if args == nil {
			args = map[string]any{}
		}
		parts = append(parts, geminiCompatPart{
			FunctionCall: &geminiCompatFuncCall{
				Name: tc.Name,
				Args: args,
			},
		})
	}

	return parts
}

func geminiCompatPartsFromAPIMessage(msg api.Message) []geminiCompatPart {
	var parts []geminiCompatPart

	if msg.ReasoningContent != nil && strings.TrimSpace(*msg.ReasoningContent) != "" {
		parts = append(parts, geminiCompatPart{
			Text: *msg.ReasoningContent,
		})
	}

	if strings.TrimSpace(msg.Content) != "" {
		parts = append(parts, geminiCompatPart{
			Text: msg.Content,
		})
	}

	for _, tc := range msg.ToolCalls {
		var args map[string]any
		if len(tc.Arguments) > 0 {
			_ = json.Unmarshal(tc.Arguments, &args)
		}
		if args == nil {
			args = map[string]any{}
		}
		parts = append(parts, geminiCompatPart{
			FunctionCall: &geminiCompatFuncCall{
				Name: tc.Name,
				Args: args,
			},
		})
	}

	return parts
}

// =============================================================================
// Helpers
// =============================================================================

func geminiCompatContentText(c geminiCompatContent) string {
	var texts []string
	for _, part := range c.Parts {
		if text := strings.TrimSpace(part.Text); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n\n")
}

func geminiCompatFinishReason(reason string) string {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case "STOP", "END_TURN", "CANCELLED", "CANCELED":
		return "STOP"
	case "MAX_TOKENS", "LENGTH", "INCOMPLETE":
		return "MAX_TOKENS"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT":
		return "SAFETY"
	case "TOOL_CALLS", "FUNCTION_CALL":
		return "STOP"
	case "":
		return ""
	default:
		return "STOP"
	}
}

func normalizeGeminiCompatJSONValue(raw any) json.RawMessage {
	if raw == nil {
		return json.RawMessage(`{}`)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(data)
}

func geminiCompatMarshalToString(v any) string {
	if v == nil {
		return ""
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}

func geminiCompatErrorEnvelopeFromTypes(err *types.Error) (int, map[string]any) {
	if err == nil {
		return http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": "unknown error"}}
	}
	status := err.HTTPStatus
	if status == 0 {
		status = http.StatusInternalServerError
	}
	return status, map[string]any{
		"error": map[string]any{
			"code":    string(err.Code),
			"message": err.Message,
			"status":  status,
		},
	}
}

func (h *ChatHandler) writeGeminiCompatJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *ChatHandler) writeGeminiCompatError(w http.ResponseWriter, err *types.Error) {
	status, payload := geminiCompatErrorEnvelopeFromTypes(err)
	h.writeGeminiCompatJSON(w, status, payload)
}
