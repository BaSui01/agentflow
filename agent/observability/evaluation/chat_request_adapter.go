package evaluation

import (
	"strings"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

func newJudgeChatRequest(model string, messages []types.Message, temperature float32) *llm.ChatRequest {
	return &llm.ChatRequest{
		Model:       strings.TrimSpace(model),
		Messages:    append([]types.Message(nil), messages...),
		Temperature: temperature,
	}
}
