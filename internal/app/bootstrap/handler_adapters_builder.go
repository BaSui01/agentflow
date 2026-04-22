package bootstrap

import (
	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/capabilities/tools"
	"github.com/BaSui01/agentflow/internal/usecase"
	"go.uber.org/zap"
)

// BuildAgentRegistries creates discovery and agent registries for runtime handlers.
func BuildAgentRegistries(logger *zap.Logger) (*discovery.CapabilityRegistry, *agent.AgentRegistry) {
	return discovery.NewCapabilityRegistry(nil, logger), agent.NewAgentRegistry(logger)
}

// BuildAgentService creates the default AgentService for HTTP handler use.
func BuildAgentService(
	discoveryRegistry discovery.Registry,
	resolver usecase.AgentResolver,
) usecase.AgentService {
	return usecase.NewDefaultAgentService(discoveryRegistry, resolver)
}
