package bootstrap

import (
	"github.com/BaSui01/agentflow/internal/usecase"
	llmobservability "github.com/BaSui01/agentflow/llm/observability"
)

// costTrackerAdapter adapts observability.CostTracker to usecase.CostTracker interface.
type costTrackerAdapter struct {
	tracker *llmobservability.CostTracker
}

// NewCostTrackerAdapter creates a usecase.CostTracker adapter for observability.CostTracker.
func NewCostTrackerAdapter(tracker *llmobservability.CostTracker) usecase.CostTracker {
	if tracker == nil {
		return nil
	}
	return &costTrackerAdapter{tracker: tracker}
}

func (a *costTrackerAdapter) TotalCost() float64 {
	return a.tracker.TotalCost()
}

func (a *costTrackerAdapter) CostByProvider() map[string]float64 {
	return a.tracker.CostByProvider()
}

func (a *costTrackerAdapter) CostByModel() map[string]float64 {
	return a.tracker.CostByModel()
}

func (a *costTrackerAdapter) CostByAgent() map[string]float64 {
	return a.tracker.CostByAgent()
}

func (a *costTrackerAdapter) CostBySession() map[string]float64 {
	return a.tracker.CostBySession()
}

func (a *costTrackerAdapter) CostByTool() map[string]float64 {
	return a.tracker.CostByTool()
}

func (a *costTrackerAdapter) Records() []usecase.CostRecord {
	records := a.tracker.Records()
	out := make([]usecase.CostRecord, len(records))
	for i, r := range records {
		out[i] = usecase.CostRecord{
			Provider:     r.Provider,
			Model:        r.Model,
			AgentID:      r.AgentID,
			SessionID:    r.SessionID,
			ToolName:     r.ToolName,
			InputTokens:  r.InputTokens,
			OutputTokens: r.OutputTokens,
			Cost:         r.Cost,
			Timestamp:    r.Timestamp,
		}
	}
	return out
}

func (a *costTrackerAdapter) Reset() {
	a.tracker.Reset()
}

// NewCostQueryService creates a CostQueryService from an observability CostTracker.
func NewCostQueryService(tracker *llmobservability.CostTracker) usecase.CostQueryService {
	adapter := NewCostTrackerAdapter(tracker)
	if adapter == nil {
		return nil
	}
	return usecase.NewDefaultCostQueryService(adapter)
}
