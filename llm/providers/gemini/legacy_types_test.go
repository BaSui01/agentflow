package gemini

import (
	"encoding/json"
	"strings"
)

// Gemini 消息结构
type geminiContent struct {
	Role  string       `json:"role,omitempty"` // user, model
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          *bool                   `json:"thought,omitempty"` // true = thinking content
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	InlineData       *geminiInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

func (p *geminiPart) UnmarshalJSON(data []byte) error {
	var aux struct {
		Text                  string                  `json:"text"`
		Thought               *bool                   `json:"thought"`
		ThoughtSnake          *bool                   `json:"is_thought"`
		ThoughtSignature      string                  `json:"thoughtSignature"`
		ThoughtSignatureSnake string                  `json:"thought_signature"`
		InlineData            *geminiInlineData       `json:"inlineData"`
		InlineDataSnake       *geminiInlineData       `json:"inline_data"`
		FunctionCall          *geminiFunctionCall     `json:"functionCall"`
		FunctionCallSnake     *geminiFunctionCall     `json:"function_call"`
		FunctionResponse      *geminiFunctionResponse `json:"functionResponse"`
		FunctionResponseSnake *geminiFunctionResponse `json:"function_response"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	p.Text = aux.Text
	p.Thought = firstBoolPtr(aux.Thought, aux.ThoughtSnake)
	p.ThoughtSignature = strings.TrimSpace(firstNonEmpty(aux.ThoughtSignature, aux.ThoughtSignatureSnake))
	p.InlineData = firstInlineData(aux.InlineData, aux.InlineDataSnake)
	p.FunctionCall = firstFunctionCall(aux.FunctionCall, aux.FunctionCallSnake)
	p.FunctionResponse = firstFunctionResponse(aux.FunctionResponse, aux.FunctionResponseSnake)
	return nil
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64 encoded
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

func (f *geminiFunctionCall) UnmarshalJSON(data []byte) error {
	var aux struct {
		Name      string         `json:"name"`
		Args      map[string]any `json:"args"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	f.Name = strings.TrimSpace(aux.Name)
	f.Args = aux.Args
	if len(f.Args) == 0 {
		f.Args = aux.Arguments
	}
	return nil
}

type geminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
	GoogleSearch         *geminiGoogleSearch         `json:"google_search,omitempty"` // google_search grounding
}

// geminiGoogleSearch 是 google_search grounding 工具的标记结构体（空对象）
type geminiGoogleSearch struct{}

type geminiFunctionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"` // JSON Schema
}

// Grounding Metadata 结构体

type geminiGroundingMetadata struct {
	WebSearchQueries  []string                 `json:"webSearchQueries,omitempty"`
	SearchEntryPoint  *geminiSearchEntryPoint  `json:"searchEntryPoint,omitempty"`
	GroundingChunks   []geminiGroundingChunk   `json:"groundingChunks,omitempty"`
	GroundingSupports []geminiGroundingSupport `json:"groundingSupports,omitempty"`
}

type geminiSearchEntryPoint struct {
	RenderedContent string `json:"renderedContent,omitempty"`
}

type geminiGroundingChunk struct {
	Web *geminiGroundingChunkWeb `json:"web,omitempty"`
}

type geminiGroundingChunkWeb struct {
	URI   string `json:"uri,omitempty"`
	Title string `json:"title,omitempty"`
}

type geminiGroundingSupport struct {
	Segment               *geminiGroundingSegment `json:"segment,omitempty"`
	GroundingChunkIndices []int                   `json:"groundingChunkIndices,omitempty"`
}

type geminiGroundingSegment struct {
	StartIndex int    `json:"startIndex,omitempty"`
	EndIndex   int    `json:"endIndex,omitempty"`
	Text       string `json:"text,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature      float32               `json:"temperature,omitempty"`
	TopP             float32               `json:"topP,omitempty"`
	TopK             int                   `json:"topK,omitempty"`
	MaxOutputTokens  int                   `json:"maxOutputTokens,omitempty"`
	StopSequences    []string              `json:"stopSequences,omitempty"`
	ResponseMimeType string                `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any        `json:"responseSchema,omitempty"`
	ThinkingConfig   *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

type geminiThinkingConfig struct {
	ThinkingLevel   string `json:"thinkingLevel,omitempty"`   // minimal, low, medium, high
	IncludeThoughts bool   `json:"includeThoughts,omitempty"` // include thought parts in response
}

type geminiToolConfig struct {
	FunctionCallingConfig            *geminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
	IncludeServerSideToolInvocations *bool                        `json:"includeServerSideToolInvocations,omitempty"`
}

type geminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"`                 // AUTO, ANY, NONE, VALIDATED
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"` // restrict callable functions
}

type geminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type geminiRequest struct {
	Contents          []geminiContent         `json:"contents"`
	Tools             []geminiTool            `json:"tools,omitempty"`
	ToolConfig        *geminiToolConfig       `json:"toolConfig,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	SafetySettings    []geminiSafetySetting   `json:"safetySettings,omitempty"`
	CachedContent     string                  `json:"cachedContent,omitempty"`
}

type geminiCandidate struct {
	Content           geminiContent            `json:"content"`
	FinishReason      string                   `json:"finishReason,omitempty"`
	Index             int                      `json:"index"`
	SafetyRatings     []any                    `json:"safetyRatings,omitempty"`
	GroundingMetadata *geminiGroundingMetadata `json:"groundingMetadata,omitempty"`
}

func (c *geminiCandidate) UnmarshalJSON(data []byte) error {
	var aux struct {
		Content                geminiContent            `json:"content"`
		FinishReason           string                   `json:"finishReason"`
		FinishReasonSnake      string                   `json:"finish_reason"`
		Index                  int                      `json:"index"`
		SafetyRatings          []any                    `json:"safetyRatings"`
		SafetyRatingsSnake     []any                    `json:"safety_ratings"`
		GroundingMetadata      *geminiGroundingMetadata `json:"groundingMetadata"`
		GroundingMetadataSnake *geminiGroundingMetadata `json:"grounding_metadata"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	c.Content = aux.Content
	c.FinishReason = strings.TrimSpace(firstNonEmpty(aux.FinishReason, aux.FinishReasonSnake))
	c.Index = aux.Index
	if len(aux.SafetyRatings) > 0 {
		c.SafetyRatings = aux.SafetyRatings
	} else {
		c.SafetyRatings = aux.SafetyRatingsSnake
	}
	c.GroundingMetadata = firstGroundingMetadata(aux.GroundingMetadata, aux.GroundingMetadataSnake)
	return nil
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
	ThoughtsTokenCount   int `json:"thoughtsTokenCount,omitempty"`
}

func (u *geminiUsageMetadata) UnmarshalJSON(data []byte) error {
	var aux struct {
		PromptTokenCount          *int `json:"promptTokenCount"`
		PromptTokenCountSnake     *int `json:"prompt_token_count"`
		PromptTokens              *int `json:"prompt_tokens"`
		CandidatesTokenCount      *int `json:"candidatesTokenCount"`
		CandidatesTokenCountSnake *int `json:"candidates_token_count"`
		CompletionTokens          *int `json:"completion_tokens"`
		OutputTokens              *int `json:"output_tokens"`
		TotalTokenCount           *int `json:"totalTokenCount"`
		TotalTokenCountSnake      *int `json:"total_token_count"`
		TotalTokens               *int `json:"total_tokens"`
		ThoughtsTokenCount        *int `json:"thoughtsTokenCount"`
		ThoughtsTokenCountSnake   *int `json:"thoughts_token_count"`
		ReasoningTokens           *int `json:"reasoning_tokens"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	u.PromptTokenCount = firstInt(aux.PromptTokenCount, aux.PromptTokenCountSnake, aux.PromptTokens)
	u.CandidatesTokenCount = firstInt(
		aux.CandidatesTokenCount,
		aux.CandidatesTokenCountSnake,
		aux.CompletionTokens,
		aux.OutputTokens,
	)
	u.TotalTokenCount = firstInt(aux.TotalTokenCount, aux.TotalTokenCountSnake, aux.TotalTokens)
	u.ThoughtsTokenCount = firstInt(aux.ThoughtsTokenCount, aux.ThoughtsTokenCountSnake, aux.ReasoningTokens)
	return nil
}

type geminiPromptFeedback struct {
	BlockReason  string `json:"blockReason,omitempty"`
	BlockMessage string `json:"blockReasonMessage,omitempty"`
}

func (p *geminiPromptFeedback) UnmarshalJSON(data []byte) error {
	var aux struct {
		BlockReason       string `json:"blockReason"`
		BlockReasonSnake  string `json:"block_reason"`
		BlockMessage      string `json:"blockReasonMessage"`
		BlockMessageSnake string `json:"block_reason_message"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	p.BlockReason = strings.TrimSpace(firstNonEmpty(aux.BlockReason, aux.BlockReasonSnake))
	p.BlockMessage = strings.TrimSpace(firstNonEmpty(aux.BlockMessage, aux.BlockMessageSnake))
	return nil
}

type geminiResponse struct {
	Candidates     []geminiCandidate     `json:"candidates"`
	UsageMetadata  *geminiUsageMetadata  `json:"usageMetadata,omitempty"`
	PromptFeedback *geminiPromptFeedback `json:"promptFeedback,omitempty"`
	ModelVersion   string                `json:"modelVersion,omitempty"`
	ResponseID     string                `json:"responseId,omitempty"`
}

func (r *geminiResponse) UnmarshalJSON(data []byte) error {
	var aux struct {
		Candidates          []geminiCandidate     `json:"candidates"`
		UsageMetadata       *geminiUsageMetadata  `json:"usageMetadata"`
		UsageMetadataSnake  *geminiUsageMetadata  `json:"usage_metadata"`
		Usage               *geminiUsageMetadata  `json:"usage"`
		PromptFeedback      *geminiPromptFeedback `json:"promptFeedback"`
		PromptFeedbackSnake *geminiPromptFeedback `json:"prompt_feedback"`
		ModelVersion        string                `json:"modelVersion"`
		ModelVersionSnake   string                `json:"model_version"`
		ResponseID          string                `json:"responseId"`
		ResponseIDSnake     string                `json:"response_id"`
		ID                  string                `json:"id"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	r.Candidates = aux.Candidates
	r.UsageMetadata = firstUsageMetadata(aux.UsageMetadata, aux.UsageMetadataSnake, aux.Usage)
	r.PromptFeedback = firstPromptFeedback(aux.PromptFeedback, aux.PromptFeedbackSnake)
	r.ModelVersion = strings.TrimSpace(firstNonEmpty(aux.ModelVersion, aux.ModelVersionSnake))
	r.ResponseID = strings.TrimSpace(firstNonEmpty(aux.ResponseID, aux.ResponseIDSnake, aux.ID))
	return nil
}

func boolPtr(v bool) *bool { return &v }

func firstInt(values ...*int) int {
	for _, v := range values {
		if v != nil {
			return *v
		}
	}
	return 0
}

func firstBoolPtr(values ...*bool) *bool {
	for _, v := range values {
		if v != nil {
			out := *v
			return &out
		}
	}
	return nil
}

func firstInlineData(values ...*geminiInlineData) *geminiInlineData {
	for _, v := range values {
		if v != nil {
			out := *v
			return &out
		}
	}
	return nil
}

func firstFunctionCall(values ...*geminiFunctionCall) *geminiFunctionCall {
	for _, v := range values {
		if v != nil {
			out := *v
			return &out
		}
	}
	return nil
}

func firstFunctionResponse(values ...*geminiFunctionResponse) *geminiFunctionResponse {
	for _, v := range values {
		if v != nil {
			out := *v
			return &out
		}
	}
	return nil
}

func firstUsageMetadata(values ...*geminiUsageMetadata) *geminiUsageMetadata {
	for _, v := range values {
		if v != nil {
			out := *v
			return &out
		}
	}
	return nil
}

func firstPromptFeedback(values ...*geminiPromptFeedback) *geminiPromptFeedback {
	for _, v := range values {
		if v != nil {
			out := *v
			return &out
		}
	}
	return nil
}

func firstGroundingMetadata(values ...*geminiGroundingMetadata) *geminiGroundingMetadata {
	for _, v := range values {
		if v != nil {
			out := *v
			return &out
		}
	}
	return nil
}
