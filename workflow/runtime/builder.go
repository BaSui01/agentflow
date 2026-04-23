package runtime

import (
	workflow "github.com/BaSui01/agentflow/workflow/core"
	"github.com/BaSui01/agentflow/workflow/dsl"
	"github.com/BaSui01/agentflow/workflow/engine"
	"go.uber.org/zap"
)

// Runtime bundles the workflow runtime surface that callers should wire once
// and then reuse across DAG execution and optional DSL parsing.
type Runtime struct {
	Executor *workflow.DAGExecutor
	Facade   *workflow.Facade
	Parser   *dsl.Parser
}

// Builder is the single workflow runtime assembly entrypoint.
type Builder struct {
	checkpointMgr         workflow.CheckpointManager
	logger                *zap.Logger
	historyStore          *workflow.ExecutionHistoryStore
	circuitBreakerConfig  *workflow.CircuitBreakerConfig
	circuitBreakerHandler workflow.CircuitBreakerEventHandler
	stepDeps              engine.StepDependencies
	enableDSLParser       bool
}

// NewBuilder creates a workflow runtime builder with the default surface:
// DAG executor + facade + DSL parser.
func NewBuilder(checkpointMgr workflow.CheckpointManager, logger *zap.Logger) *Builder {
	return &Builder{
		checkpointMgr:   checkpointMgr,
		logger:          logger,
		enableDSLParser: true,
	}
}

// WithHistoryStore overrides the executor history store.
func (b *Builder) WithHistoryStore(store *workflow.ExecutionHistoryStore) *Builder {
	b.historyStore = store
	return b
}

// WithCircuitBreaker configures circuit breaker behavior for the executor.
func (b *Builder) WithCircuitBreaker(
	config workflow.CircuitBreakerConfig,
	handler workflow.CircuitBreakerEventHandler,
) *Builder {
	b.circuitBreakerConfig = &config
	b.circuitBreakerHandler = handler
	return b
}

// WithStepDependencies shares engine-backed step dependencies with the DSL parser.
func (b *Builder) WithStepDependencies(deps engine.StepDependencies) *Builder {
	b.stepDeps = deps
	return b
}

// WithDSLParser toggles whether Build should provision a DSL parser.
func (b *Builder) WithDSLParser(enabled bool) *Builder {
	b.enableDSLParser = enabled
	return b
}

// Build assembles the workflow runtime surface once.
func (b *Builder) Build() *Runtime {
	executor := workflow.NewDAGExecutor(b.checkpointMgr, b.logger)
	if b.historyStore != nil {
		executor.SetHistoryStore(b.historyStore)
	}
	if b.circuitBreakerConfig != nil {
		executor.SetCircuitBreakerConfig(*b.circuitBreakerConfig, b.circuitBreakerHandler)
	}

	rt := &Runtime{
		Executor: executor,
		Facade:   workflow.NewFacade(executor),
	}
	if !b.enableDSLParser {
		return rt
	}

	parser := dsl.NewParser()
	parser.WithStepDependencies(b.stepDeps)
	rt.Parser = parser
	return rt
}
