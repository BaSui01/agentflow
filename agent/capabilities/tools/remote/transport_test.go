package remote

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDefaultRemoteToolTransport_HTTPInvokesEndpoint(t *testing.T) {
	var capturedBody map[string]any
	transport := NewDefaultRemoteToolTransport(zap.NewNop())
	target := RemoteToolTarget{
		Kind:     RemoteToolTargetHTTP,
		Endpoint: "https://tools.example/invoke",
		ToolName: "search",
		Headers:  map[string]string{"X-Test": "yes"},
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "https://tools.example/invoke", req.URL.String())
			assert.Equal(t, "yes", req.Header.Get("X-Test"))
			raw, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, &capturedBody))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"result":{"ok":true}}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	result, err := transport.Invoke(context.Background(), target, ToolInvocationRequest{
		Arguments: json.RawMessage(`{"query":"agentflow"}`),
		Input:     " ignored whitespace ",
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, string(result.Result))
	assert.Equal(t, "search", capturedBody["tool_name"])
	assert.Equal(t, "ignored whitespace", capturedBody["input"])
}

func TestDefaultRemoteToolTransport_MCPUsesInjectedCaller(t *testing.T) {
	caller := &recordingMCPCaller{}
	transport := NewDefaultRemoteToolTransport(zap.NewNop())

	result, err := transport.Invoke(context.Background(), RemoteToolTarget{
		Kind:      RemoteToolTargetMCP,
		ToolName:  "lookup",
		MCPClient: caller,
	}, ToolInvocationRequest{Arguments: json.RawMessage(`{"id":"42"}`)})

	require.NoError(t, err)
	assert.Equal(t, "lookup", caller.name)
	assert.Equal(t, "42", caller.args["id"])
	assert.JSONEq(t, `{"found":true}`, string(result.Result))
}

type recordingMCPCaller struct {
	name string
	args map[string]any
}

func (c *recordingMCPCaller) CallTool(_ context.Context, name string, args map[string]any) (any, error) {
	c.name = name
	c.args = args
	return map[string]any{"found": true}, nil
}
