package bootstrap

import (
	"context"
	"fmt"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/workflow/core"
)

type resolverAgentExecutor struct {
	resolver WorkflowAgentResolver
}

func (e resolverAgentExecutor) ResolveAgent(ctx context.Context, agentID string) (agent.Agent, error) {
	if e.resolver == nil {
		return nil, fmt.Errorf("workflow agent resolver is not configured")
	}
	if agentID == "" {
		return nil, fmt.Errorf("workflow agent resolver requires agent id")
	}
	return e.resolver(ctx, agentID)
}

func (e resolverAgentExecutor) Execute(ctx context.Context, input map[string]any) (*core.AgentExecutionOutput, error) {
	if e.resolver == nil {
		return nil, fmt.Errorf("workflow agent resolver is not configured")
	}

	agentID, _ := input["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("workflow agent step requires input.agent_id")
	}
	content, _ := input["content"].(string)

	ag, err := e.ResolveAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}

	out, err := ag.Execute(ctx, &agent.Input{Content: content, Context: input})
	if err != nil {
		return nil, err
	}
	return &core.AgentExecutionOutput{
		Content:      out.Content,
		TokensUsed:   out.TokensUsed,
		Cost:         out.Cost,
		Duration:     out.Duration,
		FinishReason: out.FinishReason,
	}, nil
}
