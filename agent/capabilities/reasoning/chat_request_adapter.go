package reasoning

import (
	"strings"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
)

func newGatewayChatRequest(model string, messages []types.Message, configure func(*llm.ChatRequest)) *llm.ChatRequest {
	req := &llm.ChatRequest{
		Model:    strings.TrimSpace(model),
		Messages: append([]types.Message(nil), messages...),
	}
	if configure != nil {
		configure(req)
	}
	return req
}
