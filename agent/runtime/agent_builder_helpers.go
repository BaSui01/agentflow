package runtime

import (
	"context"
	agentcore "github.com/BaSui01/agentflow/agent/core"
	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
)

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for k, v := range metadata {
		cloned[k] = v
	}
	return cloned
}

func withSkillInstructions(ctx context.Context, instructions []string) context.Context {
	return agentcontext.WithSkillInstructions(ctx, instructions)
}

func skillInstructionsFromCtx(ctx context.Context) []string {
	return agentcontext.SkillInstructionsFromContext(ctx)
}

func withMemoryContext(ctx context.Context, memory []string) context.Context {
	return agentcontext.WithMemoryContext(ctx, memory)
}

func memoryContextFromCtx(ctx context.Context) []string {
	return agentcontext.MemoryContextFromContext(ctx)
}

func shallowCopyInput(in *Input) *Input {
	cp := *in
	if in.Context != nil {
		cp.Context = make(map[string]any, len(in.Context))
		for k, v := range in.Context {
			cp.Context[k] = v
		}
	}
	return &cp
}

func normalizeInstructionList(instructions []string) []string {
	return agentcore.NormalizeInstructionList(instructions)
}

func explainabilityTimelineRecorder(obs ObservabilityRunner) ExplainabilityTimelineRecorder {
	return agentcore.ExplainabilityTimelineRecorderFrom(obs)
}
