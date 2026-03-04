package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/api"
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
	agentRegistry *agent.AgentRegistry
	resolver      AgentResolver
	service       AgentService
	logger        *zap.Logger
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
	AgentID     string            `json:"agent_id" binding:"required"`
	Content     string            `json:"content" binding:"required"`
	Provider    string            `json:"provider,omitempty"`
	Model       string            `json:"model,omitempty"`
	RoutePolicy string            `json:"route_policy,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Context     map[string]any    `json:"context,omitempty"`
	Variables   map[string]string `json:"variables,omitempty"`
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
	AgentID   string  `json:"agent_id"`
	Status    string  `json:"status"`
	Healthy   bool    `json:"healthy"`
	Endpoint  string  `json:"endpoint,omitempty"`
	Load      float64 `json:"load"`
	CheckedAt string  `json:"checked_at"`
}

// NewAgentHandler creates an Agent handler.
// The resolver parameter is optional — if nil, execute/stream endpoints return 501.
func NewAgentHandler(registry discovery.Registry, agentRegistry *agent.AgentRegistry, logger *zap.Logger, resolver ...AgentResolver) *AgentHandler {
	h := &AgentHandler{
		agentRegistry: agentRegistry,
		logger:        logger,
	}
	if len(resolver) > 0 && resolver[0] != nil {
		h.resolver = resolver[0]
	}
	h.service = NewDefaultAgentService(registry, h.resolver)
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
// @Router /api/v1/agents [get]
func (h *AgentHandler) HandleListAgents(w http.ResponseWriter, r *http.Request) {
	// Parse and validate pagination parameters (V-001)
	limit := 100 // default
	offset := 0  // default

	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 0 {
			WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest,
				"limit must be a non-negative integer", h.logger)
			return
		}
		limit = parsed
	}
	if limit > 1000 {
		limit = 1000
	}

	if v := r.URL.Query().Get("offset"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 0 {
			WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest,
				"offset must be a non-negative integer", h.logger)
			return
		}
		offset = parsed
	}

	agents, svcErr := h.service.ListAgents(r.Context())
	if svcErr != nil {
		h.handleAgentError(w, svcErr)
		return
	}

	// Apply pagination
	total := len(agents)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := agents[offset:end]

	result := make([]AgentInfo, 0, len(page))
	for _, a := range page {
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
// @Router /api/v1/agents/{id} [get]
func (h *AgentHandler) HandleGetAgent(w http.ResponseWriter, r *http.Request) {
	agentID := extractAgentID(r)
	if agentID == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "agent ID is required", h.logger)
		return
	}

	info, svcErr := h.service.GetAgent(r.Context(), agentID)
	if svcErr != nil {
		h.handleAgentError(w, svcErr)
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
// @Router /api/v1/agents/execute [post]
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

	if apiErr := h.validateAgentExecuteRequest(&req); apiErr != nil {
		WriteError(w, apiErr.WithHTTPStatus(http.StatusBadRequest), h.logger)
		return
	}

	resp, duration, execErr := h.service.ExecuteAgent(r.Context(), req, r.Header.Get("X-Request-ID"))
	if execErr != nil {
		h.handleAgentError(w, execErr)
		return
	}

	h.logger.Info("agent execution completed",
		zap.String("agent_id", req.AgentID),
		zap.Duration("duration", duration),
		zap.Int("tokens_used", resp.TokensUsed),
	)

	WriteSuccess(w, resp)
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
// @Router /api/v1/agents/execute/stream [post]
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

	if apiErr := h.validateAgentExecuteRequest(&req); apiErr != nil {
		WriteError(w, apiErr.WithHTTPStatus(http.StatusBadRequest), h.logger)
		return
	}

	// Preserve non-stream error semantics (404/501) before committing SSE headers.
	if _, svcErr := h.service.ResolveForOperation(r.Context(), req.AgentID, AgentOperationStream); svcErr != nil {
		h.handleAgentError(w, svcErr)
		return
	}

	// Verify Flusher support before committing to SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, types.NewInternalError("streaming not supported").
			WithHTTPStatus(http.StatusInternalServerError), h.logger)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = w.Header().Get("X-Request-ID")
	}

	// Build the RuntimeStreamEmitter that bridges agent events to SSE
	emitter := func(event agent.RuntimeStreamEvent) {
		var sseEvent string
		var data []byte
		var err error

		switch event.Type {
		case agent.RuntimeStreamToken:
			sseEvent = "token"
			data, err = json.Marshal(map[string]string{"content": event.Delta})
		case agent.RuntimeStreamToolCall:
			sseEvent = "tool_call"
			if event.ToolCall != nil {
				data, err = json.Marshal(event.ToolCall)
			}
		case agent.RuntimeStreamToolResult:
			sseEvent = "tool_result"
			if event.ToolResult != nil {
				data, err = json.Marshal(event.ToolResult)
			}
		case agent.RuntimeStreamToolProgress:
			sseEvent = "tool_progress"
			data, err = json.Marshal(map[string]any{
				"tool_call_id": event.ToolCallID,
				"tool_name":    event.ToolName,
				"progress":     event.Data,
			})
		default:
			return
		}

		if err != nil || data == nil {
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

	execErr := h.service.ExecuteAgentStream(r.Context(), req, requestID, emitter)
	if execErr != nil {
		h.logger.Error("agent stream execution failed",
			zap.String("agent_id", req.AgentID),
			zap.String("request_id", requestID),
			zap.Error(execErr),
		)

		// If headers are already sent (SSE mode), write error as SSE event
		errPayload, err := json.Marshal(map[string]any{
			"code":       string(types.ErrInternalError),
			"message":    "agent execution failed",
			"request_id": requestID,
		})
		if err != nil {
			errPayload = []byte(`{"code":"INTERNAL_ERROR","message":"agent execution failed"}`)
		}
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", errPayload)
		flusher.Flush()
	}

	// Send termination marker
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()

	if execErr != nil {
		h.logger.Warn("agent stream finished with error",
			zap.String("agent_id", req.AgentID),
			zap.String("request_id", requestID),
		)
		return
	}

	h.logger.Info("agent stream completed",
		zap.String("agent_id", req.AgentID),
		zap.String("request_id", requestID),
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
// @Router /api/v1/agents/plan [post]
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

	if apiErr := h.validateAgentExecuteRequest(&req); apiErr != nil {
		WriteError(w, apiErr.WithHTTPStatus(http.StatusBadRequest), h.logger)
		return
	}

	plan, planErr := h.service.PlanAgent(r.Context(), req, r.Header.Get("X-Request-ID"))
	if planErr != nil {
		h.handleAgentError(w, planErr)
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
// @Router /api/v1/agents/health [get]
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

	info, svcErr := h.service.GetAgent(r.Context(), agentID)
	if svcErr != nil {
		h.handleAgentError(w, svcErr)
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
		WriteJSON(w, http.StatusServiceUnavailable, api.Response{
			Success:   false,
			Data:      resp,
			Error:     api.ErrorInfoFromTypesError(types.NewServiceUnavailableError("agent is not healthy"), http.StatusServiceUnavailable),
			Timestamp: time.Now(),
			RequestID: w.Header().Get("X-Request-ID"),
		})
		return
	}

	WriteSuccess(w, resp)
}

// HandleCapabilities handles GET /api/v1/agents/capabilities
func (h *AgentHandler) HandleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}

	WriteSuccess(w, map[string]any{
		"route_params":         []string{"provider", "model", "route_policy", "tags", "metadata"},
		"route_policies":       supportedRoutePolicies(),
		"default_route_policy": "balanced",
		"notes": []string{
			"routing params are normalized in agent service and forwarded to llm runtime context",
			"provider hint effectiveness depends on runtime provider implementation",
		},
	})
}

// =============================================================================
// Helper Functions
// =============================================================================

// handleAgentError handles agent errors.
// Kept for test/backward compatibility while execution mapping is now centralized in AgentService.
func (h *AgentHandler) handleAgentError(w http.ResponseWriter, err error) {
	WriteError(w, toTypesAgentError(err), h.logger)
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
// Supports both /api/v1/agents/{id} (PathValue) and /api/v1/agents/some-id (prefix trim).
func extractAgentID(r *http.Request) string {
	// Try Go 1.22+ PathValue first
	if id := r.PathValue("id"); id != "" {
		if !validAgentID.MatchString(id) {
			return ""
		}
		return id
	}
	// Fallback: extract from URL path by trimming the /api/v1/agents/ prefix
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	if path != "" && path != r.URL.Path && !strings.Contains(path, "/") {
		if !validAgentID.MatchString(path) {
			return ""
		}
		return path
	}
	return ""
}

func (h *AgentHandler) validateAgentExecuteRequest(req *AgentExecuteRequest) *types.Error {
	if req == nil {
		return types.NewInvalidRequestError("request is required")
	}

	req.AgentID = strings.TrimSpace(req.AgentID)
	req.Content = strings.TrimSpace(req.Content)
	req.Model = strings.TrimSpace(req.Model)

	if req.AgentID == "" || req.Content == "" {
		return types.NewInvalidRequestError("agent_id and content are required")
	}
	if !validAgentID.MatchString(req.AgentID) {
		return types.NewInvalidRequestError("invalid agent ID format")
	}
	if len(req.Content) > 100000 {
		return types.NewInvalidRequestError("content length exceeds maximum of 100000 characters")
	}
	if _, err := normalizeProviderHint(req.Provider); err != nil {
		return err
	}
	if _, err := normalizeRoutePolicy(req.RoutePolicy); err != nil {
		return err
	}

	req.Metadata = normalizeRouteMetadata(req.Metadata)
	req.Tags = normalizeRouteTags(req.Tags)
	return nil
}
