package handlers

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type CostHandler struct {
	mu      sync.RWMutex
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

// UpdateTracker swaps the live cost tracker in place.
func (h *CostHandler) UpdateTracker(tracker *observability.CostTracker) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tracker = tracker
}

func (h *CostHandler) currentTracker() *observability.CostTracker {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.tracker
}

func (h *CostHandler) HandleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	tracker := h.currentTracker()
	if tracker == nil {
		WriteError(w, types.NewInternalError("cost tracker is not configured"), h.logger)
		return
	}
	WriteSuccess(w, map[string]any{
		"total_cost":  tracker.TotalCost(),
		"by_provider": tracker.CostByProvider(),
		"by_model":    tracker.CostByModel(),
		"by_agent":    tracker.CostByAgent(),
		"by_session":  tracker.CostBySession(),
		"by_tool":     tracker.CostByTool(),
	})
}

func (h *CostHandler) HandleRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	tracker := h.currentTracker()
	if tracker == nil {
		WriteError(w, types.NewInternalError("cost tracker is not configured"), h.logger)
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 0 {
			WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "limit must be a non-negative integer", h.logger)
			return
		}
		limit = parsed
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 0 {
			WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "offset must be a non-negative integer", h.logger)
			return
		}
		offset = parsed
	}
	records := tracker.Records()
	total := len(records)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := records[offset:end]
	out := make([]map[string]any, len(page))
	for i, rec := range page {
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
	WriteSuccess(w, map[string]any{"records": out, "total": total, "limit": limit, "offset": offset})
}

func (h *CostHandler) HandleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	tracker := h.currentTracker()
	if tracker == nil {
		WriteError(w, types.NewInternalError("cost tracker is not configured"), h.logger)
		return
	}
	tracker.Reset()
	WriteSuccess(w, map[string]string{"message": "cost records reset"})
}
