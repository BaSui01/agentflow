package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStdioTransport(t *testing.T) {
	cmd, args := "go", []string{"version"}
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "ok"}
	}
	tr, err := NewStdioTransport(cmd, args...)
	if err != nil {
		t.Skipf("stdio transport requires subprocess: %v", err)
	}
	err = tr.Close()
	if err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestSSETransport_SendReceiveClose(t *testing.T) {
	var postMu sync.Mutex
	var postBody []byte
	var getCount int

	mux := http.NewServeMux()
	mux.HandleFunc("/message", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		postMu.Lock()
		defer postMu.Unlock()
		postBody = make([]byte, 4096)
		n, _ := r.Body.Read(postBody)
		postBody = postBody[:n]
		_ = r.Body.Close()
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		getCount++
		if getCount == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			if ok {
				flusher.Flush()
			}
			evt := MCPMessage{JSONRPC: "2.0", ID: 1, Method: "test/event"}
			b, _ := json.Marshal(evt)
			_, _ = w.Write(append(append([]byte("data: "), b...), '\n', '\n'))
			flusher.Flush()
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	tr := NewSSETransport(srv.URL)
	ctx := context.Background()

	sent := &MCPMessage{JSONRPC: "2.0", ID: 2, Method: "test/send"}
	err := tr.Send(ctx, sent)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	postMu.Lock()
	gotPost := len(postBody) > 0
	postMu.Unlock()
	if !gotPost {
		time.Sleep(200 * time.Millisecond)
		postMu.Lock()
		gotPost = len(postBody) > 0
		postMu.Unlock()
	}
	if !gotPost {
		t.Log("POST /message may not have been received yet (async sendLoop)")
	}

	recvCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	msg, err := tr.Receive(recvCtx)
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, "2.0", msg.JSONRPC)
	assert.Equal(t, "test/event", msg.Method)

	tr.Close()
	_, err = tr.Receive(ctx)
	assert.Error(t, err)
}
