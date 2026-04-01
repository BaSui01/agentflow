package bootstrap

import (
	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/agent/hitl"
	"github.com/BaSui01/agentflow/agent/hosted"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/internal/usecase"
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
	resolver ...usecase.AgentResolver,
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
	return handlers.NewToolRegistryHandler(hosted.NewGormToolRegistryStore(db), runtime, logger)
}

// BuildToolProviderHandler creates DB-backed tool provider config handler when runtime is available.
func BuildToolProviderHandler(
	db *gorm.DB,
	runtime handlers.ToolRegistryRuntime,
	logger *zap.Logger,
) *handlers.ToolProviderHandler {
	if db == nil || runtime == nil {
		return nil
	}
	return handlers.NewToolProviderHandler(handlers.NewGormToolProviderStore(db), runtime, logger)
}

// BuildToolApprovalHandler creates the tool approval handler when runtime is available.
func BuildToolApprovalHandler(
	manager *hitl.InterruptManager,
	workflowID string,
	config ToolApprovalConfig,
	logger *zap.Logger,
) *handlers.ToolApprovalHandler {
	if manager == nil {
		return nil
	}
	return handlers.NewToolApprovalHandler(&toolApprovalRuntime{
		manager: manager,
		store:   defaultToolApprovalGrantStore(config, logger),
		history: defaultToolApprovalHistoryStore(config),
		config:  config,
	}, workflowID, logger)
}
