package bootstrap

import (
	"context"
	"fmt"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/workflow/core"
)

// workflowCheckpointManagerAdapter adapts the agent checkpoint manager to the workflow engine's CheckpointManager interface.
type workflowCheckpointManagerAdapter struct {
	manager *agent.CheckpointManager
}

func (a workflowCheckpointManagerAdapter) SaveCheckpoint(ctx context.Context, cp *core.EnhancedCheckpoint) error {
	if a.manager == nil {
		return fmt.Errorf("workflow checkpoint manager is not configured")
	}
	if cp == nil {
		return fmt.Errorf("workflow checkpoint is nil")
	}

	payload := &agent.Checkpoint{
		ID:       cp.ID,
		ThreadID: cp.ThreadID,
		AgentID:  "workflow",
		Version:  cp.Version,
		State:    agent.StateReady,
		Messages: []agent.CheckpointMessage{},
		Metadata: map[string]any{
			"workflow_id":       cp.WorkflowID,
			"node_id":           cp.NodeID,
			"completed_nodes":   cp.CompletedNodes,
			"pending_nodes":     cp.PendingNodes,
			"has_snapshot":      cp.Snapshot != nil,
			"checkpoint_source": "workflow_dag",
		},
		CreatedAt: cp.CreatedAt,
		ParentID:  cp.ParentID,
		ExecutionContext: &agent.ExecutionContext{
			WorkflowID:  cp.WorkflowID,
			CurrentNode: cp.NodeID,
			NodeResults: cp.NodeResults,
			Variables:   cp.Variables,
		},
	}
	return a.manager.SaveCheckpoint(ctx, payload)
}

// checkpointStoreManagerAdapter adapts a workflow checkpoint store directly to the CheckpointManager interface.
type checkpointStoreManagerAdapter struct {
	store core.CheckpointStore
}

func (a checkpointStoreManagerAdapter) SaveCheckpoint(ctx context.Context, cp *core.EnhancedCheckpoint) error {
	return a.store.Save(ctx, cp)
}

// buildWorkflowCheckpointManager creates a workflow checkpoint manager from the given options.
func buildWorkflowCheckpointManager(opts WorkflowRuntimeOptions) core.CheckpointManager {
	if opts.WorkflowCheckpointStore != nil {
		return checkpointStoreManagerAdapter{store: opts.WorkflowCheckpointStore}
	}
	if opts.CheckpointStore == nil {
		return nil
	}
	return workflowCheckpointManagerAdapter{manager: agent.NewCheckpointManagerFromNativeStore(opts.CheckpointStore, nil)}
}
