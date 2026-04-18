package agent

import (
	"context"
	"strings"

	"github.com/BaSui01/agentflow/agent/reasoning"
	"github.com/BaSui01/agentflow/types"
)

// ReasoningRuntime bridges mode selection, reasoning execution, and reflection
// into a single loop-facing runtime contract.
type ReasoningRuntime interface {
	Select(ctx context.Context, input *Input, state *LoopState) ReasoningSelection
	Execute(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error)
	Reflect(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error)
}

type defaultReasoningRuntime struct {
	registry          *reasoning.PatternRegistry
	reflectionEnabled bool
	options           types.ExecutionOptions
	selector          ReasoningModeSelector
	stepExecutor      LoopStepExecutorFunc
	reflectionStep    LoopReflectionFunc
}

// NewDefaultReasoningRuntime wraps the existing selector / executor / reflection
// callbacks behind the unified ReasoningRuntime interface.
func NewDefaultReasoningRuntime(
	options types.ExecutionOptions,
	registry *reasoning.PatternRegistry,
	reflectionEnabled bool,
	selector ReasoningModeSelector,
	stepExecutor LoopStepExecutorFunc,
	reflectionStep LoopReflectionFunc,
) ReasoningRuntime {
	return &defaultReasoningRuntime{
		registry:          registry,
		reflectionEnabled: reflectionEnabled,
		options:           options,
		selector:          selector,
		stepExecutor:      stepExecutor,
		reflectionStep:    reflectionStep,
	}
}

func (r *defaultReasoningRuntime) Select(ctx context.Context, input *Input, state *LoopState) ReasoningSelection {
	selection := ReasoningSelection{Mode: ReasoningModeReact}
	if r.selector != nil {
		selection = r.selector.Select(ctx, input, state, r.registry, r.reflectionEnabled)
		if strings.TrimSpace(selection.Mode) == "" {
			selection.Mode = ReasoningModeReact
		}
	}
	if r.options.Control.DisablePlanner {
		return normalizePlannerDisabledSelection(selection, r.registry, input, state, r.reflectionEnabled)
	}
	return selection
}

func (r *defaultReasoningRuntime) Execute(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
	if r.stepExecutor == nil {
		return nil, NewError("LOOP_STEP_EXECUTOR_MISSING", "loop step executor is required")
	}
	return r.stepExecutor(ctx, input, state, selection)
}

func (r *defaultReasoningRuntime) Reflect(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error) {
	if r.reflectionStep == nil {
		return nil, nil
	}
	return r.reflectionStep(ctx, input, output, state)
}
