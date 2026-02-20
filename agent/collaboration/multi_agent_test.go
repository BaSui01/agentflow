package collaboration

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Mock Agent
// ---------------------------------------------------------------------------

type mockAgent struct {
	id        string
	name      string
	agentType agent.AgentType
	state     agent.State
	output    *agent.Output
	err       error
	callCount atomic.Int32
}

func newMockAgent(id, name string) *mockAgent {
	return &mockAgent{
		id:        id,
		name:      name,
		agentType: agent.TypeGeneric,
		output: &agent.Output{
			Content: fmt.Sprintf("response from %s", id),
		},
	}
}

func (m *mockAgent) WithOutput(content string) *mockAgent {
	m.output = &agent.Output{Content: content}
	return m
}

func (m *mockAgent) WithError(err error) *mockAgent {
	m.err = err
	return m
}

func (m *mockAgent) ID() string              { return m.id }
func (m *mockAgent) Name() string            { return m.name }
func (m *mockAgent) Type() agent.AgentType   { return m.agentType }
func (m *mockAgent) State() agent.State      { return m.state }
func (m *mockAgent) Init(context.Context) error    { return nil }
func (m *mockAgent) Teardown(context.Context) error { return nil }
func (m *mockAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	return &agent.PlanResult{}, nil
}
func (m *mockAgent) Observe(context.Context, *agent.Feedback) error { return nil }
func (m *mockAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	m.callCount.Add(1)
	if m.err != nil {
		return nil, m.err
	}
	out := *m.output
	out.TraceID = input.TraceID
	return &out, nil
}

// ---------------------------------------------------------------------------
// DefaultMultiAgentConfig
// ---------------------------------------------------------------------------

func TestDefaultMultiAgentConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultMultiAgentConfig()
	assert.Equal(t, PatternDebate, cfg.Pattern)
	assert.Equal(t, 5, cfg.MaxRounds)
	assert.Equal(t, 0.7, cfg.ConsensusThreshold)
	assert.Equal(t, 10*time.Minute, cfg.Timeout)
	assert.True(t, cfg.EnableVoting)
}

// ---------------------------------------------------------------------------
// NewMultiAgentSystem
// ---------------------------------------------------------------------------

func TestNewMultiAgentSystem_CreatesAgentMap(t *testing.T) {
	t.Parallel()
	a1 := newMockAgent("a1", "Agent1")
	a2 := newMockAgent("a2", "Agent2")

	cfg := DefaultMultiAgentConfig()
	sys := NewMultiAgentSystem([]agent.Agent{a1, a2}, cfg, zap.NewNop())

	assert.Len(t, sys.agents, 2)
	assert.Contains(t, sys.agents, "a1")
	assert.Contains(t, sys.agents, "a2")
}

func TestNewMultiAgentSystem_NilLogger(t *testing.T) {
	t.Parallel()
	a1 := newMockAgent("a1", "Agent1")
	cfg := DefaultMultiAgentConfig()
	sys := NewMultiAgentSystem([]agent.Agent{a1}, cfg, nil)
	assert.NotNil(t, sys)
	assert.NotNil(t, sys.logger)
}

func TestNewMultiAgentSystem_PatternSelection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		pattern CollaborationPattern
	}{
		{"debate", PatternDebate},
		{"consensus", PatternConsensus},
		{"pipeline", PatternPipeline},
		{"broadcast", PatternBroadcast},
		{"network", PatternNetwork},
		{"unknown defaults to debate", CollaborationPattern("unknown")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := newMockAgent("a1", "Agent1")
			cfg := DefaultMultiAgentConfig()
			cfg.Pattern = tt.pattern
			sys := NewMultiAgentSystem([]agent.Agent{a}, cfg, zap.NewNop())
			assert.NotNil(t, sys.coordinator)
		})
	}
}

func TestNewMultiAgentSystem_EmptyAgents(t *testing.T) {
	t.Parallel()
	cfg := DefaultMultiAgentConfig()
	sys := NewMultiAgentSystem([]agent.Agent{}, cfg, zap.NewNop())
	assert.Empty(t, sys.agents)
}

// ---------------------------------------------------------------------------
// MessageHub
// ---------------------------------------------------------------------------

func TestMessageHub_CreateChannelAndSend(t *testing.T) {
	t.Parallel()
	hub := NewMessageHub(zap.NewNop())
	hub.CreateChannel("agent1")
	hub.CreateChannel("agent2")

	msg := &Message{
		FromID:  "agent1",
		ToID:    "agent2",
		Type:    MessageTypeProposal,
		Content: "hello",
	}
	err := hub.Send(msg)
	require.NoError(t, err)
	assert.NotEmpty(t, msg.ID, "ID should be auto-generated")

	received, err := hub.Receive("agent2", 1*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "hello", received.Content)
}

func TestMessageHub_BroadcastMessage(t *testing.T) {
	t.Parallel()
	hub := NewMessageHub(zap.NewNop())
	hub.CreateChannel("a1")
	hub.CreateChannel("a2")
	hub.CreateChannel("a3")

	msg := &Message{
		FromID:  "a1",
		ToID:    "", // broadcast
		Type:    MessageTypeBroadcast,
		Content: "broadcast msg",
	}
	err := hub.Send(msg)
	require.NoError(t, err)

	// a2 and a3 should receive, a1 (sender) should not
	r2, err := hub.Receive("a2", 1*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "broadcast msg", r2.Content)

	r3, err := hub.Receive("a3", 1*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "broadcast msg", r3.Content)
}

func TestMessageHub_SendToUnknownChannel(t *testing.T) {
	t.Parallel()
	hub := NewMessageHub(zap.NewNop())
	hub.CreateChannel("a1")

	msg := &Message{
		FromID: "a1",
		ToID:   "nonexistent",
		Type:   MessageTypeProposal,
	}
	err := hub.Send(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "channel not found")
}

func TestMessageHub_ReceiveTimeout(t *testing.T) {
	t.Parallel()
	hub := NewMessageHub(zap.NewNop())
	hub.CreateChannel("a1")

	_, err := hub.Receive("a1", 50*time.Millisecond)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestMessageHub_ReceiveUnknownChannel(t *testing.T) {
	t.Parallel()
	hub := NewMessageHub(zap.NewNop())

	_, err := hub.Receive("nonexistent", 50*time.Millisecond)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "channel not found")
}

func TestMessageHub_Close(t *testing.T) {
	t.Parallel()
	hub := NewMessageHub(zap.NewNop())
	hub.CreateChannel("a1")

	err := hub.Close()
	require.NoError(t, err)

	// Sending to a closed hub should fail
	msg := &Message{FromID: "a1", ToID: "a1", Content: "test"}
	err = hub.Send(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestMessageHub_StatsWithoutStore(t *testing.T) {
	t.Parallel()
	hub := NewMessageHub(zap.NewNop())
	_, err := hub.Stats(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no message store")
}

func TestMessageHub_RecoverMessagesWithoutStore(t *testing.T) {
	t.Parallel()
	hub := NewMessageHub(zap.NewNop())
	err := hub.RecoverMessages(context.Background())
	assert.NoError(t, err) // no-op when no store
}

// ---------------------------------------------------------------------------
// Coordinator patterns via Execute
// ---------------------------------------------------------------------------

func TestMultiAgentSystem_Execute_Debate(t *testing.T) {
	t.Parallel()
	a1 := newMockAgent("a1", "Agent1").WithOutput("opinion A")
	a2 := newMockAgent("a2", "Agent2").WithOutput("opinion B")

	cfg := DefaultMultiAgentConfig()
	cfg.Pattern = PatternDebate
	cfg.MaxRounds = 1

	sys := NewMultiAgentSystem([]agent.Agent{a1, a2}, cfg, zap.NewNop())
	input := &agent.Input{Content: "What is the best approach?", TraceID: "trace-1"}

	output, err := sys.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.NotNil(t, output)
	assert.NotEmpty(t, output.Content)
	// Both agents should have been called at least once (initial + debate rounds)
	assert.GreaterOrEqual(t, int(a1.callCount.Load()), 1)
	assert.GreaterOrEqual(t, int(a2.callCount.Load()), 1)
}

func TestMultiAgentSystem_Execute_Consensus(t *testing.T) {
	t.Parallel()
	a1 := newMockAgent("a1", "Agent1").WithOutput("consensus answer")
	a2 := newMockAgent("a2", "Agent2").WithOutput("consensus answer")

	cfg := DefaultMultiAgentConfig()
	cfg.Pattern = PatternConsensus

	sys := NewMultiAgentSystem([]agent.Agent{a1, a2}, cfg, zap.NewNop())
	input := &agent.Input{Content: "Agree on something"}

	output, err := sys.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.NotNil(t, output)
}

func TestMultiAgentSystem_Execute_Pipeline(t *testing.T) {
	t.Parallel()
	a1 := newMockAgent("a1", "Agent1").WithOutput("step1 result")
	a2 := newMockAgent("a2", "Agent2").WithOutput("step2 result")

	cfg := DefaultMultiAgentConfig()
	cfg.Pattern = PatternPipeline

	sys := NewMultiAgentSystem([]agent.Agent{a1, a2}, cfg, zap.NewNop())
	input := &agent.Input{Content: "pipeline input"}

	output, err := sys.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.NotNil(t, output)
}

func TestMultiAgentSystem_Execute_Broadcast(t *testing.T) {
	t.Parallel()
	a1 := newMockAgent("a1", "Agent1").WithOutput("broadcast result 1")
	a2 := newMockAgent("a2", "Agent2").WithOutput("broadcast result 2")

	cfg := DefaultMultiAgentConfig()
	cfg.Pattern = PatternBroadcast

	sys := NewMultiAgentSystem([]agent.Agent{a1, a2}, cfg, zap.NewNop())
	input := &agent.Input{Content: "broadcast input", TraceID: "trace-bc"}

	output, err := sys.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.NotNil(t, output)
	assert.Equal(t, "trace-bc", output.TraceID)
	// Broadcast combines all outputs
	assert.Contains(t, output.Content, "Agent 1:")
	assert.Contains(t, output.Content, "Agent 2:")
}

func TestMultiAgentSystem_Execute_Network(t *testing.T) {
	t.Parallel()
	a1 := newMockAgent("a1", "Agent1").WithOutput("network result")

	cfg := DefaultMultiAgentConfig()
	cfg.Pattern = PatternNetwork

	sys := NewMultiAgentSystem([]agent.Agent{a1}, cfg, zap.NewNop())
	input := &agent.Input{Content: "network input"}

	output, err := sys.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.NotNil(t, output)
}

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

func TestMultiAgentSystem_Execute_AllAgentsFail_Broadcast(t *testing.T) {
	t.Parallel()
	a1 := newMockAgent("a1", "Agent1").WithError(errors.New("fail1"))
	a2 := newMockAgent("a2", "Agent2").WithError(errors.New("fail2"))

	cfg := DefaultMultiAgentConfig()
	cfg.Pattern = PatternBroadcast

	sys := NewMultiAgentSystem([]agent.Agent{a1, a2}, cfg, zap.NewNop())
	input := &agent.Input{Content: "test"}

	_, err := sys.Execute(context.Background(), input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "all agents failed")
}

func TestMultiAgentSystem_Execute_AllAgentsFail_Consensus(t *testing.T) {
	t.Parallel()
	a1 := newMockAgent("a1", "Agent1").WithError(errors.New("fail"))

	cfg := DefaultMultiAgentConfig()
	cfg.Pattern = PatternConsensus

	sys := NewMultiAgentSystem([]agent.Agent{a1}, cfg, zap.NewNop())
	input := &agent.Input{Content: "test"}

	_, err := sys.Execute(context.Background(), input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no valid outputs")
}

func TestMultiAgentSystem_Execute_PipelineStageFailure(t *testing.T) {
	t.Parallel()
	a1 := newMockAgent("a1", "Agent1").WithOutput("ok")
	a2 := newMockAgent("a2", "Agent2").WithError(errors.New("stage 2 failed"))

	cfg := DefaultMultiAgentConfig()
	cfg.Pattern = PatternPipeline

	sys := NewMultiAgentSystem([]agent.Agent{a1, a2}, cfg, zap.NewNop())
	input := &agent.Input{Content: "test"}

	_, err := sys.Execute(context.Background(), input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline stage")
}

// ---------------------------------------------------------------------------
// Message conversion helpers
// ---------------------------------------------------------------------------

func TestMessageHub_ToPersistMessage(t *testing.T) {
	t.Parallel()
	hub := NewMessageHub(zap.NewNop())
	now := time.Now()
	msg := &Message{
		ID:        "msg-1",
		FromID:    "a1",
		ToID:      "a2",
		Type:      MessageTypeProposal,
		Content:   "test content",
		Metadata:  map[string]any{"key": "value"},
		Timestamp: now,
	}

	pm := hub.toPersistMessage(msg)
	assert.Equal(t, "msg-1", pm.ID)
	assert.Equal(t, "a1", pm.FromID)
	assert.Equal(t, "a2", pm.ToID)
	assert.Equal(t, string(MessageTypeProposal), pm.Type)
	assert.Equal(t, "test content", pm.Content)
	assert.Equal(t, now, pm.CreatedAt)
}

func TestMessageHub_FromPersistMessage(t *testing.T) {
	t.Parallel()
	hub := NewMessageHub(zap.NewNop())
	now := time.Now()
	pm := &persistence.Message{
		ID:        "msg-2",
		FromID:    "a1",
		ToID:      "a2",
		Type:      "response",
		Content:   "reply",
		Payload:   map[string]any{"k": "v"},
		CreatedAt: now,
	}

	msg := hub.fromPersistMessage(pm)
	assert.Equal(t, "msg-2", msg.ID)
	assert.Equal(t, MessageTypeResponse, msg.Type)
	assert.Equal(t, "reply", msg.Content)
	assert.Equal(t, now, msg.Timestamp)
}
