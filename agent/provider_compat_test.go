package agent

import (
	"github.com/BaSui01/agentflow/llm"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
)

func (b *AgentBuilder) WithProvider(provider llm.Provider) *AgentBuilder {
	if provider == nil {
		b.errors = append(b.errors, errNilProviderForTests())
		return b
	}
	return b.WithGateway(llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Ledger:       b.ledger,
		Logger:       b.logger,
	}))
}

func (b *AgentBuilder) WithToolProvider(provider llm.Provider) *AgentBuilder {
	if provider == nil {
		b.toolGateway = nil
		return b
	}
	return b.WithToolGateway(llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Ledger:       b.ledger,
		Logger:       b.logger,
	}))
}

func (b *BaseAgent) Provider() llm.Provider {
	return compatProviderFromGateway(b.MainGateway())
}

func (b *BaseAgent) ToolProvider() llm.Provider {
	if !b.hasDedicatedToolExecutionSurface() {
		return nil
	}
	return compatProviderFromGateway(b.ToolGateway())
}

func (b *BaseAgent) SetToolProvider(provider llm.Provider) {
	if provider == nil {
		b.SetToolGateway(nil)
		return
	}
	b.SetToolGateway(llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Ledger:       b.ledger,
		Logger:       b.logger,
	}))
}

func errNilProviderForTests() error {
	return ErrProviderNotSet
}
