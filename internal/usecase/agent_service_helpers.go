package usecase

import (
	"context"

	"fmt"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"strings"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/types"
)

// normalizedAgentIDs extracts unique, non-empty agent IDs from the request.
func normalizedAgentIDs(req AgentExecuteRequest) []string {
	ids := make([]string, 0, len(req.AgentIDs)+1)
	seen := map[string]struct{}{}
	for _, agentID := range req.AgentIDs {
		trimmed := strings.TrimSpace(agentID)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		ids = append(ids, trimmed)
	}
	if len(ids) == 0 {
		single := strings.TrimSpace(req.AgentID)
		if single != "" {
			ids = append(ids, single)
		}
	}
	return ids
}

// handoffAgentIDsFromRequest extracts handoff agent IDs from request context values.
func handoffAgentIDsFromRequest(values map[string]any) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	raw, ok := values["handoff_agents"]
	if !ok {
		return nil, nil
	}
	switch typed := raw.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, nil
		}
		return []string{strings.TrimSpace(typed)}, nil
	case []string:
		return append([]string(nil), typed...), nil
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("handoff_agents must contain only strings")
			}
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("handoff_agents must be a string array")
	}
}

// handoffAgentIDsFromConfig extracts handoff agent IDs from the source agent's config.
func handoffAgentIDsFromConfig(ctx context.Context, s *DefaultAgentService, requestAgentID string, sourceAgentID string) []string {
	if s == nil || s.resolver == nil {
		return nil
	}
	agentID := strings.TrimSpace(sourceAgentID)
	if agentID == "" {
		agentID = strings.TrimSpace(requestAgentID)
	}
	if agentID == "" {
		return nil
	}
	ag, err := s.resolver(ctx, agentID)
	if err != nil || ag == nil {
		return nil
	}
	cfgAccessor, ok := ag.(interface{ Config() types.AgentConfig })
	if !ok {
		return nil
	}
	return append([]string(nil), cfgAccessor.Config().Runtime.Handoffs...)
}

// mergeHandoffAgentIDs merges multiple handoff agent ID slices, deduplicating.
func mergeHandoffAgentIDs(slices ...[]string) []string {
	var merged []string
	seen := map[string]struct{}{}
	for _, slice := range slices {
		for _, raw := range slice {
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" {
				continue
			}
			if _, exists := seen[trimmed]; exists {
				continue
			}
			seen[trimmed] = struct{}{}
			merged = append(merged, trimmed)
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

// toAgentInput converts an AgentExecuteRequest to an agent.Input.
func toAgentInput(req AgentExecuteRequest, traceID string) *agent.Input {
	return &agent.Input{
		TraceID:   traceID,
		Content:   req.Content,
		Context:   req.Context,
		Variables: req.Variables,
	}
}

// executionFieldSnapshot captures key fields from an agent execution output.
type executionFieldSnapshot struct {
	CurrentStage          string
	IterationCount        int
	SelectedReasoningMode string
	StopReason            string
	CheckpointID          string
	Resumable             bool
}

// extractExecutionFields extracts structured fields from an agent output.
func extractExecutionFields(output *agent.Output) executionFieldSnapshot {
	if output == nil {
		return executionFieldSnapshot{}
	}

	fields := executionFieldSnapshot{
		CurrentStage:          output.CurrentStage,
		IterationCount:        output.IterationCount,
		SelectedReasoningMode: output.SelectedReasoningMode,
		StopReason:            output.StopReason,
		CheckpointID:          output.CheckpointID,
		Resumable:             output.Resumable,
	}
	if fields.StopReason == "" {
		fields.StopReason = output.FinishReason
	}
	if output.Metadata == nil {
		return fields
	}

	if fields.CurrentStage == "" {
		fields.CurrentStage = metadataString(output.Metadata, "current_stage")
	}
	if fields.IterationCount == 0 {
		fields.IterationCount = metadataInt(output.Metadata, "iteration_count")
	}
	if fields.SelectedReasoningMode == "" {
		fields.SelectedReasoningMode = metadataString(output.Metadata, "selected_reasoning_mode")
	}
	if fields.SelectedReasoningMode == "" {
		fields.SelectedReasoningMode = metadataString(output.Metadata, "mode")
	}
	if fields.StopReason == "" {
		fields.StopReason = metadataString(output.Metadata, "stop_reason")
	}
	if fields.CheckpointID == "" {
		fields.CheckpointID = metadataString(output.Metadata, "checkpoint_id")
	}
	if !fields.Resumable {
		fields.Resumable = metadataBool(output.Metadata, "resumable")
	}
	if stopReason := metadataString(output.Metadata, "stop_reason"); stopReason != "" && output.StopReason == "" {
		fields.StopReason = stopReason
	}
	return fields
}

// metadataString extracts a string value from metadata map.
func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

// metadataInt extracts an int value from metadata map.
func metadataInt(metadata map[string]any, key string) int {
	if metadata == nil {
		return 0
	}
	value, ok := metadata[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

// metadataBool extracts a bool value from metadata map.
func metadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	value, ok := metadata[key]
	if !ok {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

// applyAgentRoutingContext enriches the context with agent routing metadata.
func applyAgentRoutingContext(ctx context.Context, req AgentExecuteRequest) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	rc := &agent.RunConfig{
		Metadata: NormalizeRouteMetadata(req.Metadata),
		Tags:     NormalizeRouteTags(req.Tags),
	}
	hasRunConfig := len(rc.Metadata) > 0 || len(rc.Tags) > 0

	model := strings.TrimSpace(req.Model)
	if model != "" {
		rc.Model = agent.StringPtr(model)
		ctx = types.WithLLMModel(ctx, model)
		hasRunConfig = true
	}

	provider, providerErr := NormalizeProviderHint(req.Provider)
	if providerErr == nil && provider != "" {
		rc.Provider = agent.StringPtr(provider)
		ctx = types.WithLLMProvider(ctx, provider)
		hasRunConfig = true
	}

	routePolicy, routeErr := NormalizeRoutePolicy(req.RoutePolicy)
	if routeErr == nil && routePolicy != "" {
		policy := string(routePolicy)
		rc.RoutePolicy = agent.StringPtr(policy)
		ctx = types.WithLLMRoutePolicy(ctx, policy)
		hasRunConfig = true
	}

	if provider != "" || routePolicy != "" {
		if rc.Metadata == nil {
			rc.Metadata = make(map[string]string)
		}
		if provider != "" {
			rc.Metadata[llmcore.MetadataKeyChatProvider] = provider
		}
		if routePolicy != "" {
			rc.Metadata["route_policy"] = string(routePolicy)
		}
	}

	if hasRunConfig {
		ctx = agent.WithRunConfig(ctx, rc)
	}
	return ctx
}
