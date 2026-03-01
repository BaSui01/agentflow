package collaboration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- mock MessageStore ---

type mockMessageStore struct {
	saveFn      func(ctx context.Context, msg *persistence.Message) error
	savesFn     func(ctx context.Context, msgs []*persistence.Message) error
	getFn       func(ctx context.Context, id string) (*persistence.Message, error)
	getsFn      func(ctx context.Context, topic, cursor string, limit int) ([]*persistence.Message, string, error)
	ackFn       func(ctx context.Context, id string) error
	unackedFn   func(ctx context.Context, topic string, older time.Duration) ([]*persistence.Message, error)
	pendingFn   func(ctx context.Context, topic string, limit int) ([]*persistence.Message, error)
	incrRetryFn func(ctx context.Context, id string) error
	deleteFn    func(ctx context.Context, id string) error
	cleanupFn   func(ctx context.Context, older time.Duration) (int, error)
	statsFn     func(ctx context.Context) (*persistence.MessageStoreStats, error)
	closeFn     func() error
	pingFn      func(ctx context.Context) error
}

func (m *mockMessageStore) SaveMessage(ctx context.Context, msg *persistence.Message) error {
	if m.saveFn != nil {
		return m.saveFn(ctx, msg)
	}
	return nil
}
func (m *mockMessageStore) SaveMessages(ctx context.Context, msgs []*persistence.Message) error {
	if m.savesFn != nil {
		return m.savesFn(ctx, msgs)
	}
	return nil
}
func (m *mockMessageStore) GetMessage(ctx context.Context, id string) (*persistence.Message, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}
func (m *mockMessageStore) GetMessages(ctx context.Context, topic, cursor string, limit int) ([]*persistence.Message, string, error) {
	if m.getsFn != nil {
		return m.getsFn(ctx, topic, cursor, limit)
	}
	return nil, "", nil
}
func (m *mockMessageStore) AckMessage(ctx context.Context, id string) error {
	if m.ackFn != nil {
		return m.ackFn(ctx, id)
	}
	return nil
}
func (m *mockMessageStore) GetUnackedMessages(ctx context.Context, topic string, older time.Duration) ([]*persistence.Message, error) {
	if m.unackedFn != nil {
		return m.unackedFn(ctx, topic, older)
	}
	return nil, nil
}
func (m *mockMessageStore) GetPendingMessages(ctx context.Context, topic string, limit int) ([]*persistence.Message, error) {
	if m.pendingFn != nil {
		return m.pendingFn(ctx, topic, limit)
	}
	return nil, nil
}
func (m *mockMessageStore) IncrementRetry(ctx context.Context, id string) error {
	if m.incrRetryFn != nil {
		return m.incrRetryFn(ctx, id)
	}
	return nil
}
func (m *mockMessageStore) DeleteMessage(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
func (m *mockMessageStore) Cleanup(ctx context.Context, older time.Duration) (int, error) {
	if m.cleanupFn != nil {
		return m.cleanupFn(ctx, older)
	}
	return 0, nil
}
func (m *mockMessageStore) Stats(ctx context.Context) (*persistence.MessageStoreStats, error) {
	if m.statsFn != nil {
		return m.statsFn(ctx)
	}
	return &persistence.MessageStoreStats{}, nil
}
func (m *mockMessageStore) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}
func (m *mockMessageStore) Ping(ctx context.Context) error {
	if m.pingFn != nil {
		return m.pingFn(ctx)
	}
	return nil
}

// --- NewMessageHubWithStore ---

func TestMessageHub_NewWithStore(t *testing.T) {
	store := &mockMessageStore{}
	hub := NewMessageHubWithStore(zap.NewNop(), store)
	defer hub.Close()
	assert.NotNil(t, hub)
}

// --- SetMessageStore ---

func TestMessageHub_SetMessageStore(t *testing.T) {
	hub := NewMessageHub(zap.NewNop())
	defer hub.Close()
	hub.SetMessageStore(nil)
}

// --- Stats ---

func TestMessageHub_Stats_NoStore(t *testing.T) {
	hub := NewMessageHub(zap.NewNop())
	defer hub.Close()
	_, err := hub.Stats(context.Background())
	assert.Error(t, err)
}

func TestMessageHub_Stats_WithStore(t *testing.T) {
	store := &mockMessageStore{
		statsFn: func(ctx context.Context) (*persistence.MessageStoreStats, error) {
			return &persistence.MessageStoreStats{TotalMessages: 42}, nil
		},
	}
	hub := NewMessageHubWithStore(zap.NewNop(), store)
	defer hub.Close()

	stats, err := hub.Stats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(42), stats.TotalMessages)
}

// --- SendWithContext with store (persist + ack paths) ---

func TestMessageHub_SendWithContext_WithStore(t *testing.T) {
	saved := false
	store := &mockMessageStore{
		saveFn: func(ctx context.Context, msg *persistence.Message) error {
			saved = true
			return nil
		},
	}
	hub := NewMessageHubWithStore(zap.NewNop(), store)
	defer hub.Close()

	hub.CreateChannel("agent1")
	err := hub.SendWithContext(context.Background(), &Message{
		FromID:  "agent2",
		ToID:    "agent1",
		Type:    MessageTypeProposal,
		Content: "hello",
	})
	require.NoError(t, err)
	assert.True(t, saved)
	// Wait for async ack goroutine
	time.Sleep(50 * time.Millisecond)
}

func TestMessageHub_SendWithContext_PersistError(t *testing.T) {
	store := &mockMessageStore{
		saveFn: func(ctx context.Context, msg *persistence.Message) error {
			return fmt.Errorf("persist error")
		},
	}
	hub := NewMessageHubWithStore(zap.NewNop(), store)
	defer hub.Close()

	hub.CreateChannel("agent1")
	// Should still deliver even if persist fails
	err := hub.SendWithContext(context.Background(), &Message{
		FromID: "agent2",
		ToID:   "agent1",
		Type:   MessageTypeProposal,
	})
	require.NoError(t, err)
}

func TestMessageHub_SendWithContext_Closed(t *testing.T) {
	hub := NewMessageHub(zap.NewNop())
	hub.CreateChannel("agent1")
	hub.Close()

	err := hub.SendWithContext(context.Background(), &Message{
		FromID: "agent2",
		ToID:   "agent1",
		Type:   MessageTypeProposal,
	})
	assert.Error(t, err)
}

// --- Broadcast with store ---

func TestMessageHub_Broadcast(t *testing.T) {
	hub := NewMessageHub(zap.NewNop())
	defer hub.Close()

	hub.CreateChannel("agent1")
	hub.CreateChannel("agent2")

	err := hub.Send(&Message{
		FromID:  "agent1",
		ToID:    "",
		Type:    MessageTypeProposal,
		Content: "hello all",
	})
	assert.NoError(t, err)
}

func TestMessageHub_Broadcast_WithStore(t *testing.T) {
	store := &mockMessageStore{}
	hub := NewMessageHubWithStore(zap.NewNop(), store)
	defer hub.Close()

	hub.CreateChannel("agent1")
	hub.CreateChannel("agent2")

	err := hub.Send(&Message{
		FromID:  "agent1",
		ToID:    "",
		Type:    MessageTypeBroadcast,
		Content: "broadcast",
	})
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
}

// --- ackMessage ---

func TestMessageHub_AckMessage_Closed(t *testing.T) {
	store := &mockMessageStore{}
	hub := NewMessageHubWithStore(zap.NewNop(), store)
	hub.Close()
	// Should return early without error since hub is closed
	hub.ackMessage(context.Background(), "msg1")
}

func TestMessageHub_AckMessage_NilStore(t *testing.T) {
	hub := NewMessageHub(zap.NewNop())
	defer hub.Close()
	hub.ackMessage(context.Background(), "msg1")
}

func TestMessageHub_AckMessage_Error(t *testing.T) {
	store := &mockMessageStore{
		ackFn: func(ctx context.Context, id string) error {
			return fmt.Errorf("ack error")
		},
	}
	hub := NewMessageHubWithStore(zap.NewNop(), store)
	defer hub.Close()
	hub.ackMessage(context.Background(), "msg1")
}

// --- RecoverMessages with store ---

func TestMessageHub_RecoverMessages_NoStore(t *testing.T) {
	hub := NewMessageHub(zap.NewNop())
	defer hub.Close()
	err := hub.RecoverMessages(context.Background())
	assert.NoError(t, err)
}

func TestMessageHub_RecoverMessages_WithStore(t *testing.T) {
	retryConfig := persistence.DefaultRetryConfig()
	store := &mockMessageStore{
		unackedFn: func(ctx context.Context, topic string, older time.Duration) ([]*persistence.Message, error) {
			return []*persistence.Message{
				{
					ID:         "msg1",
					FromID:     "agent2",
					ToID:       topic,
					Type:       "proposal",
					Content:    "recover me",
					CreatedAt:  time.Now().Add(-10 * time.Minute),
					RetryCount: 0,
				},
			}, nil
		},
		incrRetryFn: func(ctx context.Context, id string) error {
			return nil
		},
	}
	_ = retryConfig

	hub := NewMessageHubWithStore(zap.NewNop(), store)
	defer hub.Close()

	hub.CreateChannel("agent1")

	err := hub.RecoverMessages(context.Background())
	require.NoError(t, err)
}

func TestMessageHub_RecoverMessages_UnackedError(t *testing.T) {
	store := &mockMessageStore{
		unackedFn: func(ctx context.Context, topic string, older time.Duration) ([]*persistence.Message, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	hub := NewMessageHubWithStore(zap.NewNop(), store)
	defer hub.Close()

	hub.CreateChannel("agent1")
	err := hub.RecoverMessages(context.Background())
	assert.NoError(t, err) // errors are logged, not returned
}

func TestMessageHub_RecoverMessages_MaxRetries(t *testing.T) {
	store := &mockMessageStore{
		unackedFn: func(ctx context.Context, topic string, older time.Duration) ([]*persistence.Message, error) {
			return []*persistence.Message{
				{
					ID:         "msg1",
					RetryCount: 999, // exceeds max retries
					CreatedAt:  time.Now(),
				},
			}, nil
		},
	}
	hub := NewMessageHubWithStore(zap.NewNop(), store)
	defer hub.Close()

	hub.CreateChannel("agent1")
	err := hub.RecoverMessages(context.Background())
	assert.NoError(t, err)
}

func TestMessageHub_RecoverMessages_IncrRetryError(t *testing.T) {
	store := &mockMessageStore{
		unackedFn: func(ctx context.Context, topic string, older time.Duration) ([]*persistence.Message, error) {
			return []*persistence.Message{
				{
					ID:         "msg1",
					RetryCount: 0,
					CreatedAt:  time.Now(),
				},
			}, nil
		},
		incrRetryFn: func(ctx context.Context, id string) error {
			return fmt.Errorf("incr error")
		},
	}
	hub := NewMessageHubWithStore(zap.NewNop(), store)
	defer hub.Close()

	hub.CreateChannel("agent1")
	err := hub.RecoverMessages(context.Background())
	assert.NoError(t, err)
}

// --- StartRetryLoop ---

func TestMessageHub_StartRetryLoop_NoStore(t *testing.T) {
	hub := NewMessageHub(zap.NewNop())
	defer hub.Close()
	ctx, cancel := context.WithCancel(context.Background())
	hub.StartRetryLoop(ctx, time.Second)
	cancel()
}

func TestMessageHub_StartRetryLoop_WithStore(t *testing.T) {
	recoverCalled := make(chan struct{}, 5)
	store := &mockMessageStore{
		unackedFn: func(ctx context.Context, topic string, older time.Duration) ([]*persistence.Message, error) {
			select {
			case recoverCalled <- struct{}{}:
			default:
			}
			return nil, nil
		},
	}
	hub := NewMessageHubWithStore(zap.NewNop(), store)
	defer hub.Close()

	hub.CreateChannel("agent1")

	ctx, cancel := context.WithCancel(context.Background())
	hub.StartRetryLoop(ctx, 50*time.Millisecond)

	// Wait for at least one tick
	select {
	case <-recoverCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("retry loop did not trigger")
	}
	cancel()
}

// --- RolePipeline ---

func TestRolePipeline_GetInstances(t *testing.T) {
	registry := NewRoleRegistry(zap.NewNop())
	require.NoError(t, registry.Register(&RoleDefinition{
		Type: "worker",
		Name: "Worker",
	}))
	config := PipelineConfig{
		Name:           "test",
		MaxConcurrency: 2,
	}
	pipeline := NewRolePipeline(config, registry, nil, zap.NewNop())
	instances := pipeline.GetInstances()
	assert.Empty(t, instances)
}

func TestRolePipeline_GetTransitions(t *testing.T) {
	registry := NewRoleRegistry(zap.NewNop())
	config := PipelineConfig{
		Name:           "test",
		MaxConcurrency: 2,
	}
	pipeline := NewRolePipeline(config, registry, nil, zap.NewNop())
	transitions := pipeline.GetTransitions()
	assert.Empty(t, transitions)
}

func TestRolePipeline_Execute_WithRetry(t *testing.T) {
	registry := NewRoleRegistry(zap.NewNop())
	require.NoError(t, registry.Register(&RoleDefinition{
		Type:    "worker",
		Name:    "Worker",
		Timeout: 5 * time.Second,
		RetryPolicy: &RetryPolicy{
			MaxRetries: 1,
			Delay:      time.Millisecond,
		},
	}))

	callCount := 0
	executeFn := func(ctx context.Context, def *RoleDefinition, input any) (any, error) {
		callCount++
		if callCount == 1 {
			return nil, assert.AnError
		}
		return "success", nil
	}

	config := PipelineConfig{
		Name:           "test",
		MaxConcurrency: 2,
	}
	pipeline := NewRolePipeline(config, registry, executeFn, zap.NewNop())
	pipeline.AddStage("worker")

	results, err := pipeline.Execute(context.Background(), "input")
	require.NoError(t, err)
	assert.NotEmpty(t, results)
}

func TestRolePipeline_Execute_RoleNotFound(t *testing.T) {
	registry := NewRoleRegistry(zap.NewNop())
	executeFn := func(ctx context.Context, def *RoleDefinition, input any) (any, error) {
		return "ok", nil
	}
	config := PipelineConfig{
		Name:           "test",
		MaxConcurrency: 2,
	}
	pipeline := NewRolePipeline(config, registry, executeFn, zap.NewNop())
	pipeline.AddStage("nonexistent")

	results, err := pipeline.Execute(context.Background(), "input")
	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestRolePipeline_Execute_WithDependencies(t *testing.T) {
	registry := NewRoleRegistry(zap.NewNop())
	require.NoError(t, registry.Register(&RoleDefinition{
		Type: "analyzer",
		Name: "Analyzer",
	}))
	require.NoError(t, registry.Register(&RoleDefinition{
		Type:         "writer",
		Name:         "Writer",
		Dependencies: []RoleType{"analyzer"},
	}))

	executeFn := func(ctx context.Context, def *RoleDefinition, input any) (any, error) {
		return fmt.Sprintf("%s_output", def.Type), nil
	}

	config := PipelineConfig{
		Name:           "test",
		MaxConcurrency: 2,
	}
	pipeline := NewRolePipeline(config, registry, executeFn, zap.NewNop())
	pipeline.AddStage("analyzer")
	pipeline.AddStage("writer")

	results, err := pipeline.Execute(context.Background(), "input")
	require.NoError(t, err)
	assert.NotEmpty(t, results)
}

// --- Close with store ---

func TestMessageHub_Close_WithStore(t *testing.T) {
	closeCalled := false
	store := &mockMessageStore{
		closeFn: func() error {
			closeCalled = true
			return nil
		},
	}
	hub := NewMessageHubWithStore(zap.NewNop(), store)
	hub.CreateChannel("agent1")
	err := hub.Close()
	assert.NoError(t, err)
	assert.True(t, closeCalled)

	// Double close should be safe
	err = hub.Close()
	assert.NoError(t, err)
}

