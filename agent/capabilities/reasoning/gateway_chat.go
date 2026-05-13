package reasoning

import (
	"context"
	"fmt"

	llmcore "github.com/BaSui01/agentflow/llm/core"
)

func invokeChatGateway(ctx context.Context, gateway llmcore.Gateway, req *llmcore.ChatRequest) (*llmcore.ChatResponse, error) {
	if gateway == nil {
		return nil, fmt.Errorf("reasoning gateway is not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("chat request is required")
	}
	return llmcore.InvokeChat(ctx, gateway, req)
}
