package main

import (
	"github.com/BaSui01/agentflow/internal/app/bootstrap"
)

func (s *Server) initHandlers() error {
	set, err := bootstrap.BuildServeHandlerSet(bootstrap.ServeHandlerSetBuildInput{
		Cfg:                 s.cfg,
		DB:                  s.infra.db,
		MongoClient:         s.infra.mongoClient,
		Logger:              s.logger,
		ToolApprovalManager: s.currentToolApprovalManager(),
		WorkflowHITLManager: s.currentWorkflowHITLManager(),
		WireMongoStores:     s.wireMongoStores,
	})
	if err != nil {
		return err
	}

	s.handlers.healthHandler = set.HealthHandler
	s.handlers.chatHandler = set.ChatHandler
	s.text.chatService = set.ChatService
	s.handlers.agentHandler = set.AgentHandler
	s.handlers.apiKeyHandler = set.APIKeyHandler
	s.handlers.toolRegistryHandler = set.ToolRegistryHandler
	s.handlers.toolProviderHandler = set.ToolProviderHandler
	s.handlers.toolApprovalHandler = set.ToolApprovalHandler
	s.handlers.authAuditHandler = set.AuthAuditHandler
	s.handlers.ragHandler = set.RAGHandler
	s.handlers.workflowHandler = set.WorkflowHandler
	s.handlers.protocolHandler = set.ProtocolHandler
	s.handlers.multimodalHandler = set.MultimodalHandler
	s.handlers.costHandler = set.CostHandler

	s.infra.multimodalRedis = set.MultimodalRedis
	s.infra.toolApprovalRedis = set.ToolApprovalRedis

	s.text.provider = set.Provider
	s.text.toolProvider = set.ToolProvider
	s.text.budgetManager = set.BudgetManager
	s.text.costTracker = set.CostTracker
	s.text.llmCache = set.LLMCache
	s.text.llmMetrics = set.LLMMetrics

	s.tooling.discoveryRegistry = set.DiscoveryRegistry
	s.tooling.agentRegistry = set.AgentRegistry
	s.tooling.toolingRuntime = set.ToolingRuntime
	s.tooling.capabilityCatalog = set.CapabilityCatalog
	s.workflow.resolver = set.Resolver

	s.workflow.checkpointStore = set.CheckpointStore
	s.workflow.checkpointManager = set.CheckpointManager
	s.workflow.workflowCheckpointStore = set.WorkflowCheckpointStore
	s.workflow.ragStore = set.RAGStore
	s.workflow.ragEmbedding = set.RAGEmbedding

	return nil
}
