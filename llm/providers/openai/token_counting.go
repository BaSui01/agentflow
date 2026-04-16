package openai

import (
	"context"
	"strings"

	"github.com/BaSui01/agentflow/llm"
)

type openAIInputTokenCountResponse struct {
	InputTokens int64 `json:"input_tokens"`
}

func (p *OpenAIProvider) CountTokens(ctx context.Context, req *llm.ChatRequest) (*llm.TokenCountResponse, error) {
	if req == nil {
		return nil, nil
	}
	reqCopy := *req
	body := p.buildResponsesRequest(&reqCopy)
	tokenBody := map[string]any{
		"model": body.Model,
		"input": body.Input,
	}
	if body.Instructions != "" {
		tokenBody["instructions"] = body.Instructions
	}
	if len(body.Tools) > 0 {
		tokenBody["tools"] = body.Tools
	}
	if body.ToolChoice != nil {
		tokenBody["tool_choice"] = body.ToolChoice
	}
	if body.ParallelToolCalls != nil {
		tokenBody["parallel_tool_calls"] = *body.ParallelToolCalls
	}
	if body.PreviousResponseID != "" {
		tokenBody["previous_response_id"] = body.PreviousResponseID
	}
	if strings.TrimSpace(req.ConversationID) != "" {
		tokenBody["conversation"] = strings.TrimSpace(req.ConversationID)
	}
	if body.Reasoning != nil {
		tokenBody["reasoning"] = body.Reasoning
	}
	if body.Text != nil {
		tokenBody["text"] = body.Text
	}
	if body.Truncation != "" {
		tokenBody["truncation"] = body.Truncation
	}

	client := p.sdkClient(ctx)
	var tokenResp openAIInputTokenCountResponse
	if err := client.Post(ctx, "/responses/input_tokens", tokenBody, &tokenResp); err != nil {
		return nil, p.mapSDKError(err)
	}
	return &llm.TokenCountResponse{
		Model:       body.Model,
		InputTokens: int(tokenResp.InputTokens),
		TotalTokens: int(tokenResp.InputTokens),
	}, nil
}
