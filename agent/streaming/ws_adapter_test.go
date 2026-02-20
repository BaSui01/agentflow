package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

// --- Interface compliance ---

func TestWebSocketStreamConnection_ImplementsStreamConnection(t *testing.T) {
	var _ StreamConnection = (*WebSocketStreamConnection)(nil)
}

// --- Helpers ---

// wsTestServer creates an httptest.Server that upgrades to WebSocket and
// echoes every message back to the client.
func wsTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")

		for {
			typ, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			if err := conn.Write(r.Context(), typ, data); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func dialConn(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL(srv), nil)
	require.NoError(t, err)
	return conn
}

// --- Tests ---

func TestWebSocketStreamConnection_ReadWriteRoundTrip(t *testing.T) {
	srv := wsTestServer(t)
	conn := dialConn(t, srv)

	ws := NewWebSocketStreamConnection(conn, nil)
	t.Cleanup(func() { _ = ws.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sent := StreamChunk{
		ID:        "chunk-1",
		Type:      StreamTypeText,
		Text:      "hello world",
		Timestamp: time.Now().Truncate(time.Millisecond),
		Sequence:  1,
		IsFinal:   false,
		Metadata:  map[string]any{"key": "value"},
	}

	err := ws.WriteChunk(ctx, sent)
	require.NoError(t, err)

	received, err := ws.ReadChunk(ctx)
	require.NoError(t, err)
	require.NotNil(t, received)

	assert.Equal(t, sent.ID, received.ID)
	assert.Equal(t, sent.Type, received.Type)
	assert.Equal(t, sent.Text, received.Text)
	assert.Equal(t, sent.Sequence, received.Sequence)
	assert.Equal(t, sent.IsFinal, received.IsFinal)
}

func TestWebSocketStreamConnection_IsAlive(t *testing.T) {
	srv := wsTestServer(t)
	conn := dialConn(t, srv)

	ws := NewWebSocketStreamConnection(conn, nil)

	assert.True(t, ws.IsAlive())

	err := ws.Close()
	require.NoError(t, err)

	assert.False(t, ws.IsAlive())
}

func TestWebSocketStreamConnection_CloseIdempotent(t *testing.T) {
	srv := wsTestServer(t)
	conn := dialConn(t, srv)

	ws := NewWebSocketStreamConnection(conn, nil)

	err := ws.Close()
	require.NoError(t, err)

	// Second close should be a no-op, not an error.
	err = ws.Close()
	assert.NoError(t, err)
}

func TestWebSocketStreamConnection_WriteAfterClose(t *testing.T) {
	srv := wsTestServer(t)
	conn := dialConn(t, srv)

	ws := NewWebSocketStreamConnection(conn, nil)
	_ = ws.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := ws.WriteChunk(ctx, StreamChunk{ID: "x", Type: StreamTypeText})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection closed")
}

func TestWebSocketStreamConnection_ReadAfterClose(t *testing.T) {
	srv := wsTestServer(t)
	conn := dialConn(t, srv)

	ws := NewWebSocketStreamConnection(conn, nil)
	_ = ws.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := ws.ReadChunk(ctx)
	assert.Error(t, err)
}

func TestWebSocketStreamConnection_ConcurrentWrites(t *testing.T) {
	srv := wsTestServer(t)
	conn := dialConn(t, srv)

	ws := NewWebSocketStreamConnection(conn, nil)
	t.Cleanup(func() { _ = ws.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(seq int) {
			defer wg.Done()
			chunk := StreamChunk{
				ID:       fmt.Sprintf("chunk-%d", seq),
				Type:     StreamTypeText,
				Text:     fmt.Sprintf("msg-%d", seq),
				Sequence: int64(seq),
			}
			_ = ws.WriteChunk(ctx, chunk)
		}(i)
	}

	wg.Wait()
	// If we get here without a panic or data race, the mutex is working.
}

func TestWebSocketStreamConnection_BinaryChunkData(t *testing.T) {
	srv := wsTestServer(t)
	conn := dialConn(t, srv)

	ws := NewWebSocketStreamConnection(conn, nil)
	t.Cleanup(func() { _ = ws.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sent := StreamChunk{
		ID:   "audio-1",
		Type: StreamTypeAudio,
		Data: []byte{0x00, 0xFF, 0x80, 0x7F, 0x01},
	}

	err := ws.WriteChunk(ctx, sent)
	require.NoError(t, err)

	received, err := ws.ReadChunk(ctx)
	require.NoError(t, err)

	assert.Equal(t, sent.Data, received.Data)
	assert.Equal(t, StreamTypeAudio, received.Type)
}

func TestWebSocketStreamFactory(t *testing.T) {
	srv := wsTestServer(t)

	factory := WebSocketStreamFactory(wsURL(srv), nil)

	conn, err := factory()
	require.NoError(t, err)
	require.NotNil(t, conn)

	assert.True(t, conn.IsAlive())

	err = conn.Close()
	assert.NoError(t, err)
	assert.False(t, conn.IsAlive())
}

func TestWebSocketStreamFactory_InvalidURL(t *testing.T) {
	factory := WebSocketStreamFactory("ws://localhost:1", nil)

	conn, err := factory()
	assert.Error(t, err)
	assert.Nil(t, conn)
}

func TestWebSocketStreamConnection_ReadChunk_InvalidJSON(t *testing.T) {
	// Server that sends invalid JSON.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		_ = conn.Write(r.Context(), websocket.MessageText, []byte("not-json"))
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsConn, _, err := websocket.Dial(ctx, wsURL(srv), nil)
	require.NoError(t, err)

	ws := NewWebSocketStreamConnection(wsConn, nil)
	t.Cleanup(func() { _ = ws.Close() })

	_, err = ws.ReadChunk(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal chunk")
}

func TestWebSocketStreamConnection_JSONRoundTrip_Fidelity(t *testing.T) {
	srv := wsTestServer(t)
	conn := dialConn(t, srv)

	ws := NewWebSocketStreamConnection(conn, nil)
	t.Cleanup(func() { _ = ws.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sent := StreamChunk{
		ID:        "fidelity-1",
		Type:      StreamTypeMixed,
		Data:      []byte("binary-payload"),
		Text:      "text-payload",
		Timestamp: time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC),
		Sequence:  42,
		IsFinal:   true,
		Metadata:  map[string]any{"nested": map[string]any{"a": float64(1)}},
	}

	err := ws.WriteChunk(ctx, sent)
	require.NoError(t, err)

	received, err := ws.ReadChunk(ctx)
	require.NoError(t, err)

	// Compare via JSON to handle any type normalization.
	sentJSON, _ := json.Marshal(sent)
	recvJSON, _ := json.Marshal(received)
	assert.JSONEq(t, string(sentJSON), string(recvJSON))
}
