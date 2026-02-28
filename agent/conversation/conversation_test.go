package conversation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- mock ConversationAgent ---

type mockAgent struct {
	id              string
	name            string
	systemPrompt    string
	replyFn         func(ctx context.Context, msgs []ChatMessage) (*ChatMessage, error)
	shouldTerminate bool
}

func (a *mockAgent) ID() string           { return a.id }
func (a *mockAgent) Name() string         { return a.name }
func (a *mockAgent) SystemPrompt() string { return a.systemPrompt }

func (a *mockAgent) Reply(ctx context.Context, msgs []ChatMessage) (*ChatMessage, error) {
	if a.replyFn != nil {
		return a.replyFn(ctx, msgs)
	}
	return &ChatMessage{
		Role:      "assistant",
		Content:   fmt.Sprintf("reply from %s", a.name),
		Timestamp: time.Now(),
	}, nil
}

func (a *mockAgent) ShouldTerminate(msgs []ChatMessage) bool {
	return a.shouldTerminate
}

// --- ConversationTree tests ---

func TestNewConversationTree(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("tree-1")
	require.NotNil(t, tree)
	assert.Equal(t, "tree-1", tree.ID)
	assert.Equal(t, "main", tree.ActiveBranch)
	assert.NotNil(t, tree.RootState)
}

func TestConversationTree_AddMessage(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")

	state := tree.AddMessage(llm.Message{Role: "user", Content: "hello"})
	require.NotNil(t, state)
	assert.Len(t, state.Messages, 1)
	assert.Equal(t, "hello", state.Messages[0].Content)

	state2 := tree.AddMessage(llm.Message{Role: "assistant", Content: "hi"})
	require.NotNil(t, state2)
	assert.Len(t, state2.Messages, 2)
}

func TestConversationTree_GetCurrentState(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	tree.AddMessage(llm.Message{Role: "user", Content: "hello"})

	state := tree.GetCurrentState()
	require.NotNil(t, state)
	assert.Len(t, state.Messages, 1)
}

func TestConversationTree_GetMessages(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	tree.AddMessage(llm.Message{Role: "user", Content: "hello"})
	tree.AddMessage(llm.Message{Role: "assistant", Content: "hi"})

	msgs := tree.GetMessages()
	assert.Len(t, msgs, 2)
}

func TestConversationTree_Fork(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	tree.AddMessage(llm.Message{Role: "user", Content: "hello"})

	branch, err := tree.Fork("experiment")
	require.NoError(t, err)
	assert.Equal(t, "experiment", branch.ID)
	assert.Len(t, branch.States[0].Messages, 1)
}

func TestConversationTree_Fork_DuplicateName(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	_, err := tree.Fork("main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestConversationTree_SwitchBranch(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	tree.AddMessage(llm.Message{Role: "user", Content: "hello"})
	_, err := tree.Fork("alt")
	require.NoError(t, err)

	require.NoError(t, tree.SwitchBranch("alt"))
	assert.Equal(t, "alt", tree.ActiveBranch)

	assert.Error(t, tree.SwitchBranch("nonexistent"))
}

func TestConversationTree_Rollback(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	tree.AddMessage(llm.Message{Role: "user", Content: "msg1"})
	tree.AddMessage(llm.Message{Role: "user", Content: "msg2"})

	history := tree.GetHistory()
	require.Len(t, history, 3) // root + 2 messages

	require.NoError(t, tree.Rollback(history[1].ID))
	assert.Len(t, tree.GetHistory(), 2)
}

func TestConversationTree_Rollback_NotFound(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	assert.Error(t, tree.Rollback("nonexistent"))
}

func TestConversationTree_RollbackN(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	tree.AddMessage(llm.Message{Role: "user", Content: "msg1"})
	tree.AddMessage(llm.Message{Role: "user", Content: "msg2"})

	require.NoError(t, tree.RollbackN(1))
	assert.Len(t, tree.GetHistory(), 2)
}

func TestConversationTree_RollbackN_TooMany(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	assert.Error(t, tree.RollbackN(5))
}

func TestConversationTree_ListBranches(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	_, _ = tree.Fork("alt1")
	_, _ = tree.Fork("alt2")

	branches := tree.ListBranches()
	assert.Len(t, branches, 3)
	assert.Contains(t, branches, "main")
	assert.Contains(t, branches, "alt1")
	assert.Contains(t, branches, "alt2")
}

func TestConversationTree_DeleteBranch(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	_, _ = tree.Fork("alt")

	require.NoError(t, tree.DeleteBranch("alt"))
	assert.Len(t, tree.ListBranches(), 1)
}

func TestConversationTree_DeleteBranch_Main(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	assert.Error(t, tree.DeleteBranch("main"))
}

func TestConversationTree_DeleteBranch_Active(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	assert.Error(t, tree.DeleteBranch("main"))
}

func TestConversationTree_DeleteBranch_NotFound(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	assert.Error(t, tree.DeleteBranch("nonexistent"))
}

func TestConversationTree_MergeBranch(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	tree.AddMessage(llm.Message{Role: "user", Content: "shared"})

	_, err := tree.Fork("alt")
	require.NoError(t, err)
	require.NoError(t, tree.SwitchBranch("alt"))
	tree.AddMessage(llm.Message{Role: "user", Content: "alt msg"})

	require.NoError(t, tree.SwitchBranch("main"))
	require.NoError(t, tree.MergeBranch("alt"))

	msgs := tree.GetMessages()
	assert.Len(t, msgs, 2)
}

func TestConversationTree_MergeBranch_NotFound(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	assert.Error(t, tree.MergeBranch("nonexistent"))
}

func TestConversationTree_ExportImport(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	tree.AddMessage(llm.Message{Role: "user", Content: "hello"})

	data, err := tree.Export()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	imported, err := Import(data)
	require.NoError(t, err)
	assert.Equal(t, "t1", imported.ID)
}

func TestConversationTree_Snapshot(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	tree.AddMessage(llm.Message{Role: "user", Content: "hello"})

	snap := tree.Snapshot("v1")
	require.NotNil(t, snap)
	assert.Equal(t, "v1", snap.Label)
}

func TestConversationTree_FindSnapshot(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	tree.AddMessage(llm.Message{Role: "user", Content: "hello"})
	tree.Snapshot("v1")

	found := tree.FindSnapshot("v1")
	require.NotNil(t, found)
	assert.Equal(t, "v1", found.Label)

	assert.Nil(t, tree.FindSnapshot("nonexistent"))
}

func TestConversationTree_RestoreSnapshot(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	tree.AddMessage(llm.Message{Role: "user", Content: "msg1"})
	tree.Snapshot("v1")
	tree.AddMessage(llm.Message{Role: "user", Content: "msg2"})

	require.NoError(t, tree.RestoreSnapshot("v1"))
	msgs := tree.GetMessages()
	assert.Len(t, msgs, 1)
}

func TestConversationTree_RestoreSnapshot_NotFound(t *testing.T) {
	t.Parallel()
	tree := NewConversationTree("t1")
	assert.Error(t, tree.RestoreSnapshot("nonexistent"))
}

// --- Conversation mode tests ---

func TestDefaultConversationConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConversationConfig()
	assert.Equal(t, 10, cfg.MaxRounds)
	assert.Equal(t, 50, cfg.MaxMessages)
	assert.True(t, cfg.AllowInterrupts)
	assert.Contains(t, cfg.TerminationWords, "TERMINATE")
}

func TestNewConversation(t *testing.T) {
	t.Parallel()
	agents := []ConversationAgent{
		&mockAgent{id: "a1", name: "Agent1"},
	}
	conv := NewConversation(ModeRoundRobin, agents, DefaultConversationConfig(), nil)
	require.NotNil(t, conv)
	assert.Equal(t, ModeRoundRobin, conv.Mode)
	assert.NotNil(t, conv.Selector)
}

func TestConversation_Start_RoundRobin(t *testing.T) {
	t.Parallel()
	agents := []ConversationAgent{
		&mockAgent{id: "a1", name: "Agent1"},
		&mockAgent{id: "a2", name: "Agent2", shouldTerminate: true},
	}
	cfg := DefaultConversationConfig()
	cfg.MaxRounds = 5
	cfg.Timeout = 5 * time.Second

	conv := NewConversation(ModeRoundRobin, agents, cfg, zap.NewNop())
	result, err := conv.Start(context.Background(), "hello")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Messages)
	assert.Equal(t, "agent_terminated", result.TerminationReason)
}

func TestConversation_Start_MaxRounds(t *testing.T) {
	t.Parallel()
	agents := []ConversationAgent{
		&mockAgent{id: "a1", name: "Agent1"},
	}
	cfg := DefaultConversationConfig()
	cfg.MaxRounds = 3
	cfg.Timeout = 5 * time.Second

	conv := NewConversation(ModeRoundRobin, agents, cfg, zap.NewNop())
	result, err := conv.Start(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "max_rounds", result.TerminationReason)
	assert.Equal(t, 3, result.TotalRounds)
}

func TestConversation_Start_TerminationWord(t *testing.T) {
	t.Parallel()
	agents := []ConversationAgent{
		&mockAgent{
			id: "a1", name: "Agent1",
			replyFn: func(_ context.Context, _ []ChatMessage) (*ChatMessage, error) {
				return &ChatMessage{Role: "assistant", Content: "TERMINATE", Timestamp: time.Now()}, nil
			},
		},
	}
	cfg := DefaultConversationConfig()
	cfg.Timeout = 5 * time.Second

	conv := NewConversation(ModeRoundRobin, agents, cfg, zap.NewNop())
	result, err := conv.Start(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "agent_terminated", result.TerminationReason)
}

func TestConversation_GetMessages(t *testing.T) {
	t.Parallel()
	agents := []ConversationAgent{
		&mockAgent{id: "a1", name: "Agent1", shouldTerminate: true},
	}
	cfg := DefaultConversationConfig()
	cfg.Timeout = 5 * time.Second

	conv := NewConversation(ModeRoundRobin, agents, cfg, zap.NewNop())
	_, _ = conv.Start(context.Background(), "hello")

	msgs := conv.GetMessages()
	assert.NotEmpty(t, msgs)
}

// --- RoundRobinSelector tests ---

func TestRoundRobinSelector_SelectNext(t *testing.T) {
	t.Parallel()
	sel := &RoundRobinSelector{}
	agents := []ConversationAgent{
		&mockAgent{id: "a1"}, &mockAgent{id: "a2"}, &mockAgent{id: "a3"},
	}

	a, err := sel.SelectNext(context.Background(), agents, nil)
	require.NoError(t, err)
	assert.Equal(t, "a1", a.ID())

	a, err = sel.SelectNext(context.Background(), agents, nil)
	require.NoError(t, err)
	assert.Equal(t, "a2", a.ID())

	a, err = sel.SelectNext(context.Background(), agents, nil)
	require.NoError(t, err)
	assert.Equal(t, "a3", a.ID())

	// wraps around
	a, err = sel.SelectNext(context.Background(), agents, nil)
	require.NoError(t, err)
	assert.Equal(t, "a1", a.ID())
}

func TestRoundRobinSelector_NoAgents(t *testing.T) {
	t.Parallel()
	sel := &RoundRobinSelector{}
	_, err := sel.SelectNext(context.Background(), nil, nil)
	assert.Error(t, err)
}

// --- LLMSelector tests ---

func TestLLMSelector_NoLLM_Fallback(t *testing.T) {
	t.Parallel()
	sel := &LLMSelector{}
	agents := []ConversationAgent{
		&mockAgent{id: "a1"}, &mockAgent{id: "a2"},
	}

	a, err := sel.SelectNext(context.Background(), agents, nil)
	require.NoError(t, err)
	assert.NotNil(t, a)
}

func TestLLMSelector_NoAgents(t *testing.T) {
	t.Parallel()
	sel := &LLMSelector{}
	_, err := sel.SelectNext(context.Background(), nil, nil)
	assert.Error(t, err)
}

// --- GroupChatManager tests ---

func TestGroupChatManager_CreateGetChat(t *testing.T) {
	t.Parallel()
	mgr := NewGroupChatManager(zap.NewNop())
	agents := []ConversationAgent{&mockAgent{id: "a1"}}

	conv := mgr.CreateChat(agents, DefaultConversationConfig())
	require.NotNil(t, conv)

	got, ok := mgr.GetChat(conv.ID)
	assert.True(t, ok)
	assert.Equal(t, conv.ID, got.ID)

	_, ok = mgr.GetChat("nonexistent")
	assert.False(t, ok)
}
