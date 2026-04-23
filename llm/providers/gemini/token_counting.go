package gemini

import (
	"context"
	"strings"

	llm "github.com/BaSui01/agentflow/llm/core"
	"google.golang.org/genai"
)

func (p *GeminiProvider) CountTokens(ctx context.Context, req *llm.ChatRequest) (*llm.TokenCountResponse, error) {
	if req == nil {
		return nil, nil
	}
	client, err := p.sdkClient(ctx)
	if err != nil {
		return nil, p.mapSDKError(err)
	}
	model := req.Model
	if strings.TrimSpace(model) == "" {
		model = p.cfg.Model
	}
	systemInstruction, contents := convertToGenAIContents(req.Messages)
	cfg := &genai.CountTokensConfig{
		Tools: convertToGenAITools(req.Tools, req.WebSearchOptions),
	}
	if p.isVertexAI() {
		cfg.SystemInstruction = systemInstruction
	}
	resp, err := client.Models.CountTokens(ctx, model, contents, cfg)
	if err != nil {
		return nil, p.mapSDKError(err)
	}
	return &llm.TokenCountResponse{
		Model:                model,
		InputTokens:          int(resp.TotalTokens),
		TotalTokens:          int(resp.TotalTokens),
		CacheReadInputTokens: int(resp.CachedContentTokenCount),
	}, nil
}
