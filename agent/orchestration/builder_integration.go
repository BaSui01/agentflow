package orchestration

import (
	"context"

	"github.com/BaSui01/agentflow/agent"
	"go.uber.org/zap"
)

// NewDefaultOrchestrator creates an Orchestrator pre-loaded with all built-in
// pattern adapters (collaboration, crew, hierarchical, handoff).
// This is the recommended entry point for wiring orchestration into the agent
// builder or any top-level API.
func NewDefaultOrchestrator(config OrchestratorConfig, logger *zap.Logger) *Orchestrator {
	o := NewOrchestrator(config, logger)
	o.RegisterPattern(NewCollaborationAdapter("debate", logger))
	o.RegisterPattern(NewCrewAdapter(logger))
	o.RegisterPattern(NewHierarchicalAdapter(logger))
	o.RegisterPattern(NewHandoffAdapter(logger))
	return o
}

// OrchestratorAdapter wraps *Orchestrator to satisfy agent.OrchestratorRunner.
// This bridges the orchestration package types with the agent-layer local
// interface, following the §12 workflow-local interface pattern.
type OrchestratorAdapter struct {
	orchestrator *Orchestrator
}

// NewOrchestratorAdapter creates an adapter that satisfies agent.OrchestratorRunner.
func NewOrchestratorAdapter(o *Orchestrator) *OrchestratorAdapter {
	return &OrchestratorAdapter{orchestrator: o}
}

// Execute converts agent-layer types to orchestration types and delegates.
func (a *OrchestratorAdapter) Execute(ctx context.Context, task *agent.OrchestrationTaskInput) (*agent.OrchestrationTaskOutput, error) {
	ot := &OrchestrationTask{
		ID:          task.ID,
		Description: task.Description,
		Input:       &agent.Input{Content: task.Input},
		Metadata:    task.Metadata,
	}

	result, err := a.orchestrator.Execute(ctx, ot)
	if err != nil {
		return nil, err
	}

	outputContent := ""
	if result.Output != nil {
		outputContent = result.Output.Content
	}

	return &agent.OrchestrationTaskOutput{
		Pattern:   string(result.Pattern),
		Output:    outputContent,
		AgentUsed: result.AgentUsed,
		Duration:  result.Duration,
		Metadata:  result.Metadata,
	}, nil
}

