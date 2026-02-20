package handlers

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// =============================================================================
// Agent Management Handler
// =============================================================================

// AgentHandler Agent management handler
type AgentHandler struct {
	registry      discovery.Registry
	agentRegistry *agent.AgentRegistry
	logger        *zap.Logger
	mu            sync.RWMutex
}

// AgentInfo Agent information returned by the API
type AgentInfo struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Type        agent.AgentType `json:"type"`
	State       string          `json:"state"`
	Description string          `json:"description,omitempty"`
	Model       string          `json:"model,omitempty"`
	CreatedAt   string          `json:"created_at,omitempty"`
}

// AgentExecuteRequest Agent execution request
type AgentExecuteRequest struct {
	AgentID   string            `json:"agent_id" binding:"required"`
	Content   string            `json:"content" binding:"required"`
	Context   map[string]any    `json:"context,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
}

// AgentExecuteResponse Agent execution response
type AgentExecuteResponse struct {
	TraceID      string         `json:"trace_id"`
	Content      string         `json:"content"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	TokensUsed   int            `json:"tokens_used,omitempty"`
	Cost         float64        `json:"cost,omitempty"`
	Duration     string         `json:"duration"`
	FinishReason string         `json:"finish_reason,omitempty"`
}

// AgentHealthResponse Agent health check response
type AgentHealthResponse struct {
	AgentID   string `json:"agent_id"`
	Status    string `json:"status"`
	Healthy   bool   `json:"healthy"`
	Endpoint  string `json:"endpoint,omitempty"`
	Load      float64 `json:"load"`
	CheckedAt string `json:"checked_at"`
}

// NewAgentHandler creates an Agent handler
func NewAgentHandler(registry discovery.Registry, agentRegistry *agent.AgentRegistry, logger *zap.Logger) *AgentHandler {
	return &AgentHandler{
		registry:      registry,
		agentRegistry: agentRegistry,
		logger:        logger,
	}
}

// =============================================================================
// HTTP Handlers
// =============================================================================

// HandleListAgents lists all registered agents
// @Summary List agents
// @Description Get a list of all registered agents
// @Tags agent
// @Produce json
// @Success 200 {object} Response{data=[]AgentInfo} "Agent list"
// @Failure 500 {object} Response "Internal error"
// @Security ApiKeyAuth
// @Router /v1/agents [get]
func (h *AgentHandler) HandleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.registry.ListAgents(r.Context())
	if err != nil {
		h.handleAgentError(w, err)
		return
	}

	result := make([]AgentInfo, 0, len(agents))
	for _, a := range agents {
		result = append(result, toAgentInfo(a))
	}

	WriteSuccess(w, result)
}

// HandleGetAgent gets a single agent's information
// @Summary Get agent
// @Description Get information about a specific agent
// @Tags agent
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} Response{data=AgentInfo} "Agent info"
// @Failure 404 {object} Response "Agent not found"
// @Security ApiKeyAuth
// @Router /v1/agents/{id} [get]
func (h *AgentHandler) HandleGetAgent(w http.ResponseWriter, r *http.Request) {
	agentID := extractAgentID(r)
	if agentID == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "agent ID is required", h.logger)
		return
	}

	info, err := h.registry.GetAgent(r.Context(), agentID)
	if err != nil {
		WriteError(w, types.NewNotFoundError("agent not found"), h.logger)
		return
	}

	WriteSuccess(w, toAgentInfo(info))
}

// HandleExecuteAgent executes an agent
// @Summary Execute agent
// @Description Execute an agent with the given input
// @Tags agent
// @Accept json
// @Produce json
// @Param request body AgentExecuteRequest true "Execution request"
// @Success 200 {object} Response{data=AgentExecuteResponse} "Execution result"
// @Failure 400 {object} Response "Invalid request"
// @Failure 404 {object} Response "Agent not found"
// @Failure 500 {object} Response "Execution failed"
// @Security ApiKeyAuth
// @Router /v1/agents/execute [post]
func (h *AgentHandler) HandleExecuteAgent(w http.ResponseWriter, r *http.Request) {
	var req AgentExecuteRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	if req.AgentID == "" || req.Content == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "agent_id and content are required", h.logger)
		return
	}

	// Verify the agent exists in the discovery registry
	info, err := h.registry.GetAgent(r.Context(), req.AgentID)
	if err != nil {
		WriteError(w, types.NewNotFoundError("agent not found"), h.logger)
		return
	}

	// Agent execution requires runtime dependencies (provider, memory, tools)
	// that are not available through the discovery registry alone.
	// For remote agents with an endpoint, we could proxy the request;
	// for local agents, a full runtime context is needed.
	if info.Endpoint != "" {
		h.logger.Info("agent execution requested for remote agent",
			zap.String("agent_id", req.AgentID),
			zap.String("endpoint", info.Endpoint),
		)
		WriteError(w, types.NewError(types.ErrInternalError,
			"remote agent execution via proxy is not yet supported").
			WithHTTPStatus(http.StatusNotImplemented), h.logger)
		return
	}

	h.logger.Info("agent execution requested for local agent",
		zap.String("agent_id", req.AgentID),
	)
	WriteError(w, types.NewError(types.ErrInternalError,
		"local agent execution requires runtime dependencies (provider, memory, tools) which are not yet wired").
		WithHTTPStatus(http.StatusNotImplemented), h.logger)
}

// HandlePlanAgent plans agent execution
// @Summary Plan agent execution
// @Description Get an execution plan for an agent
// @Tags agent
// @Accept json
// @Produce json
// @Param request body AgentExecuteRequest true "Plan request"
// @Success 200 {object} Response{data=map[string]any} "Execution plan"
// @Failure 400 {object} Response "Invalid request"
// @Failure 404 {object} Response "Agent not found"
// @Failure 500 {object} Response "Plan failed"
// @Security ApiKeyAuth
// @Router /v1/agents/plan [post]
func (h *AgentHandler) HandlePlanAgent(w http.ResponseWriter, r *http.Request) {
	var req AgentExecuteRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	if req.AgentID == "" || req.Content == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "agent_id and content are required", h.logger)
		return
	}

	// Verify the agent exists in the discovery registry
	info, err := h.registry.GetAgent(r.Context(), req.AgentID)
	if err != nil {
		WriteError(w, types.NewNotFoundError("agent not found"), h.logger)
		return
	}

	if info.Endpoint != "" {
		WriteError(w, types.NewError(types.ErrInternalError,
			"remote agent planning via proxy is not yet supported").
			WithHTTPStatus(http.StatusNotImplemented), h.logger)
		return
	}

	WriteError(w, types.NewError(types.ErrInternalError,
		"local agent planning requires runtime dependencies (provider, memory, tools) which are not yet wired").
		WithHTTPStatus(http.StatusNotImplemented), h.logger)
}

// HandleAgentHealth checks agent health status
// @Summary Agent health check
// @Description Check if an agent is healthy and ready
// @Tags agent
// @Produce json
// @Param id query string true "Agent ID"
// @Success 200 {object} Response{data=AgentHealthResponse} "Agent health"
// @Failure 404 {object} Response "Agent not found"
// @Failure 503 {object} Response "Agent not ready"
// @Security ApiKeyAuth
// @Router /v1/agents/health [get]
func (h *AgentHandler) HandleAgentHealth(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("id")
	if agentID == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "query parameter 'id' is required", h.logger)
		return
	}

	info, err := h.registry.GetAgent(r.Context(), agentID)
	if err != nil {
		WriteError(w, types.NewNotFoundError("agent not found"), h.logger)
		return
	}

	healthy := info.Status == discovery.AgentStatusOnline
	resp := AgentHealthResponse{
		AgentID:   agentID,
		Status:    string(info.Status),
		Healthy:   healthy,
		Endpoint:  info.Endpoint,
		Load:      info.Load,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if !healthy {
		WriteJSON(w, http.StatusServiceUnavailable, Response{
			Success:   true,
			Data:      resp,
			Timestamp: time.Now(),
		})
		return
	}

	WriteSuccess(w, resp)
}

// =============================================================================
// Helper Functions
// =============================================================================

// handleAgentError handles agent errors
func (h *AgentHandler) handleAgentError(w http.ResponseWriter, err error) {
	if typedErr, ok := err.(*types.Error); ok {
		WriteError(w, typedErr, h.logger)
		return
	}

	internalErr := types.NewError(types.ErrInternalError, "agent operation failed").
		WithCause(err).
		WithRetryable(false)

	WriteError(w, internalErr, h.logger)
}

// toAgentInfo converts a discovery.AgentInfo to the API AgentInfo
func toAgentInfo(info *discovery.AgentInfo) AgentInfo {
	ai := AgentInfo{
		State: string(info.Status),
	}
	if info.Card != nil {
		ai.ID = info.Card.Name
		ai.Name = info.Card.Name
		ai.Description = info.Card.Description
		ai.CreatedAt = info.RegisteredAt.UTC().Format(time.RFC3339)
	}
	return ai
}

// extractAgentID extracts the agent ID from the URL path.
// Supports both /v1/agents/{id} (PathValue) and /v1/agents/some-id (prefix trim).
func extractAgentID(r *http.Request) string {
	// Try Go 1.22+ PathValue first
	if id := r.PathValue("id"); id != "" {
		return id
	}
	// Fallback: extract from URL path by trimming the /v1/agents/ prefix
	path := strings.TrimPrefix(r.URL.Path, "/v1/agents/")
	if path != "" && path != r.URL.Path && !strings.Contains(path, "/") {
		return path
	}
	return ""
}
