package claude

import (
	"context"
	"encoding/json"

	"github.com/BaSui01/agentflow/llm"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
)

type claudeTokenCountRequest struct {
	Messages     []claudeMessage     `json:"messages"`
	Model        string              `json:"model"`
	System       string              `json:"system,omitempty"`
	Tools        []json.RawMessage   `json:"tools,omitempty"`
	ToolChoice   *claudeToolChoice   `json:"tool_choice,omitempty"`
	Thinking     *claudeThinking     `json:"thinking,omitempty"`
	OutputConfig *claudeOutputConfig `json:"output_config,omitempty"`
	CacheControl *llm.CacheControl   `json:"cache_control,omitempty"`
}

type claudeTokenCountResponse struct {
	InputTokens              int64 `json:"input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens,omitempty"`
}

func (p *ClaudeProvider) CountTokens(ctx context.Context, req *llm.ChatRequest) (*llm.TokenCountResponse, error) {
	if req == nil {
		return nil, nil
	}
	system, messages := convertToClaudeMessages(req.Messages)
	model := providerbase.ChooseModel(req, p.cfg.Model, "claude-opus-4.5-20260105")
	reasoning := buildClaudeReasoningControls(req, model)
	cacheControl, cacheErr := normalizeClaudeCacheControl(req.CacheControl)
	if cacheErr != nil {
		return nil, cacheErr
	}

	body := claudeTokenCountRequest{
		Messages:     messages,
		Model:        model,
		System:       system,
		Tools:        convertToClaudeTools(req.Tools, req.WebSearchOptions),
		ToolChoice:   convertClaudeToolChoice(req.ToolChoice, req.ParallelToolCalls, len(req.Tools) > 0 || req.WebSearchOptions != nil),
		Thinking:     reasoning.Thinking,
		OutputConfig: reasoning.OutputConfig,
		CacheControl: cacheControl,
	}

	params, err := overrideAnthropicCountTokensParams(body)
	if err != nil {
		return nil, p.mapSDKError(err)
	}

	client := p.sdkClient(p.resolveAPIKey(ctx))
	tokenRespSDK, err := client.Messages.CountTokens(ctx, params, p.sdkRequestOptions(reasoning.Speed)...)
	if err != nil {
		return nil, p.mapSDKError(err)
	}
	var tokenResp claudeTokenCountResponse
	if err := decodeAnthropicSDKRawJSON(tokenRespSDK.RawJSON(), &tokenResp); err != nil {
		return nil, p.mapSDKError(err)
	}
	return &llm.TokenCountResponse{
		Model:                    model,
		InputTokens:              int(tokenResp.InputTokens),
		TotalTokens:              int(tokenResp.InputTokens),
		CacheCreationInputTokens: int(tokenResp.CacheCreationInputTokens),
		CacheReadInputTokens:     int(tokenResp.CacheReadInputTokens),
	}, nil
}
