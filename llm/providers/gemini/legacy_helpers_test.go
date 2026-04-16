package gemini

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
	"github.com/BaSui01/agentflow/types"
)

// convertToGeminiContents 将统一格式转换为 Gemini 格式
func convertToGeminiContents(msgs []types.Message) (*geminiContent, []geminiContent) {
	var systemInstruction *geminiContent
	var contents []geminiContent

	for _, m := range msgs {
		if m.Role == llm.RoleSystem {
			systemInstruction = &geminiContent{Parts: []geminiPart{{Text: m.Content}}}
			continue
		}
		role := "user"
		if m.Role == llm.RoleAssistant {
			role = "model"
		}
		content := geminiContent{Role: role}
		partIndex := 0
		if m.Role == llm.RoleAssistant && m.ReasoningContent != nil && strings.TrimSpace(*m.ReasoningContent) != "" {
			thoughtPart := geminiPart{Text: *m.ReasoningContent, Thought: boolPtr(true)}
			if sig := geminiThoughtSignatureByIndex(m, partIndex); sig != "" {
				thoughtPart.ThoughtSignature = sig
			}
			content.Parts = append(content.Parts, thoughtPart)
			partIndex++
		}
		if m.Content != "" && m.Role != llm.RoleTool {
			textPart := geminiPart{Text: m.Content}
			if sig := geminiThoughtSignatureByIndex(m, partIndex); sig != "" {
				textPart.ThoughtSignature = sig
			}
			content.Parts = append(content.Parts, textPart)
			partIndex++
		}
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal(tc.Arguments, &args); err == nil {
					callPart := geminiPart{FunctionCall: &geminiFunctionCall{Name: tc.Name, Args: args}}
					if sig := geminiThoughtSignatureByIndex(m, partIndex); sig != "" {
						callPart.ThoughtSignature = sig
					}
					content.Parts = append(content.Parts, callPart)
					partIndex++
				}
			}
		}
		if m.Role == llm.RoleTool && m.ToolCallID != "" {
			var response map[string]any
			if err := json.Unmarshal([]byte(m.Content), &response); err == nil {
				content.Parts = append(content.Parts, geminiPart{FunctionResponse: &geminiFunctionResponse{Name: m.Name, Response: response}})
			} else {
				content.Parts = append(content.Parts, geminiPart{FunctionResponse: &geminiFunctionResponse{Name: m.Name, Response: map[string]any{"result": m.Content}}})
			}
		}
		if len(content.Parts) > 0 {
			contents = append(contents, content)
		}
	}
	return systemInstruction, contents
}

func convertToGeminiTools(tools []types.ToolSchema, wsOpts *llm.WebSearchOptions) []geminiTool {
	needGoogleSearch := wsOpts != nil
	declarations := make([]geminiFunctionDeclaration, 0, len(tools))
	for _, t := range tools {
		if t.Name == "web_search" || t.Name == "google_search" {
			needGoogleSearch = true
			continue
		}
		var params map[string]any
		if err := json.Unmarshal(t.Parameters, &params); err == nil {
			declarations = append(declarations, geminiFunctionDeclaration{Name: t.Name, Description: t.Description, Parameters: params})
		}
	}
	var result []geminiTool
	if len(declarations) > 0 {
		result = append(result, geminiTool{FunctionDeclarations: declarations})
	}
	if needGoogleSearch {
		result = append(result, geminiTool{GoogleSearch: &geminiGoogleSearch{}})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func appendGeminiThoughtPart(msg *types.Message, part geminiPart, partIndex int, provider string) {
	if msg == nil {
		return
	}
	if strings.TrimSpace(part.Text) != "" {
		if msg.ReasoningContent == nil || strings.TrimSpace(*msg.ReasoningContent) == "" {
			msg.ReasoningContent = strPtr(part.Text)
		} else {
			joined := strings.TrimSpace(*msg.ReasoningContent + "\n\n" + part.Text)
			msg.ReasoningContent = strPtr(joined)
		}
		msg.ReasoningSummaries = append(msg.ReasoningSummaries, types.ReasoningSummary{Provider: provider, Kind: "thought_summary", Text: part.Text, ID: fmt.Sprintf("part_%d", partIndex)})
	}
	if strings.TrimSpace(part.ThoughtSignature) != "" {
		msg.OpaqueReasoning = append(msg.OpaqueReasoning, types.OpaqueReasoning{Provider: provider, Kind: "thought_signature", State: part.ThoughtSignature, PartIndex: partIndex, ID: fmt.Sprintf("part_%d", partIndex)})
	}
}

func convertUsageMetadata(m *geminiUsageMetadata) *llm.ChatUsage {
	usage := &llm.ChatUsage{PromptTokens: m.PromptTokenCount, CompletionTokens: m.CandidatesTokenCount, TotalTokens: m.TotalTokenCount}
	if m.ThoughtsTokenCount > 0 {
		usage.CompletionTokensDetails = &llm.CompletionTokensDetails{ReasoningTokens: m.ThoughtsTokenCount}
	}
	return usage
}

func checkPromptFeedback(resp geminiResponse, provider string) error {
	if resp.PromptFeedback == nil || resp.PromptFeedback.BlockReason == "" {
		return nil
	}
	msg := fmt.Sprintf("request blocked by safety filter: %s", resp.PromptFeedback.BlockReason)
	if resp.PromptFeedback.BlockMessage != "" {
		msg = fmt.Sprintf("%s — %s", msg, resp.PromptFeedback.BlockMessage)
	}
	return &types.Error{Code: llm.ErrContentFiltered, Message: msg, HTTPStatus: http.StatusBadRequest, Provider: provider}
}

func convertToolChoice(toolChoice any, includeServerSide *bool) *geminiToolConfig {
	var cfg *geminiToolConfig
	spec := providerbase.NormalizeToolChoice(toolChoice)
	if _, ok := toolChoice.(string); ok && spec.Mode == "tool" {
		return nil
	}
	switch spec.Mode {
	case "auto":
		cfg = &geminiToolConfig{FunctionCallingConfig: &geminiFunctionCallingConfig{Mode: "AUTO"}}
	case "any":
		cfg = &geminiToolConfig{FunctionCallingConfig: &geminiFunctionCallingConfig{Mode: "ANY", AllowedFunctionNames: spec.AllowedFunctionNames}}
	case "none":
		cfg = &geminiToolConfig{FunctionCallingConfig: &geminiFunctionCallingConfig{Mode: "NONE"}}
	case "validated":
		cfg = &geminiToolConfig{FunctionCallingConfig: &geminiFunctionCallingConfig{Mode: "VALIDATED", AllowedFunctionNames: spec.AllowedFunctionNames}}
	case "tool":
		cfg = &geminiToolConfig{FunctionCallingConfig: &geminiFunctionCallingConfig{Mode: "ANY", AllowedFunctionNames: providerbase.NormalizeUniqueStrings([]string{spec.SpecificName})}}
	}
	if spec.IncludeServerSideToolUse != nil {
		includeServerSide = spec.IncludeServerSideToolUse
	}
	if cfg != nil && cfg.FunctionCallingConfig != nil {
		switch cfg.FunctionCallingConfig.Mode {
		case "AUTO", "ANY", "NONE", "VALIDATED":
		default:
			cfg.FunctionCallingConfig.Mode = "AUTO"
		}
	}
	if includeServerSide != nil {
		if cfg == nil {
			cfg = &geminiToolConfig{}
		}
		cfg.IncludeServerSideToolInvocations = includeServerSide
	}
	return cfg
}

func buildGenerationConfig(req *llm.ChatRequest) *geminiGenerationConfig {
	cfg := &geminiGenerationConfig{Temperature: req.Temperature, TopP: req.TopP, MaxOutputTokens: req.MaxTokens, StopSequences: req.Stop}
	if req.ResponseFormat != nil {
		switch req.ResponseFormat.Type {
		case llm.ResponseFormatJSONObject:
			cfg.ResponseMimeType = "application/json"
		case llm.ResponseFormatJSONSchema:
			cfg.ResponseMimeType = "application/json"
			if req.ResponseFormat.JSONSchema != nil {
				cfg.ResponseSchema = req.ResponseFormat.JSONSchema.Schema
			}
		}
	}
	if req.ReasoningMode != "" {
		cfg.ThinkingConfig = &geminiThinkingConfig{IncludeThoughts: true}
		switch req.ReasoningMode {
		case "minimal", "low", "medium", "high":
			cfg.ThinkingConfig.ThinkingLevel = req.ReasoningMode
		default:
			cfg.ThinkingConfig.ThinkingLevel = "medium"
		}
	}
	if cfg.Temperature == 0 && cfg.TopP == 0 && cfg.MaxOutputTokens == 0 && len(cfg.StopSequences) == 0 && cfg.ResponseMimeType == "" && cfg.ThinkingConfig == nil {
		return nil
	}
	return cfg
}

func convertSafetySettings(settings []providers.GeminiSafetySetting) []geminiSafetySetting {
	if len(settings) == 0 {
		return nil
	}
	out := make([]geminiSafetySetting, len(settings))
	for i, s := range settings {
		out[i] = geminiSafetySetting{Category: s.Category, Threshold: s.Threshold}
	}
	return out
}

func extractGroundingAnnotations(gm *geminiGroundingMetadata) []types.Annotation {
	if gm == nil {
		return nil
	}
	var annotations []types.Annotation
	if len(gm.GroundingSupports) > 0 {
		for _, support := range gm.GroundingSupports {
			for _, idx := range support.GroundingChunkIndices {
				if idx < 0 || idx >= len(gm.GroundingChunks) {
					continue
				}
				chunk := gm.GroundingChunks[idx]
				if chunk.Web == nil {
					continue
				}
				ann := types.Annotation{Type: "url_citation", URL: chunk.Web.URI, Title: chunk.Web.Title}
				if support.Segment != nil {
					ann.StartIndex = support.Segment.StartIndex
					ann.EndIndex = support.Segment.EndIndex
				}
				annotations = append(annotations, ann)
			}
		}
	} else if len(gm.GroundingChunks) > 0 {
		for _, chunk := range gm.GroundingChunks {
			if chunk.Web == nil {
				continue
			}
			annotations = append(annotations, types.Annotation{Type: "url_citation", URL: chunk.Web.URI, Title: chunk.Web.Title})
		}
	}
	return annotations
}
