package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
)

func toOpenAICompatChatResponse(resp *api.ChatResponse) openAICompatChatResponse {
	out := openAICompatChatResponse{
		ID:      firstNonEmptyString(resp.ID, fmt.Sprintf("chatcmpl_%d", time.Now().UnixNano())),
		Object:  "chat.completion",
		Created: safeUnix(resp.CreatedAt),
		Model:   resp.Model,
		Choices: make([]openAICompatChatChoice, 0, len(resp.Choices)),
		Usage: openAICompatChatUsage{
			PromptTokens:            resp.Usage.PromptTokens,
			CompletionTokens:        resp.Usage.CompletionTokens,
			TotalTokens:             resp.Usage.TotalTokens,
			PromptTokensDetails:     toOpenAICompatPromptTokenDetails(resp.Usage.PromptTokensDetails),
			CompletionTokensDetails: toOpenAICompatCompletionTokenDetails(resp.Usage.CompletionTokensDetails),
		},
	}

	for _, c := range resp.Choices {
		out.Choices = append(out.Choices, openAICompatChatChoice{
			Index: c.Index,
			Message: openAICompatOutboundMsg{
				Role:             c.Message.Role,
				Content:          c.Message.Content,
				ReasoningContent: c.Message.ReasoningContent,
				Refusal:          c.Message.Refusal,
				Name:             c.Message.Name,
				ToolCallID:       c.Message.ToolCallID,
				ToolCalls:        toOpenAICompatOutboundToolCalls(c.Message.ToolCalls),
				Annotations:      toOpenAICompatAnnotations(c.Message.Annotations),
			},
			FinishReason: c.FinishReason,
		})
	}
	return out
}

func toOpenAICompatChatChunkResponse(chunk *usecase.ChatStreamChunk, created int64, model string) openAICompatChatChunkResponse {
	out := openAICompatChatChunkResponse{
		ID:      firstNonEmptyString(chunk.ID, fmt.Sprintf("chatcmpl_%d", time.Now().UnixNano())),
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   firstNonEmptyString(chunk.Model, model),
		Choices: []openAICompatChatChunkChoice{
			{
				Index: chunk.Index,
				Delta: openAICompatOutboundMsg{
					Role:             chunk.Delta.Role,
					Content:          chunk.Delta.Content,
					ReasoningContent: chunk.Delta.ReasoningContent,
					Refusal:          chunk.Delta.Refusal,
					Name:             chunk.Delta.Name,
					ToolCallID:       chunk.Delta.ToolCallID,
					ToolCalls:        toOpenAICompatOutboundToolCalls(chunk.Delta.ToolCalls),
				},
				FinishReason: nil,
			},
		},
	}
	if strings.TrimSpace(chunk.FinishReason) != "" {
		out.Choices[0].FinishReason = chunk.FinishReason
	}
	return out
}

func toOpenAICompatResponsesResponse(resp *api.ChatResponse) openAICompatResponsesResponse {
	out := openAICompatResponsesResponse{
		ID:        firstNonEmptyString(resp.ID, fmt.Sprintf("resp_%d", time.Now().UnixNano())),
		Object:    "response",
		CreatedAt: safeUnix(resp.CreatedAt),
		Status:    "completed",
		Model:     resp.Model,
		Output:    make([]openAICompatResponsesOutput, 0, len(resp.Choices)),
		Usage: openAICompatResponsesUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
	}

	for i, c := range resp.Choices {
		if reasoningOut := toOpenAICompatResponsesReasoningOutput(c.Message, i); reasoningOut != nil {
			out.Output = append(out.Output, *reasoningOut)
		}
		msgOut := openAICompatResponsesOutput{
			ID:     fmt.Sprintf("msg_%d", i+1),
			Type:   "message",
			Status: "completed",
			Role:   c.Message.Role,
			Content: []openAICompatResponsesContent{
				{Type: "output_text", Text: c.Message.Content},
			},
		}
		out.Output = append(out.Output, msgOut)

		for _, tc := range c.Message.ToolCalls {
			callType := strings.TrimSpace(tc.Type)
			if callType == "" {
				callType = types.ToolTypeFunction
			}
			item := openAICompatResponsesOutput{
				Type:   callType,
				Name:   tc.Name,
				CallID: tc.ID,
			}
			switch callType {
			case types.ToolTypeCustom:
				item.Type = "custom_tool_call"
				item.Input = tc.Input
			default:
				item.Type = "function_call"
				item.Arguments = tc.Arguments
			}
			out.Output = append(out.Output, item)
		}
	}
	return out
}

func toOpenAICompatResponsesReasoningOutput(msg api.Message, index int) *openAICompatResponsesOutput {
	out := &openAICompatResponsesOutput{
		ID:     fmt.Sprintf("rs_%d", index+1),
		Type:   "reasoning",
		Status: "completed",
	}

	for _, summary := range msg.ReasoningSummaries {
		if strings.TrimSpace(summary.Text) == "" {
			continue
		}
		out.Summary = append(out.Summary, openAICompatResponsesContent{
			Type: "summary_text",
			Text: summary.Text,
		})
		if strings.TrimSpace(summary.ID) != "" {
			out.ID = strings.TrimSpace(summary.ID)
		}
	}
	if len(out.Summary) == 0 && msg.ReasoningContent != nil && strings.TrimSpace(*msg.ReasoningContent) != "" {
		out.Content = []openAICompatResponsesContent{{
			Type: "reasoning_text",
			Text: *msg.ReasoningContent,
		}}
	}
	for _, opaque := range msg.OpaqueReasoning {
		if strings.TrimSpace(opaque.Kind) != "encrypted_content" || strings.TrimSpace(opaque.State) == "" {
			continue
		}
		out.EncryptedContent = opaque.State
		if strings.TrimSpace(opaque.ID) != "" {
			out.ID = strings.TrimSpace(opaque.ID)
		}
		break
	}

	if len(out.Summary) == 0 && len(out.Content) == 0 && strings.TrimSpace(out.EncryptedContent) == "" {
		return nil
	}
	return out
}

func toOpenAICompatOutboundToolCalls(calls []types.ToolCall) []openAICompatOutboundToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]openAICompatOutboundToolCall, 0, len(calls))
	for _, c := range calls {
		item := openAICompatOutboundToolCall{
			ID:   c.ID,
			Type: strings.TrimSpace(c.Type),
		}
		if item.Type == "" {
			item.Type = types.ToolTypeFunction
		}
		switch item.Type {
		case types.ToolTypeCustom:
			item.Custom = &struct {
				Name  string `json:"name"`
				Input string `json:"input"`
			}{
				Name:  c.Name,
				Input: c.Input,
			}
		default:
			item.Type = types.ToolTypeFunction
			item.Function = &struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      c.Name,
				Arguments: string(c.Arguments),
			}
		}
		out = append(out, item)
	}
	return out
}

func toOpenAICompatAnnotations(annotations []types.Annotation) []openAICompatAnnotation {
	if len(annotations) == 0 {
		return nil
	}
	out := make([]openAICompatAnnotation, 0, len(annotations))
	for _, a := range annotations {
		ann := openAICompatAnnotation{Type: a.Type}
		if a.URL != "" || a.Title != "" || a.StartIndex != 0 || a.EndIndex != 0 {
			ann.URLCitation = &openAICompatURLCitationDetail{
				StartIndex: a.StartIndex,
				EndIndex:   a.EndIndex,
				URL:        a.URL,
				Title:      a.Title,
			}
		}
		out = append(out, ann)
	}
	return out
}

func toOpenAICompatPromptTokenDetails(d *api.PromptTokensDetails) *openAICompatTokenDetails {
	if d == nil {
		return nil
	}
	return &openAICompatTokenDetails{
		CachedTokens: d.CachedTokens,
		AudioTokens:  d.AudioTokens,
	}
}

func toOpenAICompatCompletionTokenDetails(d *api.CompletionTokensDetails) *openAICompatTokenDetails {
	if d == nil {
		return nil
	}
	return &openAICompatTokenDetails{
		ReasoningTokens:          d.ReasoningTokens,
		AudioTokens:              d.AudioTokens,
		AcceptedPredictionTokens: d.AcceptedPredictionTokens,
		RejectedPredictionTokens: d.RejectedPredictionTokens,
	}
}
