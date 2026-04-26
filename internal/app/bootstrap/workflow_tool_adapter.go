package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/BaSui01/agentflow/agent/integration/hosted"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
)

type hostedToolRegistryAdapter struct {
	registry      *hosted.ToolRegistry
	authorization usecase.AuthorizationService
}

func (a hostedToolRegistryAdapter) ExecuteTool(ctx context.Context, name string, params map[string]any) (any, error) {
	if a.registry == nil {
		return nil, fmt.Errorf("workflow tool registry is not configured")
	}

	payload, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal tool params: %w", err)
	}

	if err := a.authorize(ctx, name, payload, cloneAnyMap(params)); err != nil {
		return nil, err
	}

	raw, err := a.registry.Execute(ctx, name, payload)
	if err != nil {
		return nil, err
	}

	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return string(raw), nil
	}
	return out, nil
}

func (a hostedToolRegistryAdapter) Execute(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	if a.registry == nil {
		return nil, fmt.Errorf("workflow tool registry is not configured")
	}
	if err := a.authorize(ctx, name, args, workflowArgumentsFromRaw(args)); err != nil {
		return nil, err
	}
	return a.registry.Execute(ctx, name, args)
}

func (a hostedToolRegistryAdapter) authorize(ctx context.Context, name string, raw json.RawMessage, args map[string]any) error {
	if a.authorization == nil {
		return nil
	}
	tool, ok := a.registry.Get(name)
	if !ok {
		return fmt.Errorf("tool not found: %s", name)
	}
	resourceKind, riskTier, toolType, hostedRisk := workflowHostedToolAuthorizationShape(tool, name)
	authContext := map[string]any{
		"arguments":        args,
		"args_fingerprint": workflowRawFingerprint(raw),
		"metadata": map[string]string{
			"runtime":          "workflow",
			"hosted_tool_type": toolType,
			"hosted_tool_risk": hostedRisk,
		},
	}
	return authorizeWorkflowStep(ctx, a.authorization, workflowAuthorizationRequest(
		ctx,
		resourceKind,
		name,
		types.ActionExecute,
		riskTier,
		authContext,
	))
}
