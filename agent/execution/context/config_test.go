package context

import (
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
)

func TestConfigFromAgentConfigAppliesContextOverrides(t *testing.T) {
	cfg := types.AgentConfig{
		LLM: types.LLMConfig{Model: "claude-3.5-sonnet"},
		Control: types.AgentControlOptions{
			Context: &types.ContextConfig{
				Enabled:              true,
				MaxContextTokens:     1234,
				ReserveForOutput:     123,
				SoftLimit:            0.6,
				WarnLimit:            0.7,
				HardLimit:            0.8,
				TargetUsage:          0.4,
				KeepLastN:            5,
				KeepSystem:           false,
				EnableMetrics:        false,
				EnableSummarize:      false,
				MemoryBudgetRatio:    0.1,
				RetrievalBudgetRatio: 0.2,
				ToolStateBudgetRatio: 0.3,
			},
		},
	}

	out := ConfigFromAgentConfig(cfg)

	assert.True(t, out.Enabled)
	assert.Equal(t, 1234, out.MaxContextTokens)
	assert.Equal(t, 123, out.ReserveForOutput)
	assert.Equal(t, 0.6, out.SoftLimit)
	assert.Equal(t, 0.7, out.WarnLimit)
	assert.Equal(t, 0.8, out.HardLimit)
	assert.Equal(t, 0.4, out.TargetUsage)
	assert.Equal(t, 5, out.KeepLastN)
	assert.True(t, out.KeepSystem, "model default keeps system even when override is false")
	assert.True(t, out.EnableMetrics, "model default keeps metrics even when override is false")
	assert.True(t, out.EnableSummarize, "model default keeps summarization even when override is false")
	assert.Equal(t, 0.1, out.MemoryBudgetRatio)
	assert.Equal(t, 0.2, out.RetrievalBudgetRatio)
	assert.Equal(t, 0.3, out.ToolStateBudgetRatio)
}

func TestAdditionalContextTextReturnsStableJSONOrEmpty(t *testing.T) {
	assert.Empty(t, AdditionalContextText(nil))
	assert.JSONEq(t, `{"tenant":"t1","user":"u1"}`, AdditionalContextText(map[string]any{"tenant": "t1", "user": "u1"}))
	assert.Empty(t, AdditionalContextText(map[string]any{"bad": func() {}}))
}
