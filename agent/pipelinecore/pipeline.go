package pipelinecore

import "context"

// StepFunc is the call signature for the next step in the pipeline.
type StepFunc[T any] func(ctx context.Context, state *T) error

// ExecutionStep is a single step in the execution pipeline.
type ExecutionStep[T any] interface {
	Name() string
	Execute(ctx context.Context, state *T, next StepFunc[T]) error
}

// Pipeline manages a chain of execution steps.
type Pipeline[T any] struct {
	steps []ExecutionStep[T]
}

// NewPipeline creates a new Pipeline from the given steps.
func NewPipeline[T any](steps ...ExecutionStep[T]) *Pipeline[T] {
	return &Pipeline[T]{steps: steps}
}

// Run executes the pipeline by building a middleware chain from the steps.
func (p *Pipeline[T]) Run(ctx context.Context, state *T) error {
	var chain StepFunc[T] = func(_ context.Context, _ *T) error { return nil }

	for i := len(p.steps) - 1; i >= 0; i-- {
		step := p.steps[i]
		next := chain
		chain = func(ctx context.Context, state *T) error {
			return step.Execute(ctx, state, next)
		}
	}

	return chain(ctx, state)
}
