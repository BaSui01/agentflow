package bootstrap

import (
	"context"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/internal/usecase"
)

func buildServeWorkflowHandler(set *ServeHandlerSet, in ServeHandlerSetBuildInput, llmRuntime *LLMHandlerRuntime, authorizationService usecase.AuthorizationService) error {
	workflowOpts := WorkflowRuntimeOptions{
		DefaultModel:         in.Cfg.Agent.Model,
		HITLManager:          in.WorkflowHITLManager,
		CheckpointStore:      set.CheckpointStore,
		RetrievalStore:       set.RAGStore,
		EmbeddingProvider:    set.RAGEmbedding,
		AuthorizationService: authorizationService,
	}
	if llmRuntime != nil {
		workflowOpts.LLMGateway = llmRuntime.Gateway
	}
	if set.Resolver != nil {
		workflowOpts.AgentResolver = func(ctx context.Context, agentID string) (agent.Agent, error) {
			return set.Resolver.Resolve(ctx, agentID)
		}
	}
	if in.DB != nil {
		if wfStore, err := BuildWorkflowPostgreSQLCheckpointStore(context.Background(), in.DB); err == nil && wfStore != nil {
			workflowOpts.WorkflowCheckpointStore = wfStore
			set.WorkflowCheckpointStore = wfStore
		}
	}

	workflowRuntime := BuildWorkflowRuntime(in.Logger, workflowOpts)
	set.WorkflowHandler = handlers.NewWorkflowHandler(usecase.NewDefaultWorkflowService(workflowRuntime.Facade, workflowRuntime.Parser), in.Logger)
	in.Logger.Info("Workflow handler initialized")
	return nil
}
