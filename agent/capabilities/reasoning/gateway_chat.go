package reasoning

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
)

func invokeChatGateway(ctx context.Context, gateway llmcore.Gateway, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if gateway == nil {
		return nil, fmt.Errorf("reasoning gateway is not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("chat request is required")
	}

	resp, err := gateway.Invoke(ctx, &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    req,
	})
	if err != nil {
		return nil, err
	}
	return unwrapUnifiedChatResponse(resp)
}

func unwrapUnifiedChatResponse(resp *llmcore.UnifiedResponse) (*llm.ChatResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("gateway response is nil")
	}

	chatResp, ok := resp.Output.(*llm.ChatResponse)
	if !ok || chatResp == nil {
		return nil, fmt.Errorf("invalid gateway chat response output type %T", resp.Output)
	}
	return chatResp, nil
}
