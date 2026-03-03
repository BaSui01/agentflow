package bootstrap

import (
	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/api/handlers"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BuildAgentRegistries creates discovery and agent registries for runtime handlers.
func BuildAgentRegistries(logger *zap.Logger) (*discovery.CapabilityRegistry, *agent.AgentRegistry) {
	return discovery.NewCapabilityRegistry(nil, logger), agent.NewAgentRegistry(logger)
}

// BuildAgentHandler creates agent handler with optional resolver.
func BuildAgentHandler(
	discoveryRegistry discovery.Registry,
	agentRegistry *agent.AgentRegistry,
	logger *zap.Logger,
	resolver ...handlers.AgentResolver,
) *handlers.AgentHandler {
	return handlers.NewAgentHandler(discoveryRegistry, agentRegistry, logger, resolver...)
}

// BuildAPIKeyHandler creates API key handler when DB is available; otherwise returns nil.
func BuildAPIKeyHandler(db *gorm.DB, logger *zap.Logger) *handlers.APIKeyHandler {
	if db == nil {
		return nil
	}
	return handlers.NewAPIKeyHandler(handlers.NewGormAPIKeyStore(db), logger)
}

// BuildToolRegistryHandler creates DB-backed tool registration handler when runtime is available.
func BuildToolRegistryHandler(
	db *gorm.DB,
	runtime handlers.ToolRegistryRuntime,
	logger *zap.Logger,
) *handlers.ToolRegistryHandler {
	if db == nil || runtime == nil {
		return nil
	}
	return handlers.NewToolRegistryHandler(handlers.NewGormToolRegistryStore(db), runtime, logger)
}
