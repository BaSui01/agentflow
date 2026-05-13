package reasoning

import (
	"github.com/BaSui01/agentflow/types"
)

func newGatewayChatRequest(model string, messages []types.Message, configure func(*types.ChatRequest)) *types.ChatRequest {
	req := types.NewSimpleChatRequest(model, messages)
	if configure != nil {
		configure(req)
	}
	return req
}
