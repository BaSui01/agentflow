package orchestration

import (
	"context"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/agent/team"
)

func executeMode(ctx context.Context, mode string, agents []agent.Agent, input *agent.Input) (*agent.Output, error) {
	return team.ExecuteAgents(ctx, mode, agents, cloneTaskInput(input))
}

func cloneTaskInput(in *agent.Input) *agent.Input {
	if in == nil {
		return &agent.Input{}
	}
	out := *in
	if in.Context != nil {
		out.Context = make(map[string]any, len(in.Context))
		for k, v := range in.Context {
			out.Context[k] = v
		}
	}
	if in.Variables != nil {
		out.Variables = make(map[string]string, len(in.Variables))
		for k, v := range in.Variables {
			out.Variables[k] = v
		}
	}
	return &out
}

func resolveSupervisorID(agents []agent.Agent) string {
	for _, ag := range agents {
		if hasSupervisor([]agent.Agent{ag}) {
			return ag.ID()
		}
	}
	if len(agents) == 0 {
		return ""
	}
	return agents[0].ID()
}
