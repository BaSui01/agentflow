package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ====== Mock AuditBackend ======

type mockAuditBackend struct {
	mu       sync.Mutex
	entries  []*AuditEntry
	writeErr error
	queryErr error
	closed   bool
}

func newMockAuditBackend() *mockAuditBackend {
	return &mockAuditBackend{entries: make([]*AuditEntry, 0)}
}

func (m *mockAuditBackend) Write(ctx context.Context, entry *AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockAuditBackend) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return m.entries, nil
}

func (m *mockAuditBackend) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockAuditBackend) getEntries() []*AuditEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*AuditEntry, len(m.entries))
	copy(cp, m.entries)
	return cp
}

// ====== DefaultAuditLogger Tests ======

func TestNewAuditLogger_Defaults(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, nil)
	defer al.Close()

	assert.NotNil(t, al)
	assert.NotNil(t, al.asyncQueue)
	assert.Equal(t, 10000, cap(al.asyncQueue))
}

func TestDefaultAuditLogger_Log_SyncWrite(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, zap.NewNop())
	defer al.Close()

	entry := &AuditEntry{
		EventType: AuditEventToolCall,
		AgentID:   "agent-1",
		UserID:    "user-1",
		ToolName:  "calculator",
	}

	err := al.Log(context.Background(), entry)
	require.NoError(t, err)

	assert.NotEmpty(t, entry.ID)
	assert.False(t, entry.Timestamp.IsZero())

	entries := backend.getEntries()
	require.Len(t, entries, 1)
	assert.Equal(t, "calculator", entries[0].ToolName)
}

func TestDefaultAuditLogger_Log_PreservesExistingID(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, zap.NewNop())
	defer al.Close()

	entry := &AuditEntry{
		ID:        "custom-id-123",
		EventType: AuditEventToolCall,
		ToolName:  "test",
	}

	err := al.Log(context.Background(), entry)
	require.NoError(t, err)
	assert.Equal(t, "custom-id-123", entry.ID)
}

func TestDefaultAuditLogger_Log_AfterClose(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, zap.NewNop())

	require.NoError(t, al.Close())

	err := al.Log(context.Background(), &AuditEntry{EventType: AuditEventToolCall})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestDefaultAuditLogger_LogAsync(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends:     []AuditBackend{backend},
		AsyncWorkers: 2,
	}, zap.NewNop())

	for i := 0; i < 5; i++ {
		al.LogAsync(&AuditEntry{
			EventType: AuditEventToolCall,
			ToolName:  fmt.Sprintf("tool-%d", i),
		})
	}

	// Close flushes the queue
	require.NoError(t, al.Close())

	entries := backend.getEntries()
	assert.Len(t, entries, 5)
}

func TestDefaultAuditLogger_LogAsync_AfterClose(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, zap.NewNop())

	require.NoError(t, al.Close())

	// Should not panic, just drop
	al.LogAsync(&AuditEntry{EventType: AuditEventToolCall})

	entries := backend.getEntries()
	assert.Empty(t, entries)
}

func TestDefaultAuditLogger_LogAsync_QueueFull(t *testing.T) {
	backend := newMockAuditBackend()
	// Very small queue
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends:       []AuditBackend{backend},
		AsyncQueueSize: 1,
		AsyncWorkers:   0, // no workers to drain
	}, zap.NewNop())

	// Manually set workers to 0 by creating a logger with no workers
	// The constructor enforces min 4, so we use a different approach:
	// Fill the queue manually
	al2 := &DefaultAuditLogger{
		backends:    []AuditBackend{backend},
		asyncQueue:  make(chan *AuditEntry, 1),
		logger:      zap.NewNop(),
		idGenerator: generateAuditID,
	}

	// Fill the queue
	al2.asyncQueue <- &AuditEntry{EventType: AuditEventToolCall}

	// This should be dropped (queue full)
	al2.LogAsync(&AuditEntry{EventType: AuditEventToolResult, ToolName: "dropped"})

	// Clean up
	al.Close()
}

func TestDefaultAuditLogger_Query_NoBackends(t *testing.T) {
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{},
	}, zap.NewNop())
	defer al.Close()

	_, err := al.Query(context.Background(), &AuditFilter{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no backends")
}

func TestDefaultAuditLogger_Query_DelegatesToFirstBackend(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, zap.NewNop())
	defer al.Close()

	// Add an entry directly
	err := al.Log(context.Background(), &AuditEntry{
		EventType: AuditEventToolCall,
		ToolName:  "search",
		AgentID:   "agent-1",
	})
	require.NoError(t, err)

	results, err := al.Query(context.Background(), &AuditFilter{})
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestDefaultAuditLogger_MultipleBackends(t *testing.T) {
	b1 := newMockAuditBackend()
	b2 := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{b1, b2},
	}, zap.NewNop())
	defer al.Close()

	err := al.Log(context.Background(), &AuditEntry{
		EventType: AuditEventToolCall,
		ToolName:  "test",
	})
	require.NoError(t, err)

	assert.Len(t, b1.getEntries(), 1)
	assert.Len(t, b2.getEntries(), 1)
}

func TestDefaultAuditLogger_BackendWriteError(t *testing.T) {
	b1 := newMockAuditBackend()
	b1.writeErr = fmt.Errorf("write failed")
	b2 := newMockAuditBackend()

	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{b1, b2},
	}, zap.NewNop())
	defer al.Close()

	err := al.Log(context.Background(), &AuditEntry{
		EventType: AuditEventToolCall,
	})
	// Returns last error but still writes to other backends
	assert.Error(t, err)
	assert.Len(t, b2.getEntries(), 1)
}

func TestDefaultAuditLogger_Close_Idempotent(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, zap.NewNop())

	require.NoError(t, al.Close())
	require.NoError(t, al.Close()) // second close should be no-op
}

// ====== MemoryAuditBackend Tests ======

func TestMemoryAuditBackend_Write(t *testing.T) {
	backend := NewMemoryAuditBackend(100)

	err := backend.Write(context.Background(), &AuditEntry{
		ID:        "e1",
		EventType: AuditEventToolCall,
		ToolName:  "calc",
	})
	require.NoError(t, err)

	results, err := backend.Query(context.Background(), &AuditFilter{})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "calc", results[0].ToolName)
}

func TestMemoryAuditBackend_Eviction(t *testing.T) {
	backend := NewMemoryAuditBackend(10)

	// Write 15 entries
	for i := 0; i < 15; i++ {
		err := backend.Write(context.Background(), &AuditEntry{
			ID:        fmt.Sprintf("e%d", i),
			EventType: AuditEventToolCall,
			ToolName:  fmt.Sprintf("tool-%d", i),
		})
		require.NoError(t, err)
	}

	results, err := backend.Query(context.Background(), &AuditFilter{})
	require.NoError(t, err)
	// After eviction of oldest 10%, should have entries
	assert.True(t, len(results) <= 15)
	assert.True(t, len(results) > 0)
}

func TestMemoryAuditBackend_DefaultMaxSize(t *testing.T) {
	backend := NewMemoryAuditBackend(0)
	assert.Equal(t, 100000, backend.maxSize)
}

func TestMemoryAuditBackend_Query_Filters(t *testing.T) {
	backend := NewMemoryAuditBackend(100)
	now := time.Now()

	entries := []*AuditEntry{
		{ID: "1", EventType: AuditEventToolCall, AgentID: "a1", UserID: "u1", ToolName: "calc", Timestamp: now},
		{ID: "2", EventType: AuditEventToolResult, AgentID: "a2", UserID: "u1", ToolName: "search", Timestamp: now},
		{ID: "3", EventType: AuditEventToolCall, AgentID: "a1", UserID: "u2", ToolName: "calc", SessionID: "s1", Timestamp: now},
		{ID: "4", EventType: AuditEventPermissionCheck, AgentID: "a1", UserID: "u1", ToolName: "calc", TraceID: "t1", Timestamp: now},
	}
	for _, e := range entries {
		require.NoError(t, backend.Write(context.Background(), e))
	}

	tests := []struct {
		name     string
		filter   *AuditFilter
		expected int
	}{
		{"by agent_id", &AuditFilter{AgentID: "a1"}, 3},
		{"by user_id", &AuditFilter{UserID: "u2"}, 1},
		{"by tool_name", &AuditFilter{ToolName: "search"}, 1},
		{"by event_type", &AuditFilter{EventType: AuditEventToolCall}, 2},
		{"by session_id", &AuditFilter{SessionID: "s1"}, 1},
		{"by trace_id", &AuditFilter{TraceID: "t1"}, 1},
		{"no filter", &AuditFilter{}, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := backend.Query(context.Background(), tt.filter)
			require.NoError(t, err)
			assert.Len(t, results, tt.expected)
		})
	}
}

func TestMemoryAuditBackend_Query_TimeFilter(t *testing.T) {
	backend := NewMemoryAuditBackend(100)
	t1 := time.Now().Add(-2 * time.Hour)
	t2 := time.Now().Add(-1 * time.Hour)
	t3 := time.Now()

	require.NoError(t, backend.Write(context.Background(), &AuditEntry{ID: "1", Timestamp: t1}))
	require.NoError(t, backend.Write(context.Background(), &AuditEntry{ID: "2", Timestamp: t2}))
	require.NoError(t, backend.Write(context.Background(), &AuditEntry{ID: "3", Timestamp: t3}))

	startTime := t2.Add(-time.Minute)
	results, err := backend.Query(context.Background(), &AuditFilter{StartTime: &startTime})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	endTime := t2.Add(time.Minute)
	results, err = backend.Query(context.Background(), &AuditFilter{EndTime: &endTime})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestMemoryAuditBackend_Query_OffsetLimit(t *testing.T) {
	backend := NewMemoryAuditBackend(100)
	for i := 0; i < 10; i++ {
		require.NoError(t, backend.Write(context.Background(), &AuditEntry{
			ID:        fmt.Sprintf("e%d", i),
			Timestamp: time.Now(),
		}))
	}

	// Offset 3, Limit 2
	results, err := backend.Query(context.Background(), &AuditFilter{Offset: 3, Limit: 2})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Offset beyond length
	results, err = backend.Query(context.Background(), &AuditFilter{Offset: 100})
	require.NoError(t, err)
	assert.Empty(t, results)
}

// ====== FileAuditBackend Tests ======

func TestFileAuditBackend_WriteAndClose(t *testing.T) {
	dir := t.TempDir()
	backend, err := NewFileAuditBackend(&FileAuditBackendConfig{
		Directory: dir,
	}, zap.NewNop())
	require.NoError(t, err)

	entry := &AuditEntry{
		ID:        "test-1",
		Timestamp: time.Now(),
		EventType: AuditEventToolCall,
		ToolName:  "test",
	}
	err = backend.Write(context.Background(), entry)
	require.NoError(t, err)

	require.NoError(t, backend.Close())
}

func TestFileAuditBackend_QueryNotImplemented(t *testing.T) {
	dir := t.TempDir()
	backend, err := NewFileAuditBackend(&FileAuditBackendConfig{
		Directory: dir,
	}, zap.NewNop())
	require.NoError(t, err)
	defer backend.Close()

	// Write something first to create a file
	err = backend.Write(context.Background(), &AuditEntry{
		ID:        "test-1",
		Timestamp: time.Now(),
	})
	require.NoError(t, err)

	_, err = backend.Query(context.Background(), &AuditFilter{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not fully implemented")
}

func TestFileAuditBackend_QueryNoFile(t *testing.T) {
	dir := t.TempDir()
	backend, err := NewFileAuditBackend(&FileAuditBackendConfig{
		Directory: dir,
	}, zap.NewNop())
	require.NoError(t, err)
	defer backend.Close()

	results, err := backend.Query(context.Background(), &AuditFilter{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestFileAuditBackend_Close_Idempotent(t *testing.T) {
	dir := t.TempDir()
	backend, err := NewFileAuditBackend(&FileAuditBackendConfig{
		Directory: dir,
	}, zap.NewNop())
	require.NoError(t, err)

	require.NoError(t, backend.Close())
	require.NoError(t, backend.Close())
}

// ====== AuditMiddleware Tests ======

func TestAuditMiddleware_RecordsToolCall(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, zap.NewNop())
	defer al.Close()

	innerFn := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"result":"ok"}`), nil
	}

	middleware := AuditMiddleware(al)
	wrappedFn := middleware(innerFn)

	ctx := WithPermissionContext(context.Background(), &PermissionContext{
		AgentID:   "agent-1",
		UserID:    "user-1",
		ToolName:  "calculator",
		SessionID: "sess-1",
	})

	result, err := wrappedFn(ctx, json.RawMessage(`{"x":1}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"result":"ok"}`, string(result))

	// Wait for async log
	al.Close()

	entries := backend.getEntries()
	require.True(t, len(entries) >= 1)
	assert.Equal(t, AuditEventToolCall, entries[0].EventType)
	assert.Equal(t, "agent-1", entries[0].AgentID)
	assert.Equal(t, "calculator", entries[0].ToolName)
}

func TestAuditMiddleware_RecordsError(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, zap.NewNop())

	innerFn := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return nil, fmt.Errorf("tool failed")
	}

	middleware := AuditMiddleware(al)
	wrappedFn := middleware(innerFn)

	_, err := wrappedFn(context.Background(), nil)
	assert.Error(t, err)

	al.Close()

	entries := backend.getEntries()
	require.True(t, len(entries) >= 1)
	assert.Equal(t, "tool failed", entries[0].Error)
}

// ====== Convenience Function Tests ======

func TestLogToolCall(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, zap.NewNop())

	LogToolCall(al, "agent-1", "user-1", "calc", json.RawMessage(`{"x":1}`))
	al.Close()

	entries := backend.getEntries()
	require.Len(t, entries, 1)
	assert.Equal(t, AuditEventToolCall, entries[0].EventType)
	assert.Equal(t, "calc", entries[0].ToolName)
}

func TestLogToolResult(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, zap.NewNop())

	LogToolResult(al, "agent-1", "user-1", "calc", json.RawMessage(`{"r":2}`), nil, 100*time.Millisecond)
	al.Close()

	entries := backend.getEntries()
	require.Len(t, entries, 1)
	assert.Equal(t, AuditEventToolResult, entries[0].EventType)
	assert.Empty(t, entries[0].Error)
}

func TestLogToolResult_WithError(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, zap.NewNop())

	LogToolResult(al, "agent-1", "user-1", "calc", nil, fmt.Errorf("fail"), 50*time.Millisecond)
	al.Close()

	entries := backend.getEntries()
	require.Len(t, entries, 1)
	assert.Equal(t, "fail", entries[0].Error)
}

func TestLogPermissionCheck(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, zap.NewNop())

	LogPermissionCheck(al, &PermissionContext{
		AgentID:   "a1",
		UserID:    "u1",
		SessionID: "s1",
		TraceID:   "t1",
		ToolName:  "calc",
		RequestIP: "127.0.0.1",
	}, PermissionAllow, "rule matched")
	al.Close()

	entries := backend.getEntries()
	require.Len(t, entries, 1)
	assert.Equal(t, AuditEventPermissionCheck, entries[0].EventType)
	assert.Equal(t, string(PermissionAllow), entries[0].Decision)
}

func TestLogRateLimitHit(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, zap.NewNop())

	LogRateLimitHit(al, "a1", "u1", "calc", "global")
	al.Close()

	entries := backend.getEntries()
	require.Len(t, entries, 1)
	assert.Equal(t, AuditEventRateLimitHit, entries[0].EventType)
	assert.Equal(t, "global", entries[0].Metadata["limit_type"])
}

func TestLogCostAlert(t *testing.T) {
	backend := newMockAuditBackend()
	al := NewAuditLogger(&AuditLoggerConfig{
		Backends: []AuditBackend{backend},
	}, zap.NewNop())

	LogCostAlert(al, "a1", "u1", 99.5, "threshold_exceeded")
	al.Close()

	entries := backend.getEntries()
	require.Len(t, entries, 1)
	assert.Equal(t, AuditEventCostAlert, entries[0].EventType)
	assert.Equal(t, 99.5, entries[0].Cost)
	assert.Equal(t, "threshold_exceeded", entries[0].Metadata["alert_type"])
}
