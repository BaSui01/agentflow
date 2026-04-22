package reasoning

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent/adapters/structured"
	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
)

func generateStructured[T any](ctx context.Context, gateway llmcore.Gateway, req *llm.ChatRequest) (*structured.ParseResult[T], error) {
	so, err := structured.NewStructuredOutput[T](gateway)
	if err != nil {
		return nil, fmt.Errorf("initialize structured output: %w", err)
	}
	result, err := so.GenerateWithRequestAndParse(ctx, req)
	if err != nil {
		return nil, err
	}
	if !result.IsValid() || result.Value == nil {
		return nil, fmt.Errorf("invalid structured output: %v", result.Errors)
	}
	return result, nil
}

func structuredTokens[T any](result *structured.ParseResult[T]) int {
	if result == nil || result.Usage == nil {
		return 0
	}
	return result.Usage.TotalTokens
}
