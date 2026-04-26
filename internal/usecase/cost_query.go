package usecase

import (
	"time"

	"github.com/BaSui01/agentflow/types"
)

// CostRecordView represents a single cost record for API responses.
type CostRecordView struct {
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	AgentID      string    `json:"agent_id"`
	SessionID    string    `json:"session_id"`
	ToolName     string    `json:"tool_name"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	Cost         float64   `json:"cost"`
	Timestamp    time.Time `json:"timestamp"`
}

// CostSummaryView represents aggregated cost statistics.
type CostSummaryView struct {
	TotalCost  float64            `json:"total_cost"`
	ByProvider map[string]float64 `json:"by_provider"`
	ByModel    map[string]float64 `json:"by_model"`
	ByAgent    map[string]float64 `json:"by_agent"`
	BySession  map[string]float64 `json:"by_session"`
	ByTool     map[string]float64 `json:"by_tool"`
}

// CostRecordsResult contains paginated cost records.
type CostRecordsResult struct {
	Records []CostRecordView `json:"records"`
	Total   int              `json:"total"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
}

// CostTracker abstracts the cost tracking operations needed by CostQueryService.
// This decouples the usecase layer from llm/observability.
type CostTracker interface {
	TotalCost() float64
	CostByProvider() map[string]float64
	CostByModel() map[string]float64
	CostByAgent() map[string]float64
	CostBySession() map[string]float64
	CostByTool() map[string]float64
	Records() []CostRecord
	Reset()
}

// CostRecord is the DTO representation of a cost record.
// It mirrors observability.CostRecord for usecase layer isolation.
type CostRecord struct {
	Provider     string
	Model        string
	AgentID      string
	SessionID    string
	ToolName     string
	InputTokens  int
	OutputTokens int
	Cost         float64
	Timestamp    time.Time
}

// CostQueryService provides cost query operations for the API layer.
type CostQueryService interface {
	// GetSummary returns aggregated cost statistics.
	GetSummary() (*CostSummaryView, *types.Error)
	// GetRecords returns paginated cost records.
	GetRecords(limit, offset int) (*CostRecordsResult, *types.Error)
	// Reset clears all cost records.
	Reset() *types.Error
}

// DefaultCostQueryService is the default implementation of CostQueryService.
type DefaultCostQueryService struct {
	tracker CostTracker
}

// NewDefaultCostQueryService creates a new CostQueryService with the given tracker.
func NewDefaultCostQueryService(tracker CostTracker) *DefaultCostQueryService {
	return &DefaultCostQueryService{tracker: tracker}
}

// GetSummary returns aggregated cost statistics.
func (s *DefaultCostQueryService) GetSummary() (*CostSummaryView, *types.Error) {
	if s.tracker == nil {
		return nil, types.NewInternalError("cost tracker is not configured")
	}

	return &CostSummaryView{
		TotalCost:  s.tracker.TotalCost(),
		ByProvider: s.tracker.CostByProvider(),
		ByModel:    s.tracker.CostByModel(),
		ByAgent:    s.tracker.CostByAgent(),
		BySession:  s.tracker.CostBySession(),
		ByTool:     s.tracker.CostByTool(),
	}, nil
}

// GetRecords returns paginated cost records.
func (s *DefaultCostQueryService) GetRecords(limit, offset int) (*CostRecordsResult, *types.Error) {
	if s.tracker == nil {
		return nil, types.NewInternalError("cost tracker is not configured")
	}

	records := s.tracker.Records()
	total := len(records)

	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}

	page := records[offset:end]
	out := make([]CostRecordView, len(page))
	for i, rec := range page {
		out[i] = CostRecordView{
			Provider:     rec.Provider,
			Model:        rec.Model,
			AgentID:      rec.AgentID,
			SessionID:    rec.SessionID,
			ToolName:     rec.ToolName,
			InputTokens:  rec.InputTokens,
			OutputTokens: rec.OutputTokens,
			Cost:         rec.Cost,
			Timestamp:    rec.Timestamp,
		}
	}

	return &CostRecordsResult{
		Records: out,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}, nil
}

// Reset clears all cost records.
func (s *DefaultCostQueryService) Reset() *types.Error {
	if s.tracker == nil {
		return types.NewInternalError("cost tracker is not configured")
	}

	s.tracker.Reset()
	return nil
}
