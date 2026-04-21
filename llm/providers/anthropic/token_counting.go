package claude

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

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
	model := providerbase.ChooseModel(req, p.cfg.Model, defaultClaudeModel)
	thinking, outputConfig, speed := buildClaudeReasoningControls(req, model)
	cacheControl, cacheErr := normalizeClaudeCacheControl(req.CacheControl)
	if cacheErr != nil {
		return nil, cacheErr
	}

	params := anthropicsdk.MessageCountTokensParams{
		Model:    model,
		Messages: messages,
	}
	if len(system) > 0 {
		params.System = anthropicsdk.MessageCountTokensParamsSystemUnion{
			OfTextBlockArray: system,
		}
	}
	tools := convertToClaudeTools(req.Tools, req.WebSearchOptions)
	if len(tools) > 0 {
		countTools := make([]anthropicsdk.MessageCountTokensToolUnionParam, 0, len(tools))
		for _, t := range tools {
			countTools = append(countTools, convertToolUnionToCountTokensToolUnion(t))
		}
		params.Tools = countTools
	}
	tc := convertClaudeToolChoice(req.ToolChoice, req.ParallelToolCalls, len(req.Tools) > 0 || req.WebSearchOptions != nil)
	if tc.OfAuto != nil || tc.OfAny != nil || tc.OfTool != nil || tc.OfNone != nil {
		params.ToolChoice = tc
	}
	if thinking.OfEnabled != nil || thinking.OfAdaptive != nil || thinking.OfDisabled != nil {
		params.Thinking = thinking
	}
	if outputConfig.Effort != "" {
		params.OutputConfig = outputConfig
	}
	if cacheControl != nil {
		params.CacheControl = *cacheControl
	}

	client := p.sdkClient(p.resolveAPIKey(ctx))
	tokenRespSDK, err := client.Messages.CountTokens(ctx, params, p.sdkRequestOptions(speed)...)
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

// convertToolUnionToCountTokensToolUnion 将 MessageNewParams 用的 ToolUnionParam 转换为
// CountTokens 用的 MessageCountTokensToolUnionParam。
func convertToolUnionToCountTokensToolUnion(t anthropicsdk.ToolUnionParam) anthropicsdk.MessageCountTokensToolUnionParam {
	var out anthropicsdk.MessageCountTokensToolUnionParam
	if t.OfTool != nil {
		out.OfTool = t.OfTool
	} else if t.OfBashTool20250124 != nil {
		out.OfBashTool20250124 = t.OfBashTool20250124
	} else if t.OfCodeExecutionTool20250522 != nil {
		out.OfCodeExecutionTool20250522 = t.OfCodeExecutionTool20250522
	} else if t.OfCodeExecutionTool20250825 != nil {
		out.OfCodeExecutionTool20250825 = t.OfCodeExecutionTool20250825
	} else if t.OfCodeExecutionTool20260120 != nil {
		out.OfCodeExecutionTool20260120 = t.OfCodeExecutionTool20260120
	} else if t.OfMemoryTool20250818 != nil {
		out.OfMemoryTool20250818 = t.OfMemoryTool20250818
	} else if t.OfTextEditor20250124 != nil {
		out.OfTextEditor20250124 = t.OfTextEditor20250124
	} else if t.OfTextEditor20250429 != nil {
		out.OfTextEditor20250429 = t.OfTextEditor20250429
	} else if t.OfTextEditor20250728 != nil {
		out.OfTextEditor20250728 = t.OfTextEditor20250728
	} else if t.OfWebSearchTool20250305 != nil {
		out.OfWebSearchTool20250305 = t.OfWebSearchTool20250305
	} else if t.OfWebFetchTool20250910 != nil {
		out.OfWebFetchTool20250910 = t.OfWebFetchTool20250910
	} else if t.OfWebSearchTool20260209 != nil {
		out.OfWebSearchTool20260209 = t.OfWebSearchTool20260209
	} else if t.OfWebFetchTool20260209 != nil {
		out.OfWebFetchTool20260209 = t.OfWebFetchTool20260209
	} else if t.OfWebFetchTool20260309 != nil {
		out.OfWebFetchTool20260309 = t.OfWebFetchTool20260309
	} else if t.OfToolSearchToolBm25_20251119 != nil {
		out.OfToolSearchToolBm25_20251119 = t.OfToolSearchToolBm25_20251119
	} else if t.OfToolSearchToolRegex20251119 != nil {
		out.OfToolSearchToolRegex20251119 = t.OfToolSearchToolRegex20251119
	}
	return out
}
