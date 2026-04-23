package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestClaudeProvider_CountTokens(t *testing.T) {
	var capturedPath string
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"input_tokens":                21,
			"cache_creation_input_tokens": 3,
			"cache_read_input_tokens":     5,
		})
	}))
	t.Cleanup(server.Close)

	p := NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL},
	}, zap.NewNop())

	resp, err := p.CountTokens(context.Background(), &llm.ChatRequest{
		Model: "claude-opus-4.5-20260105",
		Messages: []types.Message{
			{Role: llm.RoleSystem, Content: "You are helpful."},
			{Role: llm.RoleUser, Content: "hello"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "/v1/messages/count_tokens", capturedPath)
	assert.Equal(t, 21, resp.InputTokens)
	systemBlocks, ok := capturedBody["system"].([]any)
	require.True(t, ok)
	require.Len(t, systemBlocks, 1)
	sysText, _ := systemBlocks[0].(map[string]any)["text"].(string)
	assert.Equal(t, "You are helpful.", sysText)
}
