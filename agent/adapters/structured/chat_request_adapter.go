package structured

import (
	"github.com/BaSui01/agentflow/types"
)

func newStructuredChatRequest(messages []types.Message) *types.ChatRequest {
	return types.NewSimpleChatRequest("", messages)
}
