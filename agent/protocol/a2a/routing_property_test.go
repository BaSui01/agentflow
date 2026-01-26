package a2a

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// Feature: agent-framework-2026-enhancements, Property 11: A2A Task Routing Correctness
// **Validates: Requirements 6.2**
// For any A2A task request sent to a registered Agent, the system should route the request
// to the corresponding local Agent, and the Agent's Execute method should be called.

// routingTestAgent is a test agent that tracks Execute calls for property testing.
type routingTestAgent struct {
	id           string
	name         string
	agentType    agent.AgentType
	state        agent.State
	executeCalls int64
	lastInput    *agent.Input
	mu           sync.Mutex
	executeFunc  func(ctx context.Context, input *agent.Input) (*agent.Output, error)
}

func newRoutingTestAgent(id, name string, agentType agent.AgentType) *routingTestAgent {
	return &routingTestAgent{
		id:        id,
		name:      name,
		agentType: agentType,
		state:     agent.StateReady,
	}
}

func (m *routingTestAgent) ID() string            { return m.id }
func (m *routingTestAgent) Name() string          { return m.name }
func (m *routingTestAgent) Type() agent.AgentType { return m.agentType }
func (m *routingTestAgent) State() agent.State    { return m.state }
func (m *routingTestAgent) Init(ctx context.Context) error {
	return nil
}
func (m *routingTestAgent) Teardown(ctx context.Context) error { return nil }

func (m *routingTestAgent) Plan(ctx context.Context, input *agent.Input) (*agent.PlanResult, error) {
	return &agent.PlanResult{Steps: []string{"step1"}}, nil
}

func (m *routingTestAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	atomic.AddInt64(&m.executeCalls, 1)
	m.mu.Lock()
	m.lastInput = input
	m.mu.Unlock()

	if m.executeFunc != nil {
		return m.executeFunc(ctx, input)
	}

	return &agent.Output{
		TraceID:      input.TraceID,
		Content:      "mock response for: " + input.Content,
		TokensUsed:   10,
		Duration:     time.Millisecond * 100,
		FinishReason: "stop",
	}, nil
}

func (m *routingTestAgent) Observe(ctx context.Context, feedback *agent.Feedback) error {
	return nil
}

func (m *routingTestAgent) GetExecuteCallCount() int64 {
	return atomic.LoadInt64(&m.executeCalls)
}

func (m *routingTestAgent) GetLastInput() *agent.Input {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastInput
}

func (m *routingTestAgent) ResetCalls() {
	atomic.StoreInt64(&m.executeCalls, 0)
	m.mu.Lock()
	m.lastInput = nil
	m.mu.Unlock()
}

// genValidAgentID generates a valid agent identifier for testing.
func genValidAgentID() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-z][a-z0-9-]{2,20}`)
}

// genValidAgentName generates a valid agent name for testing.
func genValidAgentName() *rapid.Generator[string] {
	return rapid.StringMatching(`[A-Z][a-zA-Z0-9 ]{2,30}`)
}

// genValidAgentType generates a valid agent type for testing.
func genValidAgentType() *rapid.Generator[agent.AgentType] {
	return rapid.SampledFrom([]agent.AgentType{
		agent.TypeGeneric,
		agent.TypeAssistant,
		agent.TypeAnalyzer,
		agent.TypeTranslator,
		agent.TypeSummarizer,
		agent.TypeReviewer,
	})
}

// genTaskPayload generates a valid task payload for testing.
func genTaskPayload() *rapid.Generator[map[string]any] {
	return rapid.Custom(func(t *rapid.T) map[string]any {
		content := rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(t, "content")
		return map[string]any{
			"content": content,
		}
	})
}

// genTaskMessage generates a valid A2A task message for testing.
func genTaskMessage(toAgentID string) *rapid.Generator[*A2AMessage] {
	return rapid.Custom(func(t *rapid.T) *A2AMessage {
		return &A2AMessage{
			ID:        genMessageID().Draw(t, "id"),
			Type:      A2AMessageTypeTask,
			From:      genValidAgentID().Draw(t, "from"),
			To:        toAgentID,
			Payload:   genTaskPayload().Draw(t, "payload"),
			Timestamp: time.Now().UTC(),
		}
	})
}

// TestProperty_A2A_TaskRouting_RegisteredAgent tests that task requests are routed to registered agents.
// Property 11: A2A Task Routing Correctness
// **Validates: Requirements 6.2**
func TestProperty_A2A_TaskRouting_RegisteredAgent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Setup: Create server and mock agent
		config := DefaultServerConfig()
		config.RequestTimeout = 5 * time.Second
		server := NewHTTPServer(config)

		// Generate random agent properties
		agentID := genValidAgentID().Draw(rt, "agentID")
		agentName := genValidAgentName().Draw(rt, "agentName")
		agentType := genValidAgentType().Draw(rt, "agentType")

		// Create and register mock agent
		testAg := newRoutingTestAgent(agentID, agentName, agentType)
		err := server.RegisterAgent(testAg)
		require.NoError(t, err, "Should register agent successfully")

		// Generate task message targeting the registered agent
		taskMsg := genTaskMessage(agentID).Draw(rt, "taskMessage")

		// Execute: Route the message
		routedAgent, err := server.routeMessage(taskMsg)
		require.NoError(t, err, "Should route message successfully")

		// Property 1: The routed agent should be the registered agent
		assert.Equal(t, agentID, routedAgent.ID(),
			"Task should be routed to the correct agent by ID")

		// Property 2: The routed agent should have the correct name
		assert.Equal(t, agentName, routedAgent.Name(),
			"Routed agent should have the correct name")

		// Property 3: The routed agent should have the correct type
		assert.Equal(t, agentType, routedAgent.Type(),
			"Routed agent should have the correct type")
	})
}

// TestProperty_A2A_TaskRouting_ExecuteMethodCalled tests that Execute method is called when processing tasks.
// Property 11: A2A Task Routing Correctness
// **Validates: Requirements 6.2**
func TestProperty_A2A_TaskRouting_ExecuteMethodCalled(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Setup: Create server and mock agent
		config := DefaultServerConfig()
		config.RequestTimeout = 5 * time.Second
		server := NewHTTPServer(config)

		// Generate random agent properties
		agentID := genValidAgentID().Draw(rt, "agentID")
		agentName := genValidAgentName().Draw(rt, "agentName")
		agentType := genValidAgentType().Draw(rt, "agentType")

		// Create and register mock agent
		testAg := newRoutingTestAgent(agentID, agentName, agentType)
		err := server.RegisterAgent(testAg)
		require.NoError(t, err, "Should register agent successfully")

		// Generate task message
		taskMsg := genTaskMessage(agentID).Draw(rt, "taskMessage")

		// Get initial execute call count
		initialCallCount := testAg.GetExecuteCallCount()

		// Execute: Process the task through executeTask
		ctx := context.Background()
		routedAgent, err := server.routeMessage(taskMsg)
		require.NoError(t, err, "Should route message successfully")

		_, err = server.executeTask(ctx, routedAgent, taskMsg)
		require.NoError(t, err, "Should execute task successfully")

		// Property: Execute method should be called exactly once
		finalCallCount := testAg.GetExecuteCallCount()
		assert.Equal(t, initialCallCount+1, finalCallCount,
			"Agent's Execute method should be called exactly once per task")
	})
}

// TestProperty_A2A_TaskRouting_InputPreserved tests that task payload is correctly passed to Execute.
// Property 11: A2A Task Routing Correctness
// **Validates: Requirements 6.2**
func TestProperty_A2A_TaskRouting_InputPreserved(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Setup: Create server and mock agent
		config := DefaultServerConfig()
		config.RequestTimeout = 5 * time.Second
		server := NewHTTPServer(config)

		// Generate random agent properties
		agentID := genValidAgentID().Draw(rt, "agentID")
		agentName := genValidAgentName().Draw(rt, "agentName")

		// Create and register mock agent
		testAg := newRoutingTestAgent(agentID, agentName, agent.TypeGeneric)
		err := server.RegisterAgent(testAg)
		require.NoError(t, err, "Should register agent successfully")

		// Generate task message with specific content
		taskContent := rapid.StringMatching(`[a-zA-Z0-9 ]{10,50}`).Draw(rt, "taskContent")
		taskMsg := &A2AMessage{
			ID:        genMessageID().Draw(rt, "id"),
			Type:      A2AMessageTypeTask,
			From:      genValidAgentID().Draw(rt, "from"),
			To:        agentID,
			Payload:   map[string]any{"content": taskContent},
			Timestamp: time.Now().UTC(),
		}

		// Execute: Process the task
		ctx := context.Background()
		routedAgent, err := server.routeMessage(taskMsg)
		require.NoError(t, err, "Should route message successfully")

		_, err = server.executeTask(ctx, routedAgent, taskMsg)
		require.NoError(t, err, "Should execute task successfully")

		// Property: The input content should be preserved
		lastInput := testAg.GetLastInput()
		require.NotNil(t, lastInput, "Execute should receive input")
		assert.Equal(t, taskContent, lastInput.Content,
			"Task content should be preserved in Execute input")

		// Property: The trace ID should match the message ID
		assert.Equal(t, taskMsg.ID, lastInput.TraceID,
			"Trace ID should match the message ID")
	})
}

// TestProperty_A2A_TaskRouting_MultipleAgents tests routing with multiple registered agents.
// Property 11: A2A Task Routing Correctness
// **Validates: Requirements 6.2**
func TestProperty_A2A_TaskRouting_MultipleAgents(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Setup: Create server
		config := DefaultServerConfig()
		config.RequestTimeout = 5 * time.Second
		server := NewHTTPServer(config)

		// Generate and register multiple agents
		numAgents := rapid.IntRange(2, 5).Draw(rt, "numAgents")
		agents := make([]*routingTestAgent, numAgents)
		agentIDs := make([]string, numAgents)

		for i := 0; i < numAgents; i++ {
			// Ensure unique IDs by appending index
			baseID := genValidAgentID().Draw(rt, "baseAgentID")
			agentID := baseID + "-" + string(rune('a'+i))
			agentName := genValidAgentName().Draw(rt, "agentName")
			agentType := genValidAgentType().Draw(rt, "agentType")

			testAg := newRoutingTestAgent(agentID, agentName, agentType)
			err := server.RegisterAgent(testAg)
			require.NoError(t, err, "Should register agent %d successfully", i)

			agents[i] = testAg
			agentIDs[i] = agentID
		}

		// Select a random target agent
		targetIdx := rapid.IntRange(0, numAgents-1).Draw(rt, "targetIdx")
		targetAgentID := agentIDs[targetIdx]
		targetAgent := agents[targetIdx]

		// Generate task message targeting the selected agent
		taskMsg := genTaskMessage(targetAgentID).Draw(rt, "taskMessage")

		// Execute: Route and execute the task
		ctx := context.Background()
		routedAgent, err := server.routeMessage(taskMsg)
		require.NoError(t, err, "Should route message successfully")

		_, err = server.executeTask(ctx, routedAgent, taskMsg)
		require.NoError(t, err, "Should execute task successfully")

		// Property 1: Only the target agent should have Execute called
		for i, ag := range agents {
			if i == targetIdx {
				assert.Equal(t, int64(1), ag.GetExecuteCallCount(),
					"Target agent should have Execute called once")
			} else {
				assert.Equal(t, int64(0), ag.GetExecuteCallCount(),
					"Non-target agent %d should not have Execute called", i)
			}
		}

		// Property 2: The routed agent should be the target agent
		assert.Equal(t, targetAgentID, routedAgent.ID(),
			"Should route to the correct target agent")

		// Property 3: The target agent should receive the correct input
		lastInput := targetAgent.GetLastInput()
		require.NotNil(t, lastInput, "Target agent should receive input")
		assert.Equal(t, taskMsg.ID, lastInput.TraceID,
			"Input trace ID should match message ID")
	})
}

// TestProperty_A2A_TaskRouting_UnregisteredAgentFallback tests fallback behavior for unregistered agents.
// Property 11: A2A Task Routing Correctness
// **Validates: Requirements 6.2**
func TestProperty_A2A_TaskRouting_UnregisteredAgentFallback(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Setup: Create server with a default agent
		config := DefaultServerConfig()
		config.RequestTimeout = 5 * time.Second
		server := NewHTTPServer(config)

		// Register a default agent
		defaultAgentID := genValidAgentID().Draw(rt, "defaultAgentID")
		defaultAgent := newRoutingTestAgent(defaultAgentID, "Default Agent", agent.TypeGeneric)
		err := server.RegisterAgent(defaultAgent)
		require.NoError(t, err, "Should register default agent successfully")

		// Generate task message targeting a non-existent agent
		nonExistentID := "non-existent-agent-" + genValidAgentID().Draw(rt, "suffix")
		taskMsg := &A2AMessage{
			ID:        genMessageID().Draw(rt, "id"),
			Type:      A2AMessageTypeTask,
			From:      genValidAgentID().Draw(rt, "from"),
			To:        nonExistentID,
			Payload:   map[string]any{"content": "test task"},
			Timestamp: time.Now().UTC(),
		}

		// Execute: Route the message (should fall back to default agent)
		routedAgent, err := server.routeMessage(taskMsg)
		require.NoError(t, err, "Should route message with fallback")

		// Property: Should fall back to the registered agent
		assert.Equal(t, defaultAgentID, routedAgent.ID(),
			"Should fall back to registered agent when target not found")
	})
}

// TestProperty_A2A_TaskRouting_ContextPreserved tests that A2A context is preserved in Execute input.
// Property 11: A2A Task Routing Correctness
// **Validates: Requirements 6.2**
func TestProperty_A2A_TaskRouting_ContextPreserved(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Setup: Create server and mock agent
		config := DefaultServerConfig()
		config.RequestTimeout = 5 * time.Second
		server := NewHTTPServer(config)

		// Generate random agent properties
		agentID := genValidAgentID().Draw(rt, "agentID")
		testAg := newRoutingTestAgent(agentID, "Test Agent", agent.TypeGeneric)
		err := server.RegisterAgent(testAg)
		require.NoError(t, err, "Should register agent successfully")

		// Generate task message
		fromAgentID := genValidAgentID().Draw(rt, "fromAgentID")
		taskMsg := &A2AMessage{
			ID:        genMessageID().Draw(rt, "id"),
			Type:      A2AMessageTypeTask,
			From:      fromAgentID,
			To:        agentID,
			Payload:   map[string]any{"content": "test task"},
			Timestamp: time.Now().UTC(),
		}

		// Execute: Process the task
		ctx := context.Background()
		routedAgent, err := server.routeMessage(taskMsg)
		require.NoError(t, err, "Should route message successfully")

		_, err = server.executeTask(ctx, routedAgent, taskMsg)
		require.NoError(t, err, "Should execute task successfully")

		// Property: A2A context should be preserved in input
		lastInput := testAg.GetLastInput()
		require.NotNil(t, lastInput, "Execute should receive input")
		require.NotNil(t, lastInput.Context, "Input should have context")

		// Check A2A-specific context fields
		assert.Equal(t, taskMsg.ID, lastInput.Context["a2a_message_id"],
			"A2A message ID should be in context")
		assert.Equal(t, string(A2AMessageTypeTask), lastInput.Context["a2a_message_type"],
			"A2A message type should be in context")
		assert.Equal(t, fromAgentID, lastInput.Context["a2a_from"],
			"A2A from agent should be in context")
	})
}

// TestProperty_A2A_TaskRouting_ResponseFormat tests that Execute response is correctly formatted.
// Property 11: A2A Task Routing Correctness
// **Validates: Requirements 6.2**
func TestProperty_A2A_TaskRouting_ResponseFormat(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Setup: Create server and mock agent
		config := DefaultServerConfig()
		config.RequestTimeout = 5 * time.Second
		server := NewHTTPServer(config)

		// Generate random agent properties
		agentID := genValidAgentID().Draw(rt, "agentID")
		testAg := newRoutingTestAgent(agentID, "Test Agent", agent.TypeGeneric)

		// Set custom response
		expectedContent := rapid.StringMatching(`[a-zA-Z0-9 ]{10,50}`).Draw(rt, "responseContent")
		expectedTokens := rapid.IntRange(1, 1000).Draw(rt, "tokens")
		testAg.executeFunc = func(ctx context.Context, input *agent.Input) (*agent.Output, error) {
			return &agent.Output{
				TraceID:      input.TraceID,
				Content:      expectedContent,
				TokensUsed:   expectedTokens,
				Duration:     time.Millisecond * 100,
				FinishReason: "stop",
			}, nil
		}

		err := server.RegisterAgent(testAg)
		require.NoError(t, err, "Should register agent successfully")

		// Generate task message
		taskMsg := genTaskMessage(agentID).Draw(rt, "taskMessage")

		// Execute: Process the task
		ctx := context.Background()
		routedAgent, err := server.routeMessage(taskMsg)
		require.NoError(t, err, "Should route message successfully")

		result, err := server.executeTask(ctx, routedAgent, taskMsg)
		require.NoError(t, err, "Should execute task successfully")

		// Property 1: Result should be a reply message
		assert.Equal(t, A2AMessageTypeResult, result.Type,
			"Result should be of type 'result'")

		// Property 2: Result should reference the original message
		assert.Equal(t, taskMsg.ID, result.ReplyTo,
			"Result should reference the original message ID")

		// Property 3: Result should be from the agent
		assert.Equal(t, agentID, result.From,
			"Result should be from the executing agent")

		// Property 4: Result should be to the original sender
		assert.Equal(t, taskMsg.From, result.To,
			"Result should be addressed to the original sender")

		// Property 5: Result payload should contain the response content
		payload, ok := result.Payload.(map[string]any)
		require.True(t, ok, "Result payload should be a map")
		assert.Equal(t, expectedContent, payload["content"],
			"Result payload should contain the response content")
	})
}

// TestProperty_A2A_TaskRouting_Idempotent tests that routing is idempotent.
// Property 11: A2A Task Routing Correctness
// **Validates: Requirements 6.2**
func TestProperty_A2A_TaskRouting_Idempotent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Setup: Create server and mock agent
		config := DefaultServerConfig()
		server := NewHTTPServer(config)

		// Generate random agent properties
		agentID := genValidAgentID().Draw(rt, "agentID")
		testAg := newRoutingTestAgent(agentID, "Test Agent", agent.TypeGeneric)
		err := server.RegisterAgent(testAg)
		require.NoError(t, err, "Should register agent successfully")

		// Generate task message
		taskMsg := genTaskMessage(agentID).Draw(rt, "taskMessage")

		// Execute: Route the same message multiple times
		numRoutes := rapid.IntRange(2, 5).Draw(rt, "numRoutes")
		for i := 0; i < numRoutes; i++ {
			routedAgent, err := server.routeMessage(taskMsg)
			require.NoError(t, err, "Should route message successfully on attempt %d", i)

			// Property: Should always route to the same agent
			assert.Equal(t, agentID, routedAgent.ID(),
				"Routing should be idempotent - always route to the same agent")
		}
	})
}
