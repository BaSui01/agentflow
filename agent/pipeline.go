package agent

import (
	"time"

	"github.com/BaSui01/agentflow/agent/pipelinecore"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
)

// PipelineContext carries intermediate state through the pipeline.
type PipelineContext struct {
	Input            *Input
	Messages         []types.Message
	RestoredMessages []types.Message
	ContextMessages  []types.Message
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
type StepFunc = pipelinecore.StepFunc[PipelineContext]

// ExecutionStep is a single step in the execution pipeline.
type ExecutionStep = pipelinecore.ExecutionStep[PipelineContext]

// Pipeline manages a chain of execution steps.
type Pipeline = pipelinecore.Pipeline[PipelineContext]

// NewPipeline creates a new Pipeline from the given steps.
func NewPipeline(steps ...ExecutionStep) *Pipeline {
	return pipelinecore.NewPipeline[PipelineContext](steps...)
}
