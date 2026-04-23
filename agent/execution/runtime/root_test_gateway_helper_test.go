package runtime

import (
	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"go.uber.org/zap"
)

func testGatewayFromProvider(provider llm.Provider) llmcore.Gateway {
	if provider == nil {
		return nil
	}
	return llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Logger:       zap.NewNop(),
	})
}

func testMainProvider(agent *BaseAgent) llm.Provider {
	if agent == nil {
		return nil
	}
	return compatProviderFromGateway(agent.MainGateway())
}

func testToolProvider(agent *BaseAgent) llm.Provider {
	if agent == nil || !agent.hasDedicatedToolExecutionSurface() {
		return nil
	}
	return compatProviderFromGateway(agent.ToolGateway())
}

func setTestToolProvider(agent *BaseAgent, provider llm.Provider) {
	if agent == nil {
		return
	}
	agent.SetToolGateway(testGatewayFromProvider(provider))
}


