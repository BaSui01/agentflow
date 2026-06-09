package evaluation

import (
	"github.com/BaSui01/agentflow/types"
)

func newJudgeChatRequest(model string, messages []types.Message, temperature float32) *types.ChatRequest {
	req := types.NewSimpleChatRequest(model, messages)
	req.Temperature = temperature
	return req
}
