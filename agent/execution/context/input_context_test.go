package context

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
)

func TestApplyInputContextInjectsKnownStringValues(t *testing.T) {
	ctx := ApplyInputContext(context.Background(), map[string]any{
		"trace_id":              "trace-1",
		"tenant_id":             "tenant-1",
		"user_id":               "user-1",
		"run_id":                "run-1",
		"parent_run_id":         "parent-1",
		"span_id":               "span-1",
		"agent_id":              "agent-1",
		"llm_model":             "gpt-4o",
		"llm_provider":          "openai",
		"llm_route_policy":      "fast",
		"prompt_bundle_version": "v2",
		"unknown":               "ignored",
	})

	assert.Equal(t, "trace-1", mustString(types.TraceID(ctx)))
	assert.Equal(t, "tenant-1", mustString(types.TenantID(ctx)))
	assert.Equal(t, "user-1", mustString(types.UserID(ctx)))
	assert.Equal(t, "run-1", mustString(types.RunID(ctx)))
	assert.Equal(t, "parent-1", mustString(types.ParentRunID(ctx)))
	assert.Equal(t, "span-1", mustString(types.SpanID(ctx)))
	assert.Equal(t, "agent-1", mustString(types.AgentID(ctx)))
	assert.Equal(t, "gpt-4o", mustString(types.LLMModel(ctx)))
	assert.Equal(t, "openai", mustString(types.LLMProvider(ctx)))
	assert.Equal(t, "fast", mustString(types.LLMRoutePolicy(ctx)))
	assert.Equal(t, "v2", mustString(types.PromptBundleVersion(ctx)))
}

func TestApplyInputContextHandlesRolesShapesAndIgnoresInvalidValues(t *testing.T) {
	base := context.Background()
	ctx := ApplyInputContext(base, map[string]any{
		"trace_id": 123,
		"roles":    []any{"admin", 7, "reviewer"},
	})

	assert.Empty(t, mustString(types.TraceID(ctx)))
	assert.Equal(t, []string{"admin", "reviewer"}, mustStrings(types.Roles(ctx)))

	ctx = ApplyInputContext(base, map[string]any{"roles": []string{"operator", "auditor"}})
	assert.Equal(t, []string{"operator", "auditor"}, mustStrings(types.Roles(ctx)))

	assert.Equal(t, base, ApplyInputContext(base, nil))
}

func mustString(value string, _ bool) string { return value }

func mustStrings(value []string, _ bool) []string { return value }
