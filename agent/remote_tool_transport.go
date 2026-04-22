package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	mcpproto "github.com/BaSui01/agentflow/agent/execution/protocol/mcp"
	"go.uber.org/zap"
)

type RemoteToolTargetKind string

const (
	RemoteToolTargetHTTP  RemoteToolTargetKind = "http"
	RemoteToolTargetMCP   RemoteToolTargetKind = "mcp"
	RemoteToolTargetA2A   RemoteToolTargetKind = "a2a"
	RemoteToolTargetStdio RemoteToolTargetKind = "stdio"
)

type remoteHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type remoteMCPToolCaller interface {
	CallTool(ctx context.Context, name string, args map[string]any) (any, error)
}

type remoteA2ATaskSender interface {
	SendTask(ctx context.Context, endpoint string, fromAgentID string, payload map[string]any) (any, error)
}

type remoteTransportFactory func(ctx context.Context, target RemoteToolTarget) (mcpproto.Transport, error)

// RemoteToolTarget describes a concrete remote invocation endpoint.
type RemoteToolTarget struct {
	Kind             RemoteToolTargetKind
	Endpoint         string
	ToolName         string
	Headers          map[string]string
	Command          string
	Args             []string
	AgentID          string
	HTTPClient       remoteHTTPDoer
	MCPClient        remoteMCPToolCaller
	A2ASender        remoteA2ATaskSender
	TransportFactory remoteTransportFactory
}

// ToolInvocationRequest is the provider-agnostic input for a remote tool call.
type ToolInvocationRequest struct {
	ToolName  string            `json:"tool_name,omitempty"`
	Arguments json.RawMessage   `json:"arguments,omitempty"`
	Input     string            `json:"input,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ToolInvocationResult is the normalized remote call output.
type ToolInvocationResult struct {
	Result   json.RawMessage   `json:"result"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RemoteToolTransport normalizes HTTP / MCP / A2A / stdio remote invocations.
type RemoteToolTransport interface {
	Invoke(ctx context.Context, target RemoteToolTarget, req ToolInvocationRequest) (ToolInvocationResult, error)
}

// DefaultRemoteToolTransport is the repository's shared remote tool adapter.
type DefaultRemoteToolTransport struct {
	httpClient remoteHTTPDoer
	logger     *zap.Logger
}

func NewDefaultRemoteToolTransport(logger *zap.Logger) RemoteToolTransport {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DefaultRemoteToolTransport{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger.With(zap.String("component", "remote_tool_transport")),
	}
}

func (t *DefaultRemoteToolTransport) Invoke(ctx context.Context, target RemoteToolTarget, req ToolInvocationRequest) (ToolInvocationResult, error) {
	switch target.Kind {
	case RemoteToolTargetHTTP:
		return t.invokeHTTP(ctx, target, req)
	case RemoteToolTargetMCP:
		return t.invokeMCP(ctx, target, req)
	case RemoteToolTargetA2A:
		return t.invokeA2A(ctx, target, req)
	case RemoteToolTargetStdio:
		return t.invokeStdio(ctx, target, req)
	default:
		return ToolInvocationResult{}, fmt.Errorf("unsupported remote tool target kind %q", target.Kind)
	}
}

func (t *DefaultRemoteToolTransport) invokeHTTP(ctx context.Context, target RemoteToolTarget, req ToolInvocationRequest) (ToolInvocationResult, error) {
	body, err := json.Marshal(map[string]any{
		"tool_name": chooseRemoteToolName(target, req),
		"arguments": decodeRemoteArguments(req.Arguments),
		"input":     strings.TrimSpace(req.Input),
		"metadata":  cloneStringMap(req.Metadata),
	})
	if err != nil {
		return ToolInvocationResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(target.Endpoint), bytes.NewReader(body))
	if err != nil {
		return ToolInvocationResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	for key, value := range target.Headers {
		httpReq.Header.Set(key, value)
	}
	client := target.HTTPClient
	if client == nil {
		client = t.httpClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return ToolInvocationResult{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ToolInvocationResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ToolInvocationResult{}, fmt.Errorf("http remote tool returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	result, err := normalizeRemoteJSONResult(raw)
	if err != nil {
		return ToolInvocationResult{}, err
	}
	return ToolInvocationResult{Result: result}, nil
}

func (t *DefaultRemoteToolTransport) invokeMCP(ctx context.Context, target RemoteToolTarget, req ToolInvocationRequest) (ToolInvocationResult, error) {
	caller, cleanup, err := t.resolveMCPCaller(ctx, target)
	if err != nil {
		return ToolInvocationResult{}, err
	}
	if cleanup != nil {
		defer cleanup()
	}
	result, err := caller.CallTool(ctx, chooseRemoteToolName(target, req), decodeRemoteArgumentsMap(req.Arguments))
	if err != nil {
		return ToolInvocationResult{}, err
	}
	raw, err := normalizeRemoteValueResult(result)
	if err != nil {
		return ToolInvocationResult{}, err
	}
	return ToolInvocationResult{Result: raw}, nil
}

func (t *DefaultRemoteToolTransport) invokeStdio(ctx context.Context, target RemoteToolTarget, req ToolInvocationRequest) (ToolInvocationResult, error) {
	stdioTarget := target
	stdioTarget.Kind = RemoteToolTargetMCP
	if stdioTarget.TransportFactory == nil {
		stdioTarget.TransportFactory = func(context.Context, RemoteToolTarget) (mcpproto.Transport, error) {
			return mcpproto.NewStdioTransport(strings.TrimSpace(target.Command), target.Args...)
		}
	}
	return t.invokeMCP(ctx, stdioTarget, req)
}

func (t *DefaultRemoteToolTransport) invokeA2A(ctx context.Context, target RemoteToolTarget, req ToolInvocationRequest) (ToolInvocationResult, error) {
	payload := map[string]any{
		"tool_name": chooseRemoteToolName(target, req),
		"arguments": decodeRemoteArguments(req.Arguments),
		"input":     strings.TrimSpace(req.Input),
		"metadata":  cloneStringMap(req.Metadata),
	}
	if target.A2ASender != nil {
		value, err := target.A2ASender.SendTask(ctx, strings.TrimSpace(target.Endpoint), firstNonEmpty(strings.TrimSpace(target.AgentID), "agentflow"), payload)
		if err != nil {
			return ToolInvocationResult{}, err
		}
		raw, err := normalizeRemoteValueResult(value)
		if err != nil {
			return ToolInvocationResult{}, err
		}
		return ToolInvocationResult{Result: raw}, nil
	}

	body, err := json.Marshal(map[string]any{
		"id":        strings.TrimSpace(target.ToolName) + "-remote-task",
		"type":      "task",
		"from":      firstNonEmpty(strings.TrimSpace(target.AgentID), "agentflow"),
		"to":        strings.TrimSpace(target.Endpoint),
		"payload":   payload,
		"timestamp": time.Now().UTC(),
	})
	if err != nil {
		return ToolInvocationResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(strings.TrimSpace(target.Endpoint), "/")+"/a2a/messages", bytes.NewReader(body))
	if err != nil {
		return ToolInvocationResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	for key, value := range target.Headers {
		httpReq.Header.Set(key, value)
	}
	client := target.HTTPClient
	if client == nil {
		client = t.httpClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return ToolInvocationResult{}, err
	}
	defer resp.Body.Close()
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ToolInvocationResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ToolInvocationResult{}, fmt.Errorf("a2a remote tool returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		return ToolInvocationResult{}, err
	}
	if msgType, ok := envelope["type"]; ok {
		var typ string
		if err := json.Unmarshal(msgType, &typ); err == nil && typ == "error" {
			return ToolInvocationResult{}, fmt.Errorf("a2a remote tool returned error response")
		}
	}
	payloadRaw := envelope["payload"]
	raw, err := normalizeRemoteJSONResult(payloadRaw)
	if err != nil {
		return ToolInvocationResult{}, err
	}
	return ToolInvocationResult{Result: raw}, nil
}

func (t *DefaultRemoteToolTransport) resolveMCPCaller(ctx context.Context, target RemoteToolTarget) (remoteMCPToolCaller, func(), error) {
	if target.MCPClient != nil {
		return target.MCPClient, nil, nil
	}
	factory := target.TransportFactory
	if factory == nil {
		return nil, nil, fmt.Errorf("mcp remote target requires MCPClient or TransportFactory")
	}
	transport, err := factory(ctx, target)
	if err != nil {
		return nil, nil, err
	}
	client := mcpproto.NewDefaultMCPClient(transport, t.logger)
	if err := client.Initialize(ctx); err != nil {
		_ = transport.Close()
		return nil, nil, err
	}
	return client, func() {
		_ = transport.Close()
	}, nil
}

func chooseRemoteToolName(target RemoteToolTarget, req ToolInvocationRequest) string {
	return firstNonEmpty(strings.TrimSpace(req.ToolName), strings.TrimSpace(target.ToolName))
}

func decodeRemoteArguments(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return value
}

func decodeRemoteArgumentsMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return map[string]any{}
	}
	return value
}

func normalizeRemoteJSONResult(raw []byte) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return json.RawMessage("null"), nil
	}
	if !json.Valid(trimmed) {
		encoded, err := json.Marshal(string(trimmed))
		return encoded, err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &envelope); err == nil {
		if result, ok := envelope["result"]; ok && len(result) > 0 {
			return result, nil
		}
	}
	return json.RawMessage(trimmed), nil
}

func normalizeRemoteValueResult(value any) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage("null"), nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return normalizeRemoteJSONResult(raw)
}
