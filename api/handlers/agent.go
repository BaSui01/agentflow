package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/pkg/middleware"
	"github.com/BaSui01/agentflow/pkg/telemetry"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// validAgentID validates agent ID format: alphanumeric start, up to 128 chars.
var validAgentID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

// =============================================================================
// Agent Management Handler
// =============================================================================

// AgentHandler Agent management handler
type AgentHandler struct {
	mu            sync.RWMutex
	registry      discovery.Registry
	agentRegistry *agent.AgentRegistry
	resolver      usecase.AgentResolver
	service       usecase.AgentService
	logger        *zap.Logger
	sessionMgr    *agent.SessionManager
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
func NewAgentHandler(registry discovery.Registry, agentRegistry *agent.AgentRegistry, logger *zap.Logger, resolver ...usecase.AgentResolver) *AgentHandler {
	h := &AgentHandler{
		registry:      registry,
		agentRegistry: agentRegistry,
		logger:        logger,
		sessionMgr:    agent.NewSessionManager(),
	}
	if len(resolver) > 0 {
		h.UpdateResolver(resolver[0])
	} else {
		h.UpdateResolver(nil)
	}
	return h
}

// UpdateResolver swaps the live resolver and rebuilds the backing service in
// place so existing HTTP route bindings keep using the latest runtime wiring.
func (h *AgentHandler) UpdateResolver(resolver usecase.AgentResolver) {
	if h == nil {
		return
	}
	service := usecase.NewDefaultAgentService(h.registry, resolver)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.resolver = resolver
	h.service = service
}

func (h *AgentHandler) currentService() usecase.AgentService {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.service
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
	// C-003: page-based pagination (page, page_size)
	page := 1
	pageSize := 20

	if v := r.URL.Query().Get("page"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 {
			WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest,
				"page must be a positive integer", h.logger)
			return
		}
		page = parsed
	}

	if v := r.URL.Query().Get("page_size"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 {
			WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest,
				"page_size must be a positive integer", h.logger)
			return
		}
		pageSize = parsed
	}
	if pageSize > 100 {
		pageSize = 100
	}

	service := h.currentService()
	agents, svcErr := service.ListAgents(r.Context())
	if svcErr != nil {
		h.handleAgentError(w, svcErr)
		return
	}

	total := len(agents)
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * pageSize
	end := offset + pageSize
	if end > total {
		end = total
	}
	slice := agents[offset:end]

	result := make([]AgentInfo, 0, len(slice))
	for _, a := range slice {
		result = append(result, toAgentInfo(a))
	}

	WriteSuccess(w, map[string]any{
		"items":       result,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": totalPages,
	})
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

	service := h.currentService()
	info, svcErr := service.GetAgent(r.Context(), agentID)
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
	var req usecase.AgentExecuteRequest
	if !ValidateRequest(w, r, &req, h.logger) {
		return
	}

	if apiErr := h.validateAgentExecuteRequest(&req); apiErr != nil {
		WriteError(w, apiErr.WithHTTPStatus(http.StatusBadRequest), h.logger)
		return
	}

	traceID, _ := types.TraceID(r.Context())
	if traceID == "" {
		traceID = middleware.RequestIDFromContext(r.Context())
	}
	traceLogger := telemetry.LoggerWithTrace(r.Context(), h.logger)
	service := h.currentService()
	resp, duration, execErr := service.ExecuteAgent(r.Context(), req, traceID)
	if execErr != nil {
		h.handleAgentError(w, execErr)
		return
	}

	traceLogger.Info("agent execution completed",
		zap.Strings("agent_ids", executionLogAgentIDs(req)),
		zap.String("mode", normalizedExecutionModeForLog(req)),
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
	var req usecase.AgentExecuteRequest
	if !ValidateRequest(w, r, &req, h.logger) {
		return
	}

	if apiErr := h.validateAgentExecuteRequest(&req); apiErr != nil {
		WriteError(w, apiErr.WithHTTPStatus(http.StatusBadRequest), h.logger)
		return
	}
	if len(req.AgentIDs) > 0 {
		WriteError(w, types.NewInvalidRequestError("agent_ids is not supported for streaming").WithHTTPStatus(http.StatusBadRequest), h.logger)
		return
	}

	// Preserve non-stream error semantics (404/501) before committing SSE headers.
	service := h.currentService()
	if _, svcErr := service.ResolveForOperation(r.Context(), req.AgentID, usecase.AgentOperationStream); svcErr != nil {
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

	// 创建执行会话（用于 steering/interrupt）
	session := h.sessionMgr.Create(req.AgentID)
	defer h.sessionMgr.Remove(session.ID)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	requestID := middleware.RequestIDFromContext(r.Context())
	if requestID == "" {
		requestID = r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = w.Header().Get("X-Request-ID")
		}
	}

	// 发送 session 事件（含 execution_id，客户端用于后续 interrupt 调用）
	if sessionData, err := json.Marshal(streamSessionPayload(session.ID)); err == nil {
		fmt.Fprintf(w, "event: session\ndata: %s\n\n", sessionData)
		flusher.Flush()
	}

	// 将 SteeringChannel 注入 context
	streamCtx := agent.WithSteeringChannel(r.Context(), session.SteeringCh)

	// Build the RuntimeStreamEmitter that bridges agent events to SSE
	emitter := func(event agent.RuntimeStreamEvent) {
		var sseEvent string
		var data []byte
		var err error

		switch event.Type {
		case agent.RuntimeStreamToken:
			sseEvent = "token"
			data, err = json.Marshal(streamPayload(mergeExecutionFields(map[string]any{"content": event.Delta}, event)))
		case agent.RuntimeStreamReasoning:
			sseEvent = "reasoning"
			data, err = json.Marshal(streamPayload(mergeExecutionFields(map[string]any{"reasoning_content": event.Reasoning}, event)))
		case agent.RuntimeStreamToolCall:
			sseEvent = "tool_call"
			if event.ToolCall != nil {
				data, err = json.Marshal(streamPayload(mergeExecutionFields(toolCallPayload(event.ToolCall), event)))
			}
		case agent.RuntimeStreamToolResult:
			sseEvent = "tool_result"
			if event.ToolResult != nil {
				data, err = json.Marshal(streamPayload(mergeExecutionFields(toolResultPayload(event.ToolResult), event)))
			}
		case agent.RuntimeStreamToolProgress:
			sseEvent = "tool_progress"
			data, err = json.Marshal(streamPayload(mergeExecutionFields(map[string]any{
				"tool_call_id": event.ToolCallID,
				"tool_name":    event.ToolName,
				"progress":     event.Data,
			}, event)))
		case agent.RuntimeStreamStatus:
			sseEvent = "status"
			fields := map[string]any{}
			if event.Data != nil {
				if payload, ok := event.Data.(map[string]any); ok {
					for key, value := range payload {
						fields[key] = value
					}
				}
			}
			h.logger.Debug("agent stream status",
				zap.String("agent_id", req.AgentID),
				zap.String("current_stage", event.CurrentStage),
				zap.Int("iteration_count", event.IterationCount),
				zap.String("selected_reasoning_mode", event.SelectedMode),
				zap.String("stop_reason", event.StopReason),
			)
			data, err = json.Marshal(streamPayload(mergeExecutionFields(fields, event)))
		case agent.RuntimeStreamSteering:
			sseEvent = "steering"
			data, err = json.Marshal(streamPayload(mergeExecutionFields(map[string]any{"content": event.SteeringContent}, event)))
		case agent.RuntimeStreamStopAndSend:
			sseEvent = "stop_and_send"
			data, err = json.Marshal(streamPayload(mergeExecutionFields(map[string]any{"status": "restarting"}, event)))
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

	execErr := service.ExecuteAgentStream(streamCtx, req, requestID, emitter)
	if execErr != nil {
		h.logger.Error("agent stream execution failed",
			zap.String("agent_id", req.AgentID),
			zap.String("request_id", requestID),
			zap.String("execution_id", session.ID),
			zap.Error(execErr),
		)

		// If headers are already sent (SSE mode), write error as SSE event.
		// Use api.ErrorInfo for consistency with JSON API error format.
		status := execErr.HTTPStatus
		if status == 0 {
			status = api.HTTPStatusFromErrorCode(execErr.Code)
		}
		errInfo := api.ErrorInfoFromTypesError(execErr, status)
		errPayload, marshalErr := json.Marshal(struct {
			Error     *api.ErrorInfo `json:"error"`
			RequestID string         `json:"request_id"`
		}{Error: errInfo, RequestID: requestID})
		if marshalErr != nil {
			errPayload = []byte(`{"error":{"code":"INTERNAL_ERROR","message":"agent execution failed"},"request_id":"` + requestID + `"}`)
		}
		if _, writeErr := fmt.Fprintf(w, "event: error\ndata: %s\n\n", errPayload); writeErr != nil {
			h.logger.Debug("SSE error event write failed (client disconnected)", zap.Error(writeErr))
			return
		}
		flusher.Flush()
	}

	// Send termination marker
	if _, writeErr := fmt.Fprint(w, "data: [DONE]\n\n"); writeErr != nil {
		h.logger.Debug("SSE DONE marker write failed (client disconnected)", zap.Error(writeErr))
		return
	}
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
		zap.String("execution_id", session.ID),
	)
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

	service := h.currentService()
	info, svcErr := service.GetAgent(r.Context(), agentID)
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

// HandleAgentInterrupt handles POST /api/v1/agents/execute/interrupt
// 统一处理 steer（实时引导）和 stop_and_send（停止后发送）两种中断类型。
// @Summary Interrupt agent execution
// @Description Send a steering message to an active streaming execution
// @Tags agent
// @Accept json
// @Produce json
// @Param request body object true "Interrupt request: execution_id, type (guide|stop_and_send), content"
// @Success 200 {object} Response "Interrupt accepted"
// @Failure 400 {object} Response "Invalid request"
// @Failure 404 {object} Response "Execution not found"
// @Failure 409 {object} Response "Execution already completed"
// @Security ApiKeyAuth
// @Router /api/v1/agents/execute/interrupt [post]
func (h *AgentHandler) HandleAgentInterrupt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExecutionID string `json:"execution_id"`
		Type        string `json:"type"`
		Content     string `json:"content"`
	}
	if !ValidateRequest(w, r, &req, h.logger) {
		return
	}

	if req.ExecutionID == "" || req.Type == "" || req.Content == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest,
			"execution_id, type, and content are required", h.logger)
		return
	}

	if req.Type != "guide" && req.Type != "stop_and_send" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest,
			"type must be 'guide' or 'stop_and_send'", h.logger)
		return
	}

	session, ok := h.sessionMgr.Get(req.ExecutionID)
	if !ok {
		WriteErrorMessage(w, http.StatusNotFound, types.ErrAgentNotFound,
			"execution not found", h.logger)
		return
	}

	if !session.IsRunning() {
		WriteErrorMessage(w, http.StatusConflict, types.ErrInvalidRequest,
			"execution already completed", h.logger)
		return
	}

	err := session.SteeringCh.Send(agent.SteeringMessage{
		Type:    agent.SteeringMessageType(req.Type),
		Content: req.Content,
	})
	if err != nil {
		if err == agent.ErrSteeringChannelClosed {
			WriteErrorMessage(w, http.StatusConflict, types.ErrInvalidRequest,
				"execution already completed", h.logger)
			return
		}
		WriteErrorMessage(w, http.StatusTooManyRequests, types.ErrInvalidRequest,
			"steering channel is full, try again later", h.logger)
		return
	}

	h.logger.Info("agent execution interrupted",
		zap.String("execution_id", req.ExecutionID),
		zap.String("type", req.Type),
	)

	WriteSuccess(w, map[string]string{
		"execution_id": req.ExecutionID,
		"type":         req.Type,
		"status":       "accepted",
	})
}

// HandleCapabilities handles GET /api/v1/agents/capabilities
func (h *AgentHandler) HandleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}

	WriteSuccess(w, map[string]any{
		"route_params":         []string{"provider", "model", "route_policy", "tags", "metadata"},
		"route_policies":       usecase.SupportedRoutePolicies(),
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
func (h *AgentHandler) handleAgentError(w http.ResponseWriter, err error) {
	WriteError(w, usecase.ToTypesAgentError(err), h.logger)
}

// Shutdown 停止 AgentHandler 的后台资源（SessionManager 清理 goroutine）
func (h *AgentHandler) Shutdown() {
	if h.sessionMgr != nil {
		h.sessionMgr.Stop()
	}
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

func (h *AgentHandler) validateAgentExecuteRequest(req *usecase.AgentExecuteRequest) *types.Error {
	if req == nil {
		return types.NewInvalidRequestError("request is required")
	}

	req.AgentID = strings.TrimSpace(req.AgentID)
	req.Mode = strings.TrimSpace(req.Mode)
	req.Content = strings.TrimSpace(req.Content)
	req.Model = strings.TrimSpace(req.Model)
	for i := range req.AgentIDs {
		req.AgentIDs[i] = strings.TrimSpace(req.AgentIDs[i])
	}

	if req.Content == "" {
		return types.NewInvalidRequestError("content is required")
	}
	if req.AgentID == "" && len(req.AgentIDs) == 0 {
		return types.NewInvalidRequestError("agent_id or agent_ids is required")
	}
	if req.AgentID != "" && !validAgentID.MatchString(req.AgentID) {
		return types.NewInvalidRequestError("invalid agent ID format")
	}
	if len(req.AgentIDs) > 0 {
		if req.AgentID != "" {
			return types.NewInvalidRequestError("agent_id and agent_ids cannot be provided together")
		}
		if len(req.AgentIDs) > 5 {
			return types.NewInvalidRequestError("agent_ids length exceeds maximum of 5")
		}
		for _, agentID := range req.AgentIDs {
			if agentID == "" {
				return types.NewInvalidRequestError("agent_ids cannot contain empty values")
			}
			if !validAgentID.MatchString(agentID) {
				return types.NewInvalidRequestError("invalid agent ID format")
			}
		}
	}
	if req.Mode != "" && !usecase.IsSupportedExecutionMode(req.Mode) {
		return types.NewInvalidRequestError(fmt.Sprintf("unsupported execution mode: %s", req.Mode))
	}
	if len(req.Content) > 100000 {
		return types.NewInvalidRequestError("content length exceeds maximum of 100000 characters")
	}
	if _, err := usecase.NormalizeProviderHint(req.Provider); err != nil {
		return err
	}
	if _, err := usecase.NormalizeRoutePolicy(req.RoutePolicy); err != nil {
		return err
	}

	req.Metadata = usecase.NormalizeRouteMetadata(req.Metadata)
	req.Tags = usecase.NormalizeRouteTags(req.Tags)
	return nil
}

func streamPayload(fields map[string]any) map[string]any {
	payload := map[string]any{
		"current_stage":           "",
		"iteration_count":         0,
		"selected_reasoning_mode": "",
		"stop_reason":             "",
		"checkpoint_id":           "",
		"resumable":               false,
	}
	for key, value := range fields {
		payload[key] = value
	}
	return payload
}

func mergeExecutionFields(fields map[string]any, event agent.RuntimeStreamEvent) map[string]any {
	if fields == nil {
		fields = map[string]any{}
	}
	if event.CurrentStage != "" {
		fields["current_stage"] = event.CurrentStage
	}
	if event.IterationCount > 0 {
		fields["iteration_count"] = event.IterationCount
	}
	if event.SelectedMode != "" {
		fields["selected_reasoning_mode"] = event.SelectedMode
	}
	if event.StopReason != "" {
		fields["stop_reason"] = event.StopReason
	}
	if event.CheckpointID != "" {
		fields["checkpoint_id"] = event.CheckpointID
	}
	if event.Resumable {
		fields["resumable"] = true
	}
	if event.SDKEventType != "" {
		fields["sdk_event_type"] = event.SDKEventType
	}
	if event.SDKEventName != "" {
		fields["sdk_event_name"] = event.SDKEventName
	}
	return fields
}

func streamSessionPayload(executionID string) map[string]any {
	return streamPayload(map[string]any{
		"execution_id": executionID,
	})
}

func toolCallPayload(toolCall *agent.RuntimeToolCall) map[string]any {
	if toolCall == nil {
		return nil
	}
	return map[string]any{
		"id":        toolCall.ID,
		"name":      toolCall.Name,
		"arguments": toolCall.Arguments,
	}
}

func toolResultPayload(toolResult *agent.RuntimeToolResult) map[string]any {
	if toolResult == nil {
		return nil
	}
	return map[string]any{
		"tool_call_id": toolResult.ToolCallID,
		"name":         toolResult.Name,
		"result":       toolResult.Result,
		"error":        toolResult.Error,
		"duration":     toolResult.Duration,
	}
}

func executionLogAgentIDs(req usecase.AgentExecuteRequest) []string {
	if len(req.AgentIDs) > 0 {
		return req.AgentIDs
	}
	if req.AgentID == "" {
		return nil
	}
	return []string{req.AgentID}
}

func normalizedExecutionModeForLog(req usecase.AgentExecuteRequest) string {
	if strings.TrimSpace(req.Mode) != "" {
		return strings.ToLower(strings.TrimSpace(req.Mode))
	}
	if len(req.AgentIDs) > 0 {
		return "parallel"
	}
	return "reasoning"
}
