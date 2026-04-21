package streaming

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- mock StreamConnection ---

type mockConn struct {
	mu        sync.Mutex
	readFn    func(ctx context.Context) (*StreamChunk, error)
	writeFn   func(ctx context.Context, chunk StreamChunk) error
	closeFn   func() error
	isAliveFn func() bool
	closed    bool
}

func (c *mockConn) ReadChunk(ctx context.Context) (*StreamChunk, error) {
	if c.readFn != nil {
		return c.readFn(ctx)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (c *mockConn) WriteChunk(ctx context.Context, chunk StreamChunk) error {
	if c.writeFn != nil {
		return c.writeFn(ctx, chunk)
	}
	return nil
}

func (c *mockConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.closeFn != nil {
		return c.closeFn()
	}
	return nil
}

func (c *mockConn) IsAlive() bool {
	if c.isAliveFn != nil {
		return c.isAliveFn()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed
}

// --- mock StreamHandler ---

type mockHandler struct {
	mu           sync.Mutex
	stateChanges []StreamState
	onInboundFn  func(ctx context.Context, chunk StreamChunk) (*StreamChunk, error)
	onOutboundFn func(ctx context.Context, chunk StreamChunk) error
}

func (h *mockHandler) OnInbound(ctx context.Context, chunk StreamChunk) (*StreamChunk, error) {
	if h.onInboundFn != nil {
		return h.onInboundFn(ctx, chunk)
	}
	return &chunk, nil
}

func (h *mockHandler) OnOutbound(ctx context.Context, chunk StreamChunk) error {
	if h.onOutboundFn != nil {
		return h.onOutboundFn(ctx, chunk)
	}
	return nil
}

func (h *mockHandler) OnStateChange(state StreamState) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stateChanges = append(h.stateChanges, state)
}

func (h *mockHandler) getStateChanges() []StreamState {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make([]StreamState, len(h.stateChanges))
	copy(cp, h.stateChanges)
	return cp
}

// --- Tests ---

func TestDefaultStreamConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultStreamConfig()
	assert.Equal(t, 1024, cfg.BufferSize)
	assert.Equal(t, 200, cfg.MaxLatencyMS)
	assert.Equal(t, 16000, cfg.SampleRate)
	assert.True(t, cfg.EnableVAD)
	assert.True(t, cfg.EnableHeartbeat)
	assert.Equal(t, 5, cfg.MaxReconnects)
}

func TestNewBidirectionalStream(t *testing.T) {
	t.Parallel()
	conn := &mockConn{}
	handler := &mockHandler{}
	cfg := DefaultStreamConfig()

	stream := NewBidirectionalStream(cfg, handler, conn, nil, nil)
	require.NotNil(t, stream)
	assert.Equal(t, StateDisconnected, stream.GetState())
	assert.NotEmpty(t, stream.ID)
}

func TestBidirectionalStream_Start_NoConnection(t *testing.T) {
	t.Parallel()
	cfg := DefaultStreamConfig()
	stream := NewBidirectionalStream(cfg, nil, nil, nil, zap.NewNop())

	err := stream.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no connection")
	assert.Equal(t, StateError, stream.GetState())
}

func TestBidirectionalStream_Start_WithConnFactory(t *testing.T) {
	t.Parallel()
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false

	factory := func() (StreamConnection, error) { return conn, nil }
	stream := NewBidirectionalStream(cfg, nil, nil, factory, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := stream.Start(ctx)
	require.NoError(t, err)
	assert.Equal(t, StateStreaming, stream.GetState())

	cancel()
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, stream.Close())
}

func TestBidirectionalStream_Start_FactoryError(t *testing.T) {
	t.Parallel()
	cfg := DefaultStreamConfig()
	factory := func() (StreamConnection, error) { return nil, errors.New("conn failed") }
	stream := NewBidirectionalStream(cfg, nil, nil, factory, zap.NewNop())

	err := stream.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conn failed")
}

func TestBidirectionalStream_Send(t *testing.T) {
	t.Parallel()
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, nil, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	err := stream.Send(StreamChunk{Type: StreamTypeText, Text: "hello"})
	require.NoError(t, err)

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

func TestBidirectionalStream_Send_BufferFull(t *testing.T) {
	t.Parallel()
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	cfg.BufferSize = 1
	stream := NewBidirectionalStream(cfg, nil, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	// Fill the buffer
	require.NoError(t, stream.Send(StreamChunk{Type: StreamTypeText, Text: "first"}))

	// Second send should fail with buffer full
	err := stream.Send(StreamChunk{Type: StreamTypeText, Text: "second"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "buffer full")

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

func TestBidirectionalStream_Close_Idempotent(t *testing.T) {
	t.Parallel()
	conn := &mockConn{}
	cfg := DefaultStreamConfig()
	stream := NewBidirectionalStream(cfg, nil, conn, nil, zap.NewNop())

	require.NoError(t, stream.Close())
	require.NoError(t, stream.Close()) // second close should be no-op
}

func TestBidirectionalStream_StateTransitions(t *testing.T) {
	t.Parallel()
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	handler := &mockHandler{}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, handler, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	states := handler.getStateChanges()
	assert.Contains(t, states, StateConnecting)
	assert.Contains(t, states, StateConnected)
	assert.Contains(t, states, StateStreaming)

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

func TestBidirectionalStream_Receive(t *testing.T) {
	t.Parallel()
	stream := NewBidirectionalStream(DefaultStreamConfig(), nil, &mockConn{}, nil, zap.NewNop())
	ch := stream.Receive()
	assert.NotNil(t, ch)
}

// --- StreamSession tests ---

func TestStreamSession_RecordSentReceived(t *testing.T) {
	t.Parallel()
	stream := NewBidirectionalStream(DefaultStreamConfig(), nil, &mockConn{}, nil, zap.NewNop())
	session := NewStreamSession(stream)

	session.RecordSent(100)
	session.RecordSent(200)
	session.RecordReceived(50)

	assert.Equal(t, int64(300), session.BytesSent)
	assert.Equal(t, int64(2), session.ChunksSent)
	assert.Equal(t, int64(50), session.BytesRecv)
	assert.Equal(t, int64(1), session.ChunksRecv)
}

func TestStreamSession_Concurrent(t *testing.T) {
	t.Parallel()
	stream := NewBidirectionalStream(DefaultStreamConfig(), nil, &mockConn{}, nil, zap.NewNop())
	session := NewStreamSession(stream)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); session.RecordSent(10) }()
		go func() { defer wg.Done(); session.RecordReceived(5) }()
	}
	wg.Wait()

	assert.Equal(t, int64(500), session.BytesSent)
	assert.Equal(t, int64(250), session.BytesRecv)
}

// --- StreamManager tests ---

func TestStreamManager_CreateGetClose(t *testing.T) {
	t.Parallel()
	mgr := NewStreamManager(zap.NewNop())
	conn := &mockConn{}
	cfg := DefaultStreamConfig()

	stream := mgr.CreateStream(cfg, nil, conn, nil)
	require.NotNil(t, stream)

	got, ok := mgr.GetStream(stream.ID)
	assert.True(t, ok)
	assert.Equal(t, stream.ID, got.ID)

	require.NoError(t, mgr.CloseStream(stream.ID))

	_, ok = mgr.GetStream(stream.ID)
	assert.False(t, ok)
}

func TestStreamManager_CloseNonexistent(t *testing.T) {
	t.Parallel()
	mgr := NewStreamManager(zap.NewNop())
	assert.NoError(t, mgr.CloseStream("nonexistent"))
}

// --- Adapter tests ---

func TestTextStreamAdapter_SendText(t *testing.T) {
	t.Parallel()
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, nil, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	adapter := NewTextStreamAdapter(stream)
	err := adapter.SendText("hello", false)
	require.NoError(t, err)

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

func TestAudioStreamAdapter_SendAudio(t *testing.T) {
	t.Parallel()
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, nil, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	adapter := NewAudioStreamAdapter(stream, 16000, 1)
	err := adapter.SendAudio([]byte{0, 1, 2, 3})
	require.NoError(t, err)

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

// --- Inbound/Outbound flow tests ---

func TestBidirectionalStream_InboundFlow_NoHandler(t *testing.T) {
	t.Parallel()
	chunks := []StreamChunk{
		{Type: StreamTypeText, Text: "hello", Sequence: 1},
		{Type: StreamTypeText, Text: "world", Sequence: 2},
	}
	idx := 0
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			if idx < len(chunks) {
				c := chunks[idx]
				idx++
				return &c, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, nil, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	ch := stream.Receive()
	got1 := <-ch
	assert.Equal(t, "hello", got1.Text)
	got2 := <-ch
	assert.Equal(t, "world", got2.Text)

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

func TestBidirectionalStream_InboundFlow_WithHandler(t *testing.T) {
	t.Parallel()
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			return &StreamChunk{Type: StreamTypeText, Text: "raw"}, nil
		},
	}
	handler := &mockHandler{
		onInboundFn: func(_ context.Context, chunk StreamChunk) (*StreamChunk, error) {
			chunk.Text = "processed:" + chunk.Text
			return &chunk, nil
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, handler, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	got := <-stream.Receive()
	assert.Equal(t, "processed:raw", got.Text)

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

func TestBidirectionalStream_InboundFlow_HeartbeatSkipped(t *testing.T) {
	t.Parallel()
	callCount := 0
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			callCount++
			if callCount == 1 {
				return &StreamChunk{Type: "heartbeat"}, nil
			}
			if callCount == 2 {
				return &StreamChunk{Type: StreamTypeText, Text: "data"}, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, nil, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	got := <-stream.Receive()
	assert.Equal(t, "data", got.Text)

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

func TestBidirectionalStream_InboundFlow_NilChunkSkipped(t *testing.T) {
	t.Parallel()
	callCount := 0
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			callCount++
			if callCount == 1 {
				return nil, nil
			}
			if callCount == 2 {
				return &StreamChunk{Type: StreamTypeText, Text: "real"}, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, nil, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	got := <-stream.Receive()
	assert.Equal(t, "real", got.Text)

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

func TestBidirectionalStream_InboundFlow_HandlerError(t *testing.T) {
	t.Parallel()
	callCount := 0
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			callCount++
			if callCount <= 2 {
				return &StreamChunk{Type: StreamTypeText, Text: "data"}, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	firstCall := true
	handler := &mockHandler{
		onInboundFn: func(_ context.Context, chunk StreamChunk) (*StreamChunk, error) {
			if firstCall {
				firstCall = false
				return nil, errors.New("handler error")
			}
			return &chunk, nil
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, handler, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	// First chunk causes handler error and is skipped; second chunk goes through
	got := <-stream.Receive()
	assert.Equal(t, "data", got.Text)

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

func TestBidirectionalStream_OutboundFlow_WithHandler(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var written []StreamChunk
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		writeFn: func(_ context.Context, chunk StreamChunk) error {
			mu.Lock()
			written = append(written, chunk)
			mu.Unlock()
			return nil
		},
	}
	handlerCalled := false
	handler := &mockHandler{
		onOutboundFn: func(_ context.Context, chunk StreamChunk) error {
			handlerCalled = true
			return nil
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, handler, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	require.NoError(t, stream.Send(StreamChunk{Type: StreamTypeText, Text: "out"}))
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.Len(t, written, 1)
	assert.Equal(t, "out", written[0].Text)
	mu.Unlock()
	assert.True(t, handlerCalled)

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

func TestBidirectionalStream_OutboundFlow_HandlerError(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var written []StreamChunk
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		writeFn: func(_ context.Context, chunk StreamChunk) error {
			mu.Lock()
			written = append(written, chunk)
			mu.Unlock()
			return nil
		},
	}
	handler := &mockHandler{
		onOutboundFn: func(_ context.Context, chunk StreamChunk) error {
			return errors.New("outbound handler error")
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, handler, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	require.NoError(t, stream.Send(StreamChunk{Type: StreamTypeText, Text: "out"}))
	time.Sleep(50 * time.Millisecond)

	// Handler error means chunk should NOT be written to connection
	mu.Lock()
	assert.Empty(t, written)
	mu.Unlock()

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

// --- ReceiveText / ReceiveAudio tests ---

func TestTextStreamAdapter_ReceiveText(t *testing.T) {
	t.Parallel()
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			return &StreamChunk{Type: StreamTypeText, Text: "hello"}, nil
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, nil, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	adapter := NewTextStreamAdapter(stream)
	ch := adapter.ReceiveText()
	got := <-ch
	assert.Equal(t, "hello", got)

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

func TestAudioStreamAdapter_ReceiveAudio(t *testing.T) {
	t.Parallel()
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			return &StreamChunk{Type: StreamTypeAudio, Data: []byte{1, 2, 3}}, nil
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, nil, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	adapter := NewAudioStreamAdapter(stream, 16000, 1)
	ch := adapter.ReceiveAudio()
	got := <-ch
	assert.Equal(t, []byte{1, 2, 3}, got)

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

// --- StreamReader tests ---

func TestStreamReader_Read(t *testing.T) {
	t.Parallel()
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			return &StreamChunk{Type: StreamTypeText, Data: []byte("hello world")}, nil
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, nil, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	reader := NewStreamReader(stream)
	buf := make([]byte, 5)
	n, err := reader.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(buf[:n]))

	// Read remaining from buffer
	n, err = reader.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, " worl", string(buf[:n]))

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}

// --- StreamReader/Writer tests ---

func TestStreamWriter_Write(t *testing.T) {
	t.Parallel()
	conn := &mockConn{
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	cfg := DefaultStreamConfig()
	cfg.EnableHeartbeat = false
	stream := NewBidirectionalStream(cfg, nil, conn, nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, stream.Start(ctx))

	writer := NewStreamWriter(stream)
	n, err := writer.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	cancel()
	time.Sleep(10 * time.Millisecond)
	stream.Close()
}
