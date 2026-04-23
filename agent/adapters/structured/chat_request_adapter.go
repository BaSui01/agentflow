package structured

import (
	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

func newStructuredChatRequest(messages []types.Message) *llm.ChatRequest {
	return &llm.ChatRequest{
		Messages: append([]types.Message(nil), messages...),
	}
}
