package context

import (
	"context"

	"github.com/BaSui01/agentflow/types"
)

// ApplyInputContext injects well-known keys from Input.Context into Go context.
// Unknown keys are ignored.
func ApplyInputContext(ctx context.Context, inputCtx map[string]any) context.Context {
	if len(inputCtx) == 0 {
		return ctx
	}
	for k, v := range inputCtx {
		switch k {
		case "trace_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithTraceID(ctx, s)
			}
		case "tenant_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithTenantID(ctx, s)
			}
		case "user_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithUserID(ctx, s)
			}
		case "run_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithRunID(ctx, s)
			}
		case "parent_run_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithParentRunID(ctx, s)
			}
		case "span_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithSpanID(ctx, s)
			}
		case "agent_id":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithAgentID(ctx, s)
			}
		case "llm_model":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithLLMModel(ctx, s)
			}
		case "llm_provider":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithLLMProvider(ctx, s)
			}
		case "llm_route_policy":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithLLMRoutePolicy(ctx, s)
			}
		case "prompt_bundle_version":
			if s, ok := v.(string); ok && s != "" {
				ctx = types.WithPromptBundleVersion(ctx, s)
			}
		case "roles":
			if roles, ok := v.([]string); ok && len(roles) > 0 {
				ctx = types.WithRoles(ctx, roles)
			}
			if arr, ok := v.([]any); ok && len(arr) > 0 {
				roles := make([]string, 0, len(arr))
				for _, item := range arr {
					if s, ok := item.(string); ok {
						roles = append(roles, s)
					}
				}
				if len(roles) > 0 {
					ctx = types.WithRoles(ctx, roles)
				}
			}
		}
	}
	return ctx
}
