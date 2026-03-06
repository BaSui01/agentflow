package observability

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostTracker_Record(t *testing.T) {
	calc := NewCostCalculator()
	tracker := NewCostTracker(calc)

	tracker.Record("openai", "gpt-4o", "agent1", 1000, 500)
	tracker.Record("openai", "gpt-4o", "agent1", 2000, 1000)

	records := tracker.Records()
	require.Len(t, records, 2)
	assert.Equal(t, "openai", records[0].Provider)
	assert.Equal(t, "gpt-4o", records[0].Model)
	assert.Equal(t, "agent1", records[0].AgentID)
	assert.Equal(t, 1000, records[0].InputTokens)
	assert.Equal(t, 500, records[0].OutputTokens)
	assert.True(t, records[0].Cost > 0)

	total := tracker.TotalCost()
	assert.True(t, total > 0)
}

func TestCostTracker_TotalCost(t *testing.T) {
	calc := NewCostCalculator()
	tracker := NewCostTracker(calc)

	assert.Equal(t, 0.0, tracker.TotalCost())
	tracker.Record("openai", "gpt-4o", "", 1000, 500)
	assert.True(t, tracker.TotalCost() > 0)
}

func TestCostTracker_CostByProvider(t *testing.T) {
	calc := NewCostCalculator()
	tracker := NewCostTracker(calc)

	tracker.Record("openai", "gpt-4o", "", 1000, 500)
	tracker.Record("openai", "gpt-4o", "", 1000, 500)
	tracker.Record("claude", "claude-3-haiku-20240307", "", 1000, 500)

	byProvider := tracker.CostByProvider()
	assert.Len(t, byProvider, 2)
	assert.True(t, byProvider["openai"] > 0)
	assert.True(t, byProvider["claude"] > 0)
}

func TestCostTracker_CostByModel(t *testing.T) {
	calc := NewCostCalculator()
	tracker := NewCostTracker(calc)

	tracker.Record("openai", "gpt-4o", "", 1000, 500)
	tracker.Record("openai", "gpt-3.5-turbo", "", 1000, 500)

	byModel := tracker.CostByModel()
	assert.True(t, byModel["openai:gpt-4o"] > 0)
	assert.True(t, byModel["openai:gpt-3.5-turbo"] > 0)
}

func TestCostTracker_CostByAgent(t *testing.T) {
	calc := NewCostCalculator()
	tracker := NewCostTracker(calc)

	tracker.Record("openai", "gpt-4o", "agent1", 1000, 500)
	tracker.Record("openai", "gpt-4o", "agent2", 1000, 500)
	tracker.Record("openai", "gpt-4o", "agent1", 1000, 500)

	byAgent := tracker.CostByAgent()
	assert.True(t, byAgent["agent1"] > byAgent["agent2"])
}

func TestCostTracker_Reset(t *testing.T) {
	calc := NewCostCalculator()
	tracker := NewCostTracker(calc)

	tracker.Record("openai", "gpt-4o", "", 1000, 500)
	tracker.Reset()

	assert.Len(t, tracker.Records(), 0)
	assert.Equal(t, 0.0, tracker.TotalCost())
}

func TestCostTracker_UnknownModel(t *testing.T) {
	calc := NewCostCalculator()
	tracker := NewCostTracker(calc)

	tracker.Record("unknown", "unknown", "", 1000, 500)

	assert.Equal(t, 0.0, tracker.TotalCost())
	records := tracker.Records()
	require.Len(t, records, 1)
	assert.Equal(t, 0.0, records[0].Cost)
}
