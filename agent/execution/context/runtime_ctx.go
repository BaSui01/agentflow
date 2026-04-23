package context

import "context"

type runtimeContextKey int

const (
	runtimeContextKeySkillInstructions runtimeContextKey = iota
	runtimeContextKeyMemoryContext
)

func WithSkillInstructions(ctx context.Context, instructions []string) context.Context {
	return context.WithValue(ctx, runtimeContextKeySkillInstructions, instructions)
}

func SkillInstructionsFromContext(ctx context.Context) []string {
	value, _ := ctx.Value(runtimeContextKeySkillInstructions).([]string)
	return value
}

func WithMemoryContext(ctx context.Context, memCtx []string) context.Context {
	return context.WithValue(ctx, runtimeContextKeyMemoryContext, memCtx)
}

func MemoryContextFromContext(ctx context.Context) []string {
	value, _ := ctx.Value(runtimeContextKeyMemoryContext).([]string)
	return value
}
