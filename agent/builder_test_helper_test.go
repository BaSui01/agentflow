package agent

import "github.com/BaSui01/agentflow/types"

// NewAgentBuilder is preserved in tests so the builder internals can still be
// validated after the public constructor is removed from production code.
func NewAgentBuilder(config types.AgentConfig) *AgentBuilder {
	return newAgentBuilder(config)
}
