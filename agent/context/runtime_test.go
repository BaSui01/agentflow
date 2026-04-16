package context

import (
	"context"
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAgentContextManager_Assemble(t *testing.T) {
	mgr := NewAgentContextManager(DefaultAgentContextConfig("gpt-4o"), zap.NewNop())
	result, err := mgr.Assemble(context.Background(), &AssembleRequest{
		SystemPrompt:  "You are helpful",
		MemoryContext: []string{"user prefers concise answers"},
		Conversation: []types.Message{
			{Role: types.RoleUser, Content: "old question"},
			{Role: types.RoleAssistant, Content: "old answer"},
		},
		UserInput: "new question",
		Query:     "new question",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Messages)
	assert.Equal(t, types.RoleSystem, result.Messages[0].Role)
	assert.Equal(t, types.RoleUser, result.Messages[len(result.Messages)-1].Role)
}

func TestAgentContextManager_SetSummaryProvider_AppliesSummary(t *testing.T) {
	cfg := DefaultAgentContextConfig("gpt-4o")
	cfg.MaxContextTokens = 40
	cfg.ReserveForOutput = 0
	mgr := NewAgentContextManager(cfg, zap.NewNop())
	mgr.SetSummaryProvider(func(ctx context.Context, messages []types.Message) (string, error) {
		return "summary: " + messages[0].Content, nil
	})
	result, err := mgr.Assemble(context.Background(), &AssembleRequest{
		SystemPrompt: "system",
		Conversation: []types.Message{
			{Role: types.RoleUser, Content: strings.Repeat("old ", 80)},
			{Role: types.RoleAssistant, Content: strings.Repeat("older ", 80)},
			{Role: types.RoleUser, Content: "latest"},
		},
		UserInput: "question",
		Query:     "question",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.SegmentsSummarized)
}
