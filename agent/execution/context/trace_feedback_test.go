package context

import (
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectTraceFeedbackSignalsDerivesRuntimeHints(t *testing.T) {
	signals := CollectTraceFeedbackSignals(CollectTraceFeedbackSignalsInput{
		UserInputContext: map[string]any{
			"checkpoint_id":              "ckpt-1",
			"tool_verification_required": true,
			"top_level_loop_budget":      float64(3),
			"agent_ids":                  []string{"a", "b"},
		},
		Snapshot: ExplainabilitySynopsisSnapshot{
			Synopsis:             "prior synopsis",
			CompressedHistory:    "older turns",
			CompressedEventCount: 2,
		},
		HasMemoryRuntime:        true,
		ContextStatus:           &Status{Level: LevelNormal, UsageRatio: 0.72},
		AcceptanceCriteriaCount: 1,
		Handoff:                 true,
	})

	assert.True(t, signals.HasPriorSynopsis)
	assert.True(t, signals.HasCompressedHistory)
	assert.True(t, signals.HasMemoryRuntime)
	assert.True(t, signals.Resume)
	assert.True(t, signals.Handoff)
	assert.True(t, signals.MultiAgent)
	assert.True(t, signals.Verification)
	assert.True(t, signals.ComplexTask)
	assert.Equal(t, "normal", signals.ContextPressure)
	assert.Equal(t, 0.72, signals.UsageRatio)
	assert.Equal(t, 2, signals.CompressedEventCount)
}

func TestRuleBasedTraceFeedbackPlannerSelectsSynopsisAndHistory(t *testing.T) {
	planner := NewRuleBasedTraceFeedbackPlanner()
	plan := planner.Plan(&TraceFeedbackPlanningInput{
		Signals: TraceFeedbackSignals{
			HasPriorSynopsis:     true,
			HasCompressedHistory: true,
			HasMemoryRuntime:     true,
			Resume:               true,
			Verification:         true,
			UsageRatio:           0.5,
		},
		Config: DefaultTraceFeedbackConfig(),
	})

	assert.Equal(t, "rule_based_trace_feedback_planner", plan.PlannerID)
	assert.Equal(t, TraceFeedbackSynopsisAndHistory, plan.RecommendedAction)
	assert.True(t, plan.InjectSynopsis)
	assert.True(t, plan.InjectHistory)
	assert.True(t, plan.InjectMemoryRecall)
	assert.Equal(t, "trace_synopsis", plan.PrimaryLayer)
	assert.Equal(t, "trace_history", plan.SecondaryLayer)
	assert.Contains(t, plan.SelectedLayers, "memory_recall")
	assert.Contains(t, plan.Reasons, "resume")
	assert.Contains(t, plan.Summary, "action=synopsis_and_history")
}

func TestRuleBasedTraceFeedbackPlannerSuppressesHeavyLayersUnderPressure(t *testing.T) {
	plan := NewRuleBasedTraceFeedbackPlanner().Plan(&TraceFeedbackPlanningInput{
		Signals: TraceFeedbackSignals{
			HasPriorSynopsis:     true,
			HasCompressedHistory: true,
			HasMemoryRuntime:     true,
			Verification:         true,
			ContextPressure:      LevelEmergency.String(),
		},
		Config: DefaultTraceFeedbackConfig(),
	})

	assert.True(t, plan.InjectSynopsis)
	assert.False(t, plan.InjectHistory)
	assert.False(t, plan.InjectMemoryRecall)
	assert.Contains(t, plan.SuppressedLayers, "trace_history")
	assert.Contains(t, plan.SuppressedLayers, "memory_recall")
	assert.Equal(t, TraceFeedbackSynopsisOnly, plan.RecommendedAction)
}

func TestComposedTraceFeedbackPlannerAppliesHints(t *testing.T) {
	planner := NewComposedTraceFeedbackPlanner(NewRuleBasedTraceFeedbackPlanner(), NewHintTraceFeedbackAdapter())
	plan := planner.Plan(&TraceFeedbackPlanningInput{
		UserInputContext: map[string]any{
			"trace_feedback_force_history":       true,
			"trace_feedback_force_memory_recall": true,
			"trace_feedback_goal":                "operator_goal",
			"trace_feedback_primary_layer":       "manual_primary",
		},
		Signals: TraceFeedbackSignals{HasPriorSynopsis: true, HasCompressedHistory: true, HasMemoryRuntime: true},
		Config:  DefaultTraceFeedbackConfig(),
	})

	require.NotNil(t, plan.Metadata)
	assert.Equal(t, "composed_trace_feedback_planner", plan.PlannerID)
	assert.Equal(t, "composed", plan.Metadata["planner_kind"])
	assert.Equal(t, "manual_primary", plan.PrimaryLayer)
	assert.Equal(t, "operator_goal", plan.Goal)
	assert.True(t, plan.InjectHistory)
	assert.True(t, plan.InjectMemoryRecall)
	assert.Contains(t, plan.SelectedLayers, "trace_history")
	assert.Contains(t, plan.SelectedLayers, "memory_recall")
	assert.Contains(t, plan.Reasons, "force_history")
	assert.Contains(t, plan.Reasons, "force_memory_recall")
}

func TestTraceFeedbackConfigFromAgentConfigReadsFormalContext(t *testing.T) {
	cfg := TraceFeedbackConfigFromAgentConfig(types.AgentConfig{
		Control: types.AgentControlOptions{
			Context: &types.ContextConfig{
				TraceFeedbackEnabled:             true,
				TraceFeedbackComplexityThreshold: 4,
				TraceSynopsisMinScore:            3,
				TraceHistoryMinScore:             5,
				TraceMemoryRecallMinScore:        2,
				TraceHistoryMaxUsageRatio:        0.6,
			},
		},
	})

	assert.True(t, cfg.Enabled)
	assert.Equal(t, 4, cfg.ComplexityThreshold)
	assert.Equal(t, 3, cfg.SynopsisMinScore)
	assert.Equal(t, 5, cfg.HistoryMinScore)
	assert.Equal(t, 2, cfg.MemoryRecallMinScore)
	assert.Equal(t, 0.6, cfg.HistoryMaxUsageRatio)
}
