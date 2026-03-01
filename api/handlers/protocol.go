package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"github.com/BaSui01/agentflow/agent/protocol/mcp"
	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ProtocolHandler handles MCP and A2A protocol API requests.
type ProtocolHandler struct {
	mcpServer mcp.MCPServer
	a2aServer a2a.A2AServer
	logger    *zap.Logger
}

// NewProtocolHandler creates a new protocol handler.
func NewProtocolHandler(mcpServer mcp.MCPServer, a2aServer a2a.A2AServer, logger *zap.Logger) *ProtocolHandler {
	return &ProtocolHandler{
		mcpServer: mcpServer,
		a2aServer: a2aServer,
		logger:    logger,
	}
}

// HandleMCPListResources handles GET /api/v1/mcp/resources
func (h *ProtocolHandler) HandleMCPListResources(w http.ResponseWriter, r *http.Request) {
	resources, err := h.mcpServer.ListResources(r.Context())
	if err != nil {
		apiErr := types.NewError(types.ErrInternalError, "failed to list resources").
			WithCause(err)
		WriteError(w, apiErr, h.logger)
		return
	}

	WriteSuccess(w, map[string]any{
		"resources": resources,
	})
}

// HandleMCPGetResource handles GET /api/v1/mcp/resources/{uri}
func (h *ProtocolHandler) HandleMCPGetResource(w http.ResponseWriter, r *http.Request) {
	// Extract URI from path: /api/v1/mcp/resources/{uri}
	uri := strings.TrimPrefix(r.URL.Path, "/api/v1/mcp/resources/")
	if uri == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "resource URI is required", h.logger)
		return
	}

	resource, err := h.mcpServer.GetResource(r.Context(), uri)
	if err != nil {
		apiErr := types.NewError(types.ErrInvalidRequest, "resource not found: "+err.Error()).
			WithHTTPStatus(http.StatusNotFound)
		WriteError(w, apiErr, h.logger)
		return
	}

	WriteSuccess(w, resource)
}

// HandleMCPListTools handles GET /api/v1/mcp/tools
func (h *ProtocolHandler) HandleMCPListTools(w http.ResponseWriter, r *http.Request) {
	tools, err := h.mcpServer.ListTools(r.Context())
	if err != nil {
		apiErr := types.NewError(types.ErrInternalError, "failed to list tools").
			WithCause(err)
		WriteError(w, apiErr, h.logger)
		return
	}

	WriteSuccess(w, map[string]any{
		"tools": tools,
	})
}

// mcpCallToolRequest is the request body for HandleMCPCallTool.
type mcpCallToolRequest struct {
	Arguments map[string]any `json:"arguments"`
}

// HandleMCPCallTool handles POST /api/v1/mcp/tools/{name}
func (h *ProtocolHandler) HandleMCPCallTool(w http.ResponseWriter, r *http.Request) {
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	// Extract tool name from path: /api/v1/mcp/tools/{name}
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/mcp/tools/")
	if name == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "tool name is required", h.logger)
		return
	}

	var req mcpCallToolRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	result, err := h.mcpServer.CallTool(r.Context(), name, req.Arguments)
	if err != nil {
		apiErr := types.NewError(types.ErrInternalError, "tool call failed: "+err.Error())
		WriteError(w, apiErr, h.logger)
		return
	}

	h.logger.Info("mcp tool called",
		zap.String("tool", name),
	)

	WriteSuccess(w, map[string]any{
		"tool":   name,
		"result": result,
	})
}

// HandleA2AAgentCard handles GET /api/v1/a2a/.well-known/agent.json
func (h *ProtocolHandler) HandleA2AAgentCard(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "agent_id query parameter is required", h.logger)
		return
	}

	card, err := h.a2aServer.GetAgentCard(agentID)
	if err != nil {
		apiErr := types.NewError(types.ErrInvalidRequest, "agent not found: "+err.Error()).
			WithHTTPStatus(http.StatusNotFound)
		WriteError(w, apiErr, h.logger)
		return
	}

	WriteSuccess(w, card)
}

// HandleA2ASendTask handles POST /api/v1/a2a/tasks
func (h *ProtocolHandler) HandleA2ASendTask(w http.ResponseWriter, r *http.Request) {
	rec := &protocolResponseRecorder{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
	h.a2aServer.ServeHTTP(rec, r)

	for k, vals := range rec.header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}

	status := rec.statusCode
	var payload any
	if rec.body.Len() > 0 {
		bodyBytes := rec.body.Bytes()
		var parsed any
		if err := json.Unmarshal(bodyBytes, &parsed); err == nil {
			payload = parsed
		} else {
			payload = map[string]any{"raw": string(bodyBytes)}
		}
	}

	success := status >= 200 && status < 400
	var errInfo *api.ErrorInfo
	var data any
	if success {
		data = payload
	} else {
		errInfo = &api.ErrorInfo{
			Code:    "A2A_ERROR",
			Message: "a2a request failed",
		}
		if m, ok := payload.(map[string]any); ok {
			if e, ok := m["error"].(map[string]any); ok {
				if code, ok := e["code"].(string); ok && code != "" {
					errInfo.Code = code
				}
				if msg, ok := e["message"].(string); ok && msg != "" {
					errInfo.Message = msg
				}
			}
			if msg, ok := m["error"].(string); ok && msg != "" {
				errInfo.Message = msg
			}
		}
	}

	WriteJSON(w, status, api.Response{
		Success:   success,
		Data:      data,
		Error:     errInfo,
		Timestamp: time.Now(),
		RequestID: w.Header().Get("X-Request-ID"),
	})
}

type protocolResponseRecorder struct {
	header     http.Header
	body       bytes.Buffer
	statusCode int
}

func (r *protocolResponseRecorder) Header() http.Header {
	return r.header
}

func (r *protocolResponseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}

func (r *protocolResponseRecorder) Write(p []byte) (int, error) {
	return r.body.Write(p)
}
