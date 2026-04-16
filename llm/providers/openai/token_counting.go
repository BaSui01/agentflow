package openai

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
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
	body.Conversation = req.ConversationID
	params := responses.InputTokenCountParams{
		Model: param.NewOpt(body.Model),
	}
	if body.Instructions != "" {
		params.Instructions = param.NewOpt(body.Instructions)
	}
	switch input := body.Input.(type) {
	case string:
		params.Input = responses.InputTokenCountParamsInputUnion{OfString: param.NewOpt(input)}
	case []any:
		params.Input = responses.InputTokenCountParamsInputUnion{
			OfResponseInputItemArray: decodeSliceSDKParam[responses.ResponseInputItemUnionParam](input),
		}
	}
	if len(body.Tools) > 0 {
		params.Tools = make([]responses.ToolUnionParam, 0, len(body.Tools))
		for _, tool := range body.Tools {
			params.Tools = append(params.Tools, decodeSDKParam[responses.ToolUnionParam](tool))
		}
	}
	if body.ToolChoice != nil {
		params.ToolChoice = decodeSDKParam[responses.InputTokenCountParamsToolChoiceUnion](body.ToolChoice)
	}
	if body.ParallelToolCalls != nil {
		params.ParallelToolCalls = param.NewOpt(*body.ParallelToolCalls)
	}
	if body.PreviousResponseID != "" {
		params.PreviousResponseID = param.NewOpt(body.PreviousResponseID)
	}
	if body.Conversation != "" {
		params.Conversation = responses.InputTokenCountParamsConversationUnion{OfString: param.NewOpt(body.Conversation)}
	}
	if body.Reasoning != nil {
		params.Reasoning = shared.ReasoningParam{
			Effort:  shared.ReasoningEffort(body.Reasoning.Effort),
			Summary: shared.ReasoningSummary(body.Reasoning.Summary),
		}
	}
	if body.Text != nil {
		text := responses.InputTokenCountParamsText{}
		if body.Text.Verbosity != "" {
			text.Verbosity = body.Text.Verbosity
		}
		if body.Text.Format != nil {
			text.Format = decodeSDKParam[responses.ResponseFormatTextConfigUnionParam](body.Text.Format)
		}
		params.Text = text
	}
	if body.Truncation != "" {
		params.Truncation = responses.InputTokenCountParamsTruncation(body.Truncation)
	}

	client := p.sdkClient(ctx)
	tokenResp, err := client.Responses.InputTokens.Count(ctx, params, responseRequestOptions(body)...)
	if err != nil {
		return nil, p.mapSDKError(err)
	}
	return &llm.TokenCountResponse{
		Model:       body.Model,
		InputTokens: int(tokenResp.InputTokens),
		TotalTokens: int(tokenResp.InputTokens),
	}, nil
}
