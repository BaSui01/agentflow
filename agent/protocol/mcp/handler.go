package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// MCPHandler HTTP 处理器，将 MCP 服务器暴露为 HTTP 端点
type MCPHandler struct {
	server *DefaultMCPServer
	logger *zap.Logger

	// SSE 客户端管理
	sseClients   map[string]chan []byte
	sseClientsMu sync.RWMutex
}

// NewMCPHandler 创建 MCP HTTP 处理器
func NewMCPHandler(server *DefaultMCPServer, logger *zap.Logger) *MCPHandler {
	return &MCPHandler{
		server:     server,
		logger:     logger,
		sseClients: make(map[string]chan []byte),
	}
}

// ServeHTTP 实现 http.Handler
func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/mcp/sse":
		h.handleSSE(w, r)
	case "/mcp/message":
		h.handleMessage(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleSSE 处理 SSE 连接
func (h *MCPHandler) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientID := fmt.Sprintf("client_%d", time.Now().UnixNano())
	ch := make(chan []byte, 100)

	h.sseClientsMu.Lock()
	h.sseClients[clientID] = ch
	h.sseClientsMu.Unlock()

	defer func() {
		h.sseClientsMu.Lock()
		delete(h.sseClients, clientID)
		h.sseClientsMu.Unlock()
		close(ch)
	}()

	// 发送 endpoint 事件（告知客户端 POST 地址）
	fmt.Fprintf(w, "event: endpoint\ndata: /mcp/message?clientId=%s\n\n", clientID)
	flusher.Flush()

	// 持续发送事件
	for {
		select {
		case <-r.Context().Done():
			return
		case data := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// handleMessage 处理 JSON-RPC 消息
func (h *MCPHandler) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg MCPMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		resp := NewMCPError(nil, ErrorCodeParseError, "parse error", nil)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// 分发请求
	response := h.dispatch(r.Context(), &msg)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	// 如果有 SSE 客户端，也推送响应
	clientID := r.URL.Query().Get("clientId")
	if clientID != "" {
		h.pushToSSEClient(clientID, response)
	}
}

// dispatch 分发 JSON-RPC 请求到对应的处理方法
func (h *MCPHandler) dispatch(ctx context.Context, msg *MCPMessage) *MCPMessage {
	switch msg.Method {
	case "initialize":
		info := h.server.GetServerInfo()
		return NewMCPResponse(msg.ID, map[string]interface{}{
			"protocolVersion": MCPVersion,
			"capabilities":    info.Capabilities,
			"serverInfo":      info,
		})

	case "tools/list":
		tools, err := h.server.ListTools(ctx)
		if err != nil {
			return NewMCPError(msg.ID, ErrorCodeInternalError, err.Error(), nil)
		}
		return NewMCPResponse(msg.ID, map[string]interface{}{"tools": tools})

	case "tools/call":
		name, _ := msg.Params["name"].(string)
		args, _ := msg.Params["arguments"].(map[string]interface{})
		result, err := h.server.CallTool(ctx, name, args)
		if err != nil {
			return NewMCPError(msg.ID, ErrorCodeInternalError, err.Error(), nil)
		}
		return NewMCPResponse(msg.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": fmt.Sprintf("%v", result)},
			},
		})

	case "resources/list":
		resources, err := h.server.ListResources(ctx)
		if err != nil {
			return NewMCPError(msg.ID, ErrorCodeInternalError, err.Error(), nil)
		}
		return NewMCPResponse(msg.ID, map[string]interface{}{"resources": resources})

	case "resources/read":
		uri, _ := msg.Params["uri"].(string)
		resource, err := h.server.GetResource(ctx, uri)
		if err != nil {
			return NewMCPError(msg.ID, ErrorCodeInternalError, err.Error(), nil)
		}
		return NewMCPResponse(msg.ID, map[string]interface{}{
			"contents": []interface{}{resource},
		})

	case "prompts/list":
		prompts, err := h.server.ListPrompts(ctx)
		if err != nil {
			return NewMCPError(msg.ID, ErrorCodeInternalError, err.Error(), nil)
		}
		return NewMCPResponse(msg.ID, map[string]interface{}{"prompts": prompts})

	case "prompts/get":
		name, _ := msg.Params["name"].(string)
		varsRaw, _ := msg.Params["arguments"].(map[string]interface{})
		vars := make(map[string]string)
		for k, v := range varsRaw {
			vars[k] = fmt.Sprintf("%v", v)
		}
		result, err := h.server.GetPrompt(ctx, name, vars)
		if err != nil {
			return NewMCPError(msg.ID, ErrorCodeInternalError, err.Error(), nil)
		}
		return NewMCPResponse(msg.ID, map[string]interface{}{
			"messages": []map[string]interface{}{
				{"role": "user", "content": map[string]interface{}{"type": "text", "text": result}},
			},
		})

	case "logging/setLevel":
		level, _ := msg.Params["level"].(string)
		h.server.SetLogLevel(level)
		return NewMCPResponse(msg.ID, map[string]interface{}{})

	default:
		return NewMCPError(msg.ID, ErrorCodeMethodNotFound,
			fmt.Sprintf("method not found: %s", msg.Method), nil)
	}
}

// pushToSSEClient 推送消息到 SSE 客户端
func (h *MCPHandler) pushToSSEClient(clientID string, msg *MCPMessage) {
	h.sseClientsMu.RLock()
	ch, exists := h.sseClients[clientID]
	h.sseClientsMu.RUnlock()

	if !exists {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	select {
	case ch <- data:
	default:
		h.logger.Warn("SSE client channel full", zap.String("client_id", clientID))
	}
}
