package bootstrap

import (
	discovery "github.com/BaSui01/agentflow/agent/capabilities/tools"
	agentcheckpoint "github.com/BaSui01/agentflow/agent/persistence/checkpoint"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/rag/core"
	workflowpkg "github.com/BaSui01/agentflow/workflow/core"
)

// StorageSet aggregates storage and registry dependencies.
type StorageSet struct {
	DiscoveryRegistry *discovery.CapabilityRegistry
	AgentRegistry     *agent.AgentRegistry
	Resolver          *agent.CachingResolver

	CheckpointStore         agentcheckpoint.Store
	CheckpointManager       *agent.CheckpointManager
	WorkflowCheckpointStore workflowpkg.CheckpointStore
	RAGStore                core.VectorStore
	RAGEmbedding            core.EmbeddingProvider
}

// HasResolver returns true if a resolver is configured.
func (s *StorageSet) HasResolver() bool {
	return s.Resolver != nil
}
