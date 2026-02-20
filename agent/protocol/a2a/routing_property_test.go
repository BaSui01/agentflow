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

// 特性:代理-框架-2026-增强,财产 11:A2A任务路线正确性
// ** 参数:要求6.2**
// 对于向注册代理发送的 A2A 任务请求,系统应引导请求
// 给对应的本地代理,并且代理的执行方法应当调用.

// 路径 TestAgent是追踪执行呼叫属性测试的测试代理.
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

// genValidAgentID生成用于测试的有效代理标识符.
func genValidAgentID() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-z][a-z0-9-]{2,20}`)
}

// genValidAgentName 生成用于测试的有效代理名称.
func genValidAgentName() *rapid.Generator[string] {
	return rapid.StringMatching(`[A-Z][a-zA-Z0-9 ]{2,30}`)
}

// genValidAgentType生成一个有效的代理类型进行测试.
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

// genTaskPayload 生成一个有效的任务有效载荷用于测试.
func genTaskPayload() *rapid.Generator[map[string]any] {
	return rapid.Custom(func(t *rapid.T) map[string]any {
		content := rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(t, "content")
		return map[string]any{
			"content": content,
		}
	})
}

// genTaskMessage 生成一个有效的 A2A 任务消息进行测试.
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

// TestProperty A2A Taskrouting 注册代理测试,任务请求被路由到注册代理.
// 属性 11: A2A 任务运行正确性
// ** 参数:要求6.2**
func TestProperty_A2A_TaskRouting_RegisteredAgent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 设置: 创建服务器和模拟代理
		config := DefaultServerConfig()
		config.RequestTimeout = 5 * time.Second
		server := NewHTTPServer(config)

		// 生成随机代理属性
		agentID := genValidAgentID().Draw(rt, "agentID")
		agentName := genValidAgentName().Draw(rt, "agentName")
		agentType := genValidAgentType().Draw(rt, "agentType")

		// 创建并注册模拟代理
		testAg := newRoutingTestAgent(agentID, agentName, agentType)
		err := server.RegisterAgent(testAg)
		require.NoError(t, err, "Should register agent successfully")

		// 生成针对注册代理的任务消息
		taskMsg := genTaskMessage(agentID).Draw(rt, "taskMessage")

		// 执行: 路由信件
		routedAgent, err := server.routeMessage(taskMsg)
		require.NoError(t, err, "Should route message successfully")

		// 财产1:路由代理应为已注册代理
		assert.Equal(t, agentID, routedAgent.ID(),
			"Task should be routed to the correct agent by ID")

		// 财产2:路由代理应当有正确名称
		assert.Equal(t, agentName, routedAgent.Name(),
			"Routed agent should have the correct name")

		// 属性3: 路由代理应当有正确的类型
		assert.Equal(t, agentType, routedAgent.Type(),
			"Routed agent should have the correct type")
	})
}

// TestProperty A2A TaskRouting Execute MethodCalled 测试,在处理任务时,执行方法被称为执行方法.
// 属性 11: A2A 任务运行正确性
// ** 参数:要求6.2**
func TestProperty_A2A_TaskRouting_ExecuteMethodCalled(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 设置: 创建服务器和模拟代理
		config := DefaultServerConfig()
		config.RequestTimeout = 5 * time.Second
		server := NewHTTPServer(config)

		// 生成随机代理属性
		agentID := genValidAgentID().Draw(rt, "agentID")
		agentName := genValidAgentName().Draw(rt, "agentName")
		agentType := genValidAgentType().Draw(rt, "agentType")

		// 创建并注册模拟代理
		testAg := newRoutingTestAgent(agentID, agentName, agentType)
		err := server.RegisterAgent(testAg)
		require.NoError(t, err, "Should register agent successfully")

		// 生成任务消息
		taskMsg := genTaskMessage(agentID).Draw(rt, "taskMessage")

		// 获得初始执行呼叫数
		initialCallCount := testAg.GetExecuteCallCount()

		// 执行: 通过执行任务处理任务
		ctx := context.Background()
		routedAgent, err := server.routeMessage(taskMsg)
		require.NoError(t, err, "Should route message successfully")

		_, err = server.executeTask(ctx, routedAgent, taskMsg)
		require.NoError(t, err, "Should execute task successfully")

		// 属性: 执行方法应精确调用一次
		finalCallCount := testAg.GetExecuteCallCount()
		assert.Equal(t, initialCallCount+1, finalCallCount,
			"Agent's Execute method should be called exactly once per task")
	})
}

// 测试Property A2A 任务设置 输入预留测试,任务有效载荷被正确传递给执行.
// 属性 11: A2A 任务运行正确性
// ** 参数:要求6.2**
func TestProperty_A2A_TaskRouting_InputPreserved(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 设置: 创建服务器和模拟代理
		config := DefaultServerConfig()
		config.RequestTimeout = 5 * time.Second
		server := NewHTTPServer(config)

		// 生成随机代理属性
		agentID := genValidAgentID().Draw(rt, "agentID")
		agentName := genValidAgentName().Draw(rt, "agentName")

		// 创建并注册模拟代理
		testAg := newRoutingTestAgent(agentID, agentName, agent.TypeGeneric)
		err := server.RegisterAgent(testAg)
		require.NoError(t, err, "Should register agent successfully")

		// 生成带有特定内容的任务信件
		taskContent := rapid.StringMatching(`[a-zA-Z0-9 ]{10,50}`).Draw(rt, "taskContent")
		taskMsg := &A2AMessage{
			ID:        genMessageID().Draw(rt, "id"),
			Type:      A2AMessageTypeTask,
			From:      genValidAgentID().Draw(rt, "from"),
			To:        agentID,
			Payload:   map[string]any{"content": taskContent},
			Timestamp: time.Now().UTC(),
		}

		// 执行: 处理任务
		ctx := context.Background()
		routedAgent, err := server.routeMessage(taskMsg)
		require.NoError(t, err, "Should route message successfully")

		_, err = server.executeTask(ctx, routedAgent, taskMsg)
		require.NoError(t, err, "Should execute task successfully")

		// 属性: 输入内容应当保存
		lastInput := testAg.GetLastInput()
		require.NotNil(t, lastInput, "Execute should receive input")
		assert.Equal(t, taskContent, lastInput.Content,
			"Task content should be preserved in Execute input")

		// 属性: 追踪ID 应与消息ID相匹配
		assert.Equal(t, taskMsg.ID, lastInput.TraceID,
			"Trace ID should match the message ID")
	})
}

// 测试Property A2A 任务盘点 多肽剂测试途径多注册代理.
// 属性 11: A2A 任务运行正确性
// ** 参数:要求6.2**
func TestProperty_A2A_TaskRouting_MultipleAgents(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 设置: 创建服务器
		config := DefaultServerConfig()
		config.RequestTimeout = 5 * time.Second
		server := NewHTTPServer(config)

		// 生成和注册多个代理
		numAgents := rapid.IntRange(2, 5).Draw(rt, "numAgents")
		agents := make([]*routingTestAgent, numAgents)
		agentIDs := make([]string, numAgents)

		for i := 0; i < numAgents; i++ {
			// 通过附加索引确保唯一的ID
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

		// 选择随机目标代理
		targetIdx := rapid.IntRange(0, numAgents-1).Draw(rt, "targetIdx")
		targetAgentID := agentIDs[targetIdx]
		targetAgent := agents[targetIdx]

		// 生成针对选中代理的任务消息
		taskMsg := genTaskMessage(targetAgentID).Draw(rt, "taskMessage")

		// 执行: 路径和任务执行
		ctx := context.Background()
		routedAgent, err := server.routeMessage(taskMsg)
		require.NoError(t, err, "Should route message successfully")

		_, err = server.executeTask(ctx, routedAgent, taskMsg)
		require.NoError(t, err, "Should execute task successfully")

		// 财产 1: 只有目标代理人才应执行
		for i, ag := range agents {
			if i == targetIdx {
				assert.Equal(t, int64(1), ag.GetExecuteCallCount(),
					"Target agent should have Execute called once")
			} else {
				assert.Equal(t, int64(0), ag.GetExecuteCallCount(),
					"Non-target agent %d should not have Execute called", i)
			}
		}

		// 物业2:路由代理应当是目标代理
		assert.Equal(t, targetAgentID, routedAgent.ID(),
			"Should route to the correct target agent")

		// 财产3:目标代理人应获得正确的输入
		lastInput := targetAgent.GetLastInput()
		require.NotNil(t, lastInput, "Target agent should receive input")
		assert.Equal(t, taskMsg.ID, lastInput.TraceID,
			"Input trace ID should match message ID")
	})
}

// TestProperty A2A TaskRouting 未注册代理Fallback测试未注册代理的倒置行为.
// 属性 11: A2A 任务运行正确性
// ** 参数:要求6.2**
func TestProperty_A2A_TaskRouting_UnregisteredAgentFallback(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 设置: 用默认代理创建服务器
		config := DefaultServerConfig()
		config.RequestTimeout = 5 * time.Second
		server := NewHTTPServer(config)

		// 注册默认代理
		defaultAgentID := genValidAgentID().Draw(rt, "defaultAgentID")
		defaultAgent := newRoutingTestAgent(defaultAgentID, "Default Agent", agent.TypeGeneric)
		err := server.RegisterAgent(defaultAgent)
		require.NoError(t, err, "Should register default agent successfully")

		// 生成针对不存在代理的任务消息
		nonExistentID := "non-existent-agent-" + genValidAgentID().Draw(rt, "suffix")
		taskMsg := &A2AMessage{
			ID:        genMessageID().Draw(rt, "id"),
			Type:      A2AMessageTypeTask,
			From:      genValidAgentID().Draw(rt, "from"),
			To:        nonExistentID,
			Payload:   map[string]any{"content": "test task"},
			Timestamp: time.Now().UTC(),
		}

		// 执行: 路由信件( 应返回默认代理)
		routedAgent, err := server.routeMessage(taskMsg)
		require.NoError(t, err, "Should route message with fallback")

		// 财产:应归还注册代理人
		assert.Equal(t, defaultAgentID, routedAgent.ID(),
			"Should fall back to registered agent when target not found")
	})
}

// TestProperty A2A TaskRouting ContextPreferent 测试,A2A上下文在Execute输入中保存.
// 属性 11: A2A 任务运行正确性
// ** 参数:要求6.2**
func TestProperty_A2A_TaskRouting_ContextPreserved(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 设置: 创建服务器和模拟代理
		config := DefaultServerConfig()
		config.RequestTimeout = 5 * time.Second
		server := NewHTTPServer(config)

		// 生成随机代理属性
		agentID := genValidAgentID().Draw(rt, "agentID")
		testAg := newRoutingTestAgent(agentID, "Test Agent", agent.TypeGeneric)
		err := server.RegisterAgent(testAg)
		require.NoError(t, err, "Should register agent successfully")

		// 生成任务消息
		fromAgentID := genValidAgentID().Draw(rt, "fromAgentID")
		taskMsg := &A2AMessage{
			ID:        genMessageID().Draw(rt, "id"),
			Type:      A2AMessageTypeTask,
			From:      fromAgentID,
			To:        agentID,
			Payload:   map[string]any{"content": "test task"},
			Timestamp: time.Now().UTC(),
		}

		// 执行: 处理任务
		ctx := context.Background()
		routedAgent, err := server.routeMessage(taskMsg)
		require.NoError(t, err, "Should route message successfully")

		_, err = server.executeTask(ctx, routedAgent, taskMsg)
		require.NoError(t, err, "Should execute task successfully")

		// 属性: A2A 上下文应在输入中保存
		lastInput := testAg.GetLastInput()
		require.NotNil(t, lastInput, "Execute should receive input")
		require.NotNil(t, lastInput.Context, "Input should have context")

		// 检查 A2A 特定上下文字段
		assert.Equal(t, taskMsg.ID, lastInput.Context["a2a_message_id"],
			"A2A message ID should be in context")
		assert.Equal(t, string(A2AMessageTypeTask), lastInput.Context["a2a_message_type"],
			"A2A message type should be in context")
		assert.Equal(t, fromAgentID, lastInput.Context["a2a_from"],
			"A2A from agent should be in context")
	})
}

// TestProperty A2A 任务设置 响应Format 测试,执行响应正确格式化.
// 属性 11: A2A 任务运行正确性
// ** 参数:要求6.2**
func TestProperty_A2A_TaskRouting_ResponseFormat(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 设置: 创建服务器和模拟代理
		config := DefaultServerConfig()
		config.RequestTimeout = 5 * time.Second
		server := NewHTTPServer(config)

		// 生成随机代理属性
		agentID := genValidAgentID().Draw(rt, "agentID")
		testAg := newRoutingTestAgent(agentID, "Test Agent", agent.TypeGeneric)

		// 设置自定义响应
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

		// 生成任务消息
		taskMsg := genTaskMessage(agentID).Draw(rt, "taskMessage")

		// 执行: 处理任务
		ctx := context.Background()
		routedAgent, err := server.routeMessage(taskMsg)
		require.NoError(t, err, "Should route message successfully")

		result, err := server.executeTask(ctx, routedAgent, taskMsg)
		require.NoError(t, err, "Should execute task successfully")

		// 属性 1: 结果应为回复信息
		assert.Equal(t, A2AMessageTypeResult, result.Type,
			"Result should be of type 'result'")

		// 属性2:结果应当引用原始消息
		assert.Equal(t, taskMsg.ID, result.ReplyTo,
			"Result should reference the original message ID")

		// 财产3:结果应来自代理人
		assert.Equal(t, agentID, result.From,
			"Result should be from the executing agent")

		// 属性 4: 结果应归原发件人
		assert.Equal(t, taskMsg.From, result.To,
			"Result should be addressed to the original sender")

		// 属性5:结果有效载荷应包含响应内容
		payload, ok := result.Payload.(map[string]any)
		require.True(t, ok, "Result payload should be a map")
		assert.Equal(t, expectedContent, payload["content"],
			"Result payload should contain the response content")
	})
}

// 检测Property A2A 任务测出 路由是一能.
// 属性 11: A2A 任务运行正确性
// ** 参数:要求6.2**
func TestProperty_A2A_TaskRouting_Idempotent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 设置: 创建服务器和模拟代理
		config := DefaultServerConfig()
		server := NewHTTPServer(config)

		// 生成随机代理属性
		agentID := genValidAgentID().Draw(rt, "agentID")
		testAg := newRoutingTestAgent(agentID, "Test Agent", agent.TypeGeneric)
		err := server.RegisterAgent(testAg)
		require.NoError(t, err, "Should register agent successfully")

		// 生成任务消息
		taskMsg := genTaskMessage(agentID).Draw(rt, "taskMessage")

		// 执行: 多次运行同一信件
		numRoutes := rapid.IntRange(2, 5).Draw(rt, "numRoutes")
		for i := 0; i < numRoutes; i++ {
			routedAgent, err := server.routeMessage(taskMsg)
			require.NoError(t, err, "Should route message successfully on attempt %d", i)

			// 属性: 应始终向同一代理商走
			assert.Equal(t, agentID, routedAgent.ID(),
				"Routing should be idempotent - always route to the same agent")
		}
	})
}
