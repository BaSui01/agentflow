package usecase

import (
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

func chatUsageFromLLM(u *llmcore.ChatUsage) *ChatUsage {
	if u == nil {
		return nil
	}
	return &ChatUsage{
		PromptTokens:            u.PromptTokens,
		CompletionTokens:        u.CompletionTokens,
		TotalTokens:             u.TotalTokens,
		PromptTokensDetails:     promptTokensDetailsFromLLM(u.PromptTokensDetails),
		CompletionTokensDetails: completionTokensDetailsFromLLM(u.CompletionTokensDetails),
	}
}

func promptTokensDetailsFromLLM(in *llmcore.PromptTokensDetails) *PromptTokensDetails {
	if in == nil {
		return nil
	}
	return &PromptTokensDetails{
		CachedTokens:        in.CachedTokens,
		CacheCreationTokens: in.CacheCreationTokens,
		AudioTokens:         in.AudioTokens,
	}
}

func completionTokensDetailsFromLLM(in *llmcore.CompletionTokensDetails) *CompletionTokensDetails {
	if in == nil {
		return nil
	}
	return &CompletionTokensDetails{
		ReasoningTokens:          in.ReasoningTokens,
		AudioTokens:              in.AudioTokens,
		AcceptedPredictionTokens: in.AcceptedPredictionTokens,
		RejectedPredictionTokens: in.RejectedPredictionTokens,
	}
}

func messageFromTypes(msg types.Message) Message {
	return Message{
		Role:               string(msg.Role),
		Content:            msg.Content,
		ReasoningContent:   msg.ReasoningContent,
		ReasoningSummaries: msg.ReasoningSummaries,
		OpaqueReasoning:    msg.OpaqueReasoning,
		ThinkingBlocks:     msg.ThinkingBlocks,
		Refusal:            msg.Refusal,
		Name:               msg.Name,
		ToolCalls:          msg.ToolCalls,
		ToolCallID:         msg.ToolCallID,
		IsToolError:        msg.IsToolError,
		Images:             imageContentsFromTypes(msg.Images),
		Videos:             msg.Videos,
		Annotations:        msg.Annotations,
		Metadata:           msg.Metadata,
		Timestamp:          msg.Timestamp,
	}
}

func imageContentsFromTypes(in []types.ImageContent) []ImageContent {
	if len(in) == 0 {
		return nil
	}
	out := make([]ImageContent, len(in))
	for i, img := range in {
		out[i] = ImageContent{
			Type: img.Type,
			URL:  img.URL,
			Data: img.Data,
		}
	}
	return out
}
