package runtime

import (
	"context"
	"testing"

	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPrepareRuntimePromptContextUsesAssemblerWithRetrievedContext(t *testing.T) {
	manager := &promptContextCapturingManager{}
	agent := newPromptContextTestAgent()
	agent.contextManager = manager
	agent.retriever = promptContextRetrievalProvider{records: []types.RetrievalRecord{{
		DocID:   "doc-1",
		Content: "retrieved context",
		Source:  "kb",
		Score:   0.9,
	}}}
	agent.toolState = promptContextToolStateProvider{snapshots: []types.ToolStateSnapshot{{
		ToolName:   "search",
		Summary:    "tool state summary",
		ArtifactID: "artifact-1",
	}}}

	input := &Input{
		TraceID:   "trace-1",
		ChannelID: "thread-1",
		Content:   "question",
		Variables: map[string]string{
			"name": "assistant",
		},
		Context: map[string]any{
			"memory_context": []string{"memory item"},
			"skill_context":  []string{"skill item"},
			"public_note":    "visible",
		},
	}
	restored := []types.Message{{Role: types.RoleAssistant, Content: "prior answer"}}

	result := agent.prepareRuntimePromptContext(context.Background(), input, NewPromptBundleFromIdentity("v1", "You are {{name}}."), restored)

	require.NotNil(t, manager.request)
	assert.Equal(t, []types.Message{{Role: types.RoleAssistant, Content: "assembled"}}, result.messages)
	assert.Equal(t, "question", manager.request.UserInput)
	assert.Contains(t, manager.request.SystemPrompt, "You are assistant.")
	assert.Contains(t, manager.request.SystemPrompt, "public_note")
	assert.Equal(t, []string{"memory item"}, manager.request.MemoryContext)
	assert.Equal(t, []string{"skill item"}, manager.request.SkillContext)
	assert.Equal(t, restored, manager.request.Conversation)
	require.Len(t, manager.request.Retrieval, 1)
	assert.Equal(t, "retrieved context", manager.request.Retrieval[0].Content)
	require.Len(t, manager.request.ToolState, 1)
	assert.Equal(t, "tool state summary", manager.request.ToolState[0].Summary)
}

func TestPrepareRuntimePromptContextPrefersHandoffConversation(t *testing.T) {
	agent := newPromptContextTestAgent()
	handoff := []types.Message{{Role: types.RoleUser, Content: "handoff user"}}
	input := &Input{
		Content: "continue",
		Context: map[string]any{
			internalContextHandoffMessages: handoff,
			"memory_context":               []string{"memory item"},
			"skill_context":                []string{"skill item"},
		},
	}
	restored := []types.Message{{Role: types.RoleAssistant, Content: "restored should be ignored"}}

	result := agent.prepareRuntimePromptContext(context.Background(), input, NewPromptBundleFromIdentity("v1", "system"), restored)

	require.Len(t, result.messages, 5)
	assert.Equal(t, types.RoleSystem, result.messages[0].Role)
	assert.Equal(t, "skill item", result.messages[1].Content)
	assert.Equal(t, "memory item", result.messages[2].Content)
	assert.Equal(t, handoff[0], result.messages[3])
	assert.Equal(t, types.Message{Role: types.RoleUser, Content: "continue"}, result.messages[4])
}

func newPromptContextTestAgent() *BaseAgent {
	return &BaseAgent{
		config: types.AgentConfig{
			Core: types.CoreConfig{
				ID:   "agent-1",
				Name: "Agent 1",
				Type: string(TypeAssistant),
			},
		},
		logger: zap.NewNop(),
	}
}

type promptContextCapturingManager struct {
	request *agentcontext.AssembleRequest
}

func (m *promptContextCapturingManager) PrepareMessages(_ context.Context, messages []types.Message, _ string) ([]types.Message, error) {
	return messages, nil
}

func (m *promptContextCapturingManager) GetStatus([]types.Message) agentcontext.Status {
	return agentcontext.Status{UsageRatio: 0.2, Level: agentcontext.LevelNormal}
}

func (m *promptContextCapturingManager) EstimateTokens([]types.Message) int {
	return 0
}

func (m *promptContextCapturingManager) Assemble(_ context.Context, request *agentcontext.AssembleRequest) (*agentcontext.AssembleResult, error) {
	copied := *request
	copied.SkillContext = append([]string(nil), request.SkillContext...)
	copied.MemoryContext = append([]string(nil), request.MemoryContext...)
	copied.Conversation = append([]types.Message(nil), request.Conversation...)
	copied.Retrieval = append([]agentcontext.RetrievalItem(nil), request.Retrieval...)
	copied.ToolState = append([]agentcontext.ToolState(nil), request.ToolState...)
	m.request = &copied
	return &agentcontext.AssembleResult{
		Messages: []types.Message{{Role: types.RoleAssistant, Content: "assembled"}},
		Plan: agentcontext.ContextPlan{
			Strategy: "captured",
		},
	}, nil
}

type promptContextRetrievalProvider struct {
	records []types.RetrievalRecord
}

func (p promptContextRetrievalProvider) Retrieve(context.Context, string, int) ([]types.RetrievalRecord, error) {
	return append([]types.RetrievalRecord(nil), p.records...), nil
}

type promptContextToolStateProvider struct {
	snapshots []types.ToolStateSnapshot
}

func (p promptContextToolStateProvider) LoadToolState(context.Context, string) ([]types.ToolStateSnapshot, error) {
	return append([]types.ToolStateSnapshot(nil), p.snapshots...), nil
}
