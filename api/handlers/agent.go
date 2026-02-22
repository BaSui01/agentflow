package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// validAgentID validates agent ID format: alphanumeric start, up to 128 chars.
var validAgentID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

// =============================================================================
// Agent Management Handler
// =============================================================================

// AgentResolver resolves an agent ID to a live Agent instance.
// This decouples the handler from how agents are stored/managed at runtime.
type AgentResolver func(ctx context.Context, agentID string) (agent.Agent, error)

// AgentHandler Agent management handler
type AgentHandler struct {
	registry      discovery.Registry
	agentRegistry *agent.AgentRegistry
	resolver      AgentResolver
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

// NewAgentHandler creates an Agent handler.
// The resolver parameter is optional — if nil, execute/stream endpoints return 501.
func NewAgentHandler(registry discovery.Registry, agentRegistry *agent.AgentRegistry, logger *zap.Logger, resolver ...AgentResolver) *AgentHandler {
	h := &AgentHandler{
		registry:      registry,
		agentRegistry: agentRegistry,
		logger:        logger,
	}
	if len(resolver) > 0 && resolver[0] != nil {
		h.resolver = resolver[0]
	}
	return h
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
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req AgentExecuteRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	if req.AgentID == "" || req.Content == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "agent_id and content are required", h.logger)
		return
	}

	if !validAgentID.MatchString(req.AgentID) {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid agent ID format", h.logger)
		return
	}

	// Try resolver first (live agent instances)
	if h.resolver != nil {
		ag, err := h.resolver(r.Context(), req.AgentID)
		if err != nil {
			WriteError(w, types.NewNotFoundError(fmt.Sprintf("agent %q not found", req.AgentID)), h.logger)
			return
		}

		input := &agent.Input{
			TraceID:   r.Header.Get("X-Request-ID"),
			Content:   req.Content,
			Context:   req.Context,
			Variables: req.Variables,
		}

		ctx := r.Context()
		start := time.Now()
		output, err := ag.Execute(ctx, input)
		duration := time.Since(start)

		if err != nil {
			h.handleAgentError(w, err)
			return
		}

		resp := AgentExecuteResponse{
			TraceID:      output.TraceID,
			Content:      output.Content,
			Metadata:     output.Metadata,
			TokensUsed:   output.TokensUsed,
			Cost:         output.Cost,
			Duration:     duration.String(),
			FinishReason: output.FinishReason,
		}

		h.logger.Info("agent execution completed",
			zap.String("agent_id", req.AgentID),
			zap.Duration("duration", duration),
			zap.Int("tokens_used", output.TokensUsed),
		)

		WriteSuccess(w, resp)
		return
	}

	// Fallback: check discovery registry for existence, return 501 if found
	_, err := h.registry.GetAgent(r.Context(), req.AgentID)
	if err != nil {
		WriteError(w, types.NewNotFoundError("agent not found"), h.logger)
		return
	}

	WriteError(w, types.NewError(types.ErrInternalError,
		"agent execution is not configured — no agent resolver available").
		WithHTTPStatus(http.StatusNotImplemented), h.logger)
}

// HandleAgentStream executes an agent with streaming SSE output.
// The agent's RuntimeStreamEmitter is wired to write SSE events to the response.
// SSE event types: token, tool_call, tool_result, error, and [DONE] terminator.
// @Summary Stream agent execution
// @Description Execute an agent and stream results via SSE
// @Tags agent
// @Accept json
// @Produce text/event-stream
// @Param request body AgentExecuteRequest true "Execution request"
// @Success 200 {string} string "SSE stream"
// @Failure 400 {object} Response "Invalid request"
// @Failure 404 {object} Response "Agent not found"
// @Failure 500 {object} Response "Execution failed"
// @Security ApiKeyAuth
// @Router /v1/agents/execute/stream [post]
func (h *AgentHandler) HandleAgentStream(w http.ResponseWriter, r *http.Request) {
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req AgentExecuteRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	if req.AgentID == "" || req.Content == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "agent_id and content are required", h.logger)
		return
	}

	if !validAgentID.MatchString(req.AgentID) {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid agent ID format", h.logger)
		return
	}

	if h.resolver == nil {
		// No resolver — check discovery registry for existence, return appropriate error
		_, err := h.registry.GetAgent(r.Context(), req.AgentID)
		if err != nil {
			WriteError(w, types.NewNotFoundError("agent not found"), h.logger)
			return
		}
		WriteError(w, types.NewError(types.ErrInternalError,
			"agent streaming is not configured — no agent resolver available").
			WithHTTPStatus(http.StatusNotImplemented), h.logger)
		return
	}

	ag, err := h.resolver(r.Context(), req.AgentID)
	if err != nil {
		WriteError(w, types.NewNotFoundError(fmt.Sprintf("agent %q not found", req.AgentID)), h.logger)
		return
	}

	// Verify Flusher support before committing to SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, types.NewError(types.ErrInternalError, "streaming not supported").
			WithHTTPStatus(http.StatusInternalServerError), h.logger)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Build the RuntimeStreamEmitter that bridges agent events to SSE
	emitter := func(event agent.RuntimeStreamEvent) {
		var sseEvent string
		var data []byte

		switch event.Type {
		case agent.RuntimeStreamToken:
			sseEvent = "token"
			data, _ = json.Marshal(map[string]string{"content": event.Delta})
		case agent.RuntimeStreamToolCall:
			sseEvent = "tool_call"
			if event.ToolCall != nil {
				data, _ = json.Marshal(event.ToolCall)
			}
		case agent.RuntimeStreamToolResult:
			sseEvent = "tool_result"
			if event.ToolResult != nil {
				data, _ = json.Marshal(event.ToolResult)
			}
		default:
			return
		}

		if data == nil {
			return
		}

		// Check client disconnect before writing
		select {
		case <-r.Context().Done():
			return
		default:
		}

		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", sseEvent, data)
		flusher.Flush()
	}

	input := &agent.Input{
		TraceID:   r.Header.Get("X-Request-ID"),
		Content:   req.Content,
		Context:   req.Context,
		Variables: req.Variables,
	}

	// Inject the emitter into context so the agent's streaming path picks it up
	ctx := agent.WithRuntimeStreamEmitter(r.Context(), emitter)

	_, err = ag.Execute(ctx, input)
	if err != nil {
		// If headers are already sent (SSE mode), write error as SSE event
		errPayload, _ := json.Marshal(map[string]string{"error": err.Error()})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", errPayload)
		flusher.Flush()
	}

	// Send termination marker
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()

	h.logger.Info("agent stream completed",
		zap.String("agent_id", req.AgentID),
	)
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
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req AgentExecuteRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	if req.AgentID == "" || req.Content == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "agent_id and content are required", h.logger)
		return
	}

	if !validAgentID.MatchString(req.AgentID) {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid agent ID format", h.logger)
		return
	}

	if h.resolver == nil {
		// No resolver — check discovery registry for existence, return appropriate error
		_, err := h.registry.GetAgent(r.Context(), req.AgentID)
		if err != nil {
			WriteError(w, types.NewNotFoundError("agent not found"), h.logger)
			return
		}
		WriteError(w, types.NewError(types.ErrInternalError,
			"agent planning is not configured — no agent resolver available").
			WithHTTPStatus(http.StatusNotImplemented), h.logger)
		return
	}

	ag, err := h.resolver(r.Context(), req.AgentID)
	if err != nil {
		WriteError(w, types.NewNotFoundError(fmt.Sprintf("agent %q not found", req.AgentID)), h.logger)
		return
	}

	input := &agent.Input{
		TraceID:   r.Header.Get("X-Request-ID"),
		Content:   req.Content,
		Context:   req.Context,
		Variables: req.Variables,
	}

	plan, err := ag.Plan(r.Context(), input)
	if err != nil {
		h.handleAgentError(w, err)
		return
	}

	WriteSuccess(w, plan)
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

	if !validAgentID.MatchString(agentID) {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid agent ID format", h.logger)
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
			Success:   false,
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
		if !validAgentID.MatchString(id) {
			return ""
		}
		return id
	}
	// Fallback: extract from URL path by trimming the /v1/agents/ prefix
	path := strings.TrimPrefix(r.URL.Path, "/v1/agents/")
	if path != "" && path != r.URL.Path && !strings.Contains(path, "/") {
		if !validAgentID.MatchString(path) {
			return ""
		}
		return path
	}
	return ""
}
