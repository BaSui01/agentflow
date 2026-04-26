package steps

import (
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/workflow/core"
)

var orchestrationPrimaryInputKeys = map[string]struct{}{
	"content": {},
	"input":   {},
	"result":  {},
	"prompt":  {},
}

func buildOrchestrationAgentInput(input core.StepInput, maxRounds int) *agent.Input {
	agentInput := &agent.Input{
		TraceID: input.Metadata["trace_id"],
		Content: orchestrationContent(input.Data),
		Context: map[string]any{},
	}
	if maxRounds > 0 {
		agentInput.Context["max_rounds"] = maxRounds
	}
	for k, v := range input.Data {
		if _, skip := orchestrationPrimaryInputKeys[k]; skip {
			continue
		}
		agentInput.Context[k] = v
	}
	return agentInput
}

func orchestrationContent(data map[string]any) string {
	if data == nil {
		return ""
	}
	for _, key := range []string{"content", "input", "result", "prompt"} {
		if value, ok := data[key].(string); ok {
			return value
		}
	}
	return ""
}
