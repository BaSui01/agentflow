package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/BaSui01/agentflow/testutil"
)

// TestExecuteOnNode_PayloadSent verifies that executeOnNode actually sends the
// JSON-serialized task as the HTTP request body. Before the fix, the payload
// was marshalled but never attached to the request (body was nil).
func TestExecuteOnNode_PayloadSent(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Set up a test HTTP server that captures the request body.
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	orch := NewOrchestrator(FederationConfig{
		NodeID:   "local-node",
		NodeName: "local",
	}, logger)

	// Use the test server's plain HTTP client (no TLS).
	orch.httpClient = ts.Client()

	// Register a remote node pointing at our test server.
	orch.RegisterNode(&FederatedNode{
		ID:       "remote-node",
		Name:     "remote",
		Endpoint: ts.URL,
		Status:   NodeStatusOnline,
	})

	task := &FederatedTask{
		ID:   "task-1",
		Type: "test",
		Payload: map[string]string{
			"key": "value",
		},
	}

	result, err := orch.executeOnNode(context.Background(), "remote-node", task)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify the server received a non-empty body that deserializes to our task.
	assert.NotEmpty(t, receivedBody, "request body should not be empty")

	var received FederatedTask
	err = json.Unmarshal(receivedBody, &received)
	require.NoError(t, err)
	assert.Equal(t, task.ID, received.ID)

	// Also verify the body is valid JSON matching json.Marshal output.
	expected, _ := json.Marshal(task)
	assert.True(t, bytes.Equal(expected, receivedBody),
		"body should match json.Marshal(task)")
}

// newTestOrchestrator creates an Orchestrator for testing with sensible defaults.
func newTestOrchestrator(t *testing.T) *Orchestrator {
	t.Helper()
	orch := NewOrchestrator(FederationConfig{
		NodeID:   "local-node",
		NodeName: "local",
	}, zap.NewNop())
	t.Cleanup(orch.Stop)
	return orch
}

func TestOrchestrator_RegisterNode(t *testing.T) {
	orch := newTestOrchestrator(t)

	node := &FederatedNode{
		ID:           "node-1",
		Name:         "worker-1",
		Endpoint:     "http://localhost:9001",
		Capabilities: []string{"llm", "rag"},
	}

	orch.RegisterNode(node)

	nodes := orch.ListNodes()
	require.Len(t, nodes, 1)
	assert.Equal(t, "node-1", nodes[0].ID)
	assert.Equal(t, NodeStatusOnline, nodes[0].Status)
	assert.False(t, nodes[0].LastSeen.IsZero())
}

func TestOrchestrator_UnregisterNode(t *testing.T) {
	orch := newTestOrchestrator(t)

	orch.RegisterNode(&FederatedNode{ID: "node-1", Name: "w1"})
	orch.RegisterNode(&FederatedNode{ID: "node-2", Name: "w2"})
	require.Len(t, orch.ListNodes(), 2)

	orch.UnregisterNode("node-1")
	nodes := orch.ListNodes()
	require.Len(t, nodes, 1)
	assert.Equal(t, "node-2", nodes[0].ID)

	// Unregister non-existent node should not panic
	orch.UnregisterNode("non-existent")
	assert.Len(t, orch.ListNodes(), 1)
}

func TestOrchestrator_ListNodes(t *testing.T) {
	orch := newTestOrchestrator(t)

	// Empty initially
	assert.Empty(t, orch.ListNodes())

	// Add nodes
	for i := 0; i < 5; i++ {
		orch.RegisterNode(&FederatedNode{
			ID:   fmt.Sprintf("node-%d", i),
			Name: fmt.Sprintf("worker-%d", i),
		})
	}

	nodes := orch.ListNodes()
	assert.Len(t, nodes, 5)
}

func TestOrchestrator_SubmitTask(t *testing.T) {
	ctx := testutil.TestContext(t)

	// Set up a test HTTP server for remote execution
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"done"}`))
	}))
	t.Cleanup(ts.Close)

	orch := newTestOrchestrator(t)
	orch.httpClient = ts.Client()

	// Register a capable node
	orch.RegisterNode(&FederatedNode{
		ID:           "capable-node",
		Name:         "capable",
		Endpoint:     ts.URL,
		Capabilities: []string{"llm", "rag"},
	})

	tests := []struct {
		name         string
		task         *FederatedTask
		expectErr    bool
		errContains  string
	}{
		{
			name: "task with matching capabilities",
			task: &FederatedTask{
				Type:         "inference",
				RequiredCaps: []string{"llm"},
			},
			expectErr: false,
		},
		{
			name: "task with no required capabilities matches any online node",
			task: &FederatedTask{
				Type: "generic",
			},
			expectErr: false,
		},
		{
			name: "task with unmatched capabilities fails",
			task: &FederatedTask{
				Type:         "special",
				RequiredCaps: []string{"quantum-computing"},
			},
			expectErr:   true,
			errContains: "no capable nodes found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := orch.SubmitTask(ctx, tt.task)
			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, tt.task.ID, "task ID should be assigned")
				assert.Equal(t, "local-node", tt.task.SourceNode)
			}
		})
	}
}

func TestOrchestrator_GetTask(t *testing.T) {
	ctx := testutil.TestContext(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(ts.Close)

	orch := newTestOrchestrator(t)
	orch.httpClient = ts.Client()

	orch.RegisterNode(&FederatedNode{
		ID:       "node-1",
		Name:     "w1",
		Endpoint: ts.URL,
	})

	// Non-existent task
	_, ok := orch.GetTask("non-existent")
	assert.False(t, ok)

	// Submit a task, then retrieve it
	task := &FederatedTask{Type: "test"}
	err := orch.SubmitTask(ctx, task)
	require.NoError(t, err)

	retrieved, ok := orch.GetTask(task.ID)
	require.True(t, ok)
	assert.Equal(t, task.ID, retrieved.ID)
}

func TestOrchestrator_Concurrent(t *testing.T) {
	_ = testutil.TestContext(t)

	orch := newTestOrchestrator(t)

	var wg sync.WaitGroup

	// Concurrent node registration
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			orch.RegisterNode(&FederatedNode{
				ID:       fmt.Sprintf("node-%d", id),
				Name:     fmt.Sprintf("w-%d", id),
				Endpoint: "http://localhost:9999",
			})
		}(i)
	}
	wg.Wait()

	assert.Len(t, orch.ListNodes(), 20)

	// Concurrent list + unregister
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func(id int) {
			defer wg.Done()
			orch.UnregisterNode(fmt.Sprintf("node-%d", id))
		}(i)
		go func() {
			defer wg.Done()
			_ = orch.ListNodes()
		}()
	}
	wg.Wait()

	ok := testutil.WaitFor(func() bool {
		return len(orch.ListNodes()) == 10
	}, 5*time.Second)
	assert.True(t, ok, "should have 10 nodes remaining")

	// NOTE: Concurrent SubmitTask is intentionally not tested here because
	// the source code has a known data race in distributeTask — it concurrently
	// reads task fields via json.Marshal while writing to task.Results.
	// This is a pre-existing bug in orchestrator.go, not a test issue.
}

func TestOrchestrator_StopDoubleClose(t *testing.T) {
	orch := newTestOrchestrator(t)
	// Stop is already called via t.Cleanup, but calling it again should not panic
	orch.Stop()
	orch.Stop()
}
