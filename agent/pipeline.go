package agent

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/llm"
)

// PipelineContext carries intermediate state through the pipeline.
type PipelineContext struct {
	Input            *Input
	Messages         []llm.Message
	RestoredMessages []llm.Message
	ContextMessages  []llm.Message
	RunID            string
	ConversationID   string
	Response         *llm.ChatResponse
	OutputContent    string
	StartTime        time.Time
	FinishReason     string
	TokensUsed       int
	Metadata         map[string]any

	// Internal reference — pipeline steps access agent components through this.
	agent *BaseAgent
}

// AgentID returns the ID of the agent executing this pipeline.
// This is a convenience accessor for plugins in sub-packages.
func (pc *PipelineContext) AgentID() string {
	if pc.agent != nil {
		return pc.agent.ID()
	}
	return ""
}

// StepFunc is the call signature for the next step in the pipeline.
type StepFunc func(ctx context.Context, pc *PipelineContext) error

// ExecutionStep is a single step in the execution pipeline.
type ExecutionStep interface {
	Name() string
	Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error
}

// Pipeline manages a chain of execution steps.
type Pipeline struct {
	steps []ExecutionStep
}

// NewPipeline creates a new Pipeline from the given steps.
func NewPipeline(steps ...ExecutionStep) *Pipeline {
	return &Pipeline{steps: steps}
}

// Run executes the pipeline by building a middleware chain from the steps.
func (p *Pipeline) Run(ctx context.Context, pc *PipelineContext) error {
	// Terminal step: no-op.
	var chain StepFunc = func(_ context.Context, _ *PipelineContext) error { return nil }

	// Build the chain in reverse so the first step runs first.
	for i := len(p.steps) - 1; i >= 0; i-- {
		step := p.steps[i]
		next := chain
		chain = func(ctx context.Context, pc *PipelineContext) error {
			return step.Execute(ctx, pc, next)
		}
	}

	return chain(ctx, pc)
}
