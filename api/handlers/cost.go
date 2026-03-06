package handlers

import (
	"net/http"

	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type CostHandler struct {
	tracker *observability.CostTracker
	logger  *zap.Logger
}

func NewCostHandler(tracker *observability.CostTracker, logger *zap.Logger) *CostHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CostHandler{
		tracker: tracker,
		logger:  logger,
	}
}

func (h *CostHandler) HandleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if h.tracker == nil {
		WriteError(w, types.NewInternalError("cost tracker is not configured"), h.logger)
		return
	}
	WriteSuccess(w, map[string]any{
		"total_cost":       h.tracker.TotalCost(),
		"by_provider":      h.tracker.CostByProvider(),
		"by_model":         h.tracker.CostByModel(),
		"by_agent":         h.tracker.CostByAgent(),
		"by_session":      h.tracker.CostBySession(),
		"by_tool":          h.tracker.CostByTool(),
	})
}

func (h *CostHandler) HandleRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if h.tracker == nil {
		WriteError(w, types.NewInternalError("cost tracker is not configured"), h.logger)
		return
	}
	records := h.tracker.Records()
	out := make([]map[string]any, len(records))
	for i, rec := range records {
		out[i] = map[string]any{
			"provider":      rec.Provider,
			"model":         rec.Model,
			"agent_id":      rec.AgentID,
			"session_id":    rec.SessionID,
			"tool_name":     rec.ToolName,
			"input_tokens":  rec.InputTokens,
			"output_tokens": rec.OutputTokens,
			"cost":          rec.Cost,
			"timestamp":     rec.Timestamp,
		}
	}
	WriteSuccess(w, map[string]any{"records": out})
}

func (h *CostHandler) HandleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if h.tracker == nil {
		WriteError(w, types.NewInternalError("cost tracker is not configured"), h.logger)
		return
	}
	h.tracker.Reset()
	WriteSuccess(w, map[string]string{"message": "cost records reset"})
}
