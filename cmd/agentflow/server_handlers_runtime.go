package main

import (
	"github.com/BaSui01/agentflow/internal/app/bootstrap"
)

func (s *Server) initHandlers() error {
	set, err := bootstrap.BuildServeHandlerSet(bootstrap.ServeHandlerSetBuildInput{
		Cfg:                 s.cfg,
		DB:                  s.db,
		MongoClient:         s.mongoClient,
		Logger:              s.logger,
		ToolApprovalManager: s.currentToolApprovalManager(),
		WorkflowHITLManager: s.currentWorkflowHITLManager(),
		WireMongoStores:     s.wireMongoStores,
	})
	if err != nil {
		return err
	}

	s.healthHandler = set.HealthHandler
	s.chatHandler = set.ChatHandler
	s.chatService = set.ChatService
	s.agentHandler = set.AgentHandler
	s.apiKeyHandler = set.APIKeyHandler
	s.toolRegistryHandler = set.ToolRegistryHandler
	s.toolProviderHandler = set.ToolProviderHandler
	s.toolApprovalHandler = set.ToolApprovalHandler
	s.ragHandler = set.RAGHandler
	s.workflowHandler = set.WorkflowHandler
	s.protocolHandler = set.ProtocolHandler
	s.multimodalHandler = set.MultimodalHandler
	s.costHandler = set.CostHandler

	s.multimodalRedis = set.MultimodalRedis
	s.toolApprovalRedis = set.ToolApprovalRedis

	s.provider = set.Provider
	s.toolProvider = set.ToolProvider
	s.budgetManager = set.BudgetManager
	s.costTracker = set.CostTracker
	s.llmCache = set.LLMCache
	s.llmMetrics = set.LLMMetrics

	s.discoveryRegistry = set.DiscoveryRegistry
	s.agentRegistry = set.AgentRegistry
	s.toolingRuntime = set.ToolingRuntime
	s.capabilityCatalog = set.CapabilityCatalog
	s.resolver = set.Resolver

	s.checkpointStore = set.CheckpointStore
	s.checkpointManager = set.CheckpointManager
	s.workflowCheckpointStore = set.WorkflowCheckpointStore
	s.ragStore = set.RAGStore
	s.ragEmbedding = set.RAGEmbedding

	return nil
}
