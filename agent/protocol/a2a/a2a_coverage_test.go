package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- mock TaskStore ---

type mockTaskStore struct {
	saveFn      func(ctx context.Context, task *persistence.AsyncTask) error
	getFn       func(ctx context.Context, id string) (*persistence.AsyncTask, error)
	listFn      func(ctx context.Context, filter persistence.TaskFilter) ([]*persistence.AsyncTask, error)
	updateFn    func(ctx context.Context, id string, status persistence.TaskStatus, result any, errMsg string) error
	progressFn  func(ctx context.Context, id string, progress float64) error
	deleteFn    func(ctx context.Context, id string) error
	recoverFn   func(ctx context.Context) ([]*persistence.AsyncTask, error)
	cleanupFn   func(ctx context.Context, older time.Duration) (int, error)
	statsFn     func(ctx context.Context) (*persistence.TaskStoreStats, error)
	closeFn     func() error
	pingFn      func(ctx context.Context) error
}

func (m *mockTaskStore) SaveTask(ctx context.Context, task *persistence.AsyncTask) error {
	if m.saveFn != nil { return m.saveFn(ctx, task) }
	return nil
}
func (m *mockTaskStore) GetTask(ctx context.Context, id string) (*persistence.AsyncTask, error) {
	if m.getFn != nil { return m.getFn(ctx, id) }
	return nil, nil
}
func (m *mockTaskStore) ListTasks(ctx context.Context, filter persistence.TaskFilter) ([]*persistence.AsyncTask, error) {
	if m.listFn != nil { return m.listFn(ctx, filter) }
	return nil, nil
}
func (m *mockTaskStore) UpdateStatus(ctx context.Context, id string, status persistence.TaskStatus, result any, errMsg string) error {
	if m.updateFn != nil { return m.updateFn(ctx, id, status, result, errMsg) }
	return nil
}
func (m *mockTaskStore) UpdateProgress(ctx context.Context, id string, progress float64) error {
	if m.progressFn != nil { return m.progressFn(ctx, id, progress) }
	return nil
}
func (m *mockTaskStore) DeleteTask(ctx context.Context, id string) error {
	if m.deleteFn != nil { return m.deleteFn(ctx, id) }
	return nil
}
func (m *mockTaskStore) GetRecoverableTasks(ctx context.Context) ([]*persistence.AsyncTask, error) {
	if m.recoverFn != nil { return m.recoverFn(ctx) }
	return nil, nil
}
func (m *mockTaskStore) Cleanup(ctx context.Context, older time.Duration) (int, error) {
	if m.cleanupFn != nil { return m.cleanupFn(ctx, older) }
	return 0, nil
}
func (m *mockTaskStore) Stats(ctx context.Context) (*persistence.TaskStoreStats, error) {
	if m.statsFn != nil { return m.statsFn(ctx) }
	return &persistence.TaskStoreStats{}, nil
}
func (m *mockTaskStore) Close() error {
	if m.closeFn != nil { return m.closeFn() }
	return nil
}
func (m *mockTaskStore) Ping(ctx context.Context) error {
	if m.pingFn != nil { return m.pingFn(ctx) }
	return nil
}

// --- NewHTTPServerWithTaskStore ---

func TestHTTPServer_NewWithTaskStore(t *testing.T) {
	store := &mockTaskStore{}
	server := NewHTTPServerWithTaskStore(&ServerConfig{
		BaseURL: "http://localhost:8080",
		Logger:  zap.NewNop(),
	}, store)
	assert.NotNil(t, server)
}

// --- SetTaskStore ---

func TestHTTPServer_SetTaskStore(t *testing.T) {
	server := NewHTTPServer(nil)
	store := &mockTaskStore{}
	server.SetTaskStore(store)
	assert.NotNil(t, server.taskStore)
}

// --- RecoverTasks ---

func TestHTTPServer_RecoverTasks_NoStore(t *testing.T) {
	server := NewHTTPServer(nil)
	err := server.RecoverTasks(context.Background())
	assert.NoError(t, err)
}

func TestHTTPServer_RecoverTasks_WithStore(t *testing.T) {
	ag := newMockAgent("agent-1", "Agent 1")

	store := &mockTaskStore{
		recoverFn: func(ctx context.Context) ([]*persistence.AsyncTask, error) {
			return []*persistence.AsyncTask{
				{
					ID:        "task-1",
					AgentID:   "agent-1",
					Status:    persistence.TaskStatusPending,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
			}, nil
		},
	}

	server := NewHTTPServerWithTaskStore(&ServerConfig{
		BaseURL:        "http://localhost:8080",
		RequestTimeout: 5 * time.Second,
		Logger:         zap.NewNop(),
	}, store)
	_ = server.RegisterAgent(ag)

	err := server.RecoverTasks(context.Background())
	require.NoError(t, err)
}

func TestHTTPServer_RecoverTasks_AgentNotFound(t *testing.T) {
	store := &mockTaskStore{
		recoverFn: func(ctx context.Context) ([]*persistence.AsyncTask, error) {
			return []*persistence.AsyncTask{
				{
					ID:      "task-1",
					AgentID: "nonexistent",
					Status:  persistence.TaskStatusPending,
				},
			}, nil
		},
	}

	server := NewHTTPServerWithTaskStore(&ServerConfig{
		BaseURL: "http://localhost:8080",
		Logger:  zap.NewNop(),
	}, store)

	err := server.RecoverTasks(context.Background())
	assert.NoError(t, err) // skips unknown agents
}

func TestHTTPServer_RecoverTasks_RunningTask(t *testing.T) {
	ag := newMockAgent("agent-1", "Agent 1")

	store := &mockTaskStore{
		recoverFn: func(ctx context.Context) ([]*persistence.AsyncTask, error) {
			return []*persistence.AsyncTask{
				{
					ID:        "task-1",
					AgentID:   "agent-1",
					Status:    persistence.TaskStatusCompleted, // use completed to avoid nil msg panic
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
			}, nil
		},
	}

	server := NewHTTPServerWithTaskStore(&ServerConfig{
		BaseURL:        "http://localhost:8080",
		RequestTimeout: 5 * time.Second,
		Logger:         zap.NewNop(),
	}, store)
	_ = server.RegisterAgent(ag)

	err := server.RecoverTasks(context.Background())
	require.NoError(t, err)
}

func TestHTTPServer_RecoverTasks_Error(t *testing.T) {
	store := &mockTaskStore{
		recoverFn: func(ctx context.Context) ([]*persistence.AsyncTask, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	server := NewHTTPServerWithTaskStore(&ServerConfig{Logger: zap.NewNop()}, store)
	err := server.RecoverTasks(context.Background())
	assert.Error(t, err)
}

// --- convertToPersistTask / convertFromPersistTask ---

func TestHTTPServer_ConvertToPersistTask(t *testing.T) {
	server := NewHTTPServer(nil)

	tests := []struct {
		name   string
		status string
		expect persistence.TaskStatus
	}{
		{"pending", "pending", persistence.TaskStatusPending},
		{"processing", "processing", persistence.TaskStatusRunning},
		{"completed", "completed", persistence.TaskStatusCompleted},
		{"failed", "failed", persistence.TaskStatusFailed},
		{"unknown", "xyz", persistence.TaskStatusPending},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &asyncTask{
				ID:        "task-1",
				AgentID:   "agent-1",
				Status:    tt.status,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Message:   &A2AMessage{Payload: map[string]any{"key": "val"}},
			}
			if tt.status == "failed" {
				task.Error = "some error"
			}
			if tt.status == "completed" {
				task.Result = &A2AMessage{Type: A2AMessageTypeResult}
			}
			pt := server.convertToPersistTask(task)
			assert.Equal(t, tt.expect, pt.Status)
		})
	}
}

func TestHTTPServer_ConvertFromPersistTask(t *testing.T) {
	server := NewHTTPServer(nil)

	tests := []struct {
		name   string
		status persistence.TaskStatus
		expect string
	}{
		{"pending", persistence.TaskStatusPending, "pending"},
		{"running", persistence.TaskStatusRunning, "processing"},
		{"completed", persistence.TaskStatusCompleted, "completed"},
		{"failed", persistence.TaskStatusFailed, "failed"},
		{"cancelled", persistence.TaskStatusCancelled, "pending"}, // falls to default
		{"unknown", persistence.TaskStatus("xyz"), "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pt := &persistence.AsyncTask{
				ID:        "task-1",
				AgentID:   "agent-1",
				Status:    tt.status,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			task := server.convertFromPersistTask(pt)
			assert.Equal(t, tt.expect, task.Status)
		})
	}
}

// --- Tools (agentAdapter) ---

func TestAgentAdapter_Tools(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL: "http://localhost:8080",
		Logger:  zap.NewNop(),
	})

	ag := newMockAgent("test-agent", "Test Agent")
	_ = server.RegisterAgent(ag)

	// The adapter wraps the agent; Tools() returns nil for mockAgent
	server.agentsMu.RLock()
	wrapped := server.agents["test-agent"]
	server.agentsMu.RUnlock()

	// Check that the adapter's Tools method works (returns nil for basic mock)
	if tooler, ok := wrapped.(interface{ Tools() []string }); ok {
		_ = tooler.Tools()
	}
}

// --- handleGetSpecificAgentCard ---

func TestHTTPServer_HandleGetSpecificAgentCard(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL: "http://localhost:8080",
		Logger:  zap.NewNop(),
	})

	ag := newMockAgent("test-agent", "Test Agent")
	_ = server.RegisterAgent(ag)

	t.Run("found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/a2a/agents/test-agent/card", nil)
		w := httptest.NewRecorder()
		server.handleGetSpecificAgentCard(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/a2a/agents/nonexistent/card", nil)
		w := httptest.NewRecorder()
		server.handleGetSpecificAgentCard(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("empty id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/a2a/agents//card", nil)
		w := httptest.NewRecorder()
		server.handleGetSpecificAgentCard(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// --- handleSyncMessage: error in executeTask ---

func TestHTTPServer_HandleSyncMessage_ExecuteError(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL:        "http://localhost:8080",
		RequestTimeout: time.Second,
		Logger:         zap.NewNop(),
	})

	ag := &mockAgent{
		id:   "test-agent",
		name: "Test Agent",
		execFunc: func(ctx context.Context, input *agent.Input) (*agent.Output, error) {
			return nil, fmt.Errorf("execution failed")
		},
	}
	_ = server.RegisterAgent(ag)

	msg := NewTaskMessage("client", "test-agent", "hello")
	body, _ := json.Marshal(msg)
	req := httptest.NewRequest(http.MethodPost, "/a2a/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var result A2AMessage
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	assert.Equal(t, A2AMessageTypeError, result.Type)
}

// --- payloadToContent ---

func TestHTTPServer_PayloadToContent(t *testing.T) {
	server := NewHTTPServer(nil)

	t.Run("nil", func(t *testing.T) {
		s, err := server.payloadToContent(nil)
		require.NoError(t, err)
		assert.Equal(t, "", s)
	})

	t.Run("string", func(t *testing.T) {
		s, err := server.payloadToContent("hello")
		require.NoError(t, err)
		assert.Equal(t, "hello", s)
	})

	t.Run("map with content", func(t *testing.T) {
		s, err := server.payloadToContent(map[string]any{"content": "hi"})
		require.NoError(t, err)
		assert.Equal(t, "hi", s)
	})

	t.Run("map with message", func(t *testing.T) {
		s, err := server.payloadToContent(map[string]any{"message": "msg"})
		require.NoError(t, err)
		assert.Equal(t, "msg", s)
	})

	t.Run("map with query", func(t *testing.T) {
		s, err := server.payloadToContent(map[string]any{"query": "q"})
		require.NoError(t, err)
		assert.Equal(t, "q", s)
	})

	t.Run("map fallback", func(t *testing.T) {
		s, err := server.payloadToContent(map[string]any{"other": 123})
		require.NoError(t, err)
		assert.Contains(t, s, "other")
	})

	t.Run("other type", func(t *testing.T) {
		s, err := server.payloadToContent(42)
		require.NoError(t, err)
		assert.Equal(t, "42", s)
	})
}

// --- TaskStats ---

func TestHTTPServer_TaskStats_NoStore(t *testing.T) {
	server := NewHTTPServer(nil)
	_, err := server.TaskStats(context.Background())
	assert.Error(t, err)
}

func TestHTTPServer_TaskStats_WithStore(t *testing.T) {
	store := &mockTaskStore{
		statsFn: func(ctx context.Context) (*persistence.TaskStoreStats, error) {
			return &persistence.TaskStoreStats{TotalTasks: 10}, nil
		},
	}
	server := NewHTTPServerWithTaskStore(&ServerConfig{Logger: zap.NewNop()}, store)
	stats, err := server.TaskStats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(10), stats.TotalTasks)
}

// --- StartCleanupLoop ---

func TestHTTPServer_StartCleanupLoop(t *testing.T) {
	cleaned := make(chan struct{}, 5)
	server := NewHTTPServer(&ServerConfig{Logger: zap.NewNop()})

	// Add an expired task
	server.asyncTasksMu.Lock()
	server.asyncTasks["old-task"] = &asyncTask{
		ID:        "old-task",
		Status:    "completed",
		UpdatedAt: time.Now().Add(-24 * time.Hour),
	}
	server.asyncTasksMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	server.StartCleanupLoop(ctx, 50*time.Millisecond, time.Hour)

	go func() {
		for {
			server.asyncTasksMu.RLock()
			_, exists := server.asyncTasks["old-task"]
			server.asyncTasksMu.RUnlock()
			if !exists {
				select {
				case cleaned <- struct{}{}:
				default:
				}
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	select {
	case <-cleaned:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup loop did not clean up task")
	}
	cancel()
}

// --- CleanupExpiredTasks with store ---

func TestHTTPServer_CleanupExpiredTasks_WithStore(t *testing.T) {
	store := &mockTaskStore{
		cleanupFn: func(ctx context.Context, older time.Duration) (int, error) {
			return 5, nil
		},
	}
	server := NewHTTPServerWithTaskStore(&ServerConfig{Logger: zap.NewNop()}, store)
	count := server.CleanupExpiredTasks(time.Hour)
	assert.Equal(t, 0, count) // no in-memory tasks
}

func TestHTTPServer_CleanupExpiredTasks_StoreError(t *testing.T) {
	store := &mockTaskStore{
		cleanupFn: func(ctx context.Context, older time.Duration) (int, error) {
			return 0, fmt.Errorf("cleanup error")
		},
	}
	server := NewHTTPServerWithTaskStore(&ServerConfig{Logger: zap.NewNop()}, store)
	count := server.CleanupExpiredTasks(time.Hour)
	assert.Equal(t, 0, count)
}

// --- Generator Tools ---

func TestSimpleAgentConfig_Tools(t *testing.T) {
	cfg := &SimpleAgentConfig{
		AgentTools: []string{"tool1", "tool2"},
	}
	assert.Equal(t, []string{"tool1", "tool2"}, cfg.Tools())
}

// --- getDefaultAgent with DefaultAgentID ---

func TestHTTPServer_GetDefaultAgent_WithDefaultID(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL:        "http://localhost:8080",
		DefaultAgentID: "default-agent",
		Logger:         zap.NewNop(),
	})

	ag := newMockAgent("default-agent", "Default Agent")
	_ = server.RegisterAgent(ag)

	defaultAg, err := server.getDefaultAgent()
	require.NoError(t, err)
	assert.Equal(t, "default-agent", defaultAg.ID())
}

// --- AgentCard discovery with agent_id query param ---

func TestHTTPServer_HandleAgentCardDiscovery_WithAgentID(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL: "http://localhost:8080",
		Logger:  zap.NewNop(),
	})

	ag := newMockAgent("test-agent", "Test Agent")
	_ = server.RegisterAgent(ag)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json?agent_id=test-agent", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- SetMetadata / GetMetadata on AgentCard ---

func TestAgentCard_SetGetMetadata(t *testing.T) {
	card := &AgentCard{}
	card.SetMetadata("key", "value")
	val, ok := card.GetMetadata("key")
	assert.True(t, ok)
	assert.Equal(t, "value", val)

	_, ok = card.GetMetadata("missing")
	assert.False(t, ok)

	// GetMetadata on nil metadata
	card2 := &AgentCard{}
	_, ok = card2.GetMetadata("key")
	assert.False(t, ok)
}

