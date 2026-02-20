package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 模拟代理工具代理。 测试代理。
type mockAgent struct {
	id        string
	name      string
	agentType agent.AgentType
	state     agent.State
	execFunc  func(ctx context.Context, input *agent.Input) (*agent.Output, error)
}

func newMockAgent(id, name string) *mockAgent {
	return &mockAgent{
		id:        id,
		name:      name,
		agentType: agent.TypeGeneric,
		state:     agent.StateReady,
		execFunc: func(ctx context.Context, input *agent.Input) (*agent.Output, error) {
			return &agent.Output{
				TraceID:  input.TraceID,
				Content:  "mock response for: " + input.Content,
				Duration: 100 * time.Millisecond,
			}, nil
		},
	}
}

func (m *mockAgent) ID() string                         { return m.id }
func (m *mockAgent) Name() string                       { return m.name }
func (m *mockAgent) Type() agent.AgentType              { return m.agentType }
func (m *mockAgent) State() agent.State                 { return m.state }
func (m *mockAgent) Init(ctx context.Context) error     { return nil }
func (m *mockAgent) Teardown(ctx context.Context) error { return nil }
func (m *mockAgent) Plan(ctx context.Context, input *agent.Input) (*agent.PlanResult, error) {
	return &agent.PlanResult{Steps: []string{"step1"}}, nil
}
func (m *mockAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	return m.execFunc(ctx, input)
}
func (m *mockAgent) Observe(ctx context.Context, feedback *agent.Feedback) error { return nil }

func TestHTTPServer_RegisterAgent(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL: "http://localhost:8080",
		Logger:  zap.NewNop(),
	})

	ag := newMockAgent("test-agent", "Test Agent")

	err := server.RegisterAgent(ag)
	require.NoError(t, err)

	assert.Equal(t, 1, server.AgentCount())
	assert.Contains(t, server.ListAgents(), "test-agent")
}

func TestHTTPServer_RegisterAgent_NilAgent(t *testing.T) {
	server := NewHTTPServer(nil)

	err := server.RegisterAgent(nil)
	assert.Error(t, err)
}

func TestHTTPServer_UnregisterAgent(t *testing.T) {
	server := NewHTTPServer(nil)
	ag := newMockAgent("test-agent", "Test Agent")

	_ = server.RegisterAgent(ag)
	assert.Equal(t, 1, server.AgentCount())

	err := server.UnregisterAgent("test-agent")
	require.NoError(t, err)
	assert.Equal(t, 0, server.AgentCount())
}

func TestHTTPServer_GetAgentCard(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL: "http://localhost:8080",
		Logger:  zap.NewNop(),
	})

	ag := newMockAgent("test-agent", "Test Agent")
	_ = server.RegisterAgent(ag)

	card, err := server.GetAgentCard("test-agent")
	require.NoError(t, err)
	assert.Equal(t, "Test Agent", card.Name)
	assert.Contains(t, card.URL, "test-agent")
}

func TestHTTPServer_GetAgentCard_NotFound(t *testing.T) {
	server := NewHTTPServer(nil)

	_, err := server.GetAgentCard("nonexistent")
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestHTTPServer_HandleAgentCardDiscovery(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL: "http://localhost:8080",
		Logger:  zap.NewNop(),
	})

	ag := newMockAgent("test-agent", "Test Agent")
	_ = server.RegisterAgent(ag)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var card AgentCard
	err := json.Unmarshal(w.Body.Bytes(), &card)
	require.NoError(t, err)
	assert.Equal(t, "Test Agent", card.Name)
}

func TestHTTPServer_HandleSyncMessage(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL:        "http://localhost:8080",
		RequestTimeout: 5 * time.Second,
		Logger:         zap.NewNop(),
	})

	ag := newMockAgent("test-agent", "Test Agent")
	_ = server.RegisterAgent(ag)

	msg := NewTaskMessage("client-agent", "test-agent", map[string]string{
		"content": "Hello, agent!",
	})

	body, _ := json.Marshal(msg)
	req := httptest.NewRequest(http.MethodPost, "/a2a/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result A2AMessage
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, A2AMessageTypeResult, result.Type)
	assert.Equal(t, msg.ID, result.ReplyTo)
}

func TestHTTPServer_HandleAsyncMessage(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL:        "http://localhost:8080",
		RequestTimeout: 5 * time.Second,
		Logger:         zap.NewNop(),
	})

	ag := newMockAgent("test-agent", "Test Agent")
	_ = server.RegisterAgent(ag)

	msg := NewTaskMessage("client-agent", "test-agent", map[string]string{
		"content": "Hello, async!",
	})

	body, _ := json.Marshal(msg)
	req := httptest.NewRequest(http.MethodPost, "/a2a/messages/async", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp AsyncResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.TaskID)
	assert.Equal(t, "accepted", resp.Status)
}

func TestHTTPServer_HandleGetTaskResult(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL:        "http://localhost:8080",
		RequestTimeout: 5 * time.Second,
		Logger:         zap.NewNop(),
	})

	ag := newMockAgent("test-agent", "Test Agent")
	_ = server.RegisterAgent(ag)

	// 提交同步任务
	msg := NewTaskMessage("client-agent", "test-agent", map[string]string{
		"content": "Hello, async!",
	})

	body, _ := json.Marshal(msg)
	req := httptest.NewRequest(http.MethodPost, "/a2a/messages/async", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	var resp AsyncResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	taskID := resp.TaskID

	// 等待任务完成
	time.Sleep(200 * time.Millisecond)

	// 获取结果
	req = httptest.NewRequest(http.MethodGet, "/a2a/tasks/"+taskID+"/result", nil)
	w = httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result A2AMessage
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, A2AMessageTypeResult, result.Type)
}

func TestHTTPServer_HandleGetTaskResult_NotFound(t *testing.T) {
	server := NewHTTPServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/a2a/tasks/nonexistent/result", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHTTPServer_Authentication(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL:    "http://localhost:8080",
		EnableAuth: true,
		AuthToken:  "secret-token",
		Logger:     zap.NewNop(),
	})

	ag := newMockAgent("test-agent", "Test Agent")
	_ = server.RegisterAgent(ag)

	// 没有认证的请求
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 用错误的符号请求
	req = httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 以正确的符号请求
	req = httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHTTPServer_NotFoundEndpoint(t *testing.T) {
	server := NewHTTPServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/unknown/endpoint", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHTTPServer_CleanupExpiredTasks(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL:        "http://localhost:8080",
		RequestTimeout: 5 * time.Second,
		Logger:         zap.NewNop(),
	})

	ag := newMockAgent("test-agent", "Test Agent")
	_ = server.RegisterAgent(ag)

	// 提交同步任务
	msg := NewTaskMessage("client-agent", "test-agent", "test")
	body, _ := json.Marshal(msg)
	req := httptest.NewRequest(http.MethodPost, "/a2a/messages/async", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	// 等待完成时
	time.Sleep(200 * time.Millisecond)

	// 持续时间为0的清理应删除已完成的任务
	count := server.CleanupExpiredTasks(0)
	assert.Equal(t, 1, count)
}

func TestHTTPServer_CancelTask(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL:        "http://localhost:8080",
		RequestTimeout: 10 * time.Second,
		Logger:         zap.NewNop(),
	})

	// 创建缓冲代理
	ag := newMockAgent("slow-agent", "Slow Agent")
	ag.execFunc = func(ctx context.Context, input *agent.Input) (*agent.Output, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return &agent.Output{Content: "done"}, nil
		}
	}
	_ = server.RegisterAgent(ag)

	// 提交同步任务
	msg := NewTaskMessage("client-agent", "slow-agent", "test")
	body, _ := json.Marshal(msg)
	req := httptest.NewRequest(http.MethodPost, "/a2a/messages/async", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	var resp AsyncResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	// 取消任务
	err := server.CancelTask(resp.TaskID)
	require.NoError(t, err)

	status, _ := server.GetTaskStatus(resp.TaskID)
	assert.Equal(t, "failed", status)
}

func TestAgentAdapter(t *testing.T) {
	ag := newMockAgent("test-id", "Test Name")
	ag.agentType = agent.TypeAssistant

	adapter := newAgentAdapter(ag)

	assert.Equal(t, "test-id", adapter.ID())
	assert.Equal(t, "Test Name", adapter.Name())
	assert.Equal(t, AgentType("assistant"), adapter.Type())
	assert.Contains(t, adapter.Description(), "Test Name")
}
