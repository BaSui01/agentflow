package bootstrap

import (
	"context"

	workflow "github.com/BaSui01/agentflow/workflow/core"
	"github.com/BaSui01/agentflow/workflow/dsl"
	workflowruntime "github.com/BaSui01/agentflow/workflow/runtime"
	"go.uber.org/zap"
)

type WorkflowRuntime struct {
	Facade *workflow.Facade
	Parser *dsl.Parser
}

func BuildWorkflowRuntime(logger *zap.Logger, opts ...WorkflowRuntimeOptions) *WorkflowRuntime {
	var cfg WorkflowRuntimeOptions
	hasOpts := len(opts) > 0
	if len(opts) > 0 {
		cfg = opts[0]
	}

	builder := workflowruntime.NewBuilder(buildWorkflowCheckpointManager(cfg), logger)
	if hasOpts {
		builder = builder.WithStepDependencies(buildStepDependencies(cfg, logger))
	}

	rt := builder.Build()
	rt.Parser.RegisterCondition("always_true", func(ctx context.Context, input any) (bool, error) {
		return true, nil
	})
	return &WorkflowRuntime{
		Facade: rt.Facade,
		Parser: rt.Parser,
	}
}
