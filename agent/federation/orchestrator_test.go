package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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
