package mcp

import (
	"context"
	"fmt"
	"sync/atomic"

	"go.uber.org/zap"
)

type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ToolsChangedHandler is called when the server notifies that its tool list has changed.
type ToolsChangedHandler func(ctx context.Context, tools []MCPTool)

type DefaultMCPClient struct {
	transport       Transport
	logger          *zap.Logger
	nextID          atomic.Int64
	toolsChangedFn  ToolsChangedHandler
	notifDone       chan struct{}
}

// ClientOption configures optional DefaultMCPClient behavior.
type ClientOption func(*DefaultMCPClient)

// WithToolsChangedHandler registers a callback for tools/list_changed notifications.
func WithToolsChangedHandler(fn ToolsChangedHandler) ClientOption {
	return func(c *DefaultMCPClient) { c.toolsChangedFn = fn }
}

func NewDefaultMCPClient(transport Transport, logger *zap.Logger, opts ...ClientOption) *DefaultMCPClient {
	if logger == nil {
		panic("agent.MCPClient: logger is required and cannot be nil")
	}
	c := &DefaultMCPClient{
		transport: transport,
		logger:    logger.With(zap.String("component", "mcp_client")),
		notifDone: make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *DefaultMCPClient) request(ctx context.Context, method string, params map[string]any) (*MCPMessage, error) {
	id := c.nextID.Add(1)
	req := NewMCPRequest(id, method, params)
	if err := c.transport.Send(ctx, req); err != nil {
		return nil, fmt.Errorf("send %s: %w", method, err)
	}
	resp, err := c.transport.Receive(ctx)
	if err != nil {
		return nil, fmt.Errorf("receive %s: %w", method, err)
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp, nil
}

func (c *DefaultMCPClient) Initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": MCPVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "agentflow-mcp-client",
			"version": "1.0.0",
		},
	}
	_, err := c.request(ctx, "initialize", params)
	if err != nil {
		return err
	}
	notif := &MCPMessage{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	return c.transport.Send(ctx, notif)
}

func (c *DefaultMCPClient) ListTools(ctx context.Context) ([]MCPTool, error) {
	resp, err := c.request(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	raw, ok := resp.Result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("tools/list: unexpected result type %T", resp.Result)
	}
	toolsRaw, ok := raw["tools"]
	if !ok {
		return nil, fmt.Errorf("tools/list: missing tools in result")
	}
	toolsSlice, ok := toolsRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("tools/list: tools is not array, got %T", toolsRaw)
	}
	out := make([]MCPTool, 0, len(toolsSlice))
	for i, t := range toolsSlice {
		tm, ok := t.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("tools/list: tool[%d] is not object, got %T", i, t)
		}
		name, _ := tm["name"].(string)
		desc, _ := tm["description"].(string)
		schema, _ := tm["inputSchema"].(map[string]any)
		out = append(out, MCPTool{Name: name, Description: desc, InputSchema: schema})
	}
	return out, nil
}

func (c *DefaultMCPClient) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	params := map[string]any{
		"name":      name,
		"arguments": args,
	}
	resp, err := c.request(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (c *DefaultMCPClient) ListResources(ctx context.Context) ([]Resource, error) {
	resp, err := c.request(ctx, "resources/list", nil)
	if err != nil {
		return nil, err
	}
	raw, ok := resp.Result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("resources/list: unexpected result type %T", resp.Result)
	}
	resList, ok := raw["resources"].([]any)
	if !ok {
		return []Resource{}, nil
	}
	out := make([]Resource, 0, len(resList))
	for _, r := range resList {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		res := Resource{}
		res.URI, _ = rm["uri"].(string)
		res.Name, _ = rm["name"].(string)
		res.Description, _ = rm["description"].(string)
		res.MimeType, _ = rm["mimeType"].(string)
		out = append(out, res)
	}
	return out, nil
}

func (c *DefaultMCPClient) ReadResource(ctx context.Context, uri string) (*Resource, error) {
	params := map[string]any{"uri": uri}
	resp, err := c.request(ctx, "resources/read", params)
	if err != nil {
		return nil, err
	}
	raw, ok := resp.Result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("resources/read: unexpected result type %T", resp.Result)
	}
	res := &Resource{URI: uri}
	if contents, ok := raw["contents"].([]any); ok && len(contents) > 0 {
		if cm, ok := contents[0].(map[string]any); ok {
			res.Content, _ = cm["text"].(string)
			res.MimeType, _ = cm["mimeType"].(string)
		}
	}
	return res, nil
}

func (c *DefaultMCPClient) ListPrompts(ctx context.Context) ([]PromptTemplate, error) {
	resp, err := c.request(ctx, "prompts/list", nil)
	if err != nil {
		return nil, err
	}
	raw, ok := resp.Result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("prompts/list: unexpected result type %T", resp.Result)
	}
	promptList, ok := raw["prompts"].([]any)
	if !ok {
		return []PromptTemplate{}, nil
	}
	out := make([]PromptTemplate, 0, len(promptList))
	for _, p := range promptList {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		pt := PromptTemplate{}
		pt.Name, _ = pm["name"].(string)
		pt.Description, _ = pm["description"].(string)
		out = append(out, pt)
	}
	return out, nil
}

func (c *DefaultMCPClient) GetPrompt(ctx context.Context, name string, vars map[string]string) (string, error) {
	args := make(map[string]any, len(vars))
	for k, v := range vars {
		args[k] = v
	}
	params := map[string]any{
		"name":      name,
		"arguments": args,
	}
	resp, err := c.request(ctx, "prompts/get", params)
	if err != nil {
		return "", err
	}
	raw, ok := resp.Result.(map[string]any)
	if !ok {
		return "", fmt.Errorf("prompts/get: unexpected result type %T", resp.Result)
	}
	if messages, ok := raw["messages"].([]any); ok && len(messages) > 0 {
		if mm, ok := messages[0].(map[string]any); ok {
			switch content := mm["content"].(type) {
			case string:
				return content, nil
			case map[string]any:
				if text, ok := content["text"].(string); ok {
					return text, nil
				}
			}
		}
	}
	return "", nil
}

// RefreshTools re-fetches the tool list and invokes the ToolsChangedHandler if registered.
// Use after receiving a tools/list_changed notification or periodically for polling.
func (c *DefaultMCPClient) RefreshTools(ctx context.Context) ([]MCPTool, error) {
	tools, err := c.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	if c.toolsChangedFn != nil {
		c.toolsChangedFn(ctx, tools)
	}
	return tools, nil
}

// StartNotificationListener begins a background goroutine that reads messages from
// the transport. Notifications (messages with Method but no ID) are dispatched to
// registered handlers. Call Close() to stop the listener.
//
// Only useful with streaming transports (SSE). For request-response transports,
// use RefreshTools with polling instead.
func (c *DefaultMCPClient) StartNotificationListener(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.notifDone:
				return
			default:
			}
			msg, err := c.transport.Receive(ctx)
			if err != nil {
				return
			}
			if msg == nil {
				continue
			}
			if msg.Method == "notifications/tools/list_changed" && c.toolsChangedFn != nil {
				tools, err := c.ListTools(ctx)
				if err != nil {
					c.logger.Warn("failed to refresh tools after list_changed", zap.Error(err))
					continue
				}
				c.toolsChangedFn(ctx, tools)
			}
		}
	}()
}

func (c *DefaultMCPClient) Close() error {
	select {
	case <-c.notifDone:
	default:
		close(c.notifDone)
	}
	return c.transport.Close()
}
