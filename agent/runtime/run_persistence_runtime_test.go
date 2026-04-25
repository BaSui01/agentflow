package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRuntimePersistenceSessionRecordsRestoresAndCompletesRun(t *testing.T) {
	ctx := context.Background()
	runStore := &runtimePersistenceRunStore{}
	conversationStore := &runtimePersistenceConversationStore{
		messages: map[string][]ConversationMessage{
			"thread-1": {
				{Role: string(types.RoleUser), Content: "before"},
				{Role: string(types.RoleAssistant), Content: "after"},
			},
		},
	}
	agent := newRuntimePersistenceTestAgent(runStore, conversationStore)
	startTime := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	input := &Input{
		TraceID:   "trace-1",
		TenantID:  "tenant-1",
		UserID:    "user-1",
		ChannelID: "thread-1",
		Content:   "hello",
	}

	session := agent.beginRuntimePersistence(ctx, input, startTime)

	require.NotNil(t, runStore.recorded)
	assert.Equal(t, "agent-1", runStore.recorded.AgentID)
	assert.Equal(t, "tenant-1", runStore.recorded.TenantID)
	assert.Equal(t, "trace-1", runStore.recorded.TraceID)
	assert.Equal(t, "running", runStore.recorded.Status)
	assert.Equal(t, "hello", runStore.recorded.Input)
	assert.Equal(t, startTime, runStore.recorded.StartTime)
	assert.NotEmpty(t, session.runID)
	assert.Equal(t, "thread-1", session.conversationID)
	assert.Equal(t, []types.Message{
		{Role: types.RoleUser, Content: "before"},
		{Role: types.RoleAssistant, Content: "after"},
	}, session.restoredMessages)

	agent.completeRuntimePersistence(ctx, session, input, runtimePersistenceCompletion{
		outputContent: "world",
		tokensUsed:    42,
		cost:          0.25,
		finishReason:  "stop",
	})

	require.Len(t, conversationStore.appended, 1)
	assert.Equal(t, "thread-1", conversationStore.appended[0].conversationID)
	assert.Equal(t, []ConversationMessage{
		{Role: string(types.RoleUser), Content: "hello", Timestamp: conversationStore.appended[0].messages[0].Timestamp},
		{Role: string(types.RoleAssistant), Content: "world", Timestamp: conversationStore.appended[0].messages[1].Timestamp},
	}, conversationStore.appended[0].messages)

	require.Len(t, runStore.updates, 1)
	assert.Equal(t, session.runID, runStore.updates[0].id)
	assert.Equal(t, "completed", runStore.updates[0].status)
	require.NotNil(t, runStore.updates[0].output)
	assert.Equal(t, "world", runStore.updates[0].output.Content)
	assert.Equal(t, 42, runStore.updates[0].output.TokensUsed)
	assert.Equal(t, 0.25, runStore.updates[0].output.Cost)
	assert.Equal(t, "stop", runStore.updates[0].output.FinishReason)
	assert.Empty(t, runStore.updates[0].errMsg)
}

func TestFinishRuntimePersistenceOnExitMarksRunFailedForReturnedError(t *testing.T) {
	runStore := &runtimePersistenceRunStore{}
	agent := newRuntimePersistenceTestAgent(runStore, nil)
	session := runtimePersistenceSession{runID: "run-1"}
	execErr := errors.New("execution failed")

	agent.finishRuntimePersistenceOnExit(context.Background(), session, &execErr)

	require.Len(t, runStore.updates, 1)
	assert.Equal(t, "run-1", runStore.updates[0].id)
	assert.Equal(t, "failed", runStore.updates[0].status)
	assert.Equal(t, "execution failed", runStore.updates[0].errMsg)
}

func TestFinishRuntimePersistenceOnExitMarksRunFailedOnPanic(t *testing.T) {
	runStore := &runtimePersistenceRunStore{}
	agent := newRuntimePersistenceTestAgent(runStore, nil)
	session := runtimePersistenceSession{runID: "run-1"}
	var execErr error

	func() {
		defer agent.finishRuntimePersistenceOnExit(context.Background(), session, &execErr)
		panic("boom")
	}()

	require.Error(t, execErr)
	assert.Contains(t, execErr.Error(), "react execution panic")
	require.Len(t, runStore.updates, 2)
	assert.Equal(t, "failed", runStore.updates[0].status)
	assert.Equal(t, "panic: boom", runStore.updates[0].errMsg)
	assert.Equal(t, "failed", runStore.updates[1].status)
	assert.Contains(t, runStore.updates[1].errMsg, "react execution panic")
}

func newRuntimePersistenceTestAgent(runStore RunStoreProvider, conversationStore ConversationStoreProvider) *BaseAgent {
	stores := NewPersistenceStores(zap.NewNop())
	stores.SetRunStore(runStore)
	stores.SetConversationStore(conversationStore)
	return &BaseAgent{
		config: types.AgentConfig{
			Core: types.CoreConfig{
				ID:   "agent-1",
				Name: "Agent 1",
				Type: string(TypeAssistant),
			},
		},
		logger:      zap.NewNop(),
		persistence: stores,
	}
}

type runtimePersistenceRunStore struct {
	recorded *RunDoc
	updates  []runtimePersistenceRunUpdate
}

type runtimePersistenceRunUpdate struct {
	id     string
	status string
	output *RunOutputDoc
	errMsg string
}

func (s *runtimePersistenceRunStore) RecordRun(_ context.Context, doc *RunDoc) error {
	copied := *doc
	s.recorded = &copied
	return nil
}

func (s *runtimePersistenceRunStore) UpdateStatus(_ context.Context, id, status string, output *RunOutputDoc, errMsg string) error {
	var copied *RunOutputDoc
	if output != nil {
		out := *output
		copied = &out
	}
	s.updates = append(s.updates, runtimePersistenceRunUpdate{
		id:     id,
		status: status,
		output: copied,
		errMsg: errMsg,
	})
	return nil
}

type runtimePersistenceConversationStore struct {
	messages map[string][]ConversationMessage
	appended []runtimePersistenceAppend
	created  *ConversationDoc
}

type runtimePersistenceAppend struct {
	conversationID string
	messages       []ConversationMessage
}

func (s *runtimePersistenceConversationStore) Create(_ context.Context, doc *ConversationDoc) error {
	copied := *doc
	copied.Messages = append([]ConversationMessage(nil), doc.Messages...)
	s.created = &copied
	return nil
}

func (s *runtimePersistenceConversationStore) GetByID(context.Context, string) (*ConversationDoc, error) {
	return nil, nil
}

func (s *runtimePersistenceConversationStore) AppendMessages(_ context.Context, conversationID string, msgs []ConversationMessage) error {
	s.appended = append(s.appended, runtimePersistenceAppend{
		conversationID: conversationID,
		messages:       append([]ConversationMessage(nil), msgs...),
	})
	return nil
}

func (s *runtimePersistenceConversationStore) List(context.Context, string, string, int, int) ([]*ConversationDoc, int64, error) {
	return nil, 0, nil
}

func (s *runtimePersistenceConversationStore) Update(context.Context, string, ConversationUpdate) error {
	return nil
}

func (s *runtimePersistenceConversationStore) Delete(context.Context, string) error {
	return nil
}

func (s *runtimePersistenceConversationStore) DeleteByParentID(context.Context, string, string) error {
	return nil
}

func (s *runtimePersistenceConversationStore) GetMessages(_ context.Context, conversationID string, offset, limit int) ([]ConversationMessage, int64, error) {
	messages := append([]ConversationMessage(nil), s.messages[conversationID]...)
	total := len(messages)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return append([]ConversationMessage(nil), messages[offset:end]...), int64(total), nil
}

func (s *runtimePersistenceConversationStore) DeleteMessage(context.Context, string, string) error {
	return nil
}

func (s *runtimePersistenceConversationStore) ClearMessages(context.Context, string) error {
	return nil
}

func (s *runtimePersistenceConversationStore) Archive(context.Context, string) error {
	return nil
}
