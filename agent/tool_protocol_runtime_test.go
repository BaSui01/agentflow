package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mcpproto "github.com/BaSui01/agentflow/agent/protocol/mcp"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type remoteMCPCallerStub struct {
	lastName string
	lastArgs map[string]any
	result   any
	err      error
}

func (s *remoteMCPCallerStub) CallTool(_ context.Context, name string, args map[string]any) (any, error) {
	s.lastName = name
	s.lastArgs = args
	return s.result, s.err
}

type remoteA2AClientStub struct {
	endpoint string
	fromID   string
	payload  map[string]any
	resp     any
	err      error
}

func (s *remoteA2AClientStub) SendTask(_ context.Context, endpoint string, fromAgentID string, payload map[string]any) (any, error) {
	s.endpoint = endpoint
	s.fromID = fromAgentID
	s.payload = payload
	return s.resp, s.err
}

type inProcessMCPHandlerStub struct{}

func (h *inProcessMCPHandlerStub) HandleRequest(_ context.Context, msg *mcpproto.MCPMessage) (*mcpproto.MCPMessage, error) {
	if msg.Method == "initialize" {
		return &mcpproto.MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result: map[string]any{
				"protocolVersion": mcpproto.MCPVersion,
			},
		}, nil
	}
	if msg.Method == "tools/call" {
		return &mcpproto.MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result: map[string]any{
				"output": "echo::echo",
			},
		}, nil
	}
	return &mcpproto.MCPMessage{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result:  map[string]any{},
	}, nil
}

func TestToolProtocolRuntime_ExecuteDelegatesToToolManager(t *testing.T) {
	agent := NewBaseAgent(
		testAgentConfig("agent-1", "Agent", "gpt-4"),
		testGatewayFromProvider(&testProvider{name: "main", supportsNative: true}),
		nil,
		&testToolManager{
			executeForAgentFn: func(_ context.Context, agentID string, calls []types.ToolCall) []llmtools.ToolResult {
				require.Equal(t, "agent-1", agentID)
				require.Len(t, calls, 1)
				return []llmtools.ToolResult{{
					ToolCallID: calls[0].ID,
					Name:       calls[0].Name,
					Result:     json.RawMessage(`{"ok":true}`),
				}}
			},
		},
		nil,
		zap.NewNop(),
		nil,
	)
	pr := &preparedRequest{
		options: types.ExecutionOptions{
			Tools: types.ToolProtocolOptions{
				AllowedTools: []string{"search"},
			},
		},
	}
	runtime := NewDefaultToolProtocolRuntime()
	prepared := runtime.Prepare(agent, pr)
	results := runtime.Execute(context.Background(), prepared, []types.ToolCall{{
		ID:        "call-1",
		Name:      "search",
		Arguments: json.RawMessage(`{"query":"hi"}`),
	}})
	require.Len(t, results, 1)
	assert.Equal(t, `{"ok":true}`, string(results[0].Result))
	messages := runtime.ToMessages(results)
	require.Len(t, messages, 1)
	assert.Equal(t, types.RoleTool, messages[0].Role)
}

func TestToolProtocolRuntime_PrepareWrapsRuntimeHandoffExecutor(t *testing.T) {
	source := NewBaseAgent(testAgentConfig("source-agent", "Source", "gpt-4"), testGatewayFromProvider(&testProvider{
		name:           "source",
		supportsNative: true,
	}), nil, nil, nil, zap.NewNop(), nil)
	target := NewBaseAgent(testAgentConfig("target-agent", "Target", "gpt-4"), testGatewayFromProvider(&testProvider{
		name:           "target",
		supportsNative: true,
	}), nil, nil, nil, zap.NewNop(), nil)
	pr := &preparedRequest{
		handoffTools: map[string]RuntimeHandoffTarget{
			"transfer_to_target_agent": {
				Agent:       target,
				ToolName:    "transfer_to_target_agent",
				Description: "handoff",
			},
		},
	}
	prepared := NewDefaultToolProtocolRuntime().Prepare(source, pr)
	require.NotNil(t, prepared)
	_, ok := prepared.Executor.(*runtimeHandoffExecutor)
	assert.True(t, ok)
}

func TestRemoteToolTransport_HTTP(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"status":"ok"}}`))
	}))
	defer server.Close()

	transport := NewDefaultRemoteToolTransport(zap.NewNop())
	result, err := transport.Invoke(context.Background(), RemoteToolTarget{
		Kind:     RemoteToolTargetHTTP,
		Endpoint: server.URL,
	}, ToolInvocationRequest{
		ToolName:  "search",
		Arguments: json.RawMessage(`{"query":"golang"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, "search", received["tool_name"])
	assert.Equal(t, `{"status":"ok"}`, string(result.Result))
}

func TestRemoteToolTransport_MCP(t *testing.T) {
	caller := &remoteMCPCallerStub{result: map[string]any{"value": 42}}
	transport := NewDefaultRemoteToolTransport(zap.NewNop())
	result, err := transport.Invoke(context.Background(), RemoteToolTarget{
		Kind:      RemoteToolTargetMCP,
		ToolName:  "echo",
		MCPClient: caller,
	}, ToolInvocationRequest{
		Arguments: json.RawMessage(`{"msg":"hello"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, "echo", caller.lastName)
	assert.Equal(t, "hello", caller.lastArgs["msg"])
	assert.Equal(t, `{"value":42}`, string(result.Result))
}

func TestRemoteToolTransport_A2A(t *testing.T) {
	client := &remoteA2AClientStub{
		resp: map[string]any{"result": map[string]any{"ok": true}},
	}
	transport := NewDefaultRemoteToolTransport(zap.NewNop())
	result, err := transport.Invoke(context.Background(), RemoteToolTarget{
		Kind:      RemoteToolTargetA2A,
		Endpoint:  "https://agent.example.com",
		AgentID:   "local-agent",
		A2ASender: client,
	}, ToolInvocationRequest{
		ToolName:  "delegate",
		Arguments: json.RawMessage(`{"input":"hello"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, "https://agent.example.com", client.endpoint)
	assert.Equal(t, "local-agent", client.fromID)
	assert.Equal(t, "delegate", client.payload["tool_name"])
	assert.Equal(t, `{"ok":true}`, string(result.Result))
}

func TestRemoteToolTransport_StdioUsesTransportFactory(t *testing.T) {
	transport := NewDefaultRemoteToolTransport(zap.NewNop())
	result, err := transport.Invoke(context.Background(), RemoteToolTarget{
		Kind:     RemoteToolTargetStdio,
		ToolName: "echo",
		TransportFactory: func(context.Context, RemoteToolTarget) (mcpproto.Transport, error) {
			return mcpproto.NewInProcessTransport(&inProcessMCPHandlerStub{}), nil
		},
	}, ToolInvocationRequest{})
	require.NoError(t, err)
	assert.Equal(t, `{"output":"echo::echo"}`, string(result.Result))
}
