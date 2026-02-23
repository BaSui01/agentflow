package mcp

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewMCPServer(t *testing.T) {
	server := NewMCPServer("test-server", "1.0.0", zap.NewNop())
	require.NotNil(t, server)
}

func TestNewMCPClient(t *testing.T) {
	reader := &bytes.Buffer{}
	writer := &bytes.Buffer{}
	client := NewMCPClient(reader, writer, zap.NewNop())
	require.NotNil(t, client)
}

func TestNewMCPRequest(t *testing.T) {
	msg := NewMCPRequest(1, "tools/list", map[string]any{"cursor": "abc"})
	require.NotNil(t, msg)
	assert.Equal(t, "2.0", msg.JSONRPC)
	assert.Equal(t, "tools/list", msg.Method)
	assert.NotNil(t, msg.Params)
}

func TestNewMCPResponse(t *testing.T) {
	msg := NewMCPResponse(1, map[string]any{"tools": []string{}})
	require.NotNil(t, msg)
	assert.Equal(t, "2.0", msg.JSONRPC)
	assert.NotNil(t, msg.Result)
	assert.Nil(t, msg.Error)
}

func TestNewMCPError(t *testing.T) {
	msg := NewMCPError(1, -32600, "invalid request", nil)
	require.NotNil(t, msg)
	assert.Equal(t, "2.0", msg.JSONRPC)
	assert.NotNil(t, msg.Error)
	assert.Equal(t, -32600, msg.Error.Code)
	assert.Equal(t, "invalid request", msg.Error.Message)
}

func TestMCPVersion(t *testing.T) {
	assert.NotEmpty(t, MCPVersion)
	assert.Equal(t, "2024-11-05", MCPVersion)
}
