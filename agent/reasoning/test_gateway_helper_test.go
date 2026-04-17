package reasoning

import (
	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"go.uber.org/zap"
)

func testGateway(provider llm.Provider) llmcore.Gateway {
	if provider == nil {
		return nil
	}
	return llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Logger:       zap.NewNop(),
	})
}
